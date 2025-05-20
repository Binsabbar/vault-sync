package datastore

import "time"

type SyncStatus string

const (
	StatusSuccess SyncStatus = "success"
	StatusFailed  SyncStatus = "failed"
	StatusPending SyncStatus = "pending"
)

type SyncedSecret struct {
	ID                 int64      // Primary key
	SecretPath         string     `db:"secret_path"`
	SourceVersion      int        `db:"source_version"` // Version from the main cluster
	DestinationCluster string     `db:"destination_cluster"`
	DestinationVersion int        `db:"destination_version"` // Version written to destination (might be same as source)
	LastSyncAttempt    time.Time  `db:"last_sync_attempt"`
	LastSyncSuccess    *time.Time `db:"last_sync_success"` // Pointer for nullable
	Status             SyncStatus `db:"status"`
	ErrorMessage       *string    `db:"error_message"` // Pointer for nullable
}
