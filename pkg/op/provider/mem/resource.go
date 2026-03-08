// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Resource represents an in-memory data resource identified by a mem: URI.
//
// Unlike file or git resources that reference external systems, a mem.Resource
// holds its content directly. The Data field contains the raw bytes (source text,
// JSON, template content, etc.). The ContentType field classifies the content
// for dispatch (e.g., "callable", "json", "template").
//
// The URI is opaque: mem:<content-type>/<qualifier>. The content hash is stored
// as a metadata field for change detection — NOT part of the URI. Two resources
// with the same URI but different hashes trigger a catalog shadow.
type Resource struct {
	op.ResourceBase
	ContentType string // "callable", "json", "template", etc.
	Qualifier   string // type-specific qualifier (e.g., "file.Reducer/myfn" for callables)
	Data        []byte // raw content
	Hash        string // SHA-256 of Data — metadata, NOT part of URI
}

// String returns a compact JSON representation of the resource.
func (r Resource) String() string { return r.Format(r) }

// buildURI computes the opaque mem: URI.
//
// Format: mem:<content-type>/<qualifier>
func (r *Resource) buildURI() string {
	if r.Qualifier != "" {
		return "mem:" + r.ContentType + "/" + r.Qualifier
	}
	return "mem:" + r.ContentType
}

// ComputeHash calculates the SHA-256 hash of Data and stores it in Hash.
func (r *Resource) ComputeHash() {
	if len(r.Data) == 0 {
		r.Hash = ""
		return
	}
	h := sha256.Sum256(r.Data)
	r.Hash = hex.EncodeToString(h[:])
}

// NewResource creates a mem.Resource with the given content type and qualifier.
// Data must be set separately; Hash is computed when ComputeHash is called.
func NewResource(contentType, qualifier string) Resource {
	r := Resource{
		ContentType: contentType,
		Qualifier:   qualifier,
	}
	r.SetURI(r.buildURI())
	return r
}

// NewResourceWithData creates a mem.Resource with content and computes the hash.
func NewResourceWithData(contentType, qualifier string, data []byte) Resource {
	r := Resource{
		ContentType: contentType,
		Qualifier:   qualifier,
		Data:        data,
	}
	r.SetURI(r.buildURI())
	r.ComputeHash()
	return r
}

func init() {
	// Register callable extractor for the bridge layer.
	op.RegisterCallableExtractor(func(fn *starlark.Function, funcType string) (op.CallableResource, error) {
		c, err := Extract(fn, funcType)
		if err != nil {
			return nil, err
		}
		if err := c.Compile(); err != nil {
			return nil, err
		}
		return c, nil
	})

	op.RegisterConstructor(func(v any) (Resource, error) {
		s, ok := v.(string)
		if !ok {
			return Resource{}, fmt.Errorf("mem.Resource: expected string URI, got %T", v)
		}
		// Parse a mem: URI back into ContentType and Qualifier.
		// Expected format: mem:<content-type>[/<qualifier>]
		if !strings.HasPrefix(s, "mem:") {
			return Resource{}, fmt.Errorf("mem.Resource: expected mem: URI, got %q", s)
		}
		opaque := s[len("mem:"):]
		contentType, qualifier, _ := strings.Cut(opaque, "/")
		if contentType == "" {
			return Resource{}, fmt.Errorf("mem.Resource: empty content type in %q", s)
		}
		r := Resource{
			ContentType: contentType,
			Qualifier:   qualifier,
		}
		r.SetURI(r.buildURI())
		return r, nil
	})
}
