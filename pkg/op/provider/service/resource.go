// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"crypto/sha256"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Interface Guard: *Resource implements op.Resource (via op.ResourceBase + own overrides).
var _ op.Resource = (*Resource)(nil)

// Interface Guard: *Resource implements json.Unmarshaler.
var _ json.Unmarshaler = (*Resource)(nil)

// Interface Guard: *Resource implements encoding.TextUnmarshaler.
var _ encoding.TextUnmarshaler = (*Resource)(nil)

// Interface Guard: *Resource implements fmt.Stringer.
var _ fmt.Stringer = (*Resource)(nil)

// Resource represents a system service identified by name.
//
// Location-keyed: the canonical URI is `tag:devlore.noblefactor.com,2026-01-01:svc:<Name>#...service.Resource`.
// Service state (running, enabled, mode, last-changed) is host-side and not part of identity — two
// service.Resources with the same Name on different hosts share a URI and a catalog entry.
type Resource struct {
	op.ResourceBase

	// Name is the service name (e.g., "nginx", "sshd"). Identity-bearing — appears in the URI <specific> as
	// `svc:<Name>`. Derivable from URI.
	Name string
}

// NewResource constructs a service.Resource and claims production via [op.ResourceCatalog.GetOrCreate].
//
// Use NewResource from a producer dispatch context — typically a provider method that has received an
// [op.ActivationRecord] from the framework. The returned Resource is the canonical catalog entry, stamped with
// `producerID = activationRecord.Unit.ID()` (or empty when `Unit` is nil for non-graph dispatch). Use
// [DiscoverResource] instead when the caller is not claiming production (rehydration, reference handles, the
// framework's slot-coercion adapter).
//
// Today no service provider method actually claims production — Start, Stop, Enable, Disable, Restart all take
// an existing *Resource and mutate the on-host service state without changing the URI. NewResource exists for
// symmetry with the two-constructor pattern and as a stable surface for any future service producer.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `unit`: the producing [op.ExecutableUnit] whose ID becomes the catalog entry's producerID, or nil
//     for non-graph dispatch.
//   - `value`: a bare service name string, or a canonical tag URI (`tag:..:svc:<name>#...`).
//
// Returns:
//   - *Resource: canonical catalog entry, or the unlinked candidate when no catalog is present.
//   - `error`: non-string input, malformed URI, or [op.ResourceBase] construction failure.
func NewResource(runtimeEnvironment *op.RuntimeEnvironment, unit op.ExecutableUnit, value any) (*Resource, error) {

	candidate, err := buildCandidate(runtimeEnvironment, value)
	if err != nil {
		return nil, err
	}

	if runtimeEnvironment.ResourceCatalog == nil {
		return candidate, nil
	}

	got, err := runtimeEnvironment.ResourceCatalog.GetOrCreate(
		unit,
		candidate.URI(),
		func() (op.Resource, error) { return candidate, nil },
	)
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf(
			"service.NewResource: catalog entry for %q is %T, want *service.Resource",
			candidate.URI(), got,
		)
	}

	return canonical, nil
}

// DiscoverResource registers a service.Resource via [op.ResourceCatalog.Discover] without claiming production.
//
// Used by the framework's resource registry adapter for slot coercion (when starlark supplies a string and
// the slot expects a *service.Resource) and by callers holding a reference handle without claiming
// production. UnmarshalJSON / UnmarshalText / UnmarshalYAML rehydration is the canonical use case.
//
// Discover does not stamp a producer, so unlike [NewResource] it takes only `runtimeEnvironment` — no
// unit reference is needed.
//
// Same value-shape dispatch as [NewResource]: bare service name or canonical tag URI.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - `runtimeEnvironment`: the session runtime environment.
//   - `value`: a bare service name string, or a canonical tag URI; same dispatch as [NewResource].
//
// Returns:
//   - *Resource: canonical catalog entry, or the unlinked candidate when no catalog is present.
//   - `error`: non-string input, malformed URI, or [op.ResourceBase] construction failure.
func DiscoverResource(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error) {

	candidate, err := buildCandidate(runtimeEnvironment, value)
	if err != nil {
		return nil, err
	}

	if runtimeEnvironment.ResourceCatalog == nil {
		return candidate, nil
	}

	got, err := runtimeEnvironment.ResourceCatalog.Discover(candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf(
			"service.DiscoverResource: catalog entry for %q is %T, want *service.Resource",
			candidate.URI(), got,
		)
	}

	return canonical, nil
}

