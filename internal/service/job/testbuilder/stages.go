package testbuilder

import (
	"vault-sync/internal/models"
	job "vault-sync/internal/service/job"
)

// MockBuilder is the entry point for the builder.
type MockBuilder interface {
	WithMount(mount string) MockBuilder
	WithKeyPath(keyPath string) MockBuilder
	WithClusters(clusters ...string) MockDatabaseStage
}

// MockDatabaseStage defines the state of the secret in the database.
type MockDatabaseStage interface {
	WithGetSyncedSecretResult(version int64, clusters ...string) MockVaultStage
	WithNotFoundGetSyncedSecret(clusters ...string) MockSyncActionStage
	WithGetSyncedSecretError(err error) MockBuildable
}

// MockVaultStage defines the state of the secret in Vault.
type MockVaultStage interface {
	WithVaultSecretExists(exists bool) MockVaultExistenceStage
}

// MockVaultExistenceStage handles logic based on whether the secret exists in Vault.
type MockVaultExistenceStage interface {
	WhenSecretExists(version int64) MockSyncActionStage
	WhenSecretDoesNotExist() MockDeleteActionStage
	WithSecretExistsError(err error) MockBuildable
}

// MockSyncActionStage defines the result of a sync operation.
type MockSyncActionStage interface {
	WithSyncSecretToReplicasResult(status models.SyncStatus, version int64, clusters ...string) MockUpdateStatusStage
	WithSyncSecretToReplicasError(err error) MockBuildable
}

// MockDeleteActionStage defines the result of a delete operation.
type MockDeleteActionStage interface {
	WithDeleteSecretFromReplicasResult(status models.SyncStatus, clusters ...string) MockDeleteFromDBStage
	WithDeleteSecretFromReplicasError(err error) MockBuildable
}

// MockUpdateStatusStage defines the result of updating the secret status in the database.
type MockUpdateStatusStage interface {
	WithUpdateSyncedSecretStatusResult(err error, clusters ...string) MockUpdateStatusStage
	Build() (*job.MockRepository, *job.MockVaultClient)
}

// MockDeleteFromDBStage defines the result of deleting the secret from the database.
type MockDeleteFromDBStage interface {
	WithDeleteSyncedSecretResult(err error, clusters ...string) MockDeleteFromDBStage
	Build() (*job.MockRepository, *job.MockVaultClient)
}

// MockBuildable is the final stage that can build the mocks.
type MockBuildable interface {
	Build() (*job.MockRepository, *job.MockVaultClient)
}
