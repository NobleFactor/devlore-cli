// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"fmt"
	"os"
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
	r := Resource{Name: name}
	r.SetURI(r.buildURI())
	return r
}

// NewTypedResource creates a Resource with explicit package name and type.
func NewTypedResource(name, typ string) Resource {
	r := Resource{Name: name, Type: typ}
	r.SetURI(r.buildURI())
	return r
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

// buildURI computes the purl-compliant opaque URI.
func (r *Resource) buildURI() string {
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

// Purl returns the canonical package-url string. For this implementation,
// URI() and Purl() produce the same string.
func (r *Resource) Purl() string {
	return r.URI()
}

// Resolve populates Type from the platform's default package manager
// (when not specified at plan time) and Version from the installed
// package version. The executor injects platform context before calling
// Resolve().
func (r *Resource) Resolve(_ *os.Root) error {
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
