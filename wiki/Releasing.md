# Release Workflow

DevLore CLI uses a branch-based release workflow with automated deployments to the documentation website.

## GitHub Actions Workflows

All CI/CD automation lives in `.github/workflows/`:

| Workflow | File | Triggers | What It Does |
|----------|------|----------|--------------|
| **CI** | `ci.yaml` | Pushes and PRs to `develop`, `main`, `release/*` | The quality gate, the tests and the scenarios |
| **Installers** | `installers.yaml` | Pushes and PRs to `develop`, `main`, `release/*` | Tests `install.sh` and `install.ps1` against this checkout's programs |
| **Knowledge Extract** | `knowledge-extract.yaml` | Pushes and PRs to `develop`, `main`, `release/*` | Builds and validates the knowledge base; a push opens a PR to devlore-registry |
| **Publish CLI Docs** | `docs-publish.yaml` | Pushes to `develop`, `main`, `release/*` | Generates the CLI reference, copies the guides, opens a PR to the website |
| **Release** | `release.yaml` | Pushes to `develop`, `main`, `release/*`; tags `v*` | Builds the archives, creates the GitHub release, syncs both installers to the website |
| **LKG Tag** | `lkg-tag.yaml` | Run by hand (`workflow_dispatch`), admins alone | Tags a merged PR's merge commit last-known-good |

### Workflow Details

**ci.yaml** - the quality gate:
- `quality-gate`: installs its tools, then runs `make quality-gate`, the same list of checks the local gate and every
  pull request's pre-flight run (build for every platform, `go.mod` tidy, generated files current, guide frontmatter,
  vet and lint for every GOOS, shell, Starlark and PowerShell lint, complexity, ldflags); then a code-metrics report
- `test`: `make test-race` on darwin-arm64, linux-amd64, linux-arm64, windows-amd64 and windows-arm64
- `scenario`: `make test-scenario` on those platforms and darwin-amd64
- The develop ruleset requires `quality-gate`

**installers.yaml** - the installers against the programs in the same commit:
- Builds a release from this checkout, then runs `scripts/Test-InstallScript.sh` on Linux and macOS (MacPorts too)
  and `scripts/Test-InstallScript.ps1` on Windows under Windows PowerShell 5.1 and PowerShell 7

**knowledge-extract.yaml** - the knowledge base:
- Builds `star`, builds the knowledge base, validates the package and knowledge schemas
- On a PR, reports what would change; on a push, opens a PR to devlore-registry

**docs-publish.yaml** - documentation sync:
- Generates CLI reference docs via `cmd/devlore-docs`
- Copies `docs/guides/` (hand-written guides)
- Opens and merges a PR to `devlore.noblefactor.com` with content in `src/content/cli/` and `src/content/guides/`

**release.yaml** - archives, the release, and installer sync:
- Builds the archives and their checksums file with `make dist`, versioned as [Version Numbering](#version-numbering)
  says
- Creates the GitHub Release: a pre-release for every branch build, a full release for a tag
- Syncs `install.sh` and `install.ps1` to the website's `public/` via PR: its `develop` for develop runs, its `main` for
  `main` and tag runs, and the same `release/*` branch for release runs

**lkg-tag.yaml** - last-known-good:
- Given a merged PR's number, tags its merge commit `<base branch>/lkg` (`develop/lkg`, say), moving the tag if it
  is there; it refuses anyone but an admin, and a PR not merged

### What Happens on Push

| Event | ci.yaml | installers.yaml | knowledge-extract.yaml | docs-publish.yaml | release.yaml |
|-------|---------|-----------------|------------------------|-------------------|--------------|
| Push to `develop` | Runs | Runs | Opens registry PR | Opens docs PR | Pre-release, both installers synced |
| Push to `main` | Runs | Runs | Opens registry PR | Opens docs PR | RC pre-release, both installers synced |
| Push to `release/*` | Runs | Runs | Opens registry PR | Opens docs PR | RC pre-release, both installers synced |
| `v*` tag | — | — | — | — | Full release, both installers synced |
| PR to any of the three | Runs | Runs | Reports changes | — | — |

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
| `NOBLEFACTOR_AUTOMATION_APP_ID` | Org | `docs-publish.yaml`, `knowledge-extract.yaml`, `release.yaml` | The NobleFactor automation GitHub App, whose token writes to the website and devlore-registry |
| `NOBLEFACTOR_AUTOMATION_APP_PRIVATE_KEY` | Org | the same three | The app's private key, from which each run makes a short-lived token |
| `GITHUB_TOKEN` | Auto | `release.yaml`, `lkg-tag.yaml` | Auto-provided: creates GitHub releases, and checks the LKG caller's role |

No long-lived personal token is used: each workflow that writes to another repository makes its own token from the
app, scoped to that repository, for that run.

See [devlore.noblefactor.com/DEPLOYMENT.md](https://github.com/NobleFactor/devlore.noblefactor.com/blob/develop/DEPLOYMENT.md) for the website's side.

## Release Artifacts

`release.yaml` runs `make dist`, and each release carries what it makes:

| Platform | Architecture | Format |
|----------|--------------|--------|
| Darwin (macOS) | amd64, arm64 | `.tar.gz` |
| Linux | amd64, arm64 | `.tar.gz` |
| Windows | amd64, arm64 | `.zip` |

and `devlore-cli_<version>_checksums.txt`, a SHA-256 line for each archive, which both installers and `self upgrade`
verify an archive against before extracting it. Each archive holds `lore`, `star` and `writ`, and star's extensions
under `share/`.

Nothing builds package-manager assets. `.goreleaser.yaml` (Homebrew, winget, `.deb`, `.rpm`) and
`packaging/macports/` (a Portfile template and its generator) are kept, and nothing runs them: `make verify-ldflags`
keeps `.goreleaser.yaml`'s version stamps correct for whoever adopts it.

## Version Numbering

`release.yaml` gives each build its version, and the channel its binaries follow for `self upgrade`:

| Built from | Version | Release | Channel |
|------------|---------|---------|---------|
| `develop` | `v0.1.0-dev.<UTC stamp, YYYYMMDDhhmmss>` | Pre-release | `develop` |
| `main` | `v0.1.0-rc.<stamp>` | Pre-release | `release` |
| `release/X.Y` | `vX.Y-rc.<stamp>` | Pre-release | `release` |
| A `v*` tag | the tag, `vX.Y.Z` | Full release | `release` |

`make dist` with no `DEVLORE_VERSION` builds `v0.1.0-draft`, for local testing.

## Website Environments

Measured 2026-10-21:

| Environment | URL | Pages | Installers |
|-------------|-----|-------|------------|
| develop | `delightful-grass-0ac0a4c1e-develop.westus2.6.azurestaticapps.net` | Sign-in | `install.sh` and `install.ps1` anonymous, devlore-cli develop's: the installers' published address (`release.yaml`'s `INSTALLERS_URL`) |
| `devlore.noblefactor.com` | `devlore.noblefactor.com` | Sign-in | An older `install.sh` anonymous; `install.ps1` behind the sign-in (NobleFactor/devlore.noblefactor.com#504) |

## Local Testing

The installers always download from GitHub Releases; nothing overrides where. To try a change to one before it
merges, run it from the working tree into a scratch prefix, with the XDG directories in scratch so no real
registration changes:

```bash
scratch=$(mktemp -d)
XDG_CONFIG_HOME=$scratch/config XDG_DATA_HOME=$scratch/data XDG_STATE_HOME=$scratch/state XDG_CACHE_HOME=$scratch/cache \
  bash install.sh --prefix="$scratch/prefix" --personal=<path>
```
