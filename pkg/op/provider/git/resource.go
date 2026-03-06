// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"fmt"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func init() {
	op.RegisterConstructor(func(v any) (Resource, error) {
		s, ok := v.(string)
		if !ok {
			return Resource{}, fmt.Errorf("git.Resource: expected string path, got %T", v)
		}
		return Resource{ClonePath: s}, nil
	})
}

// Resource represents a cloned git repository.
type Resource struct {
	op.ResourceBase
	URL       string
	ClonePath string
	Ref       string
}

// URI returns the canonical git:// URI for this resource.
func (r *Resource) URI() string { return r.NewURI(r) }

// Scheme returns "git".
func (r *Resource) Scheme() string { return op.SchemeGit }

// Host returns empty string — git URIs use path-only identification.
func (r *Resource) Host() string { return "" }

// Path returns the clone path. Before Resolve(), this is the raw path as
// given. After Resolve(), it is the canonicalized absolute path.
func (r *Resource) Path() string { return r.ClonePath }

// Resolve canonicalizes the clone path to an absolute path.
func (r *Resource) Resolve() error {
	abs, err := filepath.Abs(r.ClonePath)
	if err == nil {
		r.ClonePath = filepath.Clean(abs)
	}
	return nil
}

// Tombstone holds git-specific compensation state.
type Tombstone struct {
	op.TombstoneBase
	ClonedPath string
}
