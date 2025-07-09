package job

import (
	"context"
	"vault-sync/internal/models"
	"vault-sync/internal/repository"
	"vault-sync/internal/vault"
	"vault-sync/pkg/log"

	"github.com/rs/zerolog"
)

type SyncJob struct {
	mount          string
	keyPath        string
	vaultClient    vault.VaultSyncer
	databaseClient repository.SyncedSecretRepository
	logger         zerolog.Logger
}

type jobPayload struct {
	Mount   string
	KeyPath string
}

func NewSyncJob(mount, keyPath string, vaultClient vault.VaultSyncer, dbClient repository.SyncedSecretRepository) *SyncJob {
	return &SyncJob{
		mount:          mount,
		keyPath:        keyPath,
		vaultClient:    vaultClient,
		databaseClient: dbClient,
		logger: log.Logger.With().
			Str("component", "sync_job").
			Str("mount", mount).
			Str("key_path", keyPath).
			Logger(),
	}
}

const (
	initialSourceVersion = 0
)

func (job *SyncJob) Execute(ctx context.Context) (*SyncJobResult, error) {
	logger := job.logger.With().Str("action", "execute").Logger()
	logger.Debug().Msg("Starting secret sync job")

	syncedSecrets, secretNotFound, err := job.checkSyncedSecretInDatabaseInClusters()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to check synced secret in database")
		return nil, err
	}

	if secretNotFound {
		logger.Debug().Msg("Secret does not exist in every cluster, syncing to all replicas")
		return job.syncSecret(ctx)
	}

	sourceSecretExists, err := job.vaultClient.SecretExists(ctx, job.mount, job.keyPath)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to check if secret exists in vault")
		return nil, err
	}

	if !sourceSecretExists {
		logger.Debug().Msg("Secret does not exist in source, deleting from all replicas")
		return job.deleteSecret(ctx)
	}

	secretMetadata, err := job.vaultClient.GetSecretMetadata(ctx, job.mount, job.keyPath)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to get secret metadata from vault")
		return nil, err
	}

	clusterStatus := make([]*ClusterSyncStatus, 0, len(syncedSecrets))
	for _, syncedSecret := range syncedSecrets {
		if syncedSecret.SourceVersion < secretMetadata.CurrentVersion {
			logger.Debug().
				Int64("source_version", secretMetadata.CurrentVersion).
				Msg("Current source version is greater than DB version, syncing secret to all replicas")
			return job.syncSecret(ctx)
		}
		clusterStatus = append(clusterStatus, &ClusterSyncStatus{
			ClusterName: syncedSecret.DestinationCluster,
			Status:      SyncJobStatusUnModified,
		})
	}

	logger.Info().Msg("All synced secrets are up to date, no sync needed")
	return NewSyncJobResult(job, clusterStatus, nil), nil
}

// checkSyncedSecretInDatabaseInClusters checks if the synced secret exists in the database for every cluster
// returns the existing synced secrets and a boolean indicating if the secret was not found in any cluster.
// note: if the secret is not found in any cluster, it will return a slice, which might contain all secrets currently in the database.
func (job *SyncJob) checkSyncedSecretInDatabaseInClusters() ([]*models.SyncedSecret, bool, error) {
	logger := job.logger.With().Str("action", "check_synced_secret_exists").Logger()
	logger.Debug().Msg("Checking if synced secret exists in database")

	clusters := job.vaultClient.GetReplicaNames()
	secretNotFound := false
	existingSecrets := make([]*models.SyncedSecret, 0, len(clusters))
	for _, clusterName := range clusters {
		syncedSecret, err := job.databaseClient.GetSyncedSecret(job.mount, job.keyPath, clusterName)

		if err != nil && err == repository.ErrSecretNotFound {
			logger.Debug().Str("cluster_name", clusterName).Msg("Secret not found in database, will sync to this cluster")
			secretNotFound = true
			break
		} else if err != nil {
			logger.Error().Str("cluster_name", clusterName).Err(err).Msg("Failed to get synced secret from database")
			return nil, false, err
		}

		logger.Debug().Str("cluster_name", clusterName).Msg("Secret found in database")
		existingSecrets = append(existingSecrets, syncedSecret)
	}

	return existingSecrets, secretNotFound, nil
}

