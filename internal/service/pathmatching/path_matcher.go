package pathmatching

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"vault-sync/internal/config"
	"vault-sync/internal/vault"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// SecretPath represents a secret location
type SecretPath struct {
	Mount   string `json:"mount"`
	KeyPath string `json:"key_path"`
}

func (sp SecretPath) String() string {
	return fmt.Sprintf("%s/%s", sp.Mount, sp.KeyPath)
}

// PathMatcher discovers and filters secrets based on sync rules
type PathMatcher interface {
	// DiscoverSecretsForSync finds all secrets that should be synced based on sync rules
	DiscoverSecretsForSync(ctx context.Context) ([]SecretPath, error)

	// DiscoverFromMounts finds all secrets in specific mount points
	DiscoverFromMounts(ctx context.Context, mounts []string) ([]SecretPath, error)

	// ShouldSync checks if a secret path should be synced based on sync rules
	ShouldSync(mount, keyPath string) bool
}

type VaultPathMatcher struct {
	vaultClient vault.VaultSyncer
	syncRule    *config.SyncRule
	cpm         *CorePathMatcher
	logger      zerolog.Logger
}

func NewVaultPathMatcher(vaultClient vault.VaultSyncer, syncRule *config.SyncRule) *VaultPathMatcher {
	return &VaultPathMatcher{
		vaultClient: vaultClient,
		syncRule:    syncRule,
		cpm:         NewCorePathMatcher(syncRule),
		logger: log.Logger.With().
			Str("component", "path_matcher").
			Logger(),
	}
}

func (pm *VaultPathMatcher) DiscoverSecretsForSync(ctx context.Context) ([]SecretPath, error) {
	logger := pm.logger.With().Str("action", "discover_secrets_for_sync").Logger()
	logger.Debug().
		Strs("kv_mounts", pm.syncRule.KvMounts).
		Strs("paths_to_replicate", pm.syncRule.PathsToReplicate).
		Strs("paths_to_ignore", pm.syncRule.PathsToIgnore).
		Msg("Starting secret discovery based on sync rules")

	var allSecrets []SecretPath

	for _, mount := range pm.syncRule.KvMounts {
		logger.Debug().Str("mount", mount).Msg("Processing mount")

		secrets, err := pm.discoverSecretsInMount(ctx, mount)
		if err != nil {
			logger.Error().Str("mount", mount).Err(err).Msg("Failed to discover secrets in mount")
			continue
		}

		allSecrets = append(allSecrets, secrets...)
	}

	logger.Info().Int("discovered_count", len(allSecrets)).Msg("Secret discovery completed")
	return allSecrets, nil
}

func (pm *VaultPathMatcher) DiscoverFromMounts(ctx context.Context, mounts []string) ([]SecretPath, error) {
	logger := pm.logger.With().Str("action", "discover_from_mounts").Logger()
	logger.Debug().Strs("mounts", mounts).Msg("Discovering all secrets from specific mounts")

	var allSecrets []SecretPath

	filterFunc := func(path string, isFinalPath bool) bool { return true }

	for _, mount := range mounts {
		keyPaths, err := pm.vaultClient.GetKeysUnderMount(ctx, mount, filterFunc)
		if err != nil {
			logger.Error().Str("mount", mount).Err(err).Msg("Failed to get keys under mount")
			continue
		}

		for _, keyPath := range keyPaths {
			allSecrets = append(allSecrets, SecretPath{Mount: mount, KeyPath: keyPath})
		}
	}

	logger.Info().Int("discovered_count", len(allSecrets)).Msg("Mount-based discovery completed")

	return allSecrets, nil
}

func (pm *VaultPathMatcher) ShouldSync(mount, keyPath string) bool {
	return pm.cpm.ShouldSync(mount, keyPath)
}

func (pm *VaultPathMatcher) isMountAllowed(mount string) bool {
	return slices.Contains(pm.syncRule.KvMounts, mount)
}

