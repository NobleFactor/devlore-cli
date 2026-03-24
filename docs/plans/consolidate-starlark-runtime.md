---
title: "Consolidate Starlark runtime: eliminate internal/starlark wrapper, create lore.Application"
issue: TBD
status: draft
created: 2026-03-24
updated: 2026-03-24
---

# Plan: Consolidate Starlark Runtime

## Summary

Eliminate the `internal/starlark.Runtime` wrapper by folding its shared methods
into `pkg/op.StarlarkRuntime` and moving application-specific code into
`cmd/lore/lore.Application` and `cmd/devlore-test/devloretest/`. This produces
a single `StarlarkRuntime` consumed directly by all three CLI tools (star, lore,
devlore-test), following the same Application pattern already used by star.

This is step 1 of the broader `pkg/op` starlark extraction. It is deliberately
scoped to move code without changing behavior.

## Goals

1. **Single StarlarkRuntime** -- `pkg/op.StarlarkRuntime` is the only runtime struct; no wrappers
2. **lore.Application pattern** -- `cmd/lore/lore/` follows the `cmd/star/star/` convention
3. **devlore-test.Application pattern** -- `cmd/devlore-test/devloretest/` absorbs test runner
4. **Delete `internal/starlark/`, `internal/lore/`, `internal/starlarktest/`, `internal/devloretest/`**

## Non-Goals

- Extracting starlark code from `pkg/op` (step 2)
- Renaming `pkg/op` (step 2)
- Changing any public API signatures beyond what the consolidation requires
- Changing test behavior -- all existing tests must pass with equivalent coverage

## Current State

| Component                          | Location                                                                           | Notes                                                                |
| ---------------------------------- | ---------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Base runtime (immediate receivers) | `pkg/op/starlark_runtime.go`                                                       | `StarlarkRuntime`                                                    |
| Runtime wrapper (plan + loader)    | `internal/starlark/runtime.go`                                                     | Embeds `*op.StarlarkRuntime`, adds `BuildGlobals`, `ConfigureThread` |
| Plan namespace                     | `internal/starlark/plan_root.go`                                                   | `PlanRoot`, `collectPlannedProviders`                                |
| Package/phase contexts             | `internal/starlark/interfaces.go`, `package.go`, `phase_context.go`, `receiver.go` | Lore-specific starlark receivers                                     |
| Lore CLI commands                  | `internal/lore/`                                                                   | `commands.go`, `root.go`, `builder.go`, `onboard/`                   |
| Test runner                        | `internal/starlarktest/`                                                           | `runner.go`, `test_context.go`, `trace.go`, `data/*.star`            |
| Test CLI commands                  | `internal/devloretest/`                                                            | `commands.go`, `root.go`                                             |

### Dependency chain today

```
cmd/lore/main.go
  -> internal/lore (commands, builder, root)
     -> internal/starlark (Runtime wrapper)
        -> pkg/op (StarlarkRuntime)

cmd/devlore-test/main.go
  -> internal/devloretest (commands, root)
     -> internal/starlarktest (runner)
        -> internal/starlark (Runtime wrapper)
           -> pkg/op (StarlarkRuntime)

cmd/star/main.go
  -> cmd/star/star (Application)
     -> pkg/op (StarlarkRuntime directly)
```

### Dependency chain after

```
cmd/lore/main.go
  -> cmd/lore/lore (Application)
     -> pkg/op (StarlarkRuntime)

cmd/devlore-test/main.go
  -> cmd/devlore-test/devloretest (Application)
     -> pkg/op (StarlarkRuntime)

cmd/star/main.go
  -> cmd/star/star (Application)
     -> pkg/op (StarlarkRuntime)
```

## Implementation Phases

### Phase 1: Fold shared runtime methods into `pkg/op.StarlarkRuntime`

Move the three methods that both lore and devlore-test need from
`internal/starlark.Runtime` into `pkg/op.StarlarkRuntime`:

- `BuildGlobals(graph, project, reg)` -- builds immediate receivers + plan namespace
- `ConfigureThread(thread, graph, project, reg)` -- sets `thread.Load` to `@devlore//` loader
- `RegisterActions(reg, ctx)` -- thin wrapper over `Initialize` (kept for API clarity)

Move supporting code into `pkg/op`:

- `plan_root.go` -- `PlanRoot`, `NewPlanRootFromProviders` (plan namespace starlark.Value)
- `collectPlannedProviders()` -- collects `PlanningReceiverFactory` implementations
- Module loader logic (`makeLoader`, `resolveProvider`, `buildPlanModule`, `loaderEntry` cache)

**Files**:

| File                         | Action | Source                                                                    |
| ---------------------------- | ------ | ------------------------------------------------------------------------- |
| `pkg/op/plan_root.go`        | Create | from `internal/starlark/plan_root.go`                                     |
| `pkg/op/starlark_runtime.go` | Modify | absorb `BuildGlobals`, `ConfigureThread`, `RegisterActions`, loader logic |

- [ ] Move `plan_root.go` to `pkg/op/`
- [ ] Move loader logic into `pkg/op/starlark_runtime.go`
- [ ] Add `BuildGlobals`, `ConfigureThread`, `RegisterActions` methods to `StarlarkRuntime`
- [ ] Verify: `make vet` passes

