---
title: "Compensation Phase 4: Remaining Providers (Net, Archive, Git)"
parent: compensation.md
status: in-progress
created: 2026-02-16
updated: 2026-02-16
---

# Phase 4: Remaining Providers (Net, Archive, Git)

## Summary

Add real compensation to the 3 remaining providers. 1 compensable action each.
Small scope — straightforward state capture and cleanup.

## Changes

### Net Provider — Download

Current `Download(url)` returns `([]byte, error)` — raw bytes. The file write
happens in the action's `Do` (when `path` slot is set). Compensation removes
the downloaded file.

| Method | Change |
|---|---|
| `Download` | Unchanged — returns raw bytes |
| `CompensateDownload` | New — removes file at `state["path"]` |

State capture happens in the action `Do` (since the file write is there).
`Undo` delegates to `CompensateDownload`. In-memory downloads (no path) are
not compensable — UndoState is nil.

### Archive Provider — Extract

Current `Extract(source, prefix)` returns `error`. Changed to return
`(map[string]any, error)` with created file list for compensation.

| Method | Current Return | New Return |
|---|---|---|
| `Extract` | `error` | `(map[string]any, error)` |

| Method | Compensation Logic |
|---|---|
| `CompensateExtract` | Remove created files in reverse order, then empty dirs |

State: `{"dest": prefix, "created_files": [...]}`

The internal extract helpers (`extractTarGz`, `extractZip`) are modified to
collect and return created file paths.

### Git Provider — Clone

Current `Clone(url, path, output)` returns `error`. Changed to return
`(map[string]any, error)`.

| Method | Current Return | New Return |
|---|---|---|
| `Clone` | `error` | `(map[string]any, error)` |

| Method | Compensation Logic |
|---|---|
| `CompensateClone` | `os.RemoveAll(path)` |

State: `{"path": path}`

Non-compensable: `Checkout`, `Pull` (unchanged).

### actions_gen.go — Do/Undo Wiring

3 actions updated across 3 packages.

## Files

| File | Action |
|---|---|
| `internal/execution/provider/net/provider.go` | Modify |
| `internal/execution/provider/net/actions_gen.go` | Modify |
| `internal/execution/provider/net/provider_test.go` | Create |
| `internal/execution/provider/archive/provider.go` | Modify |
| `internal/execution/provider/archive/actions_gen.go` | Modify |
| `internal/execution/provider/archive/provider_test.go` | Create |
| `internal/execution/provider/git/provider.go` | Modify |
| `internal/execution/provider/git/actions_gen.go` | Modify |
| `internal/execution/provider/git/provider_test.go` | Create |