func (pm *VaultPathMatcher) discoverSecretsInMount(
	ctx context.Context, mount string) ([]SecretPath, error) {
	logger := pm.logger.With().
		Str("action", "discover_secrets_in_mount").
		Str("mount", mount).
		Logger()

	filterFunc := func(keyPath string, isFinalPath bool) bool {
		if isFinalPath {
			shouldInclude := pm.ShouldSync(mount, keyPath)
			logger.Debug().
				Str("path", keyPath).
				Bool("should_include", shouldInclude).
				Msg("Final path evaluation")
			return shouldInclude
		} else {
			shouldTraverse := pm.shouldTraverseIntoPath(keyPath)
			logger.Debug().
				Str("path", keyPath).
				Bool("should_traverse", shouldTraverse).
				Msg("Traversal path evaluation")
			return shouldTraverse
		}
	}

	keyPaths, err := pm.vaultClient.GetKeysUnderMount(ctx, mount, filterFunc)

	if err != nil {
		return nil, fmt.Errorf("failed to get keys under mount %s: %w", mount, err)
	}

	var secrets []SecretPath

	for _, keyPath := range keyPaths {
		secrets = append(secrets, SecretPath{
			Mount:   mount,
			KeyPath: keyPath,
		})
	}

	logger.Debug().
		Int("total_filtered_secrets", len(secrets)).
		Msg("Processed mount with sync rule filtering")

	return secrets, nil
}

func (pm *VaultPathMatcher) shouldTraverseIntoPath(keyPath string) bool {
	// First check if this path branch would be completely ignored
	if pm.isPathBranchCompletelyIgnored(keyPath) {
		return false
	}

	// If no replicate patterns specified, traverse everything not ignored
	if len(pm.syncRule.PathsToReplicate) == 0 {
		return true
	}

	// Then check if this path or its children could match replicate patterns
	for _, replicatePattern := range pm.syncRule.PathsToReplicate {
		if pm.cpm.matchesGlobPattern(replicatePattern, keyPath) {
			return true
		}

		if pm.couldPathLeadToPattern(replicatePattern, keyPath) {
			return true
		}
	}

	return false
}

func (pm *VaultPathMatcher) isPathBranchCompletelyIgnored(keyPath string) bool {
	if len(pm.syncRule.PathsToIgnore) == 0 {
		return false
	}

	for _, pattern := range pm.syncRule.PathsToIgnore {
		if pm.cpm.matchesGlobPattern(pattern, keyPath) {
			return true
		}

		// Check if current path could lead to the ignore pattern
		// This handles complex patterns like secret/*/test/app/*
		if pm.couldPathLeadToPattern(pattern, keyPath) {
			// Additional check: are we at the exact prefix that would make ALL children ignored?
			if pm.isAtIgnorePrefix(pattern, keyPath) {
				return true
			}
		}
	}

	return false
}

func (pm *VaultPathMatcher) isAtIgnorePrefix(ignorePattern, keytPath string) bool {
	for _, suffix := range []string{"/*", "/**"} {
		if strings.HasSuffix(ignorePattern, suffix) {
			ignorePatternPrefix := strings.TrimSuffix(ignorePattern, suffix)
			matched := pm.cpm.matchesGlobPattern(ignorePatternPrefix, keytPath)
			if matched {
				return true
			}
		}
	}
	return false
}

func (pm *VaultPathMatcher) couldPathLeadToPattern(pattern, keyPath string) bool {
	if strings.Contains(pattern, "**") {
		patternParts := strings.Split(pattern, "/")
		keyPathParts := strings.Split(keyPath, "/")

		// Find where ** appears
		doubleStarIdx := -1
		for i, part := range patternParts {
			if part == "**" {
				doubleStarIdx = i
				break
			}
		}

		if doubleStarIdx == 0 {
			return true
		}

		// Match all parts before the **
		for i := 0; i < doubleStarIdx && i < len(keyPathParts); i++ {
			matched, err := doublestar.Match(patternParts[i], keyPathParts[i])
			if err != nil || !matched {
				return false
			}
		}
	}

	keyPathParts := strings.Split(keyPath, "/")
	patternParts := strings.Split(pattern, "/")

	if len(keyPathParts) > len(patternParts) {
		return false
	}

	for i, currentPart := range keyPathParts {
		matched := pm.cpm.matchesGlobPattern(patternParts[i], currentPart)
		if !matched {
			return false
		}
	}

	return true
}
