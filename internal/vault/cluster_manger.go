package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"vault-sync/internal/config"
	"vault-sync/internal/models"
	"vault-sync/pkg/converter"
	"vault-sync/pkg/log"

	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/rs/zerolog"
)

type clusterManager struct {
	client *vault.Client
	config *config.VaultConfig
}

func newClusterManager(cfg *config.VaultConfig) (*clusterManager, error) {
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
		ttlSeconds, err := ttlInterface.(json.Number).Int64()
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

// checkMounts checks if the specified mounts exist in the Vault cluster.
// It returns a slice of missing mounts if any are not found.
func (cm *clusterManager) checkMounts(ctx context.Context, clusterName string, mounts []string) ([]string, error) {
	if err := cm.ensureValidToken(ctx); err != nil {
		cm.decorateLog(log.Logger.Error, "check_mounts").
			Err(err).
			Str("cluster", clusterName).
			Msg("Failed to ensure valid token")
		return nil, err
	}

	existingMounts, err := cm.getExistingMounts(ctx)
	if err != nil {
		return nil, err
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

// fetchKeysUnderMount retrieves all keys under a given mount from a specific cluster
func (cm *clusterManager) fetchKeysUnderMount(ctx context.Context, mount string) ([]string, error) {
	if err := cm.ensureValidToken(ctx); err != nil {
		cm.decorateLog(log.Logger.Error, "fetch_keys_under_mount").
			Str("mount", mount).
			Err(err).
			Msg("Failed to ensure valid token")
		return nil, err
	}

	cm.decorateLog(log.Logger.Debug, "fetch_keys_under_mount").
		Str("mount", mount).
		Msg("Listing keys under mount")

	var allKeys []string
	err := cm.listKeysRecursively(ctx, mount, "", &allKeys)
	if err != nil {
		cm.decorateLog(log.Logger.Error, "fetch_keys_under_mount").
			Err(err).
			Str("mount", mount).
			Msg("Failed to list keys recursively")
		return nil, err
	}

	cm.decorateLog(log.Logger.Debug, "fetch_keys_under_mount").
		Str("mount", mount).
		Int("key_count", len(allKeys)).
		Strs("keys", allKeys).
		Msg("Successfully retrieved all keys")

	return allKeys, nil
}

// listKeysRecursively recursively lists all keys under a path
func (cm *clusterManager) listKeysRecursively(ctx context.Context, mount, currentPath string, allKeys *[]string) error {
	listPath := ""
	if currentPath != "" {
		listPath = currentPath
	}

	resp, err := cm.client.Secrets.KvV2List(ctx, listPath, vault.WithMountPath(mount))
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "no such path") {
			return nil
		}
		return fmt.Errorf("failed to list path %s: %w", listPath, err)
	}

	if resp.Data.Keys == nil {
		return nil
	}

	for _, key := range resp.Data.Keys {
		keyPath := strings.TrimSuffix(key, mount)
		if currentPath != "" {
			keyPath = fmt.Sprintf("%s/%s", currentPath, key)
		}

		// If key ends with '/', it's a directory - recurse into it
		if strings.HasSuffix(key, "/") {
			dirPath := strings.TrimSuffix(keyPath, "/")
			err := cm.listKeysRecursively(ctx, mount, dirPath, allKeys)
			if err != nil {
				return err
			}
		} else {
			keyPath = strings.TrimPrefix(keyPath, mount+"/")
			*allKeys = append(*allKeys, keyPath)
		}
	}

	return nil
}

// fetchSecretMetadata retrieves metadata for a secret at the given mount and key path
func (cm *clusterManager) fetchSecretMetadata(ctx context.Context, mount, keyPath string) (*models.VaultSecretMetadata, error) {
	if err := cm.ensureValidToken(ctx); err != nil {
		cm.decorateLog(log.Logger.Error, "fetch_secret_metadata").
			Err(err).
			Str("mount", mount).
			Str("key_path", keyPath).
			Msg("Failed to ensure valid token")
		return nil, err
	}

	cm.decorateLog(log.Logger.Debug, "fetch_secret_metadata").
		Str("mount", mount).
		Str("key_path", keyPath).
		Msg("Fetching secret metadata")

	resp, err := cm.client.Secrets.KvV2ReadMetadata(ctx, keyPath, vault.WithMountPath(mount))
	if err != nil {
		cm.decorateLog(log.Logger.Error, "fetch_secret_metadata").
			Err(err).
			Str("mount", mount).
			Str("key_path", keyPath).
			Msg("Failed to read secret metadata")
		return nil, fmt.Errorf("failed to read metadata from %s: %w", keyPath, err)
	}

	metadata, err := models.ParseKvV2ReadMetadataResponseToVaultSecretMetadata(resp.Data)
	if err != nil {
		cm.decorateLog(log.Logger.Error, "fetch_secret_metadata").
			Err(err).
			Str("mount", mount).
			Str("key_path", keyPath).
			Msg("Failed to parse secret metadata")
		return nil, fmt.Errorf("failed to parse metadata for %s/%s: %w", mount, keyPath, err)
	}

	cm.decorateLog(log.Logger.Debug, "fetch_secret_metadata").
		Str("mount", mount).
		Str("key_path", keyPath).
		Int64("current_version", metadata.CurrentVersion).
		Int("version_count", len(metadata.Versions)).
		Msg("Successfully fetched secret metadata")

	return metadata, nil
}

// decorateLog adds common fields to the log event for cluster manager operations
// It returns a new log event with the specified event factory and event name.
func (cm *clusterManager) decorateLog(eventFactory func() *zerolog.Event, event string) *zerolog.Event {
	return eventFactory().Str("app_role", cm.config.AppRoleID).
		Str("app_role_mount", cm.config.AppRoleMount).
		Str("vault_address", cm.config.Address).
		Str("event", event)
}
