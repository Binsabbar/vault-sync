CREATE TABLE IF NOT EXISTS synced_secrets (
    id SERIAL PRIMARY KEY,
    secret_path TEXT NOT NULL,
    source_version INTEGER NOT NULL,
    destination_cluster TEXT NOT NULL,
    destination_version INTEGER NOT NULL,
    last_sync_attempt TIMESTAMPTZ NOT NULL,
    last_sync_success TIMESTAMPTZ,
    status TEXT NOT NULL,
    error_message TEXT,
    UNIQUE (secret_path, destination_cluster)
);