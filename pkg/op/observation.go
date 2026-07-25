// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// ObservationBase is the back-link-plus-existence surface shared by every concrete observation record.
//
// An observation is a point-in-time metadata snapshot of a [Resource] — a fact *about* a thing in the world, not a
// thing whose existence is in question — so it is not a [Resource] and never enters the [ResourceCatalog] (ruled
// 2026-07-14; see docs/architecture/4-resource-management.md §6.1). Its identity comes from the resource it references
// by pointer value: an observation mints no URI and carries no content hash of its own. Runtime tracking and
// verification ride the execution record — an observation produced by an observe action is that node's result, carried
// on its receipt and serialized in the trace; resume re-observes rather than reconstructs.
//
// Concrete observation types embed ObservationBase and contribute their own per-provider measurement fields plus an
// optional String() debug helper.
type ObservationBase struct {

	// OfResource is the [Resource] this observation is of — the record's identity anchor. Set at construction; non-nil
	// invariant asserted by [NewObservationBase]. Access the observed identity as `o.OfResource.URI()`.
	OfResource Resource

	// Exists is true when the observed thing was present at observation time. When false, the concrete observation's
	// measurement fields carry zero values.
	Exists bool
}

// NewObservationBase constructs an ObservationBase anchored to the resource it observes.
//
// Parameters:
//   - `ofResource`: the [Resource] this observation is of. Must be non-nil (asserted).
//   - `exists`: true when the observed thing was present at observation time.
//
// Returns:
//   - ObservationBase: the constructed base.
func NewObservationBase(ofResource Resource, exists bool) ObservationBase {

	assert.NonZero("ofResource", ofResource)

	return ObservationBase{
		OfResource: ofResource,
		Exists:     exists,
	}
}
