---
title: "Phase 7: Slot = (Parameter, Value) — type-driven fill and dispatch"
parent: "docs/plans/extract-starlark-from-op.md"
issue: 264
status: in-progress
created: 2026-04-15
updated: 2026-04-15
---

# Phase 7: Slot = (Parameter, Value)

## Summary

Redefine slot identity around `op.Parameter` (Name + Go Type), collapse
`SlotValue` to a sealed two-variant interface (`ImmediateValue`,
`PromiseValue`), unify nodes and subgraphs as executable units with
declared parameter surfaces, and replace every distinct execution path
(top-level, subgraph invocation, gather iteration, choose branch, test
harness) with a single `Execute(exec, overrides)` call. Delete the
proxy infrastructure — nothing in production ever emitted a proxy
slot.

## Design decisions

### D1 — `op.Slot` binds `Parameter` to `SlotValue`

```go
// Slot binds a Method parameter to its value in a Node.
type Slot struct {
    Parameter Parameter // by-value; Name + Type
    Value     SlotValue
}
```

Parameter identity travels with the value. The authoritative
`Parameter.Name` / `Parameter.Type` contract from `*op.Method` meets
the value it governs — the defect that forced three parallel
collections in `bind.Planner.dispatch` is structurally impossible
under this shape.

### D2 — `SlotValue` is a sealed three-variant interface

```go
// SlotValue is the value bound to a Slot. Sealed: only ImmediateValue,
// PromiseValue, and EnvironmentValue implement it. Extensibility is
// prevented — the set is closed at three.
type SlotValue interface {
    isSlotValue()
    Resolve(env RuntimeEnvironment, results map[string]any) any
}

// ImmediateValue is a Go value known at plan time.
type ImmediateValue struct {
    Value any
}

func (ImmediateValue) isSlotValue() {}

func (iv ImmediateValue) Resolve(_ RuntimeEnvironment, _ map[string]any) any {
    return iv.Value
}

// PromiseValue references the output of another executable unit,
// resolved to a Go value at execution time via the scope-chain results.
type PromiseValue struct {
    // NodeRef is the ID of the producing node or subgraph. One ID space.
    NodeRef string
    // Slot names which output of the producer to take; empty = default.
    Slot string
}

func (PromiseValue) isSlotValue() {}

func (pv PromiseValue) Resolve(_ RuntimeEnvironment, results map[string]any) any {
    if results == nil {
        return nil
    }
    return results[pv.NodeRef]
}

// EnvironmentValue binds a slot to a RuntimeEnvironment property,
// resolved at execution time. Authored at plan time via a starlark
// surface such as plan.env("target_root").
type EnvironmentValue struct {
    // Key is the RuntimeEnvironment.Property key to read at execute time.
    Key string
}

func (EnvironmentValue) isSlotValue() {}

func (ev EnvironmentValue) Resolve(env RuntimeEnvironment, _ map[string]any) any {
    if env == nil {
        return nil
    }
    v, _ := env.Property(ev.Key)
    return v
}
```

**Seal mechanism.** Unexported `isSlotValue()` on the interface; any
package outside `pkg/op` cannot define a variant without embedding a
library type. The seal prevents *extensibility* of the variant set,
not its cardinality — we have exactly three variants by design and no
mechanism for a fourth. Type switches at structural sites (edge
planner, serializer, validator) must include a `default: panic("unknown
SlotValue variant")` to fail loud on any embedding-based leak.

**Why `Resolve` is on the interface.** The common operation — "give me
the value, resolving if needed" — collapses to
`slot.Value.Resolve(env, results)`. Type switches remain only where
the variant's identity matters structurally (edge construction,
serialization, validation).

**Three semantic modes, not two.** A promise is a data-flow edge
between computations; an environment read is a query against ambient
runtime context. Earlier design drafts tried to collapse environment
reads into a promise with a magic `NodeRef == "env"` sentinel — that
forces every code path handling `PromiseValue` to know about the
magic value, which is the "suss out the intent" spaghetti Phase 7
was designed to eliminate. Three honest variants beats two variants
with a sentinel.