// buildCandidate constructs a *Resource from `value` without touching the catalog.
//
// Shared by [NewResource] and [DiscoverResource]. Strings beginning with `tag:` are treated as canonical
// tag URIs and the service name is extracted from the URI's <specific>; all other strings are taken as
// bare service names.
//
// Parameters:
//   - `runtimeEnvironment`: runtime environment threaded into the produced [op.ResourceBase].
//   - `value`: a string service name or canonical tag URI; any other type is an error.
//
// Returns:
//   - *Resource: unlinked candidate.
//   - `error`: non-string input, malformed URI, URI <specific> not in `svc:<name>` form, or [op.ResourceBase]
//     construction failure.
func buildCandidate(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error) {

	raw, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("service.Resource: expected string service name or URI, got %T", value)
	}

	name := raw
	if strings.HasPrefix(raw, "tag:") {
		specific, _, err := op.ExtractTagSpecific(raw)
		if err != nil {
			return nil, fmt.Errorf("service.Resource: %w", err)
		}
		extracted, ok := strings.CutPrefix(specific, "svc:")
		if !ok {
			return nil, fmt.Errorf("service.Resource: URI specific %q is not in svc:<name> form", specific)
		}
		name = extracted
	}

	base, err := op.NewResourceBase(runtimeEnvironment, "svc:"+name, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	return &Resource{
		ResourceBase: base,
		Name:         name,
	}, nil
}

// region EXPORTED METHODS

// region State management

// Addressing reports that service.Resource is location-keyed by service name.
//
// Overrides [op.ResourceBase.Addressing]'s [op.AddressingUnknown] default. The boot-discipline check in
// pkg/op/addressing_test.go relies on every announced Resource type returning a non-Unknown mode here.
//
// Returns:
//   - op.AddressingMode: [op.AddressingLocation] — identity is the service name embedded in the URI.
func (r *Resource) Addressing() op.AddressingMode {
	return op.AddressingLocation
}

// Digest returns the SHA-256 of the canonical URI.
//
// Service state (running/enabled/mode) is host-side and not part of identity, so the digest derives from the
// identity itself (the URI) rather than runtime state. Hashing the URI keeps the digest algorithm consistent
// with the rest of the system ([op.ParseDigest] only accepts sha256) and gives service.Resource a stable token
// for the catalog's etag-mismatch path. Overrides [op.ResourceBase.Digest]'s [op.ErrUnimplemented] default.
//
// Returns:
//   - op.Digest: sha256 of the URI; Algorithm = "sha256", Bytes = 32 raw digest bytes.
//   - `error`: nil under normal conditions.
func (r *Resource) Digest() (op.Digest, error) {
	h := sha256.Sum256([]byte(r.URI()))
	return op.Digest{Algorithm: "sha256", Bytes: h[:]}, nil
}

