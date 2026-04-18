---
title: "Phase 8: Plan-time scope and grouping combinators"
parent: "docs/plans/extract-starlark-from-op.md"
issue: TBD
status: design
created: 2026-04-17
updated: 2026-04-17
---

## Implementation status

| Step | Status | Notes |
|---|---|---|
| 1. Handle registry + default labeling | not-started | bind.HandleRegistry on StarlarkRuntime; ordered list + byLabel map; `<provider>.<method>#<N>` default labels. |
| 2. bind.ExecutableUnit handle struct | not-started | wraps op.ExecutableUnit + Promise; every plan.* call returns one. |
| 3. Planner.FillSlot dispatch on target type | not-started | if slot expects op.ExecutableUnit → pull handle.ExecutableUnit; else → pull handle.Promise and create edge. |
| 4. plan.workflow primitive | not-started | absorbs old Phase 11; accepts handles; container output = list of terminals. |
| 5. plan.flow.choose redesign | not-started | Case{When, Then any}; CompensateChoose companion; detached-by-default branches. |
| 6. plan.flow.gather redesign | not-started | body=handle; Go side from Phase 7 step 10 stays. |
| 7. plan.flow.wait_until redesign | not-started | predicate=handle; timeout errors through normal Action.Do error channel. |
| 8. plan.run explicit entry point | not-started | .star files end with `plan.run(root_handle)`; no implicit top-level graph. |
| 9. Orphan detection at plan-end | not-started | walk from root, mark reachable, error on unreached handles in the registry. |
| 10. CanConvert method + CanConvertTypes | not-started | type-only pre-flight conversion check; adds CanConvert to op.Converter. |
| 11. Plan-time type-check pass | not-started | walk Promise→slot bindings post-construction; reject at plan time via CanConvertTypes. |
| 12. Migration of existing .star callers | not-started | test_is_*.star files, codegen templates, doc snippets; switch from old Choose/Gather forms to the handle-passing form. |
| 13. Test triage | not-started | |

**Status:** design. No steps started. Design decisions below are resolved from the
phase-8 discussion; step details get refined as each lands.

**Invariants** (record as project memory when landed):
- **I1** — Plan-time type checking: every Promise→slot binding validates via
  `CanConvertTypes` before execution begins. Ill-typed bindings fail at plan,
  not at execute.
- **I2** — No hidden mutable planning state: every `plan.*` call is a pure
  function from args to a `*bind.ExecutableUnit`. The handle registry is
  write-once; graph state lives on registered handles.
- **I3** — Handle registry is write-once: handles register at construction
  and are immutable thereafter. Orphan detection and type-checking run
  against a frozen registry at plan-end.

# Phase 8: Plan-time scope and grouping combinators

## Summary

Every `plan.*` call returns a handle (`*bind.ExecutableUnit`) — it does not
attach anything to any graph. Handles are detached by default. Explicit
combinator calls (`plan.workflow`, `plan.flow.choose`, `plan.flow.gather`,
`plan.flow.wait_until`) bundle handles into containers. A `plan.run(root)`
call at the end of each `.star` file names the root — anything not in the
root's transitive closure is an orphan and errors at plan time.

