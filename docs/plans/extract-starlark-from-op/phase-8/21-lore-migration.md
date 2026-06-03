---
title: "Phase 8 · Step 21 (Bucket 4): migrate cmd/lore onto the sealed Graph API"
parent: "docs/plans/extract-starlark-from-op/phase-8/21-graph-immutability.md"
issue: TBD
status: draft
created: 2026-06-02
updated: 2026-06-02
---

# Migrate `cmd/lore` onto the sealed Graph API

## Summary

`cmd/lore` does not compile against the sealed `Graph`/`Subgraph`/`Node` API. It is the hard prerequisite for
[demo-milestone](../demo-milestone.md) **Scenario 1** (`lore deploy docker` on Darwin + Linux) — nothing in that
scenario runs until lore builds. This plan has three parts:

- **Part A.0 — Spec-based, setter-free construction API** (framework): replace the positional `NewNode`/
  `NewSubgraph`/`NewGraph` signatures with `*Spec` structs (`ExecutableUnitSpec` embedded in `NodeSpec`/
  `SubgraphSpec`/`GraphSpec`), and remove the last public construction mutator (`Promise`/`Invocation.FillSlot` →
  pure `SlotValue()`). Advances the step-21 seal directly: every unit is construction-complete, no setters.
- **Part A — `op.Origin` redesign** (framework): promote `Origin` from a concrete struct to an **interface +
  `OriginBase`** pair, mirroring `Resource`/`ResourceBase` and `Provider`/`ProviderBase`. Tool-specific data
  (lore: packages/platform/features/settings) lives in `Annotations`; tools project **typed read-only views** over
  it. The interface/base/base-methods all live in a new **`pkg/op/origin.go`**.
- **Part B — lore builds via the plan provider** (consumer): rewrite `cmd/lore/lore/builder.go` to drive the
  shared `plan.Provider` (the machinery the `.star` scripts use) instead of hand-rolling `NewNode`/`NewSubgraph` —
  Go-built and script-built invocations pool in one registry, and a single `Assemble` materializes the graph.

Part A.0 and Part A are framework-wide and small; Part B is the bulk of the work.

## Goal & exit criteria

- `make build` / `make vet` clean for `cmd/lore/...`.
- `pkg/op` stays green after the Origin change (no serialization regression — graphs still round-trip).
- A smoke check: `lore.Build(...)` assembles a non-empty `*op.Graph` for a resolved package on the host platform
  (graph construction correct; **executing** it end-to-end is the rest of Scenario 1, out of scope here).

Out of scope (the remainder of Scenario 1, tracked separately): the four planned-primitive gaps (`platform.arch`,
`plan.download`, planned `plan.file.remove`, `phase.env`), the docker registry-script rewrite, and an actual
end-to-end `lore deploy docker` run.

---

## Part A.0 — Spec-based, setter-free construction API

### Why

The sealed constructors are positional with many optional fields, most passed `nil` at most call sites, and adding
a field (e.g. `slots` to `NewNode`) breaks every caller. `*Spec` is already the house convention
(`RuntimeEnvironmentSpec` at `pkg/op/runtime_environment_spec.go`, `ResourceSpec`). Spec structs kill the
nil-noise, make new fields non-breaking, and — being the gathered construction payload — let us delete the last
public mutators. This is the step-21 seal goal ("no post-construction mutation"), finished.

### Shape (`pkg/op`)

```go
// shared base — the executableUnit fields, in one place; embedded by the unit specs
type ExecutableUnitSpec struct {
    ID          string
    Action      Action
    Annotations map[string]any
    Slots       map[string]SlotValue
    ErrorAction *Subgraph
    RetryPolicy *RetryPolicy
}

type NodeSpec     = ExecutableUnitSpec                                   // a node is a leaf unit; nothing to add
type SubgraphSpec struct { ExecutableUnitSpec; Children []ExecutableUnit }
type GraphSpec    struct {
    Origin          Origin
    Units           []ExecutableUnit
    Slots           map[string]SlotValue
    ResourceCatalog *ResourceCatalog
    ErrorAction     *Subgraph
    RetryPolicy     *RetryPolicy
    SopsClient      *sops.Client
}

func NewNode(spec NodeSpec) (*Node, error)
func NewSubgraph(spec SubgraphSpec) (*Subgraph, error)
func NewGraph(spec GraphSpec) (*Graph, error)
```

`ExecutableUnitSpec` is **embedded** (not flattened) — it mirrors `Subgraph embeds executableUnit` and the
`*Base` family. `NodeSpec` is a flat alias (a node adds nothing), so node call sites stay flat; `SubgraphSpec`
literals nest the embedded base, which lands where subgraphs are mostly framework/planner-built.

