package repository

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"vault-sync/internal/models"
	"vault-sync/pkg/db"
	"vault-sync/pkg/db/migrations"
	"vault-sync/testutil"
)

type SyncedSecretRepositoryTestSuite struct {
	suite.Suite
	pgHelper *testutil.PostgresHelper
	repo     *PsqlSyncedSecretRepository
}

func TestSyncedSecretRepositorySuite(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		t.Skip("Skipping integration tests")
	}
	suite.Run(t, new(SyncedSecretRepositoryTestSuite))
}

func (repoTest *SyncedSecretRepositoryTestSuite) SetupSuite() {
	var err error
	repoTest.pgHelper, err = testutil.NewPostgresContainer(repoTest.T(), context.Background())
	require.NoError(repoTest.T(), err, "Failed to create Postgres test container")

	psql, err := db.NewPostgresDatastore(*repoTest.pgHelper.Config, migrations.NewPostgresMigration())
	repo := NewPsqlSyncedSecretRepository(psql)
	require.NoError(repoTest.T(), err, "Failed to create PsqlSyncedSecretRepository")
	repoTest.repo = repo
}

func (repoTest *SyncedSecretRepositoryTestSuite) TearDownSuite() {
	ctx := context.Background()
	if repoTest.repo.psql.DB != nil {
		err := repoTest.repo.psql.Close()
		if err != nil {
			log.Printf("Error closing datastore: %v", err)
		}
	}
	if repoTest.pgHelper != nil {
		err := repoTest.pgHelper.Terminate(ctx)
		if err != nil {
			log.Printf("Error terminating container: %v", err)
		}
	}
}

func (repoTest *SyncedSecretRepositoryTestSuite) SetupTest() {
	_, err := repoTest.repo.psql.DB.Exec("TRUNCATE synced_secrets")
	require.NoError(repoTest.T(), err, "Failed to truncate synced_secrets table")
}

type syncedSecretGetSecretTestCase struct {
	name               string
	secretToInsert     *models.SyncedSecret
	backend            string
	path               string
	destinationCluster string
	expectError        bool
	expectedSecret     *models.SyncedSecret
}

func (repoTest *SyncedSecretRepositoryTestSuite) TestGetSyncedSecret() {
	now := time.Now().UTC().Truncate(time.Millisecond)
	successTime := now.Add(-1 * time.Minute)
	secretToInsert := &models.SyncedSecret{
		SecretBackend:      "kv",
		SecretPath:         "test/path",
		SourceVersion:      2,
		DestinationCluster: "prod",
		DestinationVersion: 1,
		LastSyncAttempt:    now,
		LastSyncSuccess:    &successTime,
		Status:             "success",
		ErrorMessage:       nil,
	}

	testCases := []syncedSecretGetSecretTestCase{
		{
			name:               "return existing secret",
			secretToInsert:     secretToInsert,
			backend:            "kv",
			path:               "test/path",
			destinationCluster: "prod",
			expectError:        false,
			expectedSecret:     secretToInsert,
		},
		{
			name:               "return error for non-existent secret",
			secretToInsert:     secretToInsert,
			backend:            "kv",
			path:               "does/not/exist",
			destinationCluster: "prod",
			expectError:        true,
			expectedSecret:     nil,
		},
	}

	for _, tc := range testCases {
		repoTest.T().Run(tc.name, func(t *testing.T) {
			repoTest.SetupTest()
			repoTest.insertTestSecret(*tc.secretToInsert)

			result, err := repoTest.repo.GetSyncedSecret(tc.backend, tc.path, tc.destinationCluster)

			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)

				assert.Equal(t, tc.expectedSecret.SecretBackend, result.SecretBackend)
				assert.Equal(t, tc.expectedSecret.SecretPath, result.SecretPath)
				assert.Equal(t, tc.expectedSecret.SourceVersion, result.SourceVersion)
				assert.Equal(t, tc.expectedSecret.DestinationCluster, result.DestinationCluster)
				assert.Equal(t, tc.expectedSecret.DestinationVersion, result.DestinationVersion)
				assert.Equal(t, tc.expectedSecret.Status, result.Status)
				assert.WithinDuration(t, tc.expectedSecret.LastSyncAttempt, result.LastSyncAttempt, time.Second)
			}
		})
	}
}

func (repoTest *SyncedSecretRepositoryTestSuite) TestGetSyncedSecrets() {
	repoTest.T().Run("returns all synced secrets", func(t *testing.T) {
		repoTest.SetupTest()

		now := time.Now().UTC().Truncate(time.Millisecond)
		successTime := now.Add(-1 * time.Minute)
		errorMsg := "test error"

		secrets := []models.SyncedSecret{
			{
				SecretBackend:      "zkv",
				SecretPath:         "test/path1",
				SourceVersion:      1,
				DestinationCluster: "prod",
				DestinationVersion: 1,
				LastSyncAttempt:    now,
				LastSyncSuccess:    &successTime,
				Status:             "success",
				ErrorMessage:       nil,
			},
			{
				SecretBackend:      "kv",
				SecretPath:         "test/path2",
				SourceVersion:      2,
				DestinationCluster: "stage",
				DestinationVersion: 2,
				LastSyncAttempt:    now,
				LastSyncSuccess:    nil,
				Status:             "error",
				ErrorMessage:       &errorMsg,
			},
			{
				SecretBackend:      "aws",
				SecretPath:         "test/path3",
				SourceVersion:      3,
				DestinationCluster: "dev",
				DestinationVersion: 1,
				LastSyncAttempt:    now,
				LastSyncSuccess:    &successTime,
				Status:             "success",
				ErrorMessage:       nil,
			},
		}

		for _, secret := range secrets {
			repoTest.insertTestSecret(secret)
		}

		result, err := repoTest.repo.GetSyncedSecrets()

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, len(secrets))

		assert.Equal(t, "aws", result[0].SecretBackend)
		assert.Equal(t, "kv", result[1].SecretBackend)
		assert.Equal(t, "zkv", result[2].SecretBackend)
	})

	repoTest.T().Run("returns empty slice for no secrets", func(t *testing.T) {
		repoTest.SetupTest()
		result, err := repoTest.repo.GetSyncedSecrets()

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Len(t, result, 0)
	})

	repoTest.T().Run("handles database error", func(t *testing.T) {
		repoTest.SetupTest()
		repoTest.repo.psql.Close()
		result, err := repoTest.repo.GetSyncedSecrets()

		assert.Error(t, err, "Failed to get all synced secrets")
		assert.NotNil(t, result)
	})
}

func (repoTest *SyncedSecretRepositoryTestSuite) insertTestSecret(secret models.SyncedSecret) {
	query := `
        INSERT INTO synced_secrets (
            secret_backend,
            secret_path,
            source_version,
            destination_cluster,
            destination_version,
            last_sync_attempt,
            last_sync_success,
            status,
            error_message
        ) VALUES (:secret_backend, :secret_path, :source_version, :destination_cluster, :destination_version, :last_sync_attempt, :last_sync_success, :status, :error_message)
    `

	_, err := repoTest.repo.psql.DB.NamedExec(query, secret)

	require.NoError(repoTest.T(), err, "Failed to insert test secret")
}
