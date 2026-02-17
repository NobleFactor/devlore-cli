// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

// Predicate evaluates a condition against a value at execution time.
//
// The execution package defines this interface; the starlark package provides
// the RuntimePredicate implementation that wraps a starlark.Callable. This
// keeps the execution package free of starlark imports.
type Predicate interface {
	// Eval evaluates the predicate against the given value.
	// Returns true if the condition is satisfied.
	Eval(value any) (bool, error)

	// String returns a human-readable representation for display and debugging.
	String() string
}
