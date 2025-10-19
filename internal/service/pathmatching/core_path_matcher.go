package pathmatching

import (
	"slices"

	"vault-sync/internal/config"

	"github.com/bmatcuk/doublestar/v4"
)

// CorePathMatcher contains the core pattern matching logic without dependencies.
type CorePathMatcher struct {
	syncRule *config.SyncRule
}

// NewCorePathMatcher creates a new core path matcher.
func NewCorePathMatcher(syncRule *config.SyncRule) *CorePathMatcher {
	return &CorePathMatcher{
		syncRule: syncRule,
	}
}

// ShouldSync checks if a secret path should be synced based on sync rules.
func (cpm *CorePathMatcher) ShouldSync(mount, keyPath string) bool {
	if !cpm.isMountAllowed(mount) {
		return false
	}

	if len(cpm.syncRule.PathsToIgnore) > 0 {
		for _, ignorePattern := range cpm.syncRule.PathsToIgnore {
			if cpm.matchesGlobPattern(ignorePattern, keyPath) {
				return false
			}
		}
	}

	if len(cpm.syncRule.PathsToReplicate) > 0 {
		for _, replicatePattern := range cpm.syncRule.PathsToReplicate {
			if cpm.matchesGlobPattern(replicatePattern, keyPath) {
				return true
			}
		}
		return false
	}

	return true
}

func (cpm *CorePathMatcher) isMountAllowed(mount string) bool {
	return slices.Contains(cpm.syncRule.KvMounts, mount)
}

func (cpm *CorePathMatcher) matchesGlobPattern(pattern, vaultPath string) bool {
	matched, err := doublestar.Match(pattern, vaultPath)
	if err != nil {
		return false
	}
	return matched
}
