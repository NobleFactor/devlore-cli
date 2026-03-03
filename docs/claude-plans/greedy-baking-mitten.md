# Sort Functions in `pkg/op/provider/file/provider.go`

## Context

The provider methods are in insertion order. The user wants them sorted with
compensable pairs kept together (action + `Compensate*`).

## Approach

Alphabetical sort within two groups, pairs kept adjacent:

### Group 1 — Compensable Pairs (action + compensation)

| # | Action | Compensate |
|---|--------|------------|
| 1 | Backup | CompensateBackup |
| 2 | Copy   | CompensateCopy |
| 3 | Link   | CompensateLink |
| 4 | Move   | CompensateMove |
| 5 | Remove | CompensateRemove |
| 6 | Unlink | CompensateUnlink |
| 7 | Write  | CompensateWrite |

### Group 2 — Standalone Methods (no compensation)

Exists, Glob, IsDir, IsFile, Join, Mkdir, Name, Parent, Read, RemoveAll, WalkTree

## File

`pkg/op/provider/file/provider.go` — reorder method bodies only (lines 29–592).
The struct definition and imports stay put.

## Verification

- `make check` passes
- `make test` passes
- No functional changes — purely a reorder
