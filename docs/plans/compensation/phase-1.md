---
title: "Compensation Phase 1: File Provider"
parent: compensation.md
status: in-progress
created: 2026-02-16
updated: 2026-02-16
---

# Phase 1: File Provider (Reference Implementation)

## Summary

Add real compensation to the file provider — the largest provider (9 actions, 7
compensable). Establishes the pattern for all subsequent providers.

## Changes

### provider.go — Forward Method Signature Changes

7 compensable Forward methods gain a `map[string]any` state return:

| Method | Current Return | New Return |
|---|---|---|
| `Link` | `error` | `(map[string]any, error)` |
| `Copy` | `(string, error)` | `(string, map[string]any, error)` |
| `Backup` | `(string, error)` | `(string, map[string]any, error)` |
| `Unlink` | `error` | `(map[string]any, error)` |
| `Remove` | `error` | `(map[string]any, error)` |
| `Write` | `error` | `(map[string]any, error)` |
| `Move` | `error` | `(map[string]any, error)` |

Non-compensable methods (`Source`, `Mkdir`) unchanged.

### provider.go — Backward Methods Added

7 new `Compensate*` methods on `*Provider`:

| Method | Compensation Logic |
|---|---|
| `CompensateLink` | Remove symlink if new; restore previous target if overwritten |
| `CompensateCopy` | Remove file if new; restore previous content if overwritten |
| `CompensateBackup` | Move backup file back to original path |
| `CompensateUnlink` | Re-create symlink with saved target |
| `CompensateRemove` | Re-create file with saved content and mode |
| `CompensateWrite` | Remove file if new; restore previous content if overwritten |
| `CompensateMove` | Move file back from destination to source |

### provider.go — State Captured Per Method

| Forward | State Keys |
|---|---|
| `Link` | `path`, `existed_before`, `previous_target?` |
| `Copy` | `path`, `existed_before`, `previous_content?`, `previous_mode?` |
| `Backup` | `original_path`, `backup_path` |
| `Unlink` | `path`, `target` |
| `Remove` | `path`, `content`, `mode` |
| `Write` | `path`, `existed_before`, `previous_content?`, `previous_mode?` |
| `Move` | `source`, `path` |

### actions_gen.go — Do/Undo Wiring

7 compensable actions updated:
- `Do`: captures state from Forward's `map[string]any` return → UndoState
- `Undo`: casts UndoState → `map[string]any`, delegates to `Impl.Compensate*(state)`

2 non-compensable actions (`Source`, `Mkdir`) unchanged.

### provider_test.go — Round-Trip Tests

Per compensable method:
1. Forward creates/modifies — verify state captured
2. Backward restores — verify previous state restored

Additional tests:
- Forward on non-existent target: Backward removes created file
- Forward on existing target: Backward restores original content
- Nil state: Backward is no-op

## Files

| File | Action |
|---|---|
| `internal/execution/provider/file/provider.go` | Modify |
| `internal/execution/provider/file/actions_gen.go` | Modify |
| `internal/execution/provider/file/provider_test.go` | Create |
