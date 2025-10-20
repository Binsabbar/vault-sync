# Release Process

This document outlines the release process for vault-sync.

## 🎯 Release Method

**There is ONE way to release: Pull Request → `releases/*` branch**

All releases are created through Pull Requests targeting `releases/*` branches. Direct pushes to `releases/*` branches are **blocked** by branch protection rules.

## Release Types

### Major Releases (x.0.0)
- Breaking changes
- Major new features
- Architectural changes
- Example: `releases/v1.0.0`

### Minor Releases (0.x.0)
- New features
- Enhancements
- Non-breaking changes
- Example: `releases/v0.1.0`

### Patch Releases (0.0.x)
- Bug fixes
- Security fixes
- Minor improvements
- Example: `releases/v0.1.1`

### Pre-release Versions
- **Release Candidates**: `releases/v0.1.0-rc.1`
- **Beta Releases**: `releases/v0.1.0-beta.1`
- **Alpha Releases**: `releases/v0.1.0-alpha.1`

## Release Workflow

### 1. Pre-Release Checklist

- [ ] All tests pass (`make go-test`)
- [ ] Code quality checks pass (`make go-lint`)
- [ ] Security vulnerabilities checked (`make go-vulncheck`)
- [ ] Documentation updated
- [ ] CHANGELOG.md updated with new version
- [ ] Version bumped in relevant files (if applicable)

### 2. Release Preparation (Feature Branch)

```bash
# 1. Create feature branch for release preparation
git checkout main
git checkout -b feature/prepare-v0.1.0

# 2. Update CHANGELOG.md
cat >> CHANGELOG.md << 'EOF'
## [0.1.0] - 2025-10-20

### Added
- New feature X
- New feature Y

### Fixed
- Bug fix Z

### Changed
- Updated dependency A
EOF

# 3. Run final tests locally (optional but recommended)
make go-test
make go-lint
make go-vulncheck

# 4. Commit changes
git add CHANGELOG.md
git commit -m "chore: prepare release v0.1.0"

# 5. Push feature branch
git push origin feature/prepare-v0.1.0
```

### 3. Create Pull Request

```bash
# Create PR targeting the release branch (which may not exist yet)
gh pr create \
  --base releases/v0.1.0 \
  --head feature/prepare-v0.1.0 \
  --title "Release v0.1.0" \
  --body "Preparing release v0.1.0

## Checklist
- [x] CHANGELOG.md updated
- [x] Documentation updated
- [x] Tests pass locally

## Changes
See CHANGELOG.md for details.
"

# GitHub will automatically create the releases/v0.1.0 branch if it doesn't exist
```

**What happens when PR is created:**
- ✅ Automated tests run (`test.yaml`)
- ✅ Code linting runs (`golangci-lint.yaml`)
- ✅ Status checks appear on the PR
- ✅ Review process (if required by branch protection)

### 4. Merge Pull Request

Once all checks pass and approvals are obtained:

```bash
# Option 1: Merge via GitHub CLI
gh pr merge --merge --delete-branch

# Option 2: Merge via GitHub UI
# - Go to the PR page
# - Click "Merge pull request"
# - Confirm merge
```

**What happens automatically when PR is merged:**
1. ✅ Tests re-run on merged commit
2. ✅ Linting re-runs on merged commit
3. ✅ GoReleaser builds binaries for all platforms
4. ✅ Git tag created automatically (e.g., `v0.1.0`)
5. ✅ GitHub Release created with CHANGELOG
6. ✅ Docker images built and pushed to GHCR
   - `ghcr.io/binsabbar/vault-sync:v0.1.0`
   - `ghcr.io/binsabbar/vault-sync:latest` (stable releases only)
7. ✅ Checksums generated for all artifacts

⏱️ **Total time**: ~8 minutes from merge to release

### 5. Post-Release Verification

- [ ] Verify GitHub release was created: `gh release view v0.1.0`
- [ ] Verify Docker images were pushed to GHCR
- [ ] Test Docker image: `docker run --rm ghcr.io/binsabbar/vault-sync:v0.1.0 version`
- [ ] Download and test a binary from GitHub releases
- [ ] Update documentation if needed
- [ ] Announce release (if applicable)

## Automated Release Pipeline

The project uses GitHub Actions and GoReleaser for fully automated releases triggered by **Pull Request merges**.

### Triggered by:
- ✅ **Pull Request merged** into `releases/v*` branches
- ✅ Manual workflow dispatch (for emergency releases)

