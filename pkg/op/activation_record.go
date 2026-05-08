// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// ActivationRecord is the per-invocation data record threaded through every [Action.Do] call.
//
// The framework constructs one [ActivationRecord] per dispatch and passes it to [Action.Do]. Forward methods
// and downstream framework layers (notably the [ResourceCatalog]) read shared session state via
// [ActivationRecord.Runtime] and per-call state directly off the record (`NodeID`, future per-call fields).
//
// Each goroutine-driven dispatch holds its own [ActivationRecord]; pointer fields on `Runtime` (Catalog,
// Status, RecoverySite, Registry, etc.) share underlying instances with their own internal synchronization.
// Concurrent dispatches cannot race on per-call fields because they hold different records.
//
// Lifecycle: created by the executor before [Action.Do]; consumed during the dispatch; discarded afterward.
// No persistent identity, no registry — each record is unique to one invocation.
type ActivationRecord struct {

	// Runtime is the session-scope execution context. Shared across every concurrent dispatch in the same
	// session; never mutated mid-execution.
	Runtime *RuntimeEnvironment

	// NodeID is the identity of the producing node for this dispatch. Stamped on Resources interned during
	// the dispatch so downstream consumers can derive producer→consumer edges via [ExtractResource]. Empty
	// outside dispatch (planning, rehydration, discovery).
	NodeID string
}