# Execution Graph Architecture

> **Status:** rewritten 2026-07-21 (phase-8 step 34) onto the landed `pkg/op` model, replacing the pre-`op` design
> (`internal/graph`, `ExecutionGraph`, `SlotValue`, `GraphState`) this document previously described. The historical
> mapping is at the end. Companion: [`2-execution-graph.status.md`](2-execution-graph.status.md).

## Thesis

One sealed graph model serves every workflow. A **planner** (the Starlark planning API) or a Go caller builds an
immutable `op.Graph`; `op.GraphExecutor` executes it under a per-run runtime environment; the **trace** records what
happened. The graph is the plan, the trace is the execution record — two separate documents, not one structure read
two ways.

## The unit model

A graph is a tree of **executable units** (`op.ExecutableUnit`), of exactly two kinds:

1. **`op.Node`** — one dispatch of one bound action (`file.WriteText`, `pkg.Install`, …).
2. **`op.Subgraph`** — a recursive container of child units. The graph's root is a subgraph
   (`Graph.Root()`), and every orchestration combinator *is* a subgraph — Choose, Gather, and WaitUntil are
   quantifiers over the one Subgraph base case, each dispatch executed by its **own child executor**
   ([2.3](2.3-orchestration-primitives.md), [3.5.2](3.5.2-flow-provider.md)).

`op.Graph` wraps the root with metadata: origin, timestamp, schema version, the planning `ResourceCatalog`, an
integrity checksum, and an optional signature. Read access is total — `Nodes()`, `Subgraphs()`, `Edges()`,
`UnitCount()`, `ResolveExecutable(id)`, `SubgraphByID(id)`, and `Parameters()` (the bubble-up variable surface) — and
mutation access is nil.

## Sealed construction

Construction is **spec-based and setter-free** (`pkg/op/graph.go`): populate a `GraphSpec`, then seal it.

```go
graph, err := op.NewGraph(op.NewGraphSpec().
	WithOrigin(origin).            // who planned this graph (op.Origin)
	WithUnits(node1, sub2, node3). // the root subgraph's children
	WithSlot("target", binding))   // root-level slot bindings
```

The spec's full builder set: `WithUnits`, `WithSlot(name, Binding)`, `WithOrigin`, `WithResourceCatalog`,
`WithRetryPolicy`, `WithTransitionPolicy`, `WithOnError(subgraph)`, `WithOnRetry(subgraph)`, `WithElevationOffer`.
`Node` and `Subgraph` are built the same way (`NewNodeSpec()`/`NewSubgraphSpec()` → `NewNode`/`NewSubgraph`).

`NewGraph` does the structural work once, at seal time:

1. Builds the root subgraph from the spec's units, slots, retry policy, and error action.
2. **Materializes dependency edges** from the units' promise bindings and **topologically sorts** the children.
3. Indexes every reachable unit into the unit table.
4. Computes the integrity checksum from `Graph.CanonicalContent()`.

The returned graph has no public setters; every later session-owner — an executor, a serializer, an inspector — reads
without mutating. Signing is deliberately not done at construction: the load path preserves a document's existing
signature, and a fresh graph is signed through `Graph.SignWith` (the ciphersuite lives in `pkg/signing`;
[5-receipt-integrity](5-receipt-integrity.md)).

## Bindings — how units receive inputs

A unit's slots are bound through the sealed **`op.Binding`** set (`pkg/op/binding.go`; the full slot model is
[2.1 Typed Slots](2.1-typed-slots.md)):

| Binding | Meaning | Resolved |
|---|---|---|
| `ImmediateBinding` | a Go value known at plan time | plan time |
| `PromiseBinding` | another unit's result, referenced by unit ID | dispatch time |
| `VariableBinding` | a named variable, with optional field projection (`NewVariableBindingWithField`) | resolve time |

Promise bindings are load-bearing structure: the dependency **edges are derived from them** at seal time, and the
plan-time validation pass (`pkg/op/validate.go`) walks every producer → consumer pair and **type-checks** the promised
value against the consuming slot (`checkPromiseTypes`), alongside orphan detection.

## Building graphs — two front doors

1. **The Starlark planning API** — `plan.Provider` and the planner machinery assemble specs from `.star` programs
   (`plan.subgraph`, `plan.choose`, `plan.gather`, `plan.wait_until`, `plan.run`, `plan.save`/`plan.load`, …). This is
   the product's primary surface ([3-operation-namespaces](3-operation-namespaces.md),
   [2.5 lifecycle-pipeline construction](2.5-lifecycle-pipeline-construction.md)).
2. **Direct Go construction** — commands and tests populate specs directly, as above.

Both produce the same sealed artifact; the executor cannot tell them apart.

## Execution

`op.GraphExecutor` (`pkg/op/graph_executor.go`) drives one execution per instance:

