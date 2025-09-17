package testbuilder

import (
	"context"
	"errors"
	"vault-sync/internal/models"
	"vault-sync/internal/repository"
	"vault-sync/internal/vault"

	"github.com/stretchr/testify/mock"
)

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
		Build() (*mockRepository, *mockVaultClient)
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
		Build() (*mockRepository, *mockVaultClient)
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

const (
	DBGetSyncedSecret          = "GetSyncedSecret"
	DBUpdateSyncedSecretStatus = "UpdateSyncedSecretStatus"
	DBDeleteSyncedSecret       = "DeleteSyncedSecret"

	VaultSecretExists             = "SecretExists"
	VaultGetSecretMetadata        = "GetSecretMetadata"
	VaultSyncSecretToReplicas     = "SyncSecretToReplicas"
	VaultDeleteSecretFromReplicas = "DeleteSecretFromReplicas"
)

func NewSyncJobMockBuilder() MockBuilderInit {
	return &SyncJobMockBuilder{
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

		mockVault:           new(mockVaultClient),
		sourceSecretVersion: 1,
		sourceSecretExists:  nil,

		vaultSyncResults:   make([]*models.SyncedSecret, 0),
		vaultDeleteResults: make([]*models.SyncSecretDeletionResult, 0),
		vaultErrors:        make(map[string]error),
	}
}

// MockBuilderInit interface implementation
func (b *SyncJobMockBuilder) WithMount(mount string) MockBuilderInit {
	b.mount = mount
	return b
}

func (b *SyncJobMockBuilder) WithKeyPath(keyPath string) MockBuilderInit {
	b.keyPath = keyPath
	return b
}

