# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

Vault Sync is a production-ready Go application that synchronizes secrets across multiple HashiCorp Vault clusters. It maintains a main cluster (read-only) and multiple replica clusters (read-write), tracking sync state in PostgreSQL and using intelligent decision logic to minimize unnecessary operations.

## Essential Commands

### Building and Testing
```bash
# Build the binary
make go-build

# Run all tests (silent mode, clears cache first)
make go-test

# Run tests with verbose output
make go-test-verbose

# Run tests with coverage
make go-test-coverage

# Run linting (production code only, uses .golangci.yml)
make go-lint

# Check for security vulnerabilities
make go-vulncheck

# Format code with gofmt
make go-fmt
```

### Local Development with Docker
```bash
# Complete local setup (starts Vault clusters, PostgreSQL, initializes everything)
make setup

# Start dependency services only (postgres, vault1, vault2)
make docker-up-deps-services

# View dependency logs
make docker-deps-logs

# View app logs (requires jq)
make docker-app-logs

# Stop all services
make docker-down

# Stop all services and remove volumes
make docker-down-volume
```

### Running the Application
```bash
# One-time sync (recommended for production)
vault-sync sync once --config config.yaml

# Dry run to preview what would be synced
vault-sync sync dry-run --config config.yaml

# Test path patterns against a list of paths
vault-sync path-matcher --paths-file test-paths.txt --config config.yaml

# View current configuration
vault-sync config-print --config config.yaml
```

## Code Architecture

### Core Components

**Wiring Layer** (`internal/core/wiring.go`)
- Dependency injection container using singleton pattern
- Initializes and wires together all major components
- Creates Vault clients, database repositories, path matchers, and orchestrator
- Handles fatal initialization errors by calling `os.Exit(-1)`

**Orchestrator** (`internal/service/orchestrator/`)
- Top-level sync coordinator
- Discovers secrets from Vault, merges with database-tracked secrets
- Executes sync jobs in parallel with configurable concurrency
- Returns aggregated `SyncResult` with counts and job outcomes

**Sync Job** (`internal/service/job/sync_job.go`)
- Individual secret synchronization unit
- Implements decision logic: NoOp, Sync, or Delete
- Decision based on: source existence, DB records, replica records, version comparison
- Each job handles one mount/keyPath pair across all replica clusters

