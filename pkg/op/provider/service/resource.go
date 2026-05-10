// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// NewResource creates a service.Resource from a value.
//
// Parameters:
//   - ctx: the execution context.
//   - value: expected to be a string service name.
//
// Returns:
//   - *Resource: the initialized resource.
//   - error: if value is not a string.
func NewResource(ctx *op.RuntimeEnvironment, value any) (*Resource, error) {

	name, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("service.Resource: expected string name, got %T", value)
	}

	base, err := op.NewResourceBase(ctx, "svc:"+name, reflect.TypeFor[*Resource]())
	if err != nil {
		return nil, err
	}

	return &Resource{
		ResourceBase: base,
		Name:         name,
	}, nil
}

// DiscoverResource constructs a service.Resource and registers it with [op.ResourceCatalog.Discover] without
// claiming production. Used by the framework's resource registry adapter for slot coercion. activationRecord
// is required for signature symmetry with the production-claim path; only activationRecord.Runtime is consumed.
// SiteID is unused (Discover doesn't stamp). Nil-Catalog tolerance returns the unlinked candidate.
func DiscoverResource(activationRecord *op.ActivationRecord, value any) (*Resource, error) {
	candidate, err := NewResource(activationRecord.Runtime, value)
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

// Resource represents a system service.
type Resource struct {
	op.ResourceBase
	Name string
}

// String returns a compact JSON representation of the resource.
func (r *Resource) String() string { return r.Format(r) }

