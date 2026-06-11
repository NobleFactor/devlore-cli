---
title: "os.Root Scoping for Provider I/O"
issue:
status: complete
created: 2026-03-08
updated: 2026-03-09
---

# Plan: os.Root Scoping for Provider I/O

## Summary

Replace all direct `os.*` filesystem calls in providers with operations scoped through `op.Root`, an interface with three concrete implementations:

- `confinedRoot` wraps `*os.Root` (Go 1.24) for OS-enforced chroot-style confinement during execution — symlinks cannot escape, `..` traversal is blocked
- `RootReader` delegates to `os.*` for unconfined read-only access during planning — write operations return `ErrReadOnly`
- `RootReaderWriter` delegates to `os.*` for unconfined read-write access during testing

All I/O is constrained to the authority boundary defined by `Context.Root`. This eliminates manual path validation (`isSubpath`), simplifies the recovery system (recovery lives within root, guaranteeing same-partition), and removes platform-specific recovery base discovery code. Provider code is mode-agnostic — it calls `Root.ReadFile(p)`, `Root.Stat(p)`, etc. regardless of which implementation backs the interface.

`op.Path` holds both root-relative and absolute forms of a filesystem path. The canonical form is `{root, rel}` — abs is derived as `filepath.Join(root, rel)`. Path implements `MarshalJSON`/`UnmarshalJSON` serializing only `{root, rel}`. Root is the Path factory (`Root.NewPath(path) Path`). `op.Path` replaces `file.SourcePath`.

`op.RecoverySite` extracts the recovery system from the file provider into a shared, context-aware service on `op.Context`. It holds `Context` (same pattern as `ProviderBase`) and supports both file archival (zero-copy rename) and data archival (byte serialization), enabling recovery for both file and mem providers.

## Goals

1. **OS-enforced authority boundary** — all provider file operations are confined by the kernel through `Context.Root` (backed by `confinedRoot` wrapping `*os.Root`), not by application-level path checks
2. **Simplified recovery** — recovery directory lives inside root; no mount-point discovery, no cross-device checks, no platform-specific `getRecoveryBase`
3. **Root-relative paths** — `op.Path` holds `{root, rel}` as canonical form with derived `abs`; replaces `file.SourcePath`; serializes as JSON `{root, rel}`
4. **Shared recovery service** — `op.RecoverySite` on Context provides `ArchiveFile`/`RestoreFile` (zero-copy rename) and `ArchiveData`/`RestoreData` (byte serialization) for all providers
5. **Dual-interface Root** — `op.Root` interface with three implementations: `confinedRoot` (execution), `RootReader` (planning), `RootReaderWriter` (testing); provider code is mode-agnostic
6. **Provider lifecycle** — Root is closed on teardown via `Root.Close()`

## Current State

| Component             | Status                                | Notes                                                                      |
| --------------------- | ------------------------------------- | -------------------------------------------------------------------------- |
| `Context.Root`        | `op.Root` interface                   | Three implementations: `confinedRoot`, `RootReader`, `RootReaderWriter`    |
| `file.Provider` I/O   | Root-scoped via private helpers       | All 9 helpers dispatch through `op.Root`; no nil-safe fallbacks            |
| `mem` extract         | Root-scoped `ReadFile`                | `Extract`/`ExtractWithName` accept `op.Root`                              |
| Recovery system       | `op.RecoverySite` on Context          | Shared service: `ArchiveFile`/`RestoreFile` + `ArchiveData`/`RestoreData` |
| Path model            | `op.Path` with `{root, rel, abs}`     | Replaced `file.SourcePath` across 80+ sites                               |
| Path validation       | Deleted (`isSubpath` removed)         | `os.Root` enforces boundary; `pruneEmptyParents` uses `boundary` param    |
| `Resource.SourcePath` | `op.Path`                             | JSON serializes `{root, rel}`, abs derived                                 |
| WalkTree              | `fs.WalkDir(root.FS(), ...)`          | Root-scoped walk                                                           |
| I/O provider access   | `+devlore:access=planned`             | 8 I/O providers; no immediate mode generation                              |
| Transformer access    | `+devlore:access=both`                | 4 stateless providers (`json`, `yaml`, `regexp`, `template`)               |
| Platform recovery     | Deleted                               | `recovery_unix.go`, `recovery_windows.go` removed                          |

