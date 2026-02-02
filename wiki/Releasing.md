# Release Workflow

DevLore CLI uses a branch-based release workflow with automated deployments to the documentation website.

## GitHub Actions Workflows

All CI/CD automation lives in `.github/workflows/`:

| Workflow | File | Triggers | What It Does |
|----------|------|----------|--------------|
| **CI** | `ci.yaml` | Push to `develop`, PRs to `develop` | Build, vet, lint, test, shell lint |
| **Docs Publish** | `docs-publish.yaml` | Push to `develop`, `main`, `release/*` | Generates CLI docs, copies guides, creates PR to website |
| **Release** | `release.yaml` | Push to `develop`, `main`, `release/*`, tags `v*` | Builds binaries, creates GitHub release, syncs `install.sh` to website |
| **Sync Knowledge** | `sync-knowledge.yaml` | Push to `develop`, `main`, `release/*` (paths: `internal/starlark/**`, `internal/writ/migrate/**`) | Syncs Starlark bindings and migration knowledge to devlore-registry |

### Workflow Details

**ci.yaml** - Quality gate for development:
- Runs on every push to `develop` and every PR
- Steps: `go build`, `go vet`, `golangci-lint`, `go test`, shell lint
- Must pass before merge

**docs-publish.yaml** - Documentation sync:
- Generates CLI reference docs via `cmd/docgen`
- Copies `docs/guides/` (hand-written guides)
- Creates PR to `devlore.noblefactor.com` with content in `src/content/cli/` and `src/content/guides/`
- Uses `NOBLEFACTOR_AUTOMATION` org secret

**release.yaml** - Binary releases and install script sync:
- Builds cross-platform binaries via `make dist`
- Creates GitHub Release (prerelease for develop/release/*, full release for tags)
- Syncs `install.sh` to website via PR
- Uses `SITE_DEPLOY_TOKEN` secret for website access, `GITHUB_TOKEN` for releases

### What Happens on Push

| Push to... | ci.yaml | docs-publish.yaml | release.yaml |
|------------|---------|-------------------|--------------|
| `develop` | Runs | Creates docs PR | Creates prerelease, syncs install.sh |
| `main` | — | Creates docs PR | Creates RC release, syncs install.sh |
| `release/*` | — | Creates docs PR | Creates RC release, syncs install.sh |
| `v*` tag | — | — | Creates full release |
| PR to `develop` | Runs | — | — |

## Branch Strategy

| Branch | Purpose |
|--------|---------|
| `develop` | Active development |
| `release/*` | Release candidates |
| `main` | Production releases |
| `v*` tags | Version releases |

## Development Flow

```
feature/* ──> develop ──> release/X.Y ──> main ──> tag vX.Y.Z
                │              │            │          │
             preview        preview    production  production
```

## Creating a Release

### 1. Stabilize on a release branch

```bash
# From develop, create a release branch
git checkout develop
git pull origin develop
git checkout -b release/1.0

# Push to trigger preview deployment
git push -u origin release/1.0
```

The release branch deploys to its own preview environment for testing.

### 2. Test the release candidate

- Verify the preview deployment at the Azure SWA preview URL
- Run manual tests against the release candidate
- Fix any issues directly on the release branch

### 3. Merge to main

```bash
# Create PR from release/1.0 to main
gh pr create --base main --head release/1.0 --title "Release 1.0"

# After approval, merge
gh pr merge --squash
```

### 4. Tag the release

```bash
git checkout main
git pull origin main
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

See [What Happens on Push](#what-happens-on-push) for what the tag triggers.

### 5. Clean up

```bash
# Delete the release branch
git branch -d release/1.0
git push origin --delete release/1.0

# Merge main back to develop
git checkout develop
git merge main
git push origin develop
```

## Automated Deployments

See [GitHub Actions Workflows](#github-actions-workflows) above for the full picture.

### Required Secrets

| Secret | Scope | Used By | Purpose |
|--------|-------|---------|---------|
| `SITE_DEPLOY_TOKEN` | Repo | `release.yaml` | PAT with write access to `devlore.noblefactor.com` only |
| `NOBLEFACTOR_AUTOMATION` | Org | `docs-publish.yaml`, `sync-knowledge.yaml` | PAT for cross-repo automation (website, registry, ops) |
| `GITHUB_TOKEN` | Auto | `release.yaml` | Auto-provided, creates GitHub releases |

**Token separation rationale**: `release.yaml` runs frequently (every push to develop/release/*), so it uses `SITE_DEPLOY_TOKEN` scoped only to the website. This limits blast radius if compromised. `NOBLEFACTOR_AUTOMATION` has broader cross-repo access for less frequent automation tasks.

See [devlore.noblefactor.com/DEPLOYMENT.md](https://github.com/NobleFactor/devlore.noblefactor.com/blob/develop/DEPLOYMENT.md) for full secrets documentation.

## Release Artifacts

Each release includes binaries and package manager assets for multiple platforms:

### Binaries

| Platform | Architecture | Format |
|----------|--------------|--------|
| Darwin (macOS) | amd64, arm64 | `.tar.gz` |
| Linux | amd64, arm64 | `.tar.gz` |
| Windows | amd64, arm64 | `.zip` |

### Package Manager Assets

| Platform | Format | File | Publishing |
|----------|--------|------|------------|
| macOS | Homebrew formula | `dist/homebrew/...` | Manual (to homebrew-tap repo) |
| macOS | MacPorts Portfile | `Portfile` | Manual (PR to macports-ports) |
| Debian/Ubuntu | `.deb` | `devlore-cli_*_amd64.deb` | Manual (to apt repo) |
| RHEL/Fedora | `.rpm` | `devlore-cli-*.x86_64.rpm` | Manual (to yum repo) |
| Windows | Winget manifest | `dist/winget/...` | Manual (PR to winget-pkgs) |

All assets are generated but **not auto-published** to package repositories. This allows review before submission.

### MacPorts Portfile Generation

The Portfile is generated from a template during the release:

1. `packaging/macports/Portfile.template` - Template with placeholders
2. `packaging/macports/generate-portfile.sh` - Substitutes version and checksums
3. GoReleaser runs the script as a post-hook and includes the result via `extra_files`

To submit to MacPorts after a release:

```bash
# Fork https://github.com/macports/macports-ports
# Copy the Portfile to devel/devlore-cli/Portfile
# Submit PR
```

### Future Publishing

When ready to auto-publish, enable by removing `skip_upload: true` from:
- `brews:` in `.goreleaser.yaml` (requires homebrew-tap repo)
- `winget:` in `.goreleaser.yaml` (requires winget-pkgs PR approval)

For apt/yum repos, consider services like Gemfury or Packagecloud.

## Version Numbering

- **Draft/testing**: `v0.1.0-draft`
- **Pre-release**: `v0.1.0-alpha`, `v0.1.0-beta`, `v0.1.0-rc.1`
- **Release**: `v1.0.0`, `v1.0.1`, `v1.1.0`

## Website Environments

| Environment | URL | Auth Required |
|-------------|-----|---------------|
| Production | `devlore.noblefactor.com` | Currently yes, later no |
| Preview (develop) | Azure SWA preview URL | Yes |
| Preview (release/*) | Azure SWA preview URL | Yes |

## Local Testing

To test the install script locally:

```bash
# Start a local server
cd /path/to/devlore.noblefactor.com/public
python3 -m http.server 8888

# Run install script with local override
DEVLORE_DOWNLOAD_BASE="http://localhost:8888/releases" bash install.sh
```
