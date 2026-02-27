# Code Consolidation Analysis

**Date**: 2026-02-27
**Scope**: devlore-cli + noblefactor-ops (binding-unification branches)
**Baseline**: 55,832 lines production code, 40,071 lines test code

---

## Codebase Size Breakdown

| Repo | Language | Production | Test | Total |
|------|----------|----------:|-----:|------:|
| devlore-cli | Go | 36,563 | 25,808 | 62,371 |
| devlore-cli | Starlark | 4,193 | — | 4,193 |
| noblefactor-ops | Go | 15,076 | 14,263 | 29,339 |
| **Total** | | **55,832** | **40,071** | **95,903** |

---

## Part 1: noblefactor-ops

### 1.1 Receiver Boilerplate (14 files, 7,857 lines)

Every receiver (`receiver_json.go`, `receiver_yaml.go`, `receiver_schema.go`, etc.) repeats:

- **Struct + constructor**: `type XyzReceiver struct { op.Receiver }` / `func NewXyzReceiver()` — 14 × ~8 lines = **112 lines**
- **Attr() switch**: dispatch to `op.MakeAttr()` per method — 14 × ~15-30 lines = **280+ lines**
- **AttrNames() list**: hardcoded sorted slice — 14 × ~3 lines = **42 lines**

**Consolidation**: Create `receiver_factory.go` with a builder that takes a method table and generates Attr/AttrNames dispatch. Each receiver file reduces to method implementations only.

**Estimated savings: ~434 lines (6% of receiver code)**

### 1.2 codegen.go Monolith (2,528 lines)

The file contains 5 distinct logical units mixed together:

| Unit | Lines | Proposed File |
|------|------:|---------------|
| Type definitions | ~150 | `codegen_types.go` |
| Name normalization | ~40 | `codegen_names.go` |
| Validation/gates | ~140 | `codegen_validate.go` |
| 31 templateFunc* functions | ~1,100 | `codegen_template_*.go` (5 files) |
| Core engine | ~500 | `codegen.go` (remains) |

**Parameter-filtering pattern** appears 8+ times:

```go
for _, p := range m.Params {
    if isContentParam(p, m) { continue }
    tm := typeMappings[p.GoType]
    if !tm.starlarkFacing { continue }
    // ... process
}
```

Appears in: `templateFuncPlanUnpackArgs`, `templateFuncPlanFillSlots`, `templateFuncImmediateUnpackArgs`, `templateFuncDryRunFmt`, `templateFuncDryRunVars`, `templateFuncGraphReaders`.

**Consolidation**: Extract `filterStarlarkParams()` helper + split into focused files.

**Estimated savings: ~400 lines of complexity reduction, ~150 lines of actual duplication**

### 1.3 starlarkToGo / goToStarlark (defined once, needed everywhere)

Recursive conversion functions live in `receiver_yaml.go` (86 lines) but are used by 4+ receiver files. Move to shared `starlark_utils.go`.

**Estimated savings: import coupling reduction, ~86 lines relocated**

### 1.4 Starlark Hook Commands (near-identical pair)

`hook-pre-commit.star` (115 lines) and `hook-pre-push.star` (115 lines) are identical except for the command name string.

**Consolidation**: Extract shared `hook_checks.star` module.

**Estimated savings: ~90 lines**

### 1.5 Lint Helper Duplication

`check_tool()` and `ensure_tool_installed()` are copied identically across `lint-go.star`, `lint-shell.star`, and inline in both hook files.

**Consolidation**: Shared `lint_utils.star` module.

**Estimated savings: ~50 lines**

### 1.6 Test Helper Expansion (codegen_test.go, 3,411 lines)

- 88 test functions, many inline descriptor creation instead of using existing fixtures
- `strings.Contains()` assertion pattern appears 200+ times
- Only 2 method fixtures exist for 88 tests

**Consolidation**: Expand fixture library (`paramFixture`, `callableFixture`, `methodFixture`), add `assertContains`/`assertNotContains` helpers.

**Estimated savings: ~200 lines**

### noblefactor-ops Subtotal

| Area | Savings |
|------|--------:|
| Receiver boilerplate | 434 |
| codegen.go split + dedup | 400 |
| Hook command dedup | 90 |
| Lint helper dedup | 50 |
| Test helpers | 200 |
| **Subtotal** | **~1,174** |

---

## Part 2: devlore-cli

### 2.1 Generated Action Wrapper Boilerplate (2,412 lines across all providers)

Every provider's `actions_gen.go` generates per-method wrapper structs:

```go
type Download struct{ Impl *Provider }
func (o *Download) Name() string { return "net.download" }
func (o *Download) Do(ctx *op.Context, slots map[string]any) (op.Result, op.UndoState, error) { ... }
```

Average: 21 lines per action × ~60 actions across all providers.

**Consolidation**: Replace per-action struct generation with a generic action wrapper using a descriptor table. The codegen template would emit a `[]ActionDescriptor` table instead of individual structs, and a single `genericAction` type dispatches via the table.

**Estimated savings: 420-500 lines (35-40% of actions_gen.go)**

### 2.2 Generated Immediate Receiver Switch Dispatch

Each `immediate_gen.go` generates an `Attr()` switch with 2-3 lines per method:

```go
case "download":
    return op.MakeAttr("net.download", r.download), nil
```

