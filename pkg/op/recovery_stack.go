// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"fmt"
)

// RecoveryStack accumulates compensable operations in LIFO order.
//
// On Unwind, each entry is compensated in reverse order. All entries are attempted regardless of individual failures;
// errors are joined via errors.Join.
//
// Per-subgraph executors own one stack each, chained into a tree (phase-8 step 28). A child stack is nested *down*
// into its parent via [RecoveryStack.PushNested], so Unwind cascades compensation through the tree; and it points
// *up* at its parent via the `parent` field, so [RecoveryStack.ResultByUnitID] walks the chain to resolve a promise
// against an ancestor stack's receipt. The nesting is durable (serialized in a [Trace]); the parent pointer is
// transient — re-derived from the nesting on load, never serialized.
type RecoveryStack struct {

	// entries is the LIFO list of compensable and audit entries pushed onto this stack.
	entries []recoveryEntry

	// parent is the enclosing subgraph's stack, or nil at the root of the chain. [RecoveryStack.ResultByUnitID] walks
	// up through it for promise resolution. Never serialized; re-derived from the nesting on [Trace] load.
	parent *RecoveryStack
}

// recoveryEntry captures one entry on a [RecoveryStack].
//
// Two kinds of entries exist:
//
//   - Receipt-bearing: receipt is non-nil; compensate is pre-bound by [RecoveryStack.pushReceipt] to invoke the
//     action's Compensate companion at unwind time. Persistable via [RecoveryStack.MarshalJSON].
//
//   - Nested: recoveryStack is non-nil; compensate runs sub.Unwind() as a transactional unit. Persistable.
type recoveryEntry struct {
	recoveryStack *RecoveryStack  // nested entries; nil otherwise
	receipt       Receipt         // receipt-bearing entries; nil otherwise
	compensate    func(any) error // pre-bound undo (receipt.Resource for receipt entries; recoveryStack for nested)
}

