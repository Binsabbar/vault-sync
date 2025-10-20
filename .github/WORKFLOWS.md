# GitHub Workflows Summary

This repository uses GitHub Actions workflows with **Makefile targets** for consistency between local development and CI/CD.

## 🎯 Release Method

**There is ONE way to release: Pull Request → `releases/*` branch**

- ✅ All `releases/*` branches are **protected** (no direct pushes)
- ✅ Changes must go through **Pull Requests**
- ✅ Tests and lint **must pass** before merge
- ✅ Upon PR merge: Automatically creates **tag**, **GitHub Release**, and **Docker images**
- 🎉 **Zero manual work** - just merge the PR!

See [Complete Release Process](#complete-release-process) for step-by-step guide.

---

## Workflow Overview

### 1. Test Workflow (`test.yaml`) 🧪
**Purpose:** Run comprehensive test suite for quality assurance

**Triggers:**
- ✅ Push to any branch (except `main`/`master`)
- ✅ Pull requests to `main`, `master`, or `releases/**` branches
- ✅ Manual trigger (`workflow_dispatch`)
- ✅ Called by other workflows (`workflow_call`)

**Path Filters:**
- Runs only when Go code, workflows, or dependencies change
- Paths: `.github/workflows/**`, `**/*.go`, `**/*.mod`, `**/*.sum`

**Jobs:**
1. **setup**: Install Go and download dependencies
2. **vet**: Run `go vet` static analysis
3. **test**: Run full test suite with `make go-test` (30-minute timeout)
4. **govulncheck**: Scan for security vulnerabilities

**Environment Variables:**
- `TEST_SILENT=1`: Suppress verbose test output

---

### 2. Lint Workflow (`golangci-lint.yaml`) 🔍
**Purpose:** Enforce code quality standards

**Triggers:**
- ✅ Push to any branch (except `main`/`master`)
- ✅ Pull requests to `main`, `master`, or `releases/**` branches
- ✅ Manual trigger (`workflow_dispatch`)
- ✅ Called by other workflows (`workflow_call`)

**Path Filters:**
- Same as test workflow

**Jobs:**
1. **golangci**: Run `make go-lint` with golangci-lint configuration

**Linters Enabled:**
- errcheck, staticcheck, unused, ineffassign, govet, misspell
- Configured via `.golangci.yml`

---

### 3. Snapshot Workflow (`snapshot.yaml`) 📸
**Purpose:** Build development snapshots without releasing

**Triggers:**
- ✅ Push to development branches: `v[0-9]+.[0-9]+.[0-9]+-dev` (e.g., `v1.2.3-dev`)
- ✅ Manual trigger (`workflow_dispatch`)

**Jobs:**
1. **test**: Runs full test suite (via `workflow_call`)
2. **lint**: Runs linting (via `workflow_call`)
3. **snapshot**: Builds binaries with `make release-snapshot`

**Output:**
- Multi-platform binaries (Linux, macOS, Windows)
- ❌ No Docker images pushed
- ❌ No GitHub releases created
- ✅ Artifacts available in workflow run

---

### 4. Release Workflow (`release.yaml`) 🚀
**Purpose:** Create official releases with binaries and Docker images via controlled PR process

**Triggers:**
- ✅ **Pull Request merged** into release branches:
  - `releases/v[0-9]+.[0-9]+.[0-9]+` (e.g., `releases/v1.2.3`)
  - `releases/v[0-9]+.[0-9]+.[0-9]+-rc.[0-9]+` (e.g., `releases/v1.2.3-rc.1`)
  - `releases/v[0-9]+.[0-9]+.[0-9]+-beta.[0-9]+` (e.g., `releases/v1.2.3-beta.1`)
  - `releases/v[0-9]+.[0-9]+.[0-9]+-alpha.[0-9]+` (e.g., `releases/v1.2.3-alpha.1`)
- ✅ Manual trigger with branch name (emergency use)

**Important:** Direct pushes to `releases/*` branches should be **disabled via branch protection** to enforce PR-based releases.

**Jobs:**
1. **test**: Runs full test suite on merge commit (via `workflow_call`)
2. **lint**: Runs linting on merge commit (via `workflow_call`)
3. **release**: After tests pass, automatically:
   - Extracts version from target branch name (e.g., `releases/v1.2.3` → `v1.2.3`)
   - Runs GoReleaser to build multi-platform binaries
   - Extracts changelog from `CHANGELOG.md`
   - **Creates Git tag automatically** (e.g., `v1.2.3`)
   - **Creates GitHub Release** with changelog and artifacts
   - Marks as pre-release for rc/beta/alpha versions
   - Builds and pushes Docker images to GHCR
   - Attaches all artifacts (binaries, checksums)

**Outputs:**
- ✅ **Git tag created automatically** (no manual tag creation!)
- ✅ GitHub Release with changelog and PR context
- ✅ Multi-platform binaries (Linux, macOS, Windows - amd64/arm64)
- ✅ Docker images: `ghcr.io/binsabbar/vault-sync:vX.Y.Z`
- ✅ Docker `:latest` tag (for stable releases only, not pre-releases)
- ✅ Checksums for verification

**Key Features:**
- 🔒 **PR-only releases**: Enforces code review before release
- 🎯 **Single release method**: One clear path to production
- 🔀 **Merge triggers release**: No manual tag creation needed
- 📝 **Audit trail**: Every release has associated PR
- 🛡️ **Protected branches**: Direct pushes blocked

---

## Makefile Targets Used in CI

| Makefile Target              | Workflow      | Description                          |
| ---------------------------- | ------------- | ------------------------------------ |
| `make go-test`               | test.yaml     | Run all tests with race detection    |
| `make go-lint`               | golangci-lint | Run golangci-lint for code quality   |
| `make release-snapshot`      | snapshot.yaml | Build snapshot without releasing     |
| `make docker-build-and-push` | release.yaml  | Build and push Docker images to GHCR |

---

## Branch & Tag Behavior Matrix

| Event Type                    | Test | Lint | Build Snapshot | Create Release | Push Docker | Create Tag | Notes                          |
| ----------------------------- | ---- | ---- | -------------- | -------------- | ----------- | ---------- | ------------------------------ |
| `feature/*` branch            | ✅    | ✅    | ❌              | ❌              | ❌           | ❌          | Development feedback           |
| `fix/*` branch                | ✅    | ✅    | ❌              | ❌              | ❌           | ❌          | Development feedback           |
| Push to `releases/v*`         | ❌    | ❌    | ❌              | ❌              | ❌           | ❌          | **BLOCKED** (branch protected) |
| **PR → `releases/v*`**        | ✅¹   | ✅¹   | ❌              | ❌              | ❌           | ❌          | Tests run before merge         |
| **PR merged → `releases/v*`** | ✅    | ✅    | ❌              | ✅              | ✅           | ✅          | **Auto-release!** 🎉            |
| PR closed (not merged)        | ❌    | ❌    | ❌              | ❌              | ❌           | ❌          | No action taken                |
| `v1.2.3-dev` branch           | ✅    | ✅    | ✅              | ❌              | ❌           | ❌          | Development builds             |
| `main`/`master` branch        | ❌²   | ❌²   | ❌              | ❌              | ❌           | ❌          | Only via PR                    |
| PR to `main`/`master`         | ✅    | ✅    | ❌              | ❌              | ❌           | ❌          | Validation before merge        |

**Notes:**
- ¹ Tests run as PR checks (before merge is allowed)
- ² Direct pushes to `main`/`master` don't trigger workflows (protected branches)
- **Single release method**: Only PR merge to `releases/*` triggers releases
- **Branch protection required**: Disable direct pushes to `releases/*` branches

---

## Release Process 🎯

### Standard Release (v1.2.3) - **100% Automated!**

```bash
# ========================================
# COMPLETE PROCESS - ZERO MANUAL WORK! 🎉
# ========================================

# 1. Create release branch from main
git checkout main
git pull origin main
git checkout -b feature/prepare-v1.2.3

# 2. Update CHANGELOG.md
cat >> CHANGELOG.md << 'EOF'
## [1.2.3] - 2025-01-20

### Added
- New feature X

### Fixed  
- Bug fix Y
EOF

# 3. Commit and push to feature branch
git add CHANGELOG.md
git commit -m "chore: prepare release v1.2.3"
git push origin feature/prepare-v1.2.3

# 4. Create Pull Request targeting releases/v1.2.3 branch
gh pr create \
  --base releases/v1.2.3 \
  --head feature/prepare-v1.2.3 \
  --title "Release v1.2.3" \
  --body "Preparing release v1.2.3 with changelog updates"

# ✅ PR is created and AUTOMATICALLY:
#    - Runs all tests (~5 min)
#    - Runs linting
#    - Shows status checks on PR
#
# 📋 Review PR, ensure tests pass, get approvals (if required)
#
# 5. Merge PR (via GitHub UI or CLI)
gh pr merge --merge --delete-branch

# ✅ Upon merge, GitHub Actions AUTOMATICALLY:
#    1. Re-runs tests for the merged commit
#    2. Re-runs linting
#    3. Builds binaries for all platforms
#    4. Creates Git tag v1.2.3 (NO MANUAL TAG!)
#    5. Creates GitHub Release with changelog
#    6. Builds Docker images
#    7. Pushes to ghcr.io/binsabbar/vault-sync:v1.2.3
#    8. Pushes to ghcr.io/binsabbar/vault-sync:latest
#
# ⏱️  Total time after merge: ~8 minutes
# 🎉 DONE! Full audit trail via PR + automated release!

# Check the release (optional)
gh release view v1.2.3
```

**That's it! Just create a PR to a `releases/*` branch and merge it - everything happens automatically!**

### Pre-release (RC/Beta/Alpha) - **Also PR-Based!**

```bash
# 1. Create feature branch for pre-release prep
git checkout -b feature/prepare-v1.2.3-rc.1

# 2. Update CHANGELOG.md with RC notes
cat >> CHANGELOG.md << 'EOF'
## [1.2.3-rc.1] - 2025-01-20

### Added (Pre-release)
- Feature X for testing
EOF

git add CHANGELOG.md
git commit -m "chore: prepare release v1.2.3-rc.1"
git push origin feature/prepare-v1.2.3-rc.1

# 3. Create PR targeting releases/v1.2.3-rc.1
gh pr create \
  --base releases/v1.2.3-rc.1 \
  --head feature/prepare-v1.2.3-rc.1 \
  --title "Release v1.2.3-rc.1" \
  --body "Pre-release for testing"

# 4. Merge PR after tests pass
gh pr merge --merge --delete-branch

# ✅ Upon merge, GitHub Actions AUTOMATICALLY:
#    - Creates tag v1.2.3-rc.1
#    - Creates GitHub pre-release (marked as pre-release)
#    - Pushes Docker: ghcr.io/binsabbar/vault-sync:v1.2.3-rc.1
#    - Does NOT update :latest tag (only stable releases do)
```

### Development Snapshot (No Release)

```bash
# 1. Create dev branch
git checkout -b v1.2.3-dev

# 2. Make changes and push
git push origin v1.2.3-dev

# ✅ Triggers: Tests + Lint + Snapshot Build
# ❌ No release created
# ❌ No tags created
# ❌ No Docker images pushed
# 📦 Binaries available in workflow artifacts
```

---

## Branch Protection Setup (Required!)

To enforce the PR-based release workflow and prevent accidental direct pushes, set up branch protection:

### Step 1: Protect `releases/*` Branches

1. Go to **Settings** → **Branches** → **Add branch protection rule**
2. Configure:
   - **Branch name pattern**: `releases/*`
   - ✅ **Require a pull request before merging**
     - ✅ Require approvals: 1 (optional, recommended for team)
     - ✅ Dismiss stale pull request approvals when new commits are pushed
   - ✅ **Require status checks to pass before merging**
     - Add required checks: `test`, `golangci-lint`
   - ✅ **Require conversation resolution before merging**
   - ✅ **Do not allow bypassing the above settings** (or allow for admins only)
   - ❌ **Allow force pushes**: Disabled
   - ❌ **Allow deletions**: Disabled

3. **Save changes**

### Step 2: Protect `main`/`master` Branch (If not already protected)

1. Add another branch protection rule
2. Configure:
   - **Branch name pattern**: `main` (or `master`)
   - ✅ **Require a pull request before merging**
   - ✅ **Require status checks to pass**: `test`, `golangci-lint`

### Result

With branch protection enabled:
- ✅ All `releases/*` changes must go through PRs
- ✅ Tests and lint must pass before merge
- ✅ Full audit trail (who approved, when merged)
- ❌ Direct pushes to `releases/*` branches blocked
- ❌ No one can bypass the workflow (including you!)
- 🎉 **Single, controlled release method!**

---

## Automated vs Manual Release

### ✅ Fully Automated Release (Recommended) - **ZERO MANUAL WORK!**

```bash
# This is ALL you need to do:
git checkout -b releases/v1.2.3
# ... update CHANGELOG.md ...
git add CHANGELOG.md
git commit -m "chore: prepare release v1.2.3"
git push origin releases/v1.2.3

# 🎉 DONE! Everything else is automatic:
# ✅ Tests run
# ✅ Tag v1.2.3 created automatically
# ✅ GitHub Release created automatically
# ✅ Docker images built and pushed automatically
# ✅ Binaries compiled for all platforms automatically
```

**Benefits:**
- ✅ No manual tag creation needed
- ✅ No manual GitHub Release creation needed
- ✅ No manual Docker builds needed
- ✅ Single command to trigger everything
- ✅ Impossible to forget steps
- ✅ Consistent process every time

### 🔧 Manual Release (Optional)

```bash
# If you need to re-trigger release for existing branch:
gh workflow run release.yaml -f branch=releases/v1.2.3

# Or via GitHub Actions UI:
# Actions → release → Run workflow → Enter branch → Run
```

---

## Quick Reference

### Release Checklist ✅

- [ ] Create `releases/vX.Y.Z` branch from `main`
- [ ] Update `CHANGELOG.md` with version changes
- [ ] Commit and push branch
- [ ] **That's it!** Everything else is automatic:
  - [ ] Tests run automatically (~5 min)
  - [ ] Git tag created automatically
  - [ ] GitHub Release created automatically
  - [ ] Docker images built and pushed automatically
  - [ ] Binaries compiled automatically
- [ ] (Optional) Verify GitHub Release created
- [ ] (Optional) Verify Docker images pushed

### Docker Images Produced

| Tag Pattern         | Docker Images                                    |
| ------------------- | ------------------------------------------------ |
| `v1.2.3`            | `ghcr.io/binsabbar/vault-sync:v1.2.3`, `:latest` |
| `v1.2.3-rc.1`       | `ghcr.io/binsabbar/vault-sync:v1.2.3-rc.1`       |
| `v1.2.3-beta.1`     | `ghcr.io/binsabbar/vault-sync:v1.2.3-beta.1`     |
| `v1.2.3-alpha.1`    | `ghcr.io/binsabbar/vault-sync:v1.2.3-alpha.1`    |
| `v1.2.3-dev` branch | ❌ No Docker images (snapshot build only)         |

### Artifact Locations

- **GitHub Releases**: https://github.com/Binsabbar/vault-sync/releases
- **Docker Images**: https://github.com/Binsabbar/vault-sync/pkgs/container/vault-sync
- **Snapshot Artifacts**: Available in GitHub Actions workflow run artifacts

---

## Benefits of This Workflow Design

✅ **100% Automated**: Push branch → tag + release created automatically  
✅ **Zero Manual Work**: No manual tag creation, no manual releases  
✅ **Single Source of Truth**: Branch name defines version  
✅ **Safe**: Tests must pass before release is created  
✅ **Fast**: Complete release in ~8 minutes  
✅ **Consistent**: Same process every time, no steps forgotten  
✅ **Flexibility**: Manual trigger available if needed  
✅ **Pre-release Support**: Automatic detection and marking  
✅ **Docker Latest**: Only stable releases update `:latest` tag  
✅ **Developer Friendly**: Same Makefile targets work locally and in CI  

---

## Troubleshooting

### Tests Failed on Release Branch
```bash
# Fix the issue
git commit -am "fix: resolve test failures"
git push origin releases/v1.2.3

# Push again triggers workflow, which will create release when tests pass
```

### Need to Re-release
```bash
# Delete the GitHub Release first
gh release delete v1.2.3 -y

# The tag will be deleted automatically by GitHub
# OR delete manually if needed:
git push origin :refs/tags/v1.2.3

# Push to branch again to re-trigger release
git push origin releases/v1.2.3 --force
```

### Want Different Version
```bash
# Just create a new branch with correct version
git checkout -b releases/v1.2.4
git cherry-pick <commits>
git push origin releases/v1.2.4
```

### Release Failed
```bash
# View workflow logs
gh run list --workflow=release.yaml
gh run view <run-id> --log

# Re-run manually if needed
gh workflow run release.yaml -f tag=v1.2.3 -f skip_tests=true
```

---

## Local Development

All CI/CD commands can be run locally:

```bash
# Run tests (same as CI)
make go-test

# Run linting (same as CI)  
make go-lint

# Build snapshot (same as CI)
make release-snapshot

# Vulnerability scan
make go-vulncheck

# Build Docker image locally
make docker-build VERSION=v1.2.3

# Build and push (requires GITHUB_TOKEN)
export GITHUB_TOKEN=ghp_xxx
make docker-build-and-push VERSION=v1.2.3
```

---

## Workflow Files

- [`test.yaml`](workflows/test.yaml) - Run tests and security checks
- [`golangci-lint.yaml`](workflows/golangci-lint.yaml) - Code quality linting
- [`snapshot.yaml`](workflows/snapshot.yaml) - Development snapshot builds
- [`release.yaml`](workflows/release.yaml) - Production releases

## Configuration Files

- [`Makefile`](../Makefile) - Build automation and CI/CD targets
- [`.golangci.yml`](../.golangci.yml) - Linting configuration
- [`.goreleaser.yaml`](../.goreleaser.yaml) - Release configuration
- [`CHANGELOG.md`](../CHANGELOG.md) - Version history and release notes
- [`RELEASE.md`](../RELEASE.md) - Detailed release process documentation