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

var (
	_ Compensator      = (*RecoveryStack)(nil) // Interface Guard: a stack compensates by unwinding its children LIFO.
	_ json.Marshaler   = (*RecoveryStack)(nil) // Interface Guard: the stack serializes its own encoded form.
	_ json.Unmarshaler = (*RecoveryStack)(nil) // Interface Guard: the stack reconstructs from its encoded form.
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
// transient — re-derived from the nesting on a load operation, never serialized.
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
	// up through it to resolve promises. Never serialized; re-derived from the nesting on a [Trace] load.
	parent *RecoveryStack

	// unitID is the dispatch identity when this stack is a stamped combinator body-run (e.g. "<gatherID>#<i>"), or "" for
	// an unstamped stack. Resume keys on it via [RecoveryStack.NestedStackByUnitID].
	unitID string

	// result is the stamped body-run's return value, replayed on resume so a completed run is skipped, not re-executed.
	result any

	// resultType is the canonical type id of `result`, captured at [RecoveryStack.Stamp], so a reloaded (untyped)
	// result retypes to its produced Go type at [RecoveryStack.rearm].
	resultType string

	// err is the stamped body-run's status: nil when it completed, non-nil (a failure or ErrPaused) otherwise.
	err error
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

// region EXPORTED METHODS

// region State management

// Err returns this stack's stamped body-run status, or nil when the stack is unstamped or the run completed.
//
// Returns:
//   - `error`: the stamped status, or nil.
func (s *RecoveryStack) Err() error { return s.err }

// Len returns the number of entries on the stack.
//
// Returns:
//   - `int`: the entry count.
func (s *RecoveryStack) Len() int {
	return len(s.entries)
}

// Result returns this stack's stamped body-run result, or nil when the stack is unstamped or the run produced nothing.
//
// Returns:
//   - `any`: the stamped result, or nil.
func (s *RecoveryStack) Result() any { return s.result }

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

// endregion

// region Behaviors

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

// Discard drops all entries without unwinding.
func (s *RecoveryStack) Discard() {
	s.entries = nil
}

// MarshalJSON encodes the stack as a JSON object via [RecoveryStack.MarshalYAML].
//
// Encoded form: `{stamp, "entries": [...]}` where each element serializes itself — a nested `*RecoveryStack` recurses
// (carrying its own `entries`), and a receipt emits its own flat encoding ([ReceiptData] base + the concrete receipt's
// id references). No `kind` tag and no stack-owned envelope: decode discriminates structurally on the presence of
// `entries` (phase-8 step 42 slice 3b).
//
// Returns:
//   - `[]byte`: the encoded JSON object.
//   - `error`: non-nil when a receipt or nested stack fails to encode.
func (s *RecoveryStack) MarshalJSON() ([]byte, error) {

	v, err := s.MarshalYAML()
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// MarshalYAML returns the stack's stamp and entries as a struct value the encoder walks.
//
// Source of truth for the encoded shape; [RecoveryStack.MarshalJSON] delegates here. Each entry's compensator
// serializes itself: a nested `*RecoveryStack` recurses through this method, a receipt through its own
// [Receipt.MarshalJSON] ([ReceiptData] base plus the concrete receipt's id references). The recovery tree owns no
// receipt envelope — a receipt is responsible for its whole encoding (phase-8 step 42 slice 3b).
//
// Returns:
//   - `any`: the encodable struct value carrying the stamp fields and entries.
//   - `error`: reserved; currently always nil.
func (s *RecoveryStack) MarshalYAML() (any, error) {

	entries := make([]any, 0, len(s.entries))
	for _, e := range s.entries {
		if e.compensator == nil {
			continue
		}
		entries = append(entries, e.compensator)
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

// NestedStackByUnitID returns the nested stamped substack for `unitID` from this stack's own entries, searched LIFO.
//
// The stamped-stack analog of [RecoveryStack.receiptByUnitID]: a combinator reads it against its own stack to decide a
// body-run's fate on resume — a stack with a nil [RecoveryStack.Err] is a completed run to replay
// ([RecoveryStack.Result] is its cached output); a stack with a non-nil err is an in-progress run to adopt and
// re-enter. Only stamped nested entries (`unitID != ""`) match; audit-only nested stacks are skipped.
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

// Push appends a [Receipt] onto the stack as an audit-trail entry.
//
// Step 12 broadens [RecoveryStack] from a compensable-only ledger to an every-dispatch ledger: the executor calls Push
// at every dispatch exit (cancellation, Do-error, success). When the receipt carries a non-nil compensator, the entry
// is also wired for compensation — [RecoveryStack.Unwind] invokes the action's Compensate companion at rollback,
// reached through the [*RuntimeEnvironment] Unwind supplies (not the receipt's resource, so a resource-less compensator
// still compensates). Otherwise the entry is audit-only and [RecoveryStack.Unwind] skips it.
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

// Receipts returns all receipt-bearing entries on the stack, descending into nested substacks, in FIFO order.
//
// Unlike [RecoveryStack.ResultByUnitID], which searches only this stack's top level, Receipts flattens nested substacks
// so callers that summarize a whole execution (see [Trace.Summarize]) observe every dispatched unit's receipt,
// including per-iteration combinator children. Nested-stack marker entries contribute their contained receipts, not
// themselves; and a receipt whose compensator is itself a [RecoveryStack] — a subgraph or file.WalkTree dispatch — also
// contributes that child stack's receipts, since the child no longer rides a separate nested entry.
//
// Returns:
//   - `[]Receipt`: the flattened receipts in push order; empty when the stack holds none.
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
			entry := stack.entries[i]
			if r := entry.receiptOrNil(); r != nil && r.UnitID() == unitID {
				return r.Result(), true
			}
			// A combinator (subgraph/choose/gather/wait_until) is a stamped nested stack carrying its own result —
			// downstream promises resolve against it just like a leaf receipt (subgraph-direct-push, step 42 slice 3a).
			if sub := entry.recoveryStackOrNil(); sub != nil && sub.unitID == unitID {
				return sub.Result(), true
			}
		}
	}

	return nil, false
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

// Unwind rolls back all stack entries in LIFO order.
//
// All entries are attempted regardless of individual failures; errors are joined via [errors.Join]. Each entry's undo
// closure is run with `runtimeEnvironment` — supplied here rather than captured at [RecoveryStack.Push] — so the env is
// bound once, at rollback, and threaded down into every nested substack's own Unwind.
//
// The stack is the compensation-failure journal (phase-8 step 21): a *clean* unwind clears the entries — the system is
// back at its pre-run baseline, nothing to journal — but a *failed* unwind RETAINS them, recording the error each leaf
// receipt's [Receipt.Compensate] returned on that receipt as its [Receipt.CompensationError] (a nested substack has no
// receipt of its own — its dirtiness rides its own retained failed children). The retained tree is what
// [GraphExecutor.Trace] reports on a stopped × [ConditionCompensationFailed] terminal, so a client can persist and
// present the source (the failing node's receipt) and the diagnostics (which Compensate failed and why).
//
// Parameters:
//   - `runtimeEnvironment`: the executor's environment, used to resolve and invoke each entry's Compensate companion.
//
// Returns:
//   - `error`: the joined errors from every entry that failed to compensate, or nil when all succeed.
func (s *RecoveryStack) Unwind(runtimeEnvironment *RuntimeEnvironment) error {

	var errs []error

	for i := len(s.entries) - 1; i >= 0; i-- {

		compensator := s.entries[i].toCompensate()
		if compensator == nil {
			continue
		}

		err := compensator.Compensate(runtimeEnvironment)
		if err == nil {
			continue
		}

		errs = append(errs, err)

		// Record the failure on its own receipt so the retained journal names which Compensate failed and why. A nested
		// substack has no receipt of its own (receiptOrNil is nil); its dirtiness rides its own retained children.
		if receipt := s.entries[i].receiptOrNil(); receipt != nil {
			receipt.receiptBase().compensationError = err
		}
	}

	// Retain the entries on a failed unwind — the dirty journal — and clear only on a clean one (nothing to journal).
	if len(errs) == 0 {
		s.entries = nil
	}

	return errors.Join(errs...)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// fromEntries builds the live recovery entries from their codec-decoded [recoveryEntryData].
//
// A `stack` entry becomes a nested substack. Otherwise the base execution state seeds a bare [ReceiptBase] (no
// environment exists at load to resolve ids), and when the receipt names a compensating action its whole flat object is
// retained as a [receiptRestore] so [RecoveryStack.rearm] reconstructs the concrete receipt against the rehydrated
// catalog at resume. That is enough for resume to skip, adopt, and summarize; a resource receipt's own undo state is
// restored at re-arm, not here. A receipt with no compensating action is audit-only and stays a bare [ReceiptBase].
//
// Parameters:
//   - `entries`: the decoded entries, in stack order.
//
// Returns:
//   - `error`: non-nil when a bare receipt's base restore fails.
func (s *RecoveryStack) fromEntries(entries []recoveryEntryData) error {

	s.entries = make([]recoveryEntry, 0, len(entries))

	for _, e := range entries {

		if e.stack != nil {
			s.entries = append(s.entries, recoveryEntry{compensator: e.stack})
			continue
		}

		receipt := &ReceiptBase{}
		if err := receipt.RestoreEncoded(nil, e.base, nil); err != nil {
			return err
		}

		entry := recoveryEntry{compensator: receipt}
		if e.base.CompensatingAction != "" {
			entry.restore = &receiptRestore{base: e.base, fields: e.fields}
		}

		s.entries = append(s.entries, entry)
	}

	return nil
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
//   - `runtimeEnvironment`: the `resume` environment, its catalog already rehydrated.
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
			concrete, err := reconstructReceipt(
				runtimeEnvironment, restore.base.CompensatingAction, restore.base, restore.fields)

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

// retypeStampedResult retypes a stamped stack's reloaded (untyped) result to its produced Go type.
//
// Mirrors [retypeResult] for receipts: the produced type id captured at [RecoveryStack.Stamp] resolves through
// [receiverRegistry.ProductTypeByID] and the value converts through the [Convert] cascade. An unstamped stack (nil
// result) or a result [Convert] cannot reconstruct is left as-is rather than failing the resume.
//
// Parameters:
//   - `runtimeEnvironment`: the `resume` environment, forwarded to [Convert] for env-sensitive types.
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

// supersede removes the top-most entry for `unitID` — a receipt or a stamped nested substack — dropping it from this
// stack.
//
// Resume calls this when an in-progress unit re-enters: its stale ErrPaused entry is removed before the unit
// re-dispatches, so the fresh completion replaces it rather than leaving a duplicate on the stack.
//
// Parameters:
//   - `unitID`: the [ExecutableUnit.ID] whose entry to remove.
func (s *RecoveryStack) supersede(unitID string) {

	for i := len(s.entries) - 1; i >= 0; i-- {
		if r := s.entries[i].receiptOrNil(); r != nil && r.UnitID() == unitID {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return
		}
		if sub := s.entries[i].recoveryStackOrNil(); sub != nil && sub.unitID == unitID {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return
		}
	}
}

// endregion

// endregion

// region SUPPORTING TYPES

// receiptRestore retains a resource receipt's codec-decoded envelope between a load and resume re-arm.
//
// At load time there is no runtime environment to resolve a receipt's id references, so the stack keeps the decoded
// envelope. The base execution state plus the provider's id-reference subfield — and [RecoveryStack.rearm] reconstructs
// the concrete receipt from it once the catalog is rehydrated. Both halves are format-neutral: whichever codec read the
// trace produced the [ReceiptData] and the `map[string]any`, so reconstruction never reparses format-specific bytes.
type receiptRestore struct {
	base   ReceiptData
	fields map[string]any
}

// recoveryEntry captures one entry on a [RecoveryStack].
//
// The entry's `compensator` is the Composite "component" — a [Receipt] (a leaf dispatch) or a [*RecoveryStack] (a
// nested subgraph body-run), both [Compensator]s. At unwind time [recoveryEntry.toCompensate] yields it (skipping an
// audit-only receipt, which carries no recovery state) and [RecoveryStack.Unwind] calls its [Compensator.Compensate];
// the non-compensation traversals recover the concrete form via [recoveryEntry.receiptOrNil] /
// [recoveryEntry.recoveryStackOrNil]. Persistable via [RecoveryStack.MarshalJSON].
type recoveryEntry struct {
	compensator Compensator     // reversal artifact: a Receipt or *RecoveryStack
	restore     *receiptRestore // decoded envelope retained at load time for a resource receipt; re-armed on resume
}

// receiptOrNil returns the entry's compensator as a [Receipt], or nil when it is a nested stack or absent.
//
// Returns:
//   - `Receipt`: the entry's receipt, or nil.
func (e recoveryEntry) receiptOrNil() Receipt {

	receipt, _ := e.compensator.(Receipt)
	return receipt
}

// recoveryStackOrNil returns the entry's compensator as a [*RecoveryStack], or nil when it is a receipt or absent.
//
// Returns:
//   - `*RecoveryStack`: the entry's nested stack, or nil.
func (e recoveryEntry) recoveryStackOrNil() *RecoveryStack {

	stack, _ := e.compensator.(*RecoveryStack)
	return stack
}

// toCompensate returns the [Compensator] to run at unwind — the entry's nested stack, or its receipt when the receipt
// carries recovery state — or nil for an audit-only entry (a receipt with no recovery state).
//
// Returns:
//   - `Compensator`: the entry's reversal artifact, or nil when the entry contributes nothing to compensate.
func (e recoveryEntry) toCompensate() Compensator {

	if receipt, ok := e.compensator.(Receipt); ok && receipt.Compensator() == nil {
		return nil
	}
	return e.compensator
}

// recoveryEntryData is the codec-decoded shape of one stack entry, discriminated structurally.
//
// [RecoveryStack.UnmarshalJSON] and [RecoveryStack.UnmarshalYAML] decode a slice of these (each through its own codec),
// then [RecoveryStack.fromEntries] builds the live entries. An entry carrying `entries` is a nested `*RecoveryStack`
// (recursed through the same two unmarshalers); otherwise it is a receipt, whose base execution state decodes into
// `base` and whose whole flat object is retained as `fields` — a format-neutral map the concrete receipt resolves at
// re-arm via [Receipt.RestoreEncoded] (phase-8 step 42 slice 3b).
type recoveryEntryData struct {
	stack  *RecoveryStack // non-nil when the entry is a nested stack (carries `entries`)
	base   ReceiptData    // a receipt's decoded base execution state
	fields map[string]any // a receipt's whole decoded object (base plus concrete id references), for RestoreEncoded
}

// UnmarshalJSON discriminates one entry structurally: an object carrying `entries` is a nested [*RecoveryStack];
// otherwise it is a receipt whose base decodes into `base` and whose whole object is retained as `fields`.
//
// Parameters:
//   - `data`: the JSON for one entry.
//
// Returns:
//   - `error`: non-nil on malformed input.
func (e *recoveryEntryData) UnmarshalJSON(data []byte) error {

	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}

	if _, isStack := probe["entries"]; isStack {
		e.stack = &RecoveryStack{}
		return json.Unmarshal(data, e.stack)
	}

	if err := json.Unmarshal(data, &e.base); err != nil {
		return err
	}
	return json.Unmarshal(data, &e.fields)
}

// UnmarshalYAML mirrors [recoveryEntryData.UnmarshalJSON] for YAML: an `entries` key marks a nested stack, otherwise a
// receipt (base into `base`, the whole node retained as `fields`).
//
// Parameters:
//   - `unmarshal`: the YAML node decoder.
//
// Returns:
//   - `error`: non-nil on malformed input.
func (e *recoveryEntryData) UnmarshalYAML(unmarshal func(any) error) error {

	var probe map[string]any
	if err := unmarshal(&probe); err != nil {
		return err
	}

	if _, isStack := probe["entries"]; isStack {
		e.stack = &RecoveryStack{}
		return unmarshal(e.stack)
	}

	if err := unmarshal(&e.base); err != nil {
		return err
	}
	e.fields = probe
	return nil
}

// endregion

// region HELPER FUNCTIONS

// errStatus returns an error's message, or "" for a nil error — the encoded form of a receipt's [Receipt.Err].
//
// Parameters:
//   - `err`: the error to encode; nil encodes as "".
//
// Returns:
//   - `string`: the error message, or "" for nil.
func errStatus(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// receiptTypeForAction returns the concrete receipt type the action's Compensate companion declares.
//
// Resolution mirrors [invokeCompensateForReceipt]: [receiverRegistry.ActionByPath] for a Go-qualified name, falling
// back to [RuntimeEnvironment.ActionByName] for a dotted name. The companion's last parameter is the receipt type.
//
// Parameters:
//   - `runtimeEnvironment`: the `resume` environment (for the dotted-name fallback).
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
		resolved, resolveErr := runtimeEnvironment.ActionByName(ActionName(action))
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

// reconstructReceipt rebuilds the concrete receipt for an action from its codec-decoded envelope.
//
// The concrete type is read off the action's Compensate companion (the same companion compensation resolves at unwind),
// so `op` instantiates the right receipt without importing the provider or consulting a registry — the type comes from
// the action name every codec carries, which is what keeps reconstruction format-neutral (no type-URL registry needed).
//
// Parameters:
//   - `runtimeEnvironment`: the `resume` environment.
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

// retypeResult retypes a receipt's reloaded (untyped) result to its produced Go type, restoring full type fidelity.
//
// The produced type id was recorded at [ReceiptBase.Commit] (see [canonicalID]); it is authoritative even when a
// combinator's static return is `any`. Resolution goes through [receiverRegistry.ProductTypeByID] and the value through
// the [Convert] cascade (a struct hydrates from its map, a plain resource resolves through the rehydrated catalog). The
// produced-type-id is scoped to struct/scalar/resource: a result it cannot reconstruct — notably an observation
// record, which round-trips by re-observe-and-verify, not reconstruction — is left as-is, as are a nil result and
// an empty or unknown id, rather than failing the resume.
//
// Parameters:
//   - `runtimeEnvironment`: the `resume` environment, forwarded to [Convert] for env-sensitive types (resources).
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
		// The produced-type-id is scoped to struct/scalar/resource; a value it cannot reconstruct -- notably an
		// observation record, which round-trips by re-observe-and-verify, not reconstruction -- is left as-is rather
		// than failing the resume. A consumer that needs the concrete type fails at its own dispatch.
		return nil
	}

	receipt.receiptBase().result = retyped
	return nil
}

// endregion
