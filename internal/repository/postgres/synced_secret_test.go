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
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"vault-sync/internal/models"
	"vault-sync/pkg/db"
	"vault-sync/pkg/db/migrations"
	"vault-sync/testutil"
)

type SyncedSecretRepositoryTestSuite struct {
	suite.Suite
	ctx      context.Context
	pgHelper *testutil.PostgresHelper
	db       *db.PostgresDatastore
}

func TestSyncedSecretRepositorySuite(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		t.Skip("Skipping integration tests")
	}
	suite.Run(t, new(SyncedSecretRepositoryTestSuite))
}

func (suite *SyncedSecretRepositoryTestSuite) SetupSuite() {
	var err error
	suite.pgHelper, err = testutil.NewPostgresContainer(suite.T(), context.Background())
	suite.NoError(err, "Failed to create Postgres test container")

	suite.db, err = db.NewPostgresDatastore(suite.pgHelper.Config, migrations.NewPostgresMigration())
	suite.NoError(err, "Failed to create PostgreSQLSyncedSecretRepository")

	suite.ctx = context.Background()
}

func (suite *SyncedSecretRepositoryTestSuite) SetupTest() {
	suite.pgHelper.Start(context.Background())
	suite.pgHelper.ExecutePsqlCommand(context.Background(), "TRUNCATE TABLE synced_secrets")
}

func (suite *SyncedSecretRepositoryTestSuite) SetupSubTest() {
	suite.pgHelper.Start(context.Background())
	suite.pgHelper.ExecutePsqlCommand(context.Background(), "TRUNCATE TABLE synced_secrets")
}

func (suite *SyncedSecretRepositoryTestSuite) TearDownSuite() {
	if suite.pgHelper != nil {
		err := suite.pgHelper.Terminate(suite.ctx)
		if err != nil {
			log.Printf("Error terminating container: %v", err)
		}
	}
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

func (suite *SyncedSecretRepositoryTestSuite) TestGetSyncedSecret() {
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
		suite.Run(tc.name, func() {
			suite.insertTestSecret(tc.secretToInsert)
			repo := NewPostgreSQLSyncedSecretRepository(suite.db)

			result, err := repo.GetSyncedSecret(tc.backend, tc.path, tc.destinationCluster)

			if tc.expectedErr != nil {
				suite.ErrorIs(err, tc.expectedErr, "Expected error does not match")
				suite.Nil(result)
			} else {
				suite.NoError(err)
				suite.NotNil(result)

				suite.Equal(tc.expectedSecret.SecretBackend, result.SecretBackend)
				suite.Equal(tc.expectedSecret.SecretPath, result.SecretPath)
				suite.Equal(tc.expectedSecret.SourceVersion, result.SourceVersion)
				suite.Equal(tc.expectedSecret.DestinationCluster, result.DestinationCluster)
				suite.Equal(tc.expectedSecret.DestinationVersion, result.DestinationVersion)
				suite.Equal(tc.expectedSecret.Status, result.Status)
				suite.WithinDuration(tc.expectedSecret.LastSyncAttempt, result.LastSyncAttempt, time.Second)
			}
		})
	}
}

func (suite *SyncedSecretRepositoryTestSuite) TestGetSyncedSecrets() {
	suite.T().Run("returns all synced secrets", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Millisecond)
		successTime := now.Add(-1 * time.Minute)

		secrets := []*models.SyncedSecret{
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
				ErrorMessage:       nil,
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
			suite.insertTestSecret(secret)
		}

		repo := NewPostgreSQLSyncedSecretRepository(suite.db)

		result, err := repo.GetSyncedSecrets()

		suite.NoError(err)
		suite.NotNil(result)
		suite.Len(result, len(secrets))

		suite.Equal("aws", result[0].SecretBackend)
		suite.Equal("kv", result[1].SecretBackend)
		suite.Equal("zkv", result[2].SecretBackend)
	})

	suite.Run("returns empty slice for no secrets without error", func() {
		repo := NewPostgreSQLSyncedSecretRepository(suite.db)

		result, err := repo.GetSyncedSecrets()

		suite.NoError(err)
		suite.NotNil(result)
		suite.Len(result, 0)
	})
}

