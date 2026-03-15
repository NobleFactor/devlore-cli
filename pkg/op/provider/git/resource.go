// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// ResourceFromValue constructs a git.Resource from a string clone path.
//
// Parameters:
//   - v: expected to be a string path
//
// Returns:
//   - Resource: initialized with the given clone path
//   - error: if v is not a string
func ResourceFromValue(v any) (Resource, error) {

	s, ok := v.(string)
	if !ok {
		return Resource{}, fmt.Errorf("git.Resource: expected string path, got %T", v)
	}
	r := Resource{ClonePath: s}
	r.SetURI(r.buildURI())
	return r, nil
}

// Resource represents a cloned git repository.
type Resource struct {
	op.ResourceBase
	URL       string
	ClonePath string
	Ref       string
}

// String returns a compact JSON representation of the resource.
func (r *Resource) String() string { return r.Format(r) }

// buildURI computes the opaque git: URI.
//
// Format: git:<encoded-repo-url>[?path=<path>]#<commit>
// When URL is empty (local clone), the clone path is used as the opaque data.
func (r *Resource) buildURI() string {
	var opaque string
	if r.URL != "" {
		opaque = escapeInnerURI(r.URL)
	} else {
		opaque = escapeInnerURI(r.ClonePath)
	}
	s := "git:" + opaque
	if r.Ref != "" {
		s += "#" + r.Ref
	}
	return s
}

// escapeInnerURI percent-encodes # and ? in an inner URI so they don't
// interfere with the outer URI's fragment and query parsing.
func escapeInnerURI(s string) string {
	// url.PathEscape is too aggressive (encodes /). We only need to
	// escape the characters that would be consumed by url.Parse.
	var b []byte
	for i := range len(s) {
		switch s[i] {
		case '#':
			b = append(b, '%', '2', '3')
		case '?':
			b = append(b, '%', '3', 'F')
		default:
			b = append(b, s[i])
		}
	}
	return string(b)
}

// unescapeInnerURI reverses the targeted escaping of escapeInnerURI.
func unescapeInnerURI(s string) string {
	u, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return u
}

// Resolve canonicalizes the clone path to an absolute path and updates the URI.
func (r *Resource) Resolve(_ op.Root) error {
	abs, err := filepath.Abs(r.ClonePath)
	if err == nil {
		r.ClonePath = filepath.Clean(abs)
		r.SetURI(r.buildURI())
	}
	return nil
}

// Tombstone holds git-specific compensation state.
type Tombstone struct {
	op.TombstoneBase
	ClonedPath string
}