A handle carries both representations needed at every binding site: the
`op.ExecutableUnit` (for slots that want an executable reference — combinator
bodies, branches, iteration targets) and a `Promise` (for slots that want a
value — consumes the handle's output via an edge). The binding layer
(`Planner.FillSlot`) picks which field to use based on the target slot's
type. Starlark authors don't distinguish — handles are polymorphic at the
call site.

Phase 8 absorbs what was formerly Phase 11 ("Implement `plan.subgraph` as a
Flow Provider Method"). `plan.workflow` is the general form; the old
single-case Phase 11 proposal is one usage of it.

## Problem

Strict-eval starlark evaluates inner expressions before outer ones. Under
the current model:

```python
plan.flow.choose(
    defaultValue=plan.file.write_text(path, "default"),
    case(when=..., then=plan.file.remove(path)),
)
```

Both `plan.file.write_text(...)` and `plan.file.remove(...)` evaluate
before `plan.flow.choose` runs. They attach to the enclosing subgraph as
children — and run unconditionally at execution time. The "choose one
branch" semantic is broken before it starts.

The problem generalizes across every grouping combinator. Without an
explicit deferral mechanism, any nested `plan.*` call attaches to the
wrong scope.

**Two alternatives considered and rejected:**

1. **Plan-time lambdas + scope stack.** The planner maintains a scope stack,
   combinators accept `lambda: …` expressions, evaluating them pushes a
   scope, and nested `plan.*` calls attach to the pushed scope. Rejected —
   the scope stack is ambient mutable state at plan time, violating
   invariant I2. Lambdas also add syntax cost at every combinator arg.
2. **Explicit `plan.detach(plan.file.write_text(...))` wrappers.** Forces
   every arg to be wrapped. Rejected on ergonomics and failure mode
   (forgetting the wrapper silently attaches to the wrong scope).

The adopted approach — handles detached by default, explicit attachment via
`plan.workflow` / combinators — eliminates both the ambient scope stack and
the wrapper burden. Every `plan.*` call is a pure function that produces a
handle; nothing attaches until the caller says so.

## Goal

- Authors write combinator calls with handle-passing syntax; no lambdas
  required for attachment.
- Containers (workflow, choose branches, gather body, wait_until predicate)
  explicitly own their members, receiving handles as args.
- Anything the author constructs but doesn't attach fails at plan time as
  an orphan — silent dead code is not tolerated.
- Type mismatches on Promise→slot bindings fail at plan time — runtime
  coercion errors are caught by a pre-flight pass.

Representative shapes:

```python
# Subgraph: bundle N handles into one executable unit.
setup = plan.workflow(
    plan.file.mkdir(path=dir),
    plan.file.write_text(destination=dir + "/hello", content="hi"),
)

# Choose: branches are handles; detached until the matching case fires.
plan.flow.choose(
    defaultValue=plan.flow.complete(),
    case(when=plan.service.is_healthy(svc="db"),
         then=plan.flow.complete(output="ok")),
    case(when=plan.service.is_down(svc="db"),
         then=plan.flow.degraded("{{.svc}} unhealthy", svc="db")),
)

# Gather: body is a handle parameterized by an iteration input.
body = plan.workflow(plan.file.write_text(destination=_item, content="…"))
plan.flow.gather(items=paths, body=body)

# WaitUntil: predicate is a handle.
plan.flow.wait_until(
    predicate=plan.service.is_healthy(svc="db"),
    timeout="5m",
    interval="10s",
)

# Entry point: explicit root.
plan.run(plan.workflow(setup, ...))
```

## Design decisions

### D1 — Handle shape

```go
package bind

// ExecutableUnit is the handle returned by every plan.* call. It carries
// both the op-level unit (for executable-reference slots) and a Promise
// (for value-consuming slots). FillSlot picks which field to use based on
// the target parameter's type.
type ExecutableUnit struct {
    ExecutableUnit op.ExecutableUnit // the Node or Subgraph this handle names
    Promise        *Promise          // value-side handle: edge source for the handle's output
}
```

For node handles, `ExecutableUnit` is a `*op.Node` and `Promise` points at
that node's output. For container handles (workflow, choose, gather,
wait_until), `ExecutableUnit` is the container's subgraph (or the
combinator node itself, per D3) and `Promise` points at the container's
defined output.

Handles are created by `plan.*` dispatch methods, registered in the
session's `HandleRegistry` (D6), and returned as the starlark value the
caller sees.

### D2 — Argument binding: target-type dispatch

`Planner.FillSlot` gains a case for `*bind.ExecutableUnit`:

```
When slot.Parameter.Type implements op.ExecutableUnit (or is assignable to it):
    slot.Value = ImmediateValue{handle.ExecutableUnit}
    No edge — the caller wanted a unit reference.

Else (target expects a value):
    edge from handle.Promise.node → consumer node
    slot.Value = PromiseValue{NodeRef: handle.Promise.node.ID(), Slot: handle.Promise.slot}
    Same behavior as today's *Promise case, but sourced from handle.Promise.
```

Starlark callers never distinguish "pass a unit" from "pass a value" — the
receiving method's Go parameter type determines the semantic.

### D3 — Container output conventions

Every container has a defined output. The container handle's `Promise`
points at whatever produces that output at execute time.

