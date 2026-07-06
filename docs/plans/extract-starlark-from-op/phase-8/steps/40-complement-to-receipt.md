---
step: 40
title: "Eliminate \"complement\" — the compensation datum is a receipt, everywhere"
status: not-started — chartered 2026-07-04 (terminology settled; execution pending)
proof_run: n/a (charter)
parent: ../../phase-8.md
---

# Step 40 — Eliminate "complement"; the compensation datum is a receipt

**Status:** `not-started` (charter). The terminology is settled (2026-07-04); this step executes the purge.

## The settled vocabulary

A provider exposes **compensable actions**. Each action returns a **result** and a **receipt** (e.g. `file.Receipt`,
implementing `op.Receipt`). To undo an action, the engine calls its **Compensate** method, passing the receipt. The
overall undo mechanism is **compensation**, and a workflow whose compensation fails terminates in
`FailedCompensation`.

Every term is either from the compensation family or the receipt metaphor, and the two families reinforce each
other. The one-line doc comment that locks it in, to be placed on the `op.Receipt` interface:

> A Receipt is the evidence returned by a compensable action, sufficient for Compensate to counteract that action's
> effects.

After that, "complement" is deleted. (The word earned its removal the same way "stranded" did: future readers should
never have to wonder what it meant.)

## Footprint (measured 2026-07-04)

**Go: ~190 occurrences across 19 files** (top: `recovery_stack.go` 41, `method.go` 37, `receipt.go` 22,
`action_types.go` 16, `graph_executor.go` 9). **Docs: ~150 occurrences** (top: step-31 doc 62, architecture 2.2 (29),
2.3 (28), 5.1 (8)).

The exported / structural surface the rename must redesign, not just re-spell:

| Site | Today | Decision at execution |
|---|---|---|
| `op.Complement` (`action.go:46`) | `type Complement = any` alias; third leg of the sealed `Action.Do` return `(Result, Complement, error)` | dissolve or rename — the leg IS the receipt |
| `Receipt.Complement()` / `ReceiptBase.complement` (`receipt.go:222`) | the receipt's accessor for its own undo payload (itself for provider receipts; the child `*RecoveryStack` for subgraph receipts) | the core naming decision: under receipt-as-the-datum, what does a receipt "carry"? (A subgraph's receipt carries its child stack; a provider's receipt carries itself.) |
| `Method.Undo(activation, receiver, complement any)` (`method.go:479`) | the parameter handed to `Compensate<Name>` | `receipt` |
| `isLegalCompensableComplement` (`method.go:802`) | validates the compensable method's second return (`*Receipt`-implementing pointer or `*RecoveryStack`) | rename (e.g. `isLegalReceiptType`); doc comments describe "the receipt the action returns" |
| `complementOrNil` (`action_types.go:266`) | typed-nil unwrap for the Do return | rename with the leg |
| `receiptEnvelope.Complement` (`recovery_stack.go:580/:736`) | **a serialized document field** — the rename touches the trace document format (greenfield: no legacy documents to support) | rename the key with the field |
| `pushAuditReceipt(…, complement any, …)` (`graph_executor.go`) | the executor's plumbing of the Do return into the pushed receipt | rename with the leg |

Plus prose: every doc comment in `pkg/op` and the providers (`flow`, `pkg`, `archive`, `git`), the test files, and
the ~150 doc occurrences across `docs/architecture/*` and the phase plans.

## Sequencing note

The step-31 doc (subgraph executor ownership) carries 62 "complement" occurrences and reworks the same
recovery-stack machinery this step renames. Execute this step **after step 31 lands** (or as its immediate
follow-on) so the rename sweeps the reworked code once instead of racing it.

## Verification

1. `grep -rci "complement"` over `pkg`, `cmd`, `internal`, `docs` → zero (the only tolerated survivors would be
   genuine English uses unrelated to compensation, expected: none).
2. `make test` green modulo the step-18 gate set; the trace save/load/resume suites prove the renamed document
   field round-trips.
3. The `op.Receipt` interface carries the lock sentence verbatim.
