//go:build integration

package job

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"vault-sync/internal/models"
	"vault-sync/internal/repository"
	"vault-sync/internal/repository/postgres"
	"vault-sync/internal/vault"
	"vault-sync/pkg/db"
	"vault-sync/pkg/db/migrations"
	"vault-sync/testutil"

	"github.com/stretchr/testify/suite"
)

type SyncJobIntegrationTestSuite struct {
	suite.Suite
	ctx context.Context

	vaultMainHelper     *testutil.VaultHelper
	vaultReplica1Helper *testutil.VaultHelper
	vaultReplica2Helper *testutil.VaultHelper
	vaultClient         vault.VaultSyncer

	pgHelper *testutil.PostgresHelper
	repo     repository.SyncedSecretRepository
}

var (
	teamAMount   = "team-a"
	teamBMount   = "team-b"
	replica1Name = "replica-0"
	replica2Name = "replica-1"
	mounts       = []string{teamAMount, teamBMount}
)

func (suite *SyncJobIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	suite.setupVaultClient()
	suite.setupRepositoryClient()
}

func (suite *SyncJobIntegrationTestSuite) SetupTest() {
	suite.pgHelper.ExecutePsqlCommand(context.Background(), "TRUNCATE TABLE synced_secrets")
	testutil.TruncateSecrets(suite.vaultMainHelper, suite.vaultReplica1Helper, suite.vaultReplica2Helper, mounts...)
}

func (suite *SyncJobIntegrationTestSuite) SetupSubTest() {
	suite.pgHelper.ExecutePsqlCommand(context.Background(), "TRUNCATE TABLE synced_secrets")
	testutil.TruncateSecrets(suite.vaultMainHelper, suite.vaultReplica1Helper, suite.vaultReplica2Helper, mounts...)
}

func (suite *SyncJobIntegrationTestSuite) TearDownTest() {
	testutil.TerminateAllClusters(suite.vaultMainHelper, suite.vaultReplica1Helper, suite.vaultReplica2Helper)
	suite.pgHelper.Terminate(suite.ctx)
}

func (suite *SyncJobIntegrationTestSuite) setupVaultClient() {
	var err error
	result := testutil.SetupOneMainTwoReplicaClusters(mounts...)
	suite.vaultMainHelper = result.MainVault
	suite.vaultReplica1Helper = result.Replica1Vault
	suite.vaultReplica2Helper = result.Replica2Vault
	suite.vaultClient, err = vault.NewMultiClusterVaultClient(suite.ctx, result.MainConfig, result.ReplicasConfig)
	suite.NoError(err, "Failed to create vault client")
}

func (suite *SyncJobIntegrationTestSuite) setupRepositoryClient() {
	var err error
	suite.pgHelper, err = testutil.NewPostgresContainer(suite.T(), suite.ctx)
	suite.NoError(err, "Failed to create Postgres container")
	db, err := db.NewPostgresDatastore(suite.pgHelper.Config, migrations.NewPostgresMigration())
	suite.NoError(err, "Failed to create PostgreSQLSyncedSecretRepository")
	suite.repo = postgres.NewPostgreSQLSyncedSecretRepository(db)
}

func TestSyncJobIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(SyncJobIntegrationTestSuite))
}

func (suite *SyncJobIntegrationTestSuite) TestSyncJob_Execute() {
	suite.Run("sync job with secret from main cluster", func() {
		keyPath := "secret1"
		sourceSecretData := map[string]string{
			"database": "testdb",
			"username": "testuser",
			"password": "testpass",
		}
		suite.vaultMainHelper.WriteSecret(suite.ctx, teamAMount, keyPath, sourceSecretData)
		job := NewSyncJob(teamAMount, keyPath, suite.vaultClient, suite.repo)

		result, err := job.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(result.Status, 2)
		suite.verifySecretsAndDatabase(keyPath, sourceSecretData, result, map[string]int64{})
	})

	suite.Run("sync job with secret from main cluster if secret exists in single replica only", func() {
		keyPath := "secret1"
		sourceSecretData := map[string]string{
			"database": "testdb1",
			"username": "testuser2",
			"password": "testpass3",
		}
		suite.vaultMainHelper.WriteSecret(suite.ctx, teamAMount, keyPath, sourceSecretData)
		suite.vaultReplica1Helper.WriteSecret(suite.ctx, teamAMount, keyPath, sourceSecretData)
		job := NewSyncJob(teamAMount, keyPath, suite.vaultClient, suite.repo)

		result, err := job.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(result.Status, 2)
		suite.verifySecretsAndDatabase(keyPath, sourceSecretData, result, map[string]int64{
			replica1Name: 2,
			replica2Name: 1,
		})
	})

	suite.Run("re-sync existing secrets in replica if source version changed", func() {
		keyPath := "secret1"
		suite.vaultMainHelper.WriteSecret(suite.ctx, teamAMount, keyPath, map[string]string{
			"database": "testdb1",
			"username": "testuser2",
			"password": "testpass3",
		})
		job := NewSyncJob(teamAMount, keyPath, suite.vaultClient, suite.repo)

		// Run initial sync
		job.Execute(suite.ctx)

		// Update the secret in the main cluster
		updatedSecretData := map[string]string{
			"database": "testdb2",
			"username": "testuser3",
			"password": "testpass4",
			"version":  "2",
		}
		suite.vaultMainHelper.WriteSecret(suite.ctx, teamAMount, keyPath, updatedSecretData)

		// Run sync again
		result, err := job.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(result.Status, 2)
		suite.verifySecretsAndDatabase(keyPath, updatedSecretData, result, map[string]int64{
			replica1Name: 2,
			replica2Name: 2,
		})
	})

	suite.Run("sync job with secret deletion from main cluster", func() {
		keyPath := "secret1"
		sourceSecretData := map[string]string{
			"database": "testdb",
			"username": "testuser",
			"password": "testpass",
		}
		suite.vaultMainHelper.WriteSecret(suite.ctx, teamAMount, keyPath, sourceSecretData)
		job := NewSyncJob(teamAMount, keyPath, suite.vaultClient, suite.repo)

		// Run initial sync
		result, _ := job.Execute(suite.ctx)
		suite.verifySecretsAndDatabase(keyPath, sourceSecretData, result, map[string]int64{})

		// Delete the secret in the main cluster
		suite.vaultMainHelper.DeleteSecret(suite.ctx, fmt.Sprintf("%s/%s", teamAMount, keyPath))

		// Run sync again
		deleteResult, err := job.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(deleteResult.Status, 2)
		suite.verifySecretsAndDatabaseDeleted(keyPath, deleteResult)
	})

	suite.Run("no-op when source missing and no DB records", func() {
		keyPath := "nonexistent-secret"
		job := NewSyncJob(teamAMount, keyPath, suite.vaultClient, suite.repo)

		result, err := job.Execute(suite.ctx)

		suite.NoError(err)
		suite.Len(result.Status, 2)
		for _, status := range result.Status {
			suite.Equal(SyncJobStatusUnModified, status.Status)
		}
	})
}

