package vault

import (
	"context"
	"time"
	"vault-sync/internal/models"
)

// Syncer is an interface that defines the methods required for synchronizing secrets.
type Syncer interface {
	GetSecretMounts(ctx context.Context, secretPaths []string) ([]string, error)
	GetSecretMetadata(ctx context.Context, mount, keyPath string) (*SecretMetadataResponse, error)
	GetKeysUnderMount(
		ctx context.Context,
		mount string,
		shouldIncludeKeyPath func(path string, isFinalPath bool) bool,
	) ([]string, error)
	SecretExists(ctx context.Context, mount, keyPath string) (bool, error)
	SecretExistsInReplica(ctx context.Context, clusterName, mount, path string) (bool, error)
	SyncSecretToReplicas(ctx context.Context, mount, keyPath string) ([]*models.SyncedSecret, error)
	DeleteSecretFromReplicas(ctx context.Context, mount, keyPath string) ([]*models.SyncSecretDeletionResult, error)
	GetReplicaNames() []string
}

// replicaSyncOperationResult is an interface that defines the methods required for a result
// of a replica synchronization operation.
type replicaSyncOperationResult interface {
	*models.SyncedSecret | *models.SyncSecretDeletionResult
	SetStatus(status models.SyncStatus)
	SetErrorMessage(msg *string)
	SetLastSuccessAttempt(t *time.Time)
	GetStatus() models.SyncStatus
	GetDestinationCluster() string
}
