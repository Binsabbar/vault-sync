package job

import (
	"context"
	"vault-sync/internal/models"
	"vault-sync/internal/repository"
	"vault-sync/internal/vault"

	"github.com/stretchr/testify/mock"
)

type (
	MockInitialStage interface {
		WithMount(mount string) MockInitialStage
		WithKeyPath(keyPath string) MockInitialStage
		WithClusters(clusters ...string) MockClustersStage
	}

	MockClustersStage interface {
		WithDatabaseSecretVersion(version int64) MockDatabaseSecretVersionStage
		WithNotFoundGetSyncedSecret(clusters ...string) MockDatabaseStage
		WithGetSyncedSecretError(err error, clusters ...string) MockDatabaseStage
	}

	MockDatabaseSecretVersionStage interface {
		WithGetSyncedSecretResult(clusters ...string) MockDatabaseStage
	}

	MockDatabaseStage interface {
		WithDatabaseSecretVersion(version int64) MockDatabaseStage
		WithSourceSecretVersion(version int64) MockDatabaseStage
		WithVaultSecretExists(exists bool) MockVaultStage
	}

	MockVaultStage interface {
		WithSyncSecretToReplicasResult(status models.SyncStatus, version int64, clusters ...string) MockResultStage
		WithDeleteSecretFromReplicasResult(status models.SyncStatus, clusters ...string) MockResultStage
		WithVaultError(operation string, err error) MockBuildableStage
	}

	MockResultStage interface {
		WithUpdateSyncedSecretStatusResult(clusters ...string) MockBuildableStage
		WithUpdateError(cluster string, err error) MockBuildableStage
	}

	MockBuildableStage interface {
		Build() (*mockRepository, *mockVaultClient)
		GetContext() context.Context
	}
)

// SyncJobMockBuilder provides a fluent interface for setting up SyncJob tests
type SyncJobMockBuilder struct {
	ctx      context.Context
	mount    string
	keyPath  string
	clusters []string

	mockRepo              *mockRepository
	secretVersion         int64
	dbGetSecretsResult    map[string]*models.SyncedSecret
	dbUpdateSecretsResult map[string]*models.SyncedSecret
	dbDeleteSecretsResult map[string]*models.SyncSecretDeletionResult

	// cluster:operation:error
	// e.g. "cluster1:update:error"
	// e.g. "cluster1:delete:error"
	dbErrors map[string]map[string]error

	mockVault           *mockVaultClient
	sourceSecretExists  *bool
	sourceSecretVersion int64
	vaultSyncResults    []*models.SyncedSecret
	vaultDeleteResults  []*models.SyncSecretDeletionResult

	// operation:error
	// e.g. "SyncSecretToReplicas:error"
	vaultErrors map[string]error
}

func NewSyncJobMockBuilder() *SyncJobMockBuilder {
	return &SyncJobMockBuilder{
		ctx:      context.Background(),
		mount:    "test-mount",
		keyPath:  "test/key/path",
		clusters: make([]string, 2),

		mockRepo:              new(mockRepository),
		secretVersion:         1,
		dbGetSecretsResult:    make(map[string]*models.SyncedSecret),
		dbUpdateSecretsResult: make(map[string]*models.SyncedSecret),
		dbDeleteSecretsResult: make(map[string]*models.SyncSecretDeletionResult),
		dbErrors:              make(map[string]map[string]error),

		mockVault:           new(mockVaultClient),
		sourceSecretVersion: 1,
		sourceSecretExists:  nil,

		vaultSyncResults:   make([]*models.SyncedSecret, 0),
		vaultDeleteResults: make([]*models.SyncSecretDeletionResult, 0),
		vaultErrors:        make(map[string]error),
	}
}

func (b *SyncJobMockBuilder) WithMount(mount string) *SyncJobMockBuilder {
	b.mount = mount
	return b
}

func (b *SyncJobMockBuilder) WithKeyPath(keyPath string) *SyncJobMockBuilder {
	b.keyPath = keyPath
	return b
}

func (b *SyncJobMockBuilder) WithSecretVersion(version int64) *SyncJobMockBuilder {
	b.secretVersion = version
	return b
}

func (b *SyncJobMockBuilder) WithClusters(clusters ...string) *SyncJobMockBuilder {
	b.clusters = clusters
	return b
}

