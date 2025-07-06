// internal/vault/client.go
package vault

import (
	"context"
	"fmt"

	"vault-sync/internal/config"
	"vault-sync/internal/models"
	"vault-sync/pkg/converter"
	"vault-sync/pkg/log"

	"github.com/rs/zerolog"
)

type MultiClusterVaultClient struct {
	mainCluster     *clusterManager
	replicaClusters map[string]*clusterManager
	logger          zerolog.Logger
}

func NewMultiClusterVaultClient(ctx context.Context, mainConfig *config.VaultClusterConfig, replicasConfig []*config.VaultClusterConfig) (*MultiClusterVaultClient, error) {
	mainClient, err := newClusterManager(mainConfig)
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
		logger:          log.Logger.With().Str("component", "multi_cluster_vault_client").Logger(),
	}

	for _, replicaCfg := range replicasConfig {
		replicaClient, err := newClusterManager(replicaCfg)
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

// GetSecretMounts retrieves the secret mounts for the given secret paths
// It checks if the mounts exist in all clusters (main and replicas).
func (mc *MultiClusterVaultClient) GetSecretMounts(ctx context.Context, secretPaths []string) ([]string, error) {
	logger := mc.logger.With().
		Str("event", "get_secret_mounts").
		Strs("secret_paths", secretPaths).
		Logger()

	mounts := extractMountsFromPaths(secretPaths)
	if len(mounts) == 0 {
		logger.Error().Msg("No valid mounts found in provided secret paths")
		return nil, fmt.Errorf("no valid mounts found in provided secret paths")
	}

	if missing, err := mc.mainCluster.checkMounts(ctx, "main", mounts); err != nil {
		return nil, err
	} else if len(missing) > 0 {
		logger.Error().Strs("missing_mounts", missing).Msg("Missing mounts in main cluster")
		return nil, fmt.Errorf("missing mounts in main cluster: %v", missing)
	}

	for name, cm := range mc.replicaClusters {
		if missing, err := cm.checkMounts(ctx, name, mounts); err != nil {
			return nil, err
		} else if len(missing) > 0 {
			logger.Error().Str("replica_cluster", name).
				Strs("missing_mounts", missing).
				Msg("Missing mounts in replica cluster")
			return nil, fmt.Errorf("missing mounts in replica cluster %s: %v", name, missing)
		}
	}

	logger.Info().
		Strs("validated_mounts", mounts).
		Msg("All secret mounts exist in all clusters")
	return mounts, nil
}

// GetSecretMetadata retrieves metadata for a secret at the given mount and key path from the main cluster.
// This operation is only performed on the main cluster as it's used for discovery and version management.
// Returns metadata including version information, creation time, and deletion status.
func (mc *MultiClusterVaultClient) GetSecretMetadata(ctx context.Context, mount, keyPath string) (*VaultSecretMetadataResponse, error) {
	logger := mc.createOperationLogger("get_secret_metadata", mount, keyPath)

	logger.Debug().Msg("Retrieving secret metadata from main cluster")
	if err := validateMountAndKeyPath(mount, keyPath); err != nil {
		logger.Error().Msg("Invalid mount or key path")
		return nil, err
	}

	logger.Debug().Msg("Retrieving secret metadata from main cluster")
	metadata, err := mc.mainCluster.fetchSecretMetadata(ctx, mount, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for %s/%s: %w", mount, keyPath, err)
	}

	logger.Info().
		Int64("current_version", metadata.CurrentVersion).
		Int("version_count", len(metadata.Versions)).
		Msg("Successfully retrieved secret metadata from main cluster")

	return metadata, nil
}

// GetKeysUnderMount retrieves all available keys (using path format) under a given mount from the main cluster.
// This operation is only performed on the main cluster as it's used for discovery purposes.
func (mc *MultiClusterVaultClient) GetKeysUnderMount(ctx context.Context, mount string, shouldIncludeKeyPath func(path string) bool) ([]string, error) {
	logger := mc.createOperationLogger("get_keys_under_mount", mount, "")
	if mount == "" {
		logger.Error().Msg("mount cannot be empty")
		return nil, fmt.Errorf("mount cannot be empty")
	}

	logger.Debug().Msg("Retrieving all keys under mount from main cluster")
	keys, err := mc.mainCluster.fetchKeysUnderMount(ctx, mount, shouldIncludeKeyPath)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to retrieve keys under mount")
		return nil, fmt.Errorf("failed to get keys under mount %s: %w", mount, err)
	}

	logger.Info().Int("key_count", len(keys)).Msg("Successfully retrieved keys from main cluster")
	return keys, nil
}

