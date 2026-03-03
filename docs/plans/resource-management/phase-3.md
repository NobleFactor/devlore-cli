# Phase 3: File Provider — Complete Input Migration

## Context

Phase 3 converts all file provider method inputs that identify external
entities (paths) from `string` to `Resource`. The outputs are already
`Resource` for Copy, WriteText, WriteBytes, and Read. This phase finishes
the input side so that provider methods always receive typed `file.Resource`
values — either resolved (existing file with metadata) or pending
(destination path with empty metadata).

Phase 2 shipped Graph integration: `Resources`/`Namespace` fields on Graph,
`FillSlot` implicit edge creation via `extractResource`, and removal of the
dead `backup` annotation.

The constructor registry (`file/resource.go:init()`) already handles
`string → file.Resource` coercion via `coerceSlotValue`. When Starlark
passes a string to a slot expecting `Resource`, the reflection bridge
invokes `NewResource(path)` automatically. This means the generated code
(`gen/*.gen.go`) does not need to change — the reflection bridge picks up
the new method signatures at runtime.

**Repo**: devlore-cli
**Branch**: `feature/resource-management-phase-3`

## Current Signatures vs. Target

| Method | Current Input | Target Input | Return Change |
|--------|-------------|-------------|---------------|
| `Copy` | `Resource, string, FileMode` | `Resource, Resource, FileMode` | No |
| `WriteText` | `string, string, FileMode` | `Resource, string, FileMode` | No |
| `WriteBytes` | `string, string, FileMode` | `Resource, string, FileMode` | No |
| `Read` | `string` | `Resource` | No |
| `Exists` | `Resource` | No change | No |
| `Link` | `string, string` | `Resource, Resource` | `string` → `Resource` |
| `Move` | `string, string` | `Resource, Resource` | `string` → `Resource` |
| `Backup` | `string, string` | `Resource, string` | `string` → `Resource` |
| `Remove` | `string, bool, string` | `Resource, bool, string` | No |
| `RemoveAll` | `string, bool, string` | `Resource, bool, string` | No |
| `Unlink` | `string, bool, string` | `Resource, bool, string` | No |

The pattern: every parameter identifying an external entity (a path)
becomes `Resource`. Configuration values (`content`, `mode`, `prune`,
`pruneBoundary`, `backupSuffix`) stay unchanged.

## Changes

### Provider Method Input Migration (`pkg/op/provider/file/provider.go`)

Each method that currently takes a `string` path parameter changes to
take `Resource`. The implementation extracts `.SourcePath` where it
previously used the raw string.

**Read**: `Read(path string)` → `Read(path Resource)`
- Implementation: `return NewResource(path.SourcePath)` (re-stats the
  file to populate full metadata)

**WriteText / WriteBytes**: `(destination string, ...)` → `(destination Resource, ...)`
- Delegate to internal `write(destination.SourcePath, ...)` — the
  internal `write` method keeps its `string` signature

**Copy**: `(sourceFile Resource, destinationFilename string, ...)` → `(sourceFile Resource, destinationFilename Resource, ...)`
- Calls `prepareWrite(destinationFilename.SourcePath)` and uses
  `destinationFilename.SourcePath` for `os.OpenFile`

**Link**: `(source, path string)` → `(source, path Resource)` returning `(Resource, map, error)`
- Uses `source.SourcePath` as the symlink target and `path.SourcePath`
  as the symlink location
- Return type changes from `string` to `Resource`; successful cases
  call `NewResource(path.SourcePath)` for the result

**Move**: `(source, destination string)` → `(source, destination Resource)` returning `(Resource, map, error)`
- Uses `.SourcePath` for `os.Stat`, `checksumFile`, `os.Rename`
- Return type changes from `string` to `Resource`; calls
  `NewResource(destination.SourcePath)` for the result

**Backup**: `(path, backupSuffix string)` → `(path Resource, backupSuffix string)` returning `(Resource, map, error)`
- Uses `path.SourcePath` for the backup path computation and `os.Rename`
- Return type changes from `string` to `Resource`; calls
  `NewResource(backupPath)` for the result

**Remove / RemoveAll / Unlink**: `(path string, ...)` → `(path Resource, ...)`
- Extract `path.SourcePath` and delegate to `moveToRecovery`
- Return type (`Tombstone`) unchanged

### Internal Method Update (`pkg/op/provider/file/provider.go`)

**`prepareWrite(path string)`** — signature unchanged. One internal call
changes: `p.Remove(result.SourcePath, false, "")` becomes
`p.Remove(result, false, "")` since `result` is already a `Resource`
from `NewResource(path)` and `Remove` now takes `Resource`.

**`write(path string, ...)`** — signature unchanged. `WriteText` and
`WriteBytes` extract `.SourcePath` before calling.

**`moveToRecovery(path string, ...)`** — signature unchanged. `Remove`,
`RemoveAll`, and `Unlink` extract `.SourcePath` before calling.

### Direct Callers Outside File Provider

Three files call `Link`/`Move` directly on `file.Provider` with string
arguments. Each call site wraps its string arguments in
`file.Resource{SourcePath: path}`:

**`internal/writ/migrate_cmd.go`**:
- Line 300: `fp.Link(sourceRoot, layerDir)` →
  `fp.Link(file.Resource{SourcePath: sourceRoot}, file.Resource{SourcePath: layerDir})`
- Line 318: `fp.Move(sourceRoot, layerDir)` →
  `fp.Move(file.Resource{SourcePath: sourceRoot}, file.Resource{SourcePath: layerDir})`

**`internal/writ/commands.go`**:
- Line 1220: `fp.Move(filePath, destPath)` →
  `fp.Move(file.Resource{SourcePath: filePath}, file.Resource{SourcePath: destPath})`
- Line 1232: `fp.Link(destPath, filePath)` →
  `fp.Link(file.Resource{SourcePath: destPath}, file.Resource{SourcePath: filePath})`

**`internal/writ/migrate/execute.go`**:
- Line 72: `fp.Move(source, target)` →
  `fp.Move(file.Resource{SourcePath: source}, file.Resource{SourcePath: target})`

All callers discard Link/Move return values with `_`, so the return
type change from `string` to `Resource` requires no additional updates.

### Generated Code

No hand edits to generated files. The reflection bridge
(`RegisterReflectedActions`, `WrapReceiver`, `WrapPlanned`) discovers
method signatures at runtime via `reflect.Method`. The constructor
registry already converts `string → file.Resource` via `coerceSlotValue`.
Param names in `params.gen.go` do not encode types — they remain
unchanged.

After provider signatures change, regenerate via
`star devlore actions generate --source=pkg/op/provider/file ...` to
verify the generator still works. If it doesn't, the fix is in the
generator (noblefactor-ops), not in generated output.

### Tests (`pkg/op/provider/file/provider_test.go`)

Every test that calls a changed method updates its call site:

**String → Resource wrapping at call sites:**
- `p.Read(path)` → `p.Read(Resource{SourcePath: path})`
- `p.WriteText(path, content, mode)` → `p.WriteText(Resource{SourcePath: path}, content, mode)`
- `p.WriteBytes(path, content, mode)` → `p.WriteBytes(Resource{SourcePath: path}, content, mode)`
- `p.Copy(blob, path, mode)` → `p.Copy(blob, Resource{SourcePath: path}, mode)`
- `p.Link(source, path)` → `p.Link(Resource{SourcePath: source}, Resource{SourcePath: path})`
- `p.Move(source, dest)` → `p.Move(Resource{SourcePath: source}, Resource{SourcePath: dest})`
- `p.Backup(path, suffix)` → `p.Backup(Resource{SourcePath: path}, suffix)`
- `p.Remove(path, prune, boundary)` → `p.Remove(Resource{SourcePath: path}, prune, boundary)`
- `p.RemoveAll(path, prune, boundary)` → `p.RemoveAll(Resource{SourcePath: path}, prune, boundary)`
- `p.Unlink(path, prune, boundary)` → `p.Unlink(Resource{SourcePath: path}, prune, boundary)`

**Return type assertions for Link, Move, Backup:**
- `result != dst` → `result.SourcePath != dst`
- `result != linkPath` → `result.SourcePath != linkPath`
- `strings.HasPrefix(result, ...)` → `strings.HasPrefix(result.SourcePath, ...)`

## Files

| File | Action | Purpose |
|------|--------|---------|
| `docs/plans/resource-management/phase-3.md` | Create | This document |
| `pkg/op/provider/file/provider.go` | Modify | Convert all method inputs to Resource; update Link/Move/Backup return types |
| `pkg/op/provider/file/provider_test.go` | Modify | Update all test call sites and return assertions |
| `internal/writ/migrate_cmd.go` | Modify | Wrap Link/Move string args in Resource |
| `internal/writ/commands.go` | Modify | Wrap Move/Link string args in Resource |
| `internal/writ/migrate/execute.go` | Modify | Wrap Move string args in Resource |

## What This Does NOT Touch

- `pkg/op/provider/file/resource.go` — `NewResource` continues using
  `fmt.Sprintf("file://%s", path)` for URI generation (will switch to
  `op.ResourceURI` in a later cleanup, not this phase)
- `pkg/op/provider/file/recovery.go` — `moveToRecovery` and
  `restoreFromRecovery` keep their `string` path signatures
- `pkg/op/provider/file/helpers.go` — unchanged
- `pkg/op/provider/file/gen/*.gen.go` — no hand edits; regenerate only
  to verify compatibility
- `internal/execution/` — executor changes are Phase 4
- No codegen tool changes (noblefactor-ops)
- Compensate methods (`CompensateCopy`, `CompensateLink`, etc.) — these
  receive `map[string]any` undo state and are unchanged

## Verification

```bash
make test       # all existing + updated tests pass
make vet        # no vet issues
make check      # full check suite
```

Specifically verify:
- All file provider tests pass with Resource inputs
- Link/Move/Backup tests assert on `result.SourcePath` (not `result`)
- Writ callers compile and pass: `internal/writ/...`
- Constructor coercion still works: Starlark string → `coerceSlotValue` → `file.Resource`
