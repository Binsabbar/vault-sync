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
	"vault-sync/internal/repository"
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
			expectedErr:        repository.ErrSecretNotFound,
			expectedSecret:     nil,
		},
		{
			name:               "return error if delete with empty backend",
			secretToInsert:     nil,
			backend:            "",
			path:               "does/not/exist",
			destinationCluster: "prod",
			expectedErr:        repository.ErrInvalidQueryParameters,
			expectedSecret:     nil,
		},
		{
			name:               "return error if delete with empty path",
			secretToInsert:     nil,
			backend:            "kv",
			path:               "",
			destinationCluster: "prod",
			expectedErr:        repository.ErrInvalidQueryParameters,
			expectedSecret:     nil,
		},
		{
			name:               "return error if delete with empty destination cluster",
			secretToInsert:     nil,
			backend:            "kv",
			path:               "does/not/exist",
			destinationCluster: "",
			expectedErr:        repository.ErrInvalidQueryParameters,
			expectedSecret:     nil,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			if tc.secretToInsert != nil {
				suite.insertTestSecret(tc.secretToInsert)
			}
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

			err := repo.UpdateSyncedSecretStatus(&tc.secretToUpdate)

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

type syncedSecretDeleteTestCase struct {
	name                string
	secretToInsert      *models.SyncedSecret
	backend             string
	path                string
	destinationCluster  string
	expectedErr         error
	shouldVerifyDeleted bool
}

func (suite *SyncedSecretRepositoryTestSuite) TestDeleteSyncedSecret() {
	now := time.Now().UTC().Truncate(time.Millisecond)
	successTime := now.Add(-1 * time.Minute)
	existingSecret := &models.SyncedSecret{
		SecretBackend:      "kv",
		SecretPath:         "test/path",
		SourceVersion:      1,
		DestinationCluster: "prod",
		DestinationVersion: 1,
		LastSyncAttempt:    now,
		LastSyncSuccess:    &successTime,
		Status:             "success",
		ErrorMessage:       nil,
	}

	testCases := []syncedSecretDeleteTestCase{
		{
			name:                "delete existing synced secret",
			secretToInsert:      existingSecret,
			backend:             existingSecret.SecretBackend,
			path:                existingSecret.SecretPath,
			destinationCluster:  existingSecret.DestinationCluster,
			expectedErr:         nil,
			shouldVerifyDeleted: true,
		},
		{
			name:                "delete non-existent secret without error",
			secretToInsert:      nil,
			backend:             "kv",
			path:                "does/not/exist",
			destinationCluster:  "prod",
			expectedErr:         nil,
			shouldVerifyDeleted: false,
		},
		{
			name:                "return error if delete with empty backend",
			secretToInsert:      nil,
			backend:             "",
			path:                "test/path",
			destinationCluster:  "prod",
			expectedErr:         repository.ErrInvalidQueryParameters,
			shouldVerifyDeleted: false,
		},
		{
			name:                "return error if delete with empty path",
			secretToInsert:      nil,
			backend:             "kv",
			path:                "",
			destinationCluster:  "prod",
			expectedErr:         repository.ErrInvalidQueryParameters,
			shouldVerifyDeleted: false,
		},
		{
			name:                "return error if delete with empty destination cluster",
			secretToInsert:      nil,
			backend:             "kv",
			path:                "test/path",
			destinationCluster:  "",
			expectedErr:         repository.ErrInvalidQueryParameters,
			shouldVerifyDeleted: false,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			if tc.secretToInsert != nil {
				suite.insertTestSecret(tc.secretToInsert)
			}

			repo := NewPostgreSQLSyncedSecretRepository(suite.db)

			err := repo.DeleteSyncedSecret(tc.backend, tc.path, tc.destinationCluster)

			if tc.expectedErr != nil {
				suite.ErrorIs(err, tc.expectedErr, "Expected error does not match")
			} else {
				suite.NoError(err, "Expected no error")
				if tc.shouldVerifyDeleted {
					result, err := repo.GetSyncedSecret(tc.backend, tc.path, tc.destinationCluster)
					suite.Error(err, "synced secret not found")
					suite.Nil(result, "Expected secret to be deleted")
				}
			}
		})
	}

}

