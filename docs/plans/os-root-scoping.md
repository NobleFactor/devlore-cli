---
title: "os.Root Scoping for Provider I/O"
issue:
status: draft
created: 2026-03-08
updated: 2026-03-08
---

# Plan: os.Root Scoping for Provider I/O

## Summary

Replace all direct `os.*` filesystem calls in providers with operations scoped through Go's `os.Root` (introduced in Go 1.24). The `os.Root` type provides OS-enforced chroot-style confinement — symlinks cannot escape, `..` traversal is blocked, and all I/O is constrained to the authority boundary defined by `Context.BaseDir`. This eliminates manual path validation (`isSubpath`), simplifies the recovery system (recovery lives within root, guaranteeing same-partition), and removes platform-specific recovery base discovery code.

A new `recovery.Site` package extracts the recovery system from the file provider into a shared, context-aware service on `op.Context`. It supports both file archival (zero-copy rename) and data archival (byte serialization), enabling recovery for both file and mem providers.

## Goals

1. **OS-enforced authority boundary** — all provider file operations are confined to `BaseDir` by the kernel, not by application-level path checks
2. **Simplified recovery** — recovery directory lives inside root; no mount-point discovery, no cross-device checks, no platform-specific `getRecoveryBase`
3. **Root-relative paths** — `Resource.SourcePath` stores a struct with `.Rel` and `.Abs` fields; absolute paths are eagerly computed at construction time
4. **Shared recovery service** — `recovery.Site` on Context provides `ArchiveFile`/`RestoreFile` (zero-copy rename) and `ArchiveData`/`RestoreData` (byte serialization) for all providers
5. **Provider lifecycle** — providers that hold an `*os.Root` close it on teardown

## Current State

| Component             | Status                            | Notes                                                                    |
| --------------------- | --------------------------------- | ------------------------------------------------------------------------ |
| `Context.BaseDir`     | string                            | Authority boundary, consulted by `Provider.Root()`                       |
| `file.Provider` I/O   | `os.*` calls with absolute paths  | No OS-level confinement                                                  |
| `mem` extract         | `os.ReadFile` with absolute paths | Source reads unscoped                                                    |
| Recovery base         | Platform-specific discovery       | `recovery_unix.go`, `recovery_windows.go` — mount-point/volume detection |
| Recovery ownership    | Embedded in file provider         | `moveToRecovery`, `restoreFromRecovery`, `getRecoveryBase` are methods on `*file.Provider` |
| Path validation       | `isSubpath` helper                | Manual boundary check in `pruneEmptyParents`                             |
| `Resource.SourcePath` | Absolute `string`                 | Set by `filepath.Abs` in `Resolve()`                                     |
| WalkTree              | `filepath.WalkDir`                | Walks absolute paths                                                     |

## Requirements

### os.Root on Context

`Context` holds the opened `*os.Root` and a `*recovery.Site`. Providers access both through `Context()`. The executor opens `os.Root` from `BaseDir` before graph execution and closes it after. `Root` must be specified at Context creation time — no exceptions.

```go
type Context struct {
    context.Context
    BaseDir      string          // retained for display, logging, and URI construction
    Root         *os.Root        // opened from BaseDir; all scoped I/O goes through this
    RecoverySite *recovery.Site  // shared recovery service for all providers
    // ... other fields unchanged
}
```

### recovery.Site

The `recovery.Site` is a shared service that manages archival and restoration of files and data within root. It lives in `pkg/op/recovery/` and is instantiated on `Context`.

```go
package recovery

type Site struct {
    root *os.Root  // obtained from Context at construction
}

// ArchiveFile moves a file to recovery via zero-copy rename (no data movement).
// Returns the recovery-relative path for tombstone storage.
func (s *Site) ArchiveFile(path string) (recoveryPath string, err error)

// RestoreFile moves a file back from recovery via zero-copy rename (no data movement).
func (s *Site) RestoreFile(originalPath, recoveryPath string) error

// ArchiveData writes bytes to a file in the recovery directory.
// Returns the recovery-relative path for tombstone storage.
func (s *Site) ArchiveData(data []byte) (recoveryPath string, err error)

// RestoreData reads bytes back from a file in the recovery directory.
func (s *Site) RestoreData(recoveryPath string) ([]byte, error)
```

Provider usage:

```go
// file provider — zero-copy file archival
recoveryPath, err := p.Context().RecoverySite.ArchiveFile(resource.SourcePath.Rel)

// file provider — restore from recovery
err := p.Context().RecoverySite.RestoreFile(tombstone.Resource.SourcePath.Rel, tombstone.RecoveryPath)

// mem provider — byte serialization
recoveryPath, err := p.Context().RecoverySite.ArchiveData(resource.Data)

// mem provider — restore bytes
data, err := p.Context().RecoverySite.RestoreData(tombstone.RecoveryPath)
```

