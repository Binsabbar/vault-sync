package job

import (
	"context"
	"fmt"
	"os"
	"testing"
	"vault-sync/internal/models"
	"vault-sync/internal/repository"
	"vault-sync/internal/vault"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type SyncJobTestSuite struct {
	suite.Suite
	ctx       context.Context
	mount     string
	keyPath   string
	mockVault *mockVaultClient
	mockRepo  *mockRepository
	worker    *SyncJob
}

func (suite *SyncJobTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.mount = "test-mount"
	suite.keyPath = "test/key/path"
}

func (suite *SyncJobTestSuite) SetupSubTest() {
	suite.mockVault = new(mockVaultClient)
	suite.mockRepo = new(mockRepository)
	suite.mockVault.On("GetReplicaNames").Return([]string{"cluster1", "cluster2"})
}

func (suite *SyncJobTestSuite) TearDownSubTest() {
	// suite.mockVault.AssertExpectations(suite.T())
	// suite.mockRepo.AssertExpectations(suite.T())
}

func TestSyncJobTest(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		t.Skip("Skipping integration tests")
	}

	// zerolog.SetGlobalLevel(zerolog.DebugLevel)
	suite.Run(t, new(SyncJobTestSuite))
}

func (suite *SyncJobTestSuite) TestExecute_Success() {
	sourceVersion := int64(1)

	suite.Run("syncs secret to replicas if secret does not exist in DB for all clusters", func() {
		suite.stubDbGetSyncedSecret(nil, repository.ErrSecretNotFound, "cluster1")
		// suite.stubDbGetSyncedSecret(nil, repository.ErrSecretNotFound, "cluster1", "cluster2")
		suite.stubDbUpdateSyncedSecretStatus("cluster1", sourceVersion, models.StatusSuccess)
		suite.stubDbUpdateSyncedSecretStatus("cluster2", sourceVersion, models.StatusSuccess)

		suite.mockVault.On("SyncSecretToReplicas", mock.Anything, suite.mount, suite.keyPath).Return([]*models.SyncedSecret{
			suite.newSyncSecret("cluster1", sourceVersion, models.StatusSuccess),
			suite.newSyncSecret("cluster2", sourceVersion, models.StatusSuccess),
		}, nil)

		suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

		jobResult, err := suite.worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(status.Status, SyncJobStatusUpdated)
			suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
		}
	})

	suite.Run("syncs secret to replicas if secret does not exist in DB for at least one cluster", func() {
		suite.stubDbGetSyncedSecret(&models.SyncedSecret{}, nil, "cluster1")
		suite.stubDbGetSyncedSecret(nil, repository.ErrSecretNotFound, "cluster2")
		suite.stubDbUpdateSyncedSecretStatus("cluster1", sourceVersion, models.StatusSuccess)
		suite.stubDbUpdateSyncedSecretStatus("cluster2", sourceVersion, models.StatusSuccess)

		suite.mockVault.On("SyncSecretToReplicas", mock.Anything, suite.mount, suite.keyPath).Return([]*models.SyncedSecret{
			suite.newSyncSecret("cluster1", sourceVersion, models.StatusSuccess),
			suite.newSyncSecret("cluster2", sourceVersion, models.StatusSuccess),
		}, nil)

		suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

		jobResult, err := suite.worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(status.Status, SyncJobStatusUpdated)
			suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
		}
	})

	suite.Run("syncs secret to replicas if it exists in DB but is not up to date with source cluster", func() {
		suite.stubDbGetSyncedSecret(&models.SyncedSecret{
			SecretBackend: suite.mount,
			SecretPath:    suite.keyPath,
			SourceVersion: sourceVersion,
		}, nil, "cluster1", "cluster2")
		suite.stubDbUpdateSyncedSecretStatus("cluster1", sourceVersion+1, models.StatusSuccess)
		suite.stubDbUpdateSyncedSecretStatus("cluster2", sourceVersion+1, models.StatusSuccess)

		suite.mockVault.On("SecretExists", mock.Anything, suite.mount, suite.keyPath).Return(true, nil)
		suite.mockVault.
			On("GetSecretMetadata", mock.Anything, suite.mount, suite.keyPath).
			Return(&vault.VaultSecretMetadataResponse{CurrentVersion: sourceVersion + 1}, nil)
		suite.mockVault.
			On("SyncSecretToReplicas", mock.Anything, suite.mount, suite.keyPath).
			Return([]*models.SyncedSecret{
				suite.newSyncSecret("cluster1", sourceVersion+1, models.StatusSuccess),
				suite.newSyncSecret("cluster2", sourceVersion+1, models.StatusSuccess),
			}, nil)

		suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

		jobResult, err := suite.worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(status.Status, SyncJobStatusUpdated)
			suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
		}
	})

	suite.Run("syncs secret to replicas if it exists in DB but SOME are not up to date with source cluster", func() {
		clusterOneSyncedSecret := suite.newSyncSecret("cluster1", sourceVersion+1, models.StatusSuccess)
		clusterTwoSyncedSecret := suite.newSyncSecret("cluster2", sourceVersion+1, models.StatusSuccess)

		suite.stubDbGetSyncedSecret(clusterOneSyncedSecret, nil, "cluster1")
		suite.stubDbGetSyncedSecret(suite.newSyncSecret("cluster2", sourceVersion, models.StatusSuccess), nil, "cluster2")
		suite.stubDbUpdateSyncedSecretStatus("cluster1", sourceVersion+1, models.StatusSuccess)
		suite.stubDbUpdateSyncedSecretStatus("cluster2", sourceVersion+1, models.StatusSuccess)

		suite.mockVault.On("SecretExists", mock.Anything, suite.mount, suite.keyPath).Return(true, nil)
		suite.mockVault.
			On("GetSecretMetadata", mock.Anything, suite.mount, suite.keyPath).
			Return(&vault.VaultSecretMetadataResponse{CurrentVersion: sourceVersion + 1}, nil)
		suite.mockVault.
			On("SyncSecretToReplicas", mock.Anything, suite.mount, suite.keyPath).
			Return([]*models.SyncedSecret{clusterOneSyncedSecret, clusterTwoSyncedSecret}, nil)

		suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

		jobResult, err := suite.worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(status.Status, SyncJobStatusUpdated)
			suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
		}
	})

	suite.Run("does not sync a secret if it exists in DB for all clusters and is up to date with source cluster", func() {
		suite.stubDbGetSyncedSecret(suite.newSyncSecret("cluster1", sourceVersion, models.StatusSuccess), nil, "cluster1", "cluster2")
		suite.mockVault.On("SecretExists", mock.Anything, suite.mount, suite.keyPath).Return(true, nil)
		suite.mockVault.
			On("GetSecretMetadata", mock.Anything, suite.mount, suite.keyPath).
			Return(&vault.VaultSecretMetadataResponse{CurrentVersion: sourceVersion}, nil)

		suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

		jobResult, err := suite.worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(status.Status, SyncJobStatusUnModified)
			suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
		}
	})

	suite.Run("deletes secret from all replicas if it is in DB but not in source cluster", func() {
		suite.stubDbGetSyncedSecret(suite.newSyncSecret("cluster1", sourceVersion, models.StatusSuccess), nil, "cluster1", "cluster2")
		suite.mockRepo.On("DeleteSyncedSecret", suite.mount, suite.keyPath, "cluster1").Return(nil)
		suite.mockRepo.On("DeleteSyncedSecret", suite.mount, suite.keyPath, "cluster2").Return(nil)

		suite.mockVault.On("SecretExists", mock.Anything, suite.mount, suite.keyPath).Return(false, nil)
		suite.mockVault.On("DeleteSecretFromReplicas", mock.Anything, suite.mount, suite.keyPath).Return([]*models.SyncSecretDeletionResult{
			suite.newSyncSecretDeletionResult("cluster1", sourceVersion, models.StatusDeleted),
			suite.newSyncSecretDeletionResult("cluster2", sourceVersion, models.StatusDeleted),
		}, nil)

		suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

		jobResult, err := suite.worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(status.Status, SyncJobStatusDeleted)
			suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
		}
	})
}

