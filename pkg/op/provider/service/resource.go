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
		r := Resource{Name: s}
		r.SetURI(r.buildURI())
		return r, nil
	})
}

// Resource represents a system service.
type Resource struct {
	op.ResourceBase
	Name string
}

// String returns a compact JSON representation of the resource.
func (r Resource) String() string { return r.Format(r) }

// buildURI computes the opaque svc: URI.
func (r *Resource) buildURI() string {
	return "svc:" + r.Name
}

// Tombstone holds service-specific compensation state.
type Tombstone struct {
	op.TombstoneBase
	Name       string
	WasRunning bool
	WasEnabled bool
}
