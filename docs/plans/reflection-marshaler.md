---
title: "Reflection-based Starlark Marshaler"
status: in-progress
created: 2026-02-27
updated: 2026-02-27
---

# Plan: Reflection-based Starlark Marshaler

## Summary

Replace ~7,500 lines of generated/hand-written boilerplate across devlore-cli and noblefactor-ops with a reflection-based Starlark marshaler in `pkg/op/`. Codegen stays (`star devlore actions generate` wraps `go generate`) but generated files shrink dramatically because they call into the marshaler instead of reimplementing type conversion, method dispatch, and struct marshaling.

## Goals

1. **Marshaler infrastructure**: `Marshal`/`Unmarshal` for Go/Starlark type conversion, `WrapReceiver` for immediate-mode method bridging, `WrapPlanned` for planned-mode node creation
2. **All providers fully generated**: Every provider has star-generated `gen/` files (no hand-written exceptions)
3. **Net reduction ~5,800 lines**: ~1,700 lines of marshaler infrastructure replaces ~7,500 lines of boilerplate

## Hard Exit Criterion

**All 19 providers must have a complete set of star-generated `gen/` files that rely on the marshaler. No hand-written exceptions. `make generate` rebuilds all of them.**

### access=both providers (9) — gen/params.gen.go, gen/immediate.gen.go, gen/planned.gen.go, gen/actions.gen.go

| Provider | Status |
| --- | --- |
| `archive` | Pending |
| `encryption` | Pending |
| `file` | Update existing |
| `git` | Pending |
| `net` | Pending |
| `pkg` | Pending |
| `service` | Pending |
| `shell` | Pending |
| `template` | Pending |

### access=immediate providers (10) — gen/params.gen.go, gen/immediate.gen.go

| Provider | Status |
| --- | --- |
| `json` (new) | Pending |
| `regexp` (new) | Pending |
| `staranalysis` | Update existing |
| `starcode` | Update existing |
| `starcomplexity` | Update existing |
| `starindex` | Update existing |
| `starsources` | Update existing |
| `starstats` | Update existing |
| `ui` | Pending |
| `yaml` (new) | Pending |

### Not providers (supporting packages, no gen files)

- `platform` — platform-specific package/service managers

### Makefile integration

Every provider gets a grouped make target. `make build` depends on `make generate`. When `provider.go` changes, `make` invokes `star devlore actions generate` and regenerates all gen files for that provider.

## Current State

| Component | Status | Notes |
| --- | --- | --- |
| `pkg/op/convert.go` | Working | 134 lines, primitive type switches only |
| File provider gen/ | Working | 1,266 lines (actions + immediate + planned) |
| Star* providers gen/ | Working | convert.gen.go + immediate.gen.go per provider |
| 9 legacy providers | Broken | Blank imports in register.go but no init() functions |
| JSON/YAML/Regexp | Missing | No providers exist yet |

## Implementation Phases

### Phase 0: Delete old `*_gen.go` files from package roots

Already done (git status shows deletions staged).

### Phase 1: Core struct marshaler

- [ ] Implement `pkg/op/marshal.go` — type cache, `Marshal`, `Unmarshal`, `camelToSnake`
- [ ] Implement `pkg/op/marshal_test.go` — round-trip, nested structs, slices, maps, nil pointers
- [ ] Delete `pkg/op/convert.go` — replaced by `Marshal`/`Unmarshal`
- [ ] Update call sites in `output.go` and gen files

**Files**: `pkg/op/marshal.go` (Create ~200), `pkg/op/marshal_test.go` (Create ~200)

### Phase 2: Method bridge

- [ ] Implement `pkg/op/receiver_reflect.go` — `WrapReceiver`, `ReflectedReceiver`, `Override`, `MethodParams`
- [ ] Implement `pkg/op/receiver_reflect_test.go` — method calls, arg unpack, return marshal, error propagation
- [ ] Method discovery: exported methods exposed as snake_case Starlark builtins
- [ ] Return type classification: auto-detect `()`, `(error)`, `(T, error)`, `(T)`, `(T, map[string]any, error)`

**Files**: `pkg/op/receiver_reflect.go` (Create ~250), `pkg/op/receiver_reflect_test.go` (Create ~250)

### Phase 3: Update star codegen (noblefactor-ops)

- [ ] `gen/params.gen.go` — new output: parameter name table from AST
- [ ] `gen/immediate.gen.go` — simplified: init() + WrapReceiver + Override() calls
- [ ] `gen/planned.gen.go` — simplified: init() + WrapPlanned
- [ ] `gen/convert.gen.go` — no longer generated (Marshal handles struct conversion)
- [ ] `gen/actions.gen.go` — unchanged (explicit Do/Undo logic)

### Phase 4: Planned mode wrapper

- [ ] Implement `pkg/op/planned_reflect.go` — `WrapPlanned`, `ReflectedPlanned`
- [ ] Implement `pkg/op/planned_reflect_test.go` — node creation, slot filling, method filtering
- [ ] Only methods with registered actions are exposed

**Files**: `pkg/op/planned_reflect.go` (Create ~150), `pkg/op/planned_reflect_test.go` (Create ~150)

### Phase 5: JSON provider

- [ ] `pkg/op/provider/json/provider.go` — Encode, EncodeIndent, Decode
- [ ] `pkg/op/provider/json/provider_test.go`
- [ ] Star-generated gen/ files via Makefile target

### Phase 6: YAML provider

- [ ] `pkg/op/provider/yaml/provider.go` — Encode, Decode
- [ ] `pkg/op/provider/yaml/provider_test.go`
- [ ] Star-generated gen/ files via Makefile target

### Phase 7: Regexp provider

- [ ] `pkg/op/provider/regexp/provider.go` — Match, Find, FindAll, FindSubmatch, FindAllSubmatch, Replace, ReplaceLiteral, Split
- [ ] `pkg/op/provider/regexp/provider_test.go`
- [ ] Star-generated gen/ files via Makefile target

### Phase 8: BindingSet factory map dedup

- [ ] Extract `collectPlannedFactories()` from duplicate blocks in `internal/starlark/binding_set.go`

### Phase 9: Action registry convenience + executor defaults

- [ ] `NewActionRegistry()` replacing duplicate call sites
- [ ] `DefaultExecutorOptions()` replacing duplicate sites

### Phase 10: Graph creation unification

- [ ] `NewGraph(tool string)` factory in `pkg/op/graph.go`
- [ ] Update `internal/writ/graph_builder.go` and `internal/lore/builder.go`

### Phase 11: GraphView base type

- [ ] `internal/execution/graphview.go` — GraphView with NodeByID, FilterNodes, SortedNodeIDs
- [ ] Embed in stateview.go and dependencyview.go

### Full provider migration (exit gate)

For ALL 19 providers:
1. Run `star devlore actions generate` (updated) to produce gen/ files
2. Verify gen/immediate.gen.go calls WrapReceiver
3. Verify gen/planned.gen.go calls WrapPlanned (access=both only)
4. Verify gen/actions.gen.go has correct Do/Undo (access=both only)
5. Verify gen/params.gen.go has correct parameter table
6. Delete gen/convert.gen.go (replaced by Marshal)
7. Verify Makefile has grouped target for the provider
8. `make check` passes

## Related Documents

- [Binding Unification](./binding-unification.md)
- [Star Gen Receiver](./star-gen-receiver.md)
- [Code Consolidation Analysis](../../CODE-CONSOLIDATION-ANALYSIS.md)