The recovery directory is `.devlore/recovery/` within root. The Site manages UUID-based subdirectories internally. `ArchiveFile`/`RestoreFile` perform zero-copy renames — no data movement. `ArchiveData`/`RestoreData` serialize bytes to/from the recovery directory.

### Provider I/O through Root

Every `os.*` call in a provider is replaced by the corresponding `root.*` call. Providers obtain root via `p.Context().Root`.

| Current                      | Replacement                       |
| ---------------------------- | --------------------------------- |
| `os.Stat(abs)`               | `root.Stat(rel)`                  |
| `os.Lstat(abs)`              | `root.Lstat(rel)`                 |
| `os.Rename(a, b)`            | `root.Rename(relA, relB)`         |
| `os.Remove(abs)`             | `root.Remove(rel)`                |
| `os.RemoveAll(abs)`          | `root.RemoveAll(rel)`             |
| `os.Symlink(target, link)`   | `root.Symlink(target, rel)`       |
| `os.Readlink(abs)`           | `root.Readlink(rel)`              |
| `os.Open(abs)`               | `root.Open(rel)`                  |
| `os.OpenFile(abs, ...)`      | `root.OpenFile(rel, ...)`         |
| `os.MkdirAll(abs, ...)`      | `root.MkdirAll(rel, ...)`         |
| `os.ReadFile(abs)`           | `root.ReadFile(rel)`              |
| `os.WriteFile(abs, ...)`     | `root.WriteFile(rel, ...)`        |
| `filepath.WalkDir(abs, ...)` | `fs.WalkDir(root.FS(), rel, ...)` |

### Root-relative Resource paths

`Resource.SourcePath` becomes a struct with both relative and absolute paths, eagerly computed at construction time:

```go
type SourcePath struct {
    Rel string // root-relative path — used for all I/O through os.Root
    Abs string // absolute path — used for URIs, display, logging
}
```

`Abs` is computed as `filepath.Join(root.Name(), rel)` at `NewResource` time. The struct is self-contained — no root reference needed after construction.

`NewResource` and `Resolve` change to work with root-relative paths and accept an `*os.Root` for metadata population (Stat, checksum).

### Elimination of manual path checks

- `isSubpath` helper — deleted; `os.Root` enforces the boundary
- `filepath.Abs` calls in `moveToRecovery` and `Resolve` — unnecessary; paths are already root-relative
- `pruneEmptyParents` boundary check — `os.Root` prevents escaping root; the `boundary` parameter continues to provide finer-grained prune limits within root

### Test runner scoping

The `t.tmp()` helper in the Starlark test runner becomes root-aware, following the same pattern as `recovery.Site`. Its base directory is `.devlore/tmp` within root. The `TestContext` uses root-scoped I/O for all file checks (`checkFileExists`, `checkNoFile`). The entire codebase operates within root bounds.

## Implementation Phases

### Phase 1: Context.Root and provider plumbing

- [ ] Add `Root *os.Root` field to `op.Context`
- [ ] Open `os.Root` from `BaseDir` in executor before graph execution
- [ ] Close `os.Root` after graph execution (defer)
- [ ] Update `testProvider` and test helpers to open `os.Root` from temp dirs
- [ ] Add `Root()` accessor or equivalent on `ProviderBase` for ergonomic access

**Files**:

- `pkg/op/context.go` — Modify: add `Root *os.Root`
- `internal/execution/executor.go` — Modify: open/close `os.Root`
- `pkg/op/provider/file/provider_test.go` — Modify: test helper opens `os.Root`

### Phase 2: Resource path model

- [ ] Define `SourcePath` struct with `Rel` and `Abs` fields
- [ ] Update `NewResource` to accept root-relative path and `*os.Root`, eagerly compute both fields
- [ ] Update `Resolve` to use `root.Stat` instead of `os.Stat`
- [ ] Update `buildURI` to use `SourcePath.Abs` for URI construction
- [ ] Update `Reader`, `WriteTo` to use root-scoped I/O
- [ ] Update `Refresh`, `RefreshWith` for root-scoped stat
- [ ] Update all call sites that access `Resource.SourcePath` (86 references in file provider)

**Files**:

- `pkg/op/provider/file/resource.go` — Modify: `SourcePath` struct, root-scoped I/O
- `pkg/op/provider/file/provider.go` — Modify: all methods use `SourcePath.Rel` for I/O
- `pkg/op/provider/file/gen/` — Regenerate: generated code adapts to new Resource construction

