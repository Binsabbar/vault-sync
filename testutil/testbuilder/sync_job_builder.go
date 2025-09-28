package testbuilder

import (
	"context"
	"vault-sync/internal/models"
	"vault-sync/internal/repository"

	"github.com/stretchr/testify/mock"
)

// Fluent interface for building SyncJob test scenarios
type (
	MockBuilderInit interface {
		WithMount(mount string) MockBuilderInit
		WithKeyPath(keyPath string) MockBuilderInit
		WithClusters(clusters ...string) MockClustersStage
	}

	MockClustersStage interface {
		WithDatabaseSecretVersion(version int64) MockDatabaseSecretVersionStage
		WithGetSyncedSecretNotFound(clusters ...string) MockDatabaseStage
		WithGetSyncedSecretError(err error, clusters ...string) MockDatabaseStage
	}

	MockDatabaseSecretVersionStage interface {
		WithGetSyncedSecret(clusters ...string) MockDatabaseStage
	}

	MockDatabaseStage interface {
		WithDatabaseSecretVersion(version int64) MockDatabaseSecretVersionStage
		WithGetSyncedSecretError(err error, clusters ...string) MockDatabaseStage
		WithGetSyncedSecretNotFound(clusters ...string) MockDatabaseStage
		WithUpdateSyncedSecretStatus(status models.SyncStatus, version int64, clusters ...string) MockDatabaseStage
		WithUpdateSyncedSecretStatusError(err error, cluster ...string) MockDatabaseStage
		WithDeleteSyncedSecret(clusters ...string) MockDatabaseStage
		WithDeleteSyncedSecretError(err error, clusters ...string) MockDatabaseStage
		SwitchToVaultStage() MockVaultStage
		SwitchToBuildableStage() MockBuildableStage
	}

	MockVaultStage interface {
		WithVaultSecretExists(exists bool) MockVaultStage
		WithVaultSecretExistsError(err error) MockVaultStage
		WithGetSecretMetadata(version int64) MockVaultStage
		WithGetSecretMetadataError(err error) MockVaultStage
		WithSyncSecretToReplicas(status models.SyncStatus, version int64, clusters ...string) MockVaultStage
		WithSyncSecretToReplicasError(err error) MockVaultStage
		WithDeleteSecretFromReplicas(status models.SyncStatus, clusters ...string) MockVaultStage
		WithDeleteSecretFromReplicasError(err error) MockVaultStage
		SwitchToBuildableStage() MockBuildableStage
	}

	MockBuildableStage interface {
		Build() (*mockRepository, *mockVaultClient)
	}
)

// SyncJobMockBuilder provides a fluent way for setting up SyncJob tests
type syncJobMockBuilder struct {
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

	vaultMockBuilder *VaultMockBuilder
}

const (
	DBGetSyncedSecret          = "GetSyncedSecret"
	DBUpdateSyncedSecretStatus = "UpdateSyncedSecretStatus"
	DBDeleteSyncedSecret       = "DeleteSyncedSecret"
)

func NewSyncJobMockBuilder() MockBuilderInit {
	return &syncJobMockBuilder{
		ctx:      context.Background(),
		mount:    "test-mount",
		keyPath:  "test/key/path",
		clusters: make([]string, 0),

		mockRepo:              new(mockRepository),
		secretVersion:         1,
		dbGetSecretsResult:    make(map[string]*models.SyncedSecret),
		dbUpdateSecretsResult: make(map[string]*models.SyncedSecret),
		dbDeleteSecretsResult: make(map[string]*models.SyncSecretDeletionResult),
		dbErrors:              make(map[string]map[string]error),

		vaultMockBuilder: nil,
	}
}

// MockBuilderInit interface implementation
func (b *syncJobMockBuilder) WithMount(mount string) MockBuilderInit {
	b.mount = mount
	return b
}

func (b *syncJobMockBuilder) WithKeyPath(keyPath string) MockBuilderInit {
	b.keyPath = keyPath
	return b
}

func (b *syncJobMockBuilder) WithClusters(clusters ...string) MockClustersStage {
	b.clusters = append(b.clusters, clusters...)

	for _, cluster := range clusters {
		b.dbGetSecretsResult[cluster] = nil
		b.dbUpdateSecretsResult[cluster] = nil
		b.dbDeleteSecretsResult[cluster] = nil
		b.dbErrors[cluster] = make(map[string]error)
	}

	return b
}

// MockClustersStage interface implementation
func (b *syncJobMockBuilder) WithDatabaseSecretVersion(version int64) MockDatabaseSecretVersionStage {
	b.secretVersion = version
	return b
}

// MockDatabaseSecretVersionStage interface implementation
func (b *syncJobMockBuilder) WithGetSyncedSecret(clusters ...string) MockDatabaseStage {
	for _, cluster := range clusters {
		secret := &models.SyncedSecret{
			SecretBackend:      b.mount,
			SecretPath:         b.keyPath,
			SourceVersion:      b.secretVersion,
			DestinationCluster: cluster,
		}
		b.dbGetSecretsResult[cluster] = secret
	}
	return b
}

// MockDatabaseStage interface implementation
func (b *syncJobMockBuilder) WithGetSyncedSecretError(err error, clusters ...string) MockDatabaseStage {
	for _, cluster := range clusters {
		b.dbErrors[cluster][DBGetSyncedSecret] = err
	}
	return b
}

