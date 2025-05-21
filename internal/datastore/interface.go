package datastore

type Datastore interface {
	GetSyncedSecret(backend, path, destinationCluster string) (*SyncedSecret, error)
	UpdateSyncedSecretStatus(secret SyncedSecret) error
	GetSyncedSecrets() ([]SyncedSecret, error)
	Close() error
}
