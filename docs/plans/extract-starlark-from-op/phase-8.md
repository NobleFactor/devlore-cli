---
title: "Phase 8: Plan-time scope and grouping combinators"
parent: "docs/plans/extract-starlark-from-op.md"
issue: 275
status: design
created: 2026-04-17
updated: 2026-04-17
---

## Implementation status

| Step | Status | Notes |
|---|---|---|
| 1. Invocation registry + options mechanism | not-started | bind.InvocationRegistry on StarlarkRuntime; ordered list + byLabel map; `<provider>.<method>#<N>` default labels; `options` kwarg + `plan.options(...)` builder. |
| 2. bind.Invocation struct | not-started | Target (op.ExecutableUnit) + Result (*Promise); every plan.* call returns one. |
| 3. Planner.FillSlot dispatch on target type | not-started | if slot expects op.ExecutableUnit → pull invocation.Target; else → pull invocation.Result and create edge. |
| 4. Merge flow provider into plan provider | not-started | Move all flow methods (choose, gather, wait_until, complete, degraded, fatal, elevate) to the plan provider; delete flow package. Surfaces flat on starlark as `plan.X`. Redesigns come in later steps. |
| 5. plan.subgraph primitive | not-started | New on plan provider; accepts variadic invocations; container output = list of terminal values (per D3). Absorbs old Phase 11. |
| 6. plan.choose redesign | not-started | Case{When, Then any}; CompensateChoose companion; detached-by-default branches; container-output-type inference per D3. |
| 7. plan.gather redesign | not-started | body is an invocation; Go side from Phase 7 step 10 stays; container-output-type inference per D3. |
| 8. plan.wait_until redesign | not-started | predicate is an invocation; timeout errors through normal Action.Do error channel; output = predicate's return type. |
| 9. plan.run explicit entry point | not-started | .star files end with `plan.run(...)`; no implicit top-level graph; owns pre-flight with error aggregation. |
| 10. Orphan detection at plan-end | not-started | walk from root, mark reachable, collect unreached invocations as errors. |
| 11. CanConvert on Converter + Planner.CanConvertTypes | not-started | type-only pre-flight conversion check; adds required CanConvert to op.Converter; method on Planner implements the cascade. |
| 12. Topological sort + plan-time type-check pass | not-started | order producer-before-consumer, walk Promise→slot bindings, apply Planner.CanConvertTypes, collect mismatches as errors. |
| 13. Migration of existing .star callers | not-started | test_is_*.star files and doc snippets; switch from old Choose/Gather forms to the invocation-passing form. |
| 14. Test triage | not-started | |

**Status:** design. No steps started. Design decisions below are resolved from the
phase-8 discussion; step details get refined as each lands. Invariants I1–I3
(see full section) should be recorded as project memory when the phase lands.

# Phase 8: Plan-time scope and grouping combinators

## Summary

Every `plan.*` call returns an invocation (`*bind.Invocation`) — it does
not attach anything to any graph. Invocations are detached by default.
Explicit combinator calls (`plan.subgraph`, `plan.choose`,
`plan.gather`, `plan.wait_until`) bundle invocations into
containers. A `plan.run(...)` call at the end of each `.star` file names
the root — anything not in the root's transitive closure is an orphan
and errors at plan time.