func (b *SyncJobMockBuilder) WithClusters(clusters ...string) MockClustersStage {
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
func (b *SyncJobMockBuilder) WithDatabaseSecretVersion(version int64) MockDatabaseSecretVersionStage {
	b.secretVersion = version
	return b
}

// MockDatabaseSecretVersionStage interface implementation
func (b *SyncJobMockBuilder) WithGetSyncedSecret(clusters ...string) MockDatabaseStage {
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
func (b *SyncJobMockBuilder) WithGetSyncedSecretError(err error, clusters ...string) MockDatabaseStage {
	for _, cluster := range clusters {
		b.dbErrors[cluster][DBGetSyncedSecret] = err
	}
	return b
}

func (b *SyncJobMockBuilder) WithGetSyncedSecretNotFound(clusters ...string) MockDatabaseStage {
	for _, cluster := range clusters {
		b.dbErrors[cluster][DBGetSyncedSecret] = repository.ErrSecretNotFound
	}
	return b
}

func (b *SyncJobMockBuilder) WithUpdateSyncedSecretStatus(status models.SyncStatus, version int64, clusters ...string) MockDatabaseStage {
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

func (b *SyncJobMockBuilder) WithUpdateSyncedSecretStatusError(err error, clusters ...string) MockDatabaseStage {
	for _, cluster := range clusters {
		b.dbErrors[cluster][DBUpdateSyncedSecretStatus] = err
	}
	return b
}

func (b *SyncJobMockBuilder) WithDeleteSyncedSecret(clusters ...string) MockDatabaseStage {
	for _, cluster := range clusters {
		b.dbErrors[cluster][DBDeleteSyncedSecret] = nil
	}
	return b
}

func (b *SyncJobMockBuilder) WithDeleteSyncedSecretError(err error, clusters ...string) MockDatabaseStage {
	for _, cluster := range clusters {
		b.dbErrors[cluster][DBDeleteSyncedSecret] = err
	}
	return b
}

func (b *SyncJobMockBuilder) SwitchToVaultStage() MockVaultStage {
	return b
}

// MockVaultStage interface implementation
func (b *SyncJobMockBuilder) WithVaultSecretExists(exists bool) MockVaultStage {
	b.sourceSecretExists = &exists
	return b
}

func (b *SyncJobMockBuilder) WithVaultSecretExistsError(err error) MockVaultStage {
	b.vaultErrors[VaultSecretExists] = err
	return b
}

func (b *SyncJobMockBuilder) WithGetSecretMetadata(version int64) MockVaultStage {
	if b.sourceSecretExists != nil && *b.sourceSecretExists {
		b.sourceSecretVersion = version
	} else if b.sourceSecretExists == nil {
		panic("Invoke WithVaultSecretExists before WithGetSecretMetadata")
	}
	return b
}

func (b *SyncJobMockBuilder) WithGetSecretMetadataError(err error) MockVaultStage {
	b.vaultErrors[VaultGetSecretMetadata] = err
	return b
}

func (b *SyncJobMockBuilder) WithSyncSecretToReplicas(status models.SyncStatus, version int64, clusters ...string) MockVaultStage {

	for _, cluster := range clusters {
		result := &models.SyncedSecret{
			SecretBackend:      b.mount,
			SecretPath:         b.keyPath,
			DestinationCluster: cluster,
			Status:             status,
			SourceVersion:      version,
		}
		b.vaultSyncResults = append(b.vaultSyncResults, result)
	}

	return b
}

func (b *SyncJobMockBuilder) WithSyncSecretToReplicasError(err error) MockVaultStage {
	b.vaultErrors[VaultSyncSecretToReplicas] = err
	return b
}

func (b *SyncJobMockBuilder) WithDeleteSecretFromReplicas(status models.SyncStatus, clusters ...string) MockVaultStage {
	if b.sourceSecretExists != nil && !*b.sourceSecretExists {
		for _, cluster := range clusters {
			result := &models.SyncSecretDeletionResult{
				SecretBackend:      b.mount,
				SecretPath:         b.keyPath,
				DestinationCluster: cluster,
				Status:             status,
			}
			b.vaultDeleteResults = append(b.vaultDeleteResults, result)
		}
	} else if b.sourceSecretExists == nil {
		panic("Invoke WithVaultSecretExists before WithDeleteSecretFromReplicas")
	}
	return b
}

func (b *SyncJobMockBuilder) WithDeleteSecretFromReplicasError(err error) MockVaultStage {
	b.vaultErrors[VaultDeleteSecretFromReplicas] = err
	return b
}

// MockBuildableStage interface implementation
func (b *SyncJobMockBuilder) Build() (*mockRepository, *mockVaultClient) {
	b.mockRepo = new(mockRepository)
	b.mockVault = new(mockVaultClient)

	b.mockVault.On("GetReplicaNames").Return(b.clusters)

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

	// Setup vault SecretExists mock
	if vaultError, hasError := b.vaultErrors[VaultSecretExists]; hasError {
		b.mockVault.On("SecretExists", mock.Anything, b.mount, b.keyPath).Return(false, vaultError)
	} else if b.sourceSecretExists != nil {
		b.mockVault.On("SecretExists", mock.Anything, b.mount, b.keyPath).Return(*b.sourceSecretExists, nil)
	} else {
		b.mockVault.On("SecretExists", mock.Anything, b.mount, b.keyPath).Return(false, nil)
	}

	// Setup vault GetSecretMetadata mock
	if vaultError, hasError := b.vaultErrors[VaultGetSecretMetadata]; hasError {
		b.mockVault.On("GetSecretMetadata", mock.Anything, b.mount, b.keyPath).Return(nil, vaultError)
	} else {
		if b.sourceSecretExists != nil {
			if !*b.sourceSecretExists {
				b.mockVault.On("GetSecretMetadata", mock.Anything, b.mount, b.keyPath).Return(nil, errors.New("secret not found"))
			} else {
				metadata := &vault.VaultSecretMetadataResponse{CurrentVersion: b.sourceSecretVersion}
				b.mockVault.On("GetSecretMetadata", mock.Anything, b.mount, b.keyPath).Return(metadata, nil)
			}
		}
	}

	// setup vault SyncSecretToReplicas mock if secret exists
	if len(b.vaultSyncResults) > 0 || b.vaultErrors[VaultSyncSecretToReplicas] != nil {
		if vaultError, hasError := b.vaultErrors[VaultSyncSecretToReplicas]; hasError {
			b.mockVault.On("SyncSecretToReplicas", mock.Anything, b.mount, b.keyPath).Return(nil, vaultError)
		} else {
			b.mockVault.On("SyncSecretToReplicas", mock.Anything, b.mount, b.keyPath).Return(b.vaultSyncResults, nil)
		}

	}

	// Setup vault DeleteSecretFromReplicas mock
	if len(b.vaultDeleteResults) > 0 || b.vaultErrors[VaultDeleteSecretFromReplicas] != nil {
		if vaultError, hasError := b.vaultErrors[VaultDeleteSecretFromReplicas]; hasError {
			b.mockVault.On("DeleteSecretFromReplicas", mock.Anything, b.mount, b.keyPath).Return(nil, vaultError)
		} else {
			b.mockVault.On("DeleteSecretFromReplicas", mock.Anything, b.mount, b.keyPath).Return(b.vaultDeleteResults, nil)
		}
	}

	return b.mockRepo, b.mockVault
}
