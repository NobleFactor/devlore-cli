// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// RecoveryStack accumulates compensable operations in LIFO order.
//
// On Unwind, each entry is compensated in reverse order. All entries are attempted regardless of individual failures;
// errors are joined via errors.Join.
//
// Per-subgraph executors own one stack each, chained into a tree (phase-8 step 31). A child stack is nested *down*
// into its parent via [RecoveryStack.PushNested], so Unwind cascades compensation through the tree; and it points
// *up* at its parent via the `parent` field, so [RecoveryStack.ResultByUnitID] walks the chain to resolve a promise
// against an ancestor stack's receipt. The nesting is durable (serialized in a [Trace]); the parent pointer is
// transient — re-derived from the nesting on load, never serialized.
//
// A stack may also be *stamped* ([RecoveryStack.Stamp]) as one combinator body-run's resumption record: it then carries
// the receipt's identity/outcome subset — `unitID`, `result`, `resultType`, `err` — directly, so a nested stamped stack
// is self-describing without a wrapper receipt. A stack undoes itself via [RecoveryStack.Unwind], so it needs identity,
// not a named compensator; the stamp is why it does *not* embed [ReceiptBase]. An unstamped stack (a subgraph's child
// stack, a root) leaves those fields zero.
type RecoveryStack struct {

	// entries is the LIFO list of compensable and audit entries pushed onto this stack.
	entries []recoveryEntry

	// parent is the enclosing subgraph's stack, or nil at the root of the chain. [RecoveryStack.ResultByUnitID] walks
	// up through it for promise resolution. Never serialized; re-derived from the nesting on [Trace] load.
	parent *RecoveryStack

	// unitID is the dispatch identity when this stack is a stamped combinator body-run (e.g. "<gatherID>#<i>"), or "" for
	// an unstamped stack. Resume keys on it via [RecoveryStack.NestedStackByUnitID].
	unitID string

	// result is the stamped body-run's return value, replayed on resume so a completed run is skipped, not re-executed.
	result any

	// resultType is the canonical type id of `result`, captured at [RecoveryStack.Stamp], so a reloaded (untyped) result
	// retypes to its produced Go type at [RecoveryStack.rearm].
	resultType string

	// err is the stamped body-run's status: nil when it completed, non-nil (a failure or ErrPaused) otherwise.
	err error
}

// recoveryEntry captures one entry on a [RecoveryStack].
//
// The entry's `compensator` is the Composite "component" — a [Receipt] (a leaf dispatch) or a [*RecoveryStack] (a
// nested subgraph body-run), both [Compensator]s. At unwind time [recoveryEntry.toCompensate] yields it (skipping an
// audit-only receipt, which carries no recovery state) and [RecoveryStack.Unwind] calls its [Compensator.Compensate];
// the non-compensation traversals recover the concrete form via [recoveryEntry.receiptOrNil] /
// [recoveryEntry.recoveryStackOrNil]. Persistable via [RecoveryStack.MarshalJSON].
type recoveryEntry struct {
	compensator Compensator     // reversal artifact: a Receipt or *RecoveryStack; nil for a guard-only node
	restore     *receiptRestore // decoded envelope retained at load for a resource receipt; re-armed on resume
	guard       GuardResult     // decision outcome recorded for a choose decision node; GuardNone otherwise
}

// toCompensate returns the [Compensator] to run at unwind — the entry's nested stack, or its receipt when the receipt
// carries recovery state — or nil for an audit-only entry (a receipt with no recovery state, or a guard-only node).
//
// Returns:
//   - Compensator: the entry's reversal artifact, or nil when the entry contributes nothing to compensate.
func (e recoveryEntry) toCompensate() Compensator {

	if receipt, ok := e.compensator.(Receipt); ok && receipt.Compensator() == nil {
		return nil
	}
	return e.compensator
}

// receiptOrNil returns the entry's compensator as a [Receipt], or nil when it is a nested stack or absent.
//
// Returns:
//   - Receipt: the entry's receipt, or nil.
func (e recoveryEntry) receiptOrNil() Receipt {

	receipt, _ := e.compensator.(Receipt)
	return receipt
}

// recoveryStackOrNil returns the entry's compensator as a [*RecoveryStack], or nil when it is a receipt or absent.
//
// Returns:
//   - *RecoveryStack: the entry's nested stack, or nil.
func (e recoveryEntry) recoveryStackOrNil() *RecoveryStack {

	stack, _ := e.compensator.(*RecoveryStack)
	return stack
}

// receiptRestore retains a resource receipt's codec-decoded envelope between load and resume re-arm.
//
// At load there is no runtime environment to resolve a receipt's id references, so the stack keeps the decoded envelope
// — the base execution state plus the provider's id-reference sub-field — and [RecoveryStack.rearm] reconstructs the
// concrete receipt from it once the catalog is rehydrated. Both halves are format-neutral: whichever codec read the
// trace produced the [ReceiptData] and the `map[string]any`, so reconstruction never re-parses format-specific bytes.
type receiptRestore struct {
	base   ReceiptData
	fields map[string]any
}

// NewRecoveryStack creates an empty RecoveryStack at the root of a chain (no parent).
//
// Returns:
//   - `*RecoveryStack`: the new root stack.
func NewRecoveryStack() *RecoveryStack {
	return newRecoveryStack(nil)
}

