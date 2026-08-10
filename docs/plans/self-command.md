# Plan: Standardize the `self` Command

| Field       | Value                                           |
|-------------|--------------------------------------------------|
| status      | draft                                            |
| branch      | feat/self-command                                |
| issue       | TBD                                              |
| started     | 2026-08-09                                       |

## Context

The CLI tools have two divergent implementations of self-installation:

- **star** uses `star self install [prefix]` via `cmd/star/cli/selfinstall.go` (positional arg, defaults to `~/.local`, installs extensions)
- **lore/writ/devlore-test** use `<tool> self-install --prefix=<dir>` via `internal/cli/selfinstall.go` (flat command, `--prefix` flag, config/cache init)

Both implementations duplicate helpers: `copyFile`, `installBinary`, `installManPagesTo`, `installCompletionsForShells`, `detectShells`, `hasMan`, `shellCompletionPath`.

All tools need a unified `self` command group with `install`, `upgrade`, and `uninstall` subcommands.

## Target Interface

```
<tool> self install [prefix]    # prefix defaults to ~/.local
<tool> self upgrade             # overwrites binary in place, refreshes supporting files
<tool> self uninstall [prefix]  # removes binary, man pages, completions from prefix
```

### Semantics

- **install**: Copies binary to `<prefix>/bin/`, installs man pages, completions, config, cache. Writes a manifest recording every installed file with its SHA-256 checksum. Full first-time setup.
- **upgrade**: Resolves current install prefix from `os.Executable()` (strips `/bin/<tool>` suffix). Overwrites binary in place. Refreshes man pages, completions, config, cache. Updates the manifest. No prefix argument needed.
- **uninstall**: Reads the manifest. Removes files whose checksums still match (untouched since install). Skips files the user modified or added — reports what was skipped. Removes empty directories left behind.

### Manifest

`self install` and `self upgrade` write a manifest to `<prefix>/share/<tool>/manifest.json`:

```json
{
  "tool": "writ",
  "version": "0.4.0",
  "prefix": "/home/user/.local",
  "installed": "2026-08-09T14:30:00Z",
  "files": [
    {"path": "bin/writ", "sha256": "abc123..."},
    {"path": "share/man/man1/writ.1", "sha256": "def456..."},
    {"path": "share/zsh/site-functions/_writ", "sha256": "789abc..."}
  ]
}
```

Paths are relative to the prefix. `self uninstall` computes the current checksum of each file and only removes it if it matches. Modified files and files not in the manifest are left alone.

## Changes

### 1. Rewrite `internal/cli/selfinstall.go`

Replace `NewSelfInstallCmd()` with `NewSelfCmd()` that returns a `self` parent command with three subcommands.

**SelfInstallInfo changes:**
- `ConfigInfo` becomes `*ConfigInfo` (pointer, nil for star which has no config)
- Add `PostInstallHooks []func(root string) error` for tool-specific post-install actions (star uses this for extensions)
- Add `PostUninstallHooks []func(root string) error` for tool-specific post-uninstall actions

**`self install [prefix]` subcommand:**
- Positional arg (0 or 1), defaults to `~/.local`
- `--shell` flag (repeatable)
- Calls existing `runSelfInstall()` (refactored to skip config/cache when `ConfigInfo` is nil)
- After installing all files, writes the manifest via `writeManifest()`

**`self upgrade` subcommand:**
- No positional args
- `--shell` flag (repeatable)
- Resolves prefix via `resolveInstalledPrefix()`: `filepath.Dir(filepath.Dir(os.Executable()))` (e.g., `/home/user/.local/bin/writ` -> `/home/user/.local`)
- Calls same `runSelfInstall()` — binary overwrites itself in place (same source/target is fine on Unix; `installBinary` already handles source==target as a no-op, which needs adjusting so upgrade actually copies)
- Updates the manifest

**`self uninstall [prefix]` subcommand:**
- Positional arg (0 or 1), defaults to resolved prefix from `os.Executable()`
- `--force` flag to skip confirmation prompt
- Reads manifest from `<prefix>/share/<tool>/manifest.json`
- For each file in the manifest: compute current SHA-256, remove only if it matches (file is unmodified)
- Reports skipped files (modified since install)
- Removes empty parent directories left behind
- Removes the manifest file itself
- Runs `PostUninstallHooks`

