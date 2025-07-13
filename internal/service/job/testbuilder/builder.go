package testbuilder

import (
	"context"
	"vault-sync/internal/models"
	"vault-sync/internal/repository"
	"vault-sync/internal/service/job"
	"vault-sync/internal/vault"

	"github.com/stretchr/testify/mock"
)

type syncJobMockBuilder struct {
	ctx      context.Context
	mount    string
	keyPath  string
	clusters []string

	mockRepo  *job.MockRepository
	mockVault *job.MockVaultClient
}

func NewSyncJobMockBuilder() MockBuilder {
	return &syncJobMockBuilder{
		ctx:       context.Background(),
		mockRepo:  new(job.MockRepository),
		mockVault: new(job.MockVaultClient),
	}
}

func (b *syncJobMockBuilder) WithMount(mount string) MockBuilder {
	b.mount = mount
	return b
}

func (b *syncJobMockBuilder) WithKeyPath(keyPath string) MockBuilder {
	b.keyPath = keyPath
	return b
}

func (b *syncJobMockBuilder) WithClusters(clusters ...string) MockDatabaseStage {
	b.clusters = clusters
	b.mockVault.On("GetReplicaNames").Return(b.clusters)
	return b
}

func (b *syncJobMockBuilder) WithGetSyncedSecretResult(version int64, clusters ...string) MockVaultStage {
	for _, cluster := range clusters {
		secret := &models.SyncedSecret{
			SecretBackend:      b.mount,
			SecretPath:         b.keyPath,
			DestinationCluster: cluster,
			SourceVersion:      version,
		}
		b.mockRepo.On("GetSyncedSecret", b.mount, b.keyPath, cluster).Return(secret, nil)
	}
	return b
}

func (b *syncJobMockBuilder) WithNotFoundGetSyncedSecret(clusters ...string) MockSyncActionStage {
	for _, cluster := range clusters {
		b.mockRepo.On("GetSyncedSecret", b.mount, b.keyPath, cluster).Return(nil, repository.ErrSecretNotFound)
	}
	return b
}

func (b *syncJobMockBuilder) WithGetSyncedSecretError(err error) MockBuildable {
	// Assume error happens on the first cluster check
	if len(b.clusters) > 0 {
		b.mockRepo.On("GetSyncedSecret", b.mount, b.keyPath, b.clusters[0]).Return(nil, err)
	}
	return b
}

func (b *syncJobMockBuilder) WithVaultSecretExists(exists bool) MockVaultExistenceStage {
	b.mockVault.On("SecretExists", mock.Anything, b.mount, b.keyPath).Return(exists, nil)
	return b
}

func (b *syncJobMockBuilder) WhenSecretExists(version int64) MockSyncActionStage {
	metadata := &vault.VaultSecretMetadataResponse{CurrentVersion: version}
	b.mockVault.On("GetSecretMetadata", mock.Anything, b.mount, b.keyPath).Return(metadata, nil)
	return b
}

func (b *syncJobMockBuilder) WhenSecretDoesNotExist() MockDeleteActionStage {
	// The WithVaultSecretExists(false) already set this up.
	return b
}

func (b *syncJobMockBuilder) WithSecretExistsError(err error) MockBuildable {
	b.mockVault.On("SecretExists", mock.Anything, b.mount, b.keyPath).Return(false, err)
	return b
}

func (b *syncJobMockBuilder) WithSyncSecretToReplicasResult(status models.SyncStatus, version int64, clusters ...string) MockUpdateStatusStage {
	var results []*models.SyncedSecret
	for _, cluster := range clusters {
		results = append(results, &models.SyncedSecret{
			SecretBackend:      b.mount,
			SecretPath:         b.keyPath,
			DestinationCluster: cluster,
			Status:             status,
			SourceVersion:      version,
		})
	}
	b.mockVault.On("SyncSecretToReplicas", mock.Anything, b.mount, b.keyPath).Return(results, nil)
	return b
}

func (b *syncJobMockBuilder) WithSyncSecretToReplicasError(err error) MockBuildable {
	b.mockVault.On("SyncSecretToReplicas", mock.Anything, b.mount, b.keyPath).Return(nil, err)
	return b
}

func (b *syncJobMockBuilder) WithUpdateSyncedSecretStatusResult(err error, clusters ...string) MockUpdateStatusStage {
	for _, cluster := range clusters {
		// The argument to UpdateSyncedSecretStatus is complex, so we use mock.AnythingOfType
		b.mockRepo.On("UpdateSyncedSecretStatus", mock.MatchedBy(func(s *models.SyncedSecret) bool {
			return s.DestinationCluster == cluster
		})).Return(err)
	}
	return b
}

func (b *syncJobMockBuilder) WithDeleteSecretFromReplicasResult(status models.SyncStatus, clusters ...string) MockDeleteFromDBStage {
	var results []*models.SyncSecretDeletionResult
	for _, cluster := range clusters {
		results = append(results, &models.SyncSecretDeletionResult{
			SecretBackend:      b.mount,
			SecretPath:         b.keyPath,
			DestinationCluster: cluster,
			Status:             status,
		})
	}
	b.mockVault.On("DeleteSecretFromReplicas", mock.Anything, b.mount, b.keyPath).Return(results, nil)
	return b
}

func (b *syncJobMockBuilder) WithDeleteSecretFromReplicasError(err error) MockBuildable {
	b.mockVault.On("DeleteSecretFromReplicas", mock.Anything, b.mount, b.keyPath).Return(nil, err)
	return b
}

func (b *syncJobMockBuilder) WithDeleteSyncedSecretResult(err error, clusters ...string) MockDeleteFromDBStage {
	for _, cluster := range clusters {
		b.mockRepo.On("DeleteSyncedSecret", b.mount, b.keyPath, cluster).Return(err)
	}
	return b
}

func (b *syncJobMockBuilder) Build() (*job.MockRepository, *job.MockVaultClient) {
	return b.mockRepo, b.mockVault
}
