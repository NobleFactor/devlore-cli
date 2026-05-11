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
//   - Nested: sub is non-nil; compensate runs sub.Unwind() as a transactional unit. Persistable.
type recoveryEntry struct {
	receipt    Receipt         // receipt-bearing entries; nil otherwise
	sub        *RecoveryStack  // nested entries; nil otherwise
	compensate func(any) error // pre-bound undo (receipt.Resource for receipt entries; sub for nested)
}

// NewRecoveryStack creates an empty RecoveryStack.
func NewRecoveryStack() *RecoveryStack {
	return &RecoveryStack{}
}

// PushComplement dispatches a complement onto the stack by shape.
//
// The classifier in [Method.NewMethod] guarantees one of three shapes for any compensable action: nil (the action ran
// but produced no undo state), a [Receipt] (single-output compensable), or a [*RecoveryStack] (multi-output compensable
// whose receipts [Method.Invoke] has already wrapped into a substack). Any other shape is silently dropped because it
// is unreachable by construction.
//
// Parameters:
//   - actionName: the canonical action name for receipt-bearing entries; ignored for nested stacks and nil.
//   - complement: the complement value returned by [Method.Invoke].
func (s *RecoveryStack) PushComplement(actionName string, complement any) {

	switch v := complement.(type) {
	case nil:
		return
	case Receipt:
		_ = s.pushReceipt(v, actionName)
	case *RecoveryStack:
		s.PushNested(v)
	}
}

// pushReceipt commits a receipt under the supplied action name and appends it as a receipt-bearing entry.
//
// The receipt's [Receipt.Commit] is invoked first to stamp the transactionID and action name (idempotent when already
// committed). The receipt's resource provides the [RuntimeEnvironment] used at unwind time to resolve the [Compensate]
// companion via [ReceiverRegistry.ActionByFullName] — no context is captured here.
func (s *RecoveryStack) pushReceipt(receipt Receipt, actionName string) error {

	if receipt == nil {
		return errors.New("pushReceipt: receipt is nil")
	}

	if receipt.Resource() == nil || receipt.Resource().RuntimeEnvironment() == nil {
		return errors.New("pushReceipt: receipt has no resource or no execution context")
	}

	if err := receipt.Commit(actionName); err != nil {
		return fmt.Errorf("pushReceipt: commit %s: %w", actionName, err)
	}

	captured := receipt
	compensate := func(_ any) error {
		return invokeCompensateForReceipt(captured)
	}

	s.entries = append(s.entries, recoveryEntry{
		receipt:    captured,
		compensate: compensate,
	})

	return nil
}

// PushNested appends a substack as a single transactional entry on this stack.
//
// The nested entry preserves the saga boundary: at unwind time the substack is unwound as a unit (its own LIFO walk,
// its own error aggregation) before the outer stack continues.
//
// A nil sub is treated as an empty saga and contributes nothing.
func (s *RecoveryStack) PushNested(sub *RecoveryStack) {

	if sub == nil {
		return
	}

	captured := sub
	compensate := func(_ any) error { return captured.Unwind() }

	s.entries = append(s.entries, recoveryEntry{
		sub:        captured,
		compensate: compensate,
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
		case e.sub != nil:
			entries = append(entries, struct {
				Sub *RecoveryStack `json:"sub" yaml:"sub"`
			}{Sub: e.sub})
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
// [ReceiverRegistry.ActionByFullName] looks up the action by its committed action name; [Method.Undo] dispatches to the
// [Compensate] companion with the receipt as the complement.
//
// [ErrNotCompensable] from the companion is treated as a success (logged elsewhere; not surfaced as an error).
func invokeCompensateForReceipt(receipt Receipt) error {

	resource := receipt.Resource()
	if resource == nil || resource.RuntimeEnvironment() == nil {
		return fmt.Errorf("invokeCompensateForReceipt: receipt %s has no resource context", receipt.Action())
	}

	ctx := resource.RuntimeEnvironment()

	prt, method, ok := ctx.Registry.ActionByFullName(receipt.Action())
	if !ok {
		return fmt.Errorf("invokeCompensateForReceipt: no registered action %q", receipt.Action())
	}

	provider, err := ctx.cachedProvider(prt)
	if err != nil {
		return fmt.Errorf("invokeCompensateForReceipt: cache provider %q: %w", prt.Name(), err)
	}

	activationRecord := &ActivationRecord{Runtime: ctx, Context: ctx.Context}
	if undoErr := method.Undo(activationRecord, provider, receipt); undoErr != nil {
		if errors.Is(undoErr, ErrNotCompensable) {
			return nil
		}
		return undoErr
	}

	return nil
}