### Phase 2: Create `cmd/lore/lore/` (lore.Application)

Create `cmd/lore/lore/` following the `cmd/star/star/` pattern. The Application
struct holds `*op.StarlarkRuntime` directly.

Absorb `internal/lore/` (builder, commands, root, onboard) and the lore-specific
parts of `internal/starlark/` (PackageContext, PhaseContext, receivers).

**File mapping**:

| Source                                                   | Destination                                              |
| -------------------------------------------------------- | -------------------------------------------------------- |
| `internal/starlark/interfaces.go`                        | `cmd/lore/lore/interfaces.go`                            |
| `internal/starlark/interfaces_test.go`                   | `cmd/lore/lore/interfaces_test.go`                       |
| `internal/starlark/package.go` (PackageContext receiver) | `cmd/lore/lore/package.go`                               |
| `internal/starlark/phase_context.go`                     | `cmd/lore/lore/phase_context.go`                         |
| `internal/starlark/receiver.go` (packageContextReceiver) | `cmd/lore/lore/receiver.go`                              |
| `internal/starlark/receiver_test.go`                     | `cmd/lore/lore/receiver_test.go`                         |
| `internal/lore/builder.go`                               | `cmd/lore/lore/application.go` (merged into Application) |
| `internal/lore/builder_test.go`                          | `cmd/lore/lore/application_test.go`                      |
| `internal/lore/commands.go`                              | `cmd/lore/lore/commands.go`                              |
| `internal/lore/commands_test.go`                         | `cmd/lore/lore/commands_test.go`                         |
| `internal/lore/root.go`                                  | `cmd/lore/lore/root.go`                                  |
| `internal/lore/onboard/onboard.go`                       | `cmd/lore/lore/onboard/onboard.go`                       |

- [ ] Create `cmd/lore/lore/` package
- [ ] Create `Application` struct holding `*op.StarlarkRuntime`
- [ ] Move lore-specific starlark types (PackageContext, PhaseContext, receivers)
- [ ] Move builder logic as Application methods
- [ ] Move commands, root, onboard
- [ ] Update `cmd/lore/main.go` to import `cmd/lore/lore`
- [ ] Verify: `make test` passes for lore

### Phase 3: Create `cmd/devlore-test/devloretest/` (test Application)

Absorb `internal/starlarktest/` and `internal/devloretest/` into
`cmd/devlore-test/devloretest/`. The runner uses `op.StarlarkRuntime` directly
instead of `internal/starlark.Runtime`.

**File mapping**:

| Source                                  | Destination                                     |
| --------------------------------------- | ----------------------------------------------- |
| `internal/starlarktest/runner.go`       | `cmd/devlore-test/devloretest/runner.go`        |
| `internal/starlarktest/runner_test.go`  | `cmd/devlore-test/devloretest/runner_test.go`   |
| `internal/starlarktest/test_context.go` | `cmd/devlore-test/devloretest/test_context.go`  |
| `internal/starlarktest/trace.go`        | `cmd/devlore-test/devloretest/trace.go`         |
| `internal/starlarktest/data/*.star`     | `cmd/devlore-test/devloretest/data/*.star`      |
| `internal/devloretest/commands.go`      | `cmd/devlore-test/devloretest/commands.go`      |
| `internal/devloretest/commands_test.go` | `cmd/devlore-test/devloretest/commands_test.go` |
| `internal/devloretest/root.go`          | `cmd/devlore-test/devloretest/root.go`          |

- [ ] Move starlarktest files to `cmd/devlore-test/devloretest/`
- [ ] Move devloretest files to `cmd/devlore-test/devloretest/`
- [ ] Update runner to use `op.StarlarkRuntime` directly (drop `loreStar.NewRuntime`)
- [ ] Update `cmd/devlore-test/main.go` import
- [ ] Update `cmd/devlore-test/cli_test.go` testdata path
- [ ] Verify: `make test` passes for devlore-test

### Phase 4: Delete internal packages and verify

- [ ] Delete `internal/starlark/`
- [ ] Delete `internal/lore/`
- [ ] Delete `internal/starlarktest/`
- [ ] Delete `internal/devloretest/`
- [ ] Grep for stale import paths -- zero matches
- [ ] `make check` passes from repo root

## Risks

| Risk                                                                                   | Mitigation                                                                                                                              |
| -------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| Adding starlark surface to `pkg/op` (plan_root, loader) goes opposite to step 2's goal | Accepted -- step 2 extracts it along with the rest of the starlark code. If step 2 stalls, revisit placement.                           |
| `internal/starlark` integration tests import `cmd/star/provider/starcode/gen`          | These tests move to `cmd/lore/lore/` or `cmd/devlore-test/devloretest/` -- both under `cmd/` so cross-cmd imports are fine within tests |
| `internal/lore/onboard/` has its own subpackage                                        | Moves to `cmd/lore/lore/onboard/` -- straightforward                                                                                    |
| Test script paths in `cli_test.go` reference `internal/starlarktest/data/`             | Update path to `devloretest/data/` after move                                                                                           |

## Open Questions

None -- all design decisions resolved during discussion.