// WithGetSyncedSecretResult uses the result of WithSecretVersion
func (b *SyncJobMockBuilder) WithGetSyncedSecretResult(clusters ...string) *SyncJobMockBuilder {
	for _, cluster := range clusters {
		secret := &models.SyncedSecret{
			SecretBackend:      b.mount,
			SecretPath:         b.keyPath,
			DestinationCluster: cluster,
			SourceVersion:      b.secretVersion,
		}

		b.dbGetSecretsResult[cluster] = secret
	}
	return b
}

func (b *SyncJobMockBuilder) WithNotFoundGetSyncedSecret(clusters ...string) *SyncJobMockBuilder {
	for _, cluster := range clusters {
		b.dbErrors[cluster] = repository.ErrSecretNotFound
	}
	return b
}

func (b *SyncJobMockBuilder) WithUpdateSyncedSecretStatusResult(status models.SyncStatus, version int64, clusters ...string) *SyncJobMockBuilder {
	for _, cluster := range clusters {
		secret := &models.SyncedSecret{
			SecretBackend:      b.mount,
			SecretPath:         b.keyPath,
			DestinationCluster: cluster,
			Status:             status,
			SourceVersion:      version,
		}

		b.dbUpdateSecretsResult[cluster] = secret
	}
	return b
}

func (b *SyncJobMockBuilder) WithSyncSecretToReplicasResult(status models.SyncStatus, version int64, clusters ...string) *SyncJobMockBuilder {
	for _, cluster := range clusters {
		secret := &models.SyncedSecret{
			SecretBackend:      b.mount,
			SecretPath:         b.keyPath,
			DestinationCluster: cluster,
			Status:             status,
			SourceVersion:      version,
		}

		b.vaultSyncResults = append(b.vaultSyncResults, secret)
	}
	return b
}

func (b *SyncJobMockBuilder) WithSecretExists(exists bool) *SyncJobMockBuilder {
	b.secretExists = &exists
	return b
}

func (b *SyncJobMockBuilder) WithSourceSecretVersion(version int64) *SyncJobMockBuilder {
	b.sourceVersion = version
	return b
}

// // WithSecret sets the mount and key path
// func (b *SyncJobMockBuilder) WithSecret(mount, keyPath string) *SyncJobMockBuilder {
// 	b.mount = mount
// 	b.keyPath = keyPath
// 	return b
// }

// // WithSecretExists sets whether the secret exists in the source cluster
// func (b *SyncJobMockBuilder) WithSecretExists(exists bool) *SyncJobMockBuilder {
// 	b.secretExists = &exists
// 	return b
// }

// // WithSecretVersion sets the current version of the secret in the source cluster
// func (b *SyncJobMockBuilder) WithSecretVersion(version int64) *SyncJobMockBuilder {
// 	b.secretVersion = version
// 	return b
// }

// // WithSourceVersion sets the source version (for backward compatibility)
// func (b *SyncJobMockBuilder) WithSourceVersion(version int64) *SyncJobMockBuilder {
// 	b.sourceVersion = version
// 	return b
// }

// // WithDBSecret sets up a database secret for a specific cluster
// func (b *SyncJobMockBuilder) WithDBSecret(cluster string, secret *models.SyncedSecret) *SyncJobMockBuilder {
// 	if secret != nil {
// 		// Ensure the secret has the correct mount and path
// 		secret.SecretBackend = b.mount
// 		secret.SecretPath = b.keyPath
// 		secret.DestinationCluster = cluster
// 	}
// 	b.dbSecrets[cluster] = secret
// 	return b
// }

// // WithDBSecretNotFound marks a secret as not found in the database for specific clusters
// func (b *SyncJobMockBuilder) WithDBSecretNotFound(clusters ...string) *SyncJobMockBuilder {
// 	for _, cluster := range clusters {
// 		b.dbErrors[cluster] = repository.ErrSecretNotFound
// 	}
// 	return b
// }

// // WithDBError sets up a database error for specific clusters
// func (b *SyncJobMockBuilder) WithDBError(cluster string, err error) *SyncJobMockBuilder {
// 	b.dbErrors[cluster] = err
// 	return b
// }

// // WithUpdateError sets up an update error for specific clusters
// func (b *SyncJobMockBuilder) WithUpdateError(cluster string, err error) *SyncJobMockBuilder {
// 	b.updateErrors[cluster] = err
// 	return b
// }

// // WithDeleteError sets up a delete error for specific clusters
// func (b *SyncJobMockBuilder) WithDeleteError(cluster string, err error) *SyncJobMockBuilder {
// 	b.deleteErrors[cluster] = err
// 	return b
// }

