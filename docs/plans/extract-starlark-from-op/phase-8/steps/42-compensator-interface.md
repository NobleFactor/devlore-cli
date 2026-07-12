---
step: 42
title: "The Compensator interface — unify receipts and recovery stacks"
status: in-progress — slice 1 (type flip) committed; slice 2a (dissolve the compensation closure) done, pending commit; slice 2b (collapse recoveryEntry + narrow isLegalCompensator) + serialization + verify remain
proof_run: n/a (charter)
parent: ../../phase-8.md
---

# Step 42 — The `Compensator` interface; unify the two forms

**Status:** `in-progress` — slice 1 (the type flip) committed; slice 2a (dissolving the compensation closure) landed
green, pending commit; slice 2b (the `recoveryEntry` collapse) + serialization + verify remain. Split from
[step 40](40-complement-to-receipt.md) (the terminology purge) on 2026-07-11 so the two changes land separately. This step **depends on step 40** — it operates on the *compensator* vocabulary, not
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
   - **2a — dissolve the compensation closure. ✅ Done — pending commit.** `Unwind` dispatches through the entry's
     `Compensator` (`recoveryEntry.compensator()` → `Compensate(env)`) instead of the pre-bound
     `recoveryEntry.compensate` closure; the closure field, `Push`/`PushNested`'s binding, and `rearm`'s re-binding are
     gone. Behavior-identical — the compensation + trace-resume suites pass; a test helper gained a `recordingReceipt`
     fake to observe `Unwind` without a real compensating action.
   - **2b — collapse `recoveryEntry` + narrow `isLegalCompensator`.** Merge `receipt` / `recoveryStack` into one
     `Compensator` field (the structural traversals — `Receipts`, `receiptByUnitID`, `NestedStackByUnitID`,
     `ResultByUnitID`, `MarshalYAML`, `rearm` — type-assert the concrete form they need); `isLegalCompensator` narrows
     to "is a `Compensator`"; the subgraph path pushes its **stamped** child stack directly (no wrapping `ReceiptBase`).
3. **Serialization.** The `receiptEnvelope` compensator field becomes a `kind`-tagged recursive tree
   (`kind: receipt` | `kind: stack`), reconstructed polymorphically at the one deserialize boundary. Greenfield — no
   legacy traces. Prove the recursive round-trip via the trace save / load / resume suites.
4. **Verify.** `make test` green (modulo the step-18 gate); `make vet` clean; the trace suites prove the tree
   round-trips; no `x.(*RecoveryStack)` / `x.(Receipt)` switch remains in the recovery recursion.

## Deferred (enabled by this shape, out of scope)

- **Black-box scope receipt** — let a committed composite optionally carry its own single receipt that overrides
  recursing into children (BPMN/WS-BPEL/1991 bound compensation).
- **Parallel compensation** — a tree with per-node ordering (Gather's parallel children compensate concurrently)
  refines the strict-LIFO, which is a safe conservative default today.

## Verification

1. No `x.(*RecoveryStack)` / `x.(Receipt)` type switch remains on the recovery recursion path; `op.Compensator` is an
   interface with the single `Compensate` method; `Receipt` embeds it; `*RecoveryStack` satisfies it and not `Receipt`.
2. `make test` green modulo the step-18 gate set; the trace save/load/resume suites prove the `kind`-tagged tree
   round-trips.
3. `op.Receipt` carries the lock sentence (from step 40); `make vet` clean.
