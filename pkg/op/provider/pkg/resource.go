// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
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
func NewResource(ctx *op.ExecutionContext, value any) (*Resource, error) {

	raw, ok := value.(string)

	if !ok {
		return nil, fmt.Errorf("pkg.Resource: expected string, got %T", value)
	}

	// Parse optional manager prefix (e.g., "brew:jq", "port:wget").

	var mgr op.PackageManager

	if prefix, after, ok := strings.Cut(raw, ":"); ok {
		mgr = ctx.Platform.GetPackageManager(prefix)
		if mgr == nil {
			return nil, fmt.Errorf("pkg.Resource: unknown package manager %q", prefix)
		}
		raw = after
	} else {
		mgr = ctx.Platform.PackageManager
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

	ctx := r.ExecutionContext()

	if ctx == nil || ctx.Platform == nil {
		return nil
	}

	mgr := ctx.Platform.GetPackageManager(r.Type)

	if mgr == nil {
		return nil
	}

	r.Version = mgr.Version(r.Name)
	return nil
}

// Tombstone holds package-specific compensation state.
type Tombstone struct {
	op.ReceiptBase
	Packages         []string
	Manager          string
	Cask             bool
	AlreadyInstalled []string
	PreviousVersions map[string]string
}

// region EXPORTED METHODS

// region Behaviors

// MarshalJSON encodes the tombstone as JSON: the base envelope (action, resource, transaction_id) extended
// with the package-specific fields (packages, manager, cask, already_installed, previous_versions).
//
// Delegates to [Tombstone.MarshalYAML] for the wire-shape value, then runs [json.Marshal] over it. The base
// inheritance via embedding does not propagate the override — Go method dispatch on an embedded receiver
// reaches only [op.ReceiptBase.MarshalYAML], which would emit the bare envelope without these provider
// fields. The override here intercepts the call so the encoder sees the full document.
//
// Returns:
//   - []byte: JSON-encoded object carrying the base envelope plus the package-specific fields.
//   - error: any error from [Tombstone.MarshalYAML] or from [json.Marshal] (including from the embedded
//     Resource's own marshaler).
func (t *Tombstone) MarshalJSON() ([]byte, error) {

	v, err := t.MarshalYAML()
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// MarshalYAML returns the tombstone's full state as an anonymous struct value the YAML encoder serializes.
//
// The struct reproduces the base envelope's three fields ([op.ReceiptBase.Action], [op.ReceiptBase.Resource],
// [op.ReceiptBase.TransactionID] — read through their public accessors) followed by the five
// package-specific fields. Both `json:` and `yaml:` tags ride on every field so the same value flows through
// either encoder via [Tombstone.MarshalJSON]'s delegation. Resource serializes via the concrete Resource
// type's own [yaml.Marshaler]; PreviousVersions serializes as a YAML mapping with sorted keys (the encoder
// default).
//
// Returns:
//   - any: the populated anonymous struct for the YAML encoder to walk.
//   - error: nil under normal conditions.
func (t *Tombstone) MarshalYAML() (any, error) {

	return struct {
		Action           string            `json:"action"            yaml:"action"`
		Resource         op.Resource       `json:"resource"          yaml:"resource"`
		TransactionID    string            `json:"transaction_id"    yaml:"transaction_id"`
		Packages         []string          `json:"packages"          yaml:"packages"`
		Manager          string            `json:"manager"           yaml:"manager"`
		Cask             bool              `json:"cask"              yaml:"cask"`
		AlreadyInstalled []string          `json:"already_installed" yaml:"already_installed"`
		PreviousVersions map[string]string `json:"previous_versions" yaml:"previous_versions"`
	}{
		Action:           t.Action(),
		Resource:         t.Resource(),
		TransactionID:    t.TransactionID(),
		Packages:         t.Packages,
		Manager:          t.Manager,
		Cask:             t.Cask,
		AlreadyInstalled: t.AlreadyInstalled,
		PreviousVersions: t.PreviousVersions,
	}, nil
}

// endregion

// endregion
