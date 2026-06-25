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
- **Carried down (receipt complement) — compensation.** When a subgraph finishes, its child stack is carried on the
  subgraph's audit receipt as that receipt's **complement** (committed via `Commit`, not a separate `PushNested` entry).
  On failure, `Unwind` walks the parent stack LIFO and invokes each receipt's `Compensate` companion; the subgraph
  receipt's `CompensateSubgraph` unwinds the complement child stack, so compensation cascades down the tree.
  (`PushNested` survives only for `Gather`'s internal per-item grouping — its `gathered` complement is itself a stack of
  per-iteration substacks.)

```
  SUBGRAPH TREE              EXECUTORS  →  STACKS

  root (flow.subgraph)       E0  ──owns──▶  S0 = [ rA , rX ]
  ├─ A   (node)                                      │
  └─ X   (subgraph)                    ┌──complement─┘   (rX.Complement() = S1 — for UNWIND)
         │                            ▼
         X dispatched →       E1  ──owns──▶  S1 = [ rB , rC ]
         ├─ B  (node)                          │
         └─ C  (node; slot →A)   S1.parent ────┘──chain UP──▶ S0   (for PROMISE RESOLUTION)
```

`C`'s slot is a promise to `A`, an upstream sibling of `X`. `A` ran under `E0`, so `rA` is on `S0`. Resolving `C`'s slot
calls `ResultByUnitID(A)`: miss on `S1` → walk up `S1.parent` to `S0` → hit `rA`. On failure, `S0.Unwind()` invokes
`rX`'s `Compensate` companion (`CompensateSubgraph`), which unwinds `rX`'s complement `S1` (compensating `rC`, `rB`),
then `rA`.

**This is the resolution to the `activation.Stack` overload** (the open regression). `activation.Stack` is simply the
executor's **own** stack (`S1` for `X`): children's receipts land there and the combinator returns it. Input resolution
is not in tension with that, because `ResultByUnitID` walks the chain up to ancestors. Today's `ResultByUnitID` searches
a single stack's top level — "nested substacks are not searched" — and there is no parent pointer; this design adds the
**up-chain for resolution** while keeping the **down-direction for unwind** — carried on the receipt complement.

### Saving and restoring the chain

The chain **is** the receipt-complement tree, so the `Trace` already carries it — no extra serialization:

- **Save.** `Trace.Stack` is the root stack, and a subgraph receipt serializes its complement child stack (`ReceiptBase`
  serializes `Complement`, which recurses through the tree). Saving the trace saves the whole tree. The **parent
  pointers are not serialized** — they would be back-references (cycles) and are fully derivable.
- **Restore.** On load, deserialize the tree, then one re-chain pass walks it and sets each child stack's parent to its
  container (`S1.parent = S0`). The up-chain is rebuilt from the down-tree; nothing beyond what was saved is needed.

> **Rule — the complement nesting is durable (serialized on the receipt); the parent pointer is transient (derived on
> load).** Save serializes the tree; load rebuilds the tree and re-derives the chain.

This is exactly what resume needs: the restored chain supports **up-resolution** (a re-dispatched unit's promise walks up
to an ancestor's receipt) and **skip-completed** (the completed children's receipts already sit in the restored
complement child stacks; the "adopt-restored" child executor in the resume descent *is* its slot in the restored chain,
so a unit with a receipt there is skipped, not re-run).

## Compensation gates on the complement, not a resource

**Decision (closing the open issue):** the named `Compensate` companion is the **live** compensation path —
`CompensateSubgraph` / `CompensateChoose` / `CompensateGather` / `CompensateWalkTree` are invoked on unwind, not bypassed
by an implicit closure. Making that work requires compensation to stop being resource-coupled.

**The latent bug.** The compensable gate in `RecoveryStack.Push` (`recovery_stack.go`) is:

```go
if receipt.Resource() != nil && receipt.Resource().RuntimeEnvironment() != nil && receipt.Complement() != nil {
```

It answers "is this compensable?" with "does it have a single `Resource`?" — and `invokeCompensateForReceipt` fetches the
env *through* that resource (`resource.RuntimeEnvironment()`). But "compensable" means "has undo state," i.e.
`Complement() != nil`, whatever its shape. A complement that is not a single resource's receipt is silently demoted to
audit-only and **never compensated**.