**No `ProxyValue`.** Production never emitted proxy slots; the gather
stub never executed. The entire proxy path is deleted in this phase.

### D3 — `Node.Slots` shape

```go
Slots       []*Slot                  `json:"slots,omitempty"`
SlotsByName map[string]*Slot         `json:"-"` // derived; rebuilt on load / mutation
```

- Slice is the single source of truth; preserves parameter order from
  the method signature for positional dispatch.
- Map is a derived index. Rebuilt on load (in `Node.Bind(method)`) and
  on every slot mutation.
- Only the slice serializes. Guarantees single-writer semantics and no
  wire-format drift.

### D4 — Nodes and subgraphs are executable units with declared parameters

Phase 7's invariant already states nodes and subgraphs are
interchangeable anywhere a reference is valid. Extension: the
interchangeability includes the parameter surface.

```go
// ExecutableUnit is anything that can be dispatched: a Node or a Subgraph.
type ExecutableUnit interface {
    ID() string
    Parameters() []Parameter
}
```

- `Node.Parameters()` returns `method.Parameters()` from its bound
  `*op.Method`.
- `Subgraph.Parameters()` **is the union (by name) of the parameters
  declared by every topological root of the subgraph**. A topological
  root is a child with no incoming edges from within the subgraph.
  If the subgraph has N roots running in parallel and each declares
  its own required inputs, the subgraph exposes all of them. Callers
  supply values for whichever apply.
- **Common case — single root.** The subgraph behaves exactly like
  "one entry": `subgraph.Parameters() == root.Parameters()`.
- **Multi-root case.** `Execute(subgraph, overrides)` routes each
  override key to the root that declared that parameter. Roots run
  in parallel (or as topology allows).
- **Recursive case.** If a root is itself a subgraph, its
  `Parameters()` is its own topological-root union — evaluated the
  same way.

**Name-collision handling.** Two roots declaring the same parameter
name (e.g., both declaring `path`) is a plan-time error. The author
must disambiguate — rename in the body, or restructure the subgraph.
Plan-time validation fails loudly; no shadowing, no implicit
disambiguation.

