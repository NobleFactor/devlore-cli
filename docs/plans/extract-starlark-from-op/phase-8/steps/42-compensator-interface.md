---
step: 42
title: "The Compensator interface — unify receipts and recovery stacks"
status: in-progress — slices 1/2a/2b/2c/3a committed; 3b (receipt-owned encoding + structural reader) done pending commit; verify (slice 4) pending
proof_run: n/a (charter)
parent: ../../phase-8.md
---

# Step 42 — The `Compensator` interface; unify the two forms

**Status:** `in-progress` — slices 1/2a/2b/2c/3a committed. Slice 3 rescoped 2026-07-12 (from a `kind`-tagged tree) to
**uniform recursive serialization + subgraph-direct-push + receipt-owned encoding** — design **approved** (sample trace
reviewed; see the Execution plan + [5.2-recovery-serialization.md](../../../../architecture/5.2-recovery-serialization.md)),
with encoding edge cases to firm during implementation + the verify gate. 3a + 3b landed (pending commit); verify pending. Split from [step 40](40-complement-to-receipt.md) (the terminology purge) on 2026-07-11 so the two
changes land separately. This step **depends on step 40** — it operates on the *compensator* vocabulary, not
"complement." The shape and its prior-art grounding are settled: see
[step 40 § The target shape](40-complement-to-receipt.md#the-target-shape--the-compensator-interface-step-42-not-this-step)
and [`2.2-phase-execution.md`](../../../../architecture/2.2-phase-execution.md) § Prior art.

## What this step does

After step 40, a compensator is `op.Compensator` (= `any`) behind a `Receipt`-vs-`*RecoveryStack` type switch — the
vocabulary is settled but there are still two concrete types and a switch. This step makes it **one narrow interface**:

```go
// Compensator reverses its own effects — the Composite "component" of the recovery tree. A leaf receipt compensates by
// invoking its compensating action; a *RecoveryStack compensates by unwinding its children LIFO. It is the only
// operation that recurses.
type Compensator interface {
	Compensate(runtimeEnvironment *RuntimeEnvironment) error
}

type Receipt interface {
	Compensator
	// ... the leaf-only audit / undo surface (CompensatingAction, TransactionID, Result, Resource, Slots, …)
}
// *RecoveryStack implements Compensator only — never the full Receipt; Compensate == its existing Unwind (LIFO).
```

The composite must **not** implement the full `Receipt`: a recovery stack has no single `TransactionID`,
`ForwardAction`, or `Result`; forcing those onto it is a Refused Bequest / LSP violation, and our own `Stamp` ruling
(`recovery_stack.go`) already said a stack needs "identity, not a named compensator." Full rationale + the prior-art
grounding (Garcia-Molina nested sagas, cCSP, BPMN/WS-BPEL, distributed-saga logs, GoF Composite) are in the two
references above.

## Execution plan

Structural core, the highest-risk `pkg/op` rework; each phase is gofmt-clean with `make test` green (modulo the
standing step-18 gate set) before the next; commit per phase.

1. **The type flip — interface + both `Compensate` methods + `Receipt` embeds `Compensator` + retype the `any` sites.**
   **✅ Committed.** `type Compensator = any` → `interface { Compensate(runtimeEnvironment *RuntimeEnvironment) error }`;
   `*RecoveryStack.Compensate` = `Unwind` under the shared name; `ReceiptBase.Compensate` = a one-line delegation to
   `invokeCompensateForReceipt` (mechanism below); `Receipt` embeds `Compensator`; the `any` compensator sites (`Commit` /
   `Method.Undo` / `pushAuditReceipt` params, the `ReceiptBase.compensator` field, the `Compensator()` accessor + return)
   retyped to `Compensator`; `compensatorOrNil` keeps the typed-nil guard; the serialization seam bridged (decoded
   envelope `any` → `Compensator` via a comma-ok assertion). **No behavior change** — `Unwind` still drives the pre-bound
   closure and the type switches stay. Build + `pkg/op` + providers + `cmd/lore` green; `make vet` + gofmt clean; standing
   FAIL set unchanged.

   **Concrete-type mechanism — resolved 2026-07-11.** No new machinery is needed. The concrete receipt is already
   captured as a *value* at `Commit` (`b.compensator = compensator`, `receipt.go`), where a compensable forward method
   returns its receipt as its own compensator — `recovery_stack.go` states it outright: "a resource receipt is its own
   compensator … Commit stores that self-reference." So `ReceiptBase.Compensate(env)` is a one-line delegation to the
   existing resolve-and-invoke helper (`invokeCompensateForReceipt`), which reads `CompensatingAction()` (the name) and
   `Compensator()` (the concrete artifact — the `*file.Receipt` for a leaf, the `*RecoveryStack` for a composite), both
   interface accessors, and reflects the provider companion with that value. **The concrete type travels as a value via
   `Compensator()`; it is never recovered from the method receiver** — so there is no self-pointer to *add* (it is
   already `b.compensator`), no cast, no generics. The precondition — **keep `Compensator()` and the `compensator`
   field** — held; the type flip retyped them to `Compensator` but kept them.
2. **Dispatch rewire (behavior-preserving), in two commits:**
   - **2a — dissolve the compensation closure. ✅ Committed.** `Unwind` dispatches through the entry's `Compensator`
     (`recoveryEntry.compensator()` → `Compensate(env)`) instead of the pre-bound `recoveryEntry.compensate` closure;
     the closure field, `Push`/`PushNested`'s binding, and `rearm`'s re-binding are gone. Behavior-identical — the
     compensation + trace-resume suites pass; a test helper gained a `recordingReceipt` fake to observe `Unwind` without
     a real compensating action.
   - **2b — collapse `recoveryEntry` to one `Compensator` field. ✅ Committed.** The compensation descent is already handled
     polymorphically by `RecoveryStack.Compensate` (= `Unwind`), so the compensation path needs no further change; the
     collapse is about the **non-compensation** traversals — `Receipts`, `receiptByUnitID`, `NestedStackByUnitID`,
     `ResultByUnitID`, `MarshalYAML`, `rearm` — which walk the tree to collect receipts / results and type-assert the
     concrete form (`Receipt` / `*RecoveryStack`) they need. Merge `receipt` / `recoveryStack` into one
     `compensator Compensator`; keep the audit-only skip-gate inside `recoveryEntry.compensator()`. It is a modest
     single-field simplification, not a "the type switches vanish" win (the audit traversals still branch, via
     type-assert). **Kept out of 2b (separable, not required by the collapse):** the `isLegalCompensator` narrowing (see
     Deferred); subgraph-direct-push (became slice 3a); the choose `guard` leak (became slice 2c, below).
   - **2c — remove the choose-specific `guard` leak. ✅ Committed.** 2b left `recoveryEntry` as
     `{compensator, restore, guard}`; the `guard GuardResult` (plus `RecoveryStack.SetGuard` / `GuardByUnitID` and the
     entry's serialized `guard`) recorded a Choose decision node's outcome — a specific unit's semantics leaking onto the
     generic recovery entry. Removed: a decision node's branch is a pure function of its result's truthiness, and flow's
     `branch` already receives that result (live on the first run, round-tripped on resume), so it re-derives
     `op.IsTruthy(result)` every time — trivial, never stored (the round-trip re-derivation replaces the "evaluate once,
     never re-derive" rule, accepted 2026-07-12). `recoveryEntry` collapses to `{compensator, restore}` — zero
     unit-specific knowledge; the recovery tree carries only results + compensators. The `GuardResult` type stays (it is
     `Edge.Guard`, the graph topology). The alternative — parking the outcome on the choose compensator — only relocates
     the leak onto `RecoveryStack`, so it was rejected. Build + all suites green incl. the end-to-end devloretest
     choose/wait_until path.
3. **Uniform recursive serialization** (design approved 2026-07-12; sample trace + spec in
   [5.2-recovery-serialization.md](../../../../architecture/5.2-recovery-serialization.md) § Uniform recovery-stack
   serialization) — in two sub-slices:
   - **3a — subgraph-direct-push. ✅ Done.** Every combinator returns its child `*RecoveryStack` as its compensator.
     `pushAuditReceipt` used to wrap that stack in a `ReceiptBase` (`compensating_action: flow.subgraph`); 3a stamps
     and nests it directly instead: (i) `pushAuditReceipt` `Stamp`s the `*RecoveryStack` compensator and `PushNested`s
     it directly, not wrapped; (ii) `subgraph.Execute` adopts the restored stack via `NestedStackByUnitID(s.ID())`;
     plus two lookups taught to read stamped stacks — `ResultByUnitID` (a combinator's result now resolves for
     downstream promises; it only checked receipts, so choose/wait_until came back nil, caught by the end-to-end
     devloretest suite) and `supersede`. Behavior deltas held: a combinator's audit loses `forward_action`/`slots` (the
     stamp carries `unit_id`/`result`/`status`), `Trace.Summarize` stops tallying combinators as their own action — no
     test regressed. Build + all suites green.
   - **3b — receipt-owned encoding + structural-discriminator reader. ✅ Done — pending commit.** The stack-owned
     `op.receiptEnvelope` is gone: `RecoveryStack.MarshalYAML` emits each entry's compensator directly (a `*RecoveryStack`
     recurses; a receipt via its own `MarshalJSON`), and the decoder discriminates **structurally** — an entry with
     `entries` is a stack, else a receipt whose base decodes into `ReceiptData` and whose whole flat object is retained
     for `rearm`'s `reconstructReceipt` (`compensating_action` → `compensatorType` → `RestoreEncoded`). The `file`/`pkg`/
     `service` receipts embed `op.ReceiptData` in their `MarshalYAML` (base + concrete id-refs, flat); `encryption`/`git`
     inherit `op.ReceiptBase.MarshalYAML`. Per the settled decisions: `file` drops `resource_uri` for `resource_id`, the
     base owns `transaction_id`, and the self-referential compensator is never serialized (it would recurse forever). All
     trace save/load/resume suites + every provider green.
     - **Deferred (charter separately):** `service`/`encryption`/`git` reconstruct through the stack via the bare
       `op.ReceiptBase.RestoreEncoded` (no resource resolved) — they override the older custom `UnmarshalJSON`/`hydrate`
       path (stack-unused) rather than `RestoreEncoded`, and are untested. Their 3b **encode** is aligned + non-regressing,
       but moving them fully onto the `RestoreEncoded` path (like `file`/`pkg`) is a follow-up. (`encryption`/`git` were
       already far behind before this step.)
4. **Verify.** `make test` green (modulo the step-18 gate); `make vet` clean; the trace suites prove the tree
   round-trips; no `x.(*RecoveryStack)` / `x.(Receipt)` switch remains in the recovery recursion.

## Deferred (enabled by this shape, out of scope)

- **Black-box scope receipt** — let a committed composite optionally carry its own single receipt that overrides
  recursing into children (BPMN/WS-BPEL/1991 bound compensation).
- **Parallel compensation** — a tree with per-node ordering (Gather's parallel children compensate concurrently)
  refines the strict-LIFO, which is a safe conservative default today.
- **`isLegalCompensator` narrowing** — narrowing the Do-return validation from "`Receipt`-pointer or `*RecoveryStack`"
  to "implements `Compensator`" is a real *widening* (it would admit a third `Compensator` form that `pushAuditReceipt`
  can't place — it wraps a non-`Receipt` in a bare `ReceiptBase` that would misfire at unwind), not an equivalence.
  Revisit only alongside making `pushAuditReceipt` handle an arbitrary `Compensator`.
  (**Subgraph-direct-push** was here; pulled into slice 3 on 2026-07-12 — the uniform serialization requires a subgraph
  to *be* a `RecoveryStack`, not a wrapped receipt.)

## Verification

1. No `x.(*RecoveryStack)` / `x.(Receipt)` type switch remains on the recovery recursion path; `op.Compensator` is an
   interface with the single `Compensate` method; `Receipt` embeds it; `*RecoveryStack` satisfies it and not `Receipt`.
2. `make test` green modulo the step-18 gate set; the trace save/load/resume suites prove the `kind`-tagged tree
   round-trips.
3. `op.Receipt` carries the lock sentence (from step 40); `make vet` clean.
