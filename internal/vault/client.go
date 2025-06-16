// internal/vault/client.go
package vault

import (
	"context"
	"fmt"
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
