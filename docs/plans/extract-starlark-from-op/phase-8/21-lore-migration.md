
.---
title: "Phase 8 · Step 21 (Bucket 4): migrate cmd/lore onto the sealed Graph API"
parent: "docs/plans/extract-starlark-from-op/phase-8/21-graph-immutability.md"
issue: TBD
status: in-progress
created: 2026-06-02
updated: 2026-06-03
---

# Migrate `cmd/lore` onto the sealed Graph API

## Summary

`cmd/lore` does not compile against the sealed `Graph`/`Subgraph`/`Node` API. It is the hard prerequisite for
[demo-milestone](../demo-milestone.md) **Scenario 1** (`lore deploy docker` on Darwin + Linux) — nothing in that
scenario runs until lore builds. This plan has three parts:

- **Part A.0 — Spec-builder construction API** (framework): replace the positional `NewNode`/`NewSubgraph`/
  `NewGraph` signatures with fluent-builder `*Spec` types (`ExecutableUnitSpec` embedded in `NodeSpec` and
  `SubgraphSpec`; `GraphSpec` standalone) passed to `NewX(spec *Spec)` constructors, and remove the last public *unit* mutator
  (`Promise`/`Invocation.FillSlot` → pure `SlotValue()`). The `With*` setters sit on the builder (pre-construction);
  the unit `NewX` returns is sealed — so this advances the step-21 seal, it does not loosen it.
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

## Part A.0 — Spec-builder construction API

### Why

The sealed constructors are positional with many optional fields, most passed `nil` at most call sites, and adding
a field (e.g. `slots` to `NewNode`) breaks every caller. `*Spec` is already the house convention: the two existing
specs are **`RuntimeEnvironmentSpec`** (`pkg/op/runtime_environment_spec.go`) and **`PlatformSpec`**
(`pkg/platform/spec.go`) — both **fluent builders** (`New*Spec` + chainable `With*`; `PlatformSpec` adds a terminal
`Build() (Platform, error)`). (There is **no `ResourceSpec`** — an earlier draft of this plan cited one that does not
exist.) Specs kill the nil-noise and make new fields non-breaking, and they let us delete the last public *unit*
mutators. The step-21 seal is about the immutable unit ("no post-construction mutation of `Node`/`Subgraph`/`Graph`"):
a fluent spec builder is **pre-construction** scaffolding and does not breach it.

**Placement standard (user-directed 2026-06-03):** a spec is a fluent builder, but it is a **supporting type** of the
unit it builds — it lives in that unit's file inside `// region SUPPORTING TYPES` (with its own `// region <SpecName>`
sub-region, the way `AnnotationMap` carries its methods), never in a standalone `*_spec.go` file. This realigns the two
existing specs too: **`PlatformSpec` moves to `pkg/platform/platform.go`** and **`RuntimeEnvironmentSpec` moves to
`pkg/op/runtime_environment.go`**, each into that file's `SUPPORTING TYPES` region; the standalone `spec.go` /
`runtime_environment_spec.go` files go away.

### Shape (`pkg/op`)

Each spec is a **fluent builder** (the house convention — `New*Spec` + pointer-receiver `With*` returning the
receiver, exactly like `RuntimeEnvironmentSpec`/`PlatformSpec`) and is passed to a `NewX(spec *Spec)` constructor —
**no `Build()` method** (decision 2026-06-03). The four are shown together for design clarity, but **each lands in
the file of the unit it constructs**, as a supporting type — see "File placement" below.

