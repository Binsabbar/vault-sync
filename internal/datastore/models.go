package datastore

import "time"

type SyncStatus string

const (
	StatusSuccess SyncStatus = "success"
	StatusFailed  SyncStatus = "failed"
	StatusPending SyncStatus = "pending"
)

type SyncedSecret struct {
	ID                 int64      `db:"id"`
	SecretBackend      string     `db:"secret_backend"`
	SecretPath         string     `db:"secret_path"`
	SourceVersion      int        `db:"source_version"`
	DestinationCluster string     `db:"destination_cluster"`
	DestinationVersion int        `db:"destination_version"`
	LastSyncAttempt    time.Time  `db:"last_sync_attempt"`
	LastSyncSuccess    *time.Time `db:"last_sync_success"`
	Status             SyncStatus `db:"status"`
	ErrorMessage       *string    `db:"error_message"`
}
