// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Resource represents an in-memory data resource identified by a mem: URI.
//
// Unlike file or git resources that reference external systems, a mem.Resource holds its content directly. The Data
// field contains the raw bytes (source text, JSON, template content, etc.). The ContentType field classifies the
// content for dispatch (e.g., "callable", "json", "template").
//
// The URI is opaque: mem:<content-type>/<qualifier>. The content hash is stored as a metadata field for change
// detection — NOT part of the URI. Two resources with the same URI but different hashes trigger a catalog shadow.
type Resource struct {
	op.ResourceBase
	ContentType string // "callable", "json", "template", "function", etc.
	Namespace   string // receiver type or grouping (e.g., "file.Reducer") — empty when not applicable
	Name        string // specific identifier (e.g., "count_python_files", "config")
	Data        []byte // raw content
	Hash        string // SHA-256 of Data — metadata, NOT part of URI
}

// ResourceSpec carries identity and payload for constructing a mem.Resource.
//
// ContentType classifies the content (e.g., "callable", "json", "template", "function").
// Namespace is the receiver type or grouping (e.g., "file.Reducer", "Predicate") — empty for non-function resources.
// Name is the specific identifier (e.g., "count_python_files", "config").
// Data is an optional payload — []byte for plain resources, *starlark.Function for Function resources.
type ResourceSpec struct {
	ContentType string
	Namespace   string
	Name        string
	Data        any
}

// URI returns the canonical mem: URI for this spec.
//
// Returns:
//   - string: the opaque URI (e.g., "mem:function/file.Reducer/count_python_files").
func (s ResourceSpec) URI() string {

	uri := "mem:" + s.ContentType
	if s.Namespace != "" {
		uri += "/" + s.Namespace
	}
	if s.Name != "" {
		uri += "/" + s.Name
	}
	return uri
}

// NewResource constructs a mem.Resource from a [ResourceSpec].
//
// If spec.Data is a []byte, it is stored as Data and the hash is computed.
//
// Parameters:
//   - ctx: execution context.
//   - value: a [ResourceSpec] with ContentType, Qualifier, and optional Data.
//
// Returns:
//   - *Resource: the constructed resource.
//   - error: if value is not a ResourceSpec or ContentType is empty.
func NewResource(ctx *op.ExecutionContext, value any) (*Resource, error) {

	spec, ok := value.(ResourceSpec)
	if !ok {
		return nil, fmt.Errorf("mem.Resource: expected ResourceSpec, got %T", value)
	}

	if spec.ContentType == "" {
		return nil, fmt.Errorf("mem.Resource: empty content type")
	}

	r := &Resource{
		ResourceBase: op.NewResourceBase(ctx, spec.URI()),
		ContentType:  spec.ContentType,
		Namespace:    spec.Namespace,
		Name:         spec.Name,
	}

	if data, ok := spec.Data.([]byte); ok {
		r.Data = data
		r.ComputeHash()
	}

	return r, nil
}

// region EXPORTED METHODS

// region State management

// String returns a compact JSON representation of the resource.
func (r *Resource) String() string { return r.ResourceBase.Format(r) }

// endregion

// region Behaviors

// ComputeHash calculates the SHA-256 hash of Data and stores it in Hash.
func (r *Resource) ComputeHash() {

	if len(r.Data) == 0 {
		r.Hash = ""
		return
	}
	h := sha256.Sum256(r.Data)
	r.Hash = hex.EncodeToString(h[:])
}

// endregion

// endregion

