// internal/vault/client.go
package vault

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sync"
	"vault-sync/internal/config"
	"vault-sync/pkg/converter"
	"vault-sync/pkg/log"

	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/rs/zerolog"
)

type MultiClusterVaultClient struct {
	mainCluster     *clusterManager
	replicaClusters map[string]*clusterManager
	mu              sync.RWMutex
}

type clusterManager struct {
	client *vault.Client
	config config.VaultConfig
}

func NewMultiClusterVaultClient(ctx context.Context, mainConfig config.MainCluster, replicasConfig []config.ReplicaCluster) (*MultiClusterVaultClient, error) {
	mainClient, err := newClusterManager(mainConfig.MapToVaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create main cluster client: %w", err)
	}
	err = mainClient.authenticate(ctx)
	if err != nil {
		return nil, err
	}

	multiClusterClient := &MultiClusterVaultClient{
		mainCluster:     mainClient,
		replicaClusters: make(map[string]*clusterManager),
		mu:              sync.RWMutex{},
	}

	for _, replicaCfg := range replicasConfig {
		replicaClient, err := newClusterManager(replicaCfg.MapToVaultConfig())
		if err != nil {
			return nil, fmt.Errorf("failed to create replica cluster client %s: %w", replicaCfg.Name, err)
		}
		err = replicaClient.authenticate(ctx)
		if err != nil {
			return nil, err
		}
		multiClusterClient.replicaClusters[replicaCfg.Name] = replicaClient
	}

	return multiClusterClient, nil
}

func newClusterManager(cfg config.VaultConfig) (*clusterManager, error) {
	tlsConfig := vault.TLSConfiguration{
		InsecureSkipVerify: cfg.TLSSkipVerify,
	}
	if cfg.TLSCertFile != "" {
		tlsConfig = vault.TLSConfiguration{
			InsecureSkipVerify: cfg.TLSSkipVerify,
			ServerCertificate: vault.ServerCertificateEntry{
				FromFile: cfg.TLSCertFile,
			},
		}
	}

	client, err := vault.New(
		vault.WithAddress(cfg.Address),
		vault.WithTLS(tlsConfig),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create Vault client: %w", err)
	}

	return &clusterManager{
		client: client,
		config: cfg,
	}, nil
}

// GetSecretMounts retrieves the secret mounts for the given secret paths
// It checks if the mounts exist in all clusters (main and replicas).
func (mc *MultiClusterVaultClient) GetSecretMounts(ctx context.Context, secretPaths []string) ([]string, error) {
	mounts := extractMountsFromPaths(secretPaths)
	if len(mounts) == 0 {
		log.Logger.Error().Strs("secret_paths", secretPaths).Msg("No valid mounts found in provided secret paths")
		return nil, fmt.Errorf("no valid mounts found in provided secret paths")
	}

	if missing, err := checkMounts(ctx, mc.mainCluster, "main", mounts); err != nil {
		return nil, err
	} else if len(missing) > 0 {
		return nil, fmt.Errorf("missing mounts in main cluster: %v", missing)
	}

	mc.mu.RLock()
	defer mc.mu.RUnlock()
	for name, cm := range mc.replicaClusters {
		if missing, err := checkMounts(ctx, cm, name, mounts); err != nil {
			return nil, err
		} else if len(missing) > 0 {
			return nil, fmt.Errorf("missing mounts in replica cluster %s: %v", name, missing)
		}
	}

	log.Logger.Info().Strs("validated_mounts", mounts).Msg("All secret mounts exist in all clusters")
	return mounts, nil
}

func checkMounts(ctx context.Context, cm *clusterManager, clusterName string, mounts []string) ([]string, error) {
	if err := cm.ensureValidToken(ctx); err != nil {
		cm.decorateLog(log.Logger.Error, "check_mounts").
			Err(err).
			Str("cluster", clusterName).
			Msg("Failed to ensure valid token")
		return nil, fmt.Errorf("failed to ensure valid token for cluster %s: %w", clusterName, err)
	}

	existingMounts, err := cm.getExistingMounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing mounts for cluster %s: %w", clusterName, err)
	}

	var missingMounts []string
	for _, mount := range mounts {
		if !existingMounts[mount] {
			missingMounts = append(missingMounts, mount)
		}
	}

	if len(missingMounts) > 0 {
		cm.decorateLog(log.Logger.Error, "check_mounts").
			Str("cluster", clusterName).
			Strs("missing_mounts", missingMounts).
			Strs("existing_mounts", converter.MapKeysToSlice(existingMounts)).
			Msg("Some secret mounts do not exist in cluster")
		return missingMounts, nil
	}
	return nil, nil
}