```go
// ExecutableUnitSpec carries the shared executableUnit fields; the unit specs embed it.
type ExecutableUnitSpec struct {
    ID          string
    Action      Action
    Annotations map[string]any
    Slots       map[string]SlotValue
    ErrorAction *Subgraph
    RetryPolicy *RetryPolicy
}
func (s *ExecutableUnitSpec) WithID(id string) *ExecutableUnitSpec          // one With* per field
func (s *ExecutableUnitSpec) WithAction(a Action) *ExecutableUnitSpec       // (WithAnnotations, WithSlot,
func (s *ExecutableUnitSpec) WithSlot(name string, v SlotValue) *ExecutableUnitSpec  //  WithErrorAction, WithRetryPolicy)

type NodeSpec     struct { ExecutableUnitSpec }                             // adds nothing; re-declares the 6 With* → *NodeSpec
type SubgraphSpec struct { ExecutableUnitSpec; Children []ExecutableUnit }  // re-declares the 6; + WithChildren(...ExecutableUnit) *SubgraphSpec
type GraphSpec    struct {
    Origin          Origin
    Units           []ExecutableUnit
    Slots           map[string]SlotValue
    ResourceCatalog *ResourceCatalog
    ErrorAction     *Subgraph
    RetryPolicy     *RetryPolicy
    SopsClient      *sops.Client
}                                                                           // + WithOrigin/WithUnits/...

func NewNodeSpec() *NodeSpec
func NewSubgraphSpec() *SubgraphSpec
func NewGraphSpec() *GraphSpec

// constructors take a pointer spec and return the sealed unit — mirrors
// NewRuntimeEnvironment(ctx, *RuntimeEnvironmentSpec); NewPlatform(spec) replaces PlatformSpec.Build().
func NewNode(spec *NodeSpec) (*Node, error)
func NewSubgraph(spec *SubgraphSpec) (*Subgraph, error)
func NewGraph(spec *GraphSpec) (*Graph, error)
```

`ExecutableUnitSpec` is **embedded** (not flattened) — it mirrors `Subgraph embeds executableUnit` and the `*Base`
family. **Symmetry decision (2026-06-03):** both `NodeSpec` and `SubgraphSpec` embed `ExecutableUnitSpec` and **each
re-declares all 6 inherited `With*`** to return its own type — so the 6 signatures appear in 3 places (the base's
real bodies + Node's shadows + Subgraph's shadows), the price of Go's embedding-isn't-inheritance semantics. Each
shadow is a one-line delegate:

```go
func (s *NodeSpec) WithAction(a Action) *NodeSpec { s.ExecutableUnitSpec.WithAction(a); return s }
```

The re-declared method sits at depth 0 and shadows the promoted one, so `*NodeSpec` / `*SubgraphSpec` chain
seamlessly (and `*SubgraphSpec` flows into its own `WithChildren`); the base method stays reachable only through the
embedded field (`s.ExecutableUnitSpec.WithAction(...)`), which normal fluent use never touches. `NodeSpec` was an
alias in an earlier draft; we chose the embedded struct so all unit specs read identically. `GraphSpec` does not
embed `ExecutableUnitSpec` — it builds a `Graph` (a document container, not an `ExecutableUnit`), so it has no
`ID`/`Action`/`Annotations` to share and carries its own `With*`. The seal is intact: the `With*` setters live on
the **builder** (pre-construction); the `Node`/`Subgraph`/`Graph` that `NewX(spec)` returns is immutable.

### File placement (style §1 item 10 — "all other structs", `// region SUPPORTING TYPES`)

`*Spec` fluent builders and `*Data` wire DTOs are **supporting types**, not primary: each lives in its unit's file
inside `// region SUPPORTING TYPES`, alphabetically. A spec with methods carries them in its own `// region <SpecName>`
sub-region (the way `AnnotationMap` does). The interface/base pattern is reserved for the unit types themselves
(`Resource`/`ResourceBase`, `executableUnit`, etc.). This migration aligns every touched type onto that one pattern —
which means relocating the two `*Data` DTOs that float outside any region **and** the two existing standalone specs.

| File | Main type(s) at top | `// region SUPPORTING TYPES` (alphabetical) |
|---|---|---|
| `executable_unit.go` | `ExecutableUnit` iface, `executableUnit` base | `AnnotationMap`, **`ExecutableUnitSpec`** (new) |
| `graph.go` | `Graph` | `Collision`, `Edge`, `Encoder`, **`GraphSpec`** (new), **`graphData`** (relocated from line 444) — `Origin` struct **removed** → `origin.go` |
| `node.go` | `Node` | **`NodeSpec`** (new; embeds `ExecutableUnitSpec`), **`nodeData`** (relocated from line 189) |
| `subgraph.go` | `Subgraph` | `Attempt`, **`SubgraphSpec`** (new), `subgraphData` (already placed) |
| `runtime_environment.go` | `RuntimeEnvironment` | **`RuntimeEnvironmentSpec`** (moved from `runtime_environment_spec.go`, which is deleted) + the existing `ConflictPolicy` it carries |
| `pkg/platform/platform.go` | `Platform` | **`PlatformSpec`** (moved from `spec.go`, which is deleted) |

