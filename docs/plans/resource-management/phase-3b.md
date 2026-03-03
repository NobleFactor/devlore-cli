# Phase 3b: Complete File Provider Input Migration + Reducer Signature

## Context

Phase 3 input migration is nearly complete. All compensable methods accept `Resource` inputs and return `Resource` results. This phase converts the remaining methods that still use `string` for path arguments or return `string` path values, changes the Reducer/Actor signature to receive `Resource` instead of `(path string, dirEntry os.DirEntry)`, and fixes the redundant checksum in Move.

## Design Rule: Resource Creation vs Resolution

**Providers are identity-unaware.** They operate on physical state and report results. The action layer (future phase) assigns identity via the NamespaceMap:

| Operation type | Namespace action | Examples |
|---|---|---|
| **Observe** (read existing state) | **Resolve** | WalkTree, Read, Exists |
| **Produce** (create/modify state) | **Shadow** | Write, Copy, Move dest, Link |
| **Consume** (input to mutation) | Caller resolves before passing | Move source, Remove, Backup |

**Resolve** says: "give me the current known resource for this URI, or discover it if unknown."
**Shadow** says: "I'm producing a new version — create a new ledger entry and update the namespace."

WalkTree is observation — it Resolves, never Shadows. Each file encountered is resolved through the NamespaceMap. If the Reducer pushes compensable mutations, those mutations Shadow.

## Files

| File | Action |
|------|--------|
| `pkg/op/provider/file/resource.go` | Mode field: `.Perm()` → full `os.FileMode` |
| `pkg/op/provider/file/provider.go` | Reducer/Actor signatures, WalkTree body, 7 method signatures, 2 internal methods, Move checksum |
| `pkg/op/provider/file/provider_test.go` | Update all call sites, reducer functions, assertions |
| `pkg/op/provider/file/gen/actions_test.go` | Verify — may need updates for Mkdir |
| `pkg/op/provider/starcode/provider.go:146` | WalkTree caller + visitor signature |
| `internal/writ/migrate_cmd.go:290,308` | Mkdir callers |
| `internal/writ/commands.go:1210` | Mkdir caller |

## Changes

### 1. Resource.Mode — store full os.FileMode

`resource.go:78` and `resource.go:209`: `info.Mode().Perm()` → `info.Mode()`.

Enables `resource.Mode.IsDir()` so Reducers can replace `dirEntry.IsDir()`. No downstream impact — Mode is only set (never read) by production code.

### 2. Reducer/Actor signature — Resource replaces (path, dirEntry)

```go
// Before
type Reducer func(initial any, path string, dirEntry os.DirEntry, stack *op.RecoveryStack) (result any, err error)

// After
type Reducer func(initial any, resource Resource, relativePath string, stack *op.RecoveryStack) (result any, err error)
```

```go
// Before
func Actor(fn func(path string, dirEntry os.DirEntry) error) Reducer

// After
func Actor(fn func(resource Resource, relativePath string) error) Reducer
```

WalkTree body: call `NewResource(absolutePath)` for each entry, pass `resource` + `relativePath` to Reducer.

Callers:
- `starcode/provider.go:146` — `entry.IsDir()` → `resource.Mode.IsDir()`
- 11 test reducers — update parameter names and `IsDir()` calls

### 3. Remaining string→Resource in provider.go

| Method | Change |
|--------|--------|
| `Exists(fileResource Resource)` | Rename param → `resource` |
| `IsDir(path string)` | → `IsDir(resource Resource)`, body: `resource.SourcePath` |
| `IsFile(path string)` | → `IsFile(resource Resource)`, body: `resource.SourcePath` |
| `Mkdir(path string, mode) (string, error)` | → `Mkdir(resource Resource, mode) (Resource, error)` |
| `WalkTree(root string, ...)` | → `WalkTree(root Resource, ...)` |
| Remove/RemoveAll/Unlink `pruneBoundary string` | → `pruneBoundary Resource` |
| Move | `checksumFile(source.SourcePath)` → `source.Checksum` |
| `prepareWrite(path string)` | → `prepareWrite(resource Resource)` |
| `write(path string, ...)` | → `write(resource Resource, ...)` |

### 4. External callers

- `migrate_cmd.go:290,308` — `fp.Mkdir(filepath.Dir(layerDir), 0o755)` → wrap in `Resource{SourcePath: ...}`
- `commands.go:1210` — `fp.Mkdir(destDir, 0o755)` → wrap in `Resource{SourcePath: ...}`
- `starcode/provider.go:146` — WalkTree root wrap + visitor signature update

### What stays as string

- `Name(path string)`, `Parent(path string)`, `Join(parts ...string)` — pure path manipulation
- `Glob(pattern string, ...)` — glob pattern
- `backupSuffix`, `content` — data values
- `recovery.go` internals — private

## Execution order

1. `resource.go` — Mode field change
2. `provider.go` — Reducer/Actor/WalkTree, then remaining method signatures (sequential)
3. `provider_test.go` — all call sites and reducer functions
4. External callers — starcode, migrate_cmd, commands
5. `make vet && make test`

## Verification

```bash
make vet     # no vet issues
make test    # all tests pass
```