// getExistingMounts retrieves the existing secret mounts from Vault
// It returns a map where keys are mount paths and values are true.
// The mount paths are cleaned to remove trailing slashes.
func (cm *clusterManager) getExistingMounts(ctx context.Context) (map[string]bool, error) {
	resp, err := cm.client.System.MountsListSecretsEngines(ctx)
	if err != nil {
		cm.decorateLog(log.Logger.Error, "get_existing_mounts").
			Err(err).
			Msg("Failed to list secret engines")
		return nil, fmt.Errorf("failed to list secret engines: %w", err)
	}

	existingMounts := make(map[string]bool)
	for mountPath := range resp.Data {
		cleanMountPath := strings.TrimSuffix(mountPath, "/")
		existingMounts[cleanMountPath] = true
	}

	cm.decorateLog(log.Logger.Debug, "get_existing_mounts").
		Strs("mount_paths", converter.MapKeysToSlice(existingMounts)).
		Msg("Found existing mounts")
	return existingMounts, nil
}

// extractMountsFromPaths extracts unique mount paths from secret paths
// Examples:
//
//	["kv/myapp/database", "secret/myapp/config"] -> ["kv", "secret"]
func extractMountsFromPaths(secretPaths []string) []string {
	mountSet := make(map[string]bool)

	for _, path := range secretPaths {
		mount := extractMountFromPath(path)
		if mount != "" {
			mountSet[mount] = true
		}
	}

	return converter.MapKeysToSlice(mountSet)
}

// extractMountFromPath extracts the mount path from a given secret path
// It assumes the mount is the first segment of the path.
// Examples:
//	"/kv/myapp/database" -> "kv"

func extractMountFromPath(secretPath string) string {
	secretPath = strings.TrimPrefix(secretPath, "/")
	parts := strings.Split(secretPath, "/")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return ""
}

// authenticate authenticates the cluster manager with Vault using AppRole
// It sets the client token on success.
func (cm *clusterManager) authenticate(ctx context.Context) error {
	cm.decorateLog(log.Logger.Info, "authenticate").Msg("Authenticating with Vault")

	res, err := cm.client.Auth.AppRoleLogin(
		ctx,
		schema.AppRoleLoginRequest{
			RoleId:   cm.config.AppRoleID,
			SecretId: cm.config.AppRoleSecret,
		},
		vault.WithMountPath(cm.config.AppRoleMount),
	)
	if err != nil {
		cm.decorateLog(log.Logger.Error, "authenticate").Err(err)
		return fmt.Errorf("failed to authenticate with role ID: %s at mount %s. (%w)", cm.config.AppRoleID, cm.config.AppRoleMount, err)
	}
	if err := cm.client.SetToken(res.Auth.ClientToken); err != nil {
		cm.decorateLog(log.Logger.Error, "authenticate").Err(err)
		return fmt.Errorf("failed to set client token: %w", err)
	}
	return nil
}

const (
	fiveMinutes = 5 * time.Minute
)

// ensureValidToken checks if the Vault token is valid and has sufficient TTL.
// If the token is invalid or has low TTL, it re-authenticates.
func (cm *clusterManager) ensureValidToken(ctx context.Context) error {
	reauthenticate := func(msg string, ttlSeconds int64, err error) error {
		cm.decorateLog(log.Logger.Warn, "ensure_valid_token").
			Int64("ttl_seconds", ttlSeconds).
			Err(err).
			Msg(msg)
		return cm.authenticate(ctx)
	}

	cm.decorateLog(log.Logger.Debug, "ensure_valid_token").Msg("Ensuring Vault token is valid")

	resp, err := cm.client.Auth.TokenLookUpSelf(ctx)
	if err != nil {
		return reauthenticate("Failed to look up token, re-authenticating", 0, err)
	}

	data := resp.Data
	if ttlInterface, ok := data["ttl"]; ok {
		ttlSeconds, err := converter.ConvertInterfaceToInt64(ttlInterface)
		if err != nil {
			return reauthenticate("Could not parse token TTL, re-authenticating", 0, err)
		}

		fiveMinutesInSeconds, _ := converter.ConvertInterfaceToInt64(fiveMinutes.Seconds())
		if ttlSeconds < fiveMinutesInSeconds {
			return reauthenticate("Token TTL is low, re-authenticating", ttlSeconds, nil)
		}

		cm.decorateLog(log.Logger.Debug, "ensure_valid_token").
			Int64("ttl_seconds", ttlSeconds).
			Msg("Token is valid")

		return nil
	}

	return reauthenticate("Could not determine token TTL, re-authenticating", 0, nil)
}

func (cm *clusterManager) decorateLog(eventFactory func() *zerolog.Event, event string) *zerolog.Event {
	return eventFactory().Str("app_role", cm.config.AppRoleID).
		Str("app_role_mount", cm.config.AppRoleMount).
		Str("vault_address", cm.config.Address).
		Str("event", event)
}
