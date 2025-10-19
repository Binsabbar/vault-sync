package models

import "time"

// SyncStatus represents the status of a secret synchronization operation.
const (
	StatusSuccess      SyncStatus = "success"
	StatusFailed       SyncStatus = "failed"
	StatusPending      SyncStatus = "pending"
	StatusDeleted      SyncStatus = "deleted"
	SyncStatusNotFound SyncStatus = "not_found"
)

type SyncStatus string

func (s SyncStatus) String() string {
	return string(s)
}

// SyncedSecret represents a secret that has been synchronized to a replica cluster.
type SyncedSecret struct {
	SecretBackend      string     `db:"secret_backend"`
	SecretPath         string     `db:"secret_path"`
	SourceVersion      int64      `db:"source_version"`
	DestinationCluster string     `db:"destination_cluster"`
	DestinationVersion int64      `db:"destination_version"`
	LastSyncAttempt    time.Time  `db:"last_sync_attempt"`
	LastSyncSuccess    *time.Time `db:"last_sync_success"`
	Status             SyncStatus `db:"status"`
	ErrorMessage       *string    `db:"error_message"`
}

func (s *SyncedSecret) SetErrorMessage(msg *string) {
	s.ErrorMessage = msg
}

func (s *SyncedSecret) SetStatus(status SyncStatus) {
	s.Status = status
}

func (s *SyncedSecret) SetLastSuccessAttempt(t *time.Time) {
	s.LastSyncSuccess = t
}

func (s *SyncedSecret) GetStatus() SyncStatus {
	return s.Status
}

func (s *SyncedSecret) GetDestinationCluster() string {
	return s.DestinationCluster
}

// SyncSecretDeletionResult represents the result of deleting a secret from a replica cluster.
type SyncSecretDeletionResult struct {
	DestinationCluster string
	SecretBackend      string
	SecretPath         string
	Status             SyncStatus
	DeletionAttempt    time.Time
	ErrorMessage       *string
}

func (s *SyncSecretDeletionResult) SetErrorMessage(msg *string) {
	s.ErrorMessage = msg
}

func (s *SyncSecretDeletionResult) SetStatus(status SyncStatus) {
	s.Status = status
}

func (s *SyncSecretDeletionResult) SetLastSuccessAttempt(t *time.Time) {
	s.DeletionAttempt = *t
}

func (s *SyncSecretDeletionResult) GetStatus() SyncStatus {
	return s.Status
}

func (s *SyncSecretDeletionResult) GetDestinationCluster() string {
	return s.DestinationCluster
}