Two pre-existing-code relocations beyond the new types:
- `graphData` (graph.go:444) and `nodeData` (node.go:189) sit *between* an `endregion` and `// region UNEXPORTED
  METHODS`, unlike `subgraphData` which is already inside `SUPPORTING TYPES`. Part A.0 touches all three files, so it
  relocates the two stragglers.
- `RuntimeEnvironmentSpec` and `PlatformSpec` currently each own a standalone `*_spec.go` file as that file's main
  type. The placement standard makes them supporting types: fold each into its unit's file (`runtime_environment.go`,
  `pkg/platform/platform.go`) under `SUPPORTING TYPES` and delete the standalone file. (`git mv` then move the block,
  to preserve history.) **`PlatformSpec` also loses its `Build()`** (`spec.go:129`): replace it with a package
  function `func NewPlatform(spec *PlatformSpec) (Platform, error)` — the `validateDistro` check it ran becomes the
  constructor's error return — so `PlatformSpec` matches the `RuntimeEnvironment(ctx, spec)` constructor convention.
  (`RuntimeEnvironmentSpec` already has no `Build()`; no change there beyond the move.)

### Unit-mutator elimination

The `With*` setters live on the spec **builder** (pre-construction); what the seal forbids is mutating a *unit*
after `NewX(spec)` returns it. This section removes the last such unit mutator.

- The public construction API already has **no exported `Set*`** on `Node`/`Subgraph`/`Graph` (the seal removed
  them; the old lore `node.SetSlot`/`target.AddChild` calls are themselves compile breaks).
- The one remaining public *unit* mutator — `Promise.FillSlot(consumer *Node, slot)` / `Invocation.FillSlot(...)` —
  becomes pure: **`Promise.SlotValue() SlotValue`** / **`Invocation.SlotValue() SlotValue`** (returns the
  `PromiseValue{UnitRef, Slot}`). Callers feed it to the spec via `spec.WithSlot(name, value)` before construction.
- `ActionPlanner.Plan` (`planner.go`) switches from construct-then-`setSlot`/`setErrorAction`/`setRetryPolicy` to
  **gather-then-construct**: build the spec (`NewNodeSpec().WithAction(…).WithSlot(…)` for each immediate / variable /
  `promise.SlotValue()` slot, then `WithErrorAction`/`WithRetryPolicy`), then one `NewNode(spec)`.
- Unexported `setSlot`/`addChild` survive only inside `populate` (the constructor body); the load path keeps its
  direct field assignment (`node.slots = p.Slots`). Net public surface: **zero post-construction unit mutators**.

### Part A.0 blast radius

| Site | Change |
|---|---|
| `pkg/op/node.go`, `subgraph.go`, `graph.go` | fluent-builder spec types (`New*Spec` + `With*`) + `NewX(spec *Spec)` constructors; delete positional params |
| `pkg/op/promise.go`, `invocation.go` | `FillSlot(node, slot)` → `SlotValue() SlotValue`; drop the mutator |
| `pkg/op/planner.go` | gather-then-construct in `ActionPlanner.Plan` (build spec via `With*`, then `NewNode(spec)`) |
| `pkg/op/load.go` | construct via spec (keeps direct field population post-decode) |
| `pkg/op/provider/flow/planners.go` | frame-binding now flows in via `spec.WithSlot(...)` |
| `pkg/op/runtime_environment.go`, `pkg/platform/platform.go` | absorb the moved `RuntimeEnvironmentSpec` / `PlatformSpec` into `SUPPORTING TYPES`; delete the `*_spec.go` files; replace `PlatformSpec.Build()` with `NewPlatform(spec *PlatformSpec) (Platform, error)` |
| `NewNode`/`NewSubgraph`/`NewGraph` + `NewPlatform` callers (13 + …) | move to fluent spec builders (lore, writ×2, tests, gen template; `PlatformSpec.Build()` call sites → `NewPlatform`) |

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

File layout mirrors `resource.go` exactly: the `Origin` **interface** is the main type; `OriginBase` (the
consumer-facing embedded base) sits **immediately after it**; its methods follow in the region hierarchy; and the
method-less wire DTO `originData` is the lone **supporting type** at the end.

