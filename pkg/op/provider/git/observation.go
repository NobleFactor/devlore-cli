// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package git

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Observation captures the runtime-observed state of a [Resource]'s on-disk clone at the moment it was observed.
//
// Distinct from [Resource], which carries identity (URI, [fsroot.Path], and the identity-extension intent fields
// `HEAD` and `Ref` from the plan). An observation is a point-in-time metadata snapshot record — not a [Resource],
// never cataloged — whose identity comes from the resource it references ([op.ObservationBase.OfResource], by pointer
// value). It embeds [op.ObservationBase] (the back-link + [op.ObservationBase.Exists]) and adds the git-specific
// measurement fields: `ObservedHEAD`, `ObservedRef`, `Bare`, `Dirty`, `Remotes`.
type Observation struct {
	op.ObservationBase

	// ObservedHEAD is the commit SHA the on-disk clone currently points at. May differ from the
	// observed [Resource]'s `HEAD` (which is plan-time intent).
	ObservedHEAD string

	// ObservedRef is the branch / tag / ref name the on-disk clone is positioned at. May differ
	// from the observed [Resource]'s `Ref` (which is plan-time intent).
	ObservedRef string

	// Bare reports whether the on-disk repository is bare (no working tree).
	Bare bool

	// Dirty reports whether the working tree had uncommitted changes at observation time. Always
	// false for bare repositories.
	Dirty bool

	// Remotes maps remote name (e.g., `origin`) to the fetch / push URL pair recorded in
	// `.git/config` at observation time.
	Remotes map[string]Remote
}

// NewObservation constructs a *Observation anchored to the resource it observes.
//
// Parameters:
//   - `ofResource`: the [Resource] this observation is of. Must be non-nil (asserted by [op.NewObservationBase]).
//   - `exists`: true when the path was a git repository at observation time.
//   - `observedHEAD`: the disk's current HEAD SHA.
//   - `observedRef`: the disk's current ref name.
//   - `bare`: true when the on-disk repository is bare.
//   - `dirty`: true when the working tree had uncommitted changes.
//   - `remotes`: the on-disk remote configuration at observation time.
//
// Returns:
//   - `*Observation`: the constructed observation.
func NewObservation(
	ofResource Resource,
	exists bool,
	observedHEAD string,
	observedRef string,
	bare bool,
	dirty bool,
	remotes map[string]Remote,
) *Observation {

	return &Observation{
		ObservationBase: op.NewObservationBase(ofResource, exists),
		ObservedHEAD:    observedHEAD,
		ObservedRef:     observedRef,
		Bare:            bare,
		Dirty:           dirty,
		Remotes:         remotes,
	}
}

// region EXPORTED METHODS

// region Behaviors

// String returns a debug-oriented single-line representation of the observation.
//
// Returns:
//   - `string`: `git.Observation{of=<OfResource.URI()>, exists=<bool>, head=<sha>, ref=<name>,
//     bare=<bool>, dirty=<bool>, remotes=<count>}`.
func (o *Observation) String() string {
	return fmt.Sprintf("git.Observation{of=%s, exists=%t, head=%s, ref=%s, bare=%t, dirty=%t, remotes=%d}",
		o.OfResource.URI(), o.Exists, o.ObservedHEAD, o.ObservedRef, o.Bare, o.Dirty, len(o.Remotes))
}

// endregion

// endregion