### Phase 3: File provider — core operations

- [ ] Replace all `os.Stat/Lstat` with `root.Stat/Lstat`
- [ ] Replace all `os.Rename` with `root.Rename`
- [ ] Replace all `os.Remove` with `root.Remove`
- [ ] Replace all `os.Symlink/Readlink` with `root.Symlink/Readlink`
- [ ] Replace all `os.Open/OpenFile` with `root.Open/OpenFile`
- [ ] Replace all `os.MkdirAll` with `root.MkdirAll`
- [ ] Replace `filepath.WalkDir` with `fs.WalkDir(root.FS(), ...)`
- [ ] Delete `isSubpath` helper
- [ ] Simplify `pruneEmptyParents` — remove `isSubpath` guard, keep `boundary` parameter
- [ ] Update `prepareWrite`, `write`, `compensateWrite` to use root-scoped I/O

**Files**:

- `pkg/op/provider/file/provider.go` — Modify: all operations use `root.*`
- `pkg/op/provider/file/helpers.go` — Modify: delete `isSubpath`; update `checksumFile` to use root
- `pkg/op/provider/file/recovery.go` — Modify: use `root.*` for moves (interim, before Phase 7 extraction)

### Phase 4: Recovery system — root-scoped (interim)

Simplify recovery to use `os.Root` while still embedded in the file provider. This is an interim step before Phase 7 extracts it into a shared package.

- [ ] Define recovery directory constant: `.devlore/recovery`
- [ ] Replace `getRecoveryBase` with constant path
- [ ] Delete `recovery_unix.go` — mount-point discovery, `findMountPoint`, `getFirstExistingAncestor`, `isSameDevice`
- [ ] Delete `recovery_windows.go` — volume detection
- [ ] Update `moveToRecovery` to use `root.MkdirAll` + `root.Rename` with relative paths
- [ ] Update `restoreFromRecovery` to use `root.Rename` with relative paths
- [ ] Remove `filepath.Abs` calls in recovery (paths are already root-relative)

**Files**:

- `pkg/op/provider/file/recovery.go` — Modify: root-scoped recovery with constant path
- `pkg/op/provider/file/recovery_unix.go` — Delete
- `pkg/op/provider/file/recovery_windows.go` — Delete

### Phase 5: mem provider — scoped source reads

- [ ] Update `extractLambdaBody` to use root-scoped `ReadFile` instead of `os.ReadFile`
- [ ] Update `extractDefSource` to use root-scoped `ReadFile`
- [ ] Thread `*os.Root` through `Extract`/`ExtractWithName` or access via provider context
- [ ] Update tests

**Files**:

- `pkg/op/provider/mem/extract.go` — Modify: root-scoped reads
- `pkg/op/provider/mem/extract_test.go` — Modify: test setup with `os.Root`

### Phase 6: Test and doc updates

- [ ] Update all file provider tests to use root-relative paths and `SourcePath` struct
- [ ] Update execution tests that construct Resources with absolute paths
- [ ] Update e2e test runner — `t.tmp()` returns root-relative paths under `.devlore/tmp`, `TestContext` uses root-scoped I/O for all file checks
- [ ] Update Starlark test scripts if they rely on absolute path semantics
- [ ] Update architecture docs (`4-resource-management.md`, `2.1-typed-slots.md`) to reflect `os.Root`
- [ ] Update plan docs that reference `getRecoveryBase` or recovery site discovery

**Files**:

- `pkg/op/provider/file/provider_test.go` — Modify
- `pkg/op/provider/file/gen/actions_gen_test.go` — Regenerate
- `internal/execution/execution_test.go` — Modify
- `internal/e2e/testrunner/test_context.go` — Modify: root-aware `t.tmp()` under `.devlore/tmp`, root-scoped checks
- `docs/architecture/4-resource-management.md` — Modify
- `docs/architecture/2.1-typed-slots.md` — Modify

### Phase 7: recovery.Site — shared recovery package

Extract the recovery system from the file provider into `pkg/op/recovery/`. Add `RecoverySite` to `op.Context`. Migrate file provider to use it. Add data archival for mem provider support.

