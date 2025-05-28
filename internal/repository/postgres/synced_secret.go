package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

func NewPsqlSyncedSecretRepository(psql *postgres.PostgresDatastore) *PsqlSyncedSecretRepository {
	gobreakerSettings := newGobreakerSettings()
	return &PsqlSyncedSecretRepository{
		psql:           psql,
		circuitBreaker: gobreaker.NewCircuitBreaker(gobreakerSettings),
		retryOptFunc:   newBackoffStrategy,
	}
}

func (r *PsqlSyncedSecretRepository) GetSyncedSecret(backend, path, destinationCluster string) (*models.SyncedSecret, error) {
	dbOperation := func() (any, error) {
		return r.getSyncedSecretQuery(backend, path, destinationCluster)
	}

	result, err := r.circuitBreaker.Execute(func() (any, error) {
		return backoff.Retry(context.Background(), dbOperation, r.retryOptFunc()...)
	})

	switch {
	case errors.Is(err, gobreaker.ErrOpenState), errors.Is(err, gobreaker.ErrTooManyRequests):
		return nil, fmt.Errorf("%w: circuit breaker is open", ErrDatabaseUnavailable)
	case err != nil:
		return nil, fmt.Errorf("%w [%w]: generic database error", ErrDatabaseGeneric, err)
	}

	secret, ok := result.(*models.SyncedSecret)
	if !ok {
		return nil, fmt.Errorf("unexpected result type from circuit breaker")
	} else if secret == nil {
		return nil, ErrSecretNotFound
	}

	r.decorateLog(log.Logger.Debug, backend, path, destinationCluster).Msg("Successfully retrieved synced secret")
	return secret, nil
}

func (r *PsqlSyncedSecretRepository) GetSyncedSecrets() ([]models.SyncedSecret, error) {
	dbOperation := r.getSyncedSecretsQuery
	result, err := r.circuitBreaker.Execute(func() (any, error) {
		return backoff.Retry(context.Background(), dbOperation, r.retryOptFunc()...)
	})

	switch {
	case errors.Is(err, gobreaker.ErrOpenState), errors.Is(err, gobreaker.ErrTooManyRequests):
		return nil, fmt.Errorf("%w: circuit breaker is open", ErrDatabaseUnavailable)
	case err != nil:
		return nil, fmt.Errorf("%w [%w]: generic database error", ErrDatabaseGeneric, err)
	}

	secrets, ok := result.([]models.SyncedSecret)
	if !ok {
		return nil, fmt.Errorf("unexpected result type from circuit breaker")
	} else if len(secrets) == 0 {
		return secrets, fmt.Errorf("no synced secrets found: %w", ErrSecretNotFound)
	}
	return secrets, nil
}

func (repo *PsqlSyncedSecretRepository) getSyncedSecretQuery(backend, path, destinationCluster string) (*models.SyncedSecret, error) {
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

func (repo *PsqlSyncedSecretRepository) getSyncedSecretsQuery() ([]models.SyncedSecret, error) {
	var secrets = make([]models.SyncedSecret, 0)
	query := `SELECT * FROM synced_secrets ORDER BY secret_backend, secret_path, destination_cluster`
	err := repo.psql.DB.Select(&secrets, query)
	if err != nil {
		log.Logger.Error().Err(err).Msg("error occurred while getting synced secrets")
		return secrets, fmt.Errorf("error occurred while getting synced secrets: %w", err)
	}
	return secrets, nil
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

func newGobreakerSettings() gobreaker.Settings {
	return gobreaker.Settings{
		Name:        "synced_secret_db",
		MaxRequests: 20,
		Interval:    30 * time.Second,
		Timeout:     15 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 20 && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			log.Logger.Info().
				Str("circuit_breaker", name).
				Str("from_state", from.String()).
				Str("to_state", to.String()).
				Msg("Circuit breaker state changed")
		},
	}
}
