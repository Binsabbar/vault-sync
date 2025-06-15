// internal/vault/client.go
package vault

import (
	"context"
	"fmt"

	"sync"
	"vault-sync/internal/config"
	"vault-sync/pkg/log"

	"github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
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
	log.Logger.Info().Str("app_role", cm.config.AppRoleID).
		Str("app_role_mount", cm.config.AppRoleMount).
		Str("vault_address", cm.config.Address).
		Msg("Authenticating with Vault using AppRole")

	res, err := cm.client.Auth.AppRoleLogin(
		ctx,
		schema.AppRoleLoginRequest{
			RoleId:   cm.config.AppRoleID,
			SecretId: cm.config.AppRoleSecret,
		},
		vault.WithMountPath(cm.config.AppRoleMount),
	)
	if err != nil {
		return fmt.Errorf("failed to authenticate with role ID: %s at mount %s. (%w)", cm.config.AppRoleID, cm.config.AppRoleMount, err)
	}
	if err := cm.client.SetToken(res.Auth.ClientToken); err != nil {
		return fmt.Errorf("failed to set client token: %w", err)
	}
	return nil
}
