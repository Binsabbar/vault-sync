#!/bin/bash

set -e

VAULT_CONFIG_FILE="./vault-config.env"

if [ ! -f "$VAULT_CONFIG_FILE" ]; then
    echo "Error: $VAULT_CONFIG_FILE not found"
    exit 1
fi

# Source the config
source "$VAULT_CONFIG_FILE"

# Generate config file
cat > sample-config-docker.yaml <<EOF
---
id: docker-vault-sync
log_level: debug
concurrency: 5

sync_rule:
  interval: 60s
  kv_mounts:
    - production
    - uat
    - stage
  paths_to_replicate: []
  paths_to_ignore: []

postgres:
  address: postgres
  port: 5432
  username: vault_role
  password: vault_password
  db_name: vault_db
  ssl_mode: disable
  max_connections: 10

vault:
  main_cluster:
    name: main-cluster
    address: http://vault1:8200
    tls_skip_verify: true
    app_role_id: ${VAULT1_ROLE_ID}
    app_role_secret: ${VAULT1_SECRET_ID}
    app_role_mount: ${VAULT1_APPROLE_MOUNT}

  replica_clusters:
    - name: replica-2
      address: http://vault2:8200
      tls_skip_verify: true
      app_role_id: ${VAULT2_ROLE_ID}
      app_role_secret: ${VAULT2_SECRET_ID}
      app_role_mount: ${VAULT2_APPROLE_MOUNT}
EOF

echo "✓ Configuration generated: sample-config-docker.yaml"