var selectQuery = "SELECT row_to_json(t) FROM (SELECT * FROM synced_secrets where secret_backend = '%s' AND secret_path = '%s' AND destination_cluster = '%s') t"

func (suite *SyncJobIntegrationTestSuite) verifySecretsAndDatabaseDeleted(keyPath string, result *SyncJobResult) {
	// Verify DB: no rows for both replicas
	for _, cluster := range []string{replica1Name, replica2Name} {
		sql, err := suite.pgHelper.ExecutePsqlCommand(suite.ctx, fmt.Sprintf(selectQuery, teamAMount, keyPath, cluster))
		suite.NoError(err)
		if strings.TrimSpace(sql) != "" {
			suite.Failf("db-not-empty", "expected no DB row for %s/%s (%s), got: %s", teamAMount, keyPath, cluster, sql)
		}
	}

	// Verify Vault: secret is gone from both replicas
	for i, helper := range []*testutil.VaultHelper{suite.vaultReplica1Helper, suite.vaultReplica2Helper} {
		data, _, err := helper.ReadSecretData(suite.ctx, teamAMount, keyPath)
		suite.Error(err, "expected read error for deleted secret on replica-%d", i)
		suite.Nil(data, "expected no data on replica-%d", i)
	}

	// Verify job result statuses are Deleted per replica
	if result != nil && len(result.Status) >= 2 {
		for i := 0; i < 2; i++ {
			suite.Equal(fmt.Sprintf("replica-%d", i), result.Status[i].ClusterName)
			suite.Equal(SyncJobStatusDeleted, result.Status[i].Status)
		}
	}
}

func (suite *SyncJobIntegrationTestSuite) verifySecretsAndDatabase(keyPath string, sourceSecretData map[string]string, result *SyncJobResult, expectedVersions map[string]int64) {
	// Check DB records for both replicas
	sql, _ := suite.pgHelper.ExecutePsqlCommand(suite.ctx, fmt.Sprintf(selectQuery, teamAMount, keyPath, replica1Name))
	replica0Secret := mapDatabaseRecordToSyncedSecret(sql)
	sql, _ = suite.pgHelper.ExecutePsqlCommand(suite.ctx, fmt.Sprintf(selectQuery, teamAMount, keyPath, replica2Name))
	replica1Secret := mapDatabaseRecordToSyncedSecret(sql)

	for index, record := range []*models.SyncedSecret{replica0Secret, replica1Secret} {
		destinationCluster := fmt.Sprintf("replica-%d", index)
		suite.Equal(destinationCluster, record.DestinationCluster)
		suite.Equal(teamAMount, record.SecretBackend)
		suite.Equal(keyPath, record.SecretPath)

		// Check source version
		suite.Equal(int64(1), record.SourceVersion) // Default to 1 if not specified

		// Check destination version
		if expectedVersion, ok := expectedVersions[destinationCluster]; ok {
			suite.Equal(expectedVersion, record.DestinationVersion)
		} else {
			suite.Equal(int64(1), record.DestinationVersion)
		}

		// Check job result status
		suite.Equal(fmt.Sprintf("replica-%d", index), result.Status[index].ClusterName)
		suite.Equal(SyncJobStatusUpdated, result.Status[index].Status)
	}

	// Check secrets in replicas
	replica0SecretData, _, _ := suite.vaultReplica1Helper.ReadSecretData(suite.ctx, teamAMount, keyPath)
	suite.Equal(sourceSecretData, replica0SecretData)

	replica1SecretData, _, _ := suite.vaultReplica2Helper.ReadSecretData(suite.ctx, teamAMount, keyPath)
	suite.Equal(sourceSecretData, replica1SecretData)
}

func mapDatabaseRecordToSyncedSecret(record string) *models.SyncedSecret {
	var jsonString map[string]interface{}
	err := json.Unmarshal([]byte(record), &jsonString)
	if err != nil {
		panic(fmt.Sprintf("Failed to unmarshal database record: %v", err))
	}
	return &models.SyncedSecret{
		SecretBackend:      jsonString["secret_backend"].(string),
		SecretPath:         jsonString["secret_path"].(string),
		DestinationCluster: jsonString["destination_cluster"].(string),
		SourceVersion:      int64(jsonString["source_version"].(float64)),
		DestinationVersion: int64(jsonString["destination_version"].(float64)),
	}
}