func (suite *SyncJobTestSuite) TestExecute_Failure() {
	// sourceVersion := int64(1)
	suite.Run("report error when database returns error", func() {
		suite.Run("return error on getting synced secret", func() {
			suite.mockRepo.On("GetSyncedSecret", suite.mount, suite.keyPath, "cluster1").Return(nil, repository.ErrDatabaseGeneric)
			suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)
			_, err := suite.worker.Execute(suite.ctx)
			suite.Error(err)
			suite.Equal(repository.ErrDatabaseGeneric, err)
		})

		suite.Run("return failed result on updating synced secret on single cluster fails", func() {
			clusterOneSecret := suite.newSyncSecret("cluster1", 2, models.StatusSuccess)
			clusterTwoSecret := suite.newSyncSecret("cluster2", 2, models.StatusSuccess)
			suite.stubDbGetSyncedSecret(nil, repository.ErrSecretNotFound, "cluster1")
			suite.mockRepo.On("UpdateSyncedSecretStatus", clusterOneSecret).Return(repository.ErrDatabaseGeneric)
			suite.mockRepo.On("UpdateSyncedSecretStatus", clusterTwoSecret).Return(nil)

			suite.mockVault.
				On("SyncSecretToReplicas", mock.Anything, suite.mount, suite.keyPath).
				Return([]*models.SyncedSecret{clusterOneSecret, clusterTwoSecret}, nil)

			suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

			result, _ := suite.worker.Execute(suite.ctx)

			resultStatus := result.Status
			suite.Len(resultStatus, 2)
			for _, status := range resultStatus {
				if status.ClusterName == "cluster1" {
					suite.Equal(status.Status, SyncJobStatusFailed)
				} else {
					suite.Equal(status.Status, SyncJobStatusUpdated)
				}
				suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
			}
		})

		suite.Run("return failed result on updating synced secret when all cluster fails", func() {
			clusterOneSecret := suite.newSyncSecret("cluster1", 2, models.StatusSuccess)
			clusterTwoSecret := suite.newSyncSecret("cluster2", 2, models.StatusSuccess)
			suite.stubDbGetSyncedSecret(nil, repository.ErrSecretNotFound, "cluster1")
			suite.mockRepo.On("UpdateSyncedSecretStatus", clusterOneSecret).Return(repository.ErrDatabaseGeneric)
			suite.mockRepo.On("UpdateSyncedSecretStatus", clusterTwoSecret).Return(repository.ErrDatabaseGeneric)

			suite.mockVault.
				On("SyncSecretToReplicas", mock.Anything, suite.mount, suite.keyPath).
				Return([]*models.SyncedSecret{clusterOneSecret, clusterTwoSecret}, nil)

			suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

			result, _ := suite.worker.Execute(suite.ctx)

			resultStatus := result.Status
			suite.Len(resultStatus, 2)
			for _, status := range resultStatus {
				suite.Equal(status.Status, SyncJobStatusFailed)
				suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
			}
		})

		suite.Run("return failed result on deleting synced secret by vault client when it fails in one cluster", func() {
			clusterSecret := suite.newSyncSecret("cluster1", 2, models.StatusSuccess)
			suite.stubDbGetSyncedSecret(clusterSecret, nil, "cluster1", "cluster2")
			suite.mockRepo.On("DeleteSyncedSecret", suite.mount, suite.keyPath, "cluster1").Return(nil)
			suite.mockRepo.On("DeleteSyncedSecret", suite.mount, suite.keyPath, "cluster2").Return(nil)
			suite.mockVault.
				On("DeleteSecretFromReplicas", mock.Anything, suite.mount, suite.keyPath).
				Return([]*models.SyncSecretDeletionResult{
					suite.newSyncSecretDeletionResult("cluster1", 2, models.StatusFailed),
					suite.newSyncSecretDeletionResult("cluster2", 2, models.StatusDeleted),
				}, nil)
			suite.mockVault.On("SecretExists", mock.Anything, suite.mount, suite.keyPath).Return(false, nil)

			suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

			result, _ := suite.worker.Execute(suite.ctx)

			resultStatus := result.Status
			suite.Len(resultStatus, 2)
			for _, status := range resultStatus {
				if status.ClusterName == "cluster1" {
					suite.Equal(status.Status, SyncJobStatusErrorDeleting)
				} else {
					suite.Equal(status.Status, SyncJobStatusDeleted)
				}
				suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
			}
		})

		suite.Run("return error on deleting synced secret when DB fails deleting", func() {
			clusterSecret := suite.newSyncSecret("cluster1", 2, models.StatusSuccess)
			suite.stubDbGetSyncedSecret(clusterSecret, nil, "cluster1", "cluster2")
			suite.mockRepo.On("DeleteSyncedSecret", suite.mount, suite.keyPath, "cluster1").Return(nil)
			suite.mockRepo.On("DeleteSyncedSecret", suite.mount, suite.keyPath, "cluster2").Return(repository.ErrDatabaseGeneric)
			suite.mockVault.
				On("DeleteSecretFromReplicas", mock.Anything, suite.mount, suite.keyPath).
				Return([]*models.SyncSecretDeletionResult{
					suite.newSyncSecretDeletionResult("cluster1", 2, models.StatusDeleted),
					suite.newSyncSecretDeletionResult("cluster2", 2, models.StatusDeleted),
				}, nil)
			suite.mockVault.On("SecretExists", mock.Anything, suite.mount, suite.keyPath).Return(false, nil)

			suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

			_, err := suite.worker.Execute(suite.ctx)

			suite.Error(err)
			suite.Equal(repository.ErrDatabaseGeneric, err)
		})
	})

	suite.Run("report an error when vault client fails", func() {
		suite.Run("returns an error on SyncSecretToReplicas", func() {
			suite.stubDbGetSyncedSecret(nil, repository.ErrSecretNotFound, "cluster1")
			suite.mockVault.On("SyncSecretToReplicas", mock.Anything, suite.mount, suite.keyPath).Return(nil, fmt.Errorf("failed to sync secret"))

			suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

			_, err := suite.worker.Execute(suite.ctx)
			suite.Error(err)
			suite.Equal(fmt.Errorf("failed to sync secret"), err)
		})

		suite.Run("returns an error on SecretExists", func() {
			suite.stubDbGetSyncedSecret(suite.newSyncSecret("cluster1", 2, models.StatusSuccess), nil, "cluster1", "cluster2")
			suite.mockVault.On("SecretExists", mock.Anything, suite.mount, suite.keyPath).Return(false, fmt.Errorf("failed to check secret existence"))

			suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

			_, err := suite.worker.Execute(suite.ctx)
			suite.Error(err)
			suite.Equal(fmt.Errorf("failed to check secret existence"), err)
		})

		suite.Run("returns an error on GetSecretMetadata", func() {
			suite.stubDbGetSyncedSecret(suite.newSyncSecret("cluster1", 2, models.StatusSuccess), nil, "cluster1", "cluster2")
			suite.mockVault.On("SecretExists", mock.Anything, suite.mount, suite.keyPath).Return(true, nil)
			suite.mockVault.On("GetSecretMetadata", mock.Anything, suite.mount, suite.keyPath).Return(nil, fmt.Errorf("failed to get secret metadata"))

			suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

			_, err := suite.worker.Execute(suite.ctx)
			suite.Error(err)
			suite.Equal(fmt.Errorf("failed to get secret metadata"), err)
		})

		suite.Run("returns an error on DeleteSecretFromReplicas", func() {
			suite.stubDbGetSyncedSecret(suite.newSyncSecret("cluster1", 2, models.StatusSuccess), nil, "cluster1", "cluster2")
			suite.mockVault.On("SecretExists", mock.Anything, suite.mount, suite.keyPath).Return(false, nil)
			suite.mockVault.On("DeleteSecretFromReplicas", mock.Anything, suite.mount, suite.keyPath).Return(nil, fmt.Errorf("failed to delete secret from replicas"))

			suite.worker = NewSyncJob(suite.mount, suite.keyPath, suite.mockVault, suite.mockRepo)

			_, err := suite.worker.Execute(suite.ctx)
			suite.Error(err)
			suite.Equal(fmt.Errorf("failed to delete secret from replicas"), err)
		})
	})
}