**`WalkTree` proves it is already real, outside flow.** `file.Provider.WalkTree` (`file/provider.go:710`) returns
`(product any, *op.RecoveryStack, err error)` — its `Reducer` accumulates each tree node's resources into that stack —
and declares `CompensateWalkTree(stack) → stack.Unwind()` (`:786`). Yet, dispatched as a node, its `*op.RecoveryStack`
complement takes `pushAuditReceipt`'s `PushNested` path and its own receipt is `&ReceiptBase{}` (no resource), so the
gate marks it audit-only and **`CompensateWalkTree` is dead code** — its compensation only works by accident, via the
nested auto-unwind. The same holds for `Subgraph` / `Choose` / `Gather`. And the instant `Gather` returns its
`[]*op.RecoveryStack` slice (the new signature), that slice is neither a `*Receipt` nor a `*RecoveryStack` the
`PushNested` path recognizes, so it would be `Commit`'d and **silently dropped**.

**The fix (base-`op` layer):**

- **Gate on `Complement() != nil`**, not `Resource() != nil`. Compensable = has undo state, of any shape (a resource
  action's receipt, a recovery stack, or a slice of stacks).
- **Supply the env from the executor**, not `receipt.Resource().RuntimeEnvironment()` — `WalkTree` and the combinators
  have no resource to read it from.
- **Route the compensate closure through the action's `Undo` companion** (resolved by action path via the registry), so
  it is re-derivable after a `Trace` load (the captured closures are transient).

**Trade-offs (for the record):** for single-stack producers (`Subgraph` / `Choose` / `WalkTree`) the companion is nearly
redundant with the auto-unwind — it unwinds the same child stack, just through a registry round-trip. The payoff is
`Gather`'s slice (which the generic auto-unwind cannot express), uniformity (resource actions, `WalkTree`, and
combinators all compensate the same way), and restorability (a registry-resolved companion survives save/load; a captured
closure does not). The cost is de-coupling compensation from `Resource` — which is the latent-bug fix, not incidental.

## The resource ledger across resume — save and reference by resource ID (settled 2026-06-24)

**Decision.** The `Trace` serializes the `ResourceCatalog` (the resource ledger) — **all generations, keyed by resource
id** — and the recovery stack references ledger entries **by id**; resume rebuilds the live catalog from the saved ledger
and resolves every receipt's references via `Lookup(id)`. This unifies two formerly-separate remaining items —
compensation-after-resume and "(c) catalog capture/restore" — into one ledger-centric mechanism. The live catalog must be
reconstructed on resume whatever the case; the fork was *how* — and saving it (not rebuilding from receipts) is what makes
resume full-fidelity.

**Why save the ledger, and why reference by id — shadowing.** A URI is not a unique identity: the catalog is an
append-only ledger, and `Shadow` (`resource_catalog.go:442`) re-catalogs an existing URI through `catalogLocked` (`:574`)
— on revival after `Gone`, a producer shadowing a prior discovery, or re-observation — minting a fresh id (`res-N`) per
generation. `byID` (id→index) distinguishes every generation; the URI→id namespace (`ns`, last-writer-wins) tracks only
the **current** one. `Discover(uri)`/`Current(uri)` (`:199`) therefore resolve a URI to the current generation only; a
superseded generation is reachable **solely** by `Lookup(id)` (`:328`). So if a receipt captured generation G1 of URI U
and U is shadowed to G2 before the pause, reconstructing that reference *by URI* yields G2 — the wrong resource. Only the
**id** pins G1. Rebuilding from receipts via `DiscoverResource(uri)` collapses shadows to the current generation; the
saved ledger holds every generation by id, so id references resolve to the exact one. (This corrects an earlier framing:
`file.Receipt.hydrate` does reconstruct `boundary`/`source` from their encoded URIs, so the discovered-not-produced case
alone did not force the ledger — **shadowing** does.)

**Shape.**

