# Changelog

## [0.0.1] 2025-10-20

### Added
- Initial vault-sync implementation
- Support for syncing secrets between Vault clusters
- Path-based filtering with glob pattern support
- Database tracking of synced secrets
- Circuit breaker pattern for resilience
- Comprehensive test suite with testcontainers
- Docker and Docker Compose support
- GitHub Actions CI/CD pipeline
- GoReleaser configuration for automated releases
- Makefile with development commands
- golangci-lint configuration for code quality
- Security vulnerability scanning with govulncheck
- Environment variable control for test logging (VAULT_SYNC_SILENT)

### Features
- **Multi-cluster sync**: Sync secrets from main Vault cluster to multiple replica clusters
- **Pattern matching**: Use glob patterns to include/exclude specific secret paths
- **Mount filtering**: Target specific secret engines for synchronization
- **Version tracking**: Track secret versions to avoid unnecessary syncs
- **Error handling**: Robust error handling with circuit breaker pattern
- **Database persistence**: PostgreSQL integration for tracking sync state
- **Docker support**: Full containerization with development environment
- **CLI interface**: Command-line tool with multiple subcommands

### Technical Details
- Written in Go 1.24
- Uses HashiCorp Vault Client Go SDK
- PostgreSQL with migrations support
- Zerolog for structured logging
- Testcontainers for integration testing
- GitHub Container Registry for Docker images

### Development Tools
- golangci-lint for code quality (errcheck, staticcheck, unused, ineffassign, govet, misspell)
- govulncheck for security vulnerability scanning
- GitHub Actions for CI/CD
- GoReleaser for automated releases
- Docker Compose for local development
- Comprehensive Makefile for development workflow

### Configuration
- YAML-based configuration
- Environment variable overrides
- Flexible sync rules with include/exclude patterns
- Multiple cluster support
- Database configuration options

### Known Issues
- Sync rule changes require manual cleanup of previously synced secrets
- Limited to KV v2 secret engines
- No built-in metrics endpoint

### Security Notes
- Vault tokens should be rotated regularly
- Database credentials should be managed securely
- Network traffic should be encrypted in production

---

For more details about any release, see the [GitHub Releases](https://github.com/Binsabbar/vault-sync/releases) page.