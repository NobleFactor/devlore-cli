// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

// ResourceSpec carries identity and payload for constructing a mem.Resource.
//
// Namespace groups related resources (e.g., "file.Reducer") — first segment of the URI specific.
// Name is the specific identifier (e.g., "config") — second segment of the URI specific.
// At least one of Namespace / Name must be non-empty; an empty spec is the deferred (known-at-execution) form
// and is constructed via [op.Defer] rather than [NewResource].
// Data is an optional payload — see [NewResource] for accepted shapes.
type ResourceSpec struct {
	Namespace string
	Name      string
	Data      any
}

// Specific returns the scheme-specific identity payload for this spec — the <ns>/<name> form passed to
// [op.NewResourceBase] as the canonical tag URI's <specific> portion.
func (s ResourceSpec) Specific() string {
	if s.Name == "" {
		return s.Namespace
	}
	if s.Namespace == "" {
		return s.Name
	}
	return s.Namespace + "/" + s.Name
}