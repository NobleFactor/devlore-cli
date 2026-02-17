---
title: "Compensation Phase 5: Generator Template Update"
parent: compensation.md
status: in-progress
created: 2026-02-17
updated: 2026-02-17
---

# Phase 5: Generator Template Update

## Summary

Update the `graph_actions` code generator to detect `Compensate<Method>` pairs
and emit correct Do/Undo wiring automatically. After this phase, providers that
follow the standard pattern produce correct actions_gen.go via the generator.

## Prerequisites

Service provider Compensate methods must be standardized to
`func(state map[string]any) error` (drop `io.Writer` param) so the generator
can emit uniform Undo delegation. Compensation runs silently via `io.Discard`.

## Changes

### noblefactor-ops: `receiver_go_gen.go`

| Change | Detail |
|---|---|
| `methodInfo.Compensable` | New `bool` field — has a Compensate pair |
| `methodInfoFromValue` | Read `compensable` from descriptor dict |
| `validateCompensableReturn` | New function: accepts `(map[string]any, error)` or `(T, map[string]any, error)` |
| Gate 2 update | Use compensable-aware validation for compensable methods |
| `tplGraphReturn` | Capture state from Forward's state return when compensable |
| `tplGraphUndo` | Emit delegation to `Impl.Compensate<GoName>(state)` when compensable; nil stub otherwise |
| Slot reader safety | Use `_, _ :=` form for safe type assertions in graphReaders |

### devlore-cli: `generate.star`

| Change | Detail |
|---|---|
| Filter Compensate* | Skip methods starting with "Compensate" from action list |
| Detect pairs | Check if `Compensate<Name>` exists for each Forward method |
| Set compensable | Add `compensable: True/False` to method descriptors |
| Gate 3 | Validate Compensate method returns `error` and takes single `map[string]any` param |

### devlore-cli: Service Provider Signature Standardization

| File | Change |
|---|---|
| `provider/service/provider.go` | Compensate methods: `(map[string]any, io.Writer) error` → `(map[string]any) error` |
| `provider/service/actions_gen.go` | Undo: `o.Impl.CompensateX(s, ctx.Logger)` → `o.Impl.CompensateX(s)` |
| `provider/service/provider_test.go` | Update mock calls to match new signatures |

### devlore-cli: `graph_actions.go.template`

No changes needed — template already uses `{{graphReturn .}}` and `{{graphUndo .}}`.

## Compensable Return Signatures

The generator handles two compensable return patterns:

| Return Signature | ValueType | State | Example |
|---|---|---|---|
| `(map[string]any, error)` | none | captured | `Install`, `Start`, `Clone` |
| `(T, map[string]any, error)` | T | captured | `Copy` (returns checksum) |

Non-compensable methods keep current validation: `error` or `(T, error)`.

## Files

### noblefactor-ops

| File | Action |
|---|---|
| `internal/starlark/receiver_go_gen.go` | Modify |
| `internal/starlark/receiver_go_gen_test.go` | Create |

### devlore-cli

| File | Action |
|---|---|
| `star/extensions/.../commands/generate.star` | Modify |
| `internal/execution/provider/service/provider.go` | Modify |
| `internal/execution/provider/service/actions_gen.go` | Modify |
| `internal/execution/provider/service/provider_test.go` | Modify |