### Setter elimination

- The public construction API already has **no exported `Set*`** on `Node`/`Subgraph`/`Graph` (the seal removed
  them; the old lore `node.SetSlot`/`target.AddChild` calls are themselves compile breaks).
- The one remaining public mutator — `Promise.FillSlot(consumer *Node, slot)` / `Invocation.FillSlot(...)` —
  becomes pure: **`Promise.SlotValue() SlotValue`** / **`Invocation.SlotValue() SlotValue`** (returns the
  `PromiseValue{UnitRef, Slot}`). Callers put it into `spec.Slots` at construction.
- `ActionPlanner.Plan` (`planner.go`) switches from construct-then-`setSlot`/`setErrorAction`/`setRetryPolicy` to
  **gather-then-construct**: build the `Slots` map (immediate / variable / `promise.SlotValue()`) and resolve
  error/retry, then one `NewNode(NodeSpec{…})`.
- Unexported `setSlot`/`addChild` survive only inside `populate` (the constructor body); the load path keeps its
  direct field assignment (`node.slots = p.Slots`). Net public surface: zero construction setters/mutators.

### Part A.0 blast radius

| Site | Change |
|---|---|
| `pkg/op/node.go`, `subgraph.go`, `graph.go` | spec structs + spec-taking constructors; delete positional params |
| `pkg/op/promise.go`, `invocation.go` | `FillSlot(node, slot)` → `SlotValue() SlotValue`; drop the mutator |
| `pkg/op/planner.go` | gather-then-construct in `ActionPlanner.Plan` |
| `pkg/op/load.go` | construct via spec (keeps direct field population post-decode) |
| `pkg/op/provider/flow/planners.go` | frame-binding now flows into `spec.Slots` |
| `NewNode`/`NewSubgraph`/`NewGraph` callers (13 + …) | move to spec literals (lore, writ×2, tests, gen template) |

---

## Part A — `op.Origin` interface + `OriginBase` (new file `pkg/op/origin.go`)

### Current state

`op.Origin` is a concrete struct (today at `pkg/op/graph.go:562`):

```go
type Origin struct {
    Tool        string        `json:"tool,omitempty"`
    Scope       string        `json:"scope,omitempty"`        // the ONLY field the framework reads (filename key)
    Annotations AnnotationMap `json:"annotations,omitempty"`  // framework round-trips, never inspects
}
```

It already embodies the "tool data in `Annotations` + typed accessors" model (per the 2026-05-30 Origin redesign).
The limitation: being concrete and owned by `pkg/op`, a tool (lore/writ) cannot give it **typed methods/fields** —
it is stuck stuffing/reading the untyped `Annotations` bag directly.

### Target state — `pkg/op/origin.go`

```go
// Origin is the framework contract: it reads Scope (filename key) and round-trips Tool + Annotations.
type Origin interface {
    Tool() string
    Scope() string
    Annotations() AnnotationMap
}

// OriginBase is the single serialized carrier; tools embed/wrap it and project typed views over Annotations.
type OriginBase struct {
    tool        string
    scope       string
    annotations AnnotationMap
}

func NewOriginBase(tool, scope string, annotations AnnotationMap) OriginBase
func (o OriginBase) Tool() string             { return o.tool }
func (o OriginBase) Scope() string            { return o.scope }
func (o OriginBase) Annotations() AnnotationMap { return o.annotations }

// Marshaling lives here too: OriginBase (un)marshals to the flat {tool, scope, annotations} shape.
func (o OriginBase) MarshalJSON() ([]byte, error)
func (o OriginBase) MarshalYAML() (any, error)
func (o *OriginBase) UnmarshalJSON([]byte) error
func (o *OriginBase) UnmarshalYAML(func(any) error) error
```

### Why interface + base is the right call (and serialization stays simple)

- It gives lore/writ a **typed Origin** (`Features()`, `Settings()`, `Packages()`, `Platform()` as real methods),
  matching the `Resource`/`ResourceBase` symmetry — the limitation the concrete struct can't address.
- Because tools extend via **views over `Annotations`** (methods, not extra typed fields), every concrete Origin
  serializes to the **same flat `{tool, scope, annotations}` shape**. So deserialization decodes into a single
  concrete type, `OriginBase` — **one custom `UnmarshalJSON`, no Tool→factory registry**. A factory is needed only
  if a future subtype carries distinct typed *fields*; the views-over-Annotations design deliberately avoids that.

### Serialization handling (the one real cost)

