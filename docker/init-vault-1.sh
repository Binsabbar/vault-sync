#/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Init and unseal vault1
VAULT_INIT_OUTPUT=$(docker compose exec vault1 sh -c "vault operator init -key-shares=1 -key-threshold=1 -format=yaml")
VAULT_UNSEAL_KEY=$(echo "$VAULT_INIT_OUTPUT" | grep 'unseal_keys_b64:' -A 1 | tail -n 1 | awk '{print $2}')
echo "Vault 1 Unseal Key: $VAULT_UNSEAL_KEY" >> ./vault-keys.txt
VAULT_ROOT_TOKEN_1=$(echo "$VAULT_INIT_OUTPUT" | grep 'root_token:' | awk '{print $2}')
echo "Vault 1 Root Token: $VAULT_ROOT_TOKEN_1" >> ./vault-keys.txt

docker compose exec vault1 vault operator unseal $VAULT_UNSEAL_KEY

# Login to vault1
docker compose exec vault1 vault login $VAULT_ROOT_TOKEN_1
docker compose exec vault1 vault auth enable userpass
docker compose exec vault1 sh -c "echo '
path \"*\" {
  capabilities = [\"create\", \"read\", \"update\", \"delete\", \"list\", \"sudo\"]
}
' > /vault/file/admin-policy.hcl"
docker compose exec vault1 vault policy write admin /vault/file/admin-policy.hcl
docker compose exec vault1 vault write auth/userpass/users/admin password=admin policies=admin

# Enable KV v2 secrets engines
docker compose exec vault1 vault secrets enable -path=production kv-v2
docker compose exec vault1 vault secrets enable -path=uat kv-v2
docker compose exec vault1 vault secrets enable -path=stage kv-v2

# Add secrets to each engine
docker compose exec vault1 vault kv put production/app/argocd SECRET=ARGO
docker compose exec vault1 vault kv put production/app/loki SECRET1=LOKI-PROD SECRET2=LOKI-PROD2

docker compose exec vault1 vault kv put uat/app/argocd SECRET=ARGO
docker compose exec vault1 vault kv put uat/app/loki SECRET1=LOKI-UAT SECRET2=LOKI-UAT2

docker compose exec vault1 vault kv put stage/app/argocd SECRET=ARGO
docker compose exec vault1 vault kv put stage/app/loki SECRET1=LOKI-STAGE SECRET2=LOKI-STAGE2

docker compose exec vault1 vault kv put production/infra/gcp SECRET=GCP
docker compose exec vault1 vault kv put production/infra/grafana SECRET=GRAFANA PASSWORD=ADMIN

# Init and unseal vault2
VAULT_INIT_OUTPUT=$(docker compose exec vault2 sh -c "vault operator init -key-shares=1 -key-threshold=1 -format=yaml")
VAULT_UNSEAL_KEY=$(echo "$VAULT_INIT_OUTPUT" | grep 'unseal_keys_b64:' -A 1 | tail -n 1 | awk '{print $2}')
echo "Vault 2 Unseal Key: $VAULT_UNSEAL_KEY" >> ./vault-keys.txt
VAULT_ROOT_TOKEN_2=$(echo "$VAULT_INIT_OUTPUT" | grep 'root_token:' | awk '{print $2}')
echo "Vault 2 Root Token: $VAULT_ROOT_TOKEN_2" >> ./vault-keys.txt
docker compose exec vault2 vault operator unseal $VAULT_UNSEAL_KEY

# Login to vault2
docker compose exec vault2 vault login $VAULT_ROOT_TOKEN_2
docker compose exec vault2 vault auth enable userpass
docker compose exec vault2 sh -c "echo '
path \"*\" {
  capabilities = [\"create\", \"read\", \"update\", \"delete\", \"list\", \"sudo\"]
}
' > /vault/file/admin-policy.hcl"
docker compose exec vault2 vault policy write admin /vault/file/admin-policy.hcl
docker compose exec vault2 vault write auth/userpass/users/admin password=admin policies=admin

echo "========================================="
echo "Setting up AppRole for Vault Sync"
echo "========================================="

echo ""
echo "========================================="
echo "Configuring Vault1 (Main - Read-Only)"
echo "========================================="


docker compose exec vault1 vault login $VAULT_ROOT_TOKEN_1
echo "Enabling AppRole auth method..."
docker compose exec vault1 vault auth enable approle || echo "AppRole already enabled"
echo "Creating read-only policy..."
docker compose exec vault1 sh -c 'cat > /tmp/vault-sync-ro-policy.hcl <<EOF
# Read access to all KV v2 secrets
path "production/*" {
  capabilities = ["read", "list"]
}

path "production/data/*" {
  capabilities = ["read", "list"]
}

path "production/metadata/*" {
  capabilities = ["read", "list"]
}

path "uat/*" {
  capabilities = ["read", "list"]
}

path "uat/data/*" {
  capabilities = ["read", "list"]
}

path "uat/metadata/*" {
  capabilities = ["read", "list"]
}

