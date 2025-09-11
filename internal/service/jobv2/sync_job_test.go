package jobv2

import (
	"context"
	"fmt"
	"os"
	"testing"
	"vault-sync/internal/models"
	"vault-sync/internal/repository"
	"vault-sync/internal/service/job/testbuilder"

	"github.com/stretchr/testify/suite"
)

type SyncJobTestSuite struct {
	suite.Suite
	ctx     context.Context
	mount   string
	keyPath string
	worker  *SyncJob
	builder testbuilder.MockClustersStage
}

func (suite *SyncJobTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.mount = "test-mount"
	suite.keyPath = "test/key/path"
}

func (suite *SyncJobTestSuite) SetupSubTest() {
	suite.builder = testbuilder.NewSyncJobMockBuilder().WithKeyPath(suite.keyPath).WithMount(suite.mount).WithClusters(clusters...)
}

const (
	cluster1 = "cluster1"
	cluster2 = "cluster2"
)

var clusters = []string{cluster1, cluster2}

func TestSyncJobTest(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		t.Skip("Skipping integration tests")
	}
	suite.Run(t, new(SyncJobTestSuite))
}

func (suite *SyncJobTestSuite) TestExecute_Success() {
	secretVersion := int64(1)
	sourceVersion := int64(2)

	suite.Run("syncs secret to replicas if secret does not exist in DB for all clusters", func() {
		mockRepo, mockVault := suite.builder.
			WithGetSyncedSecretNotFound(clusters...).
			WithUpdateSyncedSecretStatus(models.StatusSuccess, sourceVersion, clusters...).
			SwitchToVaultStage().
			WithVaultSecretExists(true).
			WithSyncSecretToReplicas(models.StatusSuccess, sourceVersion, clusters...).
			Build()
		worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

		jobResult, err := worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(SyncJobStatusUpdated, status.Status)
			suite.Contains(clusters, status.ClusterName)
		}
	})

	suite.Run("syncs secret to replicas if secret does not exist in DB for at least one cluster", func() {
		mockRepo, mockVault := suite.builder.
			WithDatabaseSecretVersion(secretVersion).
			WithGetSyncedSecret(cluster1).
			WithGetSyncedSecretNotFound(cluster2).
			WithUpdateSyncedSecretStatus(models.StatusSuccess, sourceVersion, clusters...).
			SwitchToVaultStage().
			WithVaultSecretExists(true).
			WithSyncSecretToReplicas(models.StatusSuccess, sourceVersion, clusters...).
			Build()
		worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

		jobResult, err := worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(SyncJobStatusUpdated, status.Status)
			suite.Contains(clusters, status.ClusterName)
		}
	})

	suite.Run("syncs secret to replicas if it exists in DB but is not up to date with source cluster", func() {
		mockRepo, mockVault := suite.builder.
			WithDatabaseSecretVersion(secretVersion).
			WithGetSyncedSecret(clusters...).
			WithUpdateSyncedSecretStatus(models.StatusSuccess, sourceVersion, clusters...).
			SwitchToVaultStage().
			WithVaultSecretExists(true).
			WithGetSecretMetadata(sourceVersion).
			WithSyncSecretToReplicas(models.StatusSuccess, sourceVersion, clusters...).
			Build()
		worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

		jobResult, err := worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(SyncJobStatusUpdated, status.Status)
			suite.Contains(clusters, status.ClusterName)
		}
	})

	suite.Run("syncs secret to replicas if it exists in DB but SOME are not up to date with source cluster", func() {
		mockRepo, mockVault := suite.builder.
			WithDatabaseSecretVersion(secretVersion).
			WithGetSyncedSecret(cluster1).
			WithDatabaseSecretVersion(sourceVersion).
			WithGetSyncedSecret(cluster2).
			WithUpdateSyncedSecretStatus(models.StatusSuccess, sourceVersion, clusters...).
			SwitchToVaultStage().
			WithVaultSecretExists(true).
			WithGetSecretMetadata(sourceVersion).
			WithSyncSecretToReplicas(models.StatusSuccess, sourceVersion, clusters...).
			Build()
		worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

		jobResult, err := worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(SyncJobStatusUpdated, status.Status)
			suite.Contains(clusters, status.ClusterName)
		}
	})

	suite.Run("does not sync a secret if it exists in DB for all clusters and is up to date with source cluster", func() {
		mockRepo, mockVault := suite.builder.
			WithDatabaseSecretVersion(sourceVersion).
			WithGetSyncedSecret(clusters...).
			SwitchToVaultStage().
			WithVaultSecretExists(true).
			WithGetSecretMetadata(sourceVersion).
			Build()
		worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

		jobResult, err := worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(SyncJobStatusUnModified, status.Status)
			suite.Contains(clusters, status.ClusterName)
		}
	})

	suite.Run("deletes secret from all replicas if it is in DB but not in source cluster", func() {
		mockRepo, mockVault := suite.builder.
			WithDatabaseSecretVersion(sourceVersion).
			WithGetSyncedSecret(clusters...).
			WithDeleteSyncedSecret(clusters...).
			SwitchToVaultStage().
			WithVaultSecretExists(false).
			WithDeleteSecretFromReplicas(models.StatusDeleted, clusters...).
			Build()
		worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

		jobResult, err := worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(SyncJobStatusDeleted, status.Status)
			suite.Contains(clusters, status.ClusterName)
		}
	})

	suite.Run("returns partial success if delete fails for some replicas", func() {
		mockRepo, mockVault := suite.builder.
			WithDatabaseSecretVersion(sourceVersion).
			WithGetSyncedSecret(clusters...).
			WithDeleteSyncedSecret(clusters...).
			SwitchToVaultStage().
			WithVaultSecretExists(false).
			WithDeleteSecretFromReplicas(models.StatusDeleted, cluster1).
			WithDeleteSecretFromReplicas(models.StatusFailed, cluster2).
			Build()
		worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

		jobResult, err := worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			if status.ClusterName == cluster1 {
				suite.Equal(SyncJobStatusDeleted, status.Status)
			} else {
				suite.Equal(SyncJobStatusErrorDeleting, status.Status)
			}
			suite.Contains(clusters, status.ClusterName)
		}
	})

	suite.Run("no-op when source missing and no DB records exist", func() {
		mockRepo, mockVault := suite.builder.
			WithGetSyncedSecretNotFound(clusters...).
			SwitchToVaultStage().
			WithVaultSecretExists(false).
			Build()
		worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

		jobResult, err := worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(SyncJobStatusUnModified, status.Status)
			suite.Contains(clusters, status.ClusterName)
		}
	})
}