// SyncSecretToReplicas reads a secret from the main cluster and synchronizes it to all replica clusters.
// It returns a list of SyncedSecret objects representing the sync status for each replica.
// The method handles version conflicts, missing secrets, and per-replica failures gracefully.
func (mc *MultiClusterVaultClient) SyncSecretToReplicas(ctx context.Context, mount, keyPath string) ([]*models.SyncedSecret, error) {
	logger := mc.createOperationLogger("sync_secret_to_replicas", mount, keyPath)

	if err := validateMountAndKeyPath(mount, keyPath); err != nil {
		logger.Error().Err(err).Msg("Invalid mount or key path")
		return nil, err
	}

	logger.Debug().Msg("Starting secret synchronization from main cluster to replicas")
	sourceSecret, err := mc.readSecretFromMainCluster(ctx, mount, keyPath)
	if err != nil {
		return nil, err
	}

	replicaHandler := replicaSyncHandler[*models.SyncedSecret]{
		operationType: operationTypeSync,
		ctx:           ctx,
		logger:        &logger,
		sourceVersion: sourceSecret.Metadata.Version,
		clusters:      mc.getReplicaNames(),
		mount:         mount,
		keyPath:       keyPath,
		operationFunc: mc.syncSecretFuncFactory(sourceSecret.Data),
	}

	results, err := replicaHandler.executeSync()
	if err != nil {
		return nil, err
	}

	return results, nil
}

// DeleteSecretFromReplicas deletes a secret from all replica clusters for the given mount and key path.
// It does not fail if the secret doesn't exist in the replicas, but logs the fact.
func (mc *MultiClusterVaultClient) DeleteSecretFromReplicas(ctx context.Context, mount, keyPath string) ([]*models.SyncSecretDeletionResult, error) {
	logger := mc.createOperationLogger("delete_secret_from_replicas", mount, keyPath)

	if err := validateMountAndKeyPath(mount, keyPath); err != nil {
		logger.Error().Err(err).Msg("Invalid mount or key path")
		return nil, err
	}

	logger.Debug().Msg("Starting secret deletion from replica clusters")
	replicaHandler := replicaSyncHandler[*models.SyncSecretDeletionResult]{
		operationType: operationTypeDelete,
		ctx:           ctx,
		logger:        &logger,
		sourceVersion: 0,
		clusters:      mc.getReplicaNames(),
		mount:         mount,
		keyPath:       keyPath,
		operationFunc: mc.deleteSecretFuncFactory(),
	}

	results, err := replicaHandler.executeSync()
	if err != nil {
		return nil, err
	}

	return results, nil
}

// readSecretFromMainCluster reads both secret data and metadata from the main cluster
func (mc *MultiClusterVaultClient) readSecretFromMainCluster(ctx context.Context, mount, keyPath string) (*VaultSecretResponse, error) {
	logger := mc.createOperationLogger("read_secret_main_cluster", mount, keyPath).With().Str("cluster", "main").Logger()

	logger.Debug().Msg("Reading secret")
	secretResponse, err := mc.mainCluster.readSecret(ctx, mount, keyPath)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to read secret data")
		return nil, err
	}

	return secretResponse, nil
}

func (mc *MultiClusterVaultClient) syncSecretFuncFactory(secretData map[string]interface{}) syncOperationFunc[*models.SyncedSecret] {
	secretDataCopy := converter.DeepCopy(secretData)
	return func(ctx context.Context, mount, keyPath, clusterName string, result *models.SyncedSecret) error {
		destinationVersion, err := mc.replicaClusters[clusterName].writeSecret(ctx, mount, keyPath, secretDataCopy)
		result.Status = models.StatusSuccess
		result.DestinationVersion = destinationVersion
		return err
	}
}

func (mc *MultiClusterVaultClient) deleteSecretFuncFactory() syncOperationFunc[*models.SyncSecretDeletionResult] {
	return func(ctx context.Context, mount, keyPath, clusterName string, result *models.SyncSecretDeletionResult) error {
		err := mc.replicaClusters[clusterName].deleteSecret(ctx, mount, keyPath)
		result.Status = models.StatusSuccess
		return err
	}
}

func (mc *MultiClusterVaultClient) createOperationLogger(operation, mount, keyPath string) zerolog.Logger {
	logger := mc.logger.With().Str("operation", operation)
	if mount != "" {
		logger = logger.Str("mount", mount)
	}
	if keyPath != "" {
		logger = logger.Str("key_path", keyPath)
	}
	return logger.Logger()
}

func (mc *MultiClusterVaultClient) getReplicaNames() []string {
	names := make([]string, 0, len(mc.replicaClusters))
	for name := range mc.replicaClusters {
		names = append(names, name)
	}
	return names
}