// Equal reports whether r and other identify the same service.Resource.
//
// Strict equality: other must be a *service.Resource (not merely an [op.Resource] with the same URI). Once the
// type check passes, URI comparison is delegated to [op.ResourceBase.Equal].
//
// Parameters:
//   - `other`: candidate value to compare against; nil or any non-*service.Resource value returns false.
//
// Returns:
//   - `bool`: true when `other` is a *service.Resource with the same URI as r.
func (r *Resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// Etag returns the canonical URI as the change-detection token.
//
// For a location-keyed Resource whose state is host-side and outside its identity, the URI doubles as the etag:
// two service.Resources with the same Name share a URI and are the same Resource. Any host-side state change
// (the service stops, restarts, changes mode) is detected by callers via explicit probes, not through Etag.
//
// Returns:
//   - `string`: the canonical URI (identical to [op.ResourceBase.URI]).
//   - `error`: nil under normal conditions.
func (r *Resource) Etag() (string, error) {
	return r.URI(), nil
}

// String returns the compact JSON encoding of the Resource for debug output.
//
// Delegates to [op.ResourceBase.Format] per the project Go style guideline that String() of every concrete
// Resource type calls r.Format(r).
//
// Returns:
//   - `string`: the compact JSON encoding of r.
func (r *Resource) String() string {
	return r.Format(r)
}

// CanConvertFrom reports whether `source` can be projected into a [*Resource] via [Resource.ConvertFrom].
//
// Opts the service Resource into the framework's [op.TargetConverter] contract — accepted source shape is
// `string` (interpreted as a bare service name like "nginx", or a canonical tag URI like
// `tag:..:svc:<name>#...`). The framework consults this probe both at plan-time via
// [op.typesAreInterconvertible] (the bubble-up parameter-consistency check) and at dispatch-time via
// [op.Convert] step 7 (env-less fallback). The canonical dispatch-time path remains the registered
// constructor at [op.Convert] step 6, which receives the full [op.RuntimeEnvironment] and dispatches the
// bare-name vs URI input shape via [buildCandidate].
//
// Cheap-probe contract: this method is called against a nil-or-zero `*Resource` receiver by
// [op.typesAreInterconvertible] during plan-time bubble-up checks. MUST NOT dereference receiver fields.
//
// Parameters:
//   - `source`: the candidate source type to test.
//
// Returns:
//   - `bool`: true when `source` is `string`.
func (*Resource) CanConvertFrom(source reflect.Type) bool {

	return source != nil && source.Kind() == reflect.String
}

// ConvertFrom projects `value` into an env-less unlinked [*Resource].
//
// Used by [op.Convert] step 7 when the env-aware registered constructor (step 6) is unavailable — env-less
// library callers, tests, or [op.RuntimeEnvironment.Registry]-missing contexts. The returned Resource carries
// only the Name set from `value`; the canonical URI on the embedded [op.ResourceBase] is NOT populated here.
// Provider methods consuming the projected Resource are responsible for re-canonicalization via their own
// [NewResource]/[DiscoverResource] path when full identity is required.
//
// Parameters:
//   - `value`: the source value; must be `string`.
//
// Returns:
//   - `any`: the constructed unlinked [*Resource].
//   - `error`: non-nil when `value` is not a `string`.
func (*Resource) ConvertFrom(value any) (any, error) {

	str, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("service.Resource.ConvertFrom: source must be string, got %T", value)
	}

	return &Resource{Name: str}, nil
}

// endregion

// region Behaviors

// UnmarshalJSON populates the receiver from its JSON document (a bare URI string).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method. The URI alone is sufficient — identity is the service name encoded in the URI's
// <specific> as `svc:<Name>`.
//
// Parameters:
//   - `data`: JSON bytes encoding a single bare URI string.
//
// Returns:
//   - `error`: missing RuntimeEnvironment on receiver, malformed JSON, or rehydration failure.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("service.Resource: UnmarshalJSON requires RuntimeEnvironment on receiver")
	}

	var uri string
	if err := json.Unmarshal(data, &uri); err != nil {
		return err
	}

	built, err := DiscoverResource(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing the URI.
//
// Same prerequisites and semantics as [Resource.UnmarshalJSON]; the receiver's [op.RuntimeEnvironment] must be
// set before invocation.
//
// Parameters:
//   - `text`: UTF-8 bytes containing the canonical tag URI.
//
// Returns:
//   - `error`: missing RuntimeEnvironment on receiver, or rehydration failure.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("service.Resource: UnmarshalText requires RuntimeEnvironment on receiver")
	}

	built, err := DiscoverResource(r.RuntimeEnvironment(), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from its YAML document (a bare URI scalar).
//
// Same prerequisites and semantics as [Resource.UnmarshalJSON]; the receiver's [op.RuntimeEnvironment] must be
// set before invocation.
//
// Parameters:
//   - `unmarshal`: yaml decode hook supplied by the YAML library; called with a *string target.
//
// Returns:
//   - `error`: missing RuntimeEnvironment on receiver, decode failure, or rehydration failure.
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("service.Resource: UnmarshalYAML requires RuntimeEnvironment on receiver")
	}

	var uri string
	if err := unmarshal(&uri); err != nil {
		return err
	}

	built, err := DiscoverResource(r.RuntimeEnvironment(), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// endregion

// endregion
