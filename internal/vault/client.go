// internal/vault/client.go
package vault

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

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
	logger := mc.logger.With().
		Str("event", "get_secret_metadata").
		Str("mount", mount).
		Str("key_path", keyPath).
		Logger()

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
func (mc *MultiClusterVaultClient) GetKeysUnderMount(ctx context.Context, mount string) ([]string, error) {
	if mount == "" {
		return nil, fmt.Errorf("mount cannot be empty")
	}

	logger := mc.logger.With().
		Str("event", "get_keys_under_mount").
		Str("mount", mount).
		Logger()

	logger.Debug().Msg("Retrieving all keys under mount from main cluster")
	keys, err := mc.mainCluster.fetchKeysUnderMount(ctx, mount)
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
	logger := mc.logger.With().
		Str("event", "sync_secret_to_replicas").
		Str("mount", mount).
		Str("key_path", keyPath).
		Logger()

	if err := validateMountAndKeyPath(mount, keyPath); err != nil {
		logger.Error().Err(err).Msg("Invalid mount or key path")
		return nil, err
	}

	syncAttemptTime := time.Now()
	logger.Debug().Time("sync_attempt_time", syncAttemptTime).Msg("Starting secret synchronization from main cluster to replicas")

	sourceSecret, err := mc.readSecretFromMainCluster(ctx, mount, keyPath)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to read secret from main cluster")
		return nil, fmt.Errorf("failed to read secret from main cluster %s/%s: %w", mount, keyPath, err)
	}

	replicaCount := len(mc.replicaClusters)
	if replicaCount == 0 {
		logger.Warn().Msg("No replica clusters configured, skipping synchronization")
		return []*models.SyncedSecret{}, nil
	}

	results := make([]*models.SyncedSecret, 0, replicaCount)
	resultsChan := make(chan *models.SyncedSecret, replicaCount)

	for clusterName := range mc.replicaClusters {
		go mc.syncToSingleReplica(
			ctx,
			clusterName,
			mount,
			keyPath,
			sourceSecret,
			syncAttemptTime,
			resultsChan,
		)
	}

	for i := 0; i < replicaCount; i++ {
		select {
		case result := <-resultsChan:
			results = append(results, result)
		case <-ctx.Done():
			return nil, fmt.Errorf("synchronization cancelled: %w", ctx.Err())
		}
	}

	slices.SortStableFunc(results, func(a, b *models.SyncedSecret) int {
		if a.DestinationCluster < b.DestinationCluster {
			return -1
		} else if a.DestinationCluster > b.DestinationCluster {
			return 1
		}
		return 0
	})

	successCount := 0
	failureCount := 0
	for _, result := range results {
		if result.Status == models.StatusSuccess {
			successCount++
		} else {
			failureCount++
		}
	}

	logger.Info().
		Int("success_count", successCount).
		Int("failure_count", failureCount).
		Msg("Secret synchronization completed")

	return results, nil
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
//
//	"/kv/myapp/database" -> "kv"
func extractMountFromPath(secretPath string) string {
	secretPath = strings.TrimPrefix(secretPath, "/")
	parts := strings.Split(secretPath, "/")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return ""
}

// validateMountAndKeyPath validates that mount and keyPath are not empty
func validateMountAndKeyPath(mount, keyPath string) error {
	if mount == "" {
		return fmt.Errorf("mount cannot be empty")
	}
	if keyPath == "" {
		return fmt.Errorf("key path cannot be empty")
	}
	return nil
}

// readSecretFromMainCluster reads both secret data and metadata from the main cluster
func (mc *MultiClusterVaultClient) readSecretFromMainCluster(ctx context.Context, mount, keyPath string) (*VaultSecretResponse, error) {
	logger := mc.logger.With().
		Str("event", "read_secret_main_cluster").
		Str("mount", mount).
		Str("key_path", keyPath).
		Str("cluster", "main").
		Logger()

	logger.Debug().Msg("Reading secret")
	secretResponse, err := mc.mainCluster.readSecret(ctx, mount, keyPath)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to read secret data")
		return nil, fmt.Errorf("failed to read secret data: %w", err)
	}

	return secretResponse, nil
}

// syncToSingleReplica synchronizes a secret to a single replica cluster
func (mc *MultiClusterVaultClient) syncToSingleReplica(
	ctx context.Context,
	clusterName string,
	mount, keyPath string,
	sourceSecret *VaultSecretResponse,
	syncAttemptTime time.Time,
	resultsChan chan<- *models.SyncedSecret,
) {
	syncResult := &models.SyncedSecret{
		SecretBackend:      mount,
		SecretPath:         keyPath,
		SourceVersion:      sourceSecret.Metadata.Version,
		DestinationCluster: clusterName,
		LastSyncAttempt:    syncAttemptTime,
		Status:             models.StatusPending,
	}

	defer func() {
		select {
		case resultsChan <- syncResult:
		case <-ctx.Done():
		}
	}()

	logger := mc.logger.With().
		Str("event", "sync_to_single_replica").
		Str("cluster", clusterName).
		Str("mount", mount).
		Str("key_path", keyPath).
		Logger()

	logger.Debug().Msg("Starting synchronization to replica cluster")
	clusterManager := mc.replicaClusters[clusterName]
	destinationVersion, err := clusterManager.writeSecret(ctx, mount, keyPath, sourceSecret.Data)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to write secret to replica cluster")
		syncResult.Status = models.StatusFailed
		errorMsg := fmt.Sprintf("failed to write secret to cluster %s: %v", clusterName, err)
		syncResult.ErrorMessage = &errorMsg
		return
	}

	if destinationVersion < 0 {
		logger.Error().Int64("destination_version", destinationVersion).Msg("Invalid destination version after write")
		syncResult.Status = models.StatusFailed
		errorMsg := fmt.Sprintf("invalid destination version %d after write to cluster %s", destinationVersion, clusterName)
		syncResult.ErrorMessage = &errorMsg
		return
	}

	syncResult.DestinationVersion = destinationVersion
	syncResult.Status = models.StatusSuccess
	now := time.Now()
	syncResult.LastSyncSuccess = &now

	logger.Debug().
		Int64("source_version", sourceSecret.Metadata.Version).
		Int64("destination_version", destinationVersion).
		Msg("Successfully synchronized secret to replica cluster")
}
