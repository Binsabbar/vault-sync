#/bin/bash

# Init and unseal vault1
VAULT_INIT_OUTPUT=$(docker compose exec vault1 sh -c "vault operator init -key-shares=1 -key-threshold=1 -format=yaml")
VAULT_UNSEAL_KEY=$(echo "$VAULT_INIT_OUTPUT" | grep 'unseal_keys_b64:' -A 1 | tail -n 1 | awk '{print $2}')
VAULT_ROOT_TOKEN=$(echo "$VAULT_INIT_OUTPUT" | grep 'root_token:' | awk '{print $2}')
docker compose exec vault1 vault operator unseal $VAULT_UNSEAL_KEY

# Login to vault1
docker compose exec vault1 vault login $VAULT_ROOT_TOKEN
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
docker compose exec vault2 vault operator unseal $VAULT_UNSEAL_KEY

# Login to vault2
docker compose exec vault2 vault login $VAULT_ROOT_TOKEN
docker compose exec vault2 vault auth enable userpass
docker compose exec vault2 sh -c "echo '
path \"*\" {
  capabilities = [\"create\", \"read\", \"update\", \"delete\", \"list\", \"sudo\"]
}
' > /vault/file/admin-policy.hcl"
docker compose exec vault2 vault policy write admin /vault/file/admin-policy.hcl
docker compose exec vault2 vault write auth/userpass/users/admin password=admin policies=admin