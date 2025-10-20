# Release Process

This document outlines the release process for vault-sync.

## Release Types

### Major Releases (x.0.0)
- Breaking changes
- Major new features
- Architectural changes

### Minor Releases (0.x.0)
- New features
- Enhancements
- Non-breaking changes

### Patch Releases (0.0.x)
- Bug fixes
- Security fixes
- Minor improvements

## Release Workflow

### 1. Pre-Release Checklist

- [ ] All tests pass (`make go-test`)
- [ ] Code quality checks pass (`make go-lint`)
- [ ] Security vulnerabilities checked (`make go-vulncheck`)
- [ ] Documentation updated
- [ ] CHANGELOG.md updated with new version
- [ ] Version bumped in relevant files

### 2. Release Preparation

```bash
# 1. Create release branch
git checkout -b release/v0.1.0

# 2. Update version in files
# - Update CHANGELOG.md
# - Update version references in documentation

# 3. Run final tests
make go-test
make go-lint
make go-vulncheck

# 4. Commit changes
git add .
git commit -m "chore: prepare release v0.1.0"

# 5. Push release branch
git push origin release/v0.1.0
```

### 3. Create Release

```bash
# 1. Merge to main
git checkout main
git merge release/v0.1.0

# 2. Create and push tag
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0

# 3. GoReleaser will automatically:
#    - Build binaries for multiple platforms
#    - Create GitHub release
#    - Build and push Docker images
#    - Generate checksums
```

### 4. Post-Release

- [ ] Verify GitHub release was created
- [ ] Verify Docker images were pushed to GHCR
- [ ] Update documentation if needed
- [ ] Announce release (if applicable)

## Automated Release Pipeline

The project uses GitHub Actions and GoReleaser for automated releases:

### Triggered by:
- Git tags matching `v*.*.*` pattern
- Manual workflow dispatch

### Actions performed:
1. **Build**: Cross-platform binary compilation
2. **Test**: Run full test suite
3. **Security**: Vulnerability scanning
4. **Package**: Create archives and checksums
5. **Docker**: Build and push multi-arch images
6. **Release**: Create GitHub release with artifacts

### Artifacts Generated:
- **Binaries**: Linux (amd64, arm64), macOS (amd64, arm64)
- **Archives**: tar.gz and zip formats
- **Docker Images**: Multi-arch images pushed to GHCR
- **Checksums**: SHA256 checksums for all artifacts

## Release Channels

### Stable Releases
- Tagged releases (v1.0.0, v1.1.0, etc.)
- Full GitHub releases with changelog
- Docker images tagged with version and 'latest'

### Development Snapshots
- Built from main branch
- Available via snapshot workflow
- Docker images tagged with commit SHA
- Not published as GitHub releases

## Version Strategy

We follow [Semantic Versioning](https://semver.org/):

```
MAJOR.MINOR.PATCH
```

### Pre-release versions:
- `v0.1.0-rc.1` - Release candidate
- `v0.1.0-beta.1` - Beta release
- `v0.1.0-alpha.1` - Alpha release

### Development versions:
- `v0.1.0-dev` - Development branch
- `SHA-devel` - Snapshot builds

## Manual Release Steps

If automated release fails, manual steps:

```bash
# 1. Build snapshot locally
make release-snapshot

# 2. Upload artifacts manually to GitHub release
# 3. Build and push Docker images manually
docker build -t ghcr.io/binsabbar/vault-sync:v0.1.0 .
docker push ghcr.io/binsabbar/vault-sync:v0.1.0
```

## Release Validation

After each release:

1. **Download and test binaries**
   ```bash
   # Download from GitHub releases
   curl -L https://github.com/Binsabbar/vault-sync/releases/download/v0.1.0/vault-sync_0.1.0_linux_amd64.tar.gz
   tar -xzf vault-sync_0.1.0_linux_amd64.tar.gz
   ./vault-sync version
   ```

2. **Test Docker image**
   ```bash
   docker run --rm ghcr.io/binsabbar/vault-sync:v0.1.0 version
   ```

3. **Verify checksums**
   ```bash
   # Download checksums.txt and verify
   sha256sum -c checksums.txt
   ```

## Hotfix Process

For critical fixes that need immediate release:

```bash
# 1. Create hotfix branch from main
git checkout main
git checkout -b hotfix/v0.1.1

# 2. Apply fix
# 3. Update CHANGELOG.md
# 4. Commit and push
git commit -m "fix: critical security issue"
git push origin hotfix/v0.1.1

# 5. Create tag and release immediately
git tag v0.1.1
git push origin v0.1.1
```

## Rollback Process

If a release needs to be rolled back:

1. **Remove Git tag**
   ```bash
   git tag -d v0.1.0
   git push origin :refs/tags/v0.1.0
   ```

2. **Delete GitHub release**
   - Go to GitHub releases page
   - Delete the problematic release

3. **Remove Docker images** (if needed)
   ```bash
   # Contact GHCR support or use registry API
   ```

4. **Communicate rollback**
   - Update CHANGELOG.md
   - Notify users if applicable

---

## Contact

For questions about the release process:
- Create an issue in the repository
- Contact the maintainers

## Resources

- [Semantic Versioning](https://semver.org/)
- [Keep a Changelog](https://keepachangelog.com/)
- [GoReleaser Documentation](https://goreleaser.com/)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)