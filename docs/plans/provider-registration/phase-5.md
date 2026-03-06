---
title: "Phase 5: Cleanup"
status: draft
created: 2026-03-06
updated: 2026-03-06
parent: ../provider-registration.md
---

# Phase 5: Cleanup

## Summary

Delete the old registration infrastructure. After Phase 4, nothing calls
it. This phase removes dead code and verifies the codebase is clean.

## Deletions

### `pkg/op/binding_registry.go` — entire file

Contains:
- `PlannedFactory` type
- `ImmediateFactory` type
- `ProviderBinding` struct
- `RegisterBinding()` function
- `AllBindings()` function
- `BindingByName()` function
- `bindingRegistry` map + mutex

All replaced by `Provider`/`PlannedProvider`/`ImmediateProvider` interfaces
and `Announce`/`InitAll`/`Providers` functions.

### `pkg/op/provider_registry.go` — entire file

Contains:
- `ProviderRegistrar` type
- `RegisterAllProviders()` function

Replaced by `op.InitAll()`.

### `pkg/op/provider/register.go` — simplify

The blank imports stay (they trigger `init()` → `op.Announce()`). The
`RegisterAll` wrapper is deleted — callers use `op.InitAll` directly.
If the blank imports are the only remaining content, consider whether this
file should remain as the canonical import point or be deleted.

### Dead references in tests and docs

Grep for any remaining references to `RegisterBinding`, `AllBindings`,
`BindingByName`, `ProviderBinding`, `ProviderRegistrar`,
`RegisterAllProviders`, `PlannedFactory`, `ImmediateFactory` in code,
comments, and documentation.

## Tasks

- [x] Delete `pkg/op/binding_registry.go`
- [x] Delete `pkg/op/provider_registry.go`
- [x] Delete `RegisterAll` from `pkg/op/provider/register.go` (keep blank imports)
- [x] Rewrite `binding_set_test.go` for new API *(done in Phase 3 bridge)*
- [x] Replace `PlannedFactory` type with `PlannedProvider` interface in `plan_root.go`, `binding_set.go`
- [x] Grep for dead references — zero matches in `.go` files
- [x] Update stale comments in `lifetime.go`, `binding_config.go`
- [x] `make test` passes
- [x] `make test-race` passes — no data races

## Files

| File | Action | Purpose |
| --- | --- | --- |
| `pkg/op/binding_registry.go` | Delete | Old registration infrastructure |
| `pkg/op/provider_registry.go` | Delete | Old `RegisterAllProviders` |
| `pkg/op/provider/register.go` | Modify | Remove `RegisterAll` wrapper |
| `internal/starlark/binding_set_test.go` | Modify | Update for new API |
| Various docs | Modify | Remove dead references |

## Exit Criteria

- Zero references to `RegisterBinding`, `AllBindings`, `BindingByName`, `ProviderBinding`, `ProviderRegistrar`, `RegisterAllProviders`, `PlannedFactory`, or `ImmediateFactory` in any `.go` file
- `make check` passes
- `make test-race` passes
- The only registration path is `op.Announce()` → `op.InitAll()`
