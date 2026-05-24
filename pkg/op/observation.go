// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// Observation is the framework marker interface for observation-shaped [Resource] values.
//
// Every concrete observation type embeds [ObservationBase], which satisfies Observation by
// providing the unexported `observation()` accessor that the catalog uses to read the back-link
// without crossing into per-type concrete fields. The unexported accessor means external packages
// cannot synthesize their own Observation types — the framework-provided [ObservationBase] is the
// only path to satisfying the interface, so all observations carry the same identity-plus-back-link
// surface.
type Observation interface {
	Resource

	// observation is the framework-internal accessor that lets the catalog read an observation's
	// back-link without depending on per-type concrete fields.
	observation() *ObservationBase
}

// ObservationBase is the identity-plus-back-link surface shared by every concrete observation type.
//
// Embeds [ResourceBase] for identity (URI, ID, ProducerID, etc.). Adds the typed back-link to the
// observed [Resource] (`OfResource`) plus an existence flag (`Exists`). Concrete observation types
// embed ObservationBase and contribute their own per-provider measurement fields.
//
// ObservationBase provides default implementations of the observation-shaped [Resource] interface
// methods — [Addressing], [Etag], [Resolve], and [Digest] — that hold for every observation by the
// nature of the kind: observations are content-addressable, terminal, and have a URI-as-etag.
// Concrete observation types inherit these and contribute only their measurement-field constructor
// + an optional String() debug helper.
type ObservationBase struct {
	ResourceBase

	// OfResource is the [Resource] this observation is of. Set at construction; non-nil invariant
	// asserted by [NewObservationBase]. Access the back-link's URI as `o.OfResource.URI()`.
	OfResource Resource

	// Exists is true when the observed thing was present at observation time. When false, the
	// concrete observation's measurement fields carry zero values.
	Exists bool
}

// NewObservationBase constructs an ObservationBase with a content-addressable URI.
//
// Callers (per-provider observation constructors) compute the canonical sha256 hash over their
// observation fields — `OfResource.URI()`, `Exists`, and the concrete measurement fields — and pass
// `sha256:<lowercase-hex>` as `specific`. The framework constructs the URI via [NewResourceBase],
// asserts the non-nil [Resource] invariant on `ofResource`, and returns the populated base.
//
// Parameters:
//   - `runtimeEnvironment`: the execution context; embedded via [NewResourceBase].
//   - `specific`: the `sha256:<lowercase-hex>` `<specific>` portion. Must encode the canonical
//     content hash over every observation field; identical inputs produce identical URIs.
//   - `goType`: the concrete observation Go type; passed through to [NewResourceBase] to compose
//     the typeID fragment.
//   - `ofResource`: the [Resource] this observation is of. Must be non-nil (asserted).
//   - `exists`: true when the observed thing was present at observation time.
//
// Returns:
//   - ObservationBase: the constructed base.
//   - `error`: any [NewResourceBase] failure.
func NewObservationBase(
	runtimeEnvironment *RuntimeEnvironment,
	specific string,
	goType reflect.Type,
	ofResource Resource,
	exists bool,
) (ObservationBase, error) {

	assert.NonZero("ofResource", ofResource)

	base, err := NewResourceBase(runtimeEnvironment, specific, goType)
	if err != nil {
		return ObservationBase{}, err
	}

	return ObservationBase{
		ResourceBase: base,
		OfResource:   ofResource,
		Exists:       exists,
	}, nil
}

// region EXPORTED METHODS

// region Behaviors

// Addressing reports [AddressingContent] for every observation.
//
// Observations are content-addressable by construction — the URI encodes the canonical hash of
// every observation field, so observations with byte-identical contents share an identity. There's
// nothing else for two observations to be: identical-content observations describe the same fact.
//
// Returns:
//   - AddressingMode: always [AddressingContent].
func (o *ObservationBase) Addressing() AddressingMode {
	return AddressingContent
}

// Digest returns the observation's content hash as an [Digest].
//
// For content-addressable Resources the URI IS the digest — the `<specific>` portion of the URI is
// `sha256:<lowercase-hex>`. Digest extracts the hex, decodes it, and wraps the bytes in an
// [Digest]. No per-provider override needed.
//
// Returns:
//   - Digest: sha256 algorithm with 32 raw bytes.
//   - `error`: non-nil only if the URI's `<specific>` is malformed (does not start with
//     `sha256:` or contains non-hex characters).
func (o *ObservationBase) Digest() (Digest, error) {

	specific := o.ReachabilityURI()

	const prefix = "sha256:"
	if !strings.HasPrefix(specific, prefix) {
		return Digest{}, fmt.Errorf("op.ObservationBase: digest: specific %q does not start with %q", specific, prefix)
	}

	raw, err := hex.DecodeString(specific[len(prefix):])
	if err != nil {
		return Digest{}, fmt.Errorf("op.ObservationBase: digest decode: %w", err)
	}

	return Digest{Algorithm: "sha256", Bytes: raw}, nil
}

// Etag returns the URI as the change-detection token.
//
// For content-addressable Resources the URI itself is the etag — two observations with the same URI
// have, by construction, identical contents.
//
// Returns:
//   - string: the canonical URI.
//   - `error`: nil.
func (o *ObservationBase) Etag() (string, error) {
	return o.URI(), nil
}

// Resolve is a no-op for observations.
//
// Observations are terminal — they record what was seen, they do not themselves observe anything
// downstream.
//
// Returns:
//   - `error`: always nil.
func (o *ObservationBase) Resolve() error {
	return nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region State management

// observation returns the embedded ObservationBase so the catalog can read the back-link without
// crossing into per-type concrete fields. Satisfies the [Observation] marker interface.
//
// Returns:
//   - *ObservationBase: the receiver.
func (o *ObservationBase) observation() *ObservationBase {
	return o
}

// endregion

// endregion
