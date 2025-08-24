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

func (job *SyncJob) Execute(ctx context.Context) (*SyncJobResult, error) {
	logger := job.logger.With().Str("action", "execute").Logger()
	logger.Debug().Msg("Starting secret sync job")

	replicas := job.vaultClient.GetReplicaNames()
	replicasSyncedStatus, err := job.getSyncedStatusFromDatabase(replicas)
	if err != nil {
		return job.handleError(err, "Failed to check synced secret in database")
	}

	sourceSecretExists, err := job.vaultClient.SecretExists(ctx, job.mount, job.keyPath)
	if err != nil {
		return job.handleError(err, "Failed to check if secret exists in vault")
	}

	allReplicasHaveSyncStatus := len(replicasSyncedStatus) == len(replicas)
	someOrAllReplicasHaveSyncStatus := len(replicasSyncedStatus) > 0

	if sourceSecretExists {
		if allReplicasHaveSyncStatus {
			secretMetadata, err := job.vaultClient.GetSecretMetadata(ctx, job.mount, job.keyPath)
			if err != nil {
				return job.handleError(err, "Failed to get secret metadata from vault")
			}

			if !job.doesSourceVersionMatchReplicas(secretMetadata.CurrentVersion, replicasSyncedStatus) {
				return job.syncSecret(ctx)
			}
			return job.buildUnmodifiedResult(replicas), nil
		}
		logger.Debug().Msg("Secret does not exist in every cluster, syncing to all replicas")
		return job.syncSecret(ctx)
	} else {
		if someOrAllReplicasHaveSyncStatus {
			logger.Debug().Msg("Secret does not exist in source, deleting from all replicas")
			return job.deleteSecret(ctx)
		}
		logger.Info().Msg("No sync status exists and no source secret exists, no action needed (no-op)")
		return NewSyncJobResult(job, []*ClusterSyncStatus{}, nil), nil
	}
}

// getSyncedStatusFromDatabase returns sync secret status from the databases for replicas
// if the secret is not found for a cluster, the slice will miss that item.
// always check the len of the return slice and compare it with the replicaClusters length
// if error is raised, it is returned and slice is set to nil
func (job *SyncJob) getSyncedStatusFromDatabase(replicaClusters []string) ([]*models.SyncedSecret, error) {
	logger := job.logger.With().Str("action", "check_synced_secret_exists").Logger()
	logger.Debug().Msg("Checking if synced secret exists in database")

	existingSyncStatus := make([]*models.SyncedSecret, 0, len(replicaClusters))
	for _, clusterName := range replicaClusters {
		syncedStatus, err := job.databaseClient.GetSyncedSecret(job.mount, job.keyPath, clusterName)
		if err != nil && err == repository.ErrSecretNotFound {
			logger.Debug().Str("cluster_name", clusterName).Msg("Secret not found in database, will sync to this cluster")
		} else if err != nil {
			logger.Error().Str("cluster_name", clusterName).Err(err).Msg("Failed to get synced secret from database")
			return nil, err
		}
		logger.Debug().Str("cluster_name", clusterName).Msg("Secret found in database")
		existingSyncStatus = append(existingSyncStatus, syncedStatus)
	}

	return existingSyncStatus, nil
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

	secretDeletionStatus, err := job.vaultClient.DeleteSecretFromReplicas(ctx, job.mount, job.keyPath)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to delete secret from replicas")
		return nil, err
	}
	successDeletionStatus = 

	clusterSyncStatus := make([]*ClusterSyncStatus, 0, len(secretDeletionStatus))

	for _, deletionStatus := range secretDeletionStatus {
		logger := logger.With().Str("cluster_name", deletionStatus.DestinationCluster).Logger()

		status := &ClusterSyncStatus{ClusterName: deletionStatus.DestinationCluster, Status: mapFromSyncedSecretStatus(deletionStatus.Status)}
		if status.Status == SyncJobStatusFailed {
			status.Status = SyncJobStatusErrorDeleting
			logger.Error().Err(err).Msg("Failed to delete secret from replica cluster")
		}
		logger.Debug().Msg("Update Database with deleted secret status")
		err := job.databaseClient.DeleteSyncedSecret(deletionStatus.SecretBackend, deletionStatus.SecretPath, deletionStatus.DestinationCluster)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to delete synced secret from database")
			return nil, err
		}
		clusterSyncStatus = append(clusterSyncStatus, status)
	}

	logger.Debug().Int("deleted_secrets_count", len(secretDeletionStatus)).Msg("Secrets deleted from replicas successfully")
	return NewSyncJobResult(job, clusterSyncStatus, nil), nil
}

// doesSourceVersionMatchReplicas compares source version with replica versions
func (job *SyncJob) doesSourceVersionMatchReplicas(sourceVersion int64, replicasSyncedStatus []*models.SyncedSecret) bool {
	logger := job.logger.With().Str("action", "does_source_version_match_replcas").Logger()

	doesSourceVersionMatch := true
	for _, syncStatus := range replicasSyncedStatus {
		if syncStatus.SourceVersion != sourceVersion {
			logger.Debug().
				Int64("source_version", sourceVersion).
				Int64("db_version", syncStatus.SourceVersion).
				Str("cluster", syncStatus.DestinationCluster).
				Msg("Current source version is greater than DB version")
			doesSourceVersionMatch = false
		}
	}

	return doesSourceVersionMatch
}

// handleError logs the error and returns nil result with the error
func (job *SyncJob) handleError(err error, msg string) (*SyncJobResult, error) {
	job.logger.Error().Err(err).Msg(msg)
	return nil, err
}

// buildUnmodifiedResult creates a result with all replicas marked as unmodified
func (job *SyncJob) buildUnmodifiedResult(clusters []string) *SyncJobResult {
	clusterStatus := make([]*ClusterSyncStatus, 0, len(clusters))
	for _, cluster := range clusters {
		clusterStatus = append(clusterStatus, &ClusterSyncStatus{
			ClusterName: cluster,
			Status:      SyncJobStatusUnModified,
		})
	}
	job.logger.Info().Msg("All replicas are up to date, no sync needed")
	return NewSyncJobResult(job, clusterStatus, nil)
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
		return SyncJobSitatusUnknown
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

func NewSyncJobResult(job *SyncJob, status []*ClusterSyncStatus, err error) *SyncJobResult {
	return &SyncJobResult{
		Mount:   job.mount,
		KeyPath: job.keyPath,
		Status:  status,
		Error:   err,
	}
}
