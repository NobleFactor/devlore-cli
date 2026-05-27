// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"os"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// VariableResolver assembles variable values from layered sources with explicit precedence.
// Construction captures a reference to the [application.Application] whose source maps
// (Flags / Config / Overrides) and Name drive the cascade. The resolver is read-only after
// construction except for the internal resolved map populated by [VariableResolver.Resolve].
//
// Precedence per parameter (descending; first hit wins):
//
//  1. Override  — programmatic force.
//  2. Flag      — command-line argument (snake-case keys; see
//     [application.NewApplication] for the kebab→snake normalization).
//  3. Env       — process environment, key = `strings.ToUpper(app.Name) + "_" + strings.ToUpper(CamelToSnake(name))`.
//     Env strings are parsed via [envValue] routed through [Convert] step 5.
//     Resource-typed parameters short-circuit envValue and feed [Convert] step 7
//     (registered Resource construction) directly with the raw string.
//  4. Config    — config map.
//  5. Default   — the parameter's declared default (only when `p.Optional` and
//     `p.Default != nil`).
//
// Missing required parameters (`Optional == false` with no source hit) produce aggregated
// errors naming the parameter and the literal lookup keys the cascade tried.
type VariableResolver struct {
	app *application.Application

	resolved map[string]Variable // populated by Resolve; nil until then
}

// NewVariableResolver constructs a [VariableResolver] from a tool's [application.Application].
//
// Parameters:
//   - `app`: the application handle whose source maps drive the cascade. Must be non-nil.
//
// Returns:
//   - *VariableResolver: the constructed resolver.
func NewVariableResolver(app *application.Application) *VariableResolver {

	return &VariableResolver{app: app}
}

// region EXPORTED METHODS

// region State management

// EnvPrefix returns the env-var lookup prefix derived from `app.Name`.
//
// The prefix is uppercased, with hyphens converted to underscores so multi-word program names
// (`devlore-test`, `noble-factor`) produce POSIX-valid env keys (`DEVLORE_TEST_*`, `NOBLE_FACTOR_*`).
// Returns the empty string when the underlying application is nil or its Name is empty — in that case
// the env step of the cascade is skipped (parameter names alone are too generic to safely shadow
// process env).
//
// Returns:
//   - `string`: the env-var prefix (e.g., "WRIT_" or "DEVLORE_TEST_"), or "" when app or app.Name is empty.
func (r *VariableResolver) EnvPrefix() string {

	if r.app == nil || r.app.Name == "" {
		return ""
	}
	return strings.ToUpper(strings.ReplaceAll(r.app.Name, "-", "_")) + "_"
}

// Get returns the [Variable] resolved for the named parameter.
//
// Panics if called before [VariableResolver.Resolve].
//
// Parameters:
//   - `name`: the parameter name.
//
// Returns:
//   - `Variable`: the resolved variable.
//   - `bool`: true if a variable was resolved for this name; false otherwise.
func (r *VariableResolver) Get(name string) (Variable, bool) {

	assert.True("op.VariableResolver: Get called before Resolve", r.resolved != nil)
	v, ok := r.resolved[name]
	return v, ok
}

// Variables returns the full resolved variable map. Panics if called before [VariableResolver.Resolve].
//
// Returns:
//   - map[string]Variable: the resolved variable map, keyed by parameter name.
func (r *VariableResolver) Variables() map[string]Variable {

	assert.True("op.VariableResolver: Variables called before Resolve", r.resolved != nil)
	return r.resolved
}

// endregion

// region Behaviors