func (job *SyncJob) syncSecret(ctx context.Context) (*SyncJobResult, error) {
	logger := job.logger.With().Str("action", "sync_secret").Logger()
	logger.Debug().Msg("Syncing secret to replicas")

	syncedSecrets, err := job.vaultClient.SyncSecretToReplicas(ctx, job.mount, job.keyPath)
	if err != nil {
		return nil, err
	}

	clusterStatus := make([]*ClusterSyncStatus, 0, len(syncedSecrets))
	for _, secret := range syncedSecrets {
		status := mapFromSyncedSecretStatus(secret.Status)

		logger.Debug().Str("cluster_name", secret.DestinationCluster).Msg("Update Database with synced secret status")
		err := job.databaseClient.UpdateSyncedSecretStatus(secret)
		if err != nil {
			logger.Error().Str("cluster_name", secret.DestinationCluster).Err(err).Msg("Failed to update synced secret status in database")
			status = SyncJobStatusFailed
		}
		clusterStatus = append(clusterStatus, &ClusterSyncStatus{ClusterName: secret.DestinationCluster, Status: status})
	}

	logger.Debug().Int("synced_secrets_count", len(syncedSecrets)).Msg("Secrets synced to replicas successfully")
	return NewSyncJobResult(job, clusterStatus, nil), nil
}

func (job *SyncJob) deleteSecret(ctx context.Context) (*SyncJobResult, error) {
	logger := job.logger.With().Str("action", "delete_secret").Logger()
	logger.Debug().Msg("Deleting secret from replicas")

	deletedSecrets, err := job.vaultClient.DeleteSecretFromReplicas(ctx, job.mount, job.keyPath)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to delete secret from replicas")
		return nil, err
	}

	clusterStatus := make([]*ClusterSyncStatus, 0, len(deletedSecrets))
	for _, secret := range deletedSecrets {
		status := &ClusterSyncStatus{ClusterName: secret.DestinationCluster, Status: mapFromSyncedSecretStatus(secret.Status)}
		if secret.Status == models.StatusFailed {
			status.Status = SyncJobStatusErrorDeleting
			logger.Error().Str("cluster_name", secret.DestinationCluster).Msg("Failed to delete secret from replica cluster")
		}
		logger.Debug().Str("cluster_name", secret.DestinationCluster).Msg("Update Database with deleted secret status")
		err := job.databaseClient.DeleteSyncedSecret(secret.SecretBackend, secret.SecretPath, secret.DestinationCluster)
		if err != nil {
			logger.Error().Str("cluster_name", secret.DestinationCluster).Err(err).Msg("Failed to delete synced secret from database")
			return nil, err
		}
		clusterStatus = append(clusterStatus, status)
	}

	logger.Debug().Int("deleted_secrets_count", len(deletedSecrets)).Msg("Secrets deleted from replicas successfully")
	return NewSyncJobResult(job, clusterStatus, nil), nil
}

// ***************
//
// SyncJobResult represents the result of a sync job execution
// ***************
type SyncJobStatus string

const (
	SyncJobStatusUpdated       SyncJobStatus = "updated"
	SyncJobStatusDeleted       SyncJobStatus = "deleted"
	SyncJobStatusErrorDeleting SyncJobStatus = "error_deleting"
	SyncJobStatusUnModified    SyncJobStatus = "unmodified"
	SyncJobStatusFailed        SyncJobStatus = "failed"
	SyncJobStatusUnknown       SyncJobStatus = "unknown"
	SyncJobStatusPending       SyncJobStatus = "pending"
)

func mapFromSyncedSecretStatus(status models.SyncStatus) SyncJobStatus {
	switch status {
	case models.StatusSuccess:
		return SyncJobStatusUpdated
	case models.StatusDeleted:
		return SyncJobStatusDeleted
	case models.StatusFailed:
		return SyncJobStatusFailed
	case models.StatusPending:
		return SyncJobStatusPending
	default:
		return SyncJobStatusUnknown
	}

}

type SyncJobResult struct {
	Mount   string
	KeyPath string
	Status  []*ClusterSyncStatus
	Error   error
}

type ClusterSyncStatus struct {
	ClusterName string
	Status      SyncJobStatus
}

type genericSyncedSecret interface {
	*models.SyncedSecret | *models.SyncSecretDeletionResult
}

func NewSyncJobResult(job *SyncJob, status []*ClusterSyncStatus, err error) *SyncJobResult {
	return &SyncJobResult{
		Mount:   job.mount,
		KeyPath: job.keyPath,
		Status:  status,
		Error:   err,
	}
}
