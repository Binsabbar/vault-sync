# GitHub Workflows Summary

This repository has the following GitHub Actions workflows that use **Makefile targets** for consistency:

## 1. Test Workflow (`test.yaml`)
**Triggers:**
- ✅ Push to any branch (except `main`/`master`)
- ✅ Pull requests to version branches, main, or master
- ✅ Manual trigger (workflow_dispatch)

**Jobs using Makefile targets:**
- Setup Go environment and dependencies
- **Code formatting check**: `make go-fmt-check`
- Go vet analysis: `go vet ./...`
- **Run tests**: `make go-test`
- Vulnerability scanning with `govulncheck`

## 2. Lint Workflow (`golangci-lint.yaml`)
**Triggers:**
- ✅ Push to any branch (except `main`/`master`)
- ✅ Pull requests to version branches, main, or master
- ✅ Manual trigger (workflow_dispatch)

**Jobs using Makefile targets:**
- **Run golangci-lint**: `make go-lint`
- Code quality and style checks on Ubuntu and macOS

## 3. Build and Push Workflow (`build-and-push.yaml`)
**Triggers:**
- ✅ Push to version branches with format `v[0-9]+.[0-9]+.[0-9]+` (e.g., v1.2.3)

**Jobs using Makefile targets:**
- Run tests and linting first
- **Build and release**: `make release` (uses GoReleaser)
- Builds Go binary with version info
- Builds multi-platform Docker images (linux/amd64, linux/arm64)
- Pushes to GitHub Container Registry (ghcr.io)

## 4. Snapshot Workflow (`snapshot.yaml`)
**Triggers:**
- ✅ Push to development branches with format `v[0-9]+.[0-9]+.[0-9]+-dev` (e.g., v1.2.3-dev)
- ✅ Manual trigger (workflow_dispatch)

**Jobs using Makefile targets:**
- Run tests and linting
- **Build snapshot**: `make release-snapshot` (uses GoReleaser)

## Makefile Targets Used in CI

| Makefile Target         | Usage                 | Description                                    |
| ----------------------- | --------------------- | ---------------------------------------------- |
| `make go-fmt-check`     | Test workflow         | Check if code is properly formatted            |
| `make go-test`          | Test workflow         | Run all tests with race detection              |
| `make go-lint`          | Lint workflow         | Run golangci-lint for code quality             |
| `make release`          | Build & Push workflow | Full release with GoReleaser (binary + Docker) |
| `make release-snapshot` | Snapshot workflow     | Snapshot build with GoReleaser                 |

## Branch Behavior Summary

| Branch Type        | Test | Lint | Build Binary | Build & Push Docker |
| ------------------ | ---- | ---- | ------------ | ------------------- |
| `main`/`master`    | ❌    | ❌    | ❌            | ❌                   |
| `v1.2.3` (version) | ✅    | ✅    | ✅            | ✅                   |
| `v1.2.3-dev`       | ✅    | ✅    | ✅ (snapshot) | ❌                   |
| `feature/xyz`      | ✅    | ✅    | ❌            | ❌                   |
| `fix/abc`          | ✅    | ✅    | ❌            | ❌                   |
| Any other branch   | ✅    | ✅    | ❌            | ❌                   |

## Docker Images

Version branches will push to:
- `ghcr.io/binsabbar/vault-sync:v1.2.3` (version tag)
- `ghcr.io/binsabbar/vault-sync:latest` (latest tag)

## Benefits of Using Makefile Targets

✅ **Consistency**: Same commands work locally and in CI  
✅ **Maintainability**: Changes to build process only need to be made in Makefile  
✅ **Simplicity**: Developers can run the same commands locally  
✅ **Standardization**: Common interface across all environments  

## Usage Examples

1. **Local development**: `make go-test` (same as CI)
2. **Local formatting**: `make go-fmt-check` (same as CI)
3. **Local linting**: `make go-lint` (same as CI)
4. **Local release**: `make release` (same as CI)
5. **Version release**: Push to `v1.2.3` → uses `make release`