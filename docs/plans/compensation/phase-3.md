---
title: "Compensation Phase 3: Package Provider"
parent: compensation.md
status: in-progress
created: 2026-02-16
updated: 2026-02-16
---

# Phase 3: Package Provider

## Summary

Add real compensation to the package provider. 3 compensable actions
(Install, Upgrade, Remove) out of 4 total. Update is non-compensable
(idempotent index refresh).

## Changes

### provider.go — Forward Method Signature Changes

| Method | Current Return | New Return |
|---|---|---|
| `Install` | `error` | `(map[string]any, error)` |
| `Upgrade` | `error` | `(map[string]any, error)` |
| `Remove` | `error` | `(map[string]any, error)` |

`Update` unchanged (non-compensable).

### provider.go — Backward Methods Added

| Method | Compensation Logic |
|---|---|
| `CompensateInstall` | Remove packages that weren't already installed |
| `CompensateUpgrade` | No-op — captures previous versions for diagnostics but automatic downgrade is not reliable across package managers |
| `CompensateRemove` | Reinstall the removed packages |

### provider.go — State Captured Per Method

| Forward | State Keys |
|---|---|
| `Install` | `packages`, `manager`, `cask`, `already_installed` |
| `Upgrade` | `packages`, `manager`, `cask`, `previous_versions` |
| `Remove` | `packages`, `manager`, `cask` |

### Pre-Action State Queries

Uses `host.PackageManager.Installed()` and `Version()` to query state
before acting. Test hooks on Provider for mock-based testing.

### actions_gen.go — Do/Undo Wiring

3 compensable actions updated. Update unchanged.

## Files

| File | Action |
|---|---|
| `internal/execution/provider/pkg/provider.go` | Modify |
| `internal/execution/provider/pkg/actions_gen.go` | Modify |
| `internal/execution/provider/pkg/provider_test.go` | Create |