// // WithVaultSyncResults sets the expected results from vault sync operations
// func (b *SyncJobMockBuilder) WithVaultSyncResults(results []*models.SyncedSecret) *SyncJobMockBuilder {
// 	b.vaultSyncResults = results
// 	return b
// }

// // WithVaultDeleteResults sets the expected results from vault delete operations
// func (b *SyncJobMockBuilder) WithVaultDeleteResults(results []*models.SyncSecretDeletionResult) *SyncJobMockBuilder {
// 	b.vaultDeleteResults = results
// 	return b
// }

// // WithVaultError sets up vault errors for specific operations
// func (b *SyncJobMockBuilder) WithVaultError(operation string, err error) *SyncJobMockBuilder {
// 	b.vaultErrors[operation] = err
// 	return b
// }

// // Build sets up all the mocks and returns a configured SyncJob
func (b *SyncJobMockBuilder) Build() (*mockRepository, *mockVaultClient) {
	// Setup basic vault mock expectations
	b.mockVault.On("GetReplicaNames").Return(b.clusters)

	// Setup database mocks for GetSyncedSecret
	for _, cluster := range b.clusters {
		if dbError, hasError := b.dbErrors[cluster]; hasError {
			b.mockRepo.On("GetSyncedSecret", b.mount, b.keyPath, cluster).Return(nil, dbError)
		} else if secret, hasSecret := b.dbGetSecretsResult[cluster]; hasSecret {
			b.mockRepo.On("GetSyncedSecret", b.mount, b.keyPath, cluster).Return(secret, nil)
		} else {
			// Default: secret not found
			b.mockRepo.On("GetSyncedSecret", b.mount, b.keyPath, cluster).Return(nil, repository.ErrSecretNotFound)
		}
	}

	// Setup vault SecretExists mock if specified
	if b.secretExists != nil {
		if vaultError, hasError := b.vaultErrors["SecretExists"]; hasError {
			b.mockVault.On("SecretExists", mock.Anything, b.mount, b.keyPath).Return(false, vaultError)
		} else {
			b.mockVault.On("SecretExists", mock.Anything, b.mount, b.keyPath).Return(*b.secretExists, nil)
		}
	}

	// Setup vault GetSecretMetadata mock if secret exists
	if b.secretExists != nil && *b.secretExists {
		if vaultError, hasError := b.vaultErrors["GetSecretMetadata"]; hasError {
			b.mockVault.On("GetSecretMetadata", mock.Anything, b.mount, b.keyPath).Return(nil, vaultError)
		} else {
			metadata := &vault.VaultSecretMetadataResponse{CurrentVersion: b.sourceVersion}
			b.mockVault.On("GetSecretMetadata", mock.Anything, b.mount, b.keyPath).Return(metadata, nil)
		}
	}

	// Setup vault sync operations
	if b.vaultSyncResults != nil {
		if vaultError, hasError := b.vaultErrors["SyncSecretToReplicas"]; hasError {
			b.mockVault.On("SyncSecretToReplicas", mock.Anything, b.mount, b.keyPath).Return(nil, vaultError)
		} else {
			b.mockVault.On("SyncSecretToReplicas", mock.Anything, b.mount, b.keyPath).Return(b.vaultSyncResults, nil)
		}

		// Setup update mocks for successful sync results
		for _, result := range b.vaultSyncResults {
			if updateError, hasError := b.updateErrors[result.DestinationCluster]; hasError {
				b.mockRepo.On("UpdateSyncedSecretStatus", result).Return(updateError)
			} else {
				b.mockRepo.On("UpdateSyncedSecretStatus", result).Return(nil)
			}
		}
	}

	// Setup vault delete operations
	if b.vaultDeleteResults != nil {
		if vaultError, hasError := b.vaultErrors["DeleteSecretFromReplicas"]; hasError {
			b.mockVault.On("DeleteSecretFromReplicas", mock.Anything, b.mount, b.keyPath).Return(nil, vaultError)
		} else {
			b.mockVault.On("DeleteSecretFromReplicas", mock.Anything, b.mount, b.keyPath).Return(b.vaultDeleteResults, nil)
		}

		// Setup delete mocks for successful delete results
		for _, result := range b.vaultDeleteResults {
			if deleteError, hasError := b.deleteErrors[result.DestinationCluster]; hasError {
				b.mockRepo.On("DeleteSyncedSecret", b.mount, b.keyPath, result.DestinationCluster).Return(deleteError)
			} else {
				b.mockRepo.On("DeleteSyncedSecret", b.mount, b.keyPath, result.DestinationCluster).Return(nil)
			}
		}
	}

	return b.mockRepo, b.mockVault
}

