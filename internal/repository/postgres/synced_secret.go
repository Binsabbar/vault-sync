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

type PsqlSyncedSecretRepository struct {
	psql           *postgres.PostgresDatastore
	circuitBreaker *gobreaker.CircuitBreaker
	retryOptFunc   func() []backoff.RetryOption
}

type SyncedSecretResult interface {
	*models.SyncedSecret | []models.SyncedSecret
}

func NewPsqlSyncedSecretRepository(psql *postgres.PostgresDatastore) *PsqlSyncedSecretRepository {
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
				Str("circuit_breaker", name).
				Str("from_state", from.String()).
				Str("to_state", to.String()).
				Msg("Circuit breaker state changed")
		},
	}

	return &PsqlSyncedSecretRepository{
		psql:           psql,
		circuitBreaker: gobreaker.NewCircuitBreaker(gobreakerSettings),
		retryOptFunc:   newBackoffStrategy,
	}
}

func (repo *PsqlSyncedSecretRepository) GetSyncedSecret(backend, path, destinationCluster string) (*models.SyncedSecret, error) {
	dbOperation := func() (any, error) {
		var secret models.SyncedSecret
		query := `SELECT * FROM synced_secrets WHERE secret_backend = $1 AND secret_path = $2 AND destination_cluster = $3`

		err := repo.psql.DB.Get(&secret, query, backend, path, destinationCluster)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				repo.decorateLog(log.Logger.Debug, backend, path, destinationCluster).Msg("Synced secret not found")
				return nil, nil
			}
			repo.decorateLog(log.Logger.Error, backend, path, destinationCluster).Err(err).Msg("error occurred while getting synced secret")
			return nil, fmt.Errorf("error occurred while getting synced secret: %w", err)
		}
		repo.decorateLog(log.Logger.Debug, backend, path, destinationCluster).Msg("Successfully retrieved synced secret")
		return &secret, nil
	}

	secret, err := executeOperationInCircuitBreaker[*models.SyncedSecret](repo, dbOperation)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

func (repo *PsqlSyncedSecretRepository) GetSyncedSecrets() ([]models.SyncedSecret, error) {
	dbOperation := func() (any, error) {
		var secrets = make([]models.SyncedSecret, 0)
		query := `SELECT * FROM synced_secrets ORDER BY secret_backend, secret_path, destination_cluster`
		err := repo.psql.DB.Select(&secrets, query)
		if err != nil {
			log.Logger.Error().Err(err).Msg("error occurred while getting synced secrets")
			return secrets, fmt.Errorf("error occurred while getting synced secrets: %w", err)
		}
		return secrets, nil
	}

	secrets, err := executeOperationInCircuitBreaker[[]models.SyncedSecret](repo, dbOperation)
	if err != nil {
		return []models.SyncedSecret{}, err
	}

	log.Logger.Debug().Int("count", len(secrets)).Msg("Successfully retrieved synced secrets")
	return secrets, nil
}

func executeOperationInCircuitBreaker[T SyncedSecretResult](repo *PsqlSyncedSecretRepository, operation func() (any, error)) (T, error) {
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

func (repo *PsqlSyncedSecretRepository) handleCircuitBreakerError(err error) error {
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

func (repo *PsqlSyncedSecretRepository) decorateLog(eventFactory func() *zerolog.Event, backend, path, destinationCluster string) *zerolog.Event {
	return eventFactory().Str("backend", backend).Str("path", path).Str("destinationCluster", destinationCluster)
}

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