type syncedSecretUpdateStatusTestCase struct {
	name               string
	secretToInsert     *models.SyncedSecret
	secretToUpdate     models.SyncedSecret
	expectedErr        error
	shouldUpdateFields bool
}

func (suite *SyncedSecretRepositoryTestSuite) TestUpdateSyncedSecretStatus() {
	now := time.Now().UTC().Truncate(time.Millisecond)
	successTime := now.Add(-1 * time.Minute)
	errorMsg := "sync failed"

	existingSecret := &models.SyncedSecret{
		SecretBackend:      "kv",
		SecretPath:         "test/path",
		SourceVersion:      1,
		DestinationCluster: "prod",
		DestinationVersion: 1,
		LastSyncAttempt:    now.Add(-2 * time.Minute),
		LastSyncSuccess:    &successTime,
		Status:             "success",
		ErrorMessage:       nil,
	}

	testCases := []syncedSecretUpdateStatusTestCase{
		{
			name:           "insert new secret successfully",
			secretToInsert: nil,
			secretToUpdate: models.SyncedSecret{
				SecretBackend:      "kv",
				SecretPath:         "new/path",
				SourceVersion:      1,
				DestinationCluster: "prod",
				DestinationVersion: 1,
				LastSyncAttempt:    now,
				LastSyncSuccess:    &successTime,
				Status:             "success",
				ErrorMessage:       nil,
			},
			expectedErr:        nil,
			shouldUpdateFields: true,
		},
		{
			name:           "update existing secret status to error",
			secretToInsert: existingSecret,
			secretToUpdate: models.SyncedSecret{
				SecretBackend:      "kv",
				SecretPath:         "test/path",
				SourceVersion:      2,
				DestinationCluster: "prod",
				DestinationVersion: 1,
				LastSyncAttempt:    now,
				LastSyncSuccess:    nil,
				Status:             "error",
				ErrorMessage:       &errorMsg,
			},
			expectedErr:        nil,
			shouldUpdateFields: true,
		},
		{
			name:           "update existing secret status to success",
			secretToInsert: existingSecret,
			secretToUpdate: models.SyncedSecret{
				SecretBackend:      "kv",
				SecretPath:         "test/path",
				SourceVersion:      3,
				DestinationCluster: "prod",
				DestinationVersion: 3,
				LastSyncAttempt:    now,
				LastSyncSuccess:    &successTime,
				Status:             "success",
				ErrorMessage:       nil,
			},
			expectedErr:        nil,
			shouldUpdateFields: true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			if tc.secretToInsert != nil {
				suite.insertTestSecret(tc.secretToInsert)
			}
			repo := NewPostgreSQLSyncedSecretRepository(suite.db)

			err := repo.UpdateSyncedSecretStatus(tc.secretToUpdate)

			if tc.expectedErr != nil {
				suite.ErrorIs(err, tc.expectedErr, "Expected error does not match")
			} else {
				suite.NoError(err, "Expected no error")

				if tc.shouldUpdateFields {
					result, err := repo.GetSyncedSecret(
						tc.secretToUpdate.SecretBackend,
						tc.secretToUpdate.SecretPath,
						tc.secretToUpdate.DestinationCluster,
					)
					suite.NoError(err, "Failed to retrieve updated secret")

					suite.Equal(tc.secretToUpdate.SecretBackend, result.SecretBackend)
					suite.Equal(tc.secretToUpdate.SecretPath, result.SecretPath)
					suite.Equal(tc.secretToUpdate.SourceVersion, result.SourceVersion)
					suite.Equal(tc.secretToUpdate.DestinationCluster, result.DestinationCluster)
					suite.Equal(tc.secretToUpdate.DestinationVersion, result.DestinationVersion)
					suite.Equal(tc.secretToUpdate.Status, result.Status)
					suite.WithinDuration(tc.secretToUpdate.LastSyncAttempt, result.LastSyncAttempt, time.Second)

					if tc.secretToUpdate.LastSyncSuccess != nil {
						suite.WithinDuration(*tc.secretToUpdate.LastSyncSuccess, *result.LastSyncSuccess, time.Second)
					}
					if tc.secretToUpdate.ErrorMessage != nil {
						suite.Equal(*tc.secretToUpdate.ErrorMessage, *result.ErrorMessage)
					}
				}
			}
		})
	}
}

