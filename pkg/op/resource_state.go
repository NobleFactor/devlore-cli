// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// State is the lifecycle state of a catalog entry.
//
// Three states: Pending (initial — entry exists in the namespace but the underlying resource has not yet been
// observed or produced), Active (observation succeeded or the producer created the resource; metadata is
// populated), Gone (the catalog attempted to access the underlying resource via Resolve and it failed; Gone is
// reactive, not driven by explicit "delete" calls).
//
// The state field is mutated by catalog code only; provider implementations have no setter. See
// docs/architecture/4-resource-management.md §3.1 and §6.2 for the full lifecycle spec.
type State int

const (
	// Pending is the zero value; every new catalog entry is born here.
	Pending State = iota

	// Active means the resource has been observed (discovery path) or freshly created (production path).
	Active

	// Gone means a call to Resolve has failed on this entry; the catalog has tried and the resource is not
	// where it should be.
	Gone
)

// String returns the canonical lowercase rendering of the state.
//
// Returns:
//   - string: "pending", "active", or "gone".
func (s State) String() string {

	switch s {
	case Pending:
		return "pending"
	case Active:
		return "active"
	case Gone:
		return "gone"
	}

	assert.Unreachable(fmt.Sprintf("op.State.String: invalid state value %d", int(s)))
	return ""
}