// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// NewResource creates a git.Resource from a value.
//
// The value is a string clone path. The path is canonicalized to an absolute path and the URI is computed at
// construction time as "git:<escaped-path>".
//
// Parameters:
//   - ctx: the execution context.
//   - value: expected to be a string clone path.
//
// Returns:
//   - *Resource: the initialized resource.
//   - error: if value is not a string.
func NewResource(ctx *op.ExecutionContext, value any) (*Resource, error) {

	path, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("git.Resource: expected string path, got %T", value)
	}

	return &Resource{
		ResourceBase: op.NewResourceBase(ctx, gitURI("", path, "")),
		ClonePath:    path,
	}, nil
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

// gitURI computes an opaque git: URI from a repository URL or clone path and optional ref.
func gitURI(repoURL, clonePath, ref string) string {

	var opaque string
	if repoURL != "" {
		opaque = escapeInnerURI(repoURL)
	} else {
		opaque = escapeInnerURI(clonePath)
	}
	s := "git:" + opaque
	if ref != "" {
		s += "#" + ref
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

// Resolve canonicalizes the clone path to an absolute path at execution time.
func (r *Resource) Resolve() error {

	if abs, err := filepath.Abs(r.ClonePath); err == nil {
		r.ClonePath = filepath.Clean(abs)
	}
	return nil
}

// Tombstone holds git-specific compensation state.
type Tombstone struct {
	op.TombstoneBase
	ClonedPath string
}
