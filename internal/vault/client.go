// internal/vault/client.go
package vault

import (
	"context"
	"fmt"
	"strings"

	"sync"
	"vault-sync/internal/config"
	"vault-sync/internal/models"
	"vault-sync/pkg/converter"
	"vault-sync/pkg/log"
)

type MultiClusterVaultClient struct {
	mainCluster     *clusterManager
	replicaClusters map[string]*clusterManager
	mu              sync.RWMutex
}

func NewMultiClusterVaultClient(ctx context.Context, mainConfig *config.MainCluster, replicasConfig []*config.ReplicaCluster) (*MultiClusterVaultClient, error) {
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

// GetSecretMounts retrieves the secret mounts for the given secret paths
// It checks if the mounts exist in all clusters (main and replicas).
func (mc *MultiClusterVaultClient) GetSecretMounts(ctx context.Context, secretPaths []string) ([]string, error) {
	mounts := extractMountsFromPaths(secretPaths)
	if len(mounts) == 0 {
		log.Logger.Error().Strs("secret_paths", secretPaths).Msg("No valid mounts found in provided secret paths")
		return nil, fmt.Errorf("no valid mounts found in provided secret paths")
	}

	if missing, err := mc.mainCluster.checkMounts(ctx, "main", mounts); err != nil {
		return nil, err
	} else if len(missing) > 0 {
		return nil, fmt.Errorf("missing mounts in main cluster: %v", missing)
	}

	mc.mu.RLock()
	defer mc.mu.RUnlock()
	for name, cm := range mc.replicaClusters {
		if missing, err := cm.checkMounts(ctx, name, mounts); err != nil {
			return nil, err
		} else if len(missing) > 0 {
			return nil, fmt.Errorf("missing mounts in replica cluster %s: %v", name, missing)
		}
	}

	log.Logger.Info().Strs("validated_mounts", mounts).Msg("All secret mounts exist in all clusters")
	return mounts, nil
}

// GetKeysUnderMount retrieves all available keys (using path format) under a given mount from the main cluster.
// This operation is only performed on the main cluster as it's used for discovery purposes.
func (mc *MultiClusterVaultClient) GetKeysUnderMount(ctx context.Context, mount string) ([]string, error) {
	if mount == "" {
		return nil, fmt.Errorf("mount cannot be empty")
	}

	log.Logger.Debug().
		Str("mount", mount).
		Str("event", "get_keys_under_mount").
		Msg("Retrieving all keys under mount from main cluster")

	keys, err := mc.mainCluster.fetchKeysUnderMount(ctx, mount)
	if err != nil {
		return nil, fmt.Errorf("failed to get keys under mount %s: %w", mount, err)
	}

	log.Logger.Info().
		Str("mount", mount).
		Str("event", "get_keys_under_mount").
		Int("key_count", len(keys)).
		Msg("Successfully retrieved keys from main cluster")

	return keys, nil
}

// GetSecretMetadata retrieves metadata for a secret at the given mount and key path from the main cluster.
// This operation is only performed on the main cluster as it's used for discovery and version management.
// Returns metadata including version information, creation time, and deletion status.
func (mc *MultiClusterVaultClient) GetSecretMetadata(ctx context.Context, mount, keyPath string) (*models.VaultSecretMetadata, error) {
	if mount == "" {
		log.Logger.Error().Str("event", "get_secret_metadata").Msg("Mount cannot be empty")
		return nil, fmt.Errorf("mount cannot be empty")
	}
	if keyPath == "" {
		log.Logger.Error().Str("event", "get_secret_metadata").Msg("Key path cannot be empty")

		return nil, fmt.Errorf("key path cannot be empty")
	}

	log.Logger.Debug().
		Str("mount", mount).
		Str("key_path", keyPath).
		Str("event", "get_secret_metadata").
		Msg("Retrieving secret metadata from main cluster")

	metadata, err := mc.mainCluster.fetchSecretMetadata(ctx, mount, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for %s/%s: %w", mount, keyPath, err)
	}

	log.Logger.Info().
		Str("mount", mount).
		Str("key_path", keyPath).
		Str("event", "get_secret_metadata").
		Int64("current_version", metadata.CurrentVersion).
		Int("version_count", len(metadata.Versions)).
		Msg("Successfully retrieved secret metadata from main cluster")

	return metadata, nil
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
