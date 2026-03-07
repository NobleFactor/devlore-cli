// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func init() {
	op.RegisterConstructor(func(v any) (Resource, error) {
		s, ok := v.(string)
		if !ok {
			return Resource{}, fmt.Errorf("service.Resource: expected string name, got %T", v)
		}
		return Resource{Name: s}, nil
	})
}

// Resource represents a system service.
type Resource struct {
	op.ResourceBase
	Name string
}

// String returns a compact JSON representation of the resource.
func (r Resource) String() string { return r.Format(r) }

// URI returns the canonical svc:// URI for this resource.
func (r *Resource) URI() string { return r.NewURI(r) }

// Scheme returns "svc".
func (r *Resource) Scheme() string { return op.SchemeService }

// Host returns empty string.
func (r *Resource) Host() string { return "" }

// Path returns the service name.
func (r *Resource) Path() string { return r.Name }

// Tombstone holds service-specific compensation state.
type Tombstone struct {
	op.TombstoneBase
	Name       string
	WasRunning bool
	WasEnabled bool
}
