package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"time"

	"vault-sync/internal/models"
	postgres "vault-sync/pkg/db"
	"vault-sync/pkg/log"

	"github.com/cenkalti/backoff/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog"
	"github.com/sony/gobreaker"
)

var (
	ErrSecretNotFound      = errors.New("synced secret not found")
	ErrDatabaseUnavailable = errors.New("database is unavailable")
	ErrDatabaseGeneric     = errors.New("database error occurred while processing request")
)

type PostgreSQLSyncedSecretRepository struct {
	psql           *postgres.PostgresDatastore
	circuitBreaker *gobreaker.CircuitBreaker
	retryOptFunc   func() []backoff.RetryOption
	logger         zerolog.Logger
}

type SyncedSecretResult interface {
	*models.SyncedSecret | []models.SyncedSecret
}

// NewPostgreSQLSyncedSecretRepository creates a new PostgreSQLSyncedSecretRepository instance
// with a configured circuit breaker and retry options.
//
// The circuit breaker is configured to trip after 30% failure rate with a maximum of 5 requests in half-open state.
// The retry options use an exponential backoff strategy with a maximum of 10 retries and a total elapsed time of 60 seconds.
func NewPostgreSQLSyncedSecretRepository(psql *postgres.PostgresDatastore) *PostgreSQLSyncedSecretRepository {
	gobreakerSettings := gobreaker.Settings{
		Name:        "synced_secret_db",
		MaxRequests: 5,                // Allow 5 test requests in half-open state
		Interval:    30 * time.Second, // Reset failure count every 30s
		Timeout:     15 * time.Second, // Stay open for 15s before trying again
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.3 // Trip at 30% failure rate
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			log.Logger.Info().
				Str("component", "postgres_synced_secret_repository").
				Str("event", "circuit_breaker_state_change").
				Str("circuit_breaker", name).
				Str("from_state", from.String()).
				Str("to_state", to.String()).
				Msg("Circuit breaker state changed")
		},
	}

	return &PostgreSQLSyncedSecretRepository{
		psql:           psql,
		circuitBreaker: gobreaker.NewCircuitBreaker(gobreakerSettings),
		retryOptFunc:   newBackoffStrategy,
		logger: log.Logger.With().
			Str("component", "postgres_synced_secret_repository").
			Logger(),
	}
}

func (repo *PostgreSQLSyncedSecretRepository) GetSyncedSecret(backend, path, destinationCluster string) (*models.SyncedSecret, error) {
	logger := repo.logger.With().
		Str("event", "get_synced_secret").
		Str("backend", backend).
		Str("path", path).
		Str("destinationCluster", destinationCluster).
		Logger()

	dbOperation := func() (any, error) {
		var secret models.SyncedSecret
		query := `SELECT * FROM synced_secrets WHERE secret_backend = $1 AND secret_path = $2 AND destination_cluster = $3`

		err := repo.psql.DB.Get(&secret, query, backend, path, destinationCluster)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				logger.Debug().Msg("Synced secret not found")
				return nil, nil
			}
			logger.Error().Err(err).Msg("error occurred while getting synced secret")
			return nil, fmt.Errorf("error occurred while getting synced secret: %w", err)
		}

		logger.Debug().Msg("Successfully retrieved synced secret")
		return &secret, nil
	}

	secret, err := executeOperationInCircuitBreaker[*models.SyncedSecret](repo, dbOperation)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

func (repo *PostgreSQLSyncedSecretRepository) GetSyncedSecrets() ([]models.SyncedSecret, error) {
	dbOperation := func() (any, error) {
		var secrets = make([]models.SyncedSecret, 0)
		query := `SELECT * FROM synced_secrets ORDER BY secret_backend, secret_path, destination_cluster`
		err := repo.psql.DB.Select(&secrets, query)
		if err != nil {
			repo.logger.Error().Err(err).
				Str("event", "get_synced_secrets").
				Msg("error occurred while getting synced secrets")
			return secrets, fmt.Errorf("error occurred while getting synced secrets: %w", err)
		}
		return secrets, nil
	}

	secrets, err := executeOperationInCircuitBreaker[[]models.SyncedSecret](repo, dbOperation)
	if err != nil {
		return []models.SyncedSecret{}, err
	}

	repo.logger.Debug().Int("count", len(secrets)).
		Str("event", "get_synced_secrets").
		Msg("Successfully retrieved synced secrets")
	return secrets, nil
}