- **`Trace` gains the ledger — all generations, keyed by id.** A serialized `ResourceCatalog` rides alongside `Stack` and
  `Variables` — the single source of truth resume resolves against. Each entry serializes as `{id, uri, producerID,
  state}` plus `nextID`; on restore the URI rebuilds the `Resource` object, the id is restamped as identity, and
  `byID`/`ns`/`states` replay in append order (so the namespace's current-generation pointer is reproduced).
  (`ResourceCatalog` today is an append-only `entries []Resource` + `byID` + the URI→id namespace + `states` + `nextID`,
  with no `Marshal`/`Unmarshal`.)
- **Recovery-stack envelope = today's + compensation references, by id.** The current `receiptEnvelope`
  (`unit_id`/`action`/`result`/`status`/`*RecoveryStack` complement) gains, for resource receipts, `boundary` and
  `source` (resource **id** references) plus `recoveryID` and `recoveryDigest` (scalars) — never embedded resources.
  `result`/`boundary`/`source` resolve via `Lookup(id)` against the loaded ledger. The provider receipt encoding
  (`file.Receipt`'s `resource_uri`/`boundary_uri`/`source_uri`) shifts to id-based references.
- **Reconstruction at the op↔provider seam — one registry, not two.** Resource-from-URI rebuilds each ledger entry's
  `Resource` object from its URI, reusing the existing `AnnounceResource` constructor resolved by the typeID in the URI
  fragment. Receipts need **no** registry: their concrete type is read off the `Compensate` companion `op` already
  resolves by action, and each receipt rebuilds itself via `Receipt.RestoreEncoded(env, bytes)` — see [Receipt
  reconstruction on resume](#receipt-reconstruction-on-resume--no-registry-settled-2026-06-24).

**Resume reconstruction.** Load `Trace` → rebuild the live `ResourceCatalog` from the saved ledger (each generation's
`Resource` via its registered constructor keyed by URI typeID; id/producerID/state restamped; `byID`/`ns` replayed) →
descend the graph (pseudo replay) → per receipt, resolve `result`/`boundary`/`source` by id via `Lookup` and rebuild the
concrete receipt via `Receipt.RestoreEncoded` (type from the `Compensate` companion) → re-arm the `compensate` closure
(the closures are transient; the env-bound companion is re-derived at re-entry). A resumed-then-failed unwind resolves
`boundary`/`source` from the ledger, so the `Compensate*` companions roll back the pre-pause work. The recovery-site
archive (`.devlore/recovery/<recoveryID>`) lives on disk under the root, so a same-root resume finds the undo bytes by
`recoveryID` with no extra serialization.

**Economy.** Each generation is serialized once in the ledger and referenced by id; receipts carry id references, not
embedded resources.

## Receipt reconstruction on resume — no registry (settled 2026-06-24)

The ledger hands resume the resources by id; this is how a reloaded receipt becomes the **concrete** typed receipt
compensation needs — `CompensateMkdir(receipt *file.Receipt)` — with its `boundary`/`source`/`recovery` resolved against
the rehydrated ledger. Today a reloaded receipt is a bare `ReceiptBase` with a nil `Complement()`, so a
resumed-then-failed run cannot roll back. The fix needs **no new registry**.

**`Receipt.RestoreEncoded(*RuntimeEnvironment, []byte) error` — a method on the `Receipt` interface.** Every receipt is
encoded (the recovery-stack envelope is its encoded form), so restore-from-encoding is a universal receipt capability,
not a special one. `ReceiptBase` provides the default — restore the base fields
(`unit_id`/`action`/`result`/`status`/`transaction_id`) and the `*RecoveryStack` complement, **subsuming the
`unmarshalReceiptEnvelope` free function** so every receipt inherits it via embedding. `file.Receipt` overrides it: call
the base, then resolve its own `boundary`/`source`/`recovery` ids via `ResourceCatalog.Lookup`. It is essentially
today's `hydrate` with two edits — the env arrives as a parameter, and ids resolve via `Lookup(id)` instead of
`DiscoverResource(uri)`.

**The concrete type comes from the `Compensate` companion — why no `AnnounceReceipt`.** `op` cannot construct a
`*file.Receipt` (no provider import), and `json.Unmarshal` cannot decode into an interface, so it must `reflect.New` the
concrete type before decoding. That type is already in hand: `invokeCompensateForReceipt` resolves the `Compensate`
companion by the receipt's action (`ActionByPath`/`ActionByName`), and the companion's signature declares the concrete
receipt type (`CompensateMkdir(receipt *file.Receipt)`). So `op` reads the type off the resolved method (`method.undo`'s
last parameter), `reflect.New`s it, and calls `RestoreEncoded`. The action→provider→companion path compensation already
walks doubles as the type source — no parallel registry, no per-provider gen line.

**The companion's operand is the receipt — for one of three complement shapes.** `isLegalCompensableComplement` allows a
complement to be a `Receipt`, a `[]Receipt`, or a `*RecoveryStack`. (1) **Single `Receipt`** (`file.*`, `archive.Extract`,
`service.*`): the forward method returns `(result, receipt, error)` and the companion takes that receipt
(`CompensateMkdir(receipt *file.Receipt)`), so the receipt *is* its own complement and the type `op` reads off the
companion is the receipt type. B3 reconstructs exactly these — `MarshalYAML` emits the `receipt` sub-field only when the
complement is the receipt itself. (2) **`[]Receipt`** (`pkg.Install/Remove/Upgrade`, `CompensateInstall(state
[]*Receipt)`): not yet reconstructed on resume — it carries no `receipt` sub-field, so such a trace resumes without that
receipt's compensation (a follow-up) rather than failing. (3) **`*RecoveryStack`** (`file.WalkTree`,
`flow.Subgraph/Gather/Choose`): rides the `complement` field, restored by the `ReceiptBase` default, never reaching
`reconstructReceipt`.

**Env-as-parameter, not a setter.** `pkg/op` injects the runtime environment only as a constructor parameter
(`NewResourceBase(env, …)`, `NewResource(env, …)`, `DiscoverResource(env, …)`) — there is no env setter anywhere.
`RestoreEncoded(env, bytes)` extends that convention to the one op-driven reconstruction that today smuggles the env in
through a pre-seeded env-bearing resource (`file.Receipt.UnmarshalJSON`), replacing an implicit dependency with an
explicit one. The eight provider `Resource` `Unmarshal*` methods read the env off the receiver too, but they satisfy the
**standard** decoder interfaces (`json.Unmarshaler` / `encoding.TextUnmarshaler` / `yaml.Unmarshaler`), whose signatures
have no env slot — they are interface-locked to a constructor-pre-set env and stay as-is. The receipt is free to take
the env as a parameter precisely because `op` reconstructs it explicitly, not through a standard decoder.

**Flow.** Serialize — the recovery-stack envelope gains a `receipt` sub-field (the provider's id-based encoding) for
resource receipts; subgraph receipts keep emitting their `*RecoveryStack` complement. Resume — after the ledger
rehydrates, a re-arm pass walks the restored stack: for each entry carrying a `receipt` sub-field, take the concrete
type from the companion, `reflect.New`, `RestoreEncoded(env, bytes)`, swap the bare `ReceiptBase` for the concrete
receipt, reinstate its self-complement (the identity above — `Commit` set it on the produce path, the re-arm
re-establishes it on reconstruction, framework-level so no provider's `RestoreEncoded` carries it), and bind its
`compensate` closure. A resumed-then-failed unwind then rolls the pre-pause work back.

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
  survive the round-trip; promise/slot results (which live in the stack) do. **Resolved by Option B** (ledger saved in
  `Trace` keyed by resource id, receipts reference by id; settled 2026-06-24) — see [The resource ledger across
  resume](#the-resource-ledger-across-resume--save-and-reference-by-resource-id-settled-2026-06-24).

**Implementation status (2026-06-22):**

- **Built + committed:** `newChildExecutor`, pause flag, `Run` Paused-stamp, `Subgraph` kwargs-binding, `Trace()`,
  `SaveDefinition`/`LoadDefinition`, the checksum gate; **option (C) chained stacks** (`RecoveryStack` parent pointer +
  `ResultByUnitID` walking up, chain re-derived on load) — landed, tree re-greened; the **compensation gate** (gate on
  `Complement()`, env from the executor, companion-routed) and **failure→unwind wiring** (`Method.Invoke` carries the
  complement through a dispatch error; `invokeCompensateForReceipt` falls back to `RuntimeEnvironment.ActionByName`
  for a dotted action path) — both landed, `TestCompensation` green.
- **Implemented 2026-06-23 — (b) resume re-entry (in-process):** `Run` is state-driven (accepts `RunStatePaused`, keeps
  the restored `trace.Stack`, re-publishes `trace.Variables` onto the fresh env); `node.Execute` / `subgraph.Execute`
  carry the per-unit skip/adopt/fresh guard (`RecoveryStack.receiptByUnitID`); a re-entered subgraph adopts its restored
  child stack and supersedes the stale ErrPaused receipt; `newChildExecutor` takes the caller-built child stack. Green
  via `TestGraphPauseResume_ViaPublicAPI` (flat) and `TestGraphPauseResumeNested_ViaPublicAPI` (recursive adopt). `make
  test`: `pkg/op` + plan green, zero new failures.
- **Implemented 2026-06-23 — (b) save→load→resume serialize round-trip (rows 23–26):** the recovery stack owns a
  provider-agnostic execution-state envelope (`receiptEnvelope` — `unit_id`/`action`/`result`/`status`/`*RecoveryStack`
  complement; the Go-qualified `action_path` is not serialized — the `ActionByName` fallback resolves compensation from
  the dotted `action`), so a reloaded receipt restores what resume needs regardless of which provider produced it.
  `RecoveryStack.MarshalJSON`/`UnmarshalJSON` round-trip the tree; the adopt now keys on `Err() != nil` (serialize-safe,
  not `errors.Is(ErrPaused)`). Green via `TestGraphSaveLoadResume_ViaPublicAPI` (write `Trace` to disk → reload →
  resume completes, no re-dispatch); the in-process tests stay green. `make test`: `pkg/op` + plan green, zero new
  failures.
- **Implemented 2026-06-24 — Option B (B1 + B2): resource ledger save + rehydrate, id-keyed.** B1 —
  `ResourceCatalog.Snapshot()` projects every generation (`{id, uri, producerID, state}` + observation index + `nextID`)
  into a `ResourceLedgerSnapshot`; `Trace.Catalog` carries it; `GraphExecutor` snapshots it at pause (the env is torn
  down after Run) and `Trace()` emits it. B2 — `ResourceLedgerSnapshot.Rehydrate` rebuilds the live catalog
  id-preserving: each generation is reconstructed from its URI via `ExtractTagSpecific` +
  `receiverRegistry.ResourceConstructorByTypeID` + the provider constructor (interning disabled so `restoreEntry` stamps
  the saved id rather than minting one), and the resume branch installs it. Green via
  `TestResourceLedgerRehydrate_PreservesIDs` and the existing resume tests with rehydration active; `make test`:
  `pkg/op` + plan green, zero new failures.
- **Implemented 2026-06-25 — Option B (B3): compensation-after-resume.** `file.Receipt` encodes id references
  (`resource_id`/`boundary_id`/`source_id` + recovery key/digest) and reconstructs via `Receipt.RestoreEncoded(env,
  bytes)`, resolving them through `Lookup(id)`. `RestoreEncoded` is a method on the `Receipt` interface (the
  `ReceiptBase` default folds in the former `unmarshalReceiptEnvelope`; `file.Receipt` overrides it); the concrete type
  is read off the `Compensate` companion's parameter — no registry. The recovery-stack envelope gains a `receipt`
  sub-field; a resume-time `rearm` pass reconstructs concrete receipts, reinstates the self-complement (framework-level —
  a resource receipt is its own complement, set at `Commit` on the produce path), and binds compensation. Green via
  `TestGraphResumeThenFail_RollsBack_ViaPublicAPI` (pause → save → reload → resume → fail → the pre-pause `mkdir` rolls
  back); `make test`: `pkg/op` + plan green, zero new failures.
- **Committed 2026-06-25 — end-to-end lifecycle coverage (`lifecycle_e2e_test.go`).** `TestLifecycle_ViaGoAPI` drives
  the Go executor through run-to-completion, pause+resume, and fail+rollback; `TestLifecycle_ViaStarlark` builds,
  saves, loads, and runs via `plan.run` for run-to-completion and fail+rollback — a failure inside `plan.run`
  unwinds and compensates on the same `Run()` path, so rollback holds whether the run is launched from Go or Starlark.
  Pausing a live run is the eventing API (step 33), not a synchronous-script concern, so pause+resume is exercised
  only on the Go side. `make test`: `pkg/op` + plan green, zero new failures.
- **Implemented 2026-06-25 — format-neutral trace reconstruction (sub-step 10).** `Receipt.RestoreEncoded` consumes a
  decoded `ReceiptData` + a format-neutral `map[string]any` (no bytes); `RecoveryStack` gained `UnmarshalYAML` and a
  shared `fromEntries` (both codecs decode into `recoveryEntryData`); `file.Receipt` reconstructs from the decoded map;
  the retained `entry.restore` replaces the raw-bytes `entry.encoded`. Type discovery stays registry-free (action →
  `Compensate` companion), so the path is Protobuf-ready with no type-URL registry. Green via
  `TestGraphResumeThenFail_RollsBack_ViaPublicAPI` run through both JSON and YAML traces; `make test`: `pkg/op` + plan
  green, zero new failures. See the
  [Format-neutral trace reconstruction](#format-neutral-trace-reconstruction-sub-step-10) section.
- **Remaining for step 28 — all required to close it (next slices, not a later phase):** (1) cross-pause promise
  fidelity — a post-resume consumer retyping a pre-pause producer's reloaded (untyped) result via the Convert cascade;
  (2) Gather resume (N-dispatch); (3) replace `TestSubgraph_ReturnsRecoveryStack` (Starlark build/save/load/execute +
  fail/rollback landed via the e2e suite above; Starlark-driven pause/resume is the eventing API, step 33, not a
  synchronous-script variant); (4) `[]Receipt`-complement
  compensation-after-resume — `pkg.Install/Remove/Upgrade` receipts carry a slice of receipts as their complement, which
  B3 does not yet serialize or reconstruct (such a trace resumes without that receipt's compensation). **Option B
  (B1–B3) landed** for the single-`Receipt` and `*RecoveryStack` complement shapes — the ledger-in-`Trace` mechanism
  (serialize the `ResourceCatalog` by id, reference receipts by id, rehydrate + reconstruct concrete receipts on resume)
  closed compensation-after-resume and "(c) catalog capture/restore"; see the Implemented bullets above.

## Format-neutral trace reconstruction (sub-step 10)

**The mistake (corrected here, 2026-06-25).** B3's receipt reconstruction is JSON-byte-bound:
`Receipt.RestoreEncoded(env, []byte)` calls `json.Unmarshal`, and `RecoveryStack.UnmarshalJSON` retains raw JSON bytes
in `entry.encoded` for the resume-time `rearm`. That reconstructs only a JSON-decoded trace.

**The requirement it violates.** [`graph-signing.md`](../graph-signing.md) records that the two signable artifacts —
the graph and **its execution trace** — both "serialize to **three** formats (JSON, YAML, Protobuf)," and that verify
decodes "the file (**any format**) into the artifact … re-canonicalize[s] … verif[ies]" so that "**one signature
verifies in any format**." A trace whose receipts rebuild only through `json.Unmarshal` cannot be decoded from YAML or
Protobuf — so it cannot be re-canonicalized or verified. Format-neutral reconstruction is therefore a recorded
requirement, not a future option, and B3's JSON-binding is out of step with it.

**The principle.** The write side is already neutral — `MarshalYAML` builds a both-tagged `receiptEnvelope`,
`MarshalJSON` wraps it, and a Protobuf codec would marshal the same shape. The read side becomes its mirror:
reconstruction consumes a **decoded value**, not format-specific bytes; `UnmarshalJSON`, `UnmarshalYAML`, and a later
Protobuf decoder each decode to the neutral form and feed **one** reconstruction path — the inverse of the
`marshalData()` pattern the write side uses.

**Split format-coupled decode from neutral resolution.** `RestoreEncoded([]byte)` does two jobs today: decode the
receipt's fields (format-coupled) and resolve its id references against the rehydrated ledger via `Lookup` (neutral).
Separate them — the **codec** decodes the receipt's serializable fields (it owns the format, via struct tags on an
ordinary receipt struct), and the receipt keeps only a **format-neutral** post-decode hook that resolves ids → objects
(the part that needs the env/catalog). The `receipt` sub-field stops being an opaque blob and becomes a
normally-serializable structured value every codec round-trips.

**No registry — which is what makes Protobuf work.** Type discovery stays B3's: `action` →
`RuntimeEnvironment.ActionByName` → the `Compensate` companion's parameter type, depending only on the `action` string
every codec carries. So the **"no receipt registry" decision stands**, and is exactly what lets Protobuf reconstruct
without `google.protobuf.Any` — the concrete type comes from the action, not a type-URL registry. The
registry-vs-`Any` tension (flagged in [`21-graph-immutability.md`](../21-graph-immutability.md)) resolves in favor of
no registry.

**Scope.** Touches `Receipt.RestoreEncoded`'s shape (bytes → a decoded value plus a neutral id-resolution hook), the
`ReceiptBase` / `file.Receipt` overrides, `RecoveryStack.UnmarshalJSON`'s `entry.encoded` retention, and a new
`RecoveryStack.UnmarshalYAML`; the marshal side is unchanged. JSON and YAML reconstruct through the neutral path now
(subsuming the original "YAML trace deserialization"); the Protobuf path is proto-ready by construction
(type-from-action), with one detail — how the load-time-retained sub-field (no env or registry at load) reaches
rearm-time decode — settled in the implementation plan and tracked against the signing doc's canonicalization open
question. **Status: implemented 2026-06-25.** `Receipt.RestoreEncoded` now consumes a decoded `ReceiptData` plus a
format-neutral `map[string]any` (no bytes); `RecoveryStack` gained `UnmarshalYAML` and a shared `fromEntries` (both
codecs decode into `recoveryEntryData`); `file.Receipt` reconstructs from the decoded map; the retained `entry.restore`
replaces the raw-bytes `entry.encoded`. Type discovery stays registry-free (action → companion), so the path is
Protobuf-ready. `TestGraphResumeThenFail_RollsBack_ViaPublicAPI` runs the resume-then-fail rollback through both JSON
and YAML traces; `make test`: `pkg/op` + plan green, zero new failures.

## Resume re-entry — pseudo replay (settled 2026-06-22)

**Resume is side-effect-free.** Restarting a paused run re-dispatches *nothing* the `Trace` records — no provider
call, no compensation, no resource mutation. It is a **pseudo replay**: `Run` re-descends the graph from the root, and
the trace's receipts *dictate* how far it descends. Per unit, against the stack it is handed:

- **success receipt** (`Err()==nil`) → do nothing, return the receipt's `Result()`. For a subgraph this prunes the whole
  subtree — the descent never enters it.
- **incomplete receipt** (`ErrPaused`) or **no receipt** → the in-progress spine → descend / execute.

The walk only ever touches the unfinished spine down to the **frontier** (the first un-receipted unit), where real
execution resumes; completed subtrees are pruned by their receipts.

**Why a re-walk and not a literal jump to the frontier.** When the pause fired, `ErrPaused` propagated up and unwound
every `Execute` frame back to `Run`. The recovery stack is *data* (restorable); the call stack — the nested `Execute`
calls, the `walkSubgraphChildren` loop position, the per-subgraph variable frames — is *control flow* that no longer
exists. The trace restores *what is done*, not *where we were*, so resume rebuilds the position by re-descending. A
literal jump would require persisting the active frames and replacing recursion with a resumable work-list — the larger
(Y) rewrite — a possible future shape; (X) above delivers full step-28 function without it.

**"Do nothing" is not literally passive — the descent does two side-effect-free things:**

1. **Adopt the restored child stack on descent.** Re-entering an in-progress subgraph, its child executor must walk the
   subgraph's **restored** child stack (the one on its incomplete receipt's complement), *not* a fresh
   `newRecoveryStack`. This is the one piece option (C) forces: option (C) mints a fresh per-dispatch stack, so without
   adoption the descent would walk an empty stack and re-run the completed children. Adoption is not extra restoration —
   it is *using* the tree already restored, all the way down — and it makes the completed children present so the
   frontier children skip-detect and resolve their slots.
2. **Re-resolve the per-subgraph variable frames as we descend.** The frames (`flow.Subgraph`'s kwargs→frame) are the
   one bit of execution state the `Trace` does not carry, so the descent recomputes them — pure computation against the
   restored stacks (`ResultByUnitID` resolves because the stacks are restored). No dispatch, no side effects; just
   rebuilding the frame so the frontier children resolve their slots.

**The per-unit guard (the whole behavioral change):**

- `node.Execute`: own success receipt on the stack → return its result; else dispatch.
- `subgraph.Execute`: own success receipt → return result (prune); own incomplete receipt → adopt its complement as the
  child stack and descend to the frontier; no receipt → fresh.
- **Pause-receipt supersession:** when a resuming in-progress subgraph completes, it supersedes its prior `ErrPaused`
  receipt on the parent stack rather than leaving a stale duplicate.

**Scope — increment (X).** `Run`'s preamble becomes state-driven (accept `RunStatePaused`, keep `trace.Stack`, use
`trace.Variables`); the per-unit guard above; the supersession rule. No replay-map, no work-list rewrite, no frame
persistence — the recursive dispatch model stays, and the trace dictates how far it descends. Promise/slot results ride
the stack, so promise-based graphs resume fully on (b) alone; catalog-mediated URI sharing of pre-pause resources still
needs (c).

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
