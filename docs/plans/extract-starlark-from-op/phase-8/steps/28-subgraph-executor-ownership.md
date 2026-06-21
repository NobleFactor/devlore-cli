---
step: 28 (prerequisite)
title: "Subgraphs own their executors — recovery-stack ownership moves to a per-subgraph executor"
status: approved 2026-06-20; implementation in progress (Subgraph combinator + per-subgraph executor first)
proof_run: n/a (not started)
parent: ../../phase-8.md
---

# Step 28 (prerequisite) — Subgraphs own their executors

**Status:** design draft (2026-06-20), awaiting approval. This is the execution-core prerequisite that unblocks the
step-28 pause/resume work; resume cannot skip already-completed units while flow combinators re-mint empty recovery
stacks on every dispatch.

## The model (settled)

Every subgraph executes via its own executor, and that executor owns the subgraph's recovery stack — together with the
subgraph's variable scope, pause signal, trace, and catalog scope. `Gather`, `Choose`, and `Subgraph` are not special
combinators; each *is* a subgraph with its own executor. One rule, applied recursively down the subgraph tree. (Recorded
as the authoritative principle in
[2.3-orchestration-primitives.md](../../../architecture/2.3-orchestration-primitives.md#subgraph-execution--recovery-stack-ownership-current-model--2026-06-20).)

## Current deviation

There is a single shared `op.GraphExecutor`. `Subgraph.Execute` hands that same executor to children via the
`dispatchChild` closure (`activation_record.go`), and the flow combinators hand-roll their own stacks:

- **Forward:** `flow.Subgraph` mints `op.NewRecoveryStack()` (`provider.go:369`), `flow.Gather` mints a per-iteration
  `iterStack` (`:234`) plus a `gathered` stack (`:276`), `flow.Choose` returns an empty stack (`:115`). The minted stack
  is the method's *complement* — returned as the middle value of `(any, *op.RecoveryStack, error)` and `PushNested` onto
  the parent stack by `pushAuditReceipt`.
- **Undo:** `CompensateSubgraph` / `CompensateGather` / `CompensateChoose` (`provider.go:391`/`:299`/`:133`) exist only
  to unwind that combinator-supplied complement-stack.

So the combinator owns the stack on **both** sides — it mints it forward and unwinds it back.

## The change — ownership moves to the executor, symmetric on both sides

1. **Child executor.** `Subgraph.Execute` constructs a child executor that **shares** the parent's runtime environment,
   variable frame, and pause signal, but **owns its own recovery stack**. This is a construction path distinct from
   `GraphExecutor.Run`: it does **not** rebuild the environment, clone the catalog, or rebind variables — those stay
   `Run`'s one-time top-of-tree responsibilities. Pause is run-global: the child observes the parent's
   `pauseRequested`.
2. **Forward signatures — every combinator keeps its complement.** Each combinator's forward action returns its
   compensation state as its complement; none drops to `(any, error)`. `Subgraph` drops only its vestigial `items`
   parameter (iteration is Gather's job): `Subgraph(activation, kwargs) (any, *op.RecoveryStack, error)` and
   `Choose(...) (any, *op.RecoveryStack, error)` return a single stack; `Gather(activation, items, kwargs)
   (any, []*op.RecoveryStack, error)` returns the **slice** of per-iteration stacks (one per iteration). What changes vs
   today is the *source* of the stack — the per-subgraph executor owns and creates it; `Do()` no longer mints it via
   `op.NewRecoveryStack()`. Regenerates the flow provider.
3. **Every combinator keeps its compensate companion.** `CompensateSubgraph(stack *op.RecoveryStack)`,
   `CompensateChoose(stack *op.RecoveryStack)`, and `CompensateGather(stacks []*op.RecoveryStack)` each consume the
   complement their forward returned and unwind it — Gather undoes the slice (each iteration's stack, LIFO / reverse
   completion order). **No companion is removed.** The deviation being fixed is `Do()` *minting* the stack, not the
   companion's existence.
4. **Gather calls Subgraph once per item.** Gather iterates its `items`, calling `Subgraph` for each — each call runs the
   body once under its own executor with its own stack (created in that iteration's goroutine, never shared, so no race).
   Gather collects the N stacks and returns `(results, []*op.RecoveryStack)` — the slice of per-iteration stacks; its
   companion `CompensateGather` undoes the slice (item 3). Gather no longer folds them into one `gathered` stack
   (`provider.go:276,281`). (Stack count was always "many"; this fixes who owns them and how they are returned/undone.)
5. **`DispatchChild` drops its `stack` parameter (settled).** The param exists today only to scope receipts to a saga
   boundary in the absence of per-subgraph executors — the combinator mints a stack and threads it down. Once the
   dispatching executor owns its stack, the param can only ever carry the stack that executor already holds, so it is
   redundant: `DispatchChild(ctx, child, variables)`. Retry semantics are unchanged.

## Combinator signatures (confirmed in review — 2026-06-20)

Every combinator keeps **both** an action and a compensation companion: the action returns its compensation state as its
complement, the companion undoes it. Signatures sorted by name; receivers are all `func (p *Provider) …`.

| Combinator | Action signature | Compensation signature |
|---|---|---|
| `Choose` | `Choose(activation *op.ActivationRecord, kwargs map[string]any) (any, *op.RecoveryStack, error)` | `CompensateChoose(stack *op.RecoveryStack) error` |
| `Gather` | `Gather(activation *op.ActivationRecord, items []any, kwargs map[string]any) (any, []*op.RecoveryStack, error)` | `CompensateGather(stacks []*op.RecoveryStack) error` |
| `Subgraph` | `Subgraph(activation *op.ActivationRecord, kwargs map[string]any) (any, *op.RecoveryStack, error)` | `CompensateSubgraph(stack *op.RecoveryStack) error` |
| `WaitUntil` | `WaitUntil(activation *op.ActivationRecord, kwargs map[string]any, timeout, interval time.Duration) (any, *op.RecoveryStack, error)` | `CompensateWaitUntil(stack *op.RecoveryStack) error` |

**Foundational principles (stakes in the ground):**

1. **Every combinator IS a subgraph** — each one's bound `Unit` is an `*op.Subgraph`. `Subgraph` is the base case: it
   runs its children directly.
2. **Every combinator except `Subgraph` delegates to `flow.Provider.Subgraph`** to execute one or more instances of its
   subgraph. `Subgraph` is the single primitive that actually runs children; the others are control flow over it,
   differing only in **how many** instances they run (and, for `Choose`, where selection happens):
   - **`Subgraph`** — base. Binds `kwargs` → `subgraph.Parameters()`, runs its children under that frame, returns the
     **final executable unit's result**. Single stack.
   - **`Choose`** — runs **exactly one** instance. **`Choose` does NOT select.** `ChoosePlanner` builds the branches
     **into the graph** at plan time; at runtime the **graph** selects the branch and `Choose` only **receives the
     result**. There is no runtime case-selection. `defaultCase` and `cases` are **plan-time inputs to `ChoosePlanner`**,
     not runtime inputs to the action — so the action signature is the `Subgraph` shape, and today's runtime
     `isTruthy(c.When)` loop (`provider.go:112-119`) goes.
   - **`WaitUntil`** — runs **one or more** instances. Binds `kwargs` like `Subgraph`, runs the body, tests its result
     for **truthiness** (Python-style, via the existing `isTruthy` helper), and re-runs at `interval` until true or
     `timeout`. The body subgraph is **expected side-effect-free** (nothing enforces this) — canonical use is polling for
     readiness, e.g. waiting for a container to start or a database to become available. Single stack = the final run's
     (the side-effect-free expectation is why the intermediate polls leave nothing to compensate).
   - **`Gather`** — runs **N** instances, one per item, concurrently; returns the **slice** of per-iteration stacks
     (`[]*op.RecoveryStack`, one per item); `CompensateGather` undoes the slice.
3. **Every combinator keeps its compensate companion.** No companion is removed. The deviation being fixed is `Do()`
   *minting* the stack via `op.NewRecoveryStack()`; the per-subgraph executor owns and creates it now.

## Saga-boundary semantics (settled 2026-06-20)

The saga boundary **is maintained** — rollback is a per-boundary unwind that propagates outward, **not** a single
root-level sweep. Each subgraph executor is a saga boundary and respects its retry policy:

- **Retry budget at the boundary.** No retries → one attempt; retry count N → N+1 attempts (the existing
  `DispatchChild` budget, now read as the boundary's — the subgraph's `RetryPolicy`, honored when the boundary is
  dispatched).
- **Retries exhaust before rollback propagates.** On failure the boundary runs its full retry budget first. No retries →
  rollback continues up the stack immediately; retry count N → all N are executed, then rollback continues up the stack.
- **Rollback continues up the stack** = the failure reaches the next outer saga boundary, which applies *its own* retry
  policy before unwinding its own stack and propagating further. Each executor unwinds its own stack — **replacing the
  current single top-level `Run` unwind** (`graph_executor.go:273`), which becomes one boundary among many (the root's).

- **Each failed attempt unwinds before it retries — forced by atomicity, not a choice.** A boundary is atomic, so a
  retry must run against the boundary's entry precondition. A failed attempt's completed children carry real side
  effects (a dir created, a resource allocated); re-running the body without first unwinding them double-applies
  non-idempotent operations and accrues duplicate receipts — the boundary stops being atomic. So each failed attempt
  compensates its own stack LIFO back to the entry precondition, then the next attempt runs clean. When the budget
  exhausts, the last attempt has already unwound (stack empty) and the bare failure propagates up, where the parent
  unwinds its own prior work per its own policy. (This is a behavior addition: today's `DispatchChild` re-dispatches
  without unwinding between attempts.) The "keep completed work, re-run from the failure point" model is **resume-after-
  pause** (sequence (b), skip-completed) — a different feature, not retry-on-failure; for an atomic boundary there is no
  no-undo retry.

## Files touched

- `pkg/op/graph_executor.go` — child-executor construction path; stack ownership.
- `pkg/op/subgraph.go` — `Subgraph.Execute` creates the child executor, nests its stack.
- `pkg/op/activation_record.go` — `DispatchChild` stack parameter (decision 5).
- `pkg/op/provider/flow/provider.go` + `flow/helpers.go` — combinators stop minting (`op.NewRecoveryStack()`) and take
  their stack from the executor via `activation.Stack`; each **keeps** its action **and** its compensate companion;
  `Subgraph` drops `items` and binds `kwargs` → parameters; `Choose` loses `defaultCase`/`cases` (plan-time
  `ChoosePlanner` inputs) and stops runtime case-selection; `Gather` returns a `[]*op.RecoveryStack` slice and calls
  `Subgraph` per item; `WaitUntil` becomes a combinator (poll the body until truthy/timeout).
- `pkg/op/provider/flow/gen/*` — regenerate (signature + companion changes).
- Tests: `flow`, `plan`, `cmd/devlore-test/devloretest` (gather/choose/compensation coverage).

## Sequencing within step 28

(a) **this prerequisite** → (b) resume re-entry + skip-completed (executor accepts `RunStatePaused`, preserves
`trace.Stack`, skips already-receipted units) → (c) catalog capture/restore in `op.Trace`. Step 28 does not close until
(c).

## Chained recovery stacks — up for resolution, down for unwind

Every executor instance owns its own recovery stack, and the per-subgraph stacks form one tree linked in **two
directions**, each serving a distinct job:

- **Chain up (parent pointer) — promise resolution.** A child stack points up to its parent. `ResultByUnitID` walks up
  the chain — this stack, then the parent, then the grandparent — until the producing unit's receipt is found, so a
  promise to an upstream producer resolves against whatever ancestor stack holds it.
- **Nest down (`PushNested`) — compensation.** A child stack is nested *into* its parent when the subgraph finishes. On
  failure, `Unwind` walks LIFO and recurses into nested substacks, so compensation cascades down the tree.

```
  SUBGRAPH TREE              EXECUTORS  →  STACKS

  root (flow.subgraph)       E0  ──owns──▶  S0 = [ rA , ▼ ]
  ├─ A   (node)                                      │
  └─ X   (subgraph)                    ┌──nest DOWN──┘   (PushNested: S0 contains S1 — for UNWIND)
         │                            ▼
         X dispatched →       E1  ──owns──▶  S1 = [ rB , rC ]
         ├─ B  (node)                          │
         └─ C  (node; slot →A)   S1.parent ────┘──chain UP──▶ S0   (for PROMISE RESOLUTION)
```

`C`'s slot is a promise to `A`, an upstream sibling of `X`. `A` ran under `E0`, so `rA` is on `S0`. Resolving `C`'s slot
calls `ResultByUnitID(A)`: miss on `S1` → walk up `S1.parent` to `S0` → hit `rA`. On failure, `S0.Unwind()` recurses into
the nested `S1` (compensating `rC`, `rB`), then `rA`.

**This is the resolution to the `activation.Stack` overload** (the open regression). `activation.Stack` is simply the
executor's **own** stack (`S1` for `X`): children's receipts land there and the combinator returns it. Input resolution
is not in tension with that, because `ResultByUnitID` walks the chain up to ancestors. Today's `ResultByUnitID` searches
a single stack's top level — "nested substacks are not searched" — and there is no parent pointer; this design adds the
**up-chain for resolution** while keeping the existing **down-nesting for unwind**.

### Saving and restoring the chain

The chain **is** the nested stack tree, so the `Trace` already carries it — no extra serialization:

- **Save.** `Trace.Stack` is the root stack, and `RecoveryStack` already serializes its nested substacks (the `sub` field
  recurses). Saving the trace saves the whole tree. The **parent pointers are not serialized** — they would be
  back-references (cycles) and are fully derivable.
- **Restore.** On load, deserialize the tree, then one re-chain pass walks it and sets each nested substack's parent to
  its container (`S1.parent = S0`). The up-chain is rebuilt from the down-tree; nothing beyond what was saved is needed.

> **Rule — the nesting is durable (serialized); the parent pointer is transient (derived on load).** Save serializes the
> tree; load rebuilds the tree and re-derives the chain.

This is exactly what resume needs: the restored chain supports **up-resolution** (a re-dispatched unit's promise walks up
to an ancestor's receipt) and **skip-completed** (the completed children's receipts already sit in the restored
substacks; the "seed-from-restored" child executor in PHASE 5 of the sequence below *is* its slot in the restored chain,
so a unit with a receipt there is skipped, not re-run).

## Save / load / restart sequence

The full target flow across (a) the per-subgraph executor, (b) resume re-entry + skip-completed, and (c) catalog
capture. `[Catalog]` / `(c)` markers show where the catalog work plugs in.

```
PHASE 1 — RUN (until pause)
─────────────────────────────────────────────────────────────────────────────
Host    → GE        : NewGraphExecutor(graph, spec)
Host    → GE        : Run(ctx, vars)
GE                  : state=Running ; GE.stack = NewRecoveryStack()
GE      → Subgraph.Execute(Root) : Execute(ctx, exec=GE, GE.stack, vars)   // Root is a Subgraph
  Sub.Execute       : pausePointObserved()? → false
  Sub.Execute → GE  : childExec = GE.newChildExecutor()
                      // childExec.stack = fresh ; SHARES env, vars, *pauseRequested
  Sub.Execute       : activation.Stack = childExec.stack   // ← the subgraph's OWN stack
  Sub.Execute → Comb: Do(activation)                       // flow.Provider.Subgraph
    Comb            : frame = bind kwargs → subgraph.Parameters()  (layered on activation.Variables)
    Comb → DispatchChild → childExec : child[0].Execute(ctx, childExec, childExec.stack, frame)
        child[0]    : Do() OK → pushAuditReceipt → childExec.stack.Push(receipt[0])   ✓
    Comb → DispatchChild → childExec : child[1].Execute(...)
        child[1]    : Do() OK → childExec.stack.Push(receipt[1])                       ✓
    …(child[2] about to dispatch)…

PHASE 2 — PAUSE  (from another goroutine)
─────────────────────────────────────────────────────────────────────────────
Host    → GE        : Pause() → GE.pauseRequested.Store(true)   // shared *atomic.Bool
  child[2].Execute  : pausePointObserved() → pauseRequested.Load()=true → state=Paused → return ErrPaused
  ErrPaused bubbles : child[2] → Comb → Sub.Execute → GE.Run
GE.Run              : errors.Is(err, ErrPaused) → state=RunStatePaused ; RETURN ErrPaused
                      // NO unwind — the stack IS the resume point
  ► State: GE.stack = [ nested(childExec.stack = [receipt[0], receipt[1]]) ] ; child[2..] un-run

PHASE 3 — SAVE
─────────────────────────────────────────────────────────────────────────────
Host    → GE        : Trace() → { GraphChecksum, State=Paused, Stack=GE.stack, Variables[, Catalog ← (c)] }
Host    → Disk      : SaveDefinition(graph)        // graph.Serialize  (JSON/YAML)
Host    → Disk      : document.Write(trace)

PHASE 4 — LOAD   (later / fresh process)
─────────────────────────────────────────────────────────────────────────────
Host    → Disk      : graph2 = LoadDefinition(path)
Host    → Disk      : trace2 = document.Read(path)
Assert              : graph2.Checksum() == trace2.GraphChecksum    // else incompatible → error

PHASE 5 — RESTART  (resume from the pause point)
─────────────────────────────────────────────────────────────────────────────
Host    → Resume    : exec2 = ResumeExecutor(graph2, spec, trace2)
                      // exec2.state=Paused ; exec2.stack = trace2.Stack ; vars restored [; Catalog ← (c)]
Host    → exec2     : Run(ctx, nil)
exec2.Run           : ACCEPT state==RunStatePaused  (b) ; do NOT reset stack  (b)
exec2.Run → Subgraph.Execute(Root) : Execute(ctx, exec2, exec2.stack, vars)
  Sub.Execute       : childExec2 = child executor SEEDED from the restored nested substack  (b)
  Sub.Execute → Comb: Do(activation)            // re-walks ALL children
    Comb → DispatchChild : child[0] → SKIP (successful receipt in seeded stack) → return cached result[0]  (b)
    Comb → DispatchChild : child[1] → SKIP (successful receipt) → return cached result[1]                  (b)
    Comb → DispatchChild → childExec2 : child[2].Execute(...) → Do() runs → push receipt[2]  // un-run resumes
    …continues to the end…
exec2.Run           : state=Completed ; return final result
```

**Proof points — what each phase relies on:**

- **(a)** `childExec` owns `childExec.stack`; `activation.Stack` is that stack, distinct from the parent — so the
  combinator returns it and `pushAuditReceipt` nests it without self-nesting.
- **Pause** is a *shared* `*atomic.Bool` checked at a pause-point **before each dispatch**; `ErrPaused` returns *without
  unwinding* (the stack is the resume point), and `Run` stamps `RunStatePaused`.
- **Save/Load** is checksum-gated: `graph2.Checksum() == trace2.GraphChecksum`, or the trace is rejected.
- **(b) — the crux** is RESTART: `Run` must *accept* `RunStatePaused`, *not* reset the stack, **seed each subgraph's
  child executor from the restored substack**, and **skip** children that already carry a successful receipt (returning
  their cached result) while dispatching the un-run ones.
- **(c)** is the `[Catalog]` markers in PHASE 3/5: until the catalog is in the `Trace`, resources shared by URI do not
  survive the round-trip; promise/slot results (which live in the stack) do.

**Implementation status (2026-06-21):**

- **Built:** `newChildExecutor`, shared pause flag, `Run` Paused-stamp, `Subgraph` kwargs-binding, `Trace()`,
  `SaveDefinition`/`LoadDefinition`, the checksum gate.
- **Attempted — regression diagnosed, fix chosen (C):** `Subgraph.Execute` creates `childExec` and `flow.Subgraph` reads
  `activation.Stack`, which **overloaded `activation.Stack`** — combinators already read it for *input* promise
  resolution (`resolveDispatchedValue` → upstream siblings = the **parent** stack), so repointing it at the **child**
  stack broke `TestChoose_NotExists` / `TestChoose_Predicates` and `TestSubgraph_ReturnsRecoveryStack`. **Resolution —
  option (C), chained stacks** (see *Chained recovery stacks* above): `activation.Stack` stays the executor's **own**
  stack, and `ResultByUnitID` walks the **parent chain up** for input resolution. The fix: give `RecoveryStack` a parent
  pointer (set when the child stack is minted in `newChildExecutor` / `subgraph.Execute`); make `ResultByUnitID` walk it;
  and re-derive the chain on `Trace` load (nesting durable, parent pointer transient). This code is uncommitted until (C)
  is implemented and the tree is re-greened.
- **Pending:** `Run` accepting `RunStatePaused` + seed-from-restored + **skip-completed** (all of (b)); `Catalog` in
  `Trace` (c); `TestSubgraph_ReturnsRecoveryStack` replaced by an executor-driven integration test.

## Control plane — the executor's bidirectional command / event surface

A run is **steered** from outside and **observed** from outside. Today the steering is a single shared `*atomic.Bool`
(`pauseRequested`) and the observing is a set of output channels scattered onto the runtime environment — the wrong shape
for what is, in truth, one surface. The **control plane** is a single concept with two directions:

- **Commands in** — a consumer *steers* the run: pause, stop, step, …
- **Events out** — a consumer *observes* the run: lifecycle transitions, status, results.

A listener bridges a connection to this plane: it subscribes to the events and issues the commands. Step 28 realizes only
the **pause** command — but it does so *through the plane's primitives*, so everything else here is a forward extension,
not a later rewrite.

### Commands in — `ExecutionControl`

A bool answers one yes/no question. Steering is a **command domain**, not a yes/no — and **stop** is the example that
proves it: stop is not "a louder pause." The two take different paths through `Run`:

| | Pause | Stop |
|---|---|---|
| Halt at the control-point | yes | yes |
| Recovery stack | **preserved** as the resume point | **unwound** (compensate completed work); or left, for a hard stop |
| Terminal state | `RunStatePaused` | `RunStateStopped` (terminal) |
| Resumable | yes (`ResumeExecutor`) | no |
| Error returned | `ErrPaused` (no unwind) | `ErrStopped` (`Run` unwinds, terminates) |

A bool cannot encode "halt-and-preserve-for-resume" versus "halt-and-roll-back-and-terminate," and a flag per verb
(`pauseRequested`, `stopRequested`, `stepRequested`, …) is the smell that begs the right primitive: a shared **control
command** the executor polls at each control-point.

```go
type ControlCommand int32

const (
	ControlNone ControlCommand = iota
	ControlPause
	ControlStop
	// future: ControlStep, ControlCancel, ...
)

// ExecutionControl is the commands-in half of the control plane: the pending command, set by a listener and polled by
// the executor. Shared by the whole run — every child executor holds the same pointer — so a command issued anywhere is
// observed everywhere.
type ExecutionControl struct{ pending atomic.Int32 }

func (c *ExecutionControl) Request(cmd ControlCommand) { c.pending.Store(int32(cmd)) }
func (c *ExecutionControl) Pending() ControlCommand    { return ControlCommand(c.pending.Load()) }
```

The pause-point becomes a **control-point** — a `switch`, not a bool test:

```go
switch e.control.Pending() {
case ControlPause:
	e.state = RunStatePaused
	return ErrPaused // preserve; resumable via ResumeExecutor
case ControlStop:
	e.state = RunStateStopping
	return ErrStopped // Run unwinds + terminates; not resumable
}
// ControlNone → dispatch
```

Each new command is one enum value and one `case` — no new flag, no function-signature change. `GraphExecutor.Pause()`
and `Stop()` become thin conveniences that delegate to `control.Request(...)`; the listener does the same for inbound
connection commands.

### Events out — the observability stream

The opposite direction already exists, but scattered across two objects. Three channels carry events out of a run:

- **`HookRegistry`** — lifecycle events (`FireNodeStart` / `FireNodeComplete` / `FireSubgraphStart` …); on the
  `GraphExecutor` (`graph_executor.go:44`).
- **`Status *status.Narrator`** — the user-facing progress side-channel; on `RuntimeEnvironment`.
- **`Result *result.Pipeline`** — the primary output pipeline; on `RuntimeEnvironment`.

Introducing the control plane reveals that **`Status` and `Result` are misfiled.** Neither is "the world the execution
acts on"; each is a **channel the execution reports out through** — exactly what a listener subscribes to. They sit on
the runtime environment today only because there was no control-plane object to hold them. Under this design they join
`HookRegistry` as the **events-out** half of the plane:

```
control plane  ──┬── commands in   : ExecutionControl   (pause / stop / step)
                 └── events out     : HookRegistry (lifecycle) · Status (narrator) · Result (pipeline)
```

The symmetry is the point: `ExecutionControl` is to commands-in what `HookRegistry` / `Status` / `Result` are to
events-out. One plane, two directions.

### Where the plane lives — ownership and the context-object taxonomy

The control plane is owned by the **`GraphExecutor`** — the run's *driver* and the *command target* (`Pause()`/`Stop()`
are executor methods; the listener bridges the connection to the executor) — and shared down to child executors via
`newChildExecutor`, exactly the way the runtime environment and today's pause flag are shared. It does **not** belong on
`RuntimeEnvironment`, which is the outside world the execution acts on, not the steering of the execution.

With it in place, the run's context objects separate cleanly on two axes — **scope** and **concern** — with no overlap:

| Object | Scope | Concern (what it carries) | Direction |
|---|---|---|---|
| `context.Context` | per-call (rooted per-Run, derived per-dispatch) | cancellation + deadline (+ request values) | external → in: "abort now" — terminal, irreversible |
| `RuntimeEnvironment` | per-Run, shared across the dispatch tree | the **world**: catalog, providers, root, platform, resolver | the substrate the execution acts **on** |
| control plane | per-Run, shared across child executors | **commands in** (`ExecutionControl`) + **events out** (`HookRegistry` · `Status` · `Result`) | bidirectional: consumer ↔ run |
| `ActivationRecord` | per-dispatch, transient | the **one call**: Unit, Slots, Variables, Stack, dispatchChild | bundle for a single `Action.Do` |

The control plane is the missing per-Run plane alongside the **world** (`RuntimeEnvironment`) and **cancellation**
(`Context`), with `ActivationRecord` the per-call bundle beneath them. It is **not** `Context` (control is reversible
orchestration, not a one-shot hard cancel), **not** `RuntimeEnvironment` (the world, not the steering wheel), and **not**
`ActivationRecord` (per-call/transient — the wrong lifecycle).

Sorting `RuntimeEnvironment`'s fields against this taxonomy confirms what stays and what moves:

- **World (stays):** `Application`, `Modules`, `Platform`, `Root`, `ResourceCatalog`, `RecoverySite`, `variableResolver`,
  `variables`, `resolvers`, `declaredParameters`, and the provider-cache machinery.
- **Cancellation (its own plane):** `Context`.
- **Policy / config (set up-front, not consumer-steered — stays):** `ConflictPolicy`, `BackupSuffix`.
- **Observability (moves to the control plane):** `Status`, `Result`.

### Scope and migration

Step 28 implements **only pause**, but through the plane's primitive: the shared reference threaded to child executors is
`*ExecutionControl` (carrying `ControlPause`), not `*atomic.Bool`, so the plane is forward-compatible from the start.
Everything else is documented direction, scoped as follow-on:

- `Stop` / `ErrStopped` / `RunStateStopped`, then `step` and the listener/connection — each a new enum value and `case`
  on the commands-in primitive.
- Moving `Status` and `Result` off `RuntimeEnvironment` onto the control plane. **This is a real refactor, not a field
  move:** the *do* layer emits to them today via `activation.RuntimeEnvironment.Status` / `.Result`, so the emission path
  re-threads through the control plane. The control-plane framing is precisely what exposes that they were on the wrong
  object.

(The current code still uses `*atomic.Bool` as an interim; migrate it to `*ExecutionControl` when wiring (b).)

## Implementation verification note

Every combinator keeps its compensate companion (settled). The forward action returns the executor's stack as its
complement (`Gather`: a `[]*op.RecoveryStack` slice); the companion unwinds it. During implementation, confirm the
complement routing: the forward returns `activation.Stack` (the per-subgraph executor's own child stack), and
`pushAuditReceipt` nests it onto the **parent** stack — so `activation.Stack` must be the subgraph's own child stack,
**distinct** from the parent stack, or it nests onto itself.
