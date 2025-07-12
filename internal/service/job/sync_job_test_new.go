package job

// import (
// 	"context"
// 	"fmt"
// 	"os"
// 	"testing"
// 	"vault-sync/internal/models"
// 	"vault-sync/internal/repository"

// 	"github.com/stretchr/testify/suite"
// )

// type SyncJobTestSuite struct {
// 	suite.Suite
// 	ctx     context.Context
// 	mount   string
// 	keyPath string
// }

// func (suite *SyncJobTestSuite) SetupTest() {
// 	suite.ctx = context.Background()
// 	suite.mount = "test-mount"
// 	suite.keyPath = "test/key/path"
// }

// func TestSyncJobTest(t *testing.T) {
// 	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
// 		t.Skip("Skipping integration tests")
// 	}

// 	suite.Run(t, new(SyncJobTestSuite))
// }

// func (suite *SyncJobTestSuite) TestExecute_Success() {
// 	sourceVersion := int64(1)

// 	suite.Run("syncs secret to replicas if secret does not exist in DB for all clusters", func() {
// 		worker := NewSyncJobTestBuilder().
// 			WithSecret(suite.mount, suite.keyPath).
// 			WithSourceVersion(sourceVersion).
// 			ForSecretNotInDB().
// 			Build()

// 		jobResult, err := worker.Execute(suite.ctx)

// 		suite.NoError(err)
// 		suite.Len(jobResult.Status, 2)
// 		for _, status := range jobResult.Status {
// 			suite.Equal(status.Status, SyncJobStatusUpdated)
// 			suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
// 		}
// 	})

// 	suite.Run("syncs secret to replicas if secret does not exist in DB for at least one cluster", func() {
// 		builder := NewSyncJobTestBuilder().
// 			WithSecret(suite.mount, suite.keyPath).
// 			WithSourceVersion(sourceVersion)

// 		// Set up mixed scenario: cluster1 has secret, cluster2 doesn't
// 		builder.WithDBSecret("cluster1", builder.NewSyncedSecret("cluster1", sourceVersion, models.StatusSuccess)).
// 			WithDBSecretNotFound("cluster2").
// 			WithVaultSyncResults([]*models.SyncedSecret{
// 				builder.NewSyncedSecret("cluster1", sourceVersion, models.StatusSuccess),
// 				builder.NewSyncedSecret("cluster2", sourceVersion, models.StatusSuccess),
// 			})

// 		worker := builder.Build()

// 		jobResult, err := worker.Execute(suite.ctx)

// 		suite.NoError(err)
// 		suite.Len(jobResult.Status, 2)
// 		for _, status := range jobResult.Status {
// 			suite.Equal(status.Status, SyncJobStatusUpdated)
// 			suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
// 		}
// 	})

// 	suite.Run("syncs secret to replicas if it exists in DB but is not up to date with source cluster", func() {
// 		worker := NewSyncJobTestBuilder().
// 			WithSecret(suite.mount, suite.keyPath).
// 			WithSourceVersion(sourceVersion).
// 			ForSecretOutOfDate().
// 			Build()

// 		jobResult, err := worker.Execute(suite.ctx)

// 		suite.NoError(err)
// 		suite.Len(jobResult.Status, 2)
// 		for _, status := range jobResult.Status {
// 			suite.Equal(status.Status, SyncJobStatusUpdated)
// 			suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
// 		}
// 	})

// 	suite.Run("syncs secret to replicas if it exists in DB but SOME are not up to date with source cluster", func() {
// 		builder := NewSyncJobTestBuilder().
// 			WithSecret(suite.mount, suite.keyPath).
// 			WithSourceVersion(sourceVersion)
// 		newVersion := sourceVersion + 1

// 		// cluster1 is up to date, cluster2 is not
// 		builder.WithDBSecret("cluster1", builder.NewSyncedSecret("cluster1", newVersion, models.StatusSuccess)).
// 			WithDBSecret("cluster2", builder.NewSyncedSecret("cluster2", sourceVersion, models.StatusSuccess)).
// 			WithSecretExists(true).
// 			WithSecretVersion(newVersion).
// 			WithVaultSyncResults([]*models.SyncedSecret{
// 				builder.NewSyncedSecret("cluster1", newVersion, models.StatusSuccess),
// 				builder.NewSyncedSecret("cluster2", newVersion, models.StatusSuccess),
// 			})

