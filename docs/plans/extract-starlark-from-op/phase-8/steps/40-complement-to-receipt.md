---
step: 40
title: "Eliminate \"complement\" — the compensation datum is a receipt, everywhere"
status: in-progress — phases 1 + 3 (compensator rename + Go sweep) committed 2026-07-11; phase 2 (compensating-action cluster) implemented & green, pending commit; phases 4 (doc sweep) + 5 (lock + verify) remain
proof_run: n/a (charter)
parent: ../../phase-8.md
---

# Step 40 — Eliminate "complement"; the compensation datum is a receipt

**Status:** `in-progress`. The terminology is settled (2026-07-04) and the **shape is settled** (2026-07-11, below).
Phase 1 (the `complement` → `compensator` surface rename) and phase 3 (the mechanical Go sweep) were **committed
2026-07-11** — zero "complement" in `.go`. Phase 2 (the compensating-action cluster) is **implemented and green,
pending commit** — `type compensator` → `compensatingAction`, `CompensatorByName` → `CompensatingActionByName`, the
`compensatingActionIndex`/`Once` fields, and `invokeCompensator` → `invokeCompensatingAction` renamed, with the mislabeled
local var at the resolve site fixed; `compensatorType` and the `compensator` artifact params/fields kept; build, tests,
and vet green. Phases 4 (the doc sweep) and 5 (lock + verify) remain.

## The settled vocabulary

A provider exposes **compensable actions**. Each action returns a **result** and a **compensator** — the evidence
sufficient to reverse it. A compensator has **two forms**:

- a **receipt** — a *leaf* compensator, one action's evidence (e.g. `file.Receipt`, implementing `op.Receipt`);
- a **recovery stack** — a *composite* compensator, a subgraph's child stack, which reverses by unwinding its
  children LIFO.

To undo a leaf, the engine invokes the receipt's **compensating action** — the named `Compensate<Name>` provider
method. The overall mechanism is **compensation**, and a workflow whose compensation fails terminates in
`FailedCompensation`. (That receipts and recovery stacks are *both* compensators is the model the prior art grounds and
the interface refactor makes structural — see the split below; today they remain two concrete types behind a type
switch.)

Every term is from the compensation family or the receipt metaphor. Two crisp, distinct terms replace the overloaded
"complement":

- **compensator** — the reversal artifact (a receipt or a recovery stack); this is what "complement" *was*.
- **compensating action** — the named `Compensate<Name>` method a receipt's undo invokes; this is what the internal
  `invokeCompensator` prose *was* (renamed `invokeCompensatingAction`).

The lock sentence, to be placed on the `op.Receipt` interface:

> A Receipt is the evidence returned by a compensable action, sufficient for Compensate to counteract that action's
> effects.

After that, "complement" is deleted. (The word earned its removal the same way "stranded" did: future readers should
never have to wonder what it meant.)

## The target shape — the `Compensator` interface (step 42, not this step)

**This describes the end state that [step 42](42-compensator-interface.md) implements — not step 40.** Step 40 (the
Execution plan below) only renames the vocabulary in the *current* structure. **The authoritative, updated shape lives
in [step 42](42-compensator-interface.md)** — this preview predates phase 1's rename (so it still shows the old
`Complement` spellings) and the 2026-07-11 resolution of the concrete-type mechanism (the `Compensator()` accessor +
the `compensator` self-reference field are **kept**, not dissolved — they carry the concrete artifact a leaf's
`Compensate` hands to its compensating action; see step 42). The structural unification here is kept
for continuity and because settling it fixed the naming; the prior-art grounding lives in
[`2.2-phase-execution.md`](../../../../architecture/2.2-phase-execution.md).

Deep prior-art research — nested-saga theory (Garcia-Molina et al. 1991's "tree of nested sagas"; the Sagas/cCSP
compensation calculi), BPMN 2.0 / WS-BPEL scope compensation, distributed-saga logs (the MassTransit routing slip's
`CompensateLog` is the closest production analogue to our receipt), and the GoF Composite + macro-command undo lineage —
plus our own `Stamp` ruling converge on one shape.

**The recovery structure IS a tree of receipts** — a leaf is one action's receipt; a composite node is a subgraph's
child stack; compensation recurses reverse-order (LIFO), scope-confined. This is not new structure: `RecoveryStack` is
already chained into a tree, `recoveryEntry` is already "either a receipt or a nested stack," and the trace serializes
recursively. Owning nested scopes, the tree is the *faithful* model (the flat distributed-saga log would lose the
scope-confinement BPMN treats as essential and we already enforce via `CompensateSubgraph`).