- [ ] Create `pkg/op/recovery/site.go` with `Site` struct
- [ ] Implement `ArchiveFile(path string) (string, error)` — zero-copy rename into `.devlore/recovery/<uuid>`, no data movement
- [ ] Implement `RestoreFile(originalPath, recoveryPath string) error` — zero-copy rename back, no data movement
- [ ] Implement `ArchiveData(data []byte) (string, error)` — write bytes to `.devlore/recovery/<uuid>`
- [ ] Implement `RestoreData(recoveryPath string) ([]byte, error)` — read bytes from recovery
- [ ] Add `RecoverySite *recovery.Site` to `op.Context`
- [ ] Instantiate `recovery.Site` in executor alongside `os.Root`
- [ ] Migrate file provider: replace `moveToRecovery` calls with `p.Context().RecoverySite.ArchiveFile`
- [ ] Migrate file provider: replace `restoreFromRecovery` calls with `p.Context().RecoverySite.RestoreFile`
- [ ] Delete `pkg/op/provider/file/recovery.go` — logic now lives in `pkg/op/recovery/`
- [ ] Update file provider tests to use `RecoverySite`
- [ ] Add `recovery.Site` unit tests

**Files**:

- `pkg/op/recovery/site.go` — Create: `Site` struct with `ArchiveFile`/`RestoreFile`/`ArchiveData`/`RestoreData`
- `pkg/op/recovery/site_test.go` — Create: unit tests
- `pkg/op/context.go` — Modify: add `RecoverySite *recovery.Site`
- `internal/execution/executor.go` — Modify: instantiate `recovery.Site`
- `pkg/op/provider/file/provider.go` — Modify: use `RecoverySite.ArchiveFile`/`RestoreFile`
- `pkg/op/provider/file/recovery.go` — Delete: logic extracted to `pkg/op/recovery/`
- `pkg/op/provider/file/provider_test.go` — Modify: tests use `RecoverySite`

## Files to Create/Modify

| File                                       | Action | Purpose                                                |
| ------------------------------------------ | ------ | ------------------------------------------------------ |
| `pkg/op/recovery/site.go`                  | Create | Shared recovery service: `ArchiveFile`/`Data`, `RestoreFile`/`Data` |
| `pkg/op/recovery/site_test.go`             | Create | Unit tests for recovery.Site                           |
| `pkg/op/context.go`                        | Modify | Add `Root *os.Root`, `RecoverySite *recovery.Site`     |
| `pkg/op/provider/file/resource.go`         | Modify | `SourcePath` struct with `.Rel`/`.Abs`, root-scoped I/O |
| `pkg/op/provider/file/provider.go`         | Modify | All operations through `root.*`, use `RecoverySite`    |
| `pkg/op/provider/file/helpers.go`          | Modify | Delete `isSubpath`, update `checksumFile`              |
| `pkg/op/provider/file/recovery.go`         | Delete | Logic extracted to `pkg/op/recovery/`                  |
| `pkg/op/provider/file/recovery_unix.go`    | Delete | Mount-point discovery eliminated                       |
| `pkg/op/provider/file/recovery_windows.go` | Delete | Volume detection eliminated                            |
| `pkg/op/provider/mem/extract.go`           | Modify | Root-scoped source reads                               |
| `internal/execution/executor.go`           | Modify | Open/close `os.Root`, instantiate `recovery.Site`      |
| `internal/e2e/testrunner/test_context.go`  | Modify | Root-aware `t.tmp()` under `.devlore/tmp`              |
| `pkg/op/provider/file/provider_test.go`    | Modify | Root-relative test setup, `RecoverySite`               |
| `pkg/op/provider/mem/extract_test.go`      | Modify | Root-scoped test setup                                 |
| `internal/execution/execution_test.go`     | Modify | Root-relative slot values                              |

## Related Documents

- [4-resource-management.md](../architecture/4-resource-management.md) — Resource model architecture
- [2.1-typed-slots.md](../architecture/2.1-typed-slots.md) — Slot resolution and Context.Data
- [resource-management.md](resource-management.md) — Resource management plan (phases 0-8)

## Resolved Questions

- [x] **os.Root location**: Lives on `Context`. Must be specified at creation time. No exceptions.

- [x] **Recovery directory**: `.devlore/recovery/`. Recovery system extracted into `pkg/op/recovery/` as a shared `recovery.Site` service on `Context`. Providers access it via `p.Context().RecoverySite`. Supports both file archival (zero-copy rename, no data movement) and data archival (byte serialization) for file and mem providers respectively.

- [x] **Resource.SourcePath**: Struct with `.Rel` (root-relative, used for I/O) and `.Abs` (absolute, used for URIs/display/logging). Both fields computed eagerly at construction time.

- [x] **t.tmp() helper**: Full participant in root scoping. Base directory is `.devlore/tmp` within root. Implemented following the same context-aware pattern as `recovery.Site`. The entire codebase operates within root bounds.

- [x] **Backup interaction**: Verified — Backup creates a peer file (appends suffix to original path). Stays within root. No changes needed.
