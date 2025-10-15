#!/bin/bash

set -e

VAULT_NAME="$1"
VAULT_KEYS_FILE="./vault-keys.txt"
VAULT_CONFIG_FILE="./vault-config.env"

if [ -z "$VAULT_NAME" ]; then
    echo "Usage: $0 <vault_name>"
    exit 1
fi

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}Initializing ${VAULT_NAME}...${NC}"

# Initialize vault
VAULT_INIT_OUTPUT=$(docker compose exec "$VAULT_NAME" sh -c "vault operator init -key-shares=1 -key-threshold=1 -format=yaml")

# Extract keys and tokens
VAULT_UNSEAL_KEY=$(echo "$VAULT_INIT_OUTPUT" | grep 'unseal_keys_b64:' -A 1 | tail -n 1 | awk '{print $2}')
VAULT_ROOT_TOKEN=$(echo "$VAULT_INIT_OUTPUT" | grep 'root_token:' | awk '{print $2}')

# Store in files
echo "${VAULT_NAME}_UNSEAL_KEY=$VAULT_UNSEAL_KEY" >> "$VAULT_KEYS_FILE"
echo "${VAULT_NAME}_ROOT_TOKEN=$VAULT_ROOT_TOKEN" >> "$VAULT_KEYS_FILE"

# Store in env format
VAULT_NAME_UPPER=$(echo "$VAULT_NAME" | tr '[:lower:]' '[:upper:]')
echo "export ${VAULT_NAME_UPPER}_UNSEAL_KEY=\"$VAULT_UNSEAL_KEY\"" >> "$VAULT_CONFIG_FILE"
echo "export ${VAULT_NAME_UPPER}_ROOT_TOKEN=\"$VAULT_ROOT_TOKEN\"" >> "$VAULT_CONFIG_FILE"

# Unseal vault
docker compose exec "$VAULT_NAME" vault operator unseal "$VAULT_UNSEAL_KEY" > /dev/null

# Login and enable userpass
docker compose exec "$VAULT_NAME" vault login "$VAULT_ROOT_TOKEN" > /dev/null
docker compose exec "$VAULT_NAME" vault auth enable userpass > /dev/null || true

# Create admin policy
docker compose exec "$VAULT_NAME" sh -c "cat > /tmp/admin-policy.hcl <<EOF
path \"*\" {
  capabilities = [\"create\", \"read\", \"update\", \"delete\", \"list\", \"sudo\"]
}
EOF"

docker compose exec "$VAULT_NAME" vault policy write admin /tmp/admin-policy.hcl > /dev/null
docker compose exec "$VAULT_NAME" vault write auth/userpass/users/admin password=admin policies=admin > /dev/null

# Enable KV v2 secrets engines
for mount in production uat stage; do
    docker compose exec "$VAULT_NAME" vault secrets enable -path="$mount" kv-v2 > /dev/null || true
done

echo -e "${GREEN}✓ ${VAULT_NAME} initialized and configured${NC}"