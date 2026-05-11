// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
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
// with `producerID = activationRecord.SiteID`. Use [DiscoverResource] instead when the caller is not
// claiming production (rehydration, reference handles, the framework's slot-coercion adapter).
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
//   - activationRecord: the per-dispatch activation; its `Runtime` carries the runtime environment (must
//     have Platform set) and its `SiteID` becomes the catalog entry's producerID. Must be non-nil.
//   - value: a string package name with an optional manager prefix.
//
// Returns:
//   - *Resource: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - error: if value is not a string or the manager prefix is unknown.
func NewResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {

	candidate, err := buildCandidate(activationRecord.Runtime, value)
	if err != nil {
		return nil, err
	}

	if activationRecord.Runtime.Catalog == nil {
		return candidate, nil
	}

	got, err := activationRecord.Runtime.Catalog.GetOrCreate(activationRecord, candidate.URI(), func() (op.Resource, error) {
		return candidate, nil
	})
	if err != nil {
		return nil, err
	}

	canonical, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("pkg.NewResource: catalog entry for %q is %T, want *pkg.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// DiscoverResource constructs a pkg.Resource and registers it with [op.ResourceCatalog.Discover] without
// claiming production. Used by the framework's resource registry adapter for slot coercion (when starlark
// supplies a string package name and the slot expects a *pkg.Resource), and by callers holding a reference
// handle without claiming production (receipt rehydration is the canonical example).
//
// activationRecord is required for signature symmetry with [NewResource], but only activationRecord.Runtime
// is consumed. SiteID is unused (Discover doesn't stamp). Discovery callers commonly synthesize an
// [op.ActivationRecord] with empty SiteID and only Runtime set: `&op.ActivationRecord{Runtime: ctx}`.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
func DiscoverResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {

	candidate, err := buildCandidate(activationRecord.Runtime, value)
	if err != nil {
		return nil, err
	}

	if activationRecord.Runtime.Catalog == nil {
		return candidate, nil
	}

	got, err := activationRecord.Runtime.Catalog.Discover(candidate.URI(), func() (op.Resource, error) {
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

// buildCandidate validates value, parses any manager prefix, and constructs a *Resource without touching
// the catalog. Shared by [NewResource] and [DiscoverResource].
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
		Version:      purl.Version,
	}, nil
}

// Resource represents a system package.
type Resource struct {
	op.ResourceBase
	Name    string // package name ("jq", "curl", "VisualStudioCode")
	Type    string // purl type / manager ("brew", "deb", "port", "winget")
	Version string // populated by Resolve()
}

// String returns a compact JSON representation of the resource.
func (r *Resource) String() string { return r.Format(r) }

// Resolve populates Version from the installed package version via the platform's package manager.
//
// Type and Name are established at construction time. Version is the only field that requires runtime resolution. If the
// platform or manager is unavailable, Version is left empty — no error.
//
// Parameters:
//   - root: unused (package version queries do not use the confined root).
//
// Returns:
//   - error: always nil.
func (r *Resource) Resolve() error {

	ctx := r.RuntimeEnvironment()

	if ctx == nil || ctx.Platform == nil {
		return nil
	}

	mgr := ctx.Platform.PackageManagerByName(r.Type)

	if mgr == nil {
		return nil
	}

	r.Version = mgr.Version(r.Name)
	return nil
}

