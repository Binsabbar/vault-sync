package job

import (
	"context"
	"fmt"
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

// SyncDecision represents what action to take
type SyncDecision int

const (
	DecisionNoOp SyncDecision = iota
	DecisionSync
	DecisionDelete
)

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

	state, err := job.gatherCurrentState(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to gather current state: %w", err)
	}

	decision := job.makeDecision(state)
	logger.Debug().
		Bool("source_exists", state.SourceExists).
		Int("total_replicas", len(state.ReplicaNames)).
		Str("decision", decision.String()).
		Msg("Made sync decision")

	switch decision {
	case DecisionNoOp:
		return job.buildNoOpResult(state), nil
	case DecisionSync:
		return job.executeSync(ctx)
	case DecisionDelete:
		return job.executeDelete(ctx)
	default:
		return nil, fmt.Errorf("unknown decision: %v", decision)
	}
}

// SyncState holds all the information needed to make sync decisions
type SyncState struct {
	ReplicaNames     []string
	SourceExists     bool
	SourceVersion    int64
	RecordsByCluster map[string]*models.SyncedSecret
	ReplicaExistence map[string]bool
}

func (job *SyncJob) gatherCurrentState(ctx context.Context) (*SyncState, error) {
	state := &SyncState{
		RecordsByCluster: make(map[string]*models.SyncedSecret),
		ReplicaExistence: make(map[string]bool),
	}

	state.ReplicaNames = job.vaultClient.GetReplicaNames()

	recordsByCluster, err := job.getDBRecords(state.ReplicaNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get DB records: %w", err)
	}
	state.RecordsByCluster = recordsByCluster

	sourceExists, err := job.vaultClient.SecretExists(ctx, job.mount, job.keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to check source existence: %w", err)
	}
	state.SourceExists = sourceExists

	// Get source version if exists
	if sourceExists {
		metadata, err := job.vaultClient.GetSecretMetadata(ctx, job.mount, job.keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get source metadata: %w", err)
		}
		state.SourceVersion = metadata.CurrentVersion

		replicaExistence, err := job.checkReplicaExistence(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to check replica existence: %w", err)
		}
		state.ReplicaExistence = replicaExistence
	}

	return state, nil
}

func (job *SyncJob) checkReplicaExistence(ctx context.Context) (map[string]bool, error) {
	logger := job.logger.With().Str("action", "check_replica_existence").Logger()

	existence := make(map[string]bool)
	replicaNames := job.vaultClient.GetReplicaNames()

	for _, clusterName := range replicaNames {
		exists, err := job.vaultClient.SecretExistsInReplica(ctx, clusterName, job.mount, job.keyPath)
		if err != nil {
			logger.Warn().
				Str("cluster", clusterName).
				Err(err).
				Msg("Failed to check secret existence in replica")
			existence[clusterName] = false
			continue
		}
		existence[clusterName] = exists

		logger.Debug().
			Str("cluster", clusterName).
			Bool("exists", exists).
			Msg("Checked replica secret existence")
	}

	return existence, nil
}

func (job *SyncJob) getDBRecords(replicaNames []string) (map[string]*models.SyncedSecret, error) {
	logger := job.logger.With().Str("action", "get_db_records").Logger()

	records := make(map[string]*models.SyncedSecret)
	for _, clusterName := range replicaNames {
		record, err := job.databaseClient.GetSyncedSecret(job.mount, job.keyPath, clusterName)
		if err == repository.ErrSecretNotFound {
			logger.Debug().Str("cluster", clusterName).Msg("No DB record found")
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get record for cluster %s: %w", clusterName, err)
		}
		records[clusterName] = record
	}

	return records, nil
}

func (job *SyncJob) makeDecision(state *SyncState) SyncDecision {
	allReplicasHaveRecords := len(state.RecordsByCluster) == len(state.ReplicaNames)
	someReplicasHaveRecords := len(state.RecordsByCluster) > 0

	switch {
	case !state.SourceExists && !someReplicasHaveRecords:
		// No source, no records → no-op
		return DecisionNoOp

	case !state.SourceExists && someReplicasHaveRecords:
		// No source, but some records exist → delete
		return DecisionDelete

	case state.SourceExists && !allReplicasHaveRecords:
		// Source exists, missing some records → sync
		return DecisionSync

	case state.SourceExists && allReplicasHaveRecords:
		// Source exists, all records exist → check versions
		needsSync := false
		for clusterName, record := range state.RecordsByCluster {
			// Check if version is outdated
			if record.SourceVersion < state.SourceVersion {
				needsSync = true
				break
			}

			// Check if secret actually exists in replica vault
			if exists, ok := state.ReplicaExistence[clusterName]; ok && !exists {
				job.logger.Debug().
					Str("cluster", clusterName).
					Msg("DB record exists but secret missing from replica - needs sync")
				needsSync = true
				break
			}
		}
		if needsSync {
			return DecisionSync
		}
		return DecisionNoOp
	default:
		return DecisionNoOp
	}
}

