// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Observation captures the runtime-observed state of a [*Resource] (a host package) at the moment
// it was observed.
//
// Distinct from [Resource], which carries identity (the purl URI, `Name`, `Type`). Observation
// embeds [op.ObservationBase] (which itself embeds [op.ResourceBase] and adds the typed back-link
// [op.ObservationBase.OfResource] + [op.ObservationBase.Exists]) and adds the package-specific
// observation field: `Version`. Each observation is content-addressable — the URI is sha256 over
// the canonical encoding of `(OfResource.URI(), Exists, Version)`.
type Observation struct {
	op.ObservationBase

	// Version is the version string the platform's package manager reports for the package at
	// observation time. Empty when `Exists` is false.
	Version string
}

// NewObservation constructs a *Observation with a content-addressable URI derived from its fields.
//
// The URI takes the form `tag:devlore.noblefactor.com,2026-01-01:sha256:<hex>#pkg.Observation`
// where `<hex>` is lowercase hex of sha256 over `(OfResource.URI(), Exists, Version)`. Two
// observations with identical contents share a URI; the catalog deduplicates them naturally.
//
// Parameters:
//   - `runtimeEnvironment`: the execution context; embedded via [op.NewObservationBase].
//   - `ofResource`: the [*Resource] this observation is of. Must be non-nil (asserted by
//     [op.NewObservationBase]).
//   - `exists`: true when the package was installed at observation time.
//   - `version`: the installed version reported by the package manager.
//
// Returns:
//   - *Observation: the constructed observation.
//   - `error`: any [op.NewObservationBase] failure.
func NewObservation(
	runtimeEnvironment *op.RuntimeEnvironment,
	ofResource *Resource,
	exists bool,
	version string,
) (*Observation, error) {

	specific := observationSpecific(ofResource.URI(), exists, version)

	base, err := op.NewObservationBase(
		runtimeEnvironment,
		specific,
		reflect.TypeFor[*Observation](),
		ofResource,
		exists,
	)
	if err != nil {
		return nil, fmt.Errorf("pkg.NewObservation: %w", err)
	}

	return &Observation{
		ObservationBase: base,
		Version:         version,
	}, nil
}

// region EXPORTED METHODS

// region Behaviors

// String returns a debug-oriented single-line representation of the observation.
//
// Returns:
//   - string: `pkg.Observation{of=<OfResource.URI()>, exists=<bool>, version=<string>}`.
func (o *Observation) String() string {
	return fmt.Sprintf("pkg.Observation{of=%s, exists=%t, version=%s}",
		o.OfResource.URI(), o.Exists, o.Version)
}

// endregion

// endregion

// region UNEXPORTED FUNCTIONS

// observationSpecific computes the `<specific>` portion of an Observation's URI as
// `sha256:<lowercase-hex-of-canonical-encoding>`.
//
// Canonical encoding hashes `ofURI`, an `exists` byte, and `Version` — so two observations with
// identical contents hash identically across runs. The typeID fragment (`#pkg.Observation`)
// carries the type discriminator.
//
// Parameters:
//   - `ofURI`: the URI of the [Resource] this observation is of.
//   - `exists`: true when the package was installed at observation time.
//   - `version`: the installed version reported by the package manager.
//
// Returns:
//   - string: the `sha256:<hex>` specific.
func observationSpecific(ofURI string, exists bool, version string) string {

	h := sha256.New()
	h.Write([]byte(ofURI))
	h.Write([]byte{0})
	if exists {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
	h.Write([]byte(version))

	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// endregion
