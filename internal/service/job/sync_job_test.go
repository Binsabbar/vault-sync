package job

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

	// zerolog.SetGlobalLevel(zerolog.DebugLevel)
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
			suite.Equal(status.Status, SyncJobStatusUpdated)
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
			WithSyncSecretToReplicas(models.StatusSuccess, sourceVersion, clusters...).
			Build()
		worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

		jobResult, err := worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(status.Status, SyncJobStatusUpdated)
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
			suite.Equal(status.Status, SyncJobStatusUpdated)
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
			suite.Equal(status.Status, SyncJobStatusUpdated)
			suite.Contains(clusters, status.ClusterName)
		}
	})

	suite.Run("does not sync a secret if it exists in DB for all clusters and is up to date with source cluster", func() {
		mockRepo, mockVault := suite.builder.
			WithDatabaseSecretVersion(sourceVersion).
			WithGetSyncedSecret(clusters...).
			WithUpdateSyncedSecretStatus(models.StatusSuccess, sourceVersion, clusters...).
			SwitchToVaultStage().
			WithVaultSecretExists(true).
			WithGetSecretMetadata(sourceVersion).
			Build()
		worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

		jobResult, err := worker.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(jobResult.Status, 2)
		for _, status := range jobResult.Status {
			suite.Equal(status.Status, SyncJobStatusUnModified)
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
			suite.Equal(status.Status, SyncJobStatusDeleted)
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
				suite.Equal(status.Status, SyncJobStatusDeleted)
			} else {
				suite.Equal(status.Status, SyncJobStatusErrorDeleting)
			}
			suite.Contains(clusters, status.ClusterName)
		}
	})

	// if the secret does not exist in the source initially, do not even check the DB. 
}

func (suite *SyncJobTestSuite) TestExecute_Failure() {
	// sourceVersion := int64(1)
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
			suite.Equal(repository.ErrDatabaseGeneric, err)
		})

		suite.Run("return error on getting synced secret when DB raises error for all clusters", func() {
			mockRepo, mockVault := suite.builder.WithGetSyncedSecretError(repository.ErrDatabaseGeneric, clusters...).Build()
			worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

			_, err := worker.Execute(suite.ctx)

			suite.Error(err)
			suite.Equal(repository.ErrDatabaseGeneric, err)
		})

		suite.Run("return failed result on updating synced secret when error is raised for single cluster", func() {
			mockRepo, mockVault := suite.builder.
				WithGetSyncedSecretNotFound(clusters...).
				WithUpdateSyncedSecretStatusError(repository.ErrDatabaseGeneric, cluster1).
				WithUpdateSyncedSecretStatus(models.StatusSuccess, 2, cluster2).
				SwitchToVaultStage().
				WithSyncSecretToReplicas(models.StatusSuccess, 2, clusters...).
				Build()
			worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

			result, _ := worker.Execute(suite.ctx)

			resultStatus := result.Status
			suite.Len(resultStatus, 2)
			for _, status := range resultStatus {
				if status.ClusterName == cluster1 {
					suite.Equal(status.Status, SyncJobStatusFailed)
				} else {
					suite.Equal(status.Status, SyncJobStatusUpdated)
				}
				suite.Contains(clusters, status.ClusterName)
			}
		})

		suite.Run("return failed result on updating synced secret when error is raised for all clusters", func() {
			mockRepo, mockVault := suite.builder.
				WithGetSyncedSecretNotFound(clusters...).
				WithUpdateSyncedSecretStatusError(repository.ErrDatabaseGeneric, clusters...).
				SwitchToVaultStage().
				WithSyncSecretToReplicas(models.StatusSuccess, 2, clusters...).
				Build()
			worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

			result, _ := worker.Execute(suite.ctx)

			resultStatus := result.Status
			suite.Len(resultStatus, 2)
			for _, status := range resultStatus {
				suite.Equal(status.Status, SyncJobStatusFailed)
				suite.Contains(clusters, status.ClusterName)
			}
		})

		suite.Run("return error on deleting synced secret when error is raised for single cluster", func() {
			mockRepo, mockVault := suite.builder.
				WithDatabaseSecretVersion(1).WithGetSyncedSecret(clusters...).
				WithDeleteSyncedSecretError(repository.ErrDatabaseGeneric, cluster1).
				WithDeleteSyncedSecret(cluster2).
				SwitchToVaultStage().
				WithVaultSecretExists(false).
				WithDeleteSecretFromReplicas(models.StatusDeleted, clusters...).
				Build()
			worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

			_, err := worker.Execute(suite.ctx)

			suite.Error(err)
			suite.Equal(repository.ErrDatabaseGeneric, err)
		})

		suite.Run("return error on deleting synced secret when error is raised for all clusters", func() {
			mockRepo, mockVault := suite.builder.
				WithDatabaseSecretVersion(1).WithGetSyncedSecret(clusters...).
				WithDeleteSyncedSecretError(repository.ErrDatabaseGeneric, clusters...).
				SwitchToVaultStage().
				WithVaultSecretExists(false).
				WithDeleteSecretFromReplicas(models.StatusDeleted, clusters...).
				Build()
			worker := NewSyncJob(suite.mount, suite.keyPath, mockVault, mockRepo)

			_, err := worker.Execute(suite.ctx)

			suite.Error(err)
			suite.Equal(repository.ErrDatabaseGeneric, err)
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
			suite.Equal(fmt.Errorf("failed to sync secret"), err)
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
			suite.Equal(fmt.Errorf("failed to check secret existence"), err)
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
			suite.Equal(fmt.Errorf("failed to get secret metadata"), err)
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
			suite.Equal(fmt.Errorf("failed to delete secret from replicas"), err)
		})
	})
}