func (suite *SyncedSecretRepositoryTestSuite) TestFailureWithCircuitBreakerAndRetry() {
	newMockCircuitBreaker := func() *gobreaker.CircuitBreaker {
		return gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        "test_breaker",
			MaxRequests: 2,
			Interval:    1 * time.Second,
			Timeout:     500 * time.Millisecond,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.TotalFailures >= 2
			},
		})
	}

	// default retry limits is 10 so the test could take a while to run, hence, we use a shorter timeout
	// and the retry limit to 1 to speed up the test
	retryOpsMockFunc := func() []backoff.RetryOption {
		strategyOpts := &backoff.ConstantBackOff{Interval: 100 * time.Millisecond}
		return []backoff.RetryOption{
			backoff.WithBackOff(strategyOpts),
			backoff.WithMaxTries(1),
		}
	}

	secret := models.SyncedSecret{
		SecretBackend:      "kv",
		SecretPath:         "test/path",
		SourceVersion:      1,
		DestinationCluster: "prod",
		DestinationVersion: 1,
		LastSyncAttempt:    time.Now().UTC(),
		LastSyncSuccess:    nil,
		Status:             "success",
		ErrorMessage:       nil,
	}

	suite.Run("it automatically retry to send the query", func() {
		repo := &PostgreSQLSyncedSecretRepository{
			psql:           suite.db,
			circuitBreaker: newMockCircuitBreaker(),
			retryOptFunc:   newBackoffStrategy,
		}
		suite.insertSecret("kv", "test", "prod")

		// simulate a connection failure
		suite.pgHelper.Stop(context.Background(), nil)
		go func() {
			time.Sleep(1 * time.Second)
			suite.pgHelper.Start(context.Background())
		}()

		result, err := repo.GetSyncedSecret("kv", "test", "prod")

		suite.NoError(err, "Expected to retry and succeed after connection is restored")
		suite.NotNil(result, "Expected to retrieve secret after retry")
	})

	suite.Run("it returns error if circuit breaker is open", func() {
		repo := &PostgreSQLSyncedSecretRepository{
			psql:           suite.db,
			circuitBreaker: newMockCircuitBreaker(),
			retryOptFunc:   retryOpsMockFunc,
		}

		// simulate a connection failure
		suite.pgHelper.Stop(context.Background(), nil)

		_, retryError := repo.GetSyncedSecret("kv", "test", "prod")
		_, retryError2 := repo.GetSyncedSecret("kv", "test", "prod")
		_, circutError := repo.GetSyncedSecret("kv", "test", "prod")

		suite.ErrorIs(retryError, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		suite.ErrorIs(retryError2, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		suite.ErrorIs(circutError, ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")
	})

	suite.Run("circuit breaker affects GetSyncedSecrets", func() {
		repo := &PostgreSQLSyncedSecretRepository{
			psql:           suite.db,
			circuitBreaker: newMockCircuitBreaker(),
			retryOptFunc:   retryOpsMockFunc,
		}
		suite.pgHelper.Stop(context.Background(), nil)

		_, retryError := repo.GetSyncedSecret("kv", "test", "prod")
		_, retryError2 := repo.GetSyncedSecret("kv", "test", "prod")
		_, circutError := repo.GetSyncedSecrets()

		suite.ErrorIs(retryError, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		suite.ErrorIs(retryError2, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		suite.ErrorIs(circutError, ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")
	})

	suite.Run("circuit breaker closed after time passes", func() {
		suite.insertRandomSecrets(5)
		repo := &PostgreSQLSyncedSecretRepository{
			psql:           suite.db,
			circuitBreaker: newMockCircuitBreaker(),
			retryOptFunc:   retryOpsMockFunc,
		}

		// trigger the circuit breaker to open
		suite.pgHelper.Stop(context.Background(), nil)
		_, retryError := repo.GetSyncedSecret("kv", "test", "prod")
		_, retryError2 := repo.GetSyncedSecret("kv", "test", "prod")
		suite.ErrorIs(retryError, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		suite.ErrorIs(retryError2, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")

		suite.pgHelper.Start(context.Background())

		_, circuitError := repo.GetSyncedSecrets()
		suite.ErrorIs(circuitError, ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")

		var err error
		var result []models.SyncedSecret
		suite.Eventually(func() bool {
			result, err = repo.GetSyncedSecrets()
			return err == nil && result != nil
		}, 3*time.Second, 1*time.Second)

		suite.NoError(err, "Circuit breaker did not close after timeout")
		suite.NotNil(result, "Expected to retrieve secret after circuit breaker closed")
		suite.Len(result, 5, "Expected one secret to be returned after circuit breaker closed")
	})

	suite.Run("circuit breaker opens on update failures", func() {
		repo := &PostgreSQLSyncedSecretRepository{
			psql:           suite.db,
			circuitBreaker: newMockCircuitBreaker(),
			retryOptFunc:   retryOpsMockFunc,
		}

		// simulate a connection failure
		suite.pgHelper.Stop(context.Background(), nil)

		retryError1 := repo.UpdateSyncedSecretStatus(secret)
		retryError2 := repo.UpdateSyncedSecretStatus(secret)
		circuitError := repo.UpdateSyncedSecretStatus(secret)

		suite.ErrorIs(retryError1, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		suite.ErrorIs(retryError2, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		suite.ErrorIs(circuitError, ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")
	})

	suite.Run("circuit breaker recovers after connection restored on update", func() {
		repo := &PostgreSQLSyncedSecretRepository{
			psql:           suite.db,
			circuitBreaker: newMockCircuitBreaker(),
			retryOptFunc:   retryOpsMockFunc,
		}

		// simulate a connection failure
		suite.pgHelper.Stop(context.Background(), nil)
		retryError1 := repo.UpdateSyncedSecretStatus(secret)
		retryError2 := repo.UpdateSyncedSecretStatus(secret)
		circuitError := repo.UpdateSyncedSecretStatus(secret)
		suite.ErrorIs(retryError1, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		suite.ErrorIs(retryError2, ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		suite.ErrorIs(circuitError, ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")

		// restore the connection
		suite.pgHelper.Start(context.Background())

		var err error
		suite.Eventually(func() bool {
			err = repo.UpdateSyncedSecretStatus(secret)
			return err == nil
		}, 3*time.Second, 1*time.Second)

		suite.NoError(err, "Circuit breaker did not close after timeout")

		result, err := repo.GetSyncedSecret(secret.SecretBackend, secret.SecretPath, secret.DestinationCluster)

		suite.NoError(err, "Failed to retrieve updated secret")
		suite.Equal(secret.SecretBackend, result.SecretBackend, "Expected secret backend to match")
		suite.Equal(secret.SecretPath, result.SecretPath, "Expected secret path to match")
		suite.Equal(secret.DestinationCluster, result.DestinationCluster, "Expected destination cluster to match")
		suite.Equal(secret.DestinationVersion, result.DestinationVersion, "Expected destination version to match")
		suite.Equal(secret.SourceVersion, result.SourceVersion, "Expected source version to match")
		suite.Equal(secret.Status, result.Status, "Expected status to match")
		suite.WithinDuration(secret.LastSyncAttempt, result.LastSyncAttempt, time.Second, "Expected last sync attempt to match")
	})
}

// test helper functions
func (suite *SyncedSecretRepositoryTestSuite) insertSecret(backend, path, cluster string) {
	secret := &models.SyncedSecret{
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
	suite.insertTestSecret(secret)
}

func (suite *SyncedSecretRepositoryTestSuite) insertRandomSecrets(count int) {
	for i := 0; i < count; i++ {
		suite.insertSecret("kv", fmt.Sprintf("test/path/%d", i+1), "prod")
	}
}

func (suite *SyncedSecretRepositoryTestSuite) insertTestSecret(secret *models.SyncedSecret) {
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

	_, err := suite.db.DB.NamedExec(query, secret)

	require.NoError(suite.T(), err, "Failed to insert test secret")
}