func (b *syncJobMockBuilder) WithGetSyncedSecretNotFound(clusters ...string) MockDatabaseStage {
	for _, cluster := range clusters {
		b.dbErrors[cluster][DBGetSyncedSecret] = repository.ErrSecretNotFound
	}
	return b
}

func (b *syncJobMockBuilder) WithUpdateSyncedSecretStatus(status models.SyncStatus, version int64, clusters ...string) MockDatabaseStage {
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

func (b *syncJobMockBuilder) WithUpdateSyncedSecretStatusError(err error, clusters ...string) MockDatabaseStage {
	for _, cluster := range clusters {
		b.dbErrors[cluster][DBUpdateSyncedSecretStatus] = err
	}
	return b
}

func (b *syncJobMockBuilder) WithDeleteSyncedSecret(clusters ...string) MockDatabaseStage {
	for _, cluster := range clusters {
		b.dbErrors[cluster][DBDeleteSyncedSecret] = nil
	}
	return b
}

func (b *syncJobMockBuilder) WithDeleteSyncedSecretError(err error, clusters ...string) MockDatabaseStage {
	for _, cluster := range clusters {
		b.dbErrors[cluster][DBDeleteSyncedSecret] = err
	}
	return b
}

func (b *syncJobMockBuilder) SwitchToVaultStage() MockVaultStage {
	b.vaultMockBuilder = NewVaultMockBuilder(b.mount, b.keyPath, b.clusters...)
	return b
}

// MockVaultStage interface implementation
func (b *syncJobMockBuilder) WithVaultSecretExists(exists bool) MockVaultStage {
	b.vaultMockBuilder.WithVaultSecretExists(exists)
	return b
}

func (b *syncJobMockBuilder) WithVaultSecretExistsError(err error) MockVaultStage {
	b.vaultMockBuilder.WithVaultSecretExistsError(err)
	return b
}

func (b *syncJobMockBuilder) WithGetSecretMetadata(version int64) MockVaultStage {
	b.vaultMockBuilder.WithGetSecretMetadata(version)
	return b
}

func (b *syncJobMockBuilder) WithGetSecretMetadataError(err error) MockVaultStage {
	b.vaultMockBuilder.WithGetSecretMetadataError(err)
	return b
}

func (b *syncJobMockBuilder) WithSyncSecretToReplicas(status models.SyncStatus, version int64, clusters ...string) MockVaultStage {
	b.vaultMockBuilder.WithSyncSecretToReplicas(status, version, clusters...)
	return b
}

func (b *syncJobMockBuilder) WithSyncSecretToReplicasError(err error) MockVaultStage {
	b.vaultMockBuilder.WithSyncSecretToReplicasError(err)
	return b
}

func (b *syncJobMockBuilder) WithDeleteSecretFromReplicas(status models.SyncStatus, clusters ...string) MockVaultStage {
	b.vaultMockBuilder.WithDeleteSecretFromReplicas(status, clusters...)
	return b
}

func (b *syncJobMockBuilder) WithDeleteSecretFromReplicasError(err error) MockVaultStage {
	b.vaultMockBuilder.WithDeleteSecretFromReplicasError(err)
	return b
}

func (b *syncJobMockBuilder) SwitchToBuildableStage() MockBuildableStage {
	if b.vaultMockBuilder == nil {
		b.vaultMockBuilder = NewVaultMockBuilder(b.mount, b.keyPath, b.clusters...)
	}
	return b
}

// MockBuildableStage interface implementation
func (b *syncJobMockBuilder) Build() (*mockRepository, *mockVaultClient) {
	b.mockRepo = new(mockRepository)

	// Setup database mocks for GetSyncedSecret
	for _, cluster := range b.clusters {
		if dbError, hasError := b.dbErrors[cluster][DBGetSyncedSecret]; hasError {
			b.mockRepo.On("GetSyncedSecret", b.mount, b.keyPath, cluster).Return(nil, dbError)
		} else if secret, hasSecret := b.dbGetSecretsResult[cluster]; hasSecret {
			b.mockRepo.On("GetSyncedSecret", b.mount, b.keyPath, cluster).Return(secret, nil)
		} else {
			b.mockRepo.On("GetSyncedSecret", b.mount, b.keyPath, cluster).Return(nil, repository.ErrSecretNotFound)
		}

		if updateErr, hasError := b.dbErrors[cluster][DBUpdateSyncedSecretStatus]; hasError {
			b.mockRepo.On("UpdateSyncedSecretStatus", mock.MatchedBy(func(methodArg *models.SyncedSecret) bool {
				return methodArg.DestinationCluster == cluster
			})).Return(updateErr)
		} else if secret, hasSecret := b.dbUpdateSecretsResult[cluster]; hasSecret {
			updateMatcher := mock.MatchedBy(func(methodArg *models.SyncedSecret) bool {
				return methodArg.SecretBackend == secret.SecretBackend &&
					methodArg.SecretPath == secret.SecretPath &&
					methodArg.DestinationCluster == cluster &&
					methodArg.Status == secret.Status &&
					methodArg.SourceVersion == secret.SourceVersion
			})
			b.mockRepo.On("UpdateSyncedSecretStatus", updateMatcher).Return(nil)
		}

		if deleteErr, hasError := b.dbErrors[cluster][DBDeleteSyncedSecret]; hasError {
			// error can be nil when WithDeleteSyncedSecret is used
			b.mockRepo.On("DeleteSyncedSecret", b.mount, b.keyPath, cluster).Return(deleteErr)
		}
	}

	return b.mockRepo, b.vaultMockBuilder.Build()
}
