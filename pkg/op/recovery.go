// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"fmt"
)

// errDrifted indicates that a reconcile check detected external modification, making compensation unsafe.
//
// The entry is skipped during Unwind.
var errDrifted = errors.New("state has drifted: compensation unsafe")

// recoveryEntry captures one entry on a [RecoveryStack].
//
// Three kinds of entries coexist during the saga-shape transition:
//
//   - Receipt-bearing: receipt is non-nil; compensate is pre-bound by [RecoveryStack.Push] to invoke the
//     action's Compensate companion at unwind time. Persistable via [RecoveryStack.MarshalJSON].
//   - Nested: sub is non-nil; compensate runs sub.Unwind() as a transactional unit. Persistable.
//   - Closure-only (legacy): compensate and compensateState are populated by the lower-level Push API; no
//     receipt, no sub. Not persistable — fails at marshal time. Scheduled for deletion in step 6.
type recoveryEntry struct {
	receipt         Receipt          // receipt-bearing entries; nil otherwise
	sub             *RecoveryStack   // nested entries; nil otherwise
	compensate      func(any) error  // pre-bound undo (receipt.Resource for receipt entries; sub for nested)
	compensateState any              // closure-only entries; nil for receipt and nested
	reconcile       func(any) (bool, error)
	reconcileState  any
}

// RecoveryStack accumulates compensable operations in LIFO order.
//
// On Unwind, each entry is reconciled (if a reconcile function was provided) and then compensated in reverse order.
type RecoveryStack struct {
	entries []recoveryEntry
}

// NewRecoveryStack creates an empty RecoveryStack.
func NewRecoveryStack() *RecoveryStack {
	return &RecoveryStack{}
}

// Do invokes a closure and, if it succeeds, pushes a recovery entry onto the stack.
//
// If invoke returns an error, the stack is unchanged and the error is returned without unwinding.
func (s *RecoveryStack) Do(
	invoke func() (undoState any, reconcileState any, err error),
	compensate func(any) error,
	reconcile func(any) (bool, error),
) error {
	undoState, reconcileState, err := invoke()
	if err != nil {
		return err
	}
	s.Push(compensate, reconcile, undoState, reconcileState)
	return nil
}

// Push manually adds a recovery entry to the stack.
//
// Deprecated: closure-only entries are scheduled for deletion in saga-shape step 6. New callers use
// [RecoveryStack.PushReceipt] or [RecoveryStack.PushNested] which support persistence and the new contract.
func (s *RecoveryStack) Push(
	compensate func(any) error,
	reconcile func(any) (bool, error),
	undoState any,
	reconcileState any,
) {
	s.entries = append(s.entries, recoveryEntry{
		compensate:      compensate,
		reconcile:       reconcile,
		compensateState: undoState,
		reconcileState:  reconcileState,
	})
}

