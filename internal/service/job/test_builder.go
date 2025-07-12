package job

import (
	"context"
	"vault-sync/internal/models"
	"vault-sync/internal/repository"
	"vault-sync/internal/vault"

	"github.com/stretchr/testify/mock"
)

// SyncJobTestBuilder provides a fluent interface for setting up SyncJob tests
type SyncJobTestBuilder struct {
	ctx       context.Context
	mount     string
	keyPath   string
	clusters  []string
	mockVault *mockVaultClient
	mockRepo  *mockRepository

	// Configuration for test scenarios
	secretExists       *bool
	secretVersion      int64
	sourceVersion      int64
	dbSecrets          map[string]*models.SyncedSecret
	dbErrors           map[string]error
	updateErrors       map[string]error
	deleteErrors       map[string]error
	vaultSyncResults   []*models.SyncedSecret
	vaultDeleteResults []*models.SyncSecretDeletionResult
	vaultErrors        map[string]error
}

// NewSyncJobTestBuilder creates a new test builder with sensible defaults
func NewSyncJobTestBuilder() *SyncJobTestBuilder {
	return &SyncJobTestBuilder{
		ctx:           context.Background(),
		mount:         "test-mount",
		keyPath:       "test/key/path",
		clusters:      make([]string, 2),
		mockVault:     new(mockVaultClient),
		mockRepo:      new(mockRepository),
		secretVersion: 1,
		sourceVersion: 1,
		dbSecrets:     make(map[string]*models.SyncedSecret),
		dbErrors:      make(map[string]error),
		updateErrors:  make(map[string]error),
		deleteErrors:  make(map[string]error),
		vaultErrors:   make(map[string]error),
	}
}

// WithContext sets the context for the test
func (b *SyncJobTestBuilder) WithContext(ctx context.Context) *SyncJobTestBuilder {
	b.ctx = ctx
	return b
}

// WithSecret sets the mount and key path
func (b *SyncJobTestBuilder) WithSecret(mount, keyPath string) *SyncJobTestBuilder {
	b.mount = mount
	b.keyPath = keyPath
	return b
}

// WithClusters sets the replica clusters
func (b *SyncJobTestBuilder) WithClusters(clusters ...string) *SyncJobTestBuilder {
	b.clusters = clusters
	return b
}

// WithSecretExists sets whether the secret exists in the source cluster
func (b *SyncJobTestBuilder) WithSecretExists(exists bool) *SyncJobTestBuilder {
	b.secretExists = &exists
	return b
}

// WithSecretVersion sets the current version of the secret in the source cluster
func (b *SyncJobTestBuilder) WithSecretVersion(version int64) *SyncJobTestBuilder {
	b.secretVersion = version
	return b
}

// WithSourceVersion sets the source version (for backward compatibility)
func (b *SyncJobTestBuilder) WithSourceVersion(version int64) *SyncJobTestBuilder {
	b.sourceVersion = version
	return b
}

// WithDBSecret sets up a database secret for a specific cluster
func (b *SyncJobTestBuilder) WithDBSecret(cluster string, secret *models.SyncedSecret) *SyncJobTestBuilder {
	if secret != nil {
		// Ensure the secret has the correct mount and path
		secret.SecretBackend = b.mount
		secret.SecretPath = b.keyPath
		secret.DestinationCluster = cluster
	}
	b.dbSecrets[cluster] = secret
	return b
}

// WithDBSecretNotFound marks a secret as not found in the database for specific clusters
func (b *SyncJobTestBuilder) WithDBSecretNotFound(clusters ...string) *SyncJobTestBuilder {
	for _, cluster := range clusters {
		b.dbErrors[cluster] = repository.ErrSecretNotFound
	}
	return b
}

// WithDBError sets up a database error for specific clusters
func (b *SyncJobTestBuilder) WithDBError(cluster string, err error) *SyncJobTestBuilder {
	b.dbErrors[cluster] = err
	return b
}

// WithUpdateError sets up an update error for specific clusters
func (b *SyncJobTestBuilder) WithUpdateError(cluster string, err error) *SyncJobTestBuilder {
	b.updateErrors[cluster] = err
	return b
}

// WithDeleteError sets up a delete error for specific clusters
func (b *SyncJobTestBuilder) WithDeleteError(cluster string, err error) *SyncJobTestBuilder {
	b.deleteErrors[cluster] = err
	return b
}

// WithVaultSyncResults sets the expected results from vault sync operations
func (b *SyncJobTestBuilder) WithVaultSyncResults(results []*models.SyncedSecret) *SyncJobTestBuilder {
	b.vaultSyncResults = results
	return b
}

// WithVaultDeleteResults sets the expected results from vault delete operations
func (b *SyncJobTestBuilder) WithVaultDeleteResults(results []*models.SyncSecretDeletionResult) *SyncJobTestBuilder {
	b.vaultDeleteResults = results
	return b
}

