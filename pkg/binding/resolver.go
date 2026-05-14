// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package binding

// region VariableResolver

// VariableResolver assembles variable values from layered sources with explicit precedence. Construction is
// via [NewVariableResolver] with functional [Option]s; the resolver is read-only after construction except
// for the internal variable map populated by [VariableResolver.Resolve].
//
// Precedence (descending; first hit wins per parameter):
//
//  1. Override  — programmatic force via [WithOverrides].
//  2. Flag      — command-line argument via [WithFlags].
//  3. Env       — program-specific env var via [WithEnvPrefix] (camelCase → SNAKE_CASE).
//  4. Env       — global env var (program prefix trimmed) — same [NamespaceEnv], distinguished by Origin.Name.
//  5. Config    — config map via [WithConfig].
//  6. Default   — the parameter's declared default.
//
// This skeleton (13.0(n) Phase 1) records sources at construction and produces an empty resolved map. The
// real precedence walk lands in 13.0(n) Phase 2.
type VariableResolver struct {
	overrides    map[string]any
	flags        map[string]any
	envPrefix    string
	globalPrefix string
	config       map[string]any

	resolved map[string]Variable // populated by Resolve; nil until then
}

// region EXPORTED FUNCTIONS

// NewVariableResolver constructs a [VariableResolver] from zero or more [Option]s.
//
// Parameters:
//   - opts: functional options that configure the resolver's sources.
//
// Returns:
//   - *VariableResolver: the constructed resolver.
func NewVariableResolver(opts ...Option) *VariableResolver {

	r := &VariableResolver{}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// endregion

// region EXPORTED METHODS

// Get returns the [Variable] resolved for the named parameter. Panics if called before
// [VariableResolver.Resolve].
//
// Parameters:
//   - name: the parameter name.
//
// Returns:
//   - Variable: the resolved variable.
//   - bool: true if a variable was resolved for this name; false otherwise.
func (r *VariableResolver) Get(name string) (Variable, bool) {

	if r.resolved == nil {
		panic("binding.VariableResolver: Get called before Resolve")
	}
	v, ok := r.resolved[name]
	return v, ok
}

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

// Variables returns the full resolved variable map. Panics if called before [VariableResolver.Resolve].
//
// Returns:
//   - map[string]Variable: the resolved variable map, keyed by parameter name.
func (r *VariableResolver) Variables() map[string]Variable {

	if r.resolved == nil {
		panic("binding.VariableResolver: Variables called before Resolve")
	}
	return r.resolved
}

// endregion

// endregion

// region Option

// Option configures a [VariableResolver] at construction time.
type Option func(*VariableResolver)

// region EXPORTED FUNCTIONS

// WithConfig sources values loaded from configuration files ([NamespaceConfig]).
//
// Parameters:
//   - m: parameter-name-keyed map of config values.
//
// Returns:
//   - Option: applies the config source.
func WithConfig(m map[string]any) Option {
	return func(r *VariableResolver) { r.config = m }
}

// WithEnvPrefix configures the env-var lookup prefixes ([NamespaceEnv]). The program-specific prefix is
// tried first (e.g., "DEVLORE_WRIT_TARGET_ROOT"); on miss, the resolver cascades to the global prefix
// derived by trimming the trailing underscore-separated segment (e.g., "DEVLORE_TARGET_ROOT").
//
// Parameters:
//   - programPrefix: the program-specific prefix (e.g., "DEVLORE_WRIT").
//
// Returns:
//   - Option: applies the env source.
func WithEnvPrefix(programPrefix string) Option {
	return func(r *VariableResolver) {
		r.envPrefix = programPrefix
		r.globalPrefix = derivedGlobalPrefix(programPrefix)
	}
}

// WithFlags sources values that came from command-line argument parsing ([NamespaceFlag]).
//
// Parameters:
//   - m: parameter-name-keyed map of flag-derived values.
//
// Returns:
//   - Option: applies the flag source.
func WithFlags(m map[string]any) Option {
	return func(r *VariableResolver) { r.flags = m }
}

// WithOverrides sources programmatic-force overrides ([NamespaceOverride] — highest precedence).
//
// Parameters:
//   - m: parameter-name-keyed map of override values.
//
// Returns:
//   - Option: applies the override source.
func WithOverrides(m map[string]any) Option {
	return func(r *VariableResolver) { r.overrides = m }
}

// endregion

// endregion

// region INTERNAL FUNCTIONS

// derivedGlobalPrefix returns programPrefix with its trailing underscore-separated segment removed.
// "DEVLORE_WRIT" → "DEVLORE"; "FOO_BAR_BAZ" → "FOO_BAR"; "NOUNDERSCORE" → "".
//
// Parameters:
//   - programPrefix: the program-specific prefix.
//
// Returns:
//   - string: the global cascade prefix; empty if programPrefix contains no underscore.
func derivedGlobalPrefix(programPrefix string) string {

	for i := len(programPrefix) - 1; i >= 0; i-- {
		if programPrefix[i] == '_' {
			return programPrefix[:i]
		}
	}
	return ""
}

// endregion
