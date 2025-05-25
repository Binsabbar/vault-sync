package repository

import (
	"fmt"

	"vault-sync/internal/models"
	postgres "vault-sync/pkg/db"
	"vault-sync/pkg/log"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type PsqlSyncedSecretRepository struct {
	db postgres.PostgresDatastore
}

func (p *PsqlSyncedSecretRepository) GetSyncedSecret(backend, path, destinationCluster string) (*models.SyncedSecret, error) {
	args := map[string]interface{}{
		"secret_backend":      backend,
		"secret_path":         path,
		"destination_cluster": destinationCluster,
	}
	var secret models.SyncedSecret

	err := p.preparedStatements.selectSyncedSecretByPrimaryKey.Get(&secret, args)
	if err != nil {
		log.Logger.Error().Err(err).Str("args", fmt.Sprintf("%s,%s,%s", backend, path, destinationCluster)).Msg("Failed to get synced secret")
		return nil, fmt.Errorf("failed to get synced secret: %w", err)
	}
	log.Logger.Info().Str("args", fmt.Sprintf("%s,%s,%s", backend, path, destinationCluster)).Msg("Successfully retrieved synced secret")
	return &secret, nil
}

func (p *PsqlSyncedSecretRepository) GetSyncedSecrets() ([]models.SyncedSecret, error) {
	var secrets []models.SyncedSecret
	stmt := p.preparedStatements.selectAllSyncedSecrets
	err := stmt.Select(&secrets, struct{}{})
	if err != nil {
		log.Logger.Error().Err(err).Msg("Failed to get all synced secrets")
		return nil, fmt.Errorf("failed to get all synced secrets: %w", err)
	}
	return secrets, nil
}