// 		worker := builder.Build()

// 		jobResult, err := worker.Execute(suite.ctx)

// 		suite.NoError(err)
// 		suite.Len(jobResult.Status, 2)
// 		for _, status := range jobResult.Status {
// 			suite.Equal(status.Status, SyncJobStatusUpdated)
// 			suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
// 		}
// 	})

// 	suite.Run("does not sync a secret if it exists in DB for all clusters and is up to date with source cluster", func() {
// 		worker := NewSyncJobTestBuilder().
// 			WithSecret(suite.mount, suite.keyPath).
// 			WithSourceVersion(sourceVersion).
// 			ForSecretUpToDate().
// 			Build()

// 		jobResult, err := worker.Execute(suite.ctx)

// 		suite.NoError(err)
// 		suite.Len(jobResult.Status, 2)
// 		for _, status := range jobResult.Status {
// 			suite.Equal(status.Status, SyncJobStatusUnModified)
// 			suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
// 		}
// 	})

// 	suite.Run("deletes secret from all replicas if it is in DB but not in source cluster", func() {
// 		worker := NewSyncJobTestBuilder().
// 			WithSecret(suite.mount, suite.keyPath).
// 			WithSourceVersion(sourceVersion).
// 			ForSecretDeleted().
// 			Build()

// 		jobResult, err := worker.Execute(suite.ctx)

// 		suite.NoError(err)
// 		suite.Len(jobResult.Status, 2)
// 		for _, status := range jobResult.Status {
// 			suite.Equal(status.Status, SyncJobStatusDeleted)
// 			suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
// 		}
// 	})
// }

// func (suite *SyncJobTestSuite) TestExecute_Failure() {
// 	suite.Run("report error when database returns error", func() {
// 		suite.Run("return error on getting synced secret", func() {
// 			worker := NewSyncJobTestBuilder().
// 				WithSecret(suite.mount, suite.keyPath).
// 				WithDBError("cluster1", repository.ErrDatabaseGeneric).
// 				Build()

// 			_, err := worker.Execute(suite.ctx)
// 			suite.Error(err)
// 			suite.Equal(repository.ErrDatabaseGeneric, err)
// 		})

// 		suite.Run("return failed result on updating synced secret on single cluster fails", func() {
// 			builder := NewSyncJobTestBuilder().WithSecret(suite.mount, suite.keyPath)
// 			worker := builder.
// 				WithDBSecretNotFound("cluster1").
// 				WithVaultSyncResults([]*models.SyncedSecret{
// 					builder.NewSyncedSecret("cluster1", 2, models.StatusSuccess),
// 					builder.NewSyncedSecret("cluster2", 2, models.StatusSuccess),
// 				}).
// 				WithUpdateError("cluster1", repository.ErrDatabaseGeneric).
// 				Build()

// 			result, _ := worker.Execute(suite.ctx)

// 			resultStatus := result.Status
// 			suite.Len(resultStatus, 2)
// 			for _, status := range resultStatus {
// 				if status.ClusterName == "cluster1" {
// 					suite.Equal(status.Status, SyncJobStatusFailed)
// 				} else {
// 					suite.Equal(status.Status, SyncJobStatusUpdated)
// 				}
// 				suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
// 			}
// 		})

// 		suite.Run("return failed result on updating synced secret when all cluster fails", func() {
// 			builder := NewSyncJobTestBuilder().WithSecret(suite.mount, suite.keyPath)
// 			worker := builder.
// 				WithDBSecretNotFound("cluster1").
// 				WithVaultSyncResults([]*models.SyncedSecret{
// 					builder.NewSyncedSecret("cluster1", 2, models.StatusSuccess),
// 					builder.NewSyncedSecret("cluster2", 2, models.StatusSuccess),
// 				}).
// 				WithUpdateError("cluster1", repository.ErrDatabaseGeneric).
// 				WithUpdateError("cluster2", repository.ErrDatabaseGeneric).
// 				Build()

// 			result, _ := worker.Execute(suite.ctx)

// 			resultStatus := result.Status
// 			suite.Len(resultStatus, 2)
// 			for _, status := range resultStatus {
// 				suite.Equal(status.Status, SyncJobStatusFailed)
// 				suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
// 			}
// 		})

