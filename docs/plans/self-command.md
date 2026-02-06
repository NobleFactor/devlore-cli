# Plan: Self Command with Install and Upgrade Subcommands

---
title: Self Command with Install and Upgrade Subcommands
issue: https://github.com/NobleFactor/devlore-cli/issues/83
status: draft
created: 2026-02-06
updated: 2026-02-06
---

## Summary

Refactor the current `self-install` command into a `self` parent command with two subcommands: `self install` (offline, generates local files) and `self upgrade` (network, downloads newer LKG). This aligns with package manager conventions and matches star's behavior.

## Goals

1. **Offline install**: `self install` generates completions, man pages, config without network
2. **Explicit upgrade**: `self upgrade` checks GitHub for newer LKG release
3. **In-place upgrade**: `self upgrade` replaces the running binary (no --prefix)
4. **Consistent with star**: Same command structure across all NobleFactor CLI tools

## Prior Art

| Tool | update | upgrade |
|------|--------|---------|
| apt | Refresh package index | Install newer versions |
| brew | Fetch newest formulae | Install newer versions |
| port | selfupdate = refresh + upgrade | - |
| pip | - | `install --upgrade` |

Our `self upgrade` = install newer version (like `apt upgrade`, `brew upgrade`).

## Current State

| Command | Status | Notes |
|---------|--------|-------|
| `lore self-install --prefix <dir>` | ✅ Working | Generates completions, man pages, config |
| `writ self-install --prefix <dir>` | ✅ Working | Same as lore |
| Version check | ❌ Missing | No LKG check or download |
| Release caching | ❌ Missing | No cache for downloaded releases |

### Current Implementation

`internal/cli/selfinstall.go` provides:
- `detectShells()` - auto-detect bash, fish, powershell, zsh
- `shellCompletionPath()` - XDG-compliant paths for each shell
- `installCompletionsForShells()` - generate completions using Cobra
- `installManPagesTo()` - generate man pages
- `initDevloreConfig()` - create config files

## Requirements

### Self Install (Offline)

Generate supporting files locally - no network required:

```bash
lore self install                    # Default: ~/.local
lore self install --prefix /usr/local
writ self install --prefix ~/.local
```

**Arguments**:
- `--prefix <dir>` - Installation prefix (default: `~/.local`)

**What it does**:
1. Generate and install to XDG locations:
   - `${XDG_DATA_HOME}/man/man1/{lore,writ}*.1` (man pages)
   - `${XDG_DATA_HOME}/bash-completion/completions/{lore,writ}` (bash)
   - `${XDG_DATA_HOME}/fish/vendor_completions.d/{lore,writ}.fish` (fish)
   - `${XDG_DATA_HOME}/zsh/site-functions/_{lore,writ}` (zsh)
   - `${XDG_DATA_HOME}/powershell/Completions/{lore,writ}.ps1` (PowerShell)
   - `${XDG_CONFIG_HOME}/devlore/config.yaml` (shared config)
   - `${XDG_CONFIG_HOME}/devlore/config.d/{lore,writ}.yaml` (tool config)
2. Report what was installed

**Key behaviors**:
- Works offline - no network required
- Idempotent - safe to run anytime
- XDG compliant

### Self Upgrade (Network)

Check for and install newer LKG release:

```bash
lore self upgrade                    # Requires GH_TOKEN for private repo
writ self upgrade
```

**No arguments** - upgrade is always in-place:
- Replaces the currently running binary
- Uses the same location where the binary was found
- No `--prefix` flag (unlike `self install`)

**What it does**:
1. Determine current binary location via `os.Executable()`
2. Check current version vs latest LKG on GitHub (requires `GH_TOKEN`)
3. If newer version available:
   - Download release tarball (uses cache if available)
   - Verify checksum
   - Replace binary in-place
4. Run `self install --prefix <detected-prefix>` to regenerate completions/man pages
5. Report what was updated

**Key behaviors**:
- In-place upgrade - no prefix argument
- Requires network and `GH_TOKEN` (private repo)
- Fail-fast on network errors
- Uses release cache to reduce downloads
- No-op if already at latest LKG

