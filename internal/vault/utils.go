package vault

import (
	"fmt"
	"strings"
	"vault-sync/internal/models"
	"vault-sync/pkg/converter"

	"github.com/rs/zerolog"
)

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

// isNotFoundError checks if the error is a "not found" error
func isNotFoundError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, ErrorNotFound404) ||
		strings.Contains(errStr, ErrorNoSuchPath)
}

// logOperationSummary logs a summary of the synchronization operation
// It counts the number of successful, failed, and pending operations
// and logs them using the provided logger.
func logOperationSummary[T replicaSyncOperationResult](logger *zerolog.Logger, results []T) {
	successCount := 0
	failureCount := 0
	pendingCount := 0

	for _, result := range results {
		switch result.GetStatus() {
		case models.StatusSuccess:
			successCount++
		case models.StatusFailed:
			failureCount++
		case models.StatusPending:
			pendingCount++
		}
	}

	logger.Info().
		Int("success_count", successCount).
		Int("failure_count", failureCount).
		Int("pending_count", pendingCount).
		Msg("Secret synchronization operation summary")
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}
