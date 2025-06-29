package repository

import "vault-sync/internal/models"

type SyncedSecretRepository interface {
	GetSyncedSecret(backend, path, destinationCluster string) (*models.SyncedSecret, error)
	UpdateSyncedSecretStatus(secret *models.SyncedSecret) error
	GetSyncedSecrets() ([]*models.SyncedSecret, error)
	Close() error
}