path "stage/*" {
  capabilities = ["read", "list"]
}

path "stage/data/*" {
  capabilities = ["read", "list"]
}

path "stage/metadata/*" {
  capabilities = ["read", "list"]
}

# List all mounts
path "sys/mounts" {
  capabilities = ["read", "list"]
}
EOF'

docker compose exec vault1 vault policy write vault-sync-ro /tmp/vault-sync-ro-policy.hcl

# Create AppRole
echo "Creating vault_sync AppRole..."
docker compose exec vault1 vault write auth/approle/role/vault_sync \
    token_policies="vault-sync-ro" \
    token_ttl=1h \
    token_max_ttl=4h \
    secret_id_ttl=0 \
    secret_id_num_uses=0

# Get Role ID
VAULT1_ROLE_ID=$(docker compose exec vault1 vault read -field=role_id auth/approle/role/vault_sync/role-id)
echo -e "${GREEN}Vault1 Role ID: ${VAULT1_ROLE_ID}${NC}"

# Generate Secret ID
VAULT1_SECRET_ID=$(docker compose exec vault1 vault write -field=secret_id -f auth/approle/role/vault_sync/secret-id)
echo -e "${GREEN}Vault1 Secret ID: ${VAULT1_SECRET_ID}${NC}"

echo ""
echo "========================================="
echo "Configuring Vault2 (Replica - Read-Write)"
echo "========================================="

# Login to vault2
docker compose exec vault2 vault login $VAULT_ROOT_TOKEN_2

# Enable AppRole auth method
echo "Enabling AppRole auth method..."
docker compose exec vault2 vault auth enable -path=approle2 approle || echo "AppRole already enabled"

# Enable KV v2 secrets engines on vault2 (if not already done)
echo "Enabling KV v2 secrets engines..."
docker compose exec vault2 vault secrets enable -path=production kv-v2 || echo "production already enabled"
docker compose exec vault2 vault secrets enable -path=uat kv-v2 || echo "uat already enabled"
docker compose exec vault2 vault secrets enable -path=stage kv-v2 || echo "stage already enabled"

# Create read-write policy for vault-sync
echo "Creating read-write policy..."
docker compose exec vault2 sh -c 'cat > /tmp/vault-sync-rw-policy.hcl <<EOF
# Full access to all KV v2 secrets
path "production/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "production/data/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "production/metadata/*" {
  capabilities = ["read", "list", "delete"]
}

path "uat/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "uat/data/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "uat/metadata/*" {
  capabilities = ["read", "list", "delete"]
}

path "stage/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "stage/data/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "stage/metadata/*" {
  capabilities = ["read", "list", "delete"]
}

# List all mounts
path "sys/mounts" {
  capabilities = ["read", "list"]
}
EOF'

docker compose exec vault2 vault policy write vault-sync-rw /tmp/vault-sync-rw-policy.hcl

# Create AppRole
echo "Creating vault_sync AppRole..."
docker compose exec vault2 vault write auth/approle2/role/vault_sync \
    token_policies="vault-sync-rw" \
    token_ttl=1h \
    token_max_ttl=4h \
    secret_id_ttl=0 \
    secret_id_num_uses=0

# Get Role ID
VAULT2_ROLE_ID=$(docker compose exec vault2 vault read -field=role_id auth/approle2/role/vault_sync/role-id)
echo -e "${GREEN}Vault2 Role ID: ${VAULT2_ROLE_ID}${NC}"

# Generate Secret ID
VAULT2_SECRET_ID=$(docker compose exec vault2 vault write -field=secret_id -f auth/approle2/role/vault_sync/secret-id)
echo -e "${GREEN}Vault2 Secret ID: ${VAULT2_SECRET_ID}${NC}"

echo ""
echo "========================================="
echo "Summary"
echo "========================================="
echo -e "${GREEN}✓ AppRole configured on both Vault instances${NC}"
echo ""
echo "Vault1 (Main - Read-Only):"
echo "  Auth Mount: approle"
echo "  Role ID:    $VAULT1_ROLE_ID"
echo "  Secret ID:  $VAULT1_SECRET_ID"
echo ""
echo "Vault2 (Replica - Read-Write):"
echo "  Auth Mount: approle2"
echo "  Role ID:    $VAULT2_ROLE_ID"
echo "  Secret ID:  $VAULT2_SECRET_ID"
echo ""
echo "========================================="
echo "Updated sample-config-docker.yaml"
echo "========================================="

# Generate updated config
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
    app_role_mount: approle

  replica_clusters:
    - name: replica-2
      address: http://vault2:8200
      tls_skip_verify: true
      app_role_id: ${VAULT2_ROLE_ID}
      app_role_secret: ${VAULT2_SECRET_ID}
      app_role_mount: approle2
EOF

echo -e "${GREEN}✓ Config file updated: sample-config-docker.yaml${NC}"
echo ""
echo "========================================="
echo "Test the configuration"
echo "========================================="
echo "docker compose up app"
