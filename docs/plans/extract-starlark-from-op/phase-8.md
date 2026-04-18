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
| 1. Planner scope stack | not-started | |
| 2. `plan.subgraph(lambda)` primitive | not-started | |
| 3. `plan.flow.choose` redesign | not-started | |
| 4. `plan.flow.gather` redesign | not-started | existing Go-side gather (Phase 7 step 10) stays; starlark-facing builder changes |
| 5. `plan.flow.wait_until` redesign | not-started | |
| 6. Detached subgraph representation | not-started | may land with step 1 |
| 7. Migration of existing `.star` callers | not-started | codegen / bootstrap templates, test-data .star files |
| 8. Test triage | not-started | |

**Status:** design. No steps started. This plan doc is a stub holding the problem
statement, the goal, and the open design decisions. Full step outlines, code
shapes, and test strategies get written once the design decisions are resolved
with the project owner.

# Phase 8: Plan-time scope and grouping combinators

## Summary

Grouping combinators — `plan.subgraph`, `plan.flow.choose`, `plan.flow.gather`,
`plan.flow.wait_until` — define plan-time scopes. `plan.*` calls authored
lexically inside a combinator's scope attach to a subgraph owned by that
combinator, not to the enclosing top-level flow. The combinator's runtime logic
decides when (or whether) the contained work runs.

The mechanism is plan-time lambdas. Combinators accept `lambda:` expressions at
their scope-defining positions. The planner evaluates those lambdas during
planning with a scope stack that redirects nested `plan.*` calls into the
combinator's owned subgraph. Execute-time lambdas remain forbidden — the graph
is immutable after planning.