// Resolve walks each parameter through the source precedence chain and populates the resolver's map.
//
// Aggregates errors rather than failing fast — callers (the executor's preflight pass in Phase 4) fold
// the returned slice into the D5 envelope.
//
// Parameters:
//   - `runtimeEnvironment`: the runtime environment carried into [Convert] step 7 for env-sourced Resource
//     targets; may be nil for resolver paths that never reach a Resource target.
//   - `parameters`: the parameter specs to resolve.
//
// Returns:
//   - []error: aggregated errors (missing required, type mismatch, default-type mismatch). Nil on success.
func (r *VariableResolver) Resolve(runtimeEnvironment *RuntimeEnvironment, parameters []Parameter) []error {

	r.resolved = make(map[string]Variable)

	var errs []error

	for _, p := range parameters {

		v, found, err := r.resolveOne(runtimeEnvironment, p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if found {
			r.resolved[p.Name] = v
			continue
		}
		if !p.Optional {
			errs = append(errs, r.missingRequiredError(p))
		}
	}

	return errs
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// resolveOne walks the cascade for a single parameter.
//
// Returns the picked [Variable], a found-flag, and any source-value type-assertion error encountered.
//
// Parameters:
//   - `runtimeEnvironment`: the runtime environment for [Convert] step 7 (env-sourced Resource targets).
//   - `p`: the parameter to resolve.
//
// Returns:
//   - `Variable`: the resolved variable when found.
//   - `bool`: true when a source hit produced a value; false when the cascade ran clean.
//   - `error`: non-nil on type-assertion failure for any non-env source, or on env conversion failure.
func (r *VariableResolver) resolveOne(runtimeEnvironment *RuntimeEnvironment, p Parameter) (Variable, bool, error) {

	if r.app != nil {

		if raw, ok := r.app.Overrides[p.Name]; ok {
			v, err := assignToType(runtimeEnvironment, p.Name, "override", raw, p.Type)
			if err != nil {
				return Variable{}, false, err
			}
			return Variable{
				Name:   p.Name,
				Value:  v,
				Source: VariableSource{Kind: VariableSourceKindOverride, Name: p.Name},
			}, true, nil
		}

		if raw, ok := r.app.Flags[p.Name]; ok {
			v, err := assignToType(runtimeEnvironment, p.Name, "flag", raw, p.Type)
			if err != nil {
				return Variable{}, false, err
			}
			return Variable{
				Name:   p.Name,
				Value:  v,
				Source: VariableSource{Kind: VariableSourceKindFlag, Name: p.Name},
			}, true, nil
		}
	}

	prefix := r.EnvPrefix()
	if prefix != "" {

		envKey := prefix + strings.ToUpper(CamelToSnake(p.Name))

		if raw, ok := os.LookupEnv(envKey); ok {

			// Resource targets short-circuit envValue and feed Convert step 7 (registered Resource
			// construction) with the raw string — the constructor knows the URI dialect; envValue
			// declines Resource targets via CanConvertTo precisely to let this path win.
			var source any = envValue(raw)
			if p.Type != nil && p.Type.Implements(envValueResourceType) {
				source = raw
			}

			v, err := Convert(runtimeEnvironment, source, p.Type)
			if err != nil {
				return Variable{}, false, fmt.Errorf("parameter %q: env %s: %w", p.Name, envKey, err)
			}

			return Variable{
				Name:   p.Name,
				Value:  v,
				Source: VariableSource{Kind: VariableSourceKindEnv, Name: envKey},
			}, true, nil
		}
	}

	if r.app != nil {

		if raw, ok := r.app.Config[p.Name]; ok {
			v, err := assignToType(runtimeEnvironment, p.Name, "config", raw, p.Type)
			if err != nil {
				return Variable{}, false, err
			}
			return Variable{
				Name:   p.Name,
				Value:  v,
				Source: VariableSource{Kind: VariableSourceKindConfig, Name: p.Name},
			}, true, nil
		}
	}

	if p.Optional && p.Default != nil {

		v, err := assignToType(runtimeEnvironment, p.Name, "default", p.Default, p.Type)
		if err != nil {
			return Variable{}, false, err
		}
		return Variable{
			Name:   p.Name,
			Value:  v,
			Source: VariableSource{Kind: VariableSourceKindDefault, Name: p.Name},
		}, true, nil
	}

	return Variable{}, false, nil
}

// missingRequiredError formats the aggregated error for a required parameter that found no source hit.
//
// Parameters:
//   - `p`: the parameter that came up empty across the cascade.
//
// Returns:
//   - `error`: a descriptive error naming the parameter, its declared type, and every lookup key the
//     cascade tried.
func (r *VariableResolver) missingRequiredError(p Parameter) error {

	envKey := "(disabled)"
	if prefix := r.EnvPrefix(); prefix != "" {
		envKey = prefix + strings.ToUpper(CamelToSnake(p.Name))
	}

	return fmt.Errorf(
		"parameter %q (%s) is required but no source supplied a value (tried override=%q, flag=%q, env=%s, config=%q)",
		p.Name, p.Type, p.Name, p.Name, envKey, p.Name)
}

// endregion

// endregion
