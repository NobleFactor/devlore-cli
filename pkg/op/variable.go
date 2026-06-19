// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "fmt"

// VariableSourceKind identifies a variable-value source category. Numeric values ascend with precedence —
// higher beats lower. Callers can compare kinds directly to determine which source would win in a cascade.
type VariableSourceKind int

const (
	// VariableSourceKindUnknown is the zero value; should not appear on a resolved Variable.
	VariableSourceKindUnknown VariableSourceKind = iota

	// VariableSourceKindDefault — the parameter's declared default; lowest non-unknown precedence.
	VariableSourceKindDefault

	// VariableSourceKindConfig — starlark or YAML config files.
	VariableSourceKindConfig

	// VariableSourceKindEnv — process environment variables, derived prefix from ProgramName.
	VariableSourceKindEnv

	// VariableSourceKindFlag — command-line arguments parsed by the program's flag layer.
	VariableSourceKindFlag

	// VariableSourceKindOverride — programmatic force; highest precedence.
	VariableSourceKindOverride
)

// region EXPORTED METHODS

// region Behaviors

// String returns the canonical lowercase name of the [VariableSourceKind].
//
// Returns:
//   - `string`: the canonical name ("unknown", "default", "config", "env", "flag", or "override").
func (k VariableSourceKind) String() string {

	return [...]string{"unknown", "default", "config", "env", "flag", "override"}[k]
}

// endregion

// endregion

// VariableSource records where a resolved [Variable]'s value came from.
type VariableSource struct {

	// Kind identifies the source category. See [VariableSourceKind] for the enum.
	Kind VariableSourceKind

	// Name is the literal lookup key that matched. Examples: "WRIT_TARGET_ROOT" for an env hit;
	// "target_root" for a flag/config/default hit.
	Name string
}

// region EXPORTED METHODS

// region Behaviors

// String formats as "<kind>:<name>". [VariableSourceKindUnknown] renders as "unknown" alone since no name is
// meaningful in that case.
//
// Returns:
//   - `string`: the canonical "<kind>:<name>" form, or "unknown" for the zero value.
func (s VariableSource) String() string {

	if s.Kind == VariableSourceKindUnknown {
		return "unknown"
	}
	return s.Kind.String() + ":" + s.Name
}

// endregion

// endregion

// Variable pairs a resolved value with its name and source. Produced by [VariableResolver.Resolve] and
// consumed by the executor at slot-fill time for [VariableBinding] slots.
type Variable struct {

	// Name is the parameter name the variable satisfies. Matches the parameter declared via plan.variable(name).
	Name string

	// Value is the resolved value, already parsed to the parameter's declared Go type by the resolver.
	// Env-sourced strings are parsed; other sources supply already-typed values.
	Value any

	// Source records the source kind and lookup key that produced this value.
	Source VariableSource
}

// region EXPORTED METHODS

// region Behaviors

// String formats as "<name> = <value> [<source>]". The bracketed source keeps the boundary between value
// and source unambiguous even when the value contains spaces.
//
// Returns:
//   - `string`: the canonical "<name> = <value> [<source>]" form.
func (v Variable) String() string {

	return fmt.Sprintf("%s = %v [%s]", v.Name, v.Value, v.Source)
}

// endregion

// endregion