func (repo *PostgreSQLSyncedSecretRepository) UpdateSyncedSecretStatus(secret models.SyncedSecret) error {
	logger := repo.logger.With().
		Str("event", "update_synced_secret_status").
		Str("backend", secret.SecretBackend).
		Str("path", secret.SecretPath).
		Str("destinationCluster", secret.DestinationCluster).
		Logger()

	dbOperation := func() (any, error) {
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
            ON CONFLICT (secret_backend, secret_path, destination_cluster)
            DO UPDATE SET
                source_version = EXCLUDED.source_version,
                destination_version = EXCLUDED.destination_version,
                last_sync_attempt = EXCLUDED.last_sync_attempt,
                last_sync_success = EXCLUDED.last_sync_success,
                status = EXCLUDED.status,
                error_message = EXCLUDED.error_message
        `

		result, err := repo.psql.DB.NamedExec(query, secret)
		if err != nil {
			logger.Error().Err(err).Msg("error occurred while updating synced secret status")
			return nil, fmt.Errorf("error occurred while updating synced secret status: %w", err)
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			logger.Error().Err(err).Msg("error occurred while checking rows affected")
			return nil, fmt.Errorf("error occurred while checking rows affected: %w", err)
		}

		logger.Debug().Int64("rows_affected", rowsAffected).Msg("Successfully updated synced secret status")
		return &secret, nil
	}

	_, err := executeOperationInCircuitBreaker[*models.SyncedSecret](repo, dbOperation)
	return err
}

func (repo *PostgreSQLSyncedSecretRepository) Close() error {
	if repo.psql != nil {
		return repo.psql.Close()
	}
	return nil
}

// executeOperationInCircuitBreaker executes the provided database operation within a circuit breaker context.
// It retries the operation using an exponential backoff strategy if it fails.
func executeOperationInCircuitBreaker[T SyncedSecretResult](repo *PostgreSQLSyncedSecretRepository, operation func() (any, error)) (T, error) {
	var opsResult T

	result, err := repo.circuitBreaker.Execute(func() (any, error) {
		return backoff.Retry(context.Background(), operation, repo.retryOptFunc()...)
	})

	if err := repo.handleCircuitBreakerError(err); err != nil {
		return opsResult, err
	}

	if result == nil || reflect.ValueOf(result).IsNil() {
		return opsResult, ErrSecretNotFound
	}

	typedResult, ok := result.(T)
	if !ok {
		return opsResult, fmt.Errorf("unexpected result type from circuit breaker")
	}

	return typedResult, nil
}

func (repo *PostgreSQLSyncedSecretRepository) handleCircuitBreakerError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, gobreaker.ErrOpenState), errors.Is(err, gobreaker.ErrTooManyRequests):
		return fmt.Errorf("%w: circuit breaker is open", ErrDatabaseUnavailable)
	default:
		if err.Error() == "backoff: retry limit exceeded" {
			return fmt.Errorf("%w: retry limit exceeded", ErrDatabaseUnavailable)
		}
		return fmt.Errorf("%w: %w", ErrDatabaseGeneric, err)
	}
}

// newBackoffStrategy creates a new backoff strategy for retrying database operations.
// It uses an exponential backoff strategy with a maximum of 10 retries and a total elapsed time of 60 seconds.
func newBackoffStrategy() []backoff.RetryOption {
	strategyOpts := &backoff.ExponentialBackOff{
		InitialInterval:     250 * time.Millisecond,
		RandomizationFactor: 0.5,              // [250 - 50% = 125, 250 + 50% = 375]
		Multiplier:          1.5,              // 250 * 1.5 = 375 [250, 375, 562.5, 843.75, 1,265.625, ...]
		MaxInterval:         60 * time.Second, // Maximum interval between retries
	}
	maxElapsedTime := backoff.WithMaxElapsedTime(60 * time.Second) // Maximum total time for retries
	maxRetires := backoff.WithMaxTries(10)                         // Maximum number of retries

	return []backoff.RetryOption{
		backoff.WithBackOff(strategyOpts),
		maxElapsedTime,
		maxRetires,
	}
}