**But the composite must NOT implement the full `Receipt`.** A `*RecoveryStack` has no single `TransactionID`,
`ForwardAction`, `Result`, or leaf payload — forcing those accessors onto it is a Refused Bequest / LSP violation ("the
bigger the interface, the weaker the abstraction"). Our `Stamp` doc already ruled this: a stack needs "identity, not a
named compensator." So the two share a **narrow interface**, and the leaf extends it:

```go
// Compensator is a unit that reverses its own effects — the Composite "component" of the recovery tree. A leaf Receipt
// compensates by invoking its compensating action; a *RecoveryStack compensates by unwinding its children LIFO. It is
// the only operation that recurses.
type Compensator interface {
	Compensate(runtimeEnvironment *RuntimeEnvironment) error
}

type Receipt interface {
	Compensator
	TransactionID() string
	ForwardAction() string
	// ... the leaf-only audit / undo surface (CompensatingAction, Result, Resource, Slots, Attempts, …)
}

// *RecoveryStack implements Compensator only — never Receipt. Compensate == its existing Unwind (LIFO).
```

Consequences:

- `op.Complement` (the `Action.Do` third leg + the `Undo` param) is typed **`Compensator`** (nil for non-compensable
  actions), not `any`; the `x.(*RecoveryStack)` vs `x.(Receipt)` type-switches collapse into one `Compensate()` call.
- `Receipt.Complement()` / `ReceiptBase.complement` **dissolve**: a subgraph's child stack — stamped with its audit
  subset (the existing `Stamp`) — is a `Compensator` pushed **directly**, not wrapped in a `ReceiptBase`.
- Serialization stays a recursive **data** tree with a `kind` discriminator (`receipt` | `stack`); the compensator stays
  a **name** resolved at undo time (our compensating-action index already does this — closures don't serialize).
- `invokeCompensator` → `invokeCompensatingAction`, so **`Compensator`** (the artifact — receipt or stack) and
  **compensating action** (the named `Compensate<Name>` method a leaf invokes) are two crisp, distinct terms.
- Naming rationale: `Compensator` is the agent-noun of the one method `Compensate` (Effective Go's `Reader`/`Closer`
  idiom), so an adjective (`Compensable`/`Reversible`) is wrong; it stays inside the one vocabulary the charter locks
  (compensation family + receipt metaphor).

**Deferred (out of step-40 scope, but enabled by this shape):** (1) a *black-box scope receipt* — letting a committed
composite optionally carry its own single receipt that overrides recursing into children (BPMN/BPEL/1991 bound
compensation); (2) *parallel compensation* — a tree with per-node ordering (Gather's parallel children compensate
concurrently) refines our strict-LIFO, which is a safe conservative default today. Neither is required to purge
"complement."

## Footprint (measured 2026-07-04)

**Go: ~190 occurrences across 19 files** (top: `recovery_stack.go` 41, `method.go` 37, `receipt.go` 22,
`action_types.go` 16, `graph_executor.go` 9). **Docs: ~150 occurrences** (top: step-31 doc 62, architecture 2.2 (29),
2.3 (28), 5.1 (8)).

The exported / structural surface (the "Decision at execution" column records the *original* open question): under the
split, the **renames** land in step 40's purge and the **structural dissolutions** in [step 42](42-compensator-interface.md).

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

## Execution plan — terminology purge (no structural change)

Step 40 renames "complement" to the settled vocabulary **in the current structure**: a compensator stays `any`-typed
behind the existing `Receipt`-vs-`*RecoveryStack` type switch; the `ReceiptBase`-wraps-a-compensator shape is untouched.
The structural unification (one `Compensator` interface, dissolving the switch) is [step 42](42-compensator-interface.md).
Each phase is gofmt-clean with `make test` green (modulo the standing step-18 gate set) before the next; commit per
phase.

1. **Rename the compensator surface** (a re-spell, not a re-type). **✅ Done — committed 2026-07-11.** `op.Complement` (`= any`) → `op.Compensator`
   (`= any`); `Receipt.Complement()` → `Compensator()`; `ReceiptBase.complement` field → `compensator`; the
   `complement` params on `Commit` / `Method.Undo` / `pushAuditReceipt` → `compensator`; `complementOrNil` →
   `compensatorOrNil`; `isLegalCompensableComplement` → `isLegalCompensator`; the `receiptEnvelope.Complement`
   **serialized document key** → `Compensator` (JSON/YAML `compensator`) — greenfield, no legacy traces to support. The
   two forms (a receipt, a recovery stack) stay concrete behind the current type switch.
2. **Rationalize the compensating-action cluster.** **✅ Implemented — pending commit.** The persisting overload isn't the *function* `invokeCompensator` —
   it's the internal `type compensator struct` (`receiver_registry.go`), a `Compensate*` method plus its invocation
   means: a *compensating action*, mislabeled as the *compensator* artifact. Renaming only the function would
   re-overload "compensator." Rename the cluster: `type compensator` → `compensatingAction`; `CompensatorByName` →
   `CompensatingActionByName`; `compensatorIndex` / `compensatorOnce` → `compensatingActionIndex` /
   `compensatingActionOnce`; `invokeCompensator` → `invokeCompensatingAction`. **Keep** the struct's `compensatorType`
   field — it holds the type of the *compensator artifact* the action accepts, so that name is correct. After this,
   *compensator* (the artifact) and *compensating action* (the named method) never share a word.
   (`invokeCompensatingAction` and `invokeCompensateForReceipt` are method-shaped free functions;
   [step 42](42-compensator-interface.md) promotes them to `compensatingAction.invoke` / `ReceiptBase.Compensate` — a
   rename here, a promotion there.)
3. **Mechanical Go sweep.** **✅ Done — committed 2026-07-11** (folded into phase 1's global `complement` →
   `compensator` rename): the remaining identifiers + doc comments across `pkg/op` + providers (`flow`, `pkg`, `archive`,
   `git`) + tests (~190 occurrences, 19 files); gofmt clean. (The compensating-action cluster rename above is phase 2 —
   the one Go rename still outstanding.)
4. **Doc sweep** — architecture docs (§2.2, §2.3, §5.1) + the phase-8 step / plan docs (~150 occurrences), aligned to the
   settled vocabulary. (The tree-of-compensators model + prior-art references land in §2.2 as their own change.)
5. **Lock + verify** — place the lock sentence verbatim on `op.Receipt`; run the [Verification](#verification) gate
   below (`grep -rci complement` → 0; the trace suites prove the renamed serialized key round-trips).

## Verification

1. `grep -rci "complement"` over `pkg`, `cmd`, `internal`, `docs` → zero (the only tolerated survivors would be
   genuine English uses unrelated to compensation, expected: none).
2. `make test` green modulo the step-18 gate set; the trace save/load/resume suites prove the renamed document
   field round-trips.
3. The `op.Receipt` interface carries the lock sentence verbatim.
