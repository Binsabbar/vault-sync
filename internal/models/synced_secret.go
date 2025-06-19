package models

import "time"

type SyncStatus string

const (
	StatusSuccess SyncStatus = "success"
	StatusFailed  SyncStatus = "failed"
	StatusPending SyncStatus = "pending"
)

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

func (s SyncStatus) String() string {
	return string(s)
}
