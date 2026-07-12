---
step: 42
title: "The Compensator interface — unify receipts and recovery stacks"
status: in-progress — slices 1, 2a, 2b committed; slice 3 rescoped + design approved 2026-07-12 (uniform recursive serialization + subgraph-direct-push + receipt-owned encoding; sample trace reviewed; encoding edge cases firm during implementation + verify), pending implementation
proof_run: n/a (charter)
parent: ../../phase-8.md
---

# Step 42 — The `Compensator` interface; unify the two forms

**Status:** `in-progress` — slices 1, 2a, 2b committed. Slice 3 rescoped 2026-07-12 (from a `kind`-tagged tree) to
**uniform recursive serialization + subgraph-direct-push + receipt-owned encoding** — design **approved** (sample trace
reviewed; see the Execution plan + [5.2-recovery-serialization.md](../../../../architecture/5.2-recovery-serialization.md)),
with encoding edge cases to firm during implementation + the verify gate. Pending implementation. Split from [step 40](40-complement-to-receipt.md) (the terminology purge) on 2026-07-11 so the two
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
   **✅ Done — pending commit.** `type Compensator = any` → `interface { Compensate(runtimeEnvironment *RuntimeEnvironment) error }`;
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
   - **2b — collapse `recoveryEntry` to one `Compensator` field. ✅ Done — pending commit.** The compensation descent is already handled
     polymorphically by `RecoveryStack.Compensate` (= `Unwind`), so the compensation path needs no further change; the
     collapse is about the **non-compensation** traversals — `Receipts`, `receiptByUnitID`, `NestedStackByUnitID`,
     `ResultByUnitID`, `MarshalYAML`, `rearm` — which walk the tree to collect receipts / results and type-assert the
     concrete form (`Receipt` / `*RecoveryStack`) they need. Merge `receipt` / `recoveryStack` into one
     `compensator Compensator`; keep the audit-only skip-gate inside `recoveryEntry.compensator()`. It is a modest
     single-field simplification, not a "the type switches vanish" win (the audit traversals still branch, via
     type-assert). **Kept out of 2b (separable, not required by the collapse) — see Deferred:** the `isLegalCompensator`
     narrowing and subgraph-direct-push.
3. **Uniform recursive serialization + subgraph-direct-push + receipt-owned encoding** (settled 2026-07-12). No `kind`
   tag, no stack-owned envelope, no special cases. The trace *is* the root `RecoveryStack`; every stack serializes
   identically — its stamp (`unit_id`/`result`/…) + `entries`. Each entry's compensator serializes itself: a
   `*RecoveryStack` recurses (the same method); a receipt encodes via `ReceiptBase` (the common resume state, so no
   provider can drop it) + the concrete type's own fields (its id-refs). Decode discriminates **structurally** — a node
   with `entries` is a stack, else a receipt — and resolves the *concrete* receipt type from `compensating_action`
   (`CompensatingActionByName` → `compensatorType`), then rebuilds via `Receipt.RestoreEncoded`. This pulls
   **subgraph-direct-push** in (formerly Deferred): a subgraph is pushed as its stamped stack, not wrapped in a
   `ReceiptBase`, so it *is* a `RecoveryStack` — it loses the `flow.subgraph` wrapper + companion and rolls back via
   `Unwind` (a compensation behavior change). And it retires the stack-owned `receiptEnvelope` for
   `ReceiptBase.MarshalJSON` / `RestoreEncoded` + concrete overrides. Full design + sample trace in
   [5.2-recovery-serialization.md](../../../../architecture/5.2-recovery-serialization.md) § Uniform recovery-stack
   serialization. Greenfield — no legacy traces. Prove the round-trip via the trace save / load / resume suites.
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
