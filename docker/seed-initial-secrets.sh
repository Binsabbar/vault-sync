#!/bin/bash

set -e

VAULT_KEYS_FILE="./vault-keys.txt"

if [ ! -f "$VAULT_KEYS_FILE" ]; then
    echo "Error: $VAULT_KEYS_FILE not found"
    exit 1
fi

# Get vault1 token
VAULT1_ROOT_TOKEN=$(grep "vault1_ROOT_TOKEN=" "$VAULT_KEYS_FILE" | cut -d'=' -f2)

# Login to vault1
docker compose exec vault1 vault login "$VAULT1_ROOT_TOKEN" > /dev/null

echo "Seeding initial secrets..."

# Production secrets
docker compose exec vault1 vault kv put production/app/argocd SECRET=ARGO > /dev/null
docker compose exec vault1 vault kv put production/app/loki SECRET1=LOKI-PROD SECRET2=LOKI-PROD2 > /dev/null
docker compose exec vault1 vault kv put production/infra/gcp SECRET=GCP > /dev/null
docker compose exec vault1 vault kv put production/infra/grafana SECRET=GRAFANA PASSWORD=ADMIN > /dev/null

# UAT secrets
docker compose exec vault1 vault kv put uat/app/argocd SECRET=ARGO > /dev/null
docker compose exec vault1 vault kv put uat/app/loki SECRET1=LOKI-UAT SECRET2=LOKI-UAT2 > /dev/null

# Stage secrets
docker compose exec vault1 vault kv put stage/app/argocd SECRET=ARGO > /dev/null
docker compose exec vault1 vault kv put stage/app/loki SECRET1=LOKI-STAGE SECRET2=LOKI-STAGE2 > /dev/null

echo "✓ Initial secrets seeded"