```go
// --- main type + base (top of file) ---

// Origin is the framework contract: it reads Scope (filename key) and round-trips Tool + Annotations.
type Origin interface {
    Tool() string
    Scope() string
    Annotations() AnnotationMap
}

// OriginBase is the consumer-facing embedded base; tools embed/wrap it and project typed views over Annotations.
// Its fields are unexported (the seal: set at construction, immutable thereafter), so it cannot be reflected
// onto the wire directly — it (un)marshals through `originData`.
type OriginBase struct {
    tool        string
    scope       string
    annotations AnnotationMap
}

func NewOriginBase(tool, scope string, annotations AnnotationMap) OriginBase

// region EXPORTED METHODS

// region State management
func (o OriginBase) Tool() string               { return o.tool }
func (o OriginBase) Scope() string              { return o.scope }
func (o OriginBase) Annotations() AnnotationMap { return o.annotations }
// endregion

// region Behaviors
// Fallible actions — JSON + YAML only (see "Text: not needed" below); each converts through originData.
func (o OriginBase) MarshalJSON() ([]byte, error)
func (o OriginBase) MarshalYAML() (any, error)
func (o *OriginBase) UnmarshalJSON([]byte) error
func (o *OriginBase) UnmarshalYAML(func(any) error) error
// endregion

// endregion

// region SUPPORTING TYPES

// originData is the unexported wire DTO — the flat {tool, scope, annotations} shape with exported, tagged
// fields. It exists ONLY to (de)serialize OriginBase, mirroring the house convention (graphData, subgraphData,
// nodeData, ReceiptData). OriginBase's four marshal methods project to/from it; nothing else references it.
type originData struct {
    Tool        string        `json:"tool,omitempty"        yaml:"tool,omitempty"`
    Scope       string        `json:"scope,omitempty"       yaml:"scope,omitempty"`
    Annotations AnnotationMap `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// endregion
```

**Text: not needed (checked 2026-06-03).** `ResourceBase` carries `MarshalText`/`UnmarshalText` because a Resource
*is a URI scalar* — consumed as JSON map keys, YAML scalars, and `flag.TextVar` flags (`resource.go:210`). Origin is
the opposite: a composite `{tool, scope, annotations}` object that only ever appears as a **nested object** inside
`graphData` (`graph.go:450`) and `canonicalGraph` (`graph.go:317`) — never a map key, flag, URI, or scalar, so nothing
triggers `encoding.TextMarshaler`. A text form of a composite is ill-defined (JSON-in-a-string) and speculative.
`originData` therefore backs JSON + YAML only; if a scalar Origin use ever appears we add text then.

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
sub-object decodes into a concrete `OriginBase` (the graph holds it as the `Origin` interface), and `OriginBase`'s
own marshal methods route through `originData` (the unexported wire DTO above). So `graphData.Origin` / `canonicalGraph.Origin`
become `OriginBase` (concrete), `g.origin` (interface) marshals via `OriginBase.MarshalJSON` → `originData`, and the
decode reads `originData` → populates `OriginBase`'s unexported fields. One concrete shape, one DTO, no Tool→factory.

### Tool-side views (recommended shape — open question O5)

Tools read `graph.Origin()` (an `op.Origin` interface; after a load it is an `OriginBase`). To survive the
round-trip, the typed views **wrap the interface** rather than embed the base:

```go
// package lore — read side (lore.Origin)
type Origin struct{ op.Origin }
func (o Origin) Packages() []string         { v, _ := o.Annotations().Get("packages"); … }
func (o Origin) Platform() string           { … }
func (o Origin) Features() []string          { … }
func (o Origin) Settings() map[string]string { … }

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
| `cmd/writ/...` | **audit** writ's `op.Origin{Tool:"writ",…}` construction + reads; migrate to `NewOriginBase` + a `writ.Origin` view (same convention as `lore.Origin`) |
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

- **O1 — node slots. RESOLVED by Part A.0.** A node receives slots through the spec builder at construction;
  `addNativeSoftwarePackages` builds the install node via `op.NewNode(op.NewNodeSpec().WithAction(…).WithSlot(…))`.
  No post-construction unit setter.
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

1. **Part A.0** in `pkg/op` — fluent-builder spec types + `NewX(spec *Spec)` constructors + `FillSlot`→`SlotValue` +
   planner gather-then-construct + the `RuntimeEnvironmentSpec`/`PlatformSpec` relocations (and `PlatformSpec.Build()`
   → `NewPlatform`) + migrate all callers. Gate: `pkg/op` + `pkg/platform` build, `make vet` clean, tests green.
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