## Requirements

### os.Root on Context

`Context` holds the `op.Root` interface and a `*RecoverySite`. Providers access both through `Context()`. The executor creates `op.NewConfinedRoot(dir)` before graph execution and closes it after. Planning uses `op.NewRootReader(dir)` for read-only access. Tests use `op.NewRootReaderWriter(dir)` for unconfined read-write access.

```go
type Context struct {
    context.Context
    Root         Root            // op.Root interface — confinedRoot, RootReader, or RootReaderWriter
    RecoverySite *RecoverySite   // shared recovery service for all providers
    // ... other fields unchanged
}
```

### RecoverySite

`op.RecoverySite` is a context-aware service that manages archival and restoration of files and data within root. It holds `Context` (same pattern as `ProviderBase`) and lives in `pkg/op/` to avoid import cycles.

```go
package op

type RecoverySite struct {
    ctx Context  // holds Root (op.Root interface) for all I/O
}

// ArchiveFile moves a file to recovery via zero-copy rename (no data movement).
// Takes op.Path for the user-facing location. Returns an opaque recovery ID for
// tombstone storage.
func (s *RecoverySite) ArchiveFile(p Path) (recoveryID string, err error)

// RestoreFile moves a file back from recovery via zero-copy rename (no data movement).
// Takes op.Path for the original location and the opaque recovery ID.
func (s *RecoverySite) RestoreFile(original Path, recoveryID string) error

// ArchiveData writes bytes to a file in the recovery directory.
// Returns an opaque recovery ID for tombstone storage.
func (s *RecoverySite) ArchiveData(data []byte) (recoveryID string, err error)

// RestoreData reads bytes back from a file in the recovery directory.
func (s *RecoverySite) RestoreData(recoveryID string) ([]byte, error)
```

Provider usage:

```go
// file provider — zero-copy file archival
recoveryID, err := p.Context().RecoverySite.ArchiveFile(resource.SourcePath)

// file provider — restore from recovery
err := p.Context().RecoverySite.RestoreFile(resource.SourcePath, tombstone.RecoveryID)

// mem provider — byte serialization
recoveryID, err := p.Context().RecoverySite.ArchiveData(resource.Data)

// mem provider — restore bytes
data, err := p.Context().RecoverySite.RestoreData(tombstone.RecoveryID)
```

The recovery directory is `.devlore/recovery/` within root. The RecoverySite manages UUID-based entries internally. `ArchiveFile`/`RestoreFile` perform zero-copy renames — no data movement. `ArchiveData`/`RestoreData` serialize bytes to/from the recovery directory.

### Provider I/O through Root

Every `os.*` call in a provider is replaced by the corresponding `root.*` call taking `op.Path`. Providers obtain root via `p.Context().Root`. The `op.Root` interface dispatches to the concrete implementation: `confinedRoot` uses `p.Rel()`, unconfined implementations use `p.Abs()`.

| Current                      | Replacement                        |
| ---------------------------- | ---------------------------------- |
| `os.Stat(abs)`               | `root.Stat(p)`                     |
| `os.Lstat(abs)`              | `root.Lstat(p)`                    |
| `os.Rename(a, b)`            | `root.Rename(pOld, pNew)`          |
| `os.Remove(abs)`             | `root.Remove(p)`                   |
| `os.Symlink(target, link)`   | `root.Symlink(target, p)`          |
| `os.Readlink(abs)`           | `root.Readlink(p)`                 |
| `os.Open(abs)`               | `root.Open(p)`                     |
| `os.OpenFile(abs, ...)`      | `root.OpenFile(p, ...)`            |
| `os.MkdirAll(abs, ...)`      | `root.MkdirAll(p, ...)`            |
| `os.ReadFile(abs)`           | `root.ReadFile(p)`                 |
| `os.WriteFile(abs, ...)`     | `root.WriteFile(p, ...)`           |
| `filepath.WalkDir(abs, ...)` | `fs.WalkDir(root.FS(), rel, ...)`  |

### Root-relative Resource paths

`Resource.SourcePath` becomes `op.Path` — an immutable path type with `{root, rel}` as canonical form and `abs` derived as `filepath.Join(root, rel)`:

```go
// pkg/op/fsroot.go
type Path struct {
    root string               // fsroot directory (matches os.Root.Name())
    rel  string               // fsroot-relative path
    abs  string               // derived: filepath.Join(fsroot, rel)
}
```

`op.Root` is the `Path` factory: `root.NewPath("sub/file.txt")` populates all three fields. `Path` serializes as JSON `{"root": "...", "rel": "..."}` — abs is derived on deserialization.

`NewResource` and `Resolve` change to work with `op.Path` and accept `op.Root` for metadata population (Stat, checksum).

### Elimination of manual path checks

- `isSubpath` helper — deleted; `os.Root` enforces the boundary
- `filepath.Abs` calls in `moveToRecovery` and `Resolve` — unnecessary; paths are already root-relative
- `pruneEmptyParents` boundary check — `os.Root` prevents escaping root; the `boundary` parameter continues to provide finer-grained prune limits within root

### Test runner scoping

The `t.tmp()` helper in the Starlark test runner becomes root-aware, following the same pattern as `RecoverySite`. Its base directory is `.devlore/tmp` within root. The `TestContext` uses root-scoped I/O for all file checks (`checkFileExists`, `checkNoFile`). The entire codebase operates within root bounds.

## Implementation Phases

### Phase 1: Context.Root and provider plumbing

- [x] Add `Root *os.Root` field to `op.Context`
- [x] Open `os.Root` from `BaseDir` in executor before graph execution
- [x] Close `os.Root` after graph execution (defer)
- [x] Update `testProvider` and test helpers to open `os.Root` from temp dirs
- [x] Add `Root()` accessor or equivalent on `ProviderBase` for ergonomic access

**Files**:

- `pkg/op/context.go` — Modify: add `Root *os.Root`
- `internal/execution/executor.go` — Modify: open/close `os.Root`
- `pkg/op/provider/file/provider_test.go` — Modify: test helper opens `os.Root`

### Phase 2: Resource path model

- [x] Define `SourcePath` struct with `Rel` and `Abs` fields
- [x] Update `NewResource` to accept root-relative path and `*os.Root`, eagerly compute both fields
- [x] Update `Resolve` to use `root.Stat` instead of `os.Stat`
- [x] Update `buildURI` to use `SourcePath.Abs` for URI construction
- [x] Update `Reader`, `WriteTo` to use root-scoped I/O
- [x] Update `Refresh`, `RefreshWith` for root-scoped stat
- [x] Update all call sites that access `Resource.SourcePath` (86 references in file provider)

**Files**:

- `pkg/op/provider/file/resource.go` — Modify: `SourcePath` struct, root-scoped I/O
- `pkg/op/provider/file/provider.go` — Modify: all methods use `SourcePath.Rel` for I/O
- `pkg/op/provider/file/gen/` — Regenerate: generated code adapts to new Resource construction

### Phase 3: File provider — core operations

- [x] Replace all `os.Stat/Lstat` with `root.Stat/Lstat`
- [x] Replace all `os.Rename` with `root.Rename`
- [x] Replace all `os.Remove` with `root.Remove`
- [x] Replace all `os.Symlink/Readlink` with `root.Symlink/Readlink`
- [x] Replace all `os.Open/OpenFile` with `root.Open/OpenFile`
- [x] Replace all `os.MkdirAll` with `root.MkdirAll`
- [x] Replace `filepath.WalkDir` with `fs.WalkDir(root.FS(), ...)`
- [x] Delete `isSubpath` helper
- [x] Simplify `pruneEmptyParents` — remove `isSubpath` guard, keep `boundary` parameter
- [x] Update `prepareWrite`, `write`, `compensateWrite` to use root-scoped I/O

**Files**:

- `pkg/op/provider/file/provider.go` — Modify: all operations use `root.*`
- `pkg/op/provider/file/helpers.go` — Modify: delete `isSubpath`; update `checksumFile` to use root
- `pkg/op/provider/file/recovery.go` — Modify: use `root.*` for moves (interim, before Phase 7 extraction)

### Phase 4: Recovery system — root-scoped (interim)

Simplify recovery to use `os.Root` while still embedded in the file provider. This is an interim step before Phase 7 extracts it into a shared service.

