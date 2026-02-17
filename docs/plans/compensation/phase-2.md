---
title: "Compensation Phase 2: Service Provider"
parent: compensation.md
status: pending
created: 2026-02-16
updated: 2026-02-16
---

# Phase 2: Service Provider

## Summary

Add real compensation to the service provider. 5 actions, all compensable.
Natural inverse pairs (start/stop, enable/disable). Requires querying
service state before acting.

## Changes

### provider.go — Forward Method Signature Changes

| Method | Current Return | New Return |
|---|---|---|
| `Start` | `error` | `(map[string]any, error)` |
| `Stop` | `error` | `(map[string]any, error)` |
| `Restart` | `error` | `(map[string]any, error)` |
| `Enable` | `error` | `(map[string]any, error)` |
| `Disable` | `error` | `(map[string]any, error)` |

### provider.go — Backward Methods Added

| Method | Compensation Logic |
|---|---|
| `CompensateStart` | Stop service if it wasn't running before |
| `CompensateStop` | Start service if it was running before |
| `CompensateRestart` | No-op (service was running before restart) |
| `CompensateEnable` | Disable service if it wasn't enabled before |
| `CompensateDisable` | Enable service if it was enabled before |

### provider.go — State Captured Per Method

| Forward | State Keys |
|---|---|
| `Start` | `name`, `was_running` |
| `Stop` | `name`, `was_running` |
| `Restart` | `name` |
| `Enable` | `name`, `was_enabled` |
| `Disable` | `name`, `was_enabled` |

### actions_gen.go — Do/Undo Wiring

5 actions updated:
- `Do`: captures state from Forward's `map[string]any` return → UndoState
- `Undo`: casts UndoState → `map[string]any`, delegates to `Impl.Compensate*(state)`

### provider_test.go — Round-Trip Tests

Per compensable method:
1. Forward modifies service state — verify state captured
2. Backward restores — verify previous state restored
3. Nil state: Backward is no-op

## Files

| File | Action |
|---|---|
| `internal/execution/provider/service/provider.go` | Modify |
| `internal/execution/provider/service/actions_gen.go` | Modify |
| `internal/execution/provider/service/provider_test.go` | Create |
