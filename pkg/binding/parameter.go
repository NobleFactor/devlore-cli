// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package binding

import "reflect"

// region Parameter

// Parameter is the resolver's view of one parameter to resolve. It is a deliberate small mirror of the
// relevant fields of op.Parameter, kept local to the binding package so pkg/binding has no dependency on
// pkg/op. The executor's preflight pass converts each op.Parameter to a binding.Parameter at the boundary
// before calling [VariableResolver.Resolve].
type Parameter struct {

	// Name is the parameter name (e.g., "target_root"). Used as both the map key in the resolved property
	// map and the lookup key in non-env sources.
	Name string

	// Type is the parameter's declared Go type. Drives env-string parsing and slot-fill type assertion.
	Type reflect.Type

	// Default holds the parameter's declared default. Nil pointer means required; a non-nil pointer means
	// optional, with the pointed-at value as the fallback. The pointer sentinel distinguishes "no default
	// declared" from "default declared as nil."
	Default *any
}

// endregion