// // GetMocks returns the mock objects for assertions
// func (b *SyncJobMockBuilder) GetMocks() (*mockVaultClient, *mockRepository) {
// 	return b.mockVault, b.mockRepo
// }

// // GetContext returns the context
// func (b *SyncJobMockBuilder) GetContext() context.Context {
// 	return b.ctx
// }

// // Helper methods for creating common test data

// // NewSyncedSecret creates a new synced secret with the current mount/path
// func (b *SyncJobMockBuilder) NewSyncedSecret(cluster string, version int64, status models.SyncStatus) *models.SyncedSecret {
// 	return &models.SyncedSecret{
// 		SecretBackend:      b.mount,
// 		SecretPath:         b.keyPath,
// 		DestinationCluster: cluster,
// 		SourceVersion:      version,
// 		Status:             status,
// 	}
// }

// // NewSyncSecretDeletionResult creates a new deletion result with the current mount/path
// func (b *SyncJobMockBuilder) NewSyncSecretDeletionResult(cluster string, status models.SyncStatus) *models.SyncSecretDeletionResult {
// 	return &models.SyncSecretDeletionResult{
// 		SecretBackend:      b.mount,
// 		SecretPath:         b.keyPath,
// 		DestinationCluster: cluster,
// 		Status:             status,
// 	}
// }

// // Preset scenario builders for common test cases

// // ForSecretNotInDB configures the builder for a scenario where the secret doesn't exist in the database
// func (b *SyncJobMockBuilder) ForSecretNotInDB() *SyncJobMockBuilder {
// 	return b.WithDBSecretNotFound(b.clusters...).
// 		WithVaultSyncResults([]*models.SyncedSecret{
// 			b.NewSyncedSecret("cluster1", b.sourceVersion, models.StatusSuccess),
// 			b.NewSyncedSecret("cluster2", b.sourceVersion, models.StatusSuccess),
// 		})
// }

// // ForSecretUpToDate configures the builder for a scenario where the secret is up to date
// func (b *SyncJobMockBuilder) ForSecretUpToDate() *SyncJobMockBuilder {
// 	secrets := make([]*models.SyncedSecret, 0, len(b.clusters))
// 	for _, cluster := range b.clusters {
// 		secrets = append(secrets, b.NewSyncedSecret(cluster, b.sourceVersion, models.StatusSuccess))
// 		b.WithDBSecret(cluster, b.NewSyncedSecret(cluster, b.sourceVersion, models.StatusSuccess))
// 	}
// 	return b.WithSecretExists(true).WithSecretVersion(b.sourceVersion)
// }

// // ForSecretOutOfDate configures the builder for a scenario where the secret is out of date
// func (b *SyncJobMockBuilder) ForSecretOutOfDate() *SyncJobMockBuilder {
// 	// Database has old version, source has new version
// 	oldVersion := b.sourceVersion
// 	newVersion := b.sourceVersion + 1

// 	for _, cluster := range b.clusters {
// 		b.WithDBSecret(cluster, b.NewSyncedSecret(cluster, oldVersion, models.StatusSuccess))
// 	}

// 	return b.WithSecretExists(true).
// 		WithSecretVersion(newVersion).
// 		WithVaultSyncResults([]*models.SyncedSecret{
// 			b.NewSyncedSecret("cluster1", newVersion, models.StatusSuccess),
// 			b.NewSyncedSecret("cluster2", newVersion, models.StatusSuccess),
// 		})
// }

// // ForSecretDeleted configures the builder for a scenario where the secret should be deleted
// func (b *SyncJobMockBuilder) ForSecretDeleted() *SyncJobMockBuilder {
// 	for _, cluster := range b.clusters {
// 		b.WithDBSecret(cluster, b.NewSyncedSecret(cluster, b.sourceVersion, models.StatusSuccess))
// 	}

// 	return b.WithSecretExists(false).
// 		WithVaultDeleteResults([]*models.SyncSecretDeletionResult{
// 			b.NewSyncSecretDeletionResult("cluster1", models.StatusDeleted),
// 			b.NewSyncSecretDeletionResult("cluster2", models.StatusDeleted),
// 		})
// }
