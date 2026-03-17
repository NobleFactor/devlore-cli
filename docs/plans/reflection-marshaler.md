---
title: "Reflection-based Starlark Marshaler"
status: complete
created: 2026-02-27
updated: 2026-03-16
---

# Plan: Reflection-based Starlark Marshaler

## Completion Summary (2026-03-16)

**All three goals met.** The reflection-based marshaler is in production and all providers
have star-generated `gen/` files orchestrated by `make generate`.

### What was done

- **Marshaler infrastructure** shipped as `pkg/op/starvalue_marshal.go` (1,102 lines) —
  not `marshal.go` as originally planned, but functionally complete: `Marshal`, constructor
  registry, type cache, callable resource support.
- **Method bridge** shipped as `pkg/op/receiver_reflect.go` (521 lines) — `WrapReceiver`,
  `ReflectedReceiver`, method discovery, return type classification.
- **Planned mode wrapper** shipped as `pkg/op/planned_reflect.go` (340 lines) —
  `WrapPlanned`, `ReflectedPlanned`, action-gated method exposure.
- **`convert.go` deleted** — replaced by the marshaler.
- **JSON, YAML, Regexp providers** created with tests and star-generated `gen/` files.
- **`NewActionRegistry()`** factory exists in `pkg/op/registry.go`.
- **`NewGraph(tool string)`** factory exists in `pkg/op/graph.go`.
- **All 20 providers** (plan said 19) have complete `gen/` directories. `platform` gained
  gen files despite the plan listing it as "not a provider."
- **Makefile integration** complete — `make build` and `make test` depend on `make generate`;
  every provider has a grouped target.

### What was not done (and why)

- **Phase 8 (`collectPlannedFactories` extraction):** `internal/starlark/binding_set.go`
  never existed. The duplication the plan targeted was either resolved differently or was
  misidentified. No action needed.
- **Phase 9 (`DefaultExecutorOptions`):** `NewActionRegistry()` shipped but
  `DefaultExecutorOptions()` was never created. Executor construction sites don't share
  enough structure to justify a factory — each caller configures different options.
- **Phase 11 (`graphview.go`):** The planned `GraphView` base type was not created.
  `dependencyview.go` and `stateview.go` already provide `NodeByID`, edge indexing, and
  state management. Extracting a shared base would add abstraction without removing code.

---

## Original Plan

### Goals

1. **Marshaler infrastructure**: `Marshal`/`Unmarshal` for Go/Starlark type conversion, `WrapReceiver` for immediate-mode method bridging, `WrapPlanned` for planned-mode node creation
2. **All providers fully generated**: Every provider has star-generated `gen/` files (no hand-written exceptions)
3. **Net reduction ~5,800 lines**: ~1,700 lines of marshaler infrastructure replaces ~7,500 lines of boilerplate

### Hard Exit Criterion

**All providers must have a complete set of star-generated `gen/` files that rely on the marshaler. No hand-written exceptions. `make generate` rebuilds all of them.** — MET

### Implementation Phases

#### Phase 0: Delete old `*_gen.go` files from package roots — `complete`

#### Phase 1: Core struct marshaler — `complete`

Shipped as `pkg/op/starvalue_marshal.go` (1,102 lines). `convert.go` deleted.

#### Phase 2: Method bridge — `complete`

Shipped as `pkg/op/receiver_reflect.go` (521 lines).

#### Phase 3: Update star codegen (noblefactor-ops) — `complete`

All gen templates updated: `params.gen.go`, `immediate.gen.go`, `planned.gen.go`,
`actions.gen.go`.

#### Phase 4: Planned mode wrapper — `complete`

Shipped as `pkg/op/planned_reflect.go` (340 lines).

#### Phase 5: JSON provider — `complete`

`pkg/op/provider/json/` with Encode, EncodeIndent, Decode + gen/ files.

#### Phase 6: YAML provider — `complete`

`pkg/op/provider/yaml/` with Encode, Decode + gen/ files.

#### Phase 7: Regexp provider — `complete`

`pkg/op/provider/regexp/` with Match, Find, FindAll, FindSubmatch, Replace, Split + gen/ files.

#### Phase 8: BindingSet factory map dedup — `dropped`

Target file (`internal/starlark/binding_set.go`) never existed. Duplication was either
resolved differently or misidentified during planning.

#### Phase 9: Action registry convenience + executor defaults — `partial`

`NewActionRegistry()` shipped. `DefaultExecutorOptions()` dropped — executor call sites
don't share enough structure to justify a factory.

#### Phase 10: Graph creation unification — `complete`

`NewGraph(tool string)` factory exists in `pkg/op/graph.go`.

#### Phase 11: GraphView base type — `dropped`

`dependencyview.go` and `stateview.go` already cover this functionality. Extracting a
shared base would add abstraction without removing code.

### Full provider migration (exit gate) — `complete`

All 20 providers have star-generated `gen/` directories. `make generate` rebuilds all of
them. `make check` passes.

## Related Documents

- [Binding Unification](./binding-unification.md)
- [Star Gen Receiver](./star-gen-receiver.md)
- [Code Consolidation Analysis](../../CODE-CONSOLIDATION-ANALYSIS.md)