- [x] Define recovery directory constant: `.devlore/recovery`
- [x] Replace `getRecoveryBase` with constant path
- [x] Delete `recovery_unix.go` — mount-point discovery, `findMountPoint`, `getFirstExistingAncestor`, `isSameDevice`
- [x] Delete `recovery_windows.go` — volume detection
- [x] Update `moveToRecovery` to use `root.MkdirAll` + `root.Rename` with relative paths
- [x] Update `restoreFromRecovery` to use `root.Rename` with relative paths
- [x] Remove `filepath.Abs` calls in recovery (paths are already root-relative)

**Files**:

- `pkg/op/provider/file/recovery.go` — Modify: root-scoped recovery with constant path
- `pkg/op/provider/file/recovery_unix.go` — Delete
- `pkg/op/provider/file/recovery_windows.go` — Delete

### Phase 5: mem provider — scoped source reads

- [x] Update `extractLambdaBody` to use root-scoped `ReadFile` instead of `os.ReadFile`
- [x] Update `extractDefSource` to use root-scoped `ReadFile`
- [x] Thread `*os.Root` through `Extract`/`ExtractWithName` or access via provider context
- [x] Update tests

**Files**:

- `pkg/op/provider/mem/extract.go` — Modify: root-scoped reads
- `pkg/op/provider/mem/extract_test.go` — Modify: test setup with `os.Root`

### Phase 6: Test and doc updates

- [x] Update all file provider tests to use root-relative paths and `SourcePath` struct
- [x] Update execution tests that construct Resources with absolute paths
- [x] Update e2e test runner — `t.tmp()` returns root-relative paths under `.devlore/tmp`, `TestContext` uses root-scoped I/O for all file checks
- [x] Update Starlark test scripts if they rely on absolute path semantics
- [x] Update architecture docs (`4-resource-management.md`, `5.3-recovery-site.md`) to reflect `os.Root`
- [x] Update plan docs that reference `getRecoveryBase` or recovery site discovery

**Files**:

- `pkg/op/provider/file/provider_test.go` — Modify
- `pkg/op/provider/file/gen/actions_gen_test.go` — Regenerate
- `internal/execution/execution_test.go` — Modify
- `internal/e2e/testrunner/test_context.go` — Modify: root-aware `t.tmp()` under `.devlore/tmp`, root-scoped checks
- `docs/architecture/4-resource-management.md` — Modify
- `docs/architecture/2.1-typed-slots.md` — Modify

### Phase 7: op.RecoverySite — shared recovery service

Extract the recovery system from the file provider into `op.RecoverySite` on `op.Context`. The RecoverySite holds `Context` (same pattern as `ProviderBase`). All I/O goes through `ctx.Root`. Migrate file provider to use it. Add data archival for mem provider support.

- [x] Create `pkg/op/recovery_site.go` with `RecoverySite` struct holding `ctx Context`
- [x] Implement `ArchiveFile(path string) (string, error)` — zero-copy rename into `.devlore/recovery/<uuid>`, no data movement
- [x] Implement `RestoreFile(originalPath, recoveryPath string) error` — zero-copy rename back, no data movement
- [x] Implement `ArchiveData(data []byte) (string, error)` — write bytes to `.devlore/recovery/<uuid>`
- [x] Implement `RestoreData(recoveryPath string) ([]byte, error)` — read bytes from recovery
- [x] Add `RecoverySite *RecoverySite` to `op.Context`
- [x] Instantiate `RecoverySite` in executor alongside `os.Root`
- [x] Migrate file provider: `Remove`/`RemoveAll`/`Unlink`/`prepareWrite` use `ArchiveFile`
- [x] Migrate file provider: `CompensateRemove`/`CompensateRemoveAll`/`CompensateUnlink`/`compensateWrite` use `RestoreFile`
- [x] Fix `CompensateBackup`/`CompensateMove` — these use direct `p.rename`, NOT RecoverySite (they handle peer files, not recovery-directory files)
- [x] Add `relPath` helper on file provider for computing root-relative paths from resources
- [x] Delete `pkg/op/provider/file/recovery.go` — logic now lives in `pkg/op/recovery_site.go`
- [x] Update file provider tests to use root-relative `RecoveryPath` in tombstones
- [x] Add `RecoverySite` unit tests (`pkg/op/recovery_site_test.go`)