// stub factory methods
func (suite *SyncJobTestSuite) stubDbUpdateSyncedSecretStatus(cluster string, version int64, status models.SyncStatus) {
	suite.mockRepo.On("UpdateSyncedSecretStatus",
		&models.SyncedSecret{
			SecretBackend:      suite.mount,
			SecretPath:         suite.keyPath,
			DestinationCluster: cluster,
			SourceVersion:      version,
			Status:             status,
		}).Return(nil)
}

func (suite *SyncJobTestSuite) stubDbGetSyncedSecret(returnVal *models.SyncedSecret, err error, clusterNames ...string) {
	for _, clusterName := range clusterNames {
		var syncedSecret *models.SyncedSecret
		if returnVal != nil {
			syncedSecret = &models.SyncedSecret{
				SecretBackend:      returnVal.SecretBackend,
				SecretPath:         returnVal.SecretPath,
				DestinationCluster: clusterName,
				SourceVersion:      returnVal.SourceVersion,
				Status:             returnVal.Status,
			}
		}
		suite.mockRepo.On("GetSyncedSecret", suite.mount, suite.keyPath, clusterName).Return(syncedSecret, err)
	}
}

func (suite *SyncJobTestSuite) newSyncSecret(cluster string, version int64, status models.SyncStatus) *models.SyncedSecret {
	return &models.SyncedSecret{
		SecretBackend:      suite.mount,
		SecretPath:         suite.keyPath,
		DestinationCluster: cluster,
		SourceVersion:      version,
		Status:             status,
	}
}

func (suite *SyncJobTestSuite) newSyncSecretDeletionResult(cluster string, version int64, status models.SyncStatus) *models.SyncSecretDeletionResult {
	return &models.SyncSecretDeletionResult{
		SecretBackend:      suite.mount,
		SecretPath:         suite.keyPath,
		DestinationCluster: cluster,
		Status:             status,
	}
}
