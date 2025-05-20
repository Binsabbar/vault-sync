package datastore

type Datastore interface {
	GetLastSyncedSecret(path, destinationCluster string) (*SyncedSecret, error)
	RecordSyncAttempt(secret SyncedSecret) error
	Close() error
}