7 providers × ~100 lines each.

**Consolidation**: Dynamic attribute dispatch from a method table instead of generated switch. The codegen emits a `map[string]func` and a generic Attr implementation.

**Estimated savings: 210-280 lines**

### 2.3 Generated Planned Receiver Node Creation

Each planned method wrapper repeats node creation, slot filling, graph append:

```go
node := &op.Node{ID: ..., Action: ..., Project: ...}
if err := op.FillSlot(node, p.graph, "paramName", paramValue); err != nil { ... }
p.graph.Nodes = append(p.graph.Nodes, node)
return op.NewOutput(node, p.graph, ""), nil
```

8 providers × ~100 lines each.

**Consolidation**: Extract `addNode(graph, action, project, slots)` helper. Planned methods call the helper instead of inlining.

**Estimated savings: 180-250 lines**

### 2.4 Lore/Writ Consumer Duplication

Both `internal/lore/builder.go` (447 lines) and `internal/writ/graph_builder.go` (387 lines) independently:

- Create graphs with identical initialization boilerplate
- Resolve platform via `platform.New()` with fallback logic
- Build action registries with the same pattern
- Configure executors with `ExecutorOptions{}`

**Consolidation**: Extract shared `GraphFactory` in `pkg/op` and shared `ResolveContext` helper.

**Estimated savings: 90-150 lines**

### 2.5 BindingSet Factory Map Duplication

`BuildGlobals()` and `buildPlanModule()` both iterate `op.AllBindings()` to build the same `map[string]op.PlannedFactory`:

```go
factories := make(map[string]op.PlannedFactory)
for _, b := range op.AllBindings() {
    if b.PlannedFactory != nil {
        factories[b.Name] = b.PlannedFactory
    }
}
```

Appears twice, identically.

**Consolidation**: Extract `collectPlannedFactories()`.

**Estimated savings: 15-20 lines**

### 2.6 StateView / DependencyView (961 lines combined)

`stateview.go` (517 lines) and `dependencyview.go` (444 lines) follow parallel patterns: load YAML/JSON, query/filter entries, transform results.

**Consolidation**: Extract unified `GraphView` base with common query/filter methods.

**Estimated savings: 60-100 lines**

### 2.7 Star Extension Commands (Knowledge, Package share patterns)

`extract.star`, `index.star`, `sign.star`, `validate.star` across Knowledge and Package extensions share:
- Identical argument parsing patterns
- Similar validation flows
- Common output formatting

**Consolidation**: Shared Starlark utility module for extension commands.

**Estimated savings: 80-120 lines**

### 2.8 Provider Root Field Pattern (6 star* providers)

`staranalysis`, `starcode`, `starcomplexity`, `starindex`, `starsources`, `starstats` all have `struct { Root string }` and similar initialization. File provider shares this pattern.

**Consolidation**: Common `RootProvider` mixin or embedded struct with shared path resolution.

**Estimated savings: 40-80 lines**

### devlore-cli Subtotal

| Area | Savings |
|------|--------:|
| Generated action wrappers | 460 |
| Generated immediate dispatch | 245 |
| Generated planned node creation | 215 |
| Lore/Writ consumer consolidation | 120 |
| StateView/DependencyView base | 80 |
| Star extension commands | 100 |
| Root provider pattern | 60 |
| BindingSet factory map | 18 |
| **Subtotal** | **~1,298** |

---

## Combined Summary

| Category | noblefactor-ops | devlore-cli | Total |
|----------|---------------:|------------:|------:|
| Generated code consolidation | — | 920 | 920 |
| Receiver/dispatch boilerplate | 434 | — | 434 |
| Monolith decomposition | 400 | — | 400 |
| Consumer duplication | — | 200 | 200 |
| Test helper expansion | 200 | — | 200 |
| Starlark command dedup | 140 | 100 | 240 |
| View/state consolidation | — | 80 | 80 |
| Provider struct patterns | — | 60 | 60 |
| Misc | — | 18 | 18 |
| **Total** | **1,174** | **1,298** | **~2,472** |

**Production code reduction**: ~2,472 lines from 55,832 = **4.4%**

This is conservative — it counts only clear duplication, not the cascading simplification that emerges when shared abstractions replace repeated patterns. The real savings compound: once generated code uses descriptor tables instead of per-action structs, the templates shrink, the codegen logic simplifies, and the test surface reduces proportionally.

A realistic total including second-order effects: **~3,500-4,000 lines**, or **6-7% of production code**.

---

## Priority Ranking

| Priority | Target | Effort | Savings | ROI |
|----------|--------|--------|--------:|-----|
| 1 | Generated action wrapper consolidation | Medium | 460 | High — one template change, all providers benefit |
| 2 | codegen.go decomposition | Medium | 400 | High — maintainability + reveals further dedup |
| 3 | Receiver factory (noblefactor-ops) | Low | 434 | High — 14 files simplified, pattern established |
| 4 | Generated dispatch consolidation | Medium | 460 | Medium — template + runtime changes |
| 5 | Lore/Writ consumer unification | Medium | 200 | Medium — reduces drift between consumers |
| 6 | Hook/lint Starlark dedup | Low | 140 | Medium — quick win |
| 7 | Test helper expansion | Low | 200 | Medium — improves test maintainability |
| 8 | View/state consolidation | Medium | 80 | Low — smaller payoff |