// NewChildRecoveryStack creates an empty RecoveryStack chained to `parent`.
//
// A combinator body-run (a Gather iteration, a Choose branch) runs on a child stack so its promise lookups resolve up
// the chain into `parent` and its ancestors ([RecoveryStack.ResultByUnitID]), while its compensation nests *down* into
// `parent` once the run's stamped stack is pushed. Mirrors how [Subgraph.Execute] mints a subgraph's child stack.
//
// Parameters:
//   - `parent`: the enclosing stack to chain up to; must be non-nil (use [NewRecoveryStack] for a root).
//
// Returns:
//   - `*RecoveryStack`: the new chained stack.
func NewChildRecoveryStack(parent *RecoveryStack) *RecoveryStack {
	assert.NonZero("parent", parent)
	return newRecoveryStack(parent)
}

// newRecoveryStack creates an empty RecoveryStack chained to `parent`.
//
// [RecoveryStack.ResultByUnitID] walks up through `parent` to resolve a promise against an ancestor stack's receipt; a
// nil `parent` marks the root of the chain.
//
// Parameters:
//   - `parent`: the enclosing subgraph's stack, or nil for the root.
//
// Returns:
//   - `*RecoveryStack`: the new chained stack.
func newRecoveryStack(parent *RecoveryStack) *RecoveryStack {
	return &RecoveryStack{parent: parent}
}

// Push appends a [Receipt] onto the stack as an audit-trail entry.
//
// Step 12 broadens [RecoveryStack] from a compensable-only ledger to an every-dispatch ledger: the executor calls Push
// at every dispatch exit (cancellation, Do-error, success). When the receipt carries a non-nil compensator, the entry is
// also wired for compensation — [RecoveryStack.Unwind] invokes the action's Compensate companion at rollback, reached
// through the [*RuntimeEnvironment] Unwind supplies (not the receipt's resource, so a resource-less compensator still
// compensates). Otherwise the entry is audit-only and [RecoveryStack.Unwind] skips it.
//
// The receipt must already be committed by its caller; Push does not commit. A nil receipt is a programming error and
// panics via [assert.NonZero].
//
// Parameters:
//   - `receipt`: the receipt to push. Must be non-nil and already committed.
func (s *RecoveryStack) Push(receipt Receipt) {

	assert.NonZero("receipt", receipt)

	s.entries = append(s.entries, recoveryEntry{compensator: receipt})
}

// PushNested appends a substack as a single transactional entry on this stack.
//
// The nested entry preserves the saga boundary: at unwind time the substack is unwound as a unit (its own LIFO walk,
// its own error aggregation) before the outer stack continues, driven by the [*RuntimeEnvironment] the enclosing
// [RecoveryStack.Unwind] threads down.
//
// A nil substack is a programming error and panics via [assert.NonZero].
//
// Parameters:
//   - `recoveryStack`: the substack to nest. Must be non-nil.
func (s *RecoveryStack) PushNested(recoveryStack *RecoveryStack) {

	assert.NonZero("recoveryStack", recoveryStack)

	s.entries = append(s.entries, recoveryEntry{compensator: recoveryStack})
}

// Unwind rolls back all stack entries in LIFO order.
//
// All entries are attempted regardless of individual failures; errors are joined via [errors.Join]. Each entry's undo
// closure is run with `runtimeEnvironment` — supplied here rather than captured at [RecoveryStack.Push] — so the env is
// bound once, at rollback, and threaded down into every nested substack's own Unwind.
//
// Parameters:
//   - `runtimeEnvironment`: the executor's environment, used to resolve and invoke each entry's Compensate companion.
//
// Returns:
//   - `error`: the joined errors from every entry that failed to compensate, or nil when all succeed.
func (s *RecoveryStack) Unwind(runtimeEnvironment *RuntimeEnvironment) error {

	var errs []error

	for i := len(s.entries) - 1; i >= 0; i-- {
		if compensator := s.entries[i].toCompensate(); compensator != nil {
			if err := compensator.Compensate(runtimeEnvironment); err != nil {
				errs = append(errs, err)
			}
		}
	}

	s.entries = nil
	return errors.Join(errs...)
}

// Compensate reverses this recovery stack by unwinding its children LIFO, the composite [Compensator].
//
// It is [RecoveryStack.Unwind] under the [Compensator] interface's name.
//
// Parameters:
//   - `runtimeEnvironment`: the executor's environment, threaded into each entry's compensation.
//
// Returns:
//   - `error`: the joined errors from every entry that failed to compensate, or nil when all succeed.
func (s *RecoveryStack) Compensate(runtimeEnvironment *RuntimeEnvironment) error {
	return s.Unwind(runtimeEnvironment)
}