This phase absorbs what was formerly Phase 11 ("Implement `plan.subgraph` as a
Flow Provider Method"). That work was a special case of this general problem.

## Problem

Starlark is strict-eval. A caller writing:

```python
plan.flow.choose(
    defaultValue=plan.file.write_text(destination=fallback, content="…"),
    case(when=lambda: plan.service.is_healthy(svc="db"),
         then=plan.flow.complete(output="ok")),
    case(when=lambda: plan.service.is_down(svc="db"),
         then=plan.flow.degraded("{{.svc}} unhealthy", svc="db")),
)
```

expects the fallback write to happen only when no case matches. Today:

1. `plan.file.write_text(...)` evaluates first — adds a node to the surrounding
   subgraph's children. That node runs unconditionally during the top-level
   graph walk.
2. Every other `plan.*` call in the arg list does the same. All bodies run.
3. `plan.flow.choose(...)` receives their return values (Promises) and wires
   its own node.
4. At execute time, Choose picks one Promise to surface — but all the work has
   already happened.

Side effects leak out of the intended branch. Gather has a similar problem for
its iteration body; WaitUntil for its predicate body; explicit subgraphs don't
have a primitive constructor at all.

The root cause is the absence of **lexical plan-time scoping**: no mechanism
for a combinator to say "these nested authoring calls belong to MY scope, not
the enclosing one."

## Goal

Introduce plan-time lexical scoping via plan-time lambdas and a planner scope
stack. Every grouping combinator owns one or more subgraphs; nested `plan.*`
calls attach to the innermost enclosing combinator-owned scope.

Representative shapes:

```python
plan.subgraph(lambda: (
    plan.file.mkdir(path=dir),
    plan.file.write_text(destination=dir + "/hello", content="hi"),
))

plan.flow.choose(
    defaultValue=lambda: plan.flow.complete(),
    case(when=lambda: plan.service.is_healthy(svc="db"),
         then=lambda: plan.flow.complete(output="ok")),
    case(when=lambda: plan.service.is_down(svc="db"),
         then=lambda: plan.flow.degraded("{{.svc}} unhealthy", svc="db")),
)

plan.flow.gather(items=paths, body=lambda path:
    plan.file.write_text(destination=path, content="…"))

plan.flow.wait_until(
    predicate=lambda: plan.service.is_healthy(svc="db"),
    timeout="5m",
    interval="10s",
)
```

In every case: lambdas run during planning, not execution. The planner pushes
a scope, calls the lambda, and pops. Nested `plan.*` calls attach to the
pushed scope's subgraph.

## Design decisions to resolve

### D1 — Planner scope stack

The planner maintains a stack of "current subgraph" references. Every
`plan.*` call attaches its node to the stack top. Questions to settle:

- Shape: is the stack a field on the planner, a field on `ExecutionContext`,
  or a per-thread starlark value?
- Push/pop discipline: always paired, safe against panics inside lambdas.
- Access from `Planner.dispatch`: how the attachment read flows through the
  existing dispatch path without invasively threading a parameter.

### D2 — Plan-time lambdas as the deferral mechanism

Alternatives to consider and reject with rationale in the final doc:

- Builder callbacks: `plan.subgraph(lambda sg: sg.file.write_text(...))` —
  requires per-namespace shadowing; rejected on ergonomics.
- Explicit `plan.detach(plan.file.write_text(...))` wrappers at every
  authoring site — rejected on ergonomics and failure mode (forgetting the
  wrapper silently attaches to the wrong scope).
- Code blocks / multi-statement expressions — not available in starlark.

### D3 — Detached subgraphs

Combinator-owned subgraphs are not wired into the enclosing subgraph's
children. Questions:

- Representation: do they live under a combinator node (e.g., as a hidden
  `OwnedSubgraphs []*Subgraph` field), or in a flat detached region of the
  graph indexed by the combinator?
- Top-level execution exclusion: `executeChildren` must not dispatch them.
  The current implementation walks `sg.Children`; detached subgraphs aren't
  there, so this is automatic — if the representation holds.
- Promise referencing: a Promise to a detached subgraph's terminal value
  needs to trigger lazy dispatch when resolved. Today `PromiseValue.Resolve`
  does a `results[NodeRef]` lookup. For detached subgraphs, the lookup
  would fail (they haven't run); the combinator, not the Promise, is
  responsible for driving dispatch.

### D4 — `plan.subgraph(lambda: …)` primitive

Absorbs old Phase 11. Builder shape:

```python
handle = plan.subgraph(lambda: (
    plan.file.mkdir(path),
    plan.file.write_text(...),
))
```

`handle` is a Promise/reference usable anywhere an executable unit is
accepted (e.g., inside a Choose case's `then=`). Design questions:

- Return value shape: Promise to subgraph, or a distinct "subgraph handle"
  type?
- Whether tuple-return from the lambda is the aggregation mechanism, or
  whether the scope stack collects everything authored inside automatically.
- Naming: `plan.subgraph` vs. `plan.flow.subgraph` (routed through planner
  namespace) — tie-breaker with D5/D6 naming.

### D5 — `plan.flow.choose` API

```go
// Go-side
type Case struct {
    When any // bool literal or Promise yielding bool
    Then any // any literal or Promise yielding the branch's value
}

func (p *Provider) Choose(
    ctx context.Context,
    defaultValue any,
    cases ...Case,
) (any, op.Complement, error)

func (p *Provider) CompensateChoose(stack *op.RecoveryStack) error
```

Runtime semantics:

1. Iterate cases in order. For each case resolve `When`.
2. On first true, resolve `Then` (dispatching any detached subgraph lazily),
   return its value with the Then's stack as complement.
3. Whens for later cases are not evaluated.
4. If no case matches, resolve `defaultValue` and return its value (the
   defaultValue's work is lazy — its referenced subgraph dispatches only
   here).

Compensation: complement = single chosen-branch stack (Then or default
stack). `CompensateChoose` unwinds the one stack.

Starlark-side, each `when=` / `then=` / `defaultValue=` accepts a plan-time
lambda producing a detached subgraph handle.

### D6 — `plan.flow.gather` API

Go-side `flow.Provider.Gather` stays as-is (Phase 7 step 10). The starlark-
facing builder changes:

- Current: `plan.flow.gather(items=xs, do="subgraph-id", limit=N)` — user
  pre-declares the body subgraph by ID.
- Proposed: `plan.flow.gather(items=xs, body=lambda item: …, limit=N)` —
  the lambda is plan-time; the planner calls it once with a placeholder
  item-slot to build the body subgraph, then wires items→slot for each
  iteration.

The Go receiver signature for Gather doesn't change — it still accepts a
body reference and dispatches via `ExecuteWithStack`. The planner-side
builder for `plan.flow.gather` is what changes.

### D7 — `plan.flow.wait_until` API

```python
plan.flow.wait_until(
    predicate=lambda: plan.service.is_healthy(svc="db"),
    timeout="5m",
    interval="10s",
)
```

Semantics: poll `predicate` at the interval until true or the timeout
expires. On timeout, fail. The predicate subgraph is combinator-owned —
dispatched by WaitUntil at each poll. On success, returns the predicate's
result (or nil; TBD).

### D8 — Promise semantics for combinator scopes

Combinator-owned subgraphs produce Promises that resolve differently from
top-level-attached Promises:

- **Top-level-attached**: `PromiseValue.Resolve` reads `results[NodeRef]`
  (the node already ran).
- **Combinator-owned**: must trigger lazy dispatch on resolve. Two models:
  - Combinator calls `ResolveExecutable` + `ExecuteWithStack` directly,
    bypassing Promise.Resolve.
  - `PromiseValue` grows a variant / flag that signals "this is a
    detached handle; the owner is responsible for dispatching it."

Picking a model determines whether Promise code paths branch on attachment
or the combinator runtime handles it without Promise involvement.

### D9 — Migration

- `cmd/devlore-test/devloretest/data/test_is_*.star` — uses old
  `plan.choose(when=…, then=…)` kwargs form; migrate to the new lambda-
  based case builder.
- `pkg/op/provider/flow/gen/action.gen_test.go` — generated; will
  regenerate against the new provider shape.
- Any other .star scripts under `star/`, `cmd/star/`, `docs/` using Choose
  or Gather in the old form — identify and migrate.

## Blast radius

- `pkg/op/planner.go` (or wherever the planner scope stack lives) — new
  scope-stack infrastructure.
- `pkg/op/provider/plan/` — new `plan.subgraph` method; changed `plan.flow.*`
  method surfaces.
- `pkg/op/provider/flow/provider.go` — `Choose` rewrite;
  `Gather` signature mostly stays (Phase 7 step 10); `WaitUntil` may
  change.
- Codegen templates — emit the new lambda-taking builders.
- `.star` test data — migration.

## Dependencies

- **Follows Phase 7** — gather's compensation pattern and ctx threading
  from Phase 7 step 10 are the templates.
- **Precedes Phase 12** (flow-provider defects) — some defects may only
  surface or be addressable after the new combinator APIs land.

## Related documents

- Parent plan: [extract-starlark-from-op.md](../extract-starlark-from-op.md)
- Phase 7 plan: [phase-7.md](phase-7.md)