// 		suite.Run("return failed result on deleting synced secret by vault client when it fails in one cluster", func() {
// 			builder := NewSyncJobTestBuilder().WithSecret(suite.mount, suite.keyPath)
// 			worker := builder.
// 				ForSecretDeleted().
// 				WithVaultDeleteResults([]*models.SyncSecretDeletionResult{
// 					builder.NewSyncSecretDeletionResult("cluster1", models.StatusFailed),
// 					builder.NewSyncSecretDeletionResult("cluster2", models.StatusDeleted),
// 				}).
// 				Build()

// 			result, _ := worker.Execute(suite.ctx)

// 			resultStatus := result.Status
// 			suite.Len(resultStatus, 2)
// 			for _, status := range resultStatus {
// 				if status.ClusterName == "cluster1" {
// 					suite.Equal(status.Status, SyncJobStatusErrorDeleting)
// 				} else {
// 					suite.Equal(status.Status, SyncJobStatusDeleted)
// 				}
// 				suite.Contains([]string{"cluster1", "cluster2"}, status.ClusterName)
// 			}
// 		})

// 		suite.Run("return error on deleting synced secret when DB fails deleting", func() {
// 			builder := NewSyncJobTestBuilder().WithSecret(suite.mount, suite.keyPath)
// 			worker := builder.
// 				ForSecretDeleted().
// 				WithDeleteError("cluster2", repository.ErrDatabaseGeneric).
// 				Build()

// 			_, err := worker.Execute(suite.ctx)

// 			suite.Error(err)
// 			suite.Equal(repository.ErrDatabaseGeneric, err)
// 		})
// 	})

// 	suite.Run("report an error when vault client fails", func() {
// 		suite.Run("returns an error on SyncSecretToReplicas", func() {
// 			worker := NewSyncJobTestBuilder().
// 				WithSecret(suite.mount, suite.keyPath).
// 				WithDBSecretNotFound("cluster1").
// 				WithVaultError("SyncSecretToReplicas", fmt.Errorf("failed to sync secret")).
// 				Build()

// 			_, err := worker.Execute(suite.ctx)
// 			suite.Error(err)
// 			suite.Equal(fmt.Errorf("failed to sync secret"), err)
// 		})

// 		suite.Run("returns an error on SecretExists", func() {
// 			builder := NewSyncJobTestBuilder().WithSecret(suite.mount, suite.keyPath)
// 			worker := builder.
// 				WithDBSecret("cluster1", builder.NewSyncedSecret("cluster1", 2, models.StatusSuccess)).
// 				WithDBSecret("cluster2", builder.NewSyncedSecret("cluster2", 2, models.StatusSuccess)).
// 				WithVaultError("SecretExists", fmt.Errorf("failed to check secret existence")).
// 				Build()

// 			_, err := worker.Execute(suite.ctx)
// 			suite.Error(err)
// 			suite.Equal(fmt.Errorf("failed to check secret existence"), err)
// 		})

// 		suite.Run("returns an error on GetSecretMetadata", func() {
// 			builder := NewSyncJobTestBuilder().WithSecret(suite.mount, suite.keyPath)
// 			worker := builder.
// 				WithDBSecret("cluster1", builder.NewSyncedSecret("cluster1", 2, models.StatusSuccess)).
// 				WithDBSecret("cluster2", builder.NewSyncedSecret("cluster2", 2, models.StatusSuccess)).
// 				WithSecretExists(true).
// 				WithVaultError("GetSecretMetadata", fmt.Errorf("failed to get secret metadata")).
// 				Build()

// 			_, err := worker.Execute(suite.ctx)
// 			suite.Error(err)
// 			suite.Equal(fmt.Errorf("failed to get secret metadata"), err)
// 		})

// 		suite.Run("returns an error on DeleteSecretFromReplicas", func() {
// 			builder := NewSyncJobTestBuilder().WithSecret(suite.mount, suite.keyPath)
// 			worker := builder.
// 				WithDBSecret("cluster1", builder.NewSyncedSecret("cluster1", 2, models.StatusSuccess)).
// 				WithDBSecret("cluster2", builder.NewSyncedSecret("cluster2", 2, models.StatusSuccess)).
// 				WithSecretExists(false).
// 				WithVaultError("DeleteSecretFromReplicas", fmt.Errorf("failed to delete secret from replicas")).
// 				Build()

// 			_, err := worker.Execute(suite.ctx)
// 			suite.Error(err)
// 			suite.Equal(fmt.Errorf("failed to delete secret from replicas"), err)
// 		})
// 	})
// }