**Helper additions:**
- `resolveInstalledPrefix() (string, error)` — resolves prefix from running binary location
- `writeManifest(prefix, toolName, version string, files []installedFile) error` — writes manifest JSON
- `readManifest(prefix, toolName string) (*manifest, error)` — reads manifest JSON
- `runSelfUninstall()` — manifest-based removal of installed files
- `fileSHA256(path string) (string, error)` — computes SHA-256 of a file
- Move `copyDir()` from `cmd/star/cli` to `internal/cli` (exported as `CopyDir` for use in star's hook)
- Move `findExtensionsDir()` concept to star's hook function (not in shared code)

### 2. Update `internal/cli/root.go` (line 110)

```go
// Before:
rootCmd.AddCommand(NewSelfInstallCmd(rootCmd, SelfInstallInfo{...}))

// After:
rootCmd.AddCommand(NewSelfCmd(rootCmd, SelfInstallInfo{...}))
```

`ConfigInfo` becomes `&configInfo` (pointer).

### 3. Update `cmd/devlore-test/devloretest/root.go` (line 98)

Same change: `NewSelfInstallCmd` -> `NewSelfCmd`, `ConfigInfo` -> `&configInfo`.

### 4. Update `cmd/star/main.go` (line 330)

Switch from `cli2.NewSelfCmd` to shared `cli.NewSelfCmd`:

```go
rootCmd.AddCommand(cli.NewSelfCmd(rootCmd, cli.SelfInstallInfo{
    Name:      "star",
    ManHeader: cli.ManHeader{...},
    PostInstallHooks: []func(string) error{installStarExtensions},
    PostUninstallHooks: []func(string) error{uninstallStarExtensions},
}))
```

Define `installStarExtensions(root string) error` and `uninstallStarExtensions(root string) error` as local functions in `cmd/star/main.go` (or a new `cmd/star/install.go`).

Remove `cli2` import if no longer used.

### 5. Delete `cmd/star/cli/selfinstall.go` and `cmd/star/cli/selfinstall_test.go`

All functionality moves to `internal/cli` or to hook functions in `cmd/star/`.

### 6. Update `internal/cli/selfinstall_test.go`

Remove tests for old `NewSelfInstallCmd` API. Add tests for:
- `TestNewSelfCmd_InstallDefaultPrefix`
- `TestNewSelfCmd_InstallCustomPrefix`
- `TestResolveInstalledPrefix`
- `TestNewSelfCmd_UpgradeResolvesPrefix`
- `TestCopyDir` (migrated from star tests)
- `TestRunSelfInstall_NilConfigInfo`

Keep: `TestExpandTilde`, `TestShellCompletionPath_PerShell`, `TestHasMan`, `TestDetectShells`, `TestCopyFile`, `TestCopyFile_NonExistentSource`.

### 7. Add Makefile `install` target

```makefile
### PREFIX
# Installation prefix for `make install`.
PREFIX ?= ~/.local

install: build ## Install lore, star, and writ via self install (PREFIX=~/.local)
	build/lore$(GOEXE) self install $(PREFIX)
	build/star$(GOEXE) self install $(PREFIX)
	build/writ$(GOEXE) self install $(PREFIX)
```

Add `install` to `.PHONY` line.

Note: `devlore-test` is excluded from `make install` — it is a developer tool, not an end-user tool.

### 8. Update string references

| File | Old | New |
|------|-----|-----|
| `cmd/writ/writ/config.go:238` | `writ self-install` | `writ self install` |
| `internal/cli/help.go:26` | `via self-install` | `via self install` |
| `docs/guides/getting-started.md:44-45` | `self-install --prefix=~/.local` | `self install` |
| `README.md:26,30-31` | `self-install --prefix=~/.local` | `self install` |

## Verification

1. `make build` succeeds
2. `make test` passes (including updated selfinstall tests)
3. `make install` builds then runs `self install` for lore, star, writ
4. `make install PREFIX=/tmp/devlore-test-install` installs to custom prefix
5. `lore self install /tmp/test && ls /tmp/test/bin/lore` — binary exists
6. `star self install /tmp/test && ls /tmp/test/share/star/extensions/` — extensions copied
7. `cat /tmp/test/share/lore/manifest.json` — manifest exists with checksums
8. `writ self upgrade` — overwrites writ in place, updates manifest
9. Edit a config file, then `lore self uninstall /tmp/test --force` — modified config skipped, unmodified files removed
10. Verify `which lore`, `which star`, `which writ` resolve after `make install`
