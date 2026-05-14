// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package binding

import "fmt"

// region Variable

// Variable pairs a resolved value with its name and origin. Produced by [VariableResolver.Resolve] and
// consumed by the executor at slot-fill time for [op.Variable] slots.
type Variable struct {

	// Name is the parameter name the variable satisfies. Matches the parameter declared via plan.var(name).
	Name string

	// Value is the resolved value, already parsed to the parameter's declared Go type by the resolver.
	// Env-sourced strings are parsed; other sources supply already-typed values.
	Value any

	// Origin records the source namespace and lookup key that produced this value.
	Origin Origin
}

// String formats as "<name> = <value> [<origin>]". The bracketed origin keeps the boundary between value
// and origin unambiguous even when the value contains spaces.
func (v Variable) String() string {
	return fmt.Sprintf("%s = %v [%s]", v.Name, v.Value, v.Origin)
}

// endregion
