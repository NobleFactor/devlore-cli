// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func init() {
	op.RegisterConstructor(func(v any) (Resource, error) {
		s, ok := v.(string)
		if !ok {
			return Resource{}, fmt.Errorf("pkg.Resource: expected string name, got %T", v)
		}
		return Resource{Name: s}, nil
	})

	op.RegisterPlanTimeConstructor(func(v any) (Resource, error) {
		s, ok := v.(string)
		if !ok {
			return Resource{}, fmt.Errorf("pkg.Resource: expected string name, got %T", v)
		}
		return Resource{Name: s}, nil
	})
}

// Resource represents a system package.
type Resource struct {
	op.ResourceBase
	Name string
}

// URI returns the canonical pkg:// URI for this resource.
func (r *Resource) URI() string { return r.NewURI(r) }

// Scheme returns "pkg".
func (r *Resource) Scheme() string { return op.SchemePackage }

// Host returns empty string.
func (r *Resource) Host() string { return "" }

// Path returns the package name.
func (r *Resource) Path() string { return r.Name }

// Tombstone holds package-specific compensation state.
type Tombstone struct {
	op.TombstoneBase
	Packages         []string
	Manager          string
	Cask             bool
	AlreadyInstalled []string
	PreviousVersions map[string]string
}