// rearm reconstructs concrete receipts after a resume operation rehydrates the ledger.
//
// At load a resource receipt is a bare [ReceiptBase] (no env to resolve its ids); this walks the restored tree and, for
// each entry that retained its encoded envelope, reconstructs the concrete receipt via [Receipt.RestoreEncoded] against
// the now-rehydrated catalog, so a resumed-then-failed unwind rolls it back — compensation dispatches through
// [recoveryEntry.compensator] at unwind, needing no pre-bound closure. A subgraph receipt keeps its bare base (its
// compensator is the reconstructed child stack) and recurses.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment, its catalog already rehydrated.
//
// Returns:
//   - `error`: a reconstruction failure (unresolved id, unknown action, malformed envelope).
func (s *RecoveryStack) rearm(runtimeEnvironment *RuntimeEnvironment) error {

	for i := range s.entries {
		entry := &s.entries[i]

		if stack := entry.recoveryStackOrNil(); stack != nil {
			if err := stack.rearm(runtimeEnvironment); err != nil {
				return err
			}
			continue
		}

		receipt := entry.receiptOrNil()
		if receipt == nil {
			continue
		}

		if entry.restore != nil {

			restore := entry.restore
			concrete, err := reconstructReceipt(runtimeEnvironment, restore.base.CompensatingAction, restore.base, restore.fields)

			if err != nil {
				return err
			}

			// A resource receipt is its own compensator: the compensable forward method returns (result, compensator,
			// error) with the receipt as the compensator, and Commit stores that self-reference. Reconstruction has no
			// forward call, so reinstate the identity here unless the receipt restored a compensator of its own.

			if concrete.Compensator() == nil {
				concrete.receiptBase().compensator = concrete
			}

			entry.compensator = concrete
			entry.restore = nil
			receipt = concrete
		}

		if err := retypeResult(runtimeEnvironment, receipt); err != nil {
			return err
		}

		if childStack, ok := receipt.Compensator().(*RecoveryStack); ok {
			if err := childStack.rearm(runtimeEnvironment); err != nil {
				return err
			}
		}
	}

	// Retype this stack's own stamped result (a combinator body-run's cached output), so a resumed run that skips on
	// the stamp returns the produced Go type, not the codec's untyped reload — mirroring [retypeResult] for receipts.
	s.retypeStampedResult(runtimeEnvironment)

	return nil
}

// retypeStampedResult retypes a stamped stack's reloaded (untyped) result to its produced Go type.
//
// Mirrors [retypeResult] for receipts: the produced type id captured at [RecoveryStack.Stamp] resolves through
// [receiverRegistry.ProductTypeByID] and the value converts through the [Convert] cascade. An unstamped stack (nil
// result) or a result [Convert] cannot reconstruct is left as-is rather than failing the resume.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment, forwarded to [Convert] for env-sensitive types.
func (s *RecoveryStack) retypeStampedResult(runtimeEnvironment *RuntimeEnvironment) {

	if s.result == nil || s.resultType == "" {
		return
	}

	productType, ok := ReceiverRegistry().ProductTypeByID(s.resultType)
	if !ok {
		return
	}

	if retyped, err := Convert(runtimeEnvironment, s.result, productType); err == nil {
		s.result = retyped
	}
}

// Discard drops all entries without unwinding.
func (s *RecoveryStack) Discard() {
	s.entries = nil
}

// ResultByUnitID returns the most recent receipt's [Receipt.Result] for the unit identified by `unitID`, searching this
// stack and then walking up the parent chain.
//
// The stack tree is the source of truth for per-dispatch results: every dispatch exit pushes a receipt with the
// producing unit's ID and result, so promise-style "look up an upstream unit's output" queries walk the stacks instead
// of a separate results map. Each stack is searched LIFO so a retried unit returns its latest outcome; when the unit is
// not found, the search continues into the stack's `parent`, so a promise to an upstream producer in an ancestor
// subgraph resolves against that ancestor's stack.
//
// The walk only ever goes *up* the chain, never *down* into nested substacks (a producer always runs before its
// consumer, and so lives in this stack or an ancestor, never in a child).
//
// Parameters:
//   - `unitID`: the [ExecutableUnit.ID] of the producing unit.
//
// Returns:
//   - `any`: the matched receipt's result, or nil when no match is found in this stack or any ancestor.
//   - `bool`: true when a matching receipt was found, false otherwise.
func (s *RecoveryStack) ResultByUnitID(unitID string) (any, bool) {

	for stack := s; stack != nil; stack = stack.parent {
		for i := len(stack.entries) - 1; i >= 0; i-- {
			r := stack.entries[i].receiptOrNil()
			if r != nil && r.UnitID() == unitID {
				return r.Result(), true
			}
		}
	}

	return nil, false
}

// receiptByUnitID returns the receipt for `unitID` from this stack's own entries (not the parent chain), searched LIFO.
//
// Resume reads it against the stack a unit is handed to decide the unit's fate: a receipt with a nil error marks a
// completed unit to replay; an ErrPaused receipt marks an in-progress subgraph to re-enter. The lookup stays on this
// stack — a unit's own receipt lives on its own stack, never an ancestor's — unlike [RecoveryStack.ResultByUnitID].
//
// Parameters:
//   - `unitID`: the [ExecutableUnit.ID] to look up.
//
// Returns:
//   - `Receipt`: the matching receipt, or nil when none is found on this stack.
//   - `bool`: true when a receipt for `unitID` is present.
func (s *RecoveryStack) receiptByUnitID(unitID string) (Receipt, bool) {

	for i := len(s.entries) - 1; i >= 0; i-- {
		r := s.entries[i].receiptOrNil()
		if r != nil && r.UnitID() == unitID {
			return r, true
		}
	}

	return nil, false
}

// NestedStackByUnitID returns the nested stamped substack for `unitID` from this stack's own entries, searched LIFO.
//
// The stamped-stack analog of [RecoveryStack.receiptByUnitID]: a combinator reads it against its own stack to decide a
// body-run's fate on resume — a stack with a nil [RecoveryStack.Err] is a completed run to replay ([RecoveryStack.Result]
// is its cached output); a stack with a non-nil err is an in-progress run to adopt and re-enter. Only stamped nested
// entries (`unitID != ""`) match; audit-only nested stacks are skipped.
//
// Parameters:
//   - `unitID`: the stamped body-run identity to look up (e.g. "<gatherID>#<i>").
//
// Returns:
//   - `*RecoveryStack`: the matching stamped substack, or nil when none is present on this stack.
//   - `bool`: true when a stamped substack for `unitID` is present.
func (s *RecoveryStack) NestedStackByUnitID(unitID string) (*RecoveryStack, bool) {

	for i := len(s.entries) - 1; i >= 0; i-- {
		sub := s.entries[i].recoveryStackOrNil()
		if sub != nil && sub.unitID != "" && sub.unitID == unitID {
			return sub, true
		}
	}

	return nil, false
}

// Stamp finalizes this stack as one combinator body-run's resumption record.
//
// A combinator (Gather, Choose, WaitUntil) runs its body once on a child stack, then stamps that stack with the run's
// identity and outcome before nesting it — the stamped-stack analog of [ReceiptBase.Commit]. It captures `unitID` (the
// resumption key), `result` and its canonical type id (for replay + retype-on-resume), and `err` (the run's status). It
// is a one-shot finalize, not a piecemeal setter; re-stamping an already-adopted paused substack in place updates its
// outcome once the re-entered run completes.
//
// Parameters:
//   - `unitID`: the body-run identity (e.g. "<gatherID>#<i>").
//   - `result`: the body-run's return value.
//   - `err`: the body-run's status — nil on completion, non-nil (a failure or ErrPaused) otherwise.
func (s *RecoveryStack) Stamp(unitID string, result any, err error) {
	s.unitID = unitID
	s.result = result
	s.resultType = canonicalIDOf(result)
	s.err = err
}

// UnitID returns this stack's stamped body-run identity, or "" when the stack is unstamped.
//
// Returns:
//   - `string`: the stamped identity, or "" for an unstamped stack.
func (s *RecoveryStack) UnitID() string { return s.unitID }

// Result returns this stack's stamped body-run result, or nil when the stack is unstamped or the run produced nothing.
//
// Returns:
//   - `any`: the stamped result, or nil.
func (s *RecoveryStack) Result() any { return s.result }

// Err returns this stack's stamped body-run status, or nil when the stack is unstamped or the run completed.
//
// Returns:
//   - `error`: the stamped status, or nil.
func (s *RecoveryStack) Err() error { return s.err }

// GuardByUnitID returns the guard outcome recorded for `unitID`'s top-most receipt entry on this stack.
//
// The decision-tree walk reads it before evaluating truthiness, so a replayed decision node routes by its recorded
// outcome — the guard is evaluated exactly once, on the live result, never re-derived from a round-tripped value
// (phase-8 step 10).
//
// Parameters:
//   - `unitID`: the [ExecutableUnit.ID] whose recorded guard outcome to look up.
//
// Returns:
//   - `GuardResult`: the recorded outcome, or [GuardNone] when none is recorded.
//   - `bool`: true when a guard outcome is recorded for `unitID`.
func (s *RecoveryStack) GuardByUnitID(unitID string) (GuardResult, bool) {

	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		if r := entry.receiptOrNil(); r != nil && r.UnitID() == unitID && entry.guard != GuardNone {
			return entry.guard, true
		}
	}

	return GuardNone, false
}

// SetGuard records `guard` as the decision outcome of `unitID`'s dispatch on this stack.
//
// The decision-tree walk calls it right after a decision node's live dispatch, annotating the node's top-most receipt
// entry so the outcome serializes with the trace and a resume follows the recorded path.
//
// Parameters:
//   - `unitID`: the [ExecutableUnit.ID] whose entry to annotate.
//   - `guard`: the evaluated outcome — [GuardTruthy] or [GuardFalsy].
//
// Returns:
//   - `bool`: true when a receipt entry for `unitID` was found and annotated.
func (s *RecoveryStack) SetGuard(unitID string, guard GuardResult) bool {

	for i := len(s.entries) - 1; i >= 0; i-- {
		if r := s.entries[i].receiptOrNil(); r != nil && r.UnitID() == unitID {
			s.entries[i].guard = guard
			return true
		}
	}

	return false
}

// supersede removes the top-most entry whose receipt is for `unitID`, dropping it from this stack.
//
// Resume calls this when an in-progress subgraph re-enters: its stale ErrPaused receipt is removed before the subgraph
// re-dispatches, so the fresh completion receipt replaces it rather than leaving a duplicate on the stack.
//
// Parameters:
//   - `unitID`: the [ExecutableUnit.ID] whose entry to remove.
func (s *RecoveryStack) supersede(unitID string) {

	for i := len(s.entries) - 1; i >= 0; i-- {
		r := s.entries[i].receiptOrNil()
		if r != nil && r.UnitID() == unitID {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return
		}
	}
}

// Receipts returns every receipt-bearing entry on this stack, descending into nested substacks, in push
// order (oldest first).
//
// Unlike [RecoveryStack.ResultByUnitID] — which searches only this stack's top level — Receipts flattens nested
// substacks so callers that summarize a whole execution (see [Trace.Summarize]) observe every dispatched unit's
// receipt, including per-iteration combinator children. Nested-stack marker entries contribute their contained
// receipts, not themselves; and a receipt whose compensator is itself a [RecoveryStack] — a subgraph or file.WalkTree
// dispatch — also contributes that child stack's receipts, since the child no longer rides a separate nested entry.
//
// Returns:
//   - []Receipt: the flattened receipts in push order; empty when the stack holds none.
func (s *RecoveryStack) Receipts() []Receipt {

	var receipts []Receipt
	for _, entry := range s.entries {
		if receipt := entry.receiptOrNil(); receipt != nil {
			receipts = append(receipts, receipt)
			if childStack, ok := receipt.Compensator().(*RecoveryStack); ok {
				receipts = append(receipts, childStack.Receipts()...)
			}
		} else if stack := entry.recoveryStackOrNil(); stack != nil {
			receipts = append(receipts, stack.Receipts()...)
		}
	}
	return receipts
}

// Len returns the number of entries on the stack.
func (s *RecoveryStack) Len() int {
	return len(s.entries)
}

// MarshalJSON encodes the stack's entries as a JSON object.
//
// Encoded form: `{"entries": [...]}` where each element is either a receipt envelope or a nested substack (`{"sub":
// {...}}`) — disjoint field sets, no `kind` tag. The receipt envelope (see [receiptEnvelope]) is the stack-owned record
// of a dispatch's execution state, read off the [Receipt] interface, so a reloaded stack carries every unit's id,
// result, status, and child-stack compensator regardless of which concrete receipt produced it. A `*RecoveryStack`
// compensator encodes recursively via this same method.
func (s *RecoveryStack) MarshalJSON() ([]byte, error) {

	v, err := s.MarshalYAML()
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// receiptEnvelope is the stack-owned, provider-agnostic encoding of one receipt's execution state.
//
// The recovery stack — not each provider's receipt — owns this, so a new or maintained provider cannot forget to encode
// the fields resume needs. It reads them off the [Receipt] interface ([Receipt.UnitID], [Receipt.Result], etc.), so a
// reloaded receipt restores enough to be skipped (a successful unit replays its result), adopted (a subgraph's
// `*RecoveryStack` compensator is reconstructed), and summarized — independent of the concrete receipt's own encoding.
type receiptEnvelope struct {
	UnitID             string         `json:"unit_id"                      yaml:"unit_id"`
	ForwardAction      string         `json:"forward_action,omitempty"     yaml:"forward_action,omitempty"`
	CompensatingAction string         `json:"compensating_action,omitempty" yaml:"compensating_action,omitempty"`
	Result             any            `json:"result,omitempty"             yaml:"result,omitempty"`
	ResultType         string         `json:"result_type,omitempty" yaml:"result_type,omitempty"`
	Status             string         `json:"status,omitempty"      yaml:"status,omitempty"`
	Guard              GuardResult    `json:"guard,omitempty"       yaml:"guard,omitempty"`
	Compensator        *RecoveryStack `json:"compensator,omitempty"  yaml:"compensator,omitempty"`
	Receipt            any            `json:"receipt,omitempty"     yaml:"receipt,omitempty"`
}

// MarshalYAML returns the stack's entries as anonymous struct values the encoder walks.
//
// Source of truth for the encoded shape; [RecoveryStack.MarshalJSON] delegates here. Each receipt entry becomes a
// [receiptEnvelope] carrying its execution state; a child-stack compensator encodes recursively, while a resource
// receipt's compensation state rides a `receipt` sub-field carrying the receipt's own id-based encoding (see
// file.Receipt.MarshalYAML).
func (s *RecoveryStack) MarshalYAML() (any, error) {

	entries := make([]any, 0, len(s.entries))

	for _, e := range s.entries {
		if stack := e.recoveryStackOrNil(); stack != nil {
			entries = append(entries, struct {
				Sub *RecoveryStack `json:"sub" yaml:"sub"`
			}{Sub: stack})
			continue
		}

		receipt := e.receiptOrNil()
		if receipt == nil {
			continue
		}

		envelope := receiptEnvelope{
			UnitID:             receipt.UnitID(),
			ForwardAction:      receipt.ForwardAction(),
			CompensatingAction: receipt.CompensatingAction(),
			Result:             receipt.Result(),
			ResultType:         receipt.ResultType(),
			Status:             errStatus(receipt.Err()),
			Guard:              e.guard,
		}
		if childStack, ok := receipt.Compensator().(*RecoveryStack); ok {
			envelope.Compensator = childStack
		} else if compensator, isReceipt := receipt.Compensator().(Receipt); isReceipt && compensator == receipt {
			// A single-resource receipt is its own compensator (the forward method returns it as the compensator);
			// its id-based encoding rides the `receipt` sub-field (see file.Receipt.MarshalYAML), reconstructed
			// against the rehydrated ledger at resume. The other legal compensator shapes are not reconstructed
			// here: a []Receipt (e.g. pkg.Install) is a follow-up — carrying no sub-field, it resumes without that
			// receipt's compensation rather than failing; a *RecoveryStack rides the `compensator` field above.
			envelope.Receipt = receipt
		}
		entries = append(entries, envelope)
	}

	return struct {
		UnitID     string `json:"unit_id,omitempty"     yaml:"unit_id,omitempty"`
		Result     any    `json:"result,omitempty"      yaml:"result,omitempty"`
		ResultType string `json:"result_type,omitempty" yaml:"result_type,omitempty"`
		Status     string `json:"status,omitempty"      yaml:"status,omitempty"`
		Entries    []any  `json:"entries"               yaml:"entries"`
	}{
		UnitID:     s.unitID,
		Result:     s.result,
		ResultType: s.resultType,
		Status:     errStatus(s.err),
		Entries:    entries,
	}, nil
}

// recoveryEntryData is the codec-decoded shape of one stack entry — a nested substack or a receipt envelope.
//
// [RecoveryStack.UnmarshalJSON] and [RecoveryStack.UnmarshalYAML] decode a slice of these (each through its own codec),
// then [RecoveryStack.fromEntries] builds the live entries. A `sub` marks a nested stack; otherwise the base execution
// state rides the envelope fields and a resource receipt's id references ride `receipt` as a format-neutral map the
// receipt resolves at re-arm. The nested `*RecoveryStack` fields decode recursively through the same two unmarshalers,
// so the whole tree reconstructs in whichever format the trace was stored.
type recoveryEntryData struct {
	Sub                *RecoveryStack `json:"sub,omitempty"                 yaml:"sub,omitempty"`
	UnitID             string         `json:"unit_id,omitempty"             yaml:"unit_id,omitempty"`
	ForwardAction      string         `json:"forward_action,omitempty"      yaml:"forward_action,omitempty"`
	CompensatingAction string         `json:"compensating_action,omitempty" yaml:"compensating_action,omitempty"`
	Result             any            `json:"result,omitempty"              yaml:"result,omitempty"`
	ResultType         string         `json:"result_type,omitempty" yaml:"result_type,omitempty"`
	Status             string         `json:"status,omitempty"      yaml:"status,omitempty"`
	Guard              GuardResult    `json:"guard,omitempty"       yaml:"guard,omitempty"`
	Compensator        *RecoveryStack `json:"compensator,omitempty"  yaml:"compensator,omitempty"`
	Receipt            map[string]any `json:"receipt,omitempty"     yaml:"receipt,omitempty"`
}

// UnmarshalJSON reconstructs the stack tree from the JSON form encoded by [RecoveryStack.MarshalJSON].
//
// It decodes the entries into [recoveryEntryData] and delegates to [RecoveryStack.fromEntries], which the YAML reader
// ([RecoveryStack.UnmarshalYAML]) shares — so JSON and YAML reconstruct identically. Reconstruction consumes the
// decoded values, never re-parsed bytes, per the format-neutral requirement (a trace must reload and verify across
// JSON/YAML/Protobuf — see the step doc's "Format-neutral trace reconstruction" section).
//
// Parameters:
//   - `data`: the JSON produced by [RecoveryStack.MarshalJSON].
//
// Returns:
//   - `error`: non-nil on malformed input.
func (s *RecoveryStack) UnmarshalJSON(data []byte) error {

	var encoded struct {
		UnitID     string              `json:"unit_id"`
		Result     any                 `json:"result"`
		ResultType string              `json:"result_type"`
		Status     string              `json:"status"`
		Entries    []recoveryEntryData `json:"entries"`
	}
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}

	s.restoreStamp(encoded.UnitID, encoded.Result, encoded.ResultType, encoded.Status)
	return s.fromEntries(encoded.Entries)
}

// UnmarshalYAML reconstructs the stack tree from the YAML form encoded by [RecoveryStack.MarshalYAML].
//
// The YAML mirror of [RecoveryStack.UnmarshalJSON]: it decodes the entries into [recoveryEntryData] through the YAML
// codec and delegates to the shared [RecoveryStack.fromEntries] builder, so a YAML-stored trace reconstructs through
// the same format-neutral path as a JSON one.
//
// Parameters:
//   - `unmarshal`: the YAML node decoder supplied by the yaml package.
//
// Returns:
//   - `error`: non-nil on malformed input.
func (s *RecoveryStack) UnmarshalYAML(unmarshal func(any) error) error {

	var encoded struct {
		UnitID     string              `yaml:"unit_id"`
		Result     any                 `yaml:"result"`
		ResultType string              `yaml:"result_type"`
		Status     string              `yaml:"status"`
		Entries    []recoveryEntryData `yaml:"entries"`
	}
	if err := unmarshal(&encoded); err != nil {
		return err
	}

	s.restoreStamp(encoded.UnitID, encoded.Result, encoded.ResultType, encoded.Status)
	return s.fromEntries(encoded.Entries)
}

// restoreStamp restores this stack's stamped identity/outcome fields from their codec-decoded values.
//
// Shared by [RecoveryStack.UnmarshalJSON] and [RecoveryStack.UnmarshalYAML] so a stamped nested stack reconstructs
// identically from either format. The reloaded `result` is untyped; [RecoveryStack.rearm] retypes it via `resultType`.
//
// Parameters:
//   - `unitID`: the decoded stamped identity, or "" for an unstamped stack.
//   - `result`: the decoded (untyped) result.
//   - `resultType`: the canonical type id used to retype `result` at re-arm.
//   - `status`: the decoded status message; non-empty restores as a non-nil [RecoveryStack.Err].
func (s *RecoveryStack) restoreStamp(unitID string, result any, resultType, status string) {
	s.unitID = unitID
	s.result = result
	s.resultType = resultType
	if status != "" {
		s.err = errors.New(status)
	}
}

// fromEntries builds the live recovery entries from their codec-decoded [recoveryEntryData].
//
// A `Sub` entry becomes a nested substack. Otherwise the envelope's base execution state seeds a bare [ReceiptBase] (no
// environment exists at load to resolve ids), and a present `receipt` sub-field is retained as a [receiptRestore] so
// [RecoveryStack.rearm] reconstructs the concrete receipt against the rehydrated catalog at resume. That is enough for
// resume to skip, adopt, and summarize; a resource receipt's own undo state is restored at re-arm, not here.
//
// Parameters:
//   - `entries`: the decoded entries, in stack order.
//
// Returns:
//   - `error`: non-nil when a bare receipt's base restore fails.
func (s *RecoveryStack) fromEntries(entries []recoveryEntryData) error {

	s.entries = make([]recoveryEntry, 0, len(entries))

	for _, e := range entries {

		if e.Sub != nil {
			s.entries = append(s.entries, recoveryEntry{compensator: e.Sub})
			continue
		}

		base := ReceiptData{
			UnitID:             e.UnitID,
			ForwardAction:      e.ForwardAction,
			CompensatingAction: e.CompensatingAction,
			Result:             e.Result,
			ResultType:         e.ResultType,
			Status:             e.Status,
		}
		if e.Compensator != nil {
			base.Compensator = e.Compensator
		}

		receipt := &ReceiptBase{}
		if err := receipt.RestoreEncoded(nil, base, nil); err != nil {
			return err
		}

		entry := recoveryEntry{compensator: receipt, guard: e.Guard}
		if len(e.Receipt) > 0 {
			entry.restore = &receiptRestore{base: base, fields: e.Receipt}
		}

		s.entries = append(s.entries, entry)
	}

	return nil
}

// errStatus returns an error's message, or "" for a nil error — the encoded form of a receipt's [Receipt.Err].
func errStatus(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// invokeCompensateForReceipt resolves a receipt's [Compensate] companion via the registry and invokes it.
//
// Used by [RecoveryStack.Push]'s pre-bound compensation closure at [RecoveryStack.Unwind] time. The env is supplied by
// the executor — not read off the receipt's resource — so a resource-less compensator (a combinator or file.WalkTree
// recovery stack) still resolves its companion. [ReceiverRegistry.ActionByPath] looks up the action by its committed
// Go-qualified name; a receipt that recorded the dotted name instead (a unit that bound its action by name — the graph
// root, every combinator) falls back to [RuntimeEnvironment.ActionByName]. [Method.Undo] then dispatches to the
// [Compensate] companion with [Receipt.Compensator] as the undo state.
//
// [ErrNotCompensable] from the companion is treated as a success (logged elsewhere; not surfaced as an error).
//
// Parameters:
//   - `runtimeEnvironment`: the executor's environment; resolves the [ReceiverRegistry] provider for the action.
//   - `receipt`: the audit receipt whose [Receipt.Compensator] is the undo state handed to the companion.
//
// Returns:
//   - `error`: non-nil when the env is nil, the action is unregistered, the provider fails, or the companion fails.
func invokeCompensateForReceipt(runtimeEnvironment *RuntimeEnvironment, receipt Receipt) error {

	if runtimeEnvironment == nil {
		return fmt.Errorf("invokeCompensateForReceipt: receipt %s has no runtime environment", receipt.CompensatingAction())
	}

	activationRecord := &ActivationRecord{RuntimeEnvironment: runtimeEnvironment, Context: runtimeEnvironment.Context}

	// A receipt whose compensatingAction names a registered compensating action declares its own undo: route through
	// the compensating-action index. A compensatingAction that is still a dispatch action (a not-yet-migrated
	// provider) misses the index and falls through to the forward-action Compensate<Name> path below.

	if comp, ok := ReceiverRegistry().CompensatingActionByName(receipt.CompensatingAction()); ok {
		provider, err := runtimeEnvironment.cachedProvider(comp.providerReceiverType)
		if err != nil {
			return fmt.Errorf("invokeCompensateForReceipt: cache provider %q: %w", comp.providerReceiverType.Name(), err)
		}
		return ignoreNotCompensable(invokeCompensatingAction(comp, activationRecord, provider, receipt.Compensator()))
	}

	providerReceiverType, method, ok := ReceiverRegistry().ActionByPath(receipt.CompensatingAction())

	if !ok {

		// A unit that binds its action by name (the graph root and every combinator) records the dotted action name —
		// e.g. "flow.subgraph" — as its action path, not the Go-qualified ActionName that ActionByPath keys on. Resolve
		// the dotted name through the environment's action resolver (the same one dispatch uses) and retry on the
		// resolved Go-qualified path.

		resolved, resolveErr := runtimeEnvironment.ActionByName(receipt.CompensatingAction())
		if resolveErr == nil && resolved != nil {
			providerReceiverType, method, ok = ReceiverRegistry().ActionByPath(resolved.FullName())
		}
	}

	if !ok {
		return fmt.Errorf("invokeCompensateForReceipt: no registered action %q", receipt.CompensatingAction())
	}

	provider, err := runtimeEnvironment.cachedProvider(providerReceiverType)
	if err != nil {
		return fmt.Errorf("invokeCompensateForReceipt: cache provider %q: %w", providerReceiverType.Name(), err)
	}

	return ignoreNotCompensable(method.Undo(activationRecord, provider, receipt.Compensator()))
}

// ignoreNotCompensable maps [ErrNotCompensable] to nil — a companion that declines to compensate is a success, not a
// rollback failure — and passes any other error through.
//
// Parameters:
//   - `err`: the error returned by a compensator or [Method.Undo].
//
// Returns:
//   - `error`: nil when err is nil or [ErrNotCompensable]; err otherwise.
func ignoreNotCompensable(err error) error {
	if errors.Is(err, ErrNotCompensable) {
		return nil
	}
	return err
}

// invokeCompensatingAction calls a registered compensating action on the provider receiver with the compensator as its
// undo state.
//
// It mirrors [Method.Undo]'s reflection call: the receiver, the activation when the compensating action's first
// parameter expects it, then the compensator.
//
// Parameters:
//   - `comp`: the resolved compensating action (its reflect.Method and activation-first shape).
//   - `activation`: the per-dispatch record forwarded when the compensating action's signature expects it.
//   - `receiver`: the provider value the compensating action is called on.
//   - `compensator`: the compensator handed to the compensating action as its undo state.
//
// Returns:
//   - `error`: the compensating action's error.
func invokeCompensatingAction(
	comp compensatingAction, activation *ActivationRecord, receiver any, compensator any,
) error {

	var goArgs []reflect.Value
	if comp.firstParamIsActivation {
		goArgs = []reflect.Value{reflect.ValueOf(receiver), reflect.ValueOf(activation), reflect.ValueOf(compensator)}
	} else {
		goArgs = []reflect.Value{reflect.ValueOf(receiver), reflect.ValueOf(compensator)}
	}

	results := comp.method.Func.Call(goArgs)
	return errorFromValue(results[0])
}

// retypeResult retypes a receipt's reloaded (untyped) result to its produced Go type, restoring full type fidelity.
//
// The produced type id was recorded at [ReceiptBase.Commit] (see [canonicalID]); it is authoritative even when a
// combinator's static return is `any`. Resolution goes through [receiverRegistry.ProductTypeByID] and the value through
// the [Convert] cascade (a struct hydrates from its map, a plain resource resolves through the rehydrated catalog). The
// produced-type-id is scoped to struct/scalar/resource: a result it cannot reconstruct — notably a content-addressable
// observation, which round-trips by re-observe-and-verify, not reconstruction — is left as-is, as are a nil result and
// an empty or unknown id, rather than failing the resume.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment, forwarded to [Convert] for env-sensitive types (resources).
//   - `receipt`: the receipt whose result is retyped in place.
//
// Returns:
//   - `error`: reserved; currently always nil — a retype that does not apply leaves the value as-is.
func retypeResult(runtimeEnvironment *RuntimeEnvironment, receipt Receipt) error {

	result := receipt.Result()
	if result == nil {
		return nil
	}

	productType, ok := ReceiverRegistry().ProductTypeByID(receipt.ResultType())
	if !ok {
		return nil
	}

	retyped, err := Convert(runtimeEnvironment, result, productType)
	if err != nil {
		// The produced-type-id is scoped to struct/scalar/resource; a value it cannot reconstruct -- notably a
		// content-addressable observation, which round-trips by re-observe-and-verify, not reconstruction -- is left
		// as-is rather than failing the resume. A consumer that needs the concrete type fails at its own dispatch.
		return nil
	}

	receipt.receiptBase().result = retyped
	return nil
}

// reconstructReceipt rebuilds the concrete receipt for an action from its codec-decoded envelope.
//
// The concrete type is read off the action's Compensate companion (the same companion compensation resolves at unwind),
// so `op` instantiates the right receipt without importing the provider or consulting a registry — the type comes from
// the action name every codec carries, which is what keeps reconstruction format-neutral (no type-URL registry needed).
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment.
//   - `action`: the receipt's dotted action name.
//   - `base`: the codec-decoded base execution state.
//   - `fields`: the receipt's id-reference sub-field, decoded to a format-neutral map.
//
// Returns:
//   - `Receipt`: the reconstructed concrete receipt.
//   - `error`: an unknown action, a non-Receipt companion parameter, or a [Receipt.RestoreEncoded] failure.
func reconstructReceipt(runtimeEnvironment *RuntimeEnvironment, action string, base ReceiptData,
	fields map[string]any) (Receipt, error) {

	receiptType, err := receiptTypeForAction(runtimeEnvironment, action)
	if err != nil {
		return nil, err
	}

	if receiptType.Kind() != reflect.Pointer {
		return nil, fmt.Errorf("reconstructReceipt: action %q companion parameter %s is not a pointer", action, receiptType)
	}

	receipt, ok := reflect.New(receiptType.Elem()).Interface().(Receipt)
	if !ok {
		return nil, fmt.Errorf("reconstructReceipt: action %q reconstructs %s, which is not a Receipt", action, receiptType)
	}

	if err := receipt.RestoreEncoded(runtimeEnvironment, base, fields); err != nil {
		return nil, err
	}

	return receipt, nil
}

// receiptTypeForAction returns the concrete receipt type the action's Compensate companion declares.
//
// Resolution mirrors [invokeCompensateForReceipt]: [receiverRegistry.ActionByPath] for a Go-qualified name, falling
// back to [RuntimeEnvironment.ActionByName] for a dotted name. The companion's last parameter is the receipt type.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment (for the dotted-name fallback).
//   - `action`: the receipt's action name.
//
// Returns:
//   - `reflect.Type`: the companion's receipt parameter type (a pointer type).
//   - `error`: an unregistered action or an action with no compensation companion.
func receiptTypeForAction(runtimeEnvironment *RuntimeEnvironment, action string) (reflect.Type, error) {

	// A compensatingAction that names a registered compensator resolves its receipt type directly off that compensator
	// (the same index the unwind path uses); a dispatch action misses the index and falls through to the forward path.
	if comp, ok := ReceiverRegistry().CompensatingActionByName(action); ok {
		return comp.compensatorType, nil
	}

	_, method, ok := ReceiverRegistry().ActionByPath(action)
	if !ok {
		resolved, resolveErr := runtimeEnvironment.ActionByName(action)
		if resolveErr == nil && resolved != nil {
			_, method, ok = ReceiverRegistry().ActionByPath(resolved.FullName())
		}
	}
	if !ok {
		return nil, fmt.Errorf("receiptTypeForAction: no registered action %q", action)
	}

	compensatorType, ok := method.compensatorType()
	if !ok {
		return nil, fmt.Errorf("receiptTypeForAction: action %q has no compensation companion", action)
	}

	return compensatorType, nil
}
