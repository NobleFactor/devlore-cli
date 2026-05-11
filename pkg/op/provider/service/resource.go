// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// NewResource constructs a service.Resource and claims production via [op.ResourceCatalog.GetOrCreate].
//
// Use NewResource from a producer dispatch context — typically a provider method that has received an
// [op.ActivationRecord] from the framework. The returned Resource is the canonical catalog entry, stamped
// with `producerID = activationRecord.SiteID`. Use [DiscoverResource] instead when the caller is not
// claiming production (rehydration, reference handles, the framework's slot-coercion adapter).
//
// Today no service provider method actually claims production — Start, Stop, Enable, Disable, Restart all
// take an existing *Resource and mutate the on-host service state without changing the URI. NewResource
// exists for symmetry with the m.4 two-constructor pattern and as a stable surface for any future service
// producer that creates a new svc URI.
//
// Nil-Catalog tolerance: returns the unlinked candidate when no catalog is present.
//
// Parameters:
//   - activationRecord: the per-dispatch activation; its `Runtime` carries the runtime environment and its
//     `SiteID` becomes the catalog entry's producerID. Must be non-nil.
//   - value: a string service name.
//
// Returns:
//   - *Resource: the canonical catalog entry (or the unlinked candidate when no catalog is present).
//   - error: if value is not a string.
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
		return nil, fmt.Errorf("service.NewResource: catalog entry for %q is %T, want *service.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// DiscoverResource constructs a service.Resource and registers it with [op.ResourceCatalog.Discover] without
// claiming production. Used by the framework's resource registry adapter for slot coercion (when starlark
// supplies a string service name and the slot expects a *service.Resource), and by callers holding a
// reference handle without claiming production (receipt rehydration is the canonical example).
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
		return nil, fmt.Errorf("service.DiscoverResource: catalog entry for %q is %T, want *service.Resource", candidate.URI(), got)
	}

	return canonical, nil
}

// buildCandidate validates value and constructs a *Resource without touching the catalog. Shared by
// [NewResource] and [DiscoverResource].
func buildCandidate(runtimeEnvironment *op.RuntimeEnvironment, value any) (*Resource, error) {

	name, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("service.Resource: expected string name, got %T", value)
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

// Resource represents a system service.
type Resource struct {
	op.ResourceBase
	Name string
}

// String returns a compact JSON representation of the resource.
func (r *Resource) String() string { return r.Format(r) }

