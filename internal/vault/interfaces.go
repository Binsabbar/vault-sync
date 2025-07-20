package vault

import (
	"context"
	"time"
	"vault-sync/internal/models"
)

// ************
//
// # VaultSyncer is an interface that defines the methods required for synchronizing secrets
//
// ************
type VaultSyncer interface {
	GetSecretMounts(ctx context.Context, secretPaths []string) ([]string, error)
	GetSecretMetadata(ctx context.Context, mount, keyPath string) (*VaultSecretMetadataResponse, error)
	GetKeysUnderMount(ctx context.Context, mount string, shouldIncludeKeyPath func(path string) bool) ([]string, error)
	SecretExists(ctx context.Context, mount, keyPath string) (bool, error)
	SyncSecretToReplicas(ctx context.Context, mount, keyPath string) ([]*models.SyncedSecret, error)
	DeleteSecretFromReplicas(ctx context.Context, mount, keyPath string) ([]*models.SyncSecretDeletionResult, error)
	GetReplicaNames() []string
}

// ************
//
// replicaSyncOperationResult is an interface that defines the methods required for a result
// of a replica synchronization operation.
//
// ************
type replicaSyncOperationResult interface {
	*models.SyncedSecret | *models.SyncSecretDeletionResult
	SetStatus(status models.SyncStatus)
	SetErrorMessage(msg *string)
	SetLastSuccessAttempt(t *time.Time)
	GetStatus() models.SyncStatus
	GetDestinationCluster() string
}