// NewRecoveryStack creates an empty RecoveryStack at the root of a chain (no parent).
//
// Returns:
//   - `*RecoveryStack`: the new root stack.
func NewRecoveryStack() *RecoveryStack {
	return newRecoveryStack(nil)
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
// at every dispatch exit (cancellation, Do-error, success). When the receipt carries a non-nil complement, the entry is
// also wired for compensation — [Unwind] invokes the action's Compensate companion at rollback, reached through
// `runtimeEnvironment` rather than the receipt's resource (so a resource-less complement still compensates). Otherwise,
// the entry is audit-only and [Unwind] skips it.
//
// The receipt is committed (idempotently) using its already-stamped action name. Receipts without a stamped action name
// skip commit; their TransactionID stays empty until a later [Receipt.Commit] runs.
//
// Parameters:
//   - `receipt`: the receipt to push. Must be non-nil.
//   - `runtimeEnvironment`: the executor's environment, used to resolve and invoke the Compensate companion at unwind.
//
// Returns:
//   - `error`: non-nil if receipt is nil or commit fails.
func (s *RecoveryStack) Push(receipt Receipt, runtimeEnvironment *RuntimeEnvironment) error {

	if receipt == nil {
		return errors.New("RecoveryStack.Push: receipt is nil")
	}

	entry := recoveryEntry{receipt: receipt}

	// Compensation binding: a receipt is compensable iff it carries a non-nil complement — the per-call undo state, of
	// any shape (a resource action's receipt, a recovery stack, or a slice of stacks). The env comes from the executor,
	// not the receipt's resource, so a resource-less complement (a combinator or file.WalkTree stack) still compensates
	// via its Undo companion. Audit-only entries (no complement) leave compensate nil; Unwind walks past them.

	if receipt.Complement() != nil {
		entry.compensate = func(_ any) error { return invokeCompensateForReceipt(runtimeEnvironment, receipt) }
	}

	s.entries = append(s.entries, entry)
	return nil
}

// PushNested appends a substack as a single transactional entry on this stack.
//
// The nested entry preserves the saga boundary: at unwind time the substack is unwound as a unit (its own LIFO walk,
// its own error aggregation) before the outer stack continues.
//
// A nil sub is treated as an empty saga and contributes nothing.
func (s *RecoveryStack) PushNested(recoveryStack *RecoveryStack) {

	if recoveryStack == nil {
		return
	}

	compensate := func(_ any) error { return recoveryStack.Unwind() }

	s.entries = append(s.entries, recoveryEntry{
		recoveryStack: recoveryStack,
		compensate:    compensate,
	})
}

// Unwind rolls back all stack entries in LIFO order.
//
// All entries are attempted regardless of individual failures. Errors are joined via errors.Join.
func (s *RecoveryStack) Unwind() error {

	var errs []error

	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		if entry.compensate != nil {
			if err := entry.compensate(nil); err != nil {
				errs = append(errs, err)
			}
		}
	}

	s.entries = nil
	return errors.Join(errs...)
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
			r := stack.entries[i].receipt
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
		r := s.entries[i].receipt
		if r != nil && r.UnitID() == unitID {
			return r, true
		}
	}

	return nil, false
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
		r := s.entries[i].receipt
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
// receipts, not themselves; and a receipt whose complement is itself a [RecoveryStack] — a subgraph or file.WalkTree
// dispatch — also contributes that child stack's receipts, since the child no longer rides a separate nested entry.
//
// Returns:
//   - []Receipt: the flattened receipts in push order; empty when the stack holds none.
func (s *RecoveryStack) Receipts() []Receipt {

	var receipts []Receipt
	for _, entry := range s.entries {
		switch {
		case entry.receipt != nil:
			receipts = append(receipts, entry.receipt)
			if childStack, ok := entry.receipt.Complement().(*RecoveryStack); ok {
				receipts = append(receipts, childStack.Receipts()...)
			}
		case entry.recoveryStack != nil:
			receipts = append(receipts, entry.recoveryStack.Receipts()...)
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
// Wire form: `{"entries": [...]}` where each element is either `{"receipt": {...}}` or `{"sub": {...}}` — disjoint
// field sets, no `kind` tag. Recursion is automatic. Nested substacks serialize via their own MarshalJSON when the
// encoder walks the `sub` field.
func (s *RecoveryStack) MarshalJSON() ([]byte, error) {

	v, err := s.MarshalYAML()
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// MarshalYAML returns the stack's entries as an anonymous struct value the YAML encoder walks.
//
// Source of truth for the wire shape; [RecoveryStack.MarshalJSON] delegates here.
func (s *RecoveryStack) MarshalYAML() (any, error) {

	entries := make([]any, 0, len(s.entries))

	for _, e := range s.entries {
		switch {
		case e.recoveryStack != nil:
			entries = append(entries, struct {
				Sub *RecoveryStack `json:"sub" yaml:"sub"`
			}{Sub: e.recoveryStack})
		case e.receipt != nil:
			entries = append(entries, struct {
				Receipt Receipt `json:"receipt" yaml:"receipt"`
			}{Receipt: e.receipt})
		}
	}

	return struct {
		Entries []any `json:"entries" yaml:"entries"`
	}{Entries: entries}, nil
}

// invokeCompensateForReceipt resolves a receipt's [Compensate] companion via the registry and invokes it.
//
// Used by [RecoveryStack.Push]'s pre-bound compensation closure at [RecoveryStack.Unwind] time. The env is supplied by
// the executor — not read off the receipt's resource — so a resource-less complement (a combinator or file.WalkTree
// recovery stack) still resolves its companion. [ReceiverRegistry.ActionByPath] looks up the action by its committed
// Go-qualified name; a receipt that recorded the dotted name instead (a unit that bound its action by name — the graph
// root, every combinator) falls back to [RuntimeEnvironment.ActionByName]. [Method.Undo] then dispatches to the
// [Compensate] companion with [Receipt.Complement] as the undo state.
//
// [ErrNotCompensable] from the companion is treated as a success (logged elsewhere; not surfaced as an error).
//
// Parameters:
//   - `runtimeEnvironment`: the executor's environment; resolves the [ReceiverRegistry] provider for the action.
//   - `receipt`: the audit receipt whose [Receipt.Complement] is the undo state handed to the companion.
//
// Returns:
//   - `error`: non-nil when the env is nil, the action is unregistered, the provider fails, or the companion fails.
func invokeCompensateForReceipt(runtimeEnvironment *RuntimeEnvironment, receipt Receipt) error {

	if runtimeEnvironment == nil {
		return fmt.Errorf("invokeCompensateForReceipt: receipt %s has no runtime environment", receipt.ActionPath())
	}

	providerReceiverType, method, ok := ReceiverRegistry().ActionByPath(receipt.ActionPath())

	if !ok {
		// A unit that binds its action by name (the graph root and every combinator) records the dotted action name —
		// e.g. "flow.subgraph" — as its action path, not the Go-qualified ActionName that ActionByPath keys on. Resolve
		// the dotted name through the environment's action resolver (the same one dispatch uses) and retry on the
		// resolved Go-qualified path.
		resolved, resolveErr := runtimeEnvironment.ActionByName(receipt.ActionPath())
		if resolveErr == nil && resolved != nil {
			providerReceiverType, method, ok = ReceiverRegistry().ActionByPath(resolved.FullName())
		}
	}

	if !ok {
		return fmt.Errorf("invokeCompensateForReceipt: no registered action %q", receipt.ActionPath())
	}

	provider, err := runtimeEnvironment.cachedProvider(providerReceiverType)
	if err != nil {
		return fmt.Errorf("invokeCompensateForReceipt: cache provider %q: %w", providerReceiverType.Name(), err)
	}

	activationRecord := &ActivationRecord{RuntimeEnvironment: runtimeEnvironment, Context: runtimeEnvironment.Context}

	if undoErr := method.Undo(activationRecord, provider, receipt.Complement()); undoErr != nil {
		if errors.Is(undoErr, ErrNotCompensable) {
			return nil
		}
		return undoErr
	}

	return nil
}