The graph's persist/marshal structs declare the origin by type and decode it directly:
- `graph.go:317` and `graph.go:450`: `Origin Origin` fields in the marshal/persist structs.
- `load.go:84`: `origin: p.Origin`.

With `Origin` now an interface, those fields no longer auto-unmarshal. Fix: the persisted-graph struct's origin
sub-object decodes into a concrete `OriginBase` (the graph holds it as the `Origin` interface). Implemented as
`OriginBase.UnmarshalJSON`/`UnmarshalYAML` plus a concrete `OriginBase` field (or a small custom unmarshal) on the
persist struct. Marshal side: `g.origin` (interface) marshals through `OriginBase.MarshalJSON`.

### Tool-side views (recommended shape — open question O5)

Tools read `graph.Origin()` (an `op.Origin` interface; after a load it is an `OriginBase`). To survive the
round-trip, the typed views **wrap the interface** rather than embed the base:

```go
// cmd/lore — read side
type LoreOrigin struct{ op.Origin }
func (o LoreOrigin) Packages() []string         { v, _ := o.Annotations().Get("packages"); … }
func (o LoreOrigin) Platform() string           { … }
func (o LoreOrigin) Features() []string          { … }
func (o LoreOrigin) Settings() map[string]string { … }

// build side
origin := op.NewOriginBase("lore", scope, op.NewAnnotationMap(map[string]any{
    "packages": packages, "platform": targetPlatform,
    "features": cfg.Features, "settings": cfg.Settings,
}))
```

(Whether lore should instead *embed* `OriginBase` à la `ResourceBase` is O5 below — embedding doesn't survive the
load round-trip since the stored concrete type reverts to `OriginBase`, so a wrapping view is cleaner. Flagged for
your call.)

### Part A blast radius

| Site | Change |
|---|---|
| `pkg/op/origin.go` (new) | interface + `OriginBase` + methods + marshal/unmarshal |
| `pkg/op/graph.go:562` | delete the `Origin` struct (moved to origin.go) |
| `pkg/op/graph.go:57,127,240` | `origin Origin` field / `NewGraph` param / `Origin()` getter — now interface-typed (no body change) |
| `pkg/op/graph.go:317,450` | marshal/persist structs — origin sub-object decodes into `OriginBase` |
| `pkg/op/load.go:84` | `p.Origin` now satisfies the interface via `OriginBase` |
| `pkg/op/dependencyview.go:464` | `v.graph.Origin()` — passes the interface, no change |
| `pkg/op/provider/plan/provider.go:176` | `Assemble` does `origin.Tool = app.Name` (struct field) — under the interface, construct/replace via `OriginBase` instead |
| `cmd/writ/...` | **audit** writ's `op.Origin{Tool:"writ",…}` construction + reads; migrate to `NewOriginBase` + a `WritOrigin` view |
| `cmd/lore/lore/builder.go` | build a lore `OriginBase` with Annotations, pass to `pp.Assemble(...)` (Part B) |

---

## Part B — lore builds via the plan provider

