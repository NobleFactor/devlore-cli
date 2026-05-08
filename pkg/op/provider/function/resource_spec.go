// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package function

import "go.starlark.net/starlark"

// ResourceSpec carries identity and payload for constructing a function.Resource.
//
// Namespace is the func type identifier (e.g., "file.Reducer", "Predicate") — required.
// Name is the function name (e.g., "count_python_files"); auto-derived from Data if empty.
// Data is the source of truth: the *starlark.Function whose source is extracted, compiled, and packed.
type ResourceSpec struct {
	Namespace string
	Name      string
	Data      *starlark.Function
}

// Specific returns the scheme-specific identity payload for this spec — the <ns>/<name> form
// passed to [op.NewResourceBase] as the canonical tag URI's <specific> portion.
func (s ResourceSpec) Specific() string {
	if s.Name == "" {
		return s.Namespace
	}
	return s.Namespace + "/" + s.Name
}