| Container | Output | Terminal |
|---|---|---|
| `plan.workflow(a, b, c)` | `[]any` (always a list) | topological terminals; multiple terminals → list of each |
| `plan.flow.gather(items, body)` | `[]any` in item order | the gather node itself produces the list |
| `plan.flow.choose(default, cases...)` | chosen branch's value | the choose node itself |
| `plan.flow.wait_until(predicate, ...)` | predicate's final value | the wait_until node itself; timeout surfaces as error through Action.Do's error channel |

**Workflow is always list-typed** to keep the rule predictable — one
terminal returns a one-element list; many terminals return a multi-element
list. Authors destructure or index when they want a scalar. Gather's shape
is the precedent.

### D4 — Orphan detection

At plan-end (after all starlark evaluation completes, before execution
begins), walk the graph from the handle passed to `plan.run(...)`. Mark
every reachable unit:

- Direct children of any reached container are reached.
- Edge endpoints on reached nodes are reached (transitively).

Any handle in the session's `HandleRegistry` that is not reached is an
**orphan**. Plan-end errors with a list of orphan labels.

Rationale: silent dead code is the worst failure mode — the author
believes their handle is in the graph but it isn't. Explicit discard
remains available via starlark's blank identifier: `_ = plan.file.write_text(...)`.
The underscore-bound handle still registers; orphan detection still finds
it; but the intent is explicit and the error message points at the `_ =`
site with a suggestion to either attach or remove the call.

### D5 — Explicit root via `plan.run(root)`

There is no implicit top-level graph. Every `.star` file ends with a call
that names the root:

```python
plan.run(plan.workflow(a, b, c))
```

The tool-level runner picks up the root handle and drives execution.
`plan.run` is not invocable more than once per script; a second call is a
plan-time error.

This makes orphan detection meaningful (without a root, "orphan" isn't
definable) and forces authors to explicitly compose the top-level graph
rather than relying on implicit accumulation.

### D6 — Handle registry

```go
package bind

type HandleRegistry struct {
    mu      sync.Mutex
    ordered []*ExecutableUnit          // creation order; used for deterministic iteration
    byLabel map[string]*ExecutableUnit // label → handle; used for lookup and orphan reporting
}

// Register appends to ordered and inserts into byLabel. Duplicate labels
// (user-supplied collisions) are plan-time errors.
func (r *HandleRegistry) Register(label string, h *ExecutableUnit) error

// All returns handles in creation order. Used by the plan-end orphan pass
// and the type-check pass.
func (r *HandleRegistry) All() []*ExecutableUnit

// ByLabel returns a handle by its label or nil if not registered.
func (r *HandleRegistry) ByLabel(label string) *ExecutableUnit
```

Owned by `bind.StarlarkRuntime` (the per-session runtime). Every
`Planner.dispatch` call registers its constructed handle before returning
it to the starlark caller.

Writes happen only during planning. Reads happen during planning (orphan
walk, type-check walk) and at execute time (if lookup by label is ever
needed — probably not, but the data is available).

### D7 — Default labeling

Format: `<provider>.<method>#<N>` where `N` is the creation-order ordinal
within that provider.method combination.

Examples:

```
file.write_text#1
file.write_text#2
file.mkdir#1
flow.choose#1
service.is_healthy#1
```

Derivation: the planner knows the receiver type and method name at the
point of handle construction. A per-method counter in the registry yields
the ordinal.

**User override** via a reserved kwarg `_label="name"` on any `plan.*`
call, or via `plan.label("name", handle)` applied after construction.
Collisions against prior labels (user-supplied or auto) are plan-time
errors.

Alternatives rejected:
- Monotonic global (`unit-1`, `unit-2`): opaque; no hint about what the
  handle is.
- Source-position-based (`file.write_text@manifest.star:42`): fragile
  under refactors.
- Content-hash labels: deterministic-by-args, enables caching, but
  unreadable and overkill for the current scope.

### D8 — Plan-time type checking

Every Promise→slot binding carries a type relationship: the slot's
parameter type (target) must accept the Promise's source-node output type
(source). `op.Convert` performs the runtime cascade; plan-time checking
answers "could Convert succeed?" without a value.

New function `op.CanConvertTypes(ctx, source, target reflect.Type) bool`
implements the cascade at the type level:

1. `source == target` → yes.
2. `source.AssignableTo(target)` → yes.
3. `source.Implements(converterType)` → yes (optimistic; see D9 for the
   limitation).
4. `ctx.Registry` has a `ResourceReceiverType` for `target` (or
   `target.Elem()` when pointer) → yes.