### Release Caching

Downloaded releases are cached to reduce redundant downloads:

**Cache location**: `${XDG_CACHE_HOME}/devlore/releases/`

**Structure**:
```
${XDG_CACHE_HOME}/devlore/releases/
├── devlore-cli_v1.0.0_darwin_arm64.tar.gz
├── devlore-cli_v1.0.0_darwin_arm64.tar.gz.sha256
└── latest.txt  # Last known LKG version
```

**Behavior**: If cached tarball matches latest version and checksum is valid, skip download.

### Error Handling

**self install**: Always succeeds (local operations only).

**self upgrade**: Fail-fast on network errors:
```
error: failed to fetch latest release: connection refused
error: GH_TOKEN required for private repository access
error: checksum verification failed for devlore-cli_v1.0.0_darwin_arm64.tar.gz
```

## Implementation Phases

### Phase 1: Create Self Parent Command

1. Create `internal/cli/self.go`:
   - `self` parent command
   - Register `install` and `upgrade` subcommands

2. Refactor `internal/cli/selfinstall.go` → `internal/cli/self_install.go`:
   - Change from `self-install` to `self install` subcommand
   - Keep all existing functionality
   - Default `--prefix` to `~/.local`

3. Update `cmd/lore/main.go` and `cmd/writ/main.go`:
   - Register new `self` command tree

4. Test: `lore self install` works same as old `lore self-install`

### Phase 2: Add Self Upgrade

1. Create `internal/cli/self_upgrade.go`:
   - No `--prefix` flag
   - Detect current binary location
   - Check GitHub for latest LKG
   - Download and replace in-place
   - Call `self install` with detected prefix

2. Create `internal/cli/release.go`:
   - GitHub release API client (authenticated via `GH_TOKEN`)
   - `GetLatestRelease()` - fetch latest LKG version
   - `DownloadRelease(version, os, arch)` - download tarball
   - Release caching in `${XDG_CACHE_HOME}/devlore/releases/`

3. Test: `lore self upgrade` downloads and replaces binary

### Phase 3: Update Bootstrap Script

1. Update `install.sh`:
   - Run `lore self install` / `writ self install` (not `self-install`)
   - Same for PowerShell if we add `install.ps1`

2. Test: Fresh install via `curl | bash` works

### Phase 4: Deprecation and Cleanup

1. Add deprecation warning to old `self-install` command:
   ```
   warning: 'self-install' is deprecated, use 'self install'
   ```

2. After one release cycle, remove `self-install` entirely

## Files to Create/Modify

| File | Action | Phase |
|------|--------|-------|
| `internal/cli/self.go` | Create - `self` parent command | 1 |
| `internal/cli/self_install.go` | Rename from selfinstall.go, refactor | 1 |
| `internal/cli/selfinstall.go` | Delete (after rename) | 1 |
| `cmd/lore/main.go` | Modify - register self command | 1 |
| `cmd/writ/main.go` | Modify - register self command | 1 |
| `internal/cli/self_upgrade.go` | Create - `self upgrade` subcommand | 2 |
| `internal/cli/release.go` | Create - GitHub release API + caching | 2 |
| `install.sh` | Modify - use `self install` | 3 |

## Migration Path

### For users

**Before** (deprecated):
```bash
lore self-install --prefix ~/.local
```

**After**:
```bash
lore self install --prefix ~/.local   # Same behavior
lore self upgrade                      # New: check for updates
```

### For CI

```yaml
- name: Upgrade devlore tools
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: |
    lore self upgrade || true
    writ self upgrade || true
```

## Dependencies

- Issue #82 - Versioning with timestamps and commit hashes (needed for version comparison)

## Related Documents

- [noblefactor-ops: star-release-install.md](https://github.com/NobleFactor/noblefactor-ops/blob/develop/docs/plans/star-release-install.md) - Reference implementation
- Issue #82 - Versioning scheme
- Issue #83 - This plan's tracking issue