// WithVaultError sets up vault errors for specific operations
func (b *SyncJobTestBuilder) WithVaultError(operation string, err error) *SyncJobTestBuilder {
	b.vaultErrors[operation] = err
	return b
}

// Build sets up all the mocks and returns a configured SyncJob
func (b *SyncJobTestBuilder) Build() *SyncJob {
	// Setup basic vault mock expectations
	b.mockVault.On("GetReplicaNames").Return(b.clusters)

	// Setup database mocks for GetSyncedSecret
	for _, cluster := range b.clusters {
		if dbError, hasError := b.dbErrors[cluster]; hasError {
			b.mockRepo.On("GetSyncedSecret", b.mount, b.keyPath, cluster).Return(nil, dbError)
		} else if secret, hasSecret := b.dbSecrets[cluster]; hasSecret {
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
			metadata := &vault.VaultSecretMetadataResponse{CurrentVersion: b.secretVersion}
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

	return NewSyncJob(b.mount, b.keyPath, b.mockVault, b.mockRepo)
}

// GetMocks returns the mock objects for assertions
func (b *SyncJobTestBuilder) GetMocks() (*mockVaultClient, *mockRepository) {
	return b.mockVault, b.mockRepo
}

// GetContext returns the context
func (b *SyncJobTestBuilder) GetContext() context.Context {
	return b.ctx
}

// Helper methods for creating common test data

// NewSyncedSecret creates a new synced secret with the current mount/path
func (b *SyncJobTestBuilder) NewSyncedSecret(cluster string, version int64, status models.SyncStatus) *models.SyncedSecret {
	return &models.SyncedSecret{
		SecretBackend:      b.mount,
		SecretPath:         b.keyPath,
		DestinationCluster: cluster,
		SourceVersion:      version,
		Status:             status,
	}
}

// NewSyncSecretDeletionResult creates a new deletion result with the current mount/path
func (b *SyncJobTestBuilder) NewSyncSecretDeletionResult(cluster string, status models.SyncStatus) *models.SyncSecretDeletionResult {
	return &models.SyncSecretDeletionResult{
		SecretBackend:      b.mount,
		SecretPath:         b.keyPath,
		DestinationCluster: cluster,
		Status:             status,
	}
}

// Preset scenario builders for common test cases

// ForSecretNotInDB configures the builder for a scenario where the secret doesn't exist in the database
func (b *SyncJobTestBuilder) ForSecretNotInDB() *SyncJobTestBuilder {
	return b.WithDBSecretNotFound(b.clusters...).
		WithVaultSyncResults([]*models.SyncedSecret{
			b.NewSyncedSecret("cluster1", b.sourceVersion, models.StatusSuccess),
			b.NewSyncedSecret("cluster2", b.sourceVersion, models.StatusSuccess),
		})
}

// ForSecretUpToDate configures the builder for a scenario where the secret is up to date
func (b *SyncJobTestBuilder) ForSecretUpToDate() *SyncJobTestBuilder {
	secrets := make([]*models.SyncedSecret, 0, len(b.clusters))
	for _, cluster := range b.clusters {
		secrets = append(secrets, b.NewSyncedSecret(cluster, b.sourceVersion, models.StatusSuccess))
		b.WithDBSecret(cluster, b.NewSyncedSecret(cluster, b.sourceVersion, models.StatusSuccess))
	}
	return b.WithSecretExists(true).WithSecretVersion(b.sourceVersion)
}

// ForSecretOutOfDate configures the builder for a scenario where the secret is out of date
func (b *SyncJobTestBuilder) ForSecretOutOfDate() *SyncJobTestBuilder {
	// Database has old version, source has new version
	oldVersion := b.sourceVersion
	newVersion := b.sourceVersion + 1

	for _, cluster := range b.clusters {
		b.WithDBSecret(cluster, b.NewSyncedSecret(cluster, oldVersion, models.StatusSuccess))
	}

	return b.WithSecretExists(true).
		WithSecretVersion(newVersion).
		WithVaultSyncResults([]*models.SyncedSecret{
			b.NewSyncedSecret("cluster1", newVersion, models.StatusSuccess),
			b.NewSyncedSecret("cluster2", newVersion, models.StatusSuccess),
		})
}

// ForSecretDeleted configures the builder for a scenario where the secret should be deleted
func (b *SyncJobTestBuilder) ForSecretDeleted() *SyncJobTestBuilder {
	for _, cluster := range b.clusters {
		b.WithDBSecret(cluster, b.NewSyncedSecret(cluster, b.sourceVersion, models.StatusSuccess))
	}

	return b.WithSecretExists(false).
		WithVaultDeleteResults([]*models.SyncSecretDeletionResult{
			b.NewSyncSecretDeletionResult("cluster1", models.StatusDeleted),
			b.NewSyncSecretDeletionResult("cluster2", models.StatusDeleted),
		})
}
