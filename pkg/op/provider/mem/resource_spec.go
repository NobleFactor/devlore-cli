// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

// ResourceSpec carries identity and payload for constructing a [Resource].
//
// At least one of Namespace and Name must be non-empty; an empty spec is the deferred (known-at-execution) form and is
// constructed via [op.Defer] rather than [NewResource].
type ResourceSpec struct {

	// Namespace groups related resources (e.g., "file.Reducer"); first segment of the URI <specific>. May be empty.
	Namespace string

	// Name is the specific identifier (e.g., "config"); second segment of the URI <specific>. May be empty when
	// <specific> is name-only.
	Name string

	// Data is an optional payload; see [NewResource] for the accepted shapes.
	Data any
}

// Specific returns the scheme-specific identity payload for this spec.
//
// The returned form is "<Namespace>/<Name>" when both fields are non-empty, or the non-empty single field otherwise.
// Passed to [op.NewResourceBase] as the canonical tag URI's <specific> portion.
//
// Returns:
//   - string: the <specific> portion of the canonical tag URI.
func (s ResourceSpec) Specific() string {
	if s.Name == "" {
		return s.Namespace
	}
	if s.Namespace == "" {
		return s.Name
	}
	return s.Namespace + "/" + s.Name
}