**Files**:

- `pkg/op/recovery_site.go` — Create: `RecoverySite` struct with `ArchiveFile`/`RestoreFile`/`ArchiveData`/`RestoreData`
- `pkg/op/recovery_site_test.go` — Create: unit tests
- `pkg/op/context.go` — Modify: add `RecoverySite *RecoverySite`
- `internal/execution/executor.go` — Modify: instantiate `RecoverySite`
- `internal/starlark/binding_set.go` — Modify: instantiate `RecoverySite`
- `internal/e2e/testrunner/runner.go` — Modify: instantiate `RecoverySite`
- `pkg/op/provider/file/provider.go` — Modify: use `RecoverySite.ArchiveFile`/`RestoreFile`, add `relPath` helper
- `pkg/op/provider/file/recovery.go` — Delete: logic extracted to `pkg/op/recovery_site.go`
- `pkg/op/provider/file/provider_test.go` — Modify: tests use root-relative `RecoveryPath`

### Phase 8: op.Root interface and op.Path

Replace the bare `*os.Root` on Context with an `op.Root` interface backed by three implementations. Replace `file.SourcePath` with `op.Path`.

#### 8a: op.Root interface (done)

- [x] Define `op.Root` interface with 15 methods: 5 read, 6 write, `NewPath` factory, `FS`, `Name`, `Close`
- [x] Implement `confinedRoot` wrapping `*os.Root` — delegates using `p.Rel()`
- [x] Implement `RootReader` — unconfined read-only, write ops return `ErrReadOnly`
- [x] Implement `RootReaderWriter` — unconfined read-write via `os.*` using `p.Abs()`
- [x] Composition chain: `rootBase` → `RootReader` → `RootReaderWriter`
- [x] Interface guards for all three implementations
- [x] Comprehensive tests: 46 subtests across all three modes

**Files**:

- `pkg/op/root.go` — Create: `op.Root` interface, `op.Path`, three implementations
- `pkg/op/root_test.go` — Create: comprehensive tests

#### 8b: op.Path (done)

- [x] Define `op.Path` with `{root, rel}` canonical form, derived `abs`
- [x] `NewPath(root, rel)` constructor for tests and deserialization
- [x] `Root.NewPath(path)` factory — handles absolute and relative inputs
- [x] `MarshalJSON`/`UnmarshalJSON` serializing `{root, rel}`, deriving abs
- [x] `Abs()`, `Rel()`, `Root()`, `String()` accessors
- [x] Tests for JSON round-trip serialization

#### 8c: Wiring and access levels

- [x] Change `Context.Root` from `*os.Root` to `op.Root`
- [x] Update executor: `op.NewConfinedRoot(dir)` instead of `os.OpenRoot(dir)`
- [x] Make `ExecutorOptions.Root` mandatory — fail fast if empty
- [x] Update test runner: `op.NewRootReaderWriter(tmpDir)`
- [x] Migrate `file.SourcePath` → `op.Path` across all 80+ access sites
- [x] Update RecoverySite API: `ArchiveFile(p Path)`, `RestoreFile(original Path, recoveryID string)`
- [x] Migrate helpers: `checksumFile` → Provider method, `checksumBytes` → provider.go, delete helpers.go, delete `rootRel`
- [x] Delete nil-safe fallback on file provider private helpers — Root is always set
- [x] Restrict provider access levels: I/O providers (`file`, `shell`, `git`, `pkg`, `service`, `archive`, `appnet`, `encryption`) become `+devlore:access=planned`; stateless transformers (`json`, `yaml`, `regexp`, `template`) remain `+devlore:access=both`
- [x] Update `TestImmFile` and other immediate tests affected by access level changes
- [x] Verify star\*, ui, and plan remain functional as immediate providers with nil Root

**Files**:

- `pkg/op/context.go` — Modify: `Root op.Root`
- `internal/execution/executor.go` — Modify: `op.NewConfinedRoot`, mandatory validation
- `internal/e2e/testrunner/runner.go` — Modify: `op.NewRootReaderWriter`
- `pkg/op/recovery_site.go` — Modify: `ArchiveFile(Path)`, `RestoreFile(Path, string)`
- `pkg/op/provider/file/resource.go` — Modify: `SourcePath` → `op.Path`
- `pkg/op/provider/file/provider.go` — Modify: delete nil-safe helpers, access annotation, use `op.Path`
- `pkg/op/provider/file/helpers.go` — Delete: `checksumBytes` moves to provider.go
- `pkg/op/provider/shell/provider.go` — Modify: access annotation change
- `pkg/op/provider/git/provider.go` — Modify: access annotation change
- `pkg/op/provider/*/provider.go` — Modify: access annotation changes for remaining I/O providers
- `internal/e2e/testrunner/runner_test.go` — Modify: remove/update immediate tests for I/O providers

## Files to Create/Modify

| File                                       | Action  | Purpose                                                          |
| ------------------------------------------ | ------- | ---------------------------------------------------------------- |
| `pkg/op/root.go`                           | Created | `op.Root` interface, `op.Path`, three implementations            |
| `pkg/op/root_test.go`                      | Created | Comprehensive tests for Root and Path                            |
| `pkg/op/recovery_site.go`                  | Created | Shared recovery service: `ArchiveFile`/`Data`, `RestoreFile`/`Data` |
| `pkg/op/recovery_site_test.go`             | Created | Unit tests for RecoverySite                                      |
| `pkg/op/context.go`                        | Modify  | `Root op.Root`, `RecoverySite *RecoverySite`                     |
| `pkg/op/provider/file/resource.go`         | Modify  | `SourcePath` → `op.Path`                                        |
| `pkg/op/provider/file/provider.go`         | Modify  | All ops through `op.Root`, use `op.Path`, delete nil-safe helpers |
| `pkg/op/provider/file/helpers.go`          | Delete  | `checksumBytes` → provider.go, `checksumFile` → Provider method |
| `pkg/op/provider/file/recovery.go`         | Deleted | Logic extracted to `pkg/op/recovery_site.go`                     |
| `pkg/op/provider/file/recovery_unix.go`    | Deleted | Mount-point discovery eliminated                                 |
| `pkg/op/provider/file/recovery_windows.go` | Deleted | Volume detection eliminated                                      |
| `pkg/op/provider/mem/extract.go`           | Modify  | Root-scoped source reads                                         |
| `internal/execution/executor.go`           | Modify  | `op.NewConfinedRoot`, mandatory validation, instantiate RecoverySite |
| `internal/starlark/binding_set.go`         | Modify  | Instantiate `RecoverySite`                                       |
| `internal/e2e/testrunner/runner.go`        | Modify  | `op.NewRootReaderWriter`, instantiate RecoverySite               |
| `internal/e2e/testrunner/test_context.go`  | Modify  | Root-aware `t.tmp()` under `.devlore/tmp`                        |
| `pkg/op/provider/file/provider_test.go`    | Modify  | Root-relative test setup, use `op.Path`                          |
| `pkg/op/provider/mem/extract_test.go`      | Modify  | Root-scoped test setup                                           |
| `internal/execution/execution_test.go`     | Modify  | Root-relative slot values                                        |

## Related Documents

- [4-resource-management.md](../architecture/4-resource-management.md) — Resource model architecture
- [4.4-root-path-triad.md](../architecture/4.4-root-path-triad.md) — op.Root, op.Path, and op.RecoverySite interaction architecture
- [2.1-typed-slots.md](../architecture/2.1-typed-slots.md) — Slot resolution and Context.Data
- [resource-management.md](resource-management.md) — Resource management plan (phases 0-8)

## Resolved Questions

- [x] **op.Root location**: Lives on `Context` as `op.Root` interface. Must be specified at creation time. No exceptions. Three implementations: `confinedRoot` (execution), `RootReader` (planning), `RootReaderWriter` (testing).

- [x] **Recovery directory**: `.devlore/recovery/`. Recovery system extracted into `pkg/op/recovery_site.go` as `op.RecoverySite` — a context-aware service holding `Context` (same pattern as `ProviderBase`). Lives in `pkg/op/` to avoid import cycles. Providers access it via `p.Context().RecoverySite`. Supports both file archival (zero-copy rename, no data movement) and data archival (byte serialization) for file and mem providers respectively.