**No starlark API for explicit subgraph parameters.** Allowing
callers to declare named subgraph parameters that differ from the
roots' parameters would force a name-mapping layer between subgraph
parameters and root slots, complicating slot binding for no
operational benefit. The root-union rule collapses that mapping to
identity (override name → root's slot by the same name).

Gather does not need `InputSlot()` as a separate concept. It consumes
`body.Parameters()` like any other call site. Gather's "exactly one
parameter" constraint is orthogonal — it applies to the body's
parameter surface regardless of how that surface is derived. A
multi-root body whose root-union is 2+ parameters is rejected by
gather at plan time.

### D5 — Unified `Execute(exec, overrides)`

Every execution path in the system — top-level graph run, subgraph
invocation, gather iteration, choose branch, test harness — funnels
through one function:

```go
// Execute runs an executable unit with caller-supplied slot overrides.
// overrides wins over baked-in Slot.Value; baked-in wins over unfilled.
// Promise resolution walks the scope chain; results in the current
// scope are local to this call.
func (g *Graph) Execute(
    exec ExecutableUnit,
    overrides map[string]SlotValue,
) (result any, err error)
```

Resolution order per slot at execute time:
1. `overrides[paramName]` if present — `Resolve(results, env)` on the
   override value.
2. The executable unit's baked-in `Slot.Value` — dispatches per
   variant:
   - `ImmediateValue` → the stored Go value.
   - `PromiseValue` → results lookup via the scope chain.
   - `EnvironmentValue` → `env.Property(Key)` at the current scope.
3. Unfilled → plan-time validation should have caught this; at
   execute time it is a diagnostic bug.

Call-site mapping:

| Call site | Invocation |
|---|---|
| Top-level graph run | `graph.Execute(graph.Root, nil)` |
| Subgraph invocation from a parent node | `graph.Execute(sub, nil)` |
| Gather iteration | `graph.Execute(body, map[string]SlotValue{paramName: ImmediateValue{items[i]}})` |
| Choose branch | `graph.Execute(chosen, nil)` |
| Test node in isolation | `graph.Execute(node, map[string]SlotValue{"slot": ImmediateValue{value}})` |

**Overrides are runtime-only.** They do not serialize into the graph.
Plan-time bindings are the serialized state; overrides are execution
arguments. Catalog rebind (Phase 13) continues to operate on
baked-in slots only.

### D6 — Lexical scope rules

Every subgraph is a scope boundary — a fourth consequence of its
existing role as functional, structural, and transactional boundary.

- **Subgraph = scope.** Each `Execute` call establishes a results map
  scoped to that invocation.
- **Visibility — inner sees outer.** A node in subgraph S sees S's
  sibling completed results plus every enclosing subgraph's results.
  Lexical scope, outer → inner.
- **Parallel subgraphs are mutually invisible.** Two sibling
  subgraphs at the same level — including gather's N iterations — do
  not see each other's internal results. This is what makes parallel
  execution safe without locking.
- **Sibling visibility is gated by topology.** Within a single
  subgraph, a node sees a sibling's result only after the sibling has
  completed. Kahn's ordering enforces this inside the subgraph.
- **Promise resolution walks up the scope chain.** A promise
  `{NodeRef, Slot}` resolves in the innermost scope that contains a
  completed `NodeRef`. Not found → unresolved reference error.

### D7 — Gather implementation via unified `Execute`

Gather becomes a plain parallel loop. No proxy infrastructure. No
iteration-step nodes. No graph cloning.

```go
// Gather executes body once per item, up to `limit` concurrent,
// collecting results in item order.
//
// Body is a node or subgraph ID — one ID space (Phase 7 invariant).
// The body declares its iteration input via Parameters(); gather binds
// items[i] to the body's single expected input slot per iteration.
//
// Each iteration runs in its own scope: a fresh results map, no
// cross-iteration visibility. Outer-scope results are visible
// read-only through the scope chain.
func (p *Provider) Gather(items []any, body string, limit int) ([]any, error) {
    if len(items) == 0 {
        return []any{}, nil
    }
    if limit <= 0 {
        limit = 4 * runtime.NumCPU()
    }

    exec, err := p.Graph.ResolveExecutable(body)
    if err != nil {
        return nil, fmt.Errorf("gather: %w", err)
    }

    params := exec.Parameters()
    if len(params) != 1 {
        return nil, fmt.Errorf("gather: body %q must declare exactly one parameter; got %d",
            body, len(params))
    }
    inputName := params[0].Name

    results := make([]any, len(items))
    errs    := make([]error, len(items))
    sem     := make(chan struct{}, limit)
    var wg sync.WaitGroup

    for i, item := range items {
        i, item := i, item
        wg.Add(1)
        sem <- struct{}{}
        go func() {
            defer wg.Done()
            defer func() { <-sem }()

            r, runErr := p.Graph.Execute(exec, map[string]SlotValue{
                inputName: ImmediateValue{Value: item},
            })
            if runErr != nil {
                errs[i] = runErr
                return
            }
            results[i] = r
        }()
    }
    wg.Wait()

    return results, errors.Join(errs...)
}
```

Decisions baked in:
- **B-1 (γ)** — Body declares its input via `Parameters()`; no kwarg,
  no convention. Gather validates exactly one parameter.
- **B-2 (i)** — Fresh results map per iteration; no cross-iteration
  visibility.
- **B-3 (q)** — Run all iterations, aggregate failures via
  `errors.Join`. No fail-fast.

## Updated step outline

The original 10-step outline is revised to reflect the unified
`Execute` model. Old step 6 ("retire the gather proxy") is obsolete —
proxies are deleted in step 1; gather is implemented separately as
step 8.

1. **Introduce `op.Slot` and sealed `SlotValue`.** Three-variant
   sealed interface (`ImmediateValue`, `PromiseValue`,
   `EnvironmentValue`) with `Resolve(results, env)`.
   `Node.Slots []*Slot` + derived `SlotsByName`.
   Migrate `SlotByName`, `SetSlotImmediate`, `SetSlotPromise`,
   `RequireStringSlot`, `ResolvedSlots`. Delete `SetSlotProxy`,
   `IsProxy`, `GatherRef`, `Field`, `proxyCtx`, and their tests.
2. **ExecutableUnit interface.** `Node` and `Subgraph` both implement
   `Parameters() []op.Parameter`. Subgraph declares its input surface
   explicitly.
3. **Unified `Execute(exec, overrides)`.** Single executor entry point
   with lexical scope rules. Top-level `Graph.Execute()` collapses to
   `Execute(g.Root, nil)`. All choose / recovery / nested-subgraph
   call sites converge here.
4. **Type-converter contract.** `FromStarlark(sv starlark.Value)
   (any, error)` on `ReceiverType` (or a sibling interface —
   fork open). Primitives registered in `ReceiverRegistry` at init.
5. **Rewrite `bind.FillSlot`.** New signature takes `*op.Slot` +
   `starlark.Value`. Graph-edge dispatch first; delegate to
   `slot.Parameter.Type`'s converter otherwise. Delete free-function
   internals and the starlark-kind switch.
6. **Collapse `Planner.dispatch`.** Single pass over
   `method.Parameters()` producing `[]*op.Slot`. Delete
   `regularParams` / `knownKwargs` / `paramTypes` parallel maps.
   Delete `fillResourceSlot` entirely.
7. **Delete `Action.Do`.** Remove from the `Action` interface. Move
   reflection dispatch to `(*op.Method).Invoke(receiver any, slots
   []*op.Slot) (result, complement, err)`.
8. **Implement Gather via unified `Execute`.** Per D7. Body declares
   its iteration input via `Parameters()`; gather binds items[i] per
   iteration under fresh scope.
9. **Rebind.** `(*Node).Bind(method)` and `(*Graph).Bind(ctx)` rebind
   `Parameter.Type` from the registry on load. Slots serialize
   `Parameter.Name` + `Value` only; `Parameter.Type` reattached at
   load via `node.Receiver`.
10. **Provider update.** Delete every provider's `Do` boilerplate.
    Regenerate `*_gen.go`. Hand-written methods unchanged.
11. **Executor update.** All executor call sites consume `Slot` /
    `SlotValue` under the unified model. Replace surviving
    `slot.Immediate.(T)` casts with typed `Slot` accessors.
12. **Test triage.** Run the full suite. Most Phase 9 failures
    should resolve as side effects of the coherent model. Fold
    legitimate residuals into follow-up fixes.

## Open forks

### Fork C — `SlotByName` return type — RESOLVED: C1

`func (n *Node) SlotByName(name string) *Slot` returns the full slot
(or `nil` if absent). Every call site migrates in step 1; no
convenience wrapper for immediate-only access — callers use
`slot.Value.Resolve(results)` or a type switch where structural.

### Fork D — `SetSlotImmediate(name, value)` before method binding — RESOLVED: D2

`SetSlotImmediate` requires the node to be bound to a method. It
looks up `Parameter` in `node.method.Parameters()`; returns an error
(or panics — pick one during implementation) if the node is
unbound. Same rule applies to `SetSlotPromise`.

**Tests are rewritten from scratch**, not migrated. The existing
slot-exercise tests (notably in `pkg/op/graph_test.go`,
`pkg/op/lifecycle_test.go`, `pkg/op/subgraph_test.go`,
`pkg/op/recovery_test.go`) are tied closely to the old tri-union
`SlotValue` and to unbound-node construction. Patching them is more
work than starting over against the new model. Step 1 deletes the
old tests along with the proxy infrastructure; new tests are written
against `Slot`, sealed `SlotValue`, `Bind(method)`, and unified
`Execute`.

### Fork E — `ImmediateValue.Value` wire format — RESOLVED: E1 (punt to step 9)

Step 1 defines the Go types (`Slot`, `SlotValue` interface,
`ImmediateValue`, `PromiseValue`, `EnvironmentValue`) with **no
JSON/YAML tags on the sealed interface**. Serialization is designed in step 9 (Rebind),
where catalog-driven rehydration requirements are concrete.

`any` cannot round-trip through Go's `encoding/json` without a
discriminator or an external type resolver — the marshaler works via
reflection, but the unmarshaler has no type information and defaults
to `map[string]any` / `[]any` / `float64`. Step 9 picks the
mechanism (most likely catalog-driven rehydration per Rebind).

### Fork F — `DataRef` under the sealed `SlotValue` — RESOLVED: F4 (`EnvironmentValue`)

Phase 9 documented a `DataRef` concept for binding a slot to a
`RuntimeEnvironment.Property` key. It was never implemented; the
concept survives as a TODO bridge in `pkg/op/executor.go`.

**Decision:** `EnvironmentValue{Key string}` is the third variant of
the sealed `SlotValue` interface (see §D2). It replaces `DataRef`
entirely.

- Authored at plan time via `plan.env("target_root")`.
- Resolved at execution time via `env.Property(Key)`.
- No reserved node IDs, no magic sentinels in `PromiseValue`, no
  method-registry-only metadata.
- Three honest variants (Immediate, Promise, Environment) beat two
  variants plus a sentinel: every code path handling `PromiseValue`
  stays clean; `PromiseValue.Resolve` has no conditional branches.

Alternatives considered and rejected:
- **F1** (env as pseudo-node via reserved `NodeRef == "env"`) — forces
  every promise-handling code path to know about the magic value;
  `PromiseValue.Resolve` gains a sentinel branch.
- **F2** (env sourcing as runtime overrides from method registry
  metadata) — too restrictive; authors can't bind ad-hoc. Only
  provider-pre-declared env bindings would work.
- **F3** (pre-resolve to `ImmediateValue` at plan time) — loses
  lazy-resolve semantics for env values that change between plan and
  execute.

The executor's `ctx.Data` TODO bridge is deleted when
`EnvironmentValue` is implemented.

## Blast radius

- `pkg/op` — new `Slot` type, sealed `SlotValue` interface, `Node.Slots`
  shape, `Method` dispatch, `ReceiverType` converter contract,
  `ReceiverRegistry` primitive registration, `ExecutableUnit`
  interface, `Subgraph.Parameters()`, unified `Execute`, `Graph.Bind` /
  `Node.Bind`, executor, recovery, serialization.
- `pkg/op/bind` — `FillSlot` rewrite, `Planner.dispatch` collapse,
  `fillResourceSlot` delete, `Promise` method signatures tighten.
- `pkg/op/provider/flow` — real `Gather` implementation using unified
  `Execute`.
- `pkg/op/provider/*` — every provider's `Do` boilerplate deleted;
  generated code regenerated; hand-written methods unchanged.
- Codegen — templates stop emitting `Do(ctx, slots map[string]any)`.
- Tests — any `slot.Immediate.(T)` cast replaced with typed Slot
  accessors; nodes bound to methods before `SetSlot*` calls.

## Dependencies

- **Prerequisite for Phase 11.** Phase 11's rebind walk operates on the
  new slot model and piggybacks on `(*Node).Bind(method)` introduced
  here.
- **Partially subsumes Phase 10's `Do` work.** Phase 10's
  `ReceiverName` → `Name`, `ProviderType` → `Type`, and
  `executingReceiver` rewiring remain narrow and may land first as a
  separate PR. The `Do` → `Dispatch` rename becomes moot once `Do` is
  deleted from the Action interface.
- **Phase 8 Step 9 failure triage is paused** until this phase lands.
  The 32 failures are expected to resolve or sharpen under the new
  model.

## Related documents

- Parent plan: [extract-starlark-from-op.md](../extract-starlark-from-op.md)
- Architecture: `docs/architecture/4-resource-management.md` §6.8
  "Output Specs", §6.9 "Comparison to Bazel Declared Outputs"