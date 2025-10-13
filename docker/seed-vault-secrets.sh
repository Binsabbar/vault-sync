#!/bin/bash

set -e

VAULT_ADDR="http://localhost:8200"
VAULT_SKIP_VERIFY=true  # ← Add this
VAULT_USERNAME="${VAULT_USERNAME}"
VAULT_PASSWORD="${VAULT_PASSWORD}"
VAULT_TOKEN="${VAULT_TOKEN}"

MOUNTS=("production" "uat" "stage")
SECRETS_PER_MOUNT=700

usage() {
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  With token:    $0 <vault-token>"
    echo "  With userpass: VAULT_USERNAME=user VAULT_PASSWORD=pass $0"
    echo ""
    echo "Examples:"
    echo "  $0 hvs.CAESIJ..."
    echo "  VAULT_USERNAME=admin VAULT_PASSWORD=secret $0"
    exit 1
}

# Login with userpass if credentials provided
if [ -n "$VAULT_USERNAME" ] && [ -n "$VAULT_PASSWORD" ]; then
    echo "Logging in with userpass..."
    
    # Set environment for vault CLI
    export VAULT_ADDR VAULT_SKIP_VERIFY
    
    VAULT_TOKEN=$(vault login -method=userpass \
        username="$VAULT_USERNAME" \
        password="$VAULT_PASSWORD" \
        -format=json | jq -r '.auth.client_token')
    
    if [ -z "$VAULT_TOKEN" ] || [ "$VAULT_TOKEN" = "null" ]; then
        echo "Error: Failed to login with userpass"
        exit 1
    fi
    echo "✓ Logged in successfully"
elif [ -n "$1" ]; then
    VAULT_TOKEN="$1"
elif [ -z "$VAULT_TOKEN" ]; then
    usage
fi

export VAULT_ADDR VAULT_TOKEN VAULT_SKIP_VERIFY

echo "Seeding Vault with secrets..."

for mount in "${MOUNTS[@]}"; do
    echo "Processing $mount..."
    for i in $(seq 1 $SECRETS_PER_MOUNT); do
        path="internal/app-$((i % 10))/secret-${i}"
        vault kv put "${mount}/${path}" \
            username="user${i}" \
            password="$(openssl rand -base64 24)" \
            api_key="$(uuidgen)" \
            timestamp="$(date -u +%s)" &>/dev/null
        
        [ $((i % 100)) -eq 0 ] && echo -ne "  $mount: $i/$SECRETS_PER_MOUNT\r"
    done
    echo -e "  $mount: ✓ Created $SECRETS_PER_MOUNT secrets"
done

echo "✓ Done! Created $((SECRETS_PER_MOUNT * 3)) secrets total"