5. Both `source` and `target` are slices → recurse on element types.
6. Otherwise → no.

At plan-end, after all handles are registered and before execution begins,
walk every slot in the graph. For each `PromiseValue` slot:

```
source = slot's Promise source node's output type (method return type).
target = slot.Parameter.Type.
If !CanConvertTypes(ctx, source, target):
    error: "cannot bind <source-label> output to <consumer-label> slot %s
           (have %s, want %s)", slot.Name, source, target
```

Rejected at plan-end. No ill-typed edges reach execution.

### D9 — `CanConvert` method on `op.Converter`

The `Converter` interface (Phase 7 step 8) gains a type-level predicate:

```go
package op

type Converter interface {
    Convert(target reflect.Type) (any, error)
    CanConvert(target reflect.Type) bool
}
```

**Used at runtime by `op.Convert`** as a lookahead: before calling
`c.Convert(target)` (which may have side effects or allocate), check
`c.CanConvert(target)` to avoid unnecessary attempts.

**Not directly used at plan time** — plan-time checks have no value
instance, only types. The optimistic approximation in step 3 of
`CanConvertTypes` ("if source implements Converter, trust it") is the
plan-time compromise. Converter implementations that claim the interface
but would refuse a specific target still pass plan-time and fail at
execute time — with a specific error message pointing at the exact
binding.

If fully-hermetic plan-time checking becomes important, a future variant
could require `Converter` implementations to register a type-level
static manifest (`CanConvertStatic(source, target reflect.Type) bool` as
a package-level function paired with the type). Not planned for Phase 8.

### D10 — Empty workflow

`plan.workflow()` with no arguments is a plan-time error at the call site.

Rationale:
- An empty container has no work and no output; downstream consumers of
  its handle have nothing meaningful to bind.
- Authors who want conditional contents build the arg list in starlark:
  `plan.workflow(*([a, b] + ([c] if cond else [])))`.
- A mutable builder pattern (`plan.workflow_builder()` → `b.add(...)` →
  `b.done()`) is not adopted; it conflicts with the functional,
  pure-plan-time model (invariant I2).

### D11 — Migration of existing `.star` callers

Existing callers of the old Choose/Gather APIs migrate to the handle form:

- `cmd/devlore-test/devloretest/data/test_is_*.star` — rewrite from
  `plan.choose(when=..., then=...)` kwargs form to the case-passing form.
- `pkg/op/provider/flow/gen/action.gen_test.go` — regenerates against
  the new provider shape after codegen templates are updated.
- Any `.star` doc snippets showing Choose/Gather call sites — update in
  place.
- Codegen templates that emit planner bridge code — updated to produce
  handle returns from every `plan.*` call.

Each step that lands a combinator redesign includes its migration as part
of that step's PR.

## Invariants

### I1 — Plan-time type checking

Every Promise→slot binding is validated at plan-end via `CanConvertTypes`.
Ill-typed bindings fail at plan time with a message naming the source
label, the consumer label, and the expected vs. actual types. Runtime
conversion errors remain possible only for Converter-implementing source
types that refuse a specific target — rare, and still surface with enough
context to diagnose.

### I2 — No hidden mutable planning state

Every `plan.*` call is a pure function from its starlark arguments to a
`*bind.ExecutableUnit`. The only mutable state during planning is the
`HandleRegistry`, which is append-only until planning completes. Authors
can reorder, refactor, or extract helper functions without changing graph
semantics (beyond what the refactoring itself expresses).

### I3 — Handle registry is write-once

After `plan.run(...)` is called, the registry is frozen. Orphan detection
and type-checking read from the frozen registry. Execution operates on
the graph reachable from the root handle; the registry's presence is
incidental at execute time (available if needed for label lookup, but no
longer written).

## Updated step outline

1. **Handle registry + default labeling.** `bind.HandleRegistry` on
   `StarlarkRuntime`; `ordered` + `byLabel`; `<provider>.<method>#<N>`
   default labels; user override via `_label=` kwarg and `plan.label`.
2. **`bind.ExecutableUnit` handle struct.** Wraps `op.ExecutableUnit` +
   `*Promise`. Implements `starlark.Value` so it flows through starlark
   expressions. Every `Planner.dispatch` constructs and returns one.