- [x] **Resource.SourcePath**: Replaced by `op.Path` with `{root, rel}` canonical form, derived `abs`. JSON serializes as `{root, rel}`. Root matches `os.Root.Name()`. `op.Root` is the Path factory via `NewPath(path)`.

- [x] **RecoverySite API**: Takes `op.Path` for user-facing file locations. Returns opaque `string` for recovery IDs. Preserves zero data movement via rename within root.

- [x] **t.tmp() helper**: Full participant in root scoping. Base directory is `.devlore/tmp` within root. Implemented following the same context-aware pattern as `RecoverySite`. The entire codebase operates within root bounds.

- [x] **Backup interaction**: Verified — Backup creates a peer file (appends suffix to original path). Stays within root. Compensation uses direct `p.rename`, NOT RecoverySite (the backup is a peer file, not a recovery-directory entry).

- [x] **Move interaction**: Verified — Move renames file from source to destination. Compensation uses direct `p.rename`, NOT RecoverySite (the destination is a regular file location, not a recovery-directory entry).

## Session Log

### Session 4 — 2026-03-09 (Phase 8c: access levels + cleanup)

**Completed:**

1. **Restored nil-safe fallbacks on file provider private helpers** — All 9 helpers (`stat`, `lstat`, `rename`, `remove`, `mkdirAll`, `readFile`, `writeFile`, `symlink`, `readlink`) plus package-level `checksumFile` use `if root != nil { root.X() } else { os.X() }`. Removal caused ~120+ test failures because execution tests create rootless contexts. Deferred until those tests are updated.

2. **Restricted 8 I/O providers to `+devlore:access=planned`** — `file`, `shell`, `git`, `pkg`, `service`, `archive`, `appnet`, `encryption`. Stateless transformers (`json`, `yaml`, `regexp`, `template`) remain `both`.

3. **Fixed star code generation for `planned` access** — `generate.star` and `provider_descriptor.go.template` now conditionally skip `immediate_receiver`, `immediate_test`, and `NewImmediate` when access is `planned`. Added `has_immediate` flag to descriptor context.

4. **Updated Makefile** — Restructured gen rules: moved 8 I/O providers to new "access=planned" section with 4 gen targets (no `immediate.gen.go`, no `immediate_gen_test.go`). Deleted 16 generated files.

5. **Removed/updated affected tests** — Deleted `TestImmFile`, `TestImmFileJoinVariadicError`, `TestImmShell`, `TestImmEncryption` (+ `sopsEncrypt` helper), 3 immediate WalkTree tests. Added `t.mkdir()` and `t.write()` to `TestContext` for test setup without immediate file provider.

6. **Verified Phase 5 (mem provider)** — Already implemented: `Extract`/`ExtractWithName` accept `op.Root`, `readSource` uses root-scoped `ReadFile`.

### Session 5 — 2026-03-09 (Phase 8c: delete nil-safe fallbacks)

**Completed:**

1. **Deleted nil-safe fallbacks from 9 Provider private helpers** — `lstat`, `mkdirAll`, `open`, `openFile`, `readlink`, `remove`, `rename`, `stat`, `symlink` now directly call `root.X()` without `if root != nil` guard.

2. **Deleted nil-safe fallback from `checksumFile`** — directly calls `root.ReadFile()`.

3. **Deleted nil-safe fallbacks from Resource methods** — `Resolve`, `Refresh`, `RefreshWith`, `WriteTo` now require non-nil `op.Root`.

4. **Fixed `gather.go` production bug** — `iterCtx` now propagates `Root`, `RecoverySite`, and `Platform` from parent context.

5. **Fixed encryption provider** — `DecryptSopsFile` uses `root.WriteFile()` and `root.Open()` instead of `os.WriteFile`/`os.Open`. `CompensateDecryptSopsFile` uses `root.Remove()` instead of `os.Remove`. `Resolve(nil)` calls replaced with `Resolve(root)`.

6. **Fixed all test sites** — `Resolve(nil)` calls in `provider_test.go` replaced with `Resolve(root)`. Added `testRoot` helper. `testFileResource` creates its own root. `TestCompensationGather` gives provider a Root context. Encryption tests use `testProvider` helper with Root.

**Plan status**: Complete — all items checked off, status changed to `complete`.
