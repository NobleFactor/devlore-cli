// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"sync"
)

// InvocationRegistry is the session-scoped ledger of every [Invocation] constructed during plan-time evaluation.
//
// Entries are appended to ordered in creation order and indexed in byLabel by the label under which they were
// registered. Auto-labeling uses a per-provider.method counter: [InvocationRegistry.AutoLabel] formats a monotonic
// label of the form "<providerMethod>#<N>" whose ordinal is unique for that providerMethod within the registry's
// lifetime. Every call is protected by a single mutex; the registry is written only during plan-time evaluation and
// frozen after plan.run is invoked (Phase 8 invariant I3).
type InvocationRegistry struct {
	byLabel map[string]*Invocation
	counts  map[string]int
	mutex   sync.Mutex
	ordered []*Invocation
}

// NewInvocationRegistry creates an empty registry.
//
// Returns:
//   - *InvocationRegistry: the empty registry.
func NewInvocationRegistry() *InvocationRegistry {
	return &InvocationRegistry{
		byLabel: make(map[string]*Invocation),
		counts:  make(map[string]int),
	}
}

// region EXPORTED METHODS

// All returns every registered invocation in creation order.
//
// The returned slice is a shallow copy safe for the caller to iterate without holding the registry lock. It is used by
// the plan-end orphan walk (D4) and the plan-time type-check pass (D8).
//
// Returns:
//   - []*Invocation: the registered invocations in creation order.
func (r *InvocationRegistry) All() []*Invocation {

	r.mutex.Lock()
	defer r.mutex.Unlock()

	invocations := make([]*Invocation, len(r.ordered))
	copy(invocations, r.ordered)

	return invocations
}

// AutoLabel returns the next auto-generated label for `providerMethod`.
//
// The label has the form "<providerMethod>#<N>" where N is a 1-based ordinal that increments monotonically per
// `providerMethod` across the registry's lifetime. Callers use this when the author did not supply an explicit label
// via [Options.Label].
//
// Parameters:
//   - providerMethod: the "<provider>.<method>" identifier (e.g., "file.write_text", "plan.choose").
//
// Returns:
//   - string: the formatted auto-label.
func (r *InvocationRegistry) AutoLabel(providerMethod string) string {

	r.mutex.Lock()
	defer r.mutex.Unlock()

	r.counts[providerMethod]++

	return fmt.Sprintf("%s#%d", providerMethod, r.counts[providerMethod])
}

// ByLabel returns the invocation registered under label, or nil if no such label exists.
//
// Parameters:
//   - `label`: the label to look up.
//
// Returns:
//   - `*Invocation`: the registered invocation, or nil if not found.
func (r *InvocationRegistry) ByLabel(label string) *Invocation {

	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.byLabel[label]
}

// Register appends invocation to the ordered list and inserts it into byLabel under the given label.
//
// Duplicate labels return an error without modifying either structure. Callers are expected to either supply a
// user-provided label (from [Options.Label]) or an auto-generated one from [InvocationRegistry.AutoLabel].
//
// Parameters:
//   - `label`: the unique label for this invocation.
//   - `invocation`: the invocation to register.
//
// Returns:
//   - `error`: non-nil if label is already registered.
func (r *InvocationRegistry) Register(label string, invocation *Invocation) error {

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.byLabel[label]; exists {
		return fmt.Errorf("duplicate invocation label %q", label)
	}

	r.ordered = append(r.ordered, invocation)
	r.byLabel[label] = invocation

	return nil
}

// endregion
