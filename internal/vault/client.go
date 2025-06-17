// internal/vault/client.go
package vault

import (
	"context"
	"fmt"
	"strings"

	"sync"
	"vault-sync/internal/config"
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

// checkMounts checks if the specified mounts exist in the Vault cluster.
// It returns a slice of missing mounts if any are not found.
func checkMounts(ctx context.Context, cm *clusterManager, clusterName string, mounts []string) ([]string, error) {
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
