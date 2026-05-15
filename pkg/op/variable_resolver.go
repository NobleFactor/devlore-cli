// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "strings"

// VariableResolver assembles variable values from layered sources with explicit precedence. Construction
// captures the four sources (overrides, flags, config) plus the program name from which the env-var prefix
// is derived. The resolver is read-only after construction except for the internal resolved map populated by
// [VariableResolver.Resolve].
//
// Precedence (descending; first hit wins per parameter):
//
//  1. Override  — programmatic force.
//  2. Flag      — command-line argument.
//  3. Env       — process environment, prefix = strings.ToUpper(programName) + "_".
//  4. Config    — config map.
//  5. Default   — the parameter's declared default.
//
// 13.0(n) Phase 1 ships this type as a skeleton — sources are captured at construction but [Resolve]
// populates an empty map and returns no errors. Phase 2 lands the precedence walk and the type-assertion +
// env-string-parsing logic.
type VariableResolver struct {
	envPrefix string
	flags     map[string]any
	config    map[string]any
	overrides map[string]any

	resolved map[string]Variable // populated by Resolve; nil until then
}

// NewVariableResolver constructs a [VariableResolver] from the four source inputs.
//
// Parameters:
//   - `programName`: the program name (e.g., "writ"); uppercased + "_" forms the env-var prefix.
//   - flags: parameter-name-keyed map of values parsed from command-line flags.
//   - config: parameter-name-keyed map of values loaded from config files.
//   - overrides: parameter-name-keyed map of programmatic overrides (highest precedence).
//
// Returns:
//   - *VariableResolver: the constructed resolver.
func NewVariableResolver(programName string, flags, config, overrides map[string]any) *VariableResolver {

	return &VariableResolver{
		envPrefix: strings.ToUpper(programName) + "_",
		flags:     flags,
		config:    config,
		overrides: overrides,
	}
}

// region EXPORTED METHODS

// region State management

// Get returns the [Variable] resolved for the named parameter. Panics if called before
// [VariableResolver.Resolve].
//
// Parameters:
//   - `name`: the parameter name.
//
// Returns:
//   - Variable: the resolved variable.
//   - `bool`: true if a variable was resolved for this name; false otherwise.
func (r *VariableResolver) Get(name string) (Variable, bool) {

	if r.resolved == nil {
		panic("op.VariableResolver: Get called before Resolve")
	}
	v, ok := r.resolved[name]
	return v, ok
}

// Variables returns the full resolved variable map. Panics if called before [VariableResolver.Resolve].
//
// Returns:
//   - map[string]Variable: the resolved variable map, keyed by parameter name.
func (r *VariableResolver) Variables() map[string]Variable {

	if r.resolved == nil {
		panic("op.VariableResolver: Variables called before Resolve")
	}
	return r.resolved
}

// endregion

// region Behaviors

// Resolve walks each parameter through the source precedence chain and populates the resolver's internal
// variable map. The real precedence walk lands in 13.0(n) Phase 2; this Phase 1 skeleton initializes an
// empty resolved map and returns no errors so consumers can be wired now.
//
// Parameters:
//   - parameters: the parameter specs to resolve.
//
// Returns:
//   - []error: aggregated errors (missing required, type mismatch, default-type mismatch). Nil on success.
func (r *VariableResolver) Resolve(parameters []Parameter) []error {

	r.resolved = make(map[string]Variable)
	return nil
}

// endregion

// endregion