**Path Matching** (`internal/service/pathmatching/`)
- Discovers secrets in Vault that match configured patterns
- Uses doublestar glob patterns for include/exclude rules
- Mount-relative patterns (don't include mount names in patterns)
- Key rule: patterns without `/*` match exact depth only; `/*` enables nested matching; `/**` enables recursive

### Vault Integration

**Multi-Cluster Client** (`internal/vault/client.go`)
- Manages one main cluster and multiple replica clusters
- Main cluster: read-only operations (discovery, metadata)
- Replica clusters: read-write operations (sync, delete)
- Operations return per-replica results as `[]*models.SyncedSecret`

**Cluster Manager** (`internal/vault/cluster_manger.go`)
- Individual cluster operations (authentication, token management)
- AppRole-based authentication with automatic re-authentication on low TTL (<5 minutes)
- Handles mount validation and secret engine queries
- Uses HashiCorp's official vault-client-go SDK

**Replica Sync Handler** (`internal/vault/replica_sync_handler.go`)
- Generic handler for executing operations across all replicas in parallel
- Collects results and errors per replica
- Used for both sync and delete operations

### Database Layer

**Repository Pattern** (`internal/repository/`)
- Interface-based design for PostgreSQL operations
- Tracks synced secrets with: mount, path, cluster name, source version, timestamps
- PostgreSQL implementation in `internal/repository/postgres/`
- Migrations handled by `pkg/db/migrations/`

### Configuration

**Config Loading** (`internal/config/config.go`)
- YAML-based configuration with environment variable expansion
- Viper for config management with prefix `VAULT_SYNC_`
- Custom validators for sync intervals, path patterns, and cluster configs
- Supports multiple config locations: `.`, `/etc/vault-sync/`, `$HOME/.vault-sync`

## Testing Patterns

### Test Environment Setup
- Integration tests use testcontainers (PostgreSQL and Vault)
- Helper utilities in `testutil/` for port management, Vault seeding, PostgreSQL setup
- Test builders in `testutil/testbuilder/` for creating mock objects
- Silent mode controlled by `TEST_SILENT=1` environment variable

### Linting Configuration
- Very strict golangci-lint config (`.golangci.yml`)
- Excludes test files (`*_test.go`) and testutil directory from linting
- Excludes `cmd/*.go` and `main.go` from `gochecknoinits` and `gochecknoglobals`
- Production code must follow all enabled linters

## Key Design Patterns

### Sync Decision Logic
For each secret, the job evaluates:
1. **Source exists + no DB records** → Sync (new secret)
2. **Source exists + all replicas have records** → Check versions:
   - If outdated version → Sync
   - If replica secret missing (but record exists) → Sync
   - Otherwise → NoOp
3. **Source doesn't exist + DB records exist** → Delete
4. **Source doesn't exist + no DB records** → NoOp

### Concurrency Model
- Orchestrator runs sync jobs in parallel using goroutines
- Configurable semaphore limits concurrent operations
- Each job processes one mount/keyPath across all replicas
- Results aggregated via channels and wait groups

### Error Handling
- Errors during initialization (wiring) are fatal (`os.Exit(-1)`)
- Per-replica errors during sync are captured but don't fail entire operation
- Database errors during sync are propagated up
- Context cancellation is checked at job boundaries

## Path Pattern Behavior

**Critical distinctions:**
- `secret` → exact match only
- `secret/*` → single-level nested (includes all descendants, not exact match)
- `secret/**` → recursive nested (all descendants at any depth)
- `*app*` → contains "app" at root level only (no nested paths)
- `*app*/*` → contains "app" with nested paths

**Pattern evaluation:**
1. Mount must be in `kv_mounts`
2. Ignore patterns take precedence over replicate patterns
3. Empty replicate patterns → include all (except ignored)
4. Non-empty replicate patterns → include only matches (except ignored)

See `internal/service/pathmatching/README.md` for comprehensive pattern examples.

## Database Schema

**synced_secrets table:**
- `id` (serial primary key)
- `secret_backend` (mount name)
- `secret_path` (key path within mount)
- `cluster_name` (replica cluster name)
- `source_version` (version from main cluster)
- `created_at`, `updated_at` (timestamps)
- Unique constraint on `(secret_backend, secret_path, cluster_name)`

## Configuration Notes

- **Concurrency**: Defaults to 10 if not specified, max 100
- **Sync interval**: Min 60s, max 24h (only used for future daemon mode)
- **Mount names**: Must exist in all clusters before sync
- **AppRole mounts**: Default to "approle" if not specified
- **Environment variables**: All config values can be overridden with `VAULT_SYNC_` prefix

## Security Considerations

- Main cluster needs read+list permissions only
- Replica clusters need create+read+update+delete+list permissions
- AppRole credentials should be provided via environment variables
- TLS verification can be disabled for development only
- Never log or echo secrets in plain text

## Common Development Workflows

### Adding a New Command
1. Create command file in `cmd/<command>/`
2. Add to `cmd/root.go` init function
3. Follow existing patterns (sync, pathmatcher, configprint)

### Modifying Sync Logic
1. Update decision logic in `internal/service/job/sync_job.go`
2. Update tests in corresponding `*_test.go` files
3. Consider integration test coverage in `sync_job_integration_test.go`

### Adding Configuration Options
1. Add field to appropriate struct in `internal/config/config.go`
2. Add validation tags and custom validators if needed
3. Update `sample-config.yaml` with example

### Debugging Failed Syncs
1. Check logs for specific mount/keyPath combination
2. Use `vault-sync sync dry-run` to see discovered secrets
3. Verify mount exists in all clusters
4. Check AppRole permissions in each cluster
5. Query PostgreSQL for existing sync records