An invocation carries both representations needed at every binding site:
the `op.ExecutableUnit` (for slots that want an executable reference —
combinator bodies, branches, iteration targets) and a `Promise` (for
slots that want a value — consumes the invocation's output via an edge).
The binding layer (`Planner.FillSlot`) picks which field to use based on
the target slot's type. Starlark authors don't distinguish — invocations
are polymorphic at the call site. The binding layer handles the dispatch
transparently.

Phase 8 absorbs what was formerly Phase 11 ("Implement `plan.subgraph` as a
Flow Provider Method"). `plan.subgraph` is the general form; the old
single-case Phase 11 proposal is one usage of it.

## Problem

Strict-eval starlark evaluates inner expressions before outer ones. Under
the current model:

```python
plan.choose(
    defaultValue=plan.file.write_text(path, "default"),
    case(when=..., then=plan.file.remove(path)),
)
```

Both `plan.file.write_text(...)` and `plan.file.remove(...)` evaluate
before `plan.choose` runs. They attach to the enclosing subgraph as
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

The adopted approach — invocations detached by default, explicit
attachment via `plan.subgraph` / combinators — eliminates both the ambient
scope stack and the wrapper burden. Every `plan.*` call is a pure function
that produces an invocation; nothing attaches until the caller says so.

Prior-art lesson: `op.ExecutionContext` embeds `context.Context` as a single
shared value, which broke scoped cancellation when gather needed its own
cancel scope (see Phase 7 step 10). The fix threaded `context.Context` as a
parameter through the dispatch chain so each scope could derive its own
child. The same principle applies to plan-time scope: centralizing "the
current enclosing subgraph" in ambient state (the rejected scope stack)
invites the same class of bug. Every scope has to be a value that callers
pass explicitly — for cancellation, a `context.Context`; for planning, an
invocation.

## Goal

- Authors write combinator calls with invocation-passing syntax; no
  lambdas required for attachment.
- Containers (subgraph, choose branches, gather body, wait_until predicate)
  explicitly own their members, receiving invocations as args.
- Anything the author constructs but doesn't attach fails at plan time as
  an orphan — silent dead code is not tolerated.
- Type mismatches on Promise→slot bindings fail at plan time — runtime
  coercion errors are caught by a pre-flight pass.

Representative shapes:

```python
# Subgraph: bundle N invocations into one executable unit.
setup = plan.subgraph(
    plan.file.mkdir(path=dir),
    plan.file.write_text(destination=dir + "/hello", content="hi"),
)

# Choose: branches are invocations; detached until the matching case fires.
plan.choose(
    defaultValue=plan.complete(),
    plan.case(when=plan.service.is_healthy(svc="db"),
                   then=plan.complete(output="ok")),
    plan.case(when=plan.service.is_down(svc="db"),
                   then=plan.degraded("{{.svc}} unhealthy", svc="db")),
)

# Gather: body is an invocation parameterized by an iteration input.
paths = ["/tmp/log/a.txt", "/tmp/log/b.txt", "/tmp/log/c.txt"]
body = plan.subgraph(plan.file.write_text(destination=_item, content="hello"))
plan.gather(items=paths, body=body)

# WaitUntil: predicate is an invocation.
plan.wait_until(
    predicate=plan.service.is_healthy(svc="db"),
    timeout="5m",
    interval="10s",
)

# Entry point: explicit root.
plan.run(plan.subgraph(setup, ...))
```

## Design decisions

### D1 — Invocation shape

```go
package bind

// Invocation is the value returned by every plan.* call. It represents
// a planned provider-method invocation that has not yet executed. Target
// is the op-level unit the invocation will dispatch; Result is the Promise
// to its output. FillSlot picks which field to use based on the target
// parameter's type at the binding site.
type Invocation struct {
    Target op.ExecutableUnit // the Node or Subgraph this invocation will dispatch
    Result *Promise          // value-side accessor: edge source for the invocation's output
}
```

For node invocations, `Target` is a `*op.Node` and `Result` points at
that node's output. For container invocations (subgraph, choose, gather,
wait_until), `Target` is the container's subgraph (or the combinator node
itself, per D3) and `Result` points at the container's defined output.

Invocations are created by `plan.*` dispatch methods, registered in the
session's `InvocationRegistry` (D6), and returned as the starlark value the
caller sees.

### D2 — Argument binding: target-type dispatch

`Planner.FillSlot` gains a case for `*bind.Invocation`:

```
When slot.Parameter.Type implements op.ExecutableUnit (or is assignable to it):
    slot.Value = ImmediateValue{invocation.Target}
    No edge — the caller wanted a unit reference.

Else (target expects a value):
    edge from invocation.Result.node → consumer node
    slot.Value = PromiseValue{NodeRef: invocation.Result.node.ID(), Slot: invocation.Result.slot}
    Same behavior as today's *Promise case, but sourced from invocation.Result.
```

Starlark callers never distinguish "pass a unit" from "pass a value" — the
receiving method's Go parameter type determines the semantic.

In full detail, this replaces the existing `*Promise` case in `FillSlot` —
a Promise is now always carried inside an `Invocation`, so the old case
disappears.

### D3 — Container output conventions