### Actions performed automatically:
1. **Test**: Run full test suite with govulncheck
2. **Lint**: Run golangci-lint code quality checks
3. **Build**: Cross-platform binary compilation (Linux, macOS, Windows - amd64/arm64)
4. **Tag**: Automatically create Git tag from branch name (e.g., `releases/v0.1.0` → tag `v0.1.0`)
5. **Package**: Create archives and checksums
6. **Docker**: Build and push multi-arch images to GHCR
7. **Release**: Create GitHub release with changelog and artifacts

### Artifacts Generated:
- **Binaries**: 
  - Linux: amd64, arm64
  - macOS: amd64, arm64
  - Windows: amd64, arm64
- **Archives**: tar.gz (Linux/macOS) and zip (Windows) formats
- **Docker Images**: Multi-arch images pushed to GHCR
  - Tagged with version: `ghcr.io/binsabbar/vault-sync:v0.1.0`
  - Tagged as latest (stable only): `ghcr.io/binsabbar/vault-sync:latest`
- **Checksums**: SHA256 checksums for all artifacts
- **Release Notes**: Automatically extracted from CHANGELOG.md

## Release Channels

### Stable Releases (Production)
- **Created by**: PR merged to `releases/v*.*.*` branches
- **Git tags**: `v1.0.0`, `v1.1.0`, etc.
- **GitHub Releases**: Full releases with changelog and artifacts
- **Docker tags**: 
  - Version-specific: `ghcr.io/binsabbar/vault-sync:v1.0.0`
  - Latest: `ghcr.io/binsabbar/vault-sync:latest`

### Pre-releases (Testing)
- **Created by**: PR merged to `releases/v*.*.*-rc.*`, `-beta.*`, or `-alpha.*` branches
- **Git tags**: `v1.0.0-rc.1`, `v1.0.0-beta.1`, `v1.0.0-alpha.1`
- **GitHub Releases**: Marked as "pre-release"
- **Docker tags**: Version-specific only (e.g., `ghcr.io/binsabbar/vault-sync:v1.0.0-rc.1`)
- **Note**: Does NOT update `:latest` tag

### Development Snapshots
- **Created by**: Pushes to `v*.*.*-dev` branches
- **Purpose**: Testing and development
- **Artifacts**: Available in GitHub Actions workflow artifacts
- **Docker**: Not pushed to registry
- **GitHub Releases**: Not created

## Version Strategy