func (job *SyncJob) executeSync(ctx context.Context) (*SyncJobResult, error) {
	logger := job.logger.With().Str("action", "sync").Logger()
	logger.Debug().Msg("Executing sync operation")

	syncResults, err := job.vaultClient.SyncSecretToReplicas(ctx, job.mount, job.keyPath)
	if err != nil {
		return nil, fmt.Errorf("vault sync failed: %w", err)
	}

	var multiErr MultiError
	clusterStatuses := make([]*ClusterSyncStatus, 0, len(syncResults))

	for _, syncResult := range syncResults {
		status := mapFromSyncedSecretStatus(syncResult.Status)
		if status == SyncJobStatusFailed {
			logger.Error().
				Str("cluster", syncResult.DestinationCluster).
				Msg("Failed to write to vault")
			multiErr.Add(fmt.Errorf("cluster %s vault write error", syncResult.DestinationCluster))
		}

		if dbErr := job.databaseClient.UpdateSyncedSecretStatus(syncResult); dbErr != nil {
			logger.Error().
				Str("cluster", syncResult.DestinationCluster).
				Err(dbErr).
				Msg("Failed to update database")
			status = SyncJobStatusFailed
			multiErr.Add(fmt.Errorf("cluster %s DB update: %w", syncResult.DestinationCluster, dbErr))
		}

		clusterStatuses = append(clusterStatuses, &ClusterSyncStatus{
			ClusterName: syncResult.DestinationCluster,
			Status:      status,
		})
	}

	logger.Debug().Int("synced_count", len(syncResults)).Msg("Sync operation completed")
	return NewSyncJobResult(job, clusterStatuses, multiErr.Err()), nil
}

func (job *SyncJob) executeDelete(ctx context.Context) (*SyncJobResult, error) {
	logger := job.logger.With().Str("action", "delete").Logger()
	logger.Debug().Msg("Executing delete operation")

	deleteResults, err := job.vaultClient.DeleteSecretFromReplicas(ctx, job.mount, job.keyPath)
	if err != nil {
		return nil, fmt.Errorf("vault delete failed: %w", err)
	}

	var multiErr MultiError
	clusterStatuses := make([]*ClusterSyncStatus, 0, len(deleteResults))

	for _, deleteResult := range deleteResults {
		localLogger := logger.With().Str("cluster", deleteResult.DestinationCluster).Logger()
		status := mapFromSyncedSecretStatus(deleteResult.Status)
		if status == SyncJobStatusFailed {
			localLogger.Error().Msg("Failed to delete from vault; trying to update DB with failed status")
			multiErr.Add(fmt.Errorf("cluster %s vault delete failed", deleteResult.DestinationCluster))
			status = SyncJobStatusErrorDeleting

			updateResult := &models.SyncedSecret{
				SecretBackend:      deleteResult.SecretBackend,
				SecretPath:         deleteResult.SecretPath,
				DestinationCluster: deleteResult.DestinationCluster,
				LastSyncAttempt:    deleteResult.DeletionAttempt,
				ErrorMessage:       deleteResult.ErrorMessage,
				Status:             deleteResult.Status,
				SourceVersion:      -1000,
				DestinationVersion: -1000,
			}
			if dbErr := job.databaseClient.UpdateSyncedSecretStatus(updateResult); dbErr != nil {
				localLogger.Error().Err(dbErr).Msg("Failed to update database with delete failure status")
				multiErr.Add(fmt.Errorf("cluster %s DB update after vault delete failure: %w", deleteResult.DestinationCluster, dbErr))
			}
		} else {
			localLogger.Debug().Msg("Successfully deleted from vault - removing DB record")
			if dbErr := job.databaseClient.DeleteSyncedSecret(job.mount, job.keyPath, deleteResult.DestinationCluster); dbErr != nil {
				localLogger.Error().Err(dbErr).Msg("Failed to delete from database")
				multiErr.Add(fmt.Errorf("cluster %s DB delete: %w", deleteResult.DestinationCluster, dbErr))
			}
		}

		clusterStatuses = append(clusterStatuses, &ClusterSyncStatus{
			ClusterName: deleteResult.DestinationCluster,
			Status:      status,
		})
	}

	logger.Debug().Int("deleted_count", len(deleteResults)).Msg("Delete operation completed")
	return NewSyncJobResult(job, clusterStatuses, multiErr.Err()), nil
}

func (job *SyncJob) buildNoOpResult(state *SyncState) *SyncJobResult {
	clusterStatuses := make([]*ClusterSyncStatus, 0, len(state.ReplicaNames))

	for _, clusterName := range state.ReplicaNames {
		clusterStatuses = append(clusterStatuses, &ClusterSyncStatus{
			ClusterName: clusterName,
			Status:      SyncJobStatusUnModified,
		})
	}

	job.logger.Info().Msg("No action needed - all replicas up to date")
	return NewSyncJobResult(job, clusterStatuses, nil)
}

// Helper types and functions
func (d SyncDecision) String() string {
	switch d {
	case DecisionNoOp:
		return "no-op"
	case DecisionSync:
		return "sync"
	case DecisionDelete:
		return "delete"
	default:
		return "unknown"
	}
}

type MultiError struct {
	errors []error
}

func (m *MultiError) Add(err error) {
	if err != nil {
		m.errors = append(m.errors, err)
	}
}

func (m *MultiError) Err() error {
	if len(m.errors) == 0 {
		return nil
	}
	if len(m.errors) == 1 {
		return m.errors[0]
	}
	return fmt.Errorf("multiple errors: %v", m.errors)
}

// Rest of your existing types and constants remain the same...
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

func NewSyncJobResult(job *SyncJob, status []*ClusterSyncStatus, err error) *SyncJobResult {
	return &SyncJobResult{
		Mount:   job.mount,
		KeyPath: job.keyPath,
		Status:  status,
		Error:   err,
	}
}
