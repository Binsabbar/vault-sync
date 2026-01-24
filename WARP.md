# Warp Context: vault-sync
This repo builds `vault-sync`, a Go CLI for synchronizing secrets from a main (read-only) HashiCorp Vault cluster to one or more replica (read-write) clusters, with sync state tracked in PostgreSQL.

## Tech stack
- Language: Go (module: `vault-sync`)
- CLI framework: Cobra
- Local dev: Docker Compose (Vault + Postgres)

## Key repo directories
- `cmd/`: Cobra command implementations
- `internal/`: internal application code (services, sync logic)
- `pkg/`: exported packages (if any)
- `docker/`: local dev scripts (init/config generation/seed)

## Common commands
Prefer Make targets (they encode repo conventions).

### Build
- `make go-build` (outputs `bin/vault-sync`)

### Test
- `make go-test`
- `make go-test-coverage`

### Lint / format
- `make go-lint`
- `make go-fmt`
- `make go-fmt-check`

### Local environment (Docker)
- `make setup` (starts deps, initializes Vault, generates config, seeds secrets)
- `make docker-up-deps-services`
- `make docker-up-all`
- `make docker-down`

## How to run the CLI
- One-time sync: `vault-sync sync once --config config.yaml`
- Dry-run sync: `vault-sync sync dry-run --config config.yaml`
- Print config: `vault-sync config-print --config config.yaml`

## Notes / conventions
- Path patterns in config are mount-relative; do not include the mount name in `paths_to_replicate` / `paths_to_ignore`.
- When changing sync behavior, update/extend tests and run `make go-test`.