```go
executor := op.NewGraphExecutor(graph, runtimeEnvironmentSpec)
result, err := executor.Run(ctx, variables)
status := executor.RunStatus() // phase × condition × reason
trace := executor.Trace()
```

`Run` builds a fresh per-run `RuntimeEnvironment` from the spec, clones the graph's planning catalog onto it, resolves
the variable surface (`Graph.Parameters()` against the application's sources, with caller-supplied `variables` layered
on top), dispatches the root, and tears the environment down. Each subgraph dispatch executes under its **own child
executor** that owns its recovery stack, sharing the parent's environment and control plane. On failure the recovery
stack unwinds — every completed compensable action has `Compensate` called with its receipt
([2.2](2.2-phase-execution.md), [5-receipt-integrity](5-receipt-integrity.md)).

**The result contract.** `Run` returns `(any, error)`: the value is the **final dispatch's output** (structural
subgraphs bubble their last unit's return up); the error reflects whether the run **halted** (a stop, an unhandled
failure, a pause via `ErrPaused`/`ErrStopped`). Health is read separately: `RunStatus()` is the
`phase × condition × reason` triplet, so a run whose `TransitionPolicy` continues past a failure returns
`(result, nil)` yet reports `completed × execution_failed` ([2.2](2.2-phase-execution.md) owns the machine).

**Steering, observing, resuming.**

- `Pause()` / `Stop()` / `Control()` — the control plane's command surface ([2.7](2.7-control-plane.md)).
- `SetHooks` — the lifecycle-hook seam feeding the observability surface ([2.8](2.8-eventing-infrastructure.md)).
- `ResumeExecutor(graph, spec, trace)` — forward resume of a paused trace.
- `ResumeUnwind(ctx)` — the restart contract for a `stopped × compensation_failed` trace: a resumed, state-checked
  unwind, never a forward retry.

**Policies.** Per-unit retry (tri-state: explicit / inherited default / none) and the transition policy
(continue / pause / stop per condition) resolve against config floors ([2.6](2.6-execution-policies.md)).

## Integrity and persistence

Two documents, two roles:

1. **The graph document** — the plan. `Graph.Serialize(encoder)` / `MarshalJSON` / `MarshalYAML` emit the document
   form (kind `com.noblefactor.DevLore.Graph`, `Graph.Filename()` naming); `op.LoadGraph` reconstructs a sealed graph
   from it — the `plan.save` / `plan.load` round trip. The checksum covers `CanonicalContent`; the signature (when
   present) is verified by `writ verify` against the signing policy ladder.
2. **The trace** — the execution record: the `RunStatus`, the recovery stack of per-dispatch receipts (audit +
   compensation), the catalog snapshot with content identity (Etag + Digest), and the transition journal
   ([5.2 recovery serialization](5.2-recovery-serialization.md)).

The store lives behind `internal/cli`: `WriteGraph` persists the plan once, `WriteTrace` persists every run's trace —
win or lose — and both append to the NDJSON **run index** that `writ status` and the deploy family fold over.

## The command layer

Commands stay thin: build (or load) the graph, persist the plan, execute, persist the trace.

```go
// the shape of cmd/writ/writ/deploy/deploy.go
if _, err := cli.WriteGraph(graph); err != nil { … }
executor := op.NewGraphExecutor(graph, spec)
_, runErr := executor.Run(ctx, nil)
if receiptPath, writeErr := cli.WriteTrace(executor.Trace()); … // written win or lose
```

Live call sites: `cmd/writ` (deploy, upgrade, decommission, adopt, migrate), `cmd/lore`, and `cmd/devlore-test`.
Configuration roll-up (defaults → file → environment → flags) is owned by [configuration.md](configuration.md), not by
the graph.

## What replaced the pre-`op` design

| Pre-`op` (this document's former body) | Landed model |
|---|---|
| `ExecutionGraph` in `internal/graph/` | sealed `op.Graph` in `pkg/op/` |
| `GraphBuilder.Build(config)` | planner-assembled or Go-built `GraphSpec` → `op.NewGraph` |
| `Slots map[string]SlotValue` | the sealed `op.Binding` set (Immediate / Promise / Variable) |
| `GraphState` (pending / executed / failed) | `RunStatus{Phase, Condition, Reason}` on the executor / trace |
| per-`Node` status strings mutated by `Run` | immutable graph; outcomes recorded as receipts in the trace |
| `graph.Run()` (graph executes itself) | `op.GraphExecutor.Run` (one executor per run; child executors per subgraph) |
| one structure serialized before/after `Run` | two documents: the graph (plan) and the trace (record) |
| `receipts/` directory + `state.yaml` | the `internal/cli` store: graph + trace documents + the NDJSON run index |
| checksum + optional age signing inline | `CanonicalContent` checksum at seal; `pkg/signing` via `SignWith` |