We follow [Semantic Versioning](https://semver.org/):

```
MAJOR.MINOR.PATCH[-PRERELEASE]
```

### Examples:
- **Stable**: `v1.0.0`, `v1.2.3`, `v2.0.0`
- **Release Candidate**: `v1.0.0-rc.1`, `v1.0.0-rc.2`
- **Beta**: `v1.0.0-beta.1`, `v1.0.0-beta.2`
- **Alpha**: `v1.0.0-alpha.1`, `v1.0.0-alpha.2`
- **Development**: `v1.0.0-dev` (snapshot builds only)

### Version Selection Guide:
- **MAJOR** (x.0.0): Breaking changes, major rewrites
- **MINOR** (0.x.0): New features, enhancements (backward compatible)
- **PATCH** (0.0.x): Bug fixes, security patches (backward compatible)

## Pre-release Process

For testing releases before stable:

```bash
# 1. Create feature branch
git checkout -b feature/prepare-v1.0.0-rc.1

# 2. Update CHANGELOG.md
cat >> CHANGELOG.md << 'EOF'
## [1.0.0-rc.1] - 2025-10-20

### Added (Release Candidate)
- Feature X for testing
- Feature Y for validation
EOF

git add CHANGELOG.md
git commit -m "chore: prepare release v1.0.0-rc.1"
git push origin feature/prepare-v1.0.0-rc.1

# 3. Create PR targeting releases/v1.0.0-rc.1
gh pr create \
  --base releases/v1.0.0-rc.1 \
  --head feature/prepare-v1.0.0-rc.1 \
  --title "Release v1.0.0-rc.1 (Pre-release)" \
  --body "Release candidate for testing"

# 4. Merge after tests pass
gh pr merge --merge --delete-branch

# ✅ Automated result:
#    - Tag: v1.0.0-rc.1
#    - GitHub pre-release created
#    - Docker: ghcr.io/binsabbar/vault-sync:v1.0.0-rc.1
#    - :latest tag NOT updated (only stable releases update it)
```

## Manual Release Steps (Emergency Only)

If automated release fails, use manual workflow dispatch:

```bash
# Option 1: GitHub CLI
gh workflow run release.yaml \
  -f branch=releases/v0.1.0 \
  -f pr_number=123

# Option 2: GitHub UI
# 1. Go to Actions → release workflow
# 2. Click "Run workflow"
# 3. Enter branch name (e.g., releases/v0.1.0)
# 4. Enter PR number (optional, for context)
# 5. Click "Run workflow"
```

**Note**: Manual releases should be rare. The PR-based workflow is the standard method.

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
git checkout -b hotfix/critical-security-fix

# 2. Apply fix and test
# ... make changes ...
make go-test
make go-lint

# 3. Update CHANGELOG.md
cat >> CHANGELOG.md << 'EOF'
## [0.1.1] - 2025-10-20

### Security
- Fixed critical vulnerability CVE-XXXX-YYYY
EOF

git add .
git commit -m "fix: critical security issue CVE-XXXX-YYYY"
git push origin hotfix/critical-security-fix

# 4. Create PR targeting releases/v0.1.1 (HOTFIX VERSION)
gh pr create \
  --base releases/v0.1.1 \
  --head hotfix/critical-security-fix \
  --title "HOTFIX: Release v0.1.1 - Security Fix" \
  --body "**CRITICAL SECURITY FIX**

## Issue
CVE-XXXX-YYYY allows unauthorized access

## Fix
Applied security patch from upstream

## Testing
- [x] Security vulnerability resolved
- [x] All tests pass
- [x] No regressions introduced
"

# 5. Request expedited review and merge
gh pr merge --merge --delete-branch

# ✅ Automated hotfix release within minutes!
```

**Hotfix Best Practices:**
- ⚠️ Use only for critical issues (security, data loss, severe bugs)
- ✅ Keep changes minimal and focused
- ✅ Test thoroughly despite urgency
- ✅ Document the issue and fix clearly
- ✅ Follow up with communication to users

## Rollback Process

If a release needs to be rolled back:

### 1. Remove Git Tag

```bash
# Delete local tag
git tag -d v0.1.0

# Delete remote tag
git push origin :refs/tags/v0.1.0
```

### 2. Delete GitHub Release

```bash
# Option 1: GitHub CLI
gh release delete v0.1.0 --yes

# Option 2: GitHub UI
# - Go to repository releases page
# - Find the problematic release
# - Click "Delete"
```

### 3. Remove Docker Images

```bash
# Docker images in GHCR cannot be easily deleted
# Best practice: Push a new fixed version and update documentation
# Contact GitHub support if image must be removed
```

### 4. Create Rollback/Fix Release

Instead of just removing, create a new fixed version:

```bash
# 1. Create hotfix branch
git checkout -b hotfix/fix-v0.1.0-issue

# 2. Revert problematic changes or apply fix
git revert <bad-commit-sha>
# OR apply specific fix

# 3. Update CHANGELOG.md
cat >> CHANGELOG.md << 'EOF'
## [0.1.1] - 2025-10-20

### Fixed
- Reverted problematic change from v0.1.0
- Fixed issue that caused rollback
EOF

git add .
git commit -m "fix: revert problematic changes from v0.1.0"
git push origin hotfix/fix-v0.1.0-issue

# 4. Create PR and release v0.1.1
gh pr create \
  --base releases/v0.1.1 \
  --head hotfix/fix-v0.1.0-issue \
  --title "Fix issues from v0.1.0" \
  --body "Rollback/fix for v0.1.0 issues"

gh pr merge --merge --delete-branch
```

### 5. Communicate Rollback

- ✅ Update CHANGELOG.md with rollback notice
- ✅ Update README.md if needed
- ✅ Post announcement (GitHub Discussions, issues, etc.)
- ✅ Notify users via appropriate channels

**Example Announcement:**
```markdown
# ⚠️ Release v0.1.0 Rolled Back

**Issue**: Critical bug discovered in v0.1.0 causing [description]

**Action**: 
- v0.1.0 release removed
- Fixed version v0.1.1 released

**Users should**:
- Upgrade to v0.1.1 immediately
- Avoid using v0.1.0

**Details**: See #123 for full details
```

---

## Branch Protection Setup (Required!)

To enforce PR-based releases and prevent accidental direct pushes:

### Required Branch Protection Rules

1. **Go to**: Repository Settings → Branches → Add rule

2. **Pattern**: `releases/*`

3. **Configure**:
   - ✅ **Require a pull request before merging**
     - Require approvals: 1+ (recommended)
     - Dismiss stale approvals when new commits pushed
   - ✅ **Require status checks to pass before merging**
     - Required checks: `test`, `golangci-lint`
     - Require branches to be up to date before merging
   - ✅ **Require conversation resolution before merging**
   - ✅ **Do not allow bypassing settings** (or allow admins only)
   - ❌ **Allow force pushes**: DISABLED
   - ❌ **Allow deletions**: DISABLED

4. **Save changes**

### Result of Branch Protection

- ✅ All `releases/*` changes must go through PRs
- ✅ Tests must pass before merge
- ✅ Full audit trail of all releases
- ❌ No direct pushes to `releases/*` (even for admins, unless bypass enabled)
- ❌ No accidental releases
- 🎉 **Single, controlled release method!**

---

## Quick Reference

### Create Stable Release
```bash
git checkout -b feature/prepare-v1.0.0
# Update CHANGELOG.md
git commit -m "chore: prepare release v1.0.0"
git push origin feature/prepare-v1.0.0
gh pr create --base releases/v1.0.0 --head feature/prepare-v1.0.0
gh pr merge --merge --delete-branch
```

### Create Pre-release
```bash
git checkout -b feature/prepare-v1.0.0-rc.1
# Update CHANGELOG.md
git commit -m "chore: prepare release v1.0.0-rc.1"
git push origin feature/prepare-v1.0.0-rc.1
gh pr create --base releases/v1.0.0-rc.1 --head feature/prepare-v1.0.0-rc.1
gh pr merge --merge --delete-branch
```

### Create Hotfix
```bash
git checkout -b hotfix/critical-fix
# Apply fix and update CHANGELOG.md
git commit -m "fix: critical issue"
git push origin hotfix/critical-fix
gh pr create --base releases/v1.0.1 --head hotfix/critical-fix --title "HOTFIX: v1.0.1"
gh pr merge --merge --delete-branch
```

### Check Release Status
```bash
gh release list
gh release view v1.0.0
docker pull ghcr.io/binsabbar/vault-sync:v1.0.0
docker run --rm ghcr.io/binsabbar/vault-sync:v1.0.0 version
```

---

## Troubleshooting

### PR Tests Fail
**Problem**: Tests or lint fail on PR to `releases/*` branch

**Solution**:
1. Check test/lint output in PR checks
2. Fix issues in your feature branch
3. Push fixes: `git push origin feature/prepare-v1.0.0`
4. Tests re-run automatically

### Release Workflow Fails
**Problem**: Release workflow fails after PR merge

**Solution**:
1. Check workflow logs in Actions tab
2. Identify failure point (build, GoReleaser, Docker push)
3. Fix issue and trigger manual release:
   ```bash
   gh workflow run release.yaml -f branch=releases/v1.0.0 -f pr_number=<PR#>
   ```

### Docker Push Fails
**Problem**: Docker images fail to push to GHCR

**Solution**:
1. Verify GITHUB_TOKEN has packages:write permission
2. Check GHCR authentication in workflow logs
3. Manually push if needed:
   ```bash
   docker login ghcr.io -u $GITHUB_ACTOR -p $GITHUB_TOKEN
   make docker-build-and-push VERSION=v1.0.0
   ```

### Wrong Version Tagged
**Problem**: Tag created with wrong version number

**Solution**:
1. Delete the tag:
   ```bash
   git tag -d v1.0.0
   git push origin :refs/tags/v1.0.0
   gh release delete v1.0.0 --yes
   ```
2. Fix branch name (releases/* branch must match desired version)
3. Re-merge PR or trigger manual release with correct branch

### Can't Push to Release Branch
**Problem**: `git push origin releases/v1.0.0` fails with "protected branch"

**Solution**:
This is **expected behavior**! You cannot push directly to `releases/*` branches. You must:
1. Push to a feature branch
2. Create a PR targeting the `releases/*` branch
3. Merge the PR

This is the intended workflow to ensure quality and audit trail.

---

## Contact

For questions about the release process:
- Create an issue in the repository
- Review `.github/WORKFLOWS.md` for detailed workflow documentation
- Contact the maintainers

## Resources

- [Semantic Versioning](https://semver.org/)
- [Keep a Changelog](https://keepachangelog.com/)
- [GoReleaser Documentation](https://goreleaser.com/)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [GitHub Branch Protection](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches)
- [softprops/action-gh-release](https://github.com/softprops/action-gh-release)