Every container has a defined output. The container invocation's `Result`
points at whatever produces that output at execute time. Output type is
inferred from member types when the members are homogeneous; falls back
to `any` when heterogeneous.

| Container | Output value | Output type |
|---|---|---|
| `plan.subgraph(a, b, c)` | list of terminal values in topological order | `[]T` when all terminals return `T`; `[]any` otherwise |
| `plan.gather(items, body)` | list of per-iteration results in item order | `[]T` when body returns `T` (every iteration produces the same type by construction); `[]any` when body's return is `any` |
| `plan.choose(default, cases...)` | value of the chosen branch | `T` when default and every case's Then return `T`; `any` otherwise |
| `plan.wait_until(predicate, ...)` | predicate's final value | the predicate's return type; timeout surfaces as error through Action.Do's error channel |

**Rationale.** Binding a container invocation's `Result` to a consumer's
slot requires type compatibility. Inferring the narrowest accurate output
type maximizes what can be bound cleanly and what plan-time type
verification (D8) can catch. A heterogeneous subgraph — e.g., terminals
returning `string` and `int` — is legal but its output is `[]any`; the
consumer must either accept `[]any` or the plan-time type check rejects
the binding.

**Subgraph + gather are always list-typed.** Even with one terminal or
one iteration, the output is a one-element list. Authors destructure or
index when they want the scalar. Keeps the rule predictable and the
type-inference logic uniform.

**Choose's inferred type.** Homogeneous cases produce a narrow type;
heterogeneous (including the default) fall back to `any`. The narrowing
happens at the planner by inspecting every branch's return type.

**Type-check implications.** D8's type verification uses these inferred
types as the SOURCE side of each binding that consumes a container's
`Result`. A subgraph of `[]string` bound to a slot expecting `[]string`
passes; bound to `[]int` fails; bound to `[]any` passes via
assignability.

### D4 — Orphan detection

At plan-end (after all starlark evaluation completes, before execution
begins), walk the graph from the invocation passed to `plan.run(...)`.
Mark every reachable invocation by applying these rules until fixed-point:

- The root invocation is reached.
- If a container invocation is reached, every invocation that appears as
  a child of its container is reached.
- If an invocation is reached, every edge incident on its Target has both
  endpoints reached — specifically, any invocation whose `Result` is
  consumed by a value-typed slot on a reached invocation is itself
  reached (the source must run to produce the value the consumer needs).

Any invocation in the session's `InvocationRegistry` that is not reached
is an **orphan**. Each orphan is collected; after the full walk completes,
the collected orphan errors are joined with type-verification errors and
presented together at the end of `plan.run`'s pre-flight (see D5).

Rationale: silent dead code is the worst failure mode — the author
believes their invocation is in the graph but it isn't. There is no
discard escape hatch at present. Starlark's `_` is not a blank identifier
like Go's — `_ = plan.file.write_text(...)` is a regular variable binding
to a variable named `_`, indistinguishable from any other binding at the
planner's level. Authors who don't want an invocation in the graph
simply don't construct it. If a "build but don't run" use case emerges
(inspection, testing), a future API like `plan.discard(invocation)` can
add it explicitly — not speculatively.

### D5 — Explicit root via `plan.run(root)`

`plan` is a starlark namespace, not an object. Two categories of
attribute access route through it:

- **Domain providers** — `plan.file.*`, `plan.service.*`, `plan.archive.*`,
  etc. — `plan.<provider>.<method>(...)` dispatches a domain operation.
- **Planner primitives** — `plan.subgraph`, `plan.choose`, `plan.case`,
  `plan.gather`, `plan.wait_until`, `plan.complete`, `plan.degraded`,
  `plan.fatal`, `plan.elevate`, `plan.options`, `plan.run` — direct on
  the `plan` namespace, not nested under any provider. These names are
  reserved planner-side; domain providers cannot declare methods with
  these names.

There is no "plan object," no ambient root, no accessor for a default
graph. Every `plan.*` call is a pure function from args to an invocation
(with the sole exception of `plan.run`, which terminates planning).

`plan.run(...)` is the terminal primitive. It accepts variadic
invocations and creates the graph from them:

```python
plan.run(a, b, c)                 # variadic form; common case
plan.run(plan.subgraph(a, b, c))  # single-invocation form; the one big subgraph case
```

