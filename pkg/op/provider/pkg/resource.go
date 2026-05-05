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

// NewResource creates a pkg.Resource from a value.
//
// The value is a string package name with an optional manager prefix (e.g., "jq", "brew:jq", "port:wget",
// "Microsoft.VisualStudioCode@1.89"). When no prefix is present, the platform's default package manager is used.
// The manager's ParsePURL method formulates the purl identity from the package name.
//
// Parameters:
//   - ctx: the execution context (must have Platform set).
//   - value: expected to be a string package name.
//
// Returns:
//   - *Resource: the initialized resource with a valid purl URI.
//   - error: if value is not a string or the manager prefix is unknown.
func NewResource(ctx *op.RuntimeEnvironment, value any) (*Resource, error) {

	raw, ok := value.(string)

	if !ok {
		return nil, fmt.Errorf("pkg.Resource: expected string, got %T", value)
	}

	// Parse optional manager prefix (e.g., "brew:jq", "port:wget").

	var mgr platform.PackageManager

	if prefix, after, ok := strings.Cut(raw, ":"); ok {
		mgr = ctx.Platform.PackageManagerByName(prefix)
		if mgr == nil {
			return nil, fmt.Errorf("pkg.Resource: unknown package manager %q", prefix)
		}
		raw = after
	} else {
		mgr = ctx.Platform.DefaultPackageManager()
	}

	purl := mgr.ParsePURL(raw)

	base, err := op.NewResourceBase(ctx, purl.String(), reflect.TypeFor[*Resource]())
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