// in this test, no need to check the returned values since this was already tested above individually,
// this test focuses on the behavior of the circuit breaker and retry mechanism
func (suite *SyncedSecretRepositoryTestSuite) TestFailureWithCircuitBreakerAndRetry() {

	type testCases struct {
		name              string
		prepareTestFunc   func(repo *PostgreSQLSyncedSecretRepository)
		functionToExecute func(repo *PostgreSQLSyncedSecretRepository) (any, error)
	}

	secret := &models.SyncedSecret{
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
	tests := []testCases{
		{
			name: "GetSyncedSecret",
			prepareTestFunc: func(repo *PostgreSQLSyncedSecretRepository) {
				suite.insertTestSecret(secret)
			},
			functionToExecute: func(repo *PostgreSQLSyncedSecretRepository) (any, error) {
				return repo.GetSyncedSecret(secret.SecretBackend, secret.SecretPath, secret.DestinationCluster)
			},
		},
		{
			name:            "GetSyncedSecrets",
			prepareTestFunc: func(repo *PostgreSQLSyncedSecretRepository) {},
			functionToExecute: func(repo *PostgreSQLSyncedSecretRepository) (any, error) {
				return repo.GetSyncedSecrets()
			},
		},
		{
			name: "UpdateSyncedSecretStatus",
			prepareTestFunc: func(repo *PostgreSQLSyncedSecretRepository) {
				suite.insertTestSecret(secret)
			},
			functionToExecute: func(repo *PostgreSQLSyncedSecretRepository) (any, error) {
				return nil, repo.UpdateSyncedSecretStatus(secret)
			},
		},
		{
			name: "DeleteSyncedSecret",
			prepareTestFunc: func(repo *PostgreSQLSyncedSecretRepository) {
				suite.insertTestSecret(secret)
			},
			functionToExecute: func(repo *PostgreSQLSyncedSecretRepository) (any, error) {
				return nil, repo.DeleteSyncedSecret(secret.SecretBackend, secret.SecretPath, secret.DestinationCluster)
			},
		},
	}

	suite.Run("automatically retry to execute the operation", func() {
		for _, test := range tests {
			suite.Run(test.name, func() {
				repo := &PostgreSQLSyncedSecretRepository{
					psql:           suite.db,
					circuitBreaker: createFastFailCircuitBreaker(),
					retryOptFunc:   newBackoffStrategy,
				}

				test.prepareTestFunc(repo)

				// simulate a connection failure
				suite.pgHelper.Stop(suite.ctx, nil)
				go func() {
					time.Sleep(500 * time.Millisecond)
					suite.pgHelper.Start(suite.ctx)
				}()

				// execute the function that should retry
				_, retryError1 := test.functionToExecute(repo)

				suite.NoError(retryError1, "Expected no error on first retry")
			})
		}
	})

	suite.Run("circuit breaker opens on operation failures", func() {
		for _, test := range tests {
			suite.Run(test.name, func() {
				repo := &PostgreSQLSyncedSecretRepository{
					psql:           suite.db,
					circuitBreaker: createFastFailCircuitBreaker(),
					retryOptFunc:   createConstantBackoffRetryFunc(100*time.Millisecond, 1),
				}
				test.prepareTestFunc(repo)

				// simulate a connection failure
				suite.pgHelper.Stop(context.Background(), nil)

				// trigger the circuit breaker to open
				_, retryError1 := test.functionToExecute(repo)
				_, retryError2 := test.functionToExecute(repo)
				_, circuitError := test.functionToExecute(repo)

				// check that the errors are as expected
				suite.ErrorIs(retryError1, repository.ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
				suite.ErrorIs(retryError2, repository.ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
				suite.ErrorIs(circuitError, repository.ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")
			})
		}
	})

	suite.Run("circuit breaker closes after time passes", func() {
		for _, test := range tests {
			suite.Run(test.name, func() {
				repo := &PostgreSQLSyncedSecretRepository{
					psql:           suite.db,
					circuitBreaker: createFastFailCircuitBreaker(),
					retryOptFunc:   createConstantBackoffRetryFunc(100*time.Millisecond, 1),
				}
				test.prepareTestFunc(repo)

				// simulate a connection failure
				suite.pgHelper.Stop(context.Background(), nil)

				// trigger the circuit breaker to open
				_, retryError := test.functionToExecute(repo)
				_, retryError2 := test.functionToExecute(repo)
				_, circuitError := test.functionToExecute(repo)

				suite.ErrorIs(retryError, repository.ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
				suite.ErrorIs(retryError2, repository.ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
				suite.ErrorIs(circuitError, repository.ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")

				suite.pgHelper.Start(context.Background())

				var err error
				// wait for the circuit breaker to close
				suite.Eventually(func() bool {
					_, err = test.functionToExecute(repo)
					return err == nil
				}, 3*time.Second, 1*time.Second)

				suite.NoError(err, "Expected no error after connection is restored")
			})
		}
	})

	suite.Run("circuit breaker affects calls to other functions", func() {
		repo := &PostgreSQLSyncedSecretRepository{
			psql:           suite.db,
			circuitBreaker: createFastFailCircuitBreaker(),
			retryOptFunc:   createConstantBackoffRetryFunc(100*time.Millisecond, 1),
		}
		suite.pgHelper.Stop(context.Background(), nil)

		_, retryError := repo.GetSyncedSecret("kv", "test", "prod")
		_, retryError2 := repo.GetSyncedSecret("kv", "test", "prod")
		suite.ErrorIs(retryError, repository.ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")
		suite.ErrorIs(retryError2, repository.ErrDatabaseGeneric, "Expected ErrDatabaseGeneric error")

		_, circuitError := repo.GetSyncedSecrets()
		suite.ErrorIs(circuitError, repository.ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")

		circuitError = repo.UpdateSyncedSecretStatus(secret)
		suite.ErrorIs(circuitError, repository.ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")

		circuitError = repo.DeleteSyncedSecret("kv", "test", "prod")
		suite.ErrorIs(circuitError, repository.ErrDatabaseUnavailable, "Expected ErrDatabaseUnavailable error")
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

func createFastFailCircuitBreaker() *gobreaker.CircuitBreaker {
	// Create a circuit breaker that opens after 2 failures and closes after 1 second
	// with a timeout of 500 milliseconds for the operation.
	// This is a fast-failing circuit breaker for testing purposes.
	// It will trip quickly to simulate failure scenarios in tests.
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

func createConstantBackoffRetryFunc(interval time.Duration, maxRetries uint) func() []backoff.RetryOption {
	// default retry limits is 10 so the test could take a while to run, hence, we use a shorter timeout
	// and the retry limit to 1 to speed up the test
	return func() []backoff.RetryOption {
		strategyOpts := &backoff.ConstantBackOff{Interval: interval}
		return []backoff.RetryOption{
			backoff.WithBackOff(strategyOpts),
			backoff.WithMaxTries(maxRetries),
		}
	}
}