// PushReceipt commits a receipt under the supplied action name and appends it as a receipt-bearing entry.
//
// The receipt's [Receipt.Commit] is invoked first to stamp the transactionID and action name (idempotent
// when already committed). The receipt's resource provides the [RuntimeEnvironment] used at unwind time to
// resolve the [Compensate] companion via [ReceiverRegistry.ActionByFullName] — no context is captured here.
//
// Parameters:
//   - receipt: the [Receipt] returned by the forward action; must be non-nil and carry a [Resource] with an
//     [RuntimeEnvironment] reachable via [Receipt.Resource].
//   - actionName: the canonical action name (<pkg-path>.<receiverName>.<methodName>) — typically the value
//     returned by [Action.FullName] at the executor's push site or [Method.ActionName] at [Method.Invoke].
//
// Returns:
//   - error: non-nil if the receipt is nil, lacks a resource, lacks an [RuntimeEnvironment], or [Receipt.Commit]
//     fails.
func (s *RecoveryStack) PushReceipt(receipt Receipt, actionName string) error {

	if receipt == nil {
		return errors.New("PushReceipt: receipt is nil")
	}

	if receipt.Resource() == nil || receipt.Resource().RuntimeEnvironment() == nil {
		return errors.New("PushReceipt: receipt has no resource or no execution context")
	}

	if err := receipt.Commit(actionName); err != nil {
		return fmt.Errorf("PushReceipt: commit %s: %w", actionName, err)
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

// PushNested appends a sub-stack as a single transactional entry on this stack.
//
// The nested entry preserves the saga boundary: at unwind time the sub-stack is unwound as a unit (its own
// LIFO walk, its own error aggregation) before the outer stack continues. Used by the executor to splice a
// completed subgraph's local stack — or a multi-receipt action's engine-built sub-stack — into its parent.
//
// Parameters:
//   - sub: the sub-stack to nest. May be nil — a nil sub is treated as an empty saga and contributes nothing.
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

// Unwind compensates all entries in LIFO order.
//
// For each entry:
//
//  1. If reconcile is non-nil, it is called first. If reconcile returns false (drifted), the entry is skipped and an
//     ErrDrifted is collected.
//
//  2. If reconcile is nil or returns true (safe), compensate is called.
//
// All entries are attempted regardless of individual failures. Errors are joined via errors.Join.
func (s *RecoveryStack) Unwind() error {
	var errs []error
	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]

		if entry.reconcile != nil {
			safe, reconcileErr := entry.reconcile(entry.reconcileState)
			if reconcileErr != nil {
				errs = append(errs, reconcileErr)
				continue
			}
			if !safe {
				errs = append(errs, errDrifted)
				continue
			}
		}

		if entry.compensate != nil {
			if err := entry.compensate(entry.compensateState); err != nil {
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

// MarshalJSON encodes the stack's persistable entries as a JSON object.
//
// Wire form: `{"entries": [...]}` where each element is either `{"receipt": {...}}` or `{"sub": {...}}` —
// disjoint field sets, no `kind` tag. Recursion is automatic — nested sub-stacks serialize via their own
// MarshalJSON when the encoder walks the `sub` field. Closure-only entries (legacy [RecoveryStack.Push]) are
// not persistable; encountering one returns an error per the "fail loudly" policy of the saga design.
//
// Returns:
//   - []byte: JSON-encoded `{"entries": [...]}`.
//   - error: any error from [json.Marshal], or [errClosureEntryNotPersistable] if a closure-only entry is
//     present.
func (s *RecoveryStack) MarshalJSON() ([]byte, error) {

	v, err := s.MarshalYAML()
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// MarshalYAML returns the stack's persistable entries as an anonymous struct value the YAML encoder walks.
//
// Source of truth for the wire shape; [RecoveryStack.MarshalJSON] delegates here. Tags on the anonymous struct
// fields cover both encoders. Closure-only entries fail with [errClosureEntryNotPersistable] — the saga design
// forbids non-receipt entries from existing on a persisted stack.
//
// Returns:
//   - any: the populated anonymous struct for the YAML encoder to walk.
//   - error: [errClosureEntryNotPersistable] if a closure-only entry is present.
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
		default:
			return nil, errClosureEntryNotPersistable
		}
	}

	return struct {
		Entries []any `json:"entries" yaml:"entries"`
	}{Entries: entries}, nil
}

// errClosureEntryNotPersistable is returned by [RecoveryStack.MarshalJSON] / [RecoveryStack.MarshalYAML] when
// a closure-only entry is encountered.
//
// Closure-only entries originate from the deprecated [RecoveryStack.Push] (compensate, reconcile, undoState,
// reconcileState) API and cannot be reconstructed at reload time. The saga design treats every saga as
// universally persistable; this sentinel surfaces the violation rather than skipping silently.
var errClosureEntryNotPersistable = errors.New("RecoveryStack: closure-only entry is not persistable")

// invokeCompensateForReceipt resolves a receipt's [Compensate] companion via the registry and invokes it.
//
// Used by [RecoveryStack.PushReceipt]'s pre-bound compensate closure at [RecoveryStack.Unwind] time. The
// receipt's resource carries the [RuntimeEnvironment] from which the [ReceiverRegistry] is reached;
// [ReceiverRegistry.ActionByFullName] looks up the action by its committed action name; [Method.Undo] dispatches
// to the [Compensate] companion with the receipt as the complement.
//
// Parameters:
//   - receipt: the [Receipt] whose forward action's [Compensate] companion to invoke.
//
// Returns:
//   - error: non-nil if the receipt has no resource / context, the action is not registered, the provider
//     instance cannot be cached, or the [Compensate] companion returns an error. [ErrNotCompensable] from the
//     companion is treated as success (logged elsewhere; not surfaced as an error).
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

	if undoErr := method.Undo(provider, receipt); undoErr != nil {
		if errors.Is(undoErr, ErrNotCompensable) {
			return nil
		}
		return undoErr
	}

	return nil
}