The variadic form is shorthand for `plan.run(plan.subgraph(a, b, c))` —
the runner wraps the variadic invocations in a subgraph when more than
one is passed. Passing a single already-subgraph invocation uses it
directly.

**Graph creation happens here, not before.** Until `plan.run` is called,
authors are dealing only with invocations (which reference nodes and
subgraphs that exist conceptually but have no graph instance to belong
to). `plan.run` materializes the `op.Graph`, installs its single
`*op.Subgraph` root populated from the passed invocations, runs the
plan-end pre-flight, and hands the graph to the tool-level runner.

**Pre-flight error aggregation.** The pre-flight pass does not fail
fast. It runs every check (orphan detection D4, topological sort,
type verification D8) and collects every violation it finds.
`plan.run` joins the collected errors via `errors.Join` and returns
one report at the end. Users see the complete picture — every orphan,
every type mismatch — on a single run, not a one-at-a-time
fix-rerun-fix loop. A pre-flight with any violations aborts execution;
a clean pre-flight hands the graph off to the runner.

`plan.run` is single-call per `.star` file; a second call is a plan-time
error. Multi-graph scenarios (running multiple graphs in sequence or
parallel from one file) are composed at the tool level, not inside one
starlark script.

**Storage.** The `bind.StarlarkRuntime` session gains a `root *Invocation`
field (actually a slice when the variadic form is used) set by the first
`plan.run` call and consumed by the tool runner after starlark evaluation
completes. Orphan detection and type-checking walk from the invocations
stored there.

### D6 — Invocation registry

```go
package bind

type InvocationRegistry struct {
    mu      sync.Mutex
    ordered []*Invocation          // creation order; used for deterministic iteration
    byLabel map[string]*Invocation // label → invocation; used for lookup and orphan reporting
}

// Register appends inv to ordered and inserts it into byLabel under the
// given label. Duplicate labels (user-supplied collisions) are plan-time
// errors.
func (r *InvocationRegistry) Register(label string, inv *Invocation) error

// All returns every registered invocation in creation order. Used by the
// plan-end orphan pass and the type-check pass.
func (r *InvocationRegistry) All() []*Invocation

// ByLabel returns the invocation registered under label, or nil if no
// such invocation was registered.
func (r *InvocationRegistry) ByLabel(label string) *Invocation
```

Owned by `bind.StarlarkRuntime` (the per-session runtime). Every
`Planner.dispatch` call registers the invocation it constructed before
returning it to the starlark caller.

Writes happen only during planning. Reads happen during planning (orphan
walk, type-check walk) and at execute time (if lookup by label is ever
needed — probably not, but the data is available).

### D7 — Invocation options (label, retry policy)

Cross-cutting invocation concerns — currently the label and the retry
policy — are supplied via a single reserved kwarg `options` that accepts
a value built by `plan.options(...)`. A single reserved name keeps the
planner's kwarg surface tight; fields on the options value are
free to grow without claiming more kwargs.

```python
plan.file.write_text(
    destination=path,
    content=text,
    options=plan.options(label="write-config", retry_policy=plan.retry.exponential(max_attempts=3)),
)

plan.subgraph(a, b, c, options=plan.options(label="setup"))

plan.gather(items=xs, body=body, options=plan.options(retry_policy=linear))
```

**Go-side representation.**

```go
package bind

// Options collects plan-time-settable, cross-cutting concerns that apply
// uniformly to every invocation. Zero values mean "use the default":
// auto-generated label, no retry policy.
type Options struct {
    Label       string           // empty → auto-generated default label
    RetryPolicy *op.RetryPolicy  // nil → no retry
}
```

**Reserved kwarg: `options`.** Provider methods cannot declare a
parameter named `options`. Enforced at method registration (where
`parameters []string` is built in `receiver_type.go`) — any provider
that declares it fails program init with a clear message. Same treatment
applied to `*args` and `**kwargs`.

**Dispatch flow.** The planner's generic dispatch path (the code that
routes every `plan.*` call) intercepts the `options` kwarg before
passing the remaining kwargs to the method. Effective options are
applied to the constructed `Invocation`:

- `options.Label` supplied → registered under that label; auto-label
  skipped.