3. **`Planner.FillSlot` dispatch by target type.** Target implements
   `op.ExecutableUnit` → pull `handle.ExecutableUnit`; else pull
   `handle.Promise` and use the existing Promise/edge logic from Phase 7.
4. **`plan.workflow(handles…)` primitive.** Takes variadic handles, builds
   a subgraph. Output = list of terminal values. Empty workflow errors.
   Absorbs old Phase 11.
5. **`plan.flow.choose` redesign.** `Case{When any, Then any}`; compensable
   method; `CompensateChoose` companion; lazy dispatch of branches via
   `Graph.ExecuteWithStack`.
6. **`plan.flow.gather` redesign.** `body=handle`; existing Go-side Gather
   from Phase 7 step 10 stays; starlark-facing builder changes.
7. **`plan.flow.wait_until` redesign.** `predicate=handle`; timeout surfaces
   as Action.Do error.
8. **`plan.run(root)` explicit entry point.** No implicit top-level graph.
   Tool-runner picks up the root handle.
9. **Orphan detection at plan-end.** Walk from `plan.run`'s root; mark
   reachable; error on unreached registry entries.
10. **`CanConvert` method + `CanConvertTypes` function.** Interface addition
    to `op.Converter`; new package-level `op.CanConvertTypes` implementing
    the type-level cascade.
11. **Plan-time type-check pass.** Walk Promise→slot bindings; apply
    `CanConvertTypes`; reject mismatches.
12. **Migration of existing `.star` callers.** Per D11.
13. **Test triage.** Run the full suite; fold residuals into follow-ups.

## Blast radius

- `pkg/op/context.go`, `pkg/op/convert.go`, `pkg/op/action.go` —
  `CanConvert` interface method, `CanConvertTypes` function.
- `pkg/op/bind/planner.go` — `Planner.dispatch` returns `*ExecutableUnit`;
  `Planner.FillSlot` dispatches by target type.
- `pkg/op/bind/promise.go` — `Promise` may stay or fold into the handle;
  decide during step 2.
- `pkg/op/bind/starlark_runtime.go` — `HandleRegistry` field; plan.run
  wiring.
- `pkg/op/provider/flow/provider.go` — Choose/Gather/WaitUntil redesigns.
- `pkg/op/provider/flow/gen/*` — codegen-produced announce maps; update
  templates.
- `cmd/devlore-test/devloretest/data/test_is_*.star` — migration.
- Any starlark test fixtures using the old Choose/Gather forms.

## Dependencies

- **Follows Phase 7.** Gather's compensation pattern (Phase 7 step 10) and
  ctx threading (Phase 7 step 10) are the templates the new Choose design
  mirrors.
- **Precedes Phase 12.** Some flow-provider defects may only surface or
  become addressable after the handle-based APIs land.

## Post-refactoring discussion topics

These are deferred until the current refactoring completes (Phase 7 through
the end of the planned phases). Raise them then.

### F1 — Named handle lookup (Bazel labels as escape hatch)

Bazel references targets by label: `//pkg:target`. The label registry
exists (D6), so `plan.lookup("file.write_text#3")` or
`plan.lookup("mylabel")` is a trivial addition. Not needed now because
handle-passing covers the typical case, but worth revisiting if scripts
grow large and cross-function handle threading becomes cumbersome.

### F2 — Multi-output providers (Bazel-style Providers)

Bazel rules return lists of typed `Provider` objects; consumers pattern-
match to pull named fields. Our handle currently exposes one `Promise`
(one output). If combinators grow multi-field outputs (e.g., a workflow
returning "primary value" + "diagnostic trace"), a typed provider system
scales better than single-Promise handles. Not needed until a concrete
use case arises.

### F3 — Hermeticity tightening

Bazel's action sandbox enforces that executions see only declared inputs.
Our execution already confines filesystem access via `Root`, but ambient
context access (via `ExecutionContext`) is broader. Tightening would
require every provider method to declare its inputs/outputs explicitly,
with the executor enforcing the boundary. Aligns with the existing
design goal of full plan-time hermeticity; extension to execute-time
remains an aspiration.

## Related documents

- Parent plan: [extract-starlark-from-op.md](../extract-starlark-from-op.md)
- Phase 7 plan: [phase-7.md](phase-7.md)
- Architecture:
  - `docs/architecture/4-resource-management.md` §6 — catalog + reconciliation
  - Dependency-analysis prototype notes — (to add pointer when located)