func (suite *SyncJobTestSuite) TestExecute_Failure() {
	suite.Run("repository client failures", func() {
		suite.Run("return error on getting synced secret when DB raises error for single cluster", func() {
			mockRepo, mockVault := suite.builder.
				WithDatabaseSecretVersion(1).
				WithGetSyncedSecret(cluster1).
				WithGetSyncedSecretError(repository.ErrDatabaseGeneric, cluster2).
				Build()
			worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

			_, err := worker.Execute(suite.ctx)

			suite.Error(err)
			suite.Contains(err.Error(), "failed to gather current state")
		})

		suite.Run("return partial result with error on updating synced secret when error is raised for single cluster", func() {
			mockRepo, mockVault := suite.builder.
				WithGetSyncedSecretNotFound(clusters...).
				WithUpdateSyncedSecretStatusError(repository.ErrDatabaseGeneric, cluster1).
				WithUpdateSyncedSecretStatus(models.StatusSuccess, 2, cluster2).
				SwitchToVaultStage().
				WithVaultSecretExists(true).
				WithSyncSecretToReplicas(models.StatusSuccess, 2, clusters...).
				Build()
			worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

			result, err := worker.Execute(suite.ctx)

			suite.NoError(err)         // No error returned, but partial failure in result
			suite.NotNil(result.Error) // Error should be in the result
			resultStatus := result.Status
			suite.Len(resultStatus, 2)
			for _, status := range resultStatus {
				if status.ClusterName == cluster1 {
					suite.Equal(SyncJobStatusFailed, status.Status)
				} else {
					suite.Equal(SyncJobStatusUpdated, status.Status)
				}
				suite.Contains(clusters, status.ClusterName)
			}
		})

		suite.Run("return partial result with error on deleting synced secret when error is raised for single cluster", func() {
			mockRepo, mockVault := suite.builder.
				WithDatabaseSecretVersion(1).WithGetSyncedSecret(clusters...).
				WithDeleteSyncedSecretError(repository.ErrDatabaseGeneric, cluster1).
				WithDeleteSyncedSecret(cluster2).
				SwitchToVaultStage().
				WithVaultSecretExists(false).
				WithDeleteSecretFromReplicas(models.StatusDeleted, clusters...).
				Build()
			worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

			result, err := worker.Execute(suite.ctx)

			suite.NoError(err)         // No error returned, but partial failure in result
			suite.NotNil(result.Error) // Error should be in the result
		})
	})

	suite.Run("vault client failures", func() {
		suite.Run("returns an error when SyncSecretToReplicas returns an error", func() {
			mockRepo, mockVault := suite.builder.
				WithGetSyncedSecretNotFound(clusters...).
				SwitchToVaultStage().
				WithVaultSecretExists(true).
				WithSyncSecretToReplicasError(fmt.Errorf("failed to sync secret")).
				Build()
			worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

			_, err := worker.Execute(suite.ctx)

			suite.Error(err)
			suite.Contains(err.Error(), "vault sync failed")
		})

		suite.Run("returns an error when SecretExists returns an error", func() {
			mockRepo, mockVault := suite.builder.
				WithDatabaseSecretVersion(1).
				WithGetSyncedSecret(clusters...).
				SwitchToVaultStage().
				WithVaultSecretExistsError(fmt.Errorf("failed to check secret existence")).
				Build()
			worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

			_, err := worker.Execute(suite.ctx)

			suite.Error(err)
			suite.Contains(err.Error(), "failed to gather current state")
		})

		suite.Run("returns an error when GetSecretMetadata returns an error", func() {
			mockRepo, mockVault := suite.builder.
				WithDatabaseSecretVersion(1).
				WithGetSyncedSecret(clusters...).
				SwitchToVaultStage().
				WithVaultSecretExists(true).
				WithGetSecretMetadataError(fmt.Errorf("failed to get secret metadata")).
				Build()
			worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

			_, err := worker.Execute(suite.ctx)

			suite.Error(err)
			suite.Contains(err.Error(), "failed to gather current state")
		})

		suite.Run("returns an error when DeleteSecretFromReplicas returns an error", func() {
			mockRepo, mockVault := suite.builder.
				WithDatabaseSecretVersion(1).
				WithGetSyncedSecret(clusters...).
				SwitchToVaultStage().
				WithVaultSecretExists(false).
				WithDeleteSecretFromReplicasError(fmt.Errorf("failed to delete secret from replicas")).
				Build()

			worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

			_, err := worker.Execute(suite.ctx)

			suite.Error(err)
			suite.Contains(err.Error(), "vault delete failed")
		})
	})
}
