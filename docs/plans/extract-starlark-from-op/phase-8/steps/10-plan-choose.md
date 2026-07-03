---
step: 10
title: "plan.choose — a subgraph whose topology is a binary decision tree; flow.Choose only executes it"
former_step: 13
former_title: "plan.choose initial redesign (superseded; successor open)"
status: implemented 2026-07-02 — decision tree landed (pieces 1–4 + tests); goal proof green
proof_run: 2026-06-16
parent: ../../phase-8.md
---

# Step 10 — plan.choose (formerly step 13)

**Status:** `implemented 2026-07-02`. The conditional-edge decision tree is landed — pieces 1–4 plus tests — and
`TestChoose_UnchosenInvocationBranchDoesNotRun` (the goal proof) is green. `flow.Choose` does nothing but execute the
graph.

## The model

**Choose is a subgraph. The graph *is* the logic.** `flow.Choose` carries no selection logic — it executes the graph
exactly as `flow.Subgraph` does (it can share `flow.Subgraph`'s body). The first-truthy short-circuit lives entirely in
the choose subgraph's **topology**, which `ChoosePlanner` builds at plan time from the case statements.

The topology is a **binary decision tree**: each `when`-subgraph is a *decision node* with two outgoing branches — a
**truthy** branch to that case's `then`-subgraph (a leaf), and a **falsy** branch to the next case's `when` (another
decision), with the last falsy branch going to the `default`-subgraph (a leaf). A right-leaning tree. Executing it is an
ordinary tree traversal that runs one root-to-leaf path; the branches not taken never run — that is the short-circuit.

This is why Choose is not shaped like Gather. **Gather's N is unknown until runtime**, so its method must create
activation records and fan out subgraphs in parallel. **Choose's cases are known at graph-creation time**, so the full
topology is built in advance and there is nothing left to do at runtime but execute it.

## The structure

`plan.choose(default=<body>, plan.case(when=<body>, then=<body>), …)`:

- `plan.case(when=<body>, then=<body>)` builds **two subgraphs** — one from each body, the same construction
  `plan.subgraph(body=[...])` uses (`resolveBodyChildren` → `op.NewSubgraph`). A body is a list of invocations, a
  singleton — a bare invocation is a one-element body — or a lambda: a bare starlark function is sugar for
  `plan.function.call(<lambda>)`, archived as a content-addressed `function.Resource` at plan time and invoked at
  dispatch (both settled 2026-07-02; `default=` accepts the same shapes). It returns:

  ```go
  type Case struct {
      When op.Subgraph // the when-subgraph, evaluated for truthiness (a decision node)
      Then op.Subgraph // the then-subgraph, run when When is truthy (a leaf)
  }
  ```

- `plan.choose` takes the `default` body (→ the default subgraph, a leaf) plus the variadic cases. `ChoosePlanner`
  lays out the `when`/`then`/`default` subgraphs as the choose subgraph's children and wires the decision tree with
  conditional edges. Zero cases is **defined behavior** (settled 2026-07-02, per the switch-statement precedent —
  Go's default-only `switch`, Python's lone `case _:`): `ChoosePlanner` emits the default subgraph as the only child
  with no guarded edges, the run-all path executes it, and the choose result is the default's result — by design, not
  coincidence. (The truly valueless form — no cases *and* no default — is unspellable: `default=` is required.)

## The graph — a binary decision tree

```
when₀ ───truthy───► then₀ (leaf)
  └─────falsy────► when₁ ───truthy───► then₁ (leaf)
                     └─────falsy────► … ── whenₙ ───truthy───► thenₙ (leaf)
                                              └─────falsy────► default (leaf)
```

Traversal: run `when₀`; read `isTruthy(result)`; follow the one branch whose guard matches — `GuardTruthy` lands on
`then₀` (a leaf, run it, its result is the choose result); `GuardFalsy` moves to `when₁`; repeat; the last `when`'s falsy
branch lands on `default`. Only the path taken runs.

## The conditional edge

`op.Edge` today is `{From, To}` — a pure ordering constraint consumed only by `topologicallySorted`. The rebuild adds a
guard:

```go
// GuardResult keys a conditional edge to an outcome of the guard —
// the truthiness evaluation of the From node's result.
type GuardResult uint8

const (
    // GuardNone means the edge is unguarded: a plain ordering edge,
    // always followed. As the zero value it is the default, and with
    // omitempty it never appears in serialized output, so existing
    // traces are unchanged.
    GuardNone GuardResult = iota
    // GuardTruthy is followed when the From node's result is truthy
    // in the Python sense: non-nil, non-false, non-zero, non-empty.
    // See isTruthy() for the exact rules.
    GuardTruthy
    // GuardFalsy is followed when the From node's result is falsy.
    GuardFalsy
)

var guardResultNames = [...]string{
    GuardNone:   "none",
    GuardTruthy: "truthy",
    GuardFalsy:  "falsy",
}

type Edge struct {
    From  string      `json:"from" yaml:"from"`
    To    string      `json:"to"   yaml:"to"`
    Guard GuardResult `json:"guard,omitempty" yaml:"guard,omitempty"` // omitempty ⇒ old traces unchanged
}
```

`GuardResult` serializes over `guardResultNames`: `MarshalText`/`UnmarshalText` cover JSON; matching `MarshalYAML`/
`UnmarshalYAML` are also required because `gopkg.in/yaml.v3` does not honor `encoding.TextMarshaler`. A serialized edge
then reads `"guard": "truthy"` in both formats.

A decision node emits two edges — `{when → then, GuardTruthy}` and `{when → next, GuardFalsy}`. Unguarded edges
(`GuardNone` — every existing subgraph) keep running all their children unchanged.

## The traversal

`flow.Choose` stays `walkSubgraphChildren`. That function gains one branch: a subgraph carrying conditional edges is
traversed as a decision tree instead of run in order.

```go
func walkSubgraphChildren(...) (any, error) {
    if hasConditionalEdges(subgraph) {                 // a choose subgraph
        return walkDecisionTree(activation, ctx, subgraph, stack, frame)
    }
    // ... unchanged run-all loop for Subgraph / Gather ...
}

func walkDecisionTree(activation, ctx, sg, stack, frame) (any, error) {
    current := root(sg)                                 // the one child that is no edge's To — when₀
    var result any
    for current != nil {
        r, err := activation.DispatchChild(ctx, current, stack, frame)
        if err != nil { return nil, fmt.Errorf("choose node %q: %w", current.ID(), err) }
        result = r
        current = branch(sg, current.ID(), isTruthy(r)) // matching GuardTruthy/GuardFalsy edge's To, or nil at a leaf
    }
    return result, nil                                  // the leaf (then/default) result
}
```

`branch` scans `sg.Edges()` for `From == id` whose `Guard` matches the source's truthiness and returns
`sg.ChildByID(edge.To)`; a node with no matching edge is a leaf, and more than one matching edge is an error (defense
in depth behind `validateGuardedEdges`). Both `Edges()` and `ChildByID` already exist. Subgraph/Gather subgraphs have
no conditional edges, so they take the run-all path — unchanged.

**Compensation & resume** ride the ordinary subgraph machinery: the nodes on the taken path leave receipts on
`activation.Stack`; `CompensateChoose` unwinds them. **Resume follows stamped guard outcomes** (settled 2026-07-01): a
decision node's receipt stamps the evaluated `GuardResult` alongside its result, and a serialized reload follows those
stamps — the recorded path — re-running nothing. Truthiness is evaluated exactly once per decision, on the live value;
it is never re-derived from a round-tripped result, because deserialization can flip truthiness (e.g. a non-zero struct
whose exported fields serialize empty replays as an empty map — falsy).

## Implementation order (four pieces)

1. **`op` — conditional edge + traversal support.** Add `GuardResult` + `Edge.Guard` and the name-table marshaling;
   give the recovery-stack stamp an optional guard outcome (`Guard GuardResult`, omitempty) so resume follows the
   recorded path; add `SubgraphSpec.Edges` + `WithEdges` and make `op.NewSubgraph` apply them; add
   `validateGuardedEdges` on the `validateEdges` path (the guarded-subgraph invariant below, enforced at both
   `ValidateGraph` boundaries) and ordering-edge cycle detection at the same boundaries (the validation error
   `topologicallySorted`'s doc comment promises but which does not exist yet); ensure `topologicallySorted` tolerates
   the tree edges (it will — the tree is acyclic, and topo order is only used by the run-all path). This is the
   substance and the risk.
2. **`flow` — the decision-tree walk.** `walkSubgraphChildren` gains the `hasConditionalEdges` branch + `walkDecisionTree`
   / `root` / `branch`. On replay `walkDecisionTree` follows the stamped guard outcome; `isTruthy` runs only on live
   results.
3. **`flow.NewCase` + `plan.case`.** `NewCase(when, then any) (*Case, error)` builds a subgraph per body
   (`resolveBodyChildren` + `op.NewSubgraph(WithActionNamed("flow.subgraph"))`); `plan.Provider.Case` delegates to it.
4. **`ChoosePlanner`.** Build the default subgraph + collect the cases; append `when`/`then`/`default` as children and
   emit the `GuardTruthy`/`GuardFalsy` edges; construct the choose subgraph (`WithActionNamed("flow.choose")` +
   `WithChildren` + `WithEdges`). `flow.Choose` itself is already reduced to execute-the-graph — leave it.

## Integration points to get right (flagged, not guessed)

- `op.NewSubgraph` must apply `spec.Edges` (today `SubgraphSpec` has no edges field; edges are only set via the
  unexported `setEdges`).
- **The guarded-subgraph invariant (settled 2026-07-01).** If a subgraph carries any guarded edge, its guarded edges
  must form a binary decision tree spanning its children: exactly one root (a child that is no edge's `To`); every
  other child exactly one incoming edge; every decision node exactly one `GuardTruthy` and one `GuardFalsy` out-edge;
  leaves none; no `GuardNone` edges mixed in; every child reachable from the root. `validateGuardedEdges` enforces
  this at both `ValidateGraph` boundaries — plan-seal and load — so a graph signed like ours but hand-built with a
  malformed decision point (two truthy edges, a cycle, an unreachable child) is detected when loaded, not mid-walk.
  A subgraph bound to `flow.choose` must additionally be either a well-formed decision tree or the degenerate
  zero-case form — exactly one child, zero edges (settled 2026-07-02) — so a hand-authored multi-child guardless
  choose (which run-all would execute in its entirety) is likewise rejected at load. Also confirm the validator does
  not reject leaf/childless nodes or children unreachable by ordering edges — `then`/`default` leaves have no
  outgoing edge.
- **Ordering-edge cycle validation (settled 2026-07-01).** `topologicallySorted` is deliberately cycle-tolerant — on a
  cycle it sorts what it can and appends the remainder so dispatch makes forward progress — and its doc comment claims
  the cycle "surfaces as a separate validation error"; no such check exists anywhere in `op` today. Piece 1 adds it at
  the same `ValidateGraph` boundaries (Kahn already knows: sorted count < unit count ⇔ cycle), completing the
  load-time principle: any signed-but-malformed document — guarded or plain — is rejected when loaded.
- **Loops are the anticipated extension (noted 2026-07-02).** `walkDecisionTree` is a guarded state machine — a falsy
  back-edge would loop, and loops-in-topology follow the same principle that settled choose. Cycle rejection is
  therefore scoped, not final: a future loop construct (`plan.while` / `plan.until`) legalizes its own well-formed
  cycles under a loop-shaped invariant (single entry, one `GuardTruthy` + one `GuardFalsy` out-edge per decision node,
  all nodes reachable, back-edges permitted), with two prerequisites the edge shape alone cannot supply:
  iteration-scoped receipt identity (Gather's `"<id>#<i>"` stamp pattern — without it a revisited unit replays its
  first receipt instead of re-executing) and a mandatory runtime budget (max iterations and/or timeout — topology
  cannot prove termination). An unexplained cycle is still rejected at load; only the construct grants one. WaitUntil
  (step 12) is the natural first customer.
- `Case` holds `op.Subgraph` **by value** (per the settled shape), so `&c.When` in the planner and `*w` in `NewCase`
  copy the struct — `go vet` will flag it if `op.Subgraph` contains a lock; if it does, that is the signal the field
  should be a pointer after all.
- `Edge.Guard` serialization uses `omitempty` so existing traces (all `GuardNone`, unguarded) round-trip unchanged.

## What is gutted (done 2026-07-01)

- `flow.Choose`'s `isTruthy` value-picker loop and its `defaultCase, cases ...Case` parameters — replaced by the
  execute-the-graph body (`walkSubgraphChildren`), signature `Choose(activation, kwargs)`.
- `resolveDispatchedValue` + `starlarkValueToGo` (helpers.go) — deleted.
- The value-semantics `Case{When any, Then any}` — replaced by `Case{When, Then op.Subgraph}`.
- **`isTruthy` stays** (the traversal reads it on `when` results; WaitUntil needs it too) — updated 2026-07-01 to
  Python/Starlark truth semantics: empty strings, slices, arrays, and maps are falsy, as are zero-value structs (the
  Go-conventions analogue of Python's empty-is-falsy) and typed-nil pointers/functions/channels; previously only the
  empty string was falsy and everything else fell to the truthy default.

**Current code state (implemented 2026-07-02):** pieces 1–4 are landed — `op.GuardResult` + `Edge.Guard` + text/YAML
marshaling, guard-stamped receipts (`SetGuard` / `GuardByUnitID`), `validateGuardedEdges` + ordering-edge cycle
detection at both boundaries, `SubgraphSpec.Edges` / `WithEdges`; `walkDecisionTree` / `root` / `branch` behind
`hasConditionalEdges`; `flow.NewCase` + `plan.case`; the tree-building `ChoosePlanner` (`planSubgraphFromParams`
deleted). The lambda surface rides the new `function.call` action (`function.Provider.Call(callable, *args,
**kwargs)` — the callable archived as a content-addressed `function.Resource`). Three latent defects fell out of
exercising it, all fixed: `extractSpan`'s span-end off-by-one (a mid-line lambda leaked the next byte into its
synthesized source), the `_lambda` / `_callable` synthesized-name mismatch, and bridge receiver derivation
hard-failing on `(T, bool)` methods (now skipped in derive-fresh mode so `*op.RecoveryStack` still projects). All
seven choose-family fixtures are rewritten to the decision-tree surface and green.

## Tests

- `TestChoose_UnchosenInvocationBranchDoesNotRun` — the goal proof: a side-effecting `when`/`then` on an unchosen or
  after-the-match branch must not execute. **Green 2026-07-02** (`test_choose_unchosen_branch.star`: canary writes on
  the unchosen then, the after-the-match case, and the default — all asserted absent).
- first-truthy short-circuit, no-match → default, and resume-replay (a reload follows the stamped guard outcomes —
  truthiness is not re-derived from round-tripped results).
- `op` unit tests — what `op` owns (settled 2026-07-02: tests split by package, no code moves; `op` consumes
  `GuardResult` structurally and never computes truthiness): `GuardResult` marshaling in both formats
  (`"guard": "truthy"`), `validateGuardedEdges` rejections (double-truthy edge, cycle, unreachable child, `GuardNone`
  mixing, no root), ordering-edge cycle rejection, and guard-stamp round-trip — all over explicit labels.
- `flow` unit tests — what `flow` owns (the walk is the only producer of a computed `GuardResult`): truthy/falsy
  routing, leaf termination, unguarded = run-all preserved, the `branch` ambiguity error, stamped-guard replay, and
  zero-case choose returns the default (`TestChoose_NoCases_ReturnsDefault` rewritten against the graph form — the
  one value-picker test whose asserted behavior survives).
- `isTruthy` unit tests updated 2026-07-01 for the settled truth semantics (empty containers, zero-value structs, and
  typed nils are falsy). The seven choose-family `.star` fixtures (5 choose + is_dir + is_file) are rewritten to the
  decision-tree surface — lambda bodies, singleton whens, keyword `default=` — with unit counts recomputed (done
  2026-07-02; a predicate invocation is adopted by exactly one when-subgraph, so multi-use scenarios mint fresh
  invocations).
