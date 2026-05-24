// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Observation captures the runtime-observed state of a [*Resource]'s on-disk clone at the moment it
// was observed.
//
// Distinct from [Resource], which carries identity (URI, [op.Path], and the identity-extension
// intent fields `HEAD` and `Ref` from the plan). Observation embeds [op.ObservationBase] (which
// itself embeds [op.ResourceBase] and adds the typed back-link [op.ObservationBase.OfResource] +
// [op.ObservationBase.Exists]) and adds the git-specific observation fields: `ObservedHEAD`,
// `ObservedRef`, `Bare`, `Dirty`, `Remotes`. Each observation is content-addressable — the URI is
// sha256 over the canonical encoding of its fields.
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

// NewObservation constructs a *Observation with a content-addressable URI derived from its fields.
//
// The URI takes the form `tag:devlore.noblefactor.com,2026-01-01:sha256:<hex>#git.Observation`
// where `<hex>` is lowercase hex of sha256 over the canonical encoding of `(OfResource.URI(),
// Exists, ObservedHEAD, ObservedRef, Bare, Dirty, Remotes-sorted-by-name)`. Two observations with
// identical contents share a URI; the catalog deduplicates them naturally.
//
// Parameters:
//   - `runtimeEnvironment`: the execution context; embedded via [op.NewObservationBase].
//   - `ofResource`: the [*Resource] this observation is of. Must be non-nil (asserted by
//     [op.NewObservationBase]).
//   - `exists`: true when the path was a git repository at observation time.
//   - `observedHEAD`: the disk's current HEAD SHA.
//   - `observedRef`: the disk's current ref name.
//   - `bare`: true when the on-disk repository is bare.
//   - `dirty`: true when the working tree had uncommitted changes.
//   - `remotes`: the on-disk remote configuration at observation time.
//
// Returns:
//   - *Observation: the constructed observation.
//   - `error`: any [op.NewObservationBase] failure.
func NewObservation(
	runtimeEnvironment *op.RuntimeEnvironment,
	ofResource *Resource,
	exists bool,
	observedHEAD string,
	observedRef string,
	bare bool,
	dirty bool,
	remotes map[string]Remote,
) (*Observation, error) {

	specific := observationSpecific(ofResource.URI(), exists, observedHEAD, observedRef, bare, dirty, remotes)

	base, err := op.NewObservationBase(
		runtimeEnvironment,
		specific,
		reflect.TypeFor[*Observation](),
		ofResource,
		exists,
	)
	if err != nil {
		return nil, fmt.Errorf("git.NewObservation: %w", err)
	}

	return &Observation{
		ObservationBase: base,
		ObservedHEAD:    observedHEAD,
		ObservedRef:     observedRef,
		Bare:            bare,
		Dirty:           dirty,
		Remotes:         remotes,
	}, nil
}

// region EXPORTED METHODS

// region Behaviors

// String returns a debug-oriented single-line representation of the observation.
//
// Returns:
//   - string: `git.Observation{of=<OfResource.URI()>, exists=<bool>, head=<sha>, ref=<name>,
//     bare=<bool>, dirty=<bool>, remotes=<count>}`.
func (o *Observation) String() string {
	return fmt.Sprintf("git.Observation{of=%s, exists=%t, head=%s, ref=%s, bare=%t, dirty=%t, remotes=%d}",
		o.OfResource.URI(), o.Exists, o.ObservedHEAD, o.ObservedRef, o.Bare, o.Dirty, len(o.Remotes))
}

// endregion

// endregion

// region UNEXPORTED FUNCTIONS

// observationSpecific computes the `<specific>` portion of an Observation's URI as
// `sha256:<lowercase-hex-of-canonical-encoding>`.
//
// Canonical encoding hashes `ofURI`, an `exists` byte, `ObservedHEAD`, `ObservedRef`, a `bare`
// byte, a `dirty` byte, and `Remotes` serialized as sorted-key `name=fetch|push;` tuples — so two
// observations with identical contents hash identically across runs. The typeID fragment
// (`#git.Observation`) carries the type discriminator.
//
// Parameters:
//   - `ofURI`: the URI of the [Resource] this observation is of.
//   - `exists`: true when the path was a git repository at observation time.
//   - `observedHEAD`: the disk's current HEAD SHA.
//   - `observedRef`: the disk's current ref name.
//   - `bare`: true when the on-disk repository is bare.
//   - `dirty`: true when the working tree had uncommitted changes.
//   - `remotes`: the on-disk remote configuration at observation time.
//
// Returns:
//   - string: the `sha256:<hex>` specific.
func observationSpecific(
	ofURI string,
	exists bool,
	observedHEAD string,
	observedRef string,
	bare bool,
	dirty bool,
	remotes map[string]Remote,
) string {

	h := sha256.New()
	h.Write([]byte(ofURI))
	h.Write([]byte{0})
	if exists {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
	h.Write([]byte(observedHEAD))
	h.Write([]byte{0})
	h.Write([]byte(observedRef))
	h.Write([]byte{0})
	if bare {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
	if dirty {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}

	names := make([]string, 0, len(remotes))
	for name := range remotes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		remote := remotes[name]
		h.Write([]byte(name))
		h.Write([]byte{'='})
		h.Write([]byte(remote.FetchURL))
		h.Write([]byte{'|'})
		h.Write([]byte(remote.PushURL))
		h.Write([]byte{';'})
	}

	return "sha256:" + strings.ToLower(hex.EncodeToString(h.Sum(nil)))
}

// endregion
