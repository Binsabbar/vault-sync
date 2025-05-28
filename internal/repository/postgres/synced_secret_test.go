package repository

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/sony/gobreaker"
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
	require.NoError(repoTest.T(), err, "Failed to create PsqlSyncedSecretRepository")

	repoTest.repo = NewPsqlSyncedSecretRepository(psql)
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
	expectedErr        error
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
			expectedErr:        nil,
			expectedSecret:     secretToInsert,
		},
		{
			name:               "return error for non existent secret",
			secretToInsert:     secretToInsert,
			backend:            "kv",
			path:               "does/not/exist",
			destinationCluster: "prod",
			expectedErr:        ErrSecretNotFound,
			expectedSecret:     nil,
		},
	}

	for _, tc := range testCases {
		repoTest.T().Run(tc.name, func(t *testing.T) {
			repoTest.SetupTest()
			repoTest.insertTestSecret(*tc.secretToInsert)

			result, err := repoTest.repo.GetSyncedSecret(tc.backend, tc.path, tc.destinationCluster)

			if tc.expectedErr != nil {
				assert.ErrorIs(t, err, tc.expectedErr, "Expected error does not match")
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

		assert.ErrorIs(t, err, ErrSecretNotFound, "Expected ErrSecretNotFound error")
		assert.NotNil(t, result)
		assert.Len(t, result, 0)
	})
}

func (repoTest *SyncedSecretRepositoryTestSuite) TestFailureWithCircuitBreakerAndRetry() {
	newCircuitBreaker := func() *gobreaker.CircuitBreaker {
		return gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        "test_breaker",
			MaxRequests: 3,
			Interval:    1 * time.Second,
			Timeout:     1 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.TotalFailures >= 2
			},
		})
	}

	retryOpsFunc := func() []backoff.RetryOption {
		strategyOpts := &backoff.ConstantBackOff{Interval: 50 * time.Millisecond}
		maxRetires := backoff.WithMaxTries(1)
		return []backoff.RetryOption{backoff.WithBackOff(strategyOpts), maxRetires}
	}

	repoTest.T().Run("it automatically retry to send the query", func(t *testing.T) {
		repoTest.repo = &PsqlSyncedSecretRepository{
			psql:           repoTest.repo.psql,
			circuitBreaker: newCircuitBreaker(),
			retryOptFunc:   newBackoffStrategy,
		}
		repoTest.insertSecret("kv", "test", "prod")

		// simulate a connection failure
		repoTest.pgHelper.Stop(context.Background(), nil)
		go func() {
			time.Sleep(2 * time.Second)
			repoTest.pgHelper.Start(context.Background())
		}()

		result, err := repoTest.repo.GetSyncedSecret("kv", "test", "prod")

		assert.NoError(t, err, "Expected to retry and succeed after connection is restored")
		assert.NotNil(t, result, "Expected to retrieve secret after retry")
	})

	repoTest.T().Run("it returns error if circuit breaker is open", func(t *testing.T) {
		repoTest.repo = &PsqlSyncedSecretRepository{
			psql:           repoTest.repo.psql,
			circuitBreaker: newCircuitBreaker(),
			// default retry limits is 10 so the test could take a while to run, hence, we use a shorter timeout
			// and the retry limit to 1 to speed up the test
			retryOptFunc: retryOpsFunc,
		}

		// simulate a connection failure
		repoTest.pgHelper.Stop(context.Background(), nil)

		_, retryError := repoTest.repo.GetSyncedSecret("kv", "test", "prod")
		_, retryError2 := repoTest.repo.GetSyncedSecret("kv", "test", "prod")
		_, circutError := repoTest.repo.GetSyncedSecret("kv", "test", "prod")

		assert.ErrorIs(t, retryError, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		assert.ErrorIs(t, retryError2, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		assert.ErrorIs(t, circutError, ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")
	})

	repoTest.T().Run("circuit breaker affects GetSyncedSecrets", func(t *testing.T) {
		repoTest.repo = &PsqlSyncedSecretRepository{
			psql:           repoTest.repo.psql,
			circuitBreaker: newCircuitBreaker(),
			retryOptFunc:   retryOpsFunc,
		}
		repoTest.pgHelper.Stop(context.Background(), nil)

		_, retryError := repoTest.repo.GetSyncedSecret("kv", "test", "prod")
		_, retryError2 := repoTest.repo.GetSyncedSecret("kv", "test", "prod")
		_, circutError := repoTest.repo.GetSyncedSecrets()

		assert.ErrorIs(t, retryError, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		assert.ErrorIs(t, retryError2, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		assert.ErrorIs(t, circutError, ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")
	})

	repoTest.T().Run("circuit breaker closed after time passes", func(t *testing.T) {
		repoTest.pgHelper.Start(context.Background())
		repoTest.SetupTest()
		repoTest.insertRandomSecrets(5)

		// trigger the circuit breaker to open
		repoTest.repo.GetSyncedSecret("kv", "test", "prod")
		repoTest.repo.GetSyncedSecret("kv", "test", "prod")
		_, circutError := repoTest.repo.GetSyncedSecrets()
		assert.ErrorIs(t, circutError, ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")

		var err error
		var result []models.SyncedSecret
		assert.Eventually(t, func() bool {
			result, err = repoTest.repo.GetSyncedSecrets()
			return err == nil && result != nil
		}, 5*time.Second, 1*time.Second)

		assert.NoError(t, err, "Circuit breaker did not close after timeout")
		assert.NotNil(t, result, "Expected to retrieve secret after circuit breaker closed")
		assert.Len(t, result, 5, "Expected one secret to be returned after circuit breaker closed")
	})
}

// test helper functions
func (repoTest *SyncedSecretRepositoryTestSuite) insertSecret(backend, path, cluster string) {
	secret := models.SyncedSecret{
		SecretBackend:      backend,
		SecretPath:         path,
		SourceVersion:      1,
		DestinationCluster: cluster,
		DestinationVersion: 1,
		LastSyncAttempt:    time.Now().UTC(),
		LastSyncSuccess:    nil,
		Status:             "success",
		ErrorMessage:       nil,
	}
	repoTest.insertTestSecret(secret)
}

func (repoTest *SyncedSecretRepositoryTestSuite) insertRandomSecrets(count int) {
	for i := 0; i < count; i++ {
		repoTest.insertSecret("kv", fmt.Sprintf("test/path/%d", i+1), "prod")
	}
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
