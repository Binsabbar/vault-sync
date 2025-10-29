#!/bin/bash

set -e

VAULT_NAME="$1"
ROOT_TOKEN="$2"
POLICY_TYPE="$3"  # ro OR rw

if [ -z "$VAULT_NAME" ] || [ -z "$ROOT_TOKEN" ] || [ -z "$POLICY_TYPE" ]; then
    echo "Usage: $0 <vault_name> <root_token> <policy_type>"
    exit 1
fi

# Colors
GREEN='\033[0;32m'
NC='\033[0m'

VAULT_CONFIG_FILE="./vault-config.env"

# Login to vault
docker compose exec "$VAULT_NAME" vault login "$ROOT_TOKEN" > /dev/null

# Determine AppRole mount path
if [ "$VAULT_NAME" = "vault1" ]; then
    APPROLE_MOUNT="approle"
else
    APPROLE_MOUNT="approle2"
fi

# Enable AppRole auth method
docker compose exec "$VAULT_NAME" vault auth enable -path="$APPROLE_MOUNT" approle > /dev/null 2>&1 || true

# Create policy based on type
POLICY_NAME="vault-sync-${POLICY_TYPE}"

if [ "$POLICY_TYPE" = "ro" ]; then
    # Read-only policy
    docker compose exec "$VAULT_NAME" sh -c "cat > /tmp/${POLICY_NAME}.hcl <<EOF
# Read access to all KV v2 secrets
path \"production/*\" {
  capabilities = [\"read\", \"list\"]
}
path \"production/data/*\" {
  capabilities = [\"read\", \"list\"]
}
path \"production/metadata/*\" {
  capabilities = [\"read\", \"list\"]
}
path \"uat/*\" {
  capabilities = [\"read\", \"list\"]
}
path \"uat/data/*\" {
  capabilities = [\"read\", \"list\"]
}
path \"uat/metadata/*\" {
  capabilities = [\"read\", \"list\"]
}
path \"stage/*\" {
  capabilities = [\"read\", \"list\"]
}
path \"stage/data/*\" {
  capabilities = [\"read\", \"list\"]
}
path \"stage/metadata/*\" {
  capabilities = [\"read\", \"list\"]
}
path \"sys/mounts\" {
  capabilities = [\"read\", \"list\"]
}
EOF"
else
    # Read-write policy
    docker compose exec "$VAULT_NAME" sh -c "cat > /tmp/${POLICY_NAME}.hcl <<EOF
# Full access to all KV v2 secrets
path \"production/*\" {
  capabilities = [\"create\", \"read\", \"update\", \"delete\", \"list\"]
}
path \"production/data/*\" {
  capabilities = [\"create\", \"read\", \"update\", \"delete\", \"list\"]
}
path \"production/metadata/*\" {
  capabilities = [\"read\", \"list\", \"delete\"]
}
path \"uat/*\" {
  capabilities = [\"create\", \"read\", \"update\", \"delete\", \"list\"]
}
path \"uat/data/*\" {
  capabilities = [\"create\", \"read\", \"update\", \"delete\", \"list\"]
}
path \"uat/metadata/*\" {
  capabilities = [\"read\", \"list\", \"delete\"]
}
path \"stage/*\" {
  capabilities = [\"create\", \"read\", \"update\", \"delete\", \"list\"]
}
path \"stage/data/*\" {
  capabilities = [\"create\", \"read\", \"update\", \"delete\", \"list\"]
}
path \"stage/metadata/*\" {
  capabilities = [\"read\", \"list\", \"delete\"]
}
path \"sys/mounts\" {
  capabilities = [\"read\", \"list\"]
}
EOF"
fi

# Write policy
docker compose exec "$VAULT_NAME" vault policy write "$POLICY_NAME" "/tmp/${POLICY_NAME}.hcl" > /dev/null

# Create AppRole
docker compose exec "$VAULT_NAME" vault write "auth/${APPROLE_MOUNT}/role/vault_sync" \
    token_policies="$POLICY_NAME" \
    token_ttl=1h \
    token_max_ttl=4h \
    secret_id_ttl=0 \
    secret_id_num_uses=0 > /dev/null

# Get Role ID and Secret ID
ROLE_ID=$(docker compose exec "$VAULT_NAME" vault read -field=role_id "auth/${APPROLE_MOUNT}/role/vault_sync/role-id")
SECRET_ID=$(docker compose exec "$VAULT_NAME" vault write -field=secret_id -f "auth/${APPROLE_MOUNT}/role/vault_sync/secret-id")

# Store in config file
VAULT_NAME_UPPER=$(echo "$VAULT_NAME" | tr '[:lower:]' '[:upper:]')
echo "export ${VAULT_NAME_UPPER}_ROLE_ID=\"$ROLE_ID\"" >> "$VAULT_CONFIG_FILE"
echo "export ${VAULT_NAME_UPPER}_SECRET_ID=\"$SECRET_ID\"" >> "$VAULT_CONFIG_FILE"
echo "export ${VAULT_NAME_UPPER}_APPROLE_MOUNT=\"$APPROLE_MOUNT\"" >> "$VAULT_CONFIG_FILE"

echo -e "${GREEN}✓ ${VAULT_NAME} AppRole configured (${POLICY_TYPE})${NC}"