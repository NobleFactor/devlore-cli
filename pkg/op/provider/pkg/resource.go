// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"fmt"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func init() {
	op.RegisterConstructor(func(v any) (Resource, error) {
		s, ok := v.(string)
		if !ok {
			return Resource{}, fmt.Errorf("pkg.Resource: expected string name, got %T", v)
		}
		return NewResource(s), nil
	})
}

// NewResource creates a Resource with the given package name.
// The Type is left empty for auto-detection at Resolve() time.
func NewResource(name string) Resource {
	return Resource{Name: name}
}

// NewTypedResource creates a Resource with explicit package name and type.
func NewTypedResource(name, typ string) Resource {
	return Resource{Name: name, Type: typ}
}

// Resource represents a system package.
type Resource struct {
	op.ResourceBase
	Name    string // package name ("jq", "curl", "VisualStudioCode")
	Type    string // purl type / manager ("brew", "deb", "port", "winget")
	Version string // populated by Resolve()
}

// String returns a compact JSON representation of the resource.
func (r Resource) String() string { return r.Format(r) }

// URI returns the canonical pkg:// URI for this resource.
func (r *Resource) URI() string { return r.NewURI(r) }

// Scheme returns "pkg".
func (r *Resource) Scheme() string { return op.SchemePackage }

// Host returns the package type (manager), used as the URI authority.
// With type: pkg://brew/jq. Without type: pkg:///jq.
func (r *Resource) Host() string { return r.Type }

// Path returns the package name with a leading slash.
func (r *Resource) Path() string { return "/" + r.Name }

// Purl returns the canonical package-url string (ECMA-427).
func (r *Resource) Purl() string {
	if r.Type == "winget" {
		// Split "Microsoft.VisualStudioCode" → namespace "Microsoft" / name "VisualStudioCode"
		if ns, name, ok := strings.Cut(r.Name, "."); ok {
			s := "pkg:winget/" + ns + "/" + name
			if r.Version != "" {
				s += "@" + r.Version
			}
			return s
		}
	}
	s := "pkg:" + r.Type + "/" + r.Name
	if r.Version != "" {
		s += "@" + r.Version
	}
	return s
}

// Resolve populates Type from the platform's default package manager
// (when not specified at plan time) and Version from the installed
// package version. The executor injects platform context before calling
// Resolve().
func (r *Resource) Resolve() error {
	// Type and Version resolution requires platform context, which is
	// injected by the executor. This is a skeleton — the executor calls
	// Resolve() after platform injection.
	return nil
}

// Tombstone holds package-specific compensation state.
type Tombstone struct {
	op.TombstoneBase
	Packages         []string
	Manager          string
	Cask             bool
	AlreadyInstalled []string
	PreviousVersions map[string]string
}