- `options.Label` empty → auto-labelled `<provider>.<method>#<N>` where
  N is the creation-order ordinal for that provider.method combination.
- `options.RetryPolicy` supplied → applied to the underlying Node or
  Subgraph (same hook as today's `Promise.retry` builtin).
- `options.RetryPolicy` nil → no retry.

Label collisions (user-supplied vs. user-supplied, or user-supplied vs.
auto-generated) are plan-time errors with a message naming both call
sites.

**Auto-labeling.** Format: `<provider>.<method>#<N>`.

```
file.write_text#1
file.write_text#2
file.mkdir#1
plan.choose#1
plan.subgraph#1
service.is_healthy#1
```

Derivation: planner knows the receiver type and method name at the
point of dispatch. A per-method counter in the registry yields the
ordinal. Monotonic within a `.star` evaluation; deterministic across
runs of the same script.

**Rejected alternatives** for the overall mechanism:
- **Individual reserved kwargs** (`label="…"`, `retry_policy=…`):
  every cross-cutting concern claims another reserved name; grows the
  planner's kwarg surface over time.
- **Fluent API** (`.label().retry_policy()`): if the initial dispatch
  registered under auto-label, fluent chains either mutate in place
  (violates I2) or create new `Invocation` copies that re-register
  under new labels (registry contains duplicates pointing at the same
  Target/Result — confusing for orphan detection and collision
  checking).
- **Decorator function** (`plan.create(inv, label=..., retry_policy=...)`):
  two-step construction; adds ceremony for the common case where users
  accept the default label.
- **Construction + mutation** (`inv.label = "name"`): explicit mutation
  of an Invocation after construction; violates I2 and I3.
- **Context-manager scope** (`with plan.retry(policy): …`): starlark
  has no `with` construct.

**Rejected alternatives** for the label format specifically:
- **Monotonic global** (`unit-1`, `unit-2`): opaque; gives no hint about
  what the invocation is.
- **Source-position-based** (`file.write_text@manifest.star:42`):
  fragile under refactors; labels shift whenever lines move.
- **Content-hash labels**: deterministic-by-args, enables caching, but
  unreadable and overkill for the current scope.

### D8 — Plan-time type checking

Every Promise→slot binding carries a type relationship: the slot's
parameter type (target) must accept the Promise's source-node output type
(source). `op.Convert` performs the runtime cascade; plan-time checking
answers "could Convert succeed?" without a value.

The per-type "can I convert to this target?" answer lives on the
`Converter` interface (D9). The Planner orchestrates the overall
cascade — it owns the walk over slot bindings, delegates the per-type
decision to `Converter.CanConvert` where applicable, and enforces the
fail-at-plan-time contract.

```go
package bind

// CanConvertTypes answers whether a source type can be converted to a
// target type under the current registry. Mirrors op.Convert's runtime
// cascade at the type level. The per-type decision for Converter-
// implementing source types delegates to Converter.CanConvert; other
// steps are answered via reflect.Type alone.
func (p *Planner) CanConvertTypes(source, target reflect.Type) bool {
    if source == target {
        return true
    }
    if source.AssignableTo(target) {
        return true
    }
    if source.Implements(converterType) {
        zero := reflect.Zero(source).Interface().(op.Converter)
        return zero.CanConvert(target)
    }
    if rt, ok := p.graph.ExecutionContext().Registry.TypeByReflection(target); ok {
        if _, isResource := rt.(op.ResourceReceiverType); isResource {
            return true
        }
    }
    if target.Kind() == reflect.Ptr {
        if rt, ok := p.graph.ExecutionContext().Registry.TypeByReflection(target.Elem()); ok {
            if _, isResource := rt.(op.ResourceReceiverType); isResource {
                return true
            }
        }
    }
    if source.Kind() == reflect.Slice && target.Kind() == reflect.Slice {
        return p.CanConvertTypes(source.Elem(), target.Elem())
    }
    return false
}
```

**`reflect.Zero(source).Interface().(op.Converter)`.** Plan-time type
check calls `CanConvert` on a zero value of the source type. Converter
implementations must be callable on zero receivers — no dereferencing,
no field access, pure type logic. This is a documented contract of the
`Converter` interface (D9).

**Plan-end pass ordering.** Runs after starlark evaluation completes, in
this order:

1. **Orphan detection** (D4). Walk from `plan.run`'s root; mark
   reachable invocations; error if any registered invocation is
   unreached.
2. **Topological sort.** Order the graph so type verification can walk
   edges in producer-before-consumer order.
3. **Type verification.** Walk every slot that holds a `PromiseValue`
   in topological order. For each:

```
source = slot's Promise source node's output type (inferred per D3 for
         container sources).
target = slot.Parameter.Type.
If !p.CanConvertTypes(source, target):
    error: "cannot bind <source-label> output to <consumer-label> slot %s
           (have %s, want %s)", slot.Name, source, target
```

Every type-mismatch is collected during the walk and joined with
orphan-detection errors at the end of pre-flight (see D5). No ill-typed
edges reach execution; users see every mismatch in a single report.

### D9 — `CanConvert` method on `op.Converter`

The `Converter` interface (Phase 7 step 8) gains a required type-level
predicate:

```go
package op

type Converter interface {
    Convert(target reflect.Type) (any, error)
    CanConvert(target reflect.Type) bool
}
```

Every type that implements `Converter` must implement `CanConvert`. The
method answers "can I, as a source value of my type, convert to this
target type?" without performing the conversion or any I/O.

**Nil-safety contract.** `CanConvert` is invoked by the Planner at
plan-time on a zero value of the source type
(`reflect.Zero(source).Interface().(Converter)`). Implementations must
not dereference the receiver or access fields. The method answers on
TYPE information alone — the receiver is present only to satisfy the
interface-method-call mechanism.

**Runtime use.** `op.Convert` calls `c.CanConvert(target)` before
`c.Convert(target)` as a lookahead. If `CanConvert` returns false,
`Convert` is skipped (no cost, no side effects). If it returns true,
`Convert` runs and may still error for a specific reason (e.g., an
actual I/O failure that the type-level check couldn't predict) — but
type-mismatch errors are ruled out by construction.

**Plan-time use.** The Planner's `CanConvertTypes` method (D8)
delegates the Converter-branch of its cascade to `CanConvert`. The
decision at plan time is final — there's no "optimistic trust" gap —
because `CanConvert` is required to be accurate on type information.

### D10 — Empty containers

A container without any operations is a plan-time error at the call
site. The rule applies uniformly to every grouping combinator — there is
no meaningful container that does nothing.

| Container | Empty-when | Error |
|---|---|---|
| `plan.subgraph(...)` | no invocations passed | "subgraph must contain at least one invocation" |
| `plan.choose(default, ...)` | no cases passed | "choose must declare at least one case" |
| `plan.gather(items, body, ...)` | no `body` | "gather requires a body invocation" |
| `plan.wait_until(predicate, ...)` | no `predicate` | "wait_until requires a predicate invocation" |

Items-empty gather is **not** an error — a gather over zero items is a
valid no-op iteration (the body never runs) and returns `[]any{}`. The
rule targets missing WORK, not missing ITEMS.

Rationale:
- An empty container has no work and no output; downstream consumers of
  its invocation have nothing meaningful to bind.
- Authors who want conditional contents build the arg list in starlark:
  `plan.subgraph(*([a, b] + ([c] if cond else [])))`.
- A mutable builder pattern (`plan.subgraph_builder()` → `b.add(...)` →
  `b.done()`) is not adopted; it conflicts with the functional,
  pure-plan-time model (invariant I2).

Empty-container errors are collected and joined with the rest of
pre-flight via D5's aggregation — users see every violation on a single
plan.run attempt, not one at a time.

### D11 — Migration of existing `.star` callers

Existing callers of the old Choose/Gather APIs migrate to the
invocation-passing form:

- `cmd/devlore-test/devloretest/data/test_is_*.star` — rewrite from
  `plan.choose(when=..., then=...)` kwargs form to the invocation-
  passing form with `plan.case(...)` members.
- `pkg/op/provider/plan/gen/*` — regenerates against the new plan
  provider shape (which absorbs the former flow provider) as each
  combinator redesign lands. The former `pkg/op/provider/flow/gen/*`
  files are removed along with the flow provider.
- Any `.star` doc snippets showing Choose/Gather call sites — update in
  place.

Each step that lands a combinator redesign includes its migration as
part of that step's PR.

**Deferred for now:**

- **Codegen template changes.** The current codegen templates emit the
  planner bridge under the old model. Instead of predicting what
  templates need to look like under the new model, we address template
  updates as each combinator redesign surfaces them — reactive rather
  than speculative.
- **`devlore-registry` and lore packages.** The `devlore-registry` repo
  and every lore package consuming this API will need a rewrite against
  the new planner surface (invocations, options kwarg, plan.run entry
  point, new Choose/Gather/Subgraph/WaitUntil shapes). That migration
  is a separate cross-repo effort tracked outside this phase. Phase 8
  lands the new API in this repo; downstream repos migrate in their
  own time.

## Invariants

### I1 — Plan-time type checking

Every Promise→slot binding is validated at plan-end via the Planner's
`CanConvertTypes`. Ill-typed bindings fail at plan time with a message
naming the source label, the consumer label, and the expected vs. actual
types. Because `Converter.CanConvert` is required to answer accurately
on type information alone (D9), plan-time decisions are final — no
trust gap between plan-time and runtime, no type-mismatch surprises
during execution.

### I2 — No hidden mutable planning state

Every `plan.*` call is a pure function from its starlark arguments to a
`*bind.Invocation`. The only mutable state during planning is the
`InvocationRegistry`, which is append-only until planning completes. Authors
can reorder, refactor, or extract helper functions without changing graph
semantics (beyond what the refactoring itself expresses).

### I3 — Invocation registry is write-once

After `plan.run(...)` is called, the registry is frozen. Orphan detection
and type-checking read from the frozen registry. Execution operates on
the graph reachable from the root invocation(s); the registry's presence
is incidental at execute time (available if needed for label lookup, but
no longer written).

## Updated step outline

1. **Invocation registry + options mechanism.** `bind.InvocationRegistry`
   on `StarlarkRuntime` with `ordered` + `byLabel`. Reserve the `options`
   kwarg at method registration and wire the planner's generic dispatch
   to intercept it. Introduce `bind.Options{Label, RetryPolicy}` and the
   `plan.options(...)` starlark builder. Auto-label format
   `<provider>.<method>#<N>`; user override via
   `options=plan.options(label=...)`.
2. **`bind.Invocation` struct.** `Target op.ExecutableUnit` +
   `Result *Promise`. Implements `starlark.Value` so it flows through
   starlark expressions. Every `Planner.dispatch` constructs and returns
   one.
3. **`Planner.FillSlot` dispatch by target type.** Slot expects
   `op.ExecutableUnit` → pull `invocation.Target`; else pull
   `invocation.Result` and use the existing Promise/edge logic from
   Phase 7.
4. **Merge flow provider into plan provider.** Move every flow method
   — `choose`, `gather`, `wait_until`, `complete`, `degraded`, `fatal`,
   `elevate` — from `pkg/op/provider/flow/` into the plan provider.
   Delete the flow package. Reserve the method names planner-side so
   domain providers can't collide. Starlark-side, all of these surface
   flat on `plan.X`. The combinator redesigns in steps 6–8 happen on
   the plan provider, not flow.
5. **`plan.subgraph(invocations…)` primitive.** New method on plan
   provider; takes variadic invocations, builds a subgraph. Owns
   container-output-type inference for subgraph per D3: `[]T` when
   terminals are homogeneous, `[]any` otherwise. Empty subgraph
   errors. Absorbs old Phase 11.
6. **`plan.choose` redesign.** `Case{When any, Then any}`; compensable
   method; `CompensateChoose` companion; lazy dispatch of branches via
   `Graph.ExecuteWithStack`. Owns container-output-type inference for
   choose per D3: `T` when default and every case's Then are
   homogeneous, `any` otherwise.
7. **`plan.gather` redesign.** `body=invocation`; existing Go-side
   Gather from Phase 7 step 10 stays; starlark-facing builder changes.
   Owns container-output-type inference for gather per D3: `[]T` where
   T is the body's return type; `[]any` when the body returns `any`.
8. **`plan.wait_until` redesign.** `predicate=invocation`; timeout
   surfaces as Action.Do error. Owns container-output-type inference
   for wait_until per D3: the predicate's return type.
9. **`plan.run(...)` explicit entry point.** Variadic invocations,
   wrapped in a subgraph when more than one is passed. No implicit
   top-level graph. Tool-runner picks up the root invocation(s). Owns
   the pre-flight pass that runs steps 10 + topological sort + 12 with
   error aggregation per D5.
10. **Orphan detection at plan-end.** Walk from `plan.run`'s root;
    mark reachable; collect unreached registry entries as errors.
11. **`CanConvert` method on `op.Converter` + `Planner.CanConvertTypes`
    method.** Interface addition to `op.Converter` (D9); corresponding
    method on the planner implementing the type-level cascade (D8).
12. **Topological sort + plan-time type-check pass.** Order the graph
    producer-before-consumer, then walk Promise→slot bindings in
    topological order; apply `Planner.CanConvertTypes`; collect
    mismatches as errors.
13. **Migration of existing `.star` callers.** Per D11.
14. **Test triage.** Run the full suite; fold residuals into follow-ups.

## Blast radius

- `pkg/op/action.go` — `CanConvert` interface method on `Converter`
  (D9) with the nil-safety contract documented.
- `pkg/op/bind/planner.go` — `Planner.dispatch` returns `*Invocation`;
  `Planner.FillSlot` dispatches by target type (D2); `Planner`
  gains `CanConvertTypes` (D8).
- `pkg/op/bind/promise.go` — `Promise` may stay as an internal helper
  or fold into `Invocation`; decide during step 2.
- `pkg/op/bind/starlark_runtime.go` — `InvocationRegistry` field
  (D6); `plan.run` wiring with pre-flight pass and error aggregation
  (D5).
- `pkg/op/provider/plan/` — absorbs the flow provider. Every planner
  primitive (`subgraph`, `choose`, `case`, `gather`, `wait_until`,
  `complete`, `degraded`, `fatal`, `elevate`, `options`, `run`) lives
  here as a provider method. Starlark-side, they surface flat on the
  `plan` namespace (not `plan.flow.*`).
- `pkg/op/provider/flow/` — **removed**. Contents absorbed into the
  plan provider.
- Codegen templates — update reactively as each combinator step lands;
  not a speculative upfront rewrite.
- `cmd/devlore-test/devloretest/data/test_is_*.star` — migration from
  the old kwargs form to the invocation-passing form.
- Any starlark test fixtures using the old Choose/Gather forms — same.

**Cross-repo follow-up (not blast-radius for this phase):**

- `devlore-registry` and every lore package. They consume this API and
  will rewrite against the new planner surface in their own time. The
  phase-8 plan lands the new shape here; downstream repos migrate
  separately.

## Dependencies

- **Follows Phase 7.** Gather's compensation pattern (Phase 7 step 10) and
  ctx threading (Phase 7 step 10) are the templates the new Choose design
  mirrors.
- **Precedes Phase 12.** Phase 12 addresses defects on what was formerly
  the flow provider — now the planner primitives on the plan provider.
  Some of those defects may only surface or become addressable after
  the invocation-based APIs land.
- **Precedes `devlore-registry` + lore-package rewrite.** Downstream
  consumers (the `devlore-registry` repo and every lore package that
  consumes this API) rewrite against the new planner surface —
  invocations, `options` kwarg, `plan.run` entry point, flat
  `plan.subgraph / choose / gather / wait_until / complete / degraded
  / fatal / elevate / options / run` namespace, old Choose/Gather
  forms replaced. Tracked as a cross-repo follow-up outside this
  phase; Phase 8 lands the new shape here, downstream migrates in
  its own time.

## Post-refactoring discussion topics

These are deferred until the current refactoring completes (Phase 7 through
the end of the planned phases). Raise them then.

### F1 — Multi-output providers (Bazel-style Providers)

Bazel rules return lists of typed `Provider` objects; consumers pattern-
match to pull named fields. Our invocation currently exposes one
`Promise` (one output). If combinators grow multi-field outputs (e.g.,
a subgraph returning "primary value" + "diagnostic trace"), a typed
provider system scales better than single-Promise invocations. Not
needed until a concrete use case arises.

### F2 — Hermeticity tightening

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
