// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package pkg

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Observation captures the runtime-observed state of a [*Resource] (a host package) at the moment it was observed.
//
// Distinct from [Resource], which carries identity (the purl URI, `Name`, `Type`). An observation is a point-in-time
// metadata snapshot record — not a [Resource], never cataloged — whose identity comes from the resource it references
// ([op.ObservationBase.OfResource], by pointer value). It embeds [op.ObservationBase] (the back-link +
// [op.ObservationBase.Exists]) and adds the package-specific measurement field: `Version`.
type Observation struct {
	op.ObservationBase

	// Version is the version string the platform's package manager reports for the package at
	// observation time. Empty when `Exists` is false.
	Version string
}

// NewObservation constructs a *Observation anchored to the resource it observes.
//
// Parameters:
//   - `ofResource`: the [*Resource] this observation is of. Must be non-nil (asserted by [op.NewObservationBase]).
//   - `exists`: true when the package was installed at observation time.
//   - `version`: the installed version reported by the package manager.
//
// Returns:
//   - `*Observation`: the constructed observation.
func NewObservation(ofResource *Resource, exists bool, version string) *Observation {

	return &Observation{
		ObservationBase: op.NewObservationBase(ofResource, exists),
		Version:         version,
	}
}

// region EXPORTED METHODS

// region Behaviors

// String returns a debug-oriented single-line representation of the observation.
//
// Returns:
//   - `string`: `pkg.Observation{of=<OfResource.URI()>, exists=<bool>, version=<string>}`.
func (o *Observation) String() string {
	return fmt.Sprintf("pkg.Observation{of=%s, exists=%t, version=%s}",
		o.OfResource.URI(), o.Exists, o.Version)
}

// endregion

// endregion
