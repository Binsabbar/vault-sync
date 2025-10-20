#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

VAULT_KEYS_FILE="./vault-keys.txt"
VAULT_CONFIG_FILE="./vault-config.env"

echo -e "${BLUE}=========================================${NC}"
echo -e "${BLUE}Initializing Vault Cluster${NC}"
echo -e "${BLUE}=========================================${NC}"
echo ""

# Clean up previous runs
rm -f "$VAULT_KEYS_FILE" "$VAULT_CONFIG_FILE"

echo -e "${YELLOW}1. Initializing Vault1 (Main)...${NC}"
./docker/init-single-vault.sh vault1

echo ""
echo -e "${YELLOW}2. Initializing Vault2 (Replica)...${NC}"
./docker/init-single-vault.sh vault2

echo ""
echo -e "${YELLOW}3. Setting up AppRole authentication...${NC}"

# Extract tokens from the vault keys file
VAULT1_ROOT_TOKEN=$(grep "vault1_ROOT_TOKEN=" "$VAULT_KEYS_FILE" | cut -d'=' -f2)
VAULT2_ROOT_TOKEN=$(grep "vault2_ROOT_TOKEN=" "$VAULT_KEYS_FILE" | cut -d'=' -f2)

# Setup AppRole for each vault with proper arguments
./docker/setup-approle.sh vault1 "$VAULT1_ROOT_TOKEN" ro
./docker/setup-approle.sh vault2 "$VAULT2_ROOT_TOKEN" rw

echo ""
echo -e "${YELLOW}4. Generating application configuration...${NC}"
./docker/config-generator.sh

echo ""
echo -e "${YELLOW}5. Seeding initial secrets...${NC}"
./docker/seed-initial-secrets.sh

echo ""
echo -e "${GREEN}=========================================${NC}"
echo -e "${GREEN}✓ Vault cluster initialization complete!${NC}"
echo -e "${GREEN}=========================================${NC}"
echo ""
echo -e "${BLUE}Generated files:${NC}"
echo -e "  ${VAULT_KEYS_FILE}    - Vault keys and tokens"
echo -e "  ${VAULT_CONFIG_FILE}  - Environment variables"
echo -e "  sample-config-docker.yaml - App configuration"
echo ""
echo -e "${BLUE}Next steps:${NC}"
echo -e "  make up     - Start all services"
echo -e "  make logs   - View application logs"