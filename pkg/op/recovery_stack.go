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
type RecoveryStack struct {
	entries []recoveryEntry
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

// NewRecoveryStack creates an empty RecoveryStack.
func NewRecoveryStack() *RecoveryStack {
	return &RecoveryStack{}
}

// Push appends a [Receipt] onto the stack as an audit-trail entry.
//
// Step 12 broadens [RecoveryStack] from a compensable-only ledger to an every-dispatch ledger: the executor calls Push
// at every dispatch exit (cancellation, Do-error, success). When the receipt carries a [Resource] with a live
// [RuntimeEnvironment] AND the receipt's complement is non-nil, the entry is also wired for compensation — [Unwind]
// will invoke the action's Compensate companion at rollback. Otherwise, the entry is audit-only and [Unwind] skips it.
//
// The receipt is committed (idempotently) using its already-stamped action name. Receipts without a stamped action name
// skip commit; their TransactionID stays empty until a later [Receipt.Commit] runs.
//
// Parameters:
//   - `receipt`: the receipt to push. Must be non-nil.
//
// Returns:
//   - `error`: non-nil if receipt is nil or commit fails.
func (s *RecoveryStack) Push(receipt Receipt) error {

	if receipt == nil {
		return errors.New("RecoveryStack.Push: receipt is nil")
	}

	entry := recoveryEntry{receipt: receipt}

	// Compensation binding: the receipt is compensable iff it carries a Resource (with a non-nil RuntimeEnvironment)
	// and a non-nil complement. Audit-only entries (no resource OR no complement) leave compensate nil; Unwind walks
	// past them without invoking.

	if receipt.Resource() != nil && receipt.Resource().RuntimeEnvironment() != nil && receipt.Complement() != nil {
		entry.compensate = func(_ any) error { return invokeCompensateForReceipt(receipt) }
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

// ResultByUnitID returns the most recent receipt's [Receipt.Result] for the unit identified by `unitID`.
//
// The stack is the source of truth for per-dispatch results: every dispatch exit pushes a receipt with the producing
// unit's ID and result, so promise-style "look up an upstream unit's output" queries walk the stack instead of a
// separate results map. Walk order is LIFO so a retried unit returns its latest outcome.
//
// Nested substacks are not searched. Combinators that own a substack also push an audit receipt at the parent level
// carrying the combinator's overall result; that parent-level receipt is what promise resolution finds.
//
// Parameters:
//   - `unitID`: the [ExecutableUnit.ID] of the producing unit.
//
// Returns:
//   - `any`: the matched receipt's result, or nil when no match is found.
//   - `bool`: true when a matching receipt was found, false otherwise.
func (s *RecoveryStack) ResultByUnitID(unitID string) (any, bool) {

	for i := len(s.entries) - 1; i >= 0; i-- {
		r := s.entries[i].receipt
		if r != nil && r.UnitID() == unitID {
			return r.Result(), true
		}
	}

	return nil, false
}

// Receipts returns every receipt-bearing entry on this stack, descending into nested substacks, in push
// order (oldest first).
//
// Unlike [RecoveryStack.ResultByUnitID] — which searches only this stack's top level — Receipts flattens nested
// substacks so callers that summarize a whole execution (see [Trace.Summarize]) observe every dispatched unit's
// receipt, including per-iteration combinator children. Nested-stack marker entries contribute their contained
// receipts, not themselves.
//
// Returns:
//   - []Receipt: the flattened receipts in push order; empty when the stack holds none.
func (s *RecoveryStack) Receipts() []Receipt {

	var receipts []Receipt
	for _, entry := range s.entries {
		switch {
		case entry.receipt != nil:
			receipts = append(receipts, entry.receipt)
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
// Used by [RecoveryStack.pushReceipt]'s pre-bound compensation closure at [RecoveryStack.Unwind] time. The receipt's
// resource carries the [RuntimeEnvironment] from which the [ReceiverRegistry] is reached.
// [ReceiverRegistry.ActionByPath] looks up the action by its committed action name; [Method.Undo] dispatches to the
// [Compensate] companion with the receipt as the complement.
//
// [ErrNotCompensable] from the companion is treated as a success (logged elsewhere; not surfaced as an error).
func invokeCompensateForReceipt(receipt Receipt) error {

	resource := receipt.Resource()

	if resource == nil || resource.RuntimeEnvironment() == nil {
		return fmt.Errorf("invokeCompensateForReceipt: receipt %s has no resource context", receipt.ActionPath())
	}

	runtimeEnvironment := resource.RuntimeEnvironment()

	providerReceiverType, method, ok := runtimeEnvironment.ReceiverRegistry.ActionByPath(receipt.ActionPath())
	if !ok {
		return fmt.Errorf("invokeCompensateForReceipt: no registered action %q", receipt.ActionPath())
	}

	provider, err := runtimeEnvironment.cachedProvider(providerReceiverType)
	if err != nil {
		return fmt.Errorf("invokeCompensateForReceipt: cache provider %q: %w", providerReceiverType.Name(), err)
	}

	activationRecord := &ActivationRecord{RuntimeEnvironment: runtimeEnvironment, Context: runtimeEnvironment.Context}

	if undoErr := method.Undo(activationRecord, provider, receipt); undoErr != nil {
		if errors.Is(undoErr, ErrNotCompensable) {
			return nil
		}
		return undoErr
	}

	return nil
}
