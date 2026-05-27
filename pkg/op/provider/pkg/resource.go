// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// NewResource constructs a pkg.Resource and claims production via [op.ResourceCatalog.GetOrCreate].
//
// Use NewResource from a producer dispatch context — typically a provider method that has received an
// [op.ActivationRecord] from the framework. The returned Resource is the canonical catalog entry, stamped
// with `producerID = activationRecord.Unit.ID()` (or empty when `Unit` is nil for non-graph dispatch). Use
// [DiscoverResource] instead when the caller is not claiming production (rehydration, reference handles,
// the framework's slot-coercion adapter).
//
// Today no pkg provider method actually claims production — Install / Remove / Upgrade all take an existing
// `[]*Resource` and return the same pointers with their `Type` field updated to reflect which platform
// manager handled them. URIs (purls) are unchanged. NewResource exists for symmetry with the m.4
// two-constructor pattern and as a stable surface for any future pkg producer that creates a new purl.
//
// The value is a string package name with an optional manager prefix (e.g., "jq", "brew:jq", "port:wget",
// "Microsoft.VisualStudioCode@1.89"). When no prefix is present, the platform's default package manager is
// used. The manager's ParsePURL method formulates the purl identity from the package name.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - `activationRecord`: the per-dispatch activation; its `RuntimeEnvironment` carries the runtime
//     environment (must have `Platform` set) and its `Unit.ID()` becomes the catalog entry's producerID
//     (empty when `Unit` is nil). Must be non-nil.
//   - `value`: a string package name with an optional manager prefix.
//
// Returns:
//   - *Resource: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string or the manager prefix is unknown.
func NewResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {

	candidate, err := buildCandidate(activationRecord.RuntimeEnvironment, value)
	if err != nil {
		return nil, err
	}

	if activationRecord.RuntimeEnvironment.Catalog == nil {
		return candidate, nil
	}

	got, err := activationRecord.RuntimeEnvironment.Catalog.GetOrCreate(
		activationRecord,
		candidate.URI(),
		func() (op.Resource, error) { return candidate, nil },
	)
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("pkg.NewResource: catalog entry for %q is %T, want *pkg.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// DiscoverResource registers a pkg.Resource via [op.ResourceCatalog.Discover] without claiming production.
//
// Used by the framework's resource registry adapter for slot coercion (when starlark supplies a string
// package name and the slot expects a *pkg.Resource), and by callers holding a reference handle without
// claiming production (receipt rehydration is the canonical example).
//
// `activationRecord` is required for signature symmetry with [NewResource], but only its `RuntimeEnvironment`
// is consumed — `Unit` is unused since Discover doesn't stamp a producer. Discovery callers commonly construct
// one as `op.NewActivationRecord(nil, nil, ctx)` — both `Graph` and `Unit` nil.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - `activationRecord`: provides the runtime environment via `activationRecord.RuntimeEnvironment`. `Unit` is
//     unused. Must be non-nil.
//   - `value`: a string package name with an optional manager prefix.
//
// Returns:
//   - *Resource: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - `error`: if `value` is not a string or the manager prefix is unknown.
func DiscoverResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {

	candidate, err := buildCandidate(activationRecord.RuntimeEnvironment, value)
	if err != nil {
		return nil, err
	}

	if activationRecord.RuntimeEnvironment.Catalog == nil {
		return candidate, nil
	}

	got, err := activationRecord.RuntimeEnvironment.Catalog.Discover(candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("pkg.DiscoverResource: catalog entry for %q is %T, want *pkg.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// buildCandidate constructs a *Resource from `value` without touching the catalog.
//
// Validates that `value` is a string, parses any `manager:` prefix, and resolves the package URL. Shared
// by [NewResource] and [DiscoverResource].
//
// Parameters:
//   - `runtimeEnvironment`: the runtime environment; must have `Platform` set.
//   - `value`: a string package name with an optional `manager:` prefix.
//
// Returns:
//   - *Resource: the constructed candidate, not yet interned in the catalog.
//   - `error`: if `value` is not a string or the manager prefix is unknown.
func buildCandidate(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error) {

	raw, ok := value.(string)

	if !ok {
		return nil, fmt.Errorf("pkg.Resource: expected string, got %T", value)
	}

	// Parse optional manager prefix (e.g., "brew:jq", "port:wget").

	var mgr platform.PackageManager

	if prefix, after, ok := strings.Cut(raw, ":"); ok {
		mgr = runtimeEnvironment.Platform.PackageManagerByName(prefix)
		if mgr == nil {
			return nil, fmt.Errorf("pkg.Resource: unknown package manager %q", prefix)
		}
		raw = after
	} else {
		mgr = runtimeEnvironment.Platform.DefaultPackageManager()
	}

	purl := mgr.ParsePURL(raw)

	base, err := op.NewResourceBase(runtimeEnvironment, purl.String(), reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	return &Resource{
		ResourceBase: base,
		Name:         purl.Name,
		Type:         purl.Type,
	}, nil
}

// Resource represents a system package.
// Resource identifies a host package by its package-URL (purl) coordinates.
//
// Identity-only: `Name` and `Type` together form the purl encoded in the [op.ResourceBase] URI.
// Runtime-observed state (the installed `Version` reported by the platform's package manager)
// lives on a separate [*Observation] minted by [Provider.Observe].
type Resource struct {
	op.ResourceBase
	Name string // package name ("jq", "curl", "VisualStudioCode")
	Type string // purl type / manager ("brew", "deb", "port", "winget")
}

// String returns a compact JSON representation of the resource.
//
// Returns:
//   - `string`: the compact JSON encoding of r.
func (r *Resource) String() string { return r.Format(r) }

// Addressing reports that pkg.Resource is location-keyed: identity is the package URI (the purl).
//
// The installed state (version, presence) under that purl is mutable, and the catalog uses
// [op.AddressingLocation] semantics — content drift triggers shadow chains, not new URIs.
//
// Returns:
//   - op.AddressingMode: always [op.AddressingLocation].
func (r *Resource) Addressing() op.AddressingMode {
	return op.AddressingLocation
}

// Etag returns the currently-installed version of the package as a cheap change-detection token.
//
// Empty string when the package is not installed (a valid state, distinguishable from errors by the nil
// error return). The catalog uses Etag as the cheap signal; mismatch triggers a full [Resource.Digest]
// comparison.
//
// Always fresh — queries the platform's package manager at call time. Does not consult [Resource.Version], which
// is a [Resolve]-populated snapshot rather than current state. Errors when the runtime environment has no
// Platform, or no package manager is registered for the Resource's Type.
//
// Returns:
//   - `string`: the installed version string, or "" when uninstalled.
//   - `error`: when Platform is missing or the manager for [Resource.Type] is unavailable.
func (r *Resource) Etag() (string, error) {

	ctx := r.RuntimeEnvironment()
	if ctx == nil || ctx.Platform == nil {
		return "", fmt.Errorf("pkg.Resource: etag: no Platform in runtime")
	}

	mgr := ctx.Platform.PackageManagerByName(r.Type)
	if mgr == nil {
		return "", fmt.Errorf("pkg.Resource: etag: no manager for type %q", r.Type)
	}

	return mgr.Version(r.Name), nil
}

// Digest returns the honest content hash: sha256 of (installed version + "\n" + canonical purl URI).
//
// The canonical purl encodes the package identity (type + name); the installed version encodes the
// mutable state. Hashing the pair gives a stable, content-addressable token that changes when either
// the identity (which would normally mean a different URI / different Resource) or the installed state
// changes.
//
// Uninstalled packages produce a deterministic digest of (empty version + URI), distinct from any installed
// digest for the same package, and distinct across different packages (since URIs differ).
//
// Always fresh — re-queries the version at call time via [Resource.Etag]. Errors when Etag would error.
//
// Returns:
//   - op.Digest: sha256 algorithm with 32 raw bytes.
//   - `error`: any error from [Resource.Etag] (no Platform, no manager for Type).
func (r *Resource) Digest() (op.Digest, error) {

	version, err := r.Etag()
	if err != nil {
		return op.Digest{}, err
	}

	h := sha256.New()
	h.Write([]byte(version))
	h.Write([]byte("\n"))
	h.Write([]byte(r.URI()))

	return op.Digest{Algorithm: "sha256", Bytes: h.Sum(nil)}, nil
}

// Equal reports whether r and other identify the same pkg resource.
//
// Strict equality: other must be a *pkg.Resource (not merely an [op.Resource] with the same URI). Once the
// type check passes, URI comparison is delegated to [op.ResourceBase.Equal]. A cross-type URI collision is
// treated as a caller-side construction error, not a case Equal needs to disambiguate.
//
// Parameters:
//   - `other`: the value to compare against; may be any, including nil or a non-Resource.
//
// Returns:
//   - `bool`: true if `other` is a *pkg.Resource with the same URI as r.
func (r *Resource) Equal(other any) bool {

	if other == nil {
		return false
	}

	if _, ok := other.(*Resource); !ok {
		return false
	}

	return r.ResourceBase.Equal(other)
}

// CanConvertFrom reports whether `source` can be projected into a [*Resource] via [Resource.ConvertFrom].
//
// Opts the pkg Resource into the framework's [op.TargetConverter] contract — accepted source shape is `string`
// (interpreted as a package identifier, either a bare name like "jq" or a purl-prefixed form like "brew:jq").
// The framework consults this probe both at plan-time via [op.typesAreInterconvertible] (the bubble-up
// parameter-consistency check) and at dispatch-time via [op.Convert] step 7 (env-less fallback). The
// canonical dispatch-time path remains the registered constructor at [op.Convert] step 6, which receives the
// full [op.RuntimeEnvironment] and parses any manager prefix via [buildCandidate].
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
// only the Name set from `value`; the manager prefix is NOT parsed here, and the canonical URI on the
// embedded [op.ResourceBase] is not populated. Provider methods consuming the projected Resource are
// responsible for re-canonicalization via their own [NewResource]/[DiscoverResource] path when full identity
// is required.
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
		return nil, fmt.Errorf("pkg.Resource.ConvertFrom: source must be string, got %T", value)
	}

	return &Resource{Name: str}, nil
}

// UnmarshalJSON populates the receiver from its JSON wire form (a bare purl string).
//
// The caller pre-seeds the receiver's embedded [op.ResourceBase] with a valid [op.RuntimeEnvironment] before
// invoking this method; the runtime environment provides the platform needed to parse the purl. Rehydration
// flows through [DiscoverResource] (non-production claim).
//
// Parameters:
//   - `data`: JSON-encoded purl string.
//
// Returns:
//   - `error`: non-nil if the RuntimeEnvironment is missing, the JSON does not decode as a string, or resource
//     construction fails.
func (r *Resource) UnmarshalJSON(data []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("pkg.Resource: UnmarshalJSON requires RuntimeEnvironment on receiver")
	}

	var uri string
	if err := json.Unmarshal(data, &uri); err != nil {
		return err
	}

	built, err := DiscoverResource(op.NewActivationRecord(nil, nil, r.RuntimeEnvironment()), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalText populates the receiver from raw UTF-8 bytes containing the purl string.
//
// Parameters:
//   - `text`: UTF-8 bytes containing the purl.
//
// Returns:
//   - `error`: missing RuntimeEnvironment on receiver, or rehydration failure.
func (r *Resource) UnmarshalText(text []byte) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("pkg.Resource: UnmarshalText requires RuntimeEnvironment on receiver")
	}

	built, err := DiscoverResource(op.NewActivationRecord(nil, nil, r.RuntimeEnvironment()), string(text))
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// UnmarshalYAML populates the receiver from its YAML wire form (a bare purl scalar).
//
// Parameters:
//   - `unmarshal`: yaml decode hook supplied by the YAML library; called with a *string target.
//
// Returns:
//   - `error`: missing RuntimeEnvironment on receiver, decode failure, or rehydration failure.
func (r *Resource) UnmarshalYAML(unmarshal func(any) error) error {

	if r.RuntimeEnvironment() == nil {
		return errors.New("pkg.Resource: UnmarshalYAML requires RuntimeEnvironment on receiver")
	}

	var uri string
	if err := unmarshal(&uri); err != nil {
		return err
	}

	built, err := DiscoverResource(op.NewActivationRecord(nil, nil, r.RuntimeEnvironment()), uri)
	if err != nil {
		return err
	}

	*r = *built
	return nil
}

// Resolve populates Version from the installed package version via the platform's package manager.
//
// Type and Name are established at construction time. Version is the only field that requires runtime
// resolution. If the platform or manager is unavailable, Version is left empty — no error.
//
// Returns:
//   - `error`: always nil.
func (r *Resource) Resolve() error {
	return nil
}