lore does **not** call `op.NewNode`/`op.NewSubgraph`/`op.NewGraph` directly. It drives the **shared
`plan.Provider`** — the same machinery the `.star` phase scripts use. Both paths register invocations into one
registry (the env's cached `plan.Provider.invocations`); a single `Assemble` materializes the graph. lore never
touches the sealed constructors or an `ActivationRecord` — planning is not execution.

### Framework add — a Go-facing plan entry

```go
// pkg/op/provider/plan — register an invocation from Go, mirroring what the bridge does from starlark.
// The framework formulates the action from `name`; the caller never builds an op.Action.
func (p *Provider) Plan(name string, args []any, kwargs map[string]any) (*op.Invocation, error)
// resolves `name` ("pkg.install") → receiverType + method via env.Registry, calls ActionPlanner.Plan,
// wraps the node in an *op.Invocation, registers it in p.invocations, and returns it.
```

### How lore builds (sketch)

```go
sharedEnv := op.NewRuntimeEnvironment(ctx, spec)              // ONE env, all modules
pp := sharedEnv.ProviderByType(reflect.TypeFor[plan.Provider]()).(*plan.Provider) // shared registry owner

// native-software-packages — Go path:
pp.Plan("pkg.install", nil, map[string]any{"packages": …, "phase": …})

// phase script — starlark path, SAME registry; lifecycle verbs denied:
rt := starlarkbridge.NewRuntime(sharedEnv, denyLifecycleVerbs) // plan.assemble/run/save/load/clear denied
runPhaseScript(rt, pkg, phase)                                 // plan.* self-registers into pp.invocations

// once, at the end:
graph, _ := pp.Assemble(pp.InvocationRegistry().All(), nil, nil, nil, loreOrigin(scope))
```

### Helper inversion (consumers stop mutating a graph)

| Function (current → new) |
|---|
| `addNativePMNodes(target *Subgraph, …) error` → `addNativeSoftwarePackages(pp, pkg, action) (*op.Invocation, error)` — one `pp.Plan("pkg.install", …)` |
| `executeScriptAction(graph, …) (*RetryPolicy, error)` → runs the phase script on a runtime over the **shared env** (lifecycle verbs denied); `plan.*` self-registers — no return-wiring |
| `prepareScriptEnv(graph, …)` → takes the **shared env** (drop the per-script `op.NewRuntimeEnvironment`) |
| `buildPackageNodes(graph, …) error` → `buildPackage(pp, pkg, platform, cfg)` — drives the phases; phase→subgraph grouping per the deferred O2 sub-design |
| `PlanPackages/PlanByName(graph, …) ([]string, error)` → `(pp, …) ([]string, error)` — drive `pp`, return resolved package names |
| `Build(cfg)` | one env → register all invocations (Go + scripts) → `pp.Assemble(…, loreOrigin(scope))` |

---

## Open questions / decisions

O1 and O2 are resolved in approach (mechanics below); O3–O5 and the O2 phase-grouping sub-design remain to decide
at implementation.

- **O1 — node slots. RESOLVED by Part A.0.** A node receives slots through `NodeSpec.Slots` at construction;
  `addNativeSoftwarePackages` builds the install node via `op.NewNode(op.NodeSpec{…, Slots})`. No setter.
- **O2 — script-action invocation collection. RESOLVED (approach): share the env's plan provider.** `buildOne`
  binds the `plan` global to the env's *cached* `plan.Provider` (`ModuleByName` → `cachedProvider`, runtime.go:336),
  which owns the one invocation registry. The only gap was `prepareScriptEnv` (builder.go:443) creating a *fresh*
  env per script → a throwaway registry. Fix: lore's `Build` constructs one env (all modules); each phase script
  runs on `starlarkbridge.NewRuntime(sharedEnv)`, so `plan.*` registers into the shared `plan.Provider`; lore's Go
  path registers into the same registry via `plan.Provider.Plan(...)`; a single `Assemble` collects both. The
  phase-script runtimes are **denied the lifecycle verbs** (`plan.assemble`/`run`/`save`/`load`/`clear`, via the
  existing `applyDenials`) so scripts only contribute, never orchestrate. **Sub-design deferred to implementation
  (per the user):** phase attribution (which invocations form which phase subgraph) and session-wide node-ID
  uniqueness — candidate mechanisms include per-phase `Clear()`/collect, a per-phase scope where lore folds its
  `pp.Plan` invocations into the phase subgraph (the deny-assemble idea), or snapshot-delta. Decide when we get there.
- **O3 — `sopsClient`.** lore passes `nil` (no signing) for now? Confirm.
- **O4 — provenance key.** `node.Origin = pkg.Name` → which annotation key (`"package"`?).
- **O5 — lore/writ view shape.** Wrap `op.Origin` (survives load) vs. embed `OriginBase` (ResourceBase-style, but
  reverts to `OriginBase` on load). Recommendation: wrap. Also: shared `Annotations`-accessor helper for lore+writ?

---

## Sequencing

1. **Part A.0** in `pkg/op` — spec structs + spec constructors + `FillSlot`→`SlotValue` + planner gather-then-
   construct + migrate all callers. Gate: `pkg/op` builds, `make vet` clean, `pkg/op` tests green.
2. **Part A** in `pkg/op` (origin.go + graph/load wiring + writ audit). Gate: `pkg/op` builds, `make vet` clean,
   `pkg/op` tests green (graphs round-trip through save/load).
3. **Part B** — drive the plan provider in `cmd/lore`: add `plan.Provider.Plan(name, args, kwargs)` (framework),
   share one env across the Go builder + phase scripts (deny lifecycle verbs to script runtimes), register via
   `pp.Plan` + `plan.*`, then one `pp.Assemble`. O2 approach resolved; its phase-grouping sub-design is decided
   here at implementation. Gate: `cmd/lore/...` builds, `make vet` clean.
4. **Smoke check**: `lore.Build` assembles a non-empty graph for a host-platform package.
5. (Separate) the Scenario-1 remainder: primitive gaps + docker registry-script rewrite + end-to-end run.

## Verification

- `go build ./cmd/lore/...` clean; `make vet`.
- `pkg/op` tests (incl. graph save/load round-trip) green.
- A focused `lore.Build` smoke test asserting `graph.UnitCount() > 0` for a resolved package.
