package repository

import (
	"fmt"

	"vault-sync/internal/models"
	postgres "vault-sync/pkg/db"
	"vault-sync/pkg/log"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog"
)

type PsqlSyncedSecretRepository struct {
	psql *postgres.PostgresDatastore
}

func NewPsqlSyncedSecretRepository(psql *postgres.PostgresDatastore) *PsqlSyncedSecretRepository {
	return &PsqlSyncedSecretRepository{
		psql: psql,
	}
}

func (repo *PsqlSyncedSecretRepository) GetSyncedSecret(backend, path, destinationCluster string) (*models.SyncedSecret, error) {
	var secret models.SyncedSecret
	query := `SELECT * FROM synced_secrets WHERE secret_backend = $1 AND secret_path = $2 AND destination_cluster = $3`

	err := repo.psql.DB.Get(&secret, query, backend, path, destinationCluster)
	if err != nil {
		repo.decorateLog(log.Logger.Error, backend, path, destinationCluster).Err(err).Msg("Failed to get synced secret")
		return nil, fmt.Errorf("failed to get synced secret: %w", err)
	}
	repo.decorateLog(log.Logger.Debug, backend, path, destinationCluster).Msg("Successfully retrieved synced secret")
	return &secret, nil
}

func (repo *PsqlSyncedSecretRepository) GetSyncedSecrets() ([]models.SyncedSecret, error) {
	var secrets = make([]models.SyncedSecret, 0)
	query := `SELECT * FROM synced_secrets ORDER BY secret_backend, secret_path, destination_cluster`
	err := repo.psql.DB.Select(&secrets, query)
	if err != nil {
		log.Logger.Error().Err(err).Msg("Failed to get all synced secrets")
		return secrets, fmt.Errorf("failed to get all synced secrets: %w", err)
	}
	return secrets, nil
}

func (repo *PsqlSyncedSecretRepository) decorateLog(eventFactory func() *zerolog.Event, backend, path, destinationCluster string) *zerolog.Event {
	return eventFactory().Str("backend", backend).Str("path", path).Str("destinationCluster", destinationCluster)
}
