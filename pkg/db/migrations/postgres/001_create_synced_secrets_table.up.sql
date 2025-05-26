CREATE TABLE IF NOT EXISTS synced_secrets (
    secret_path TEXT NOT NULL,
    secret_backend TEXT NOT NULL,
    source_version INTEGER NOT NULL,
    destination_cluster TEXT NOT NULL,
    destination_version INTEGER,
    last_sync_attempt TIMESTAMPTZ NOT NULL,
    last_sync_success TIMESTAMPTZ,
    status TEXT NOT NULL,
    error_message TEXT,
    PRIMARY KEY (secret_backend, secret_path, destination_cluster)
);