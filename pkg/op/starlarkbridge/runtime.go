// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"os"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// Runtime manages a Starlark scripting runtime.
//
// It constructs providers as Starlark globals from the selected modules and provides the @devlore// module loader.
type Runtime struct {
	environment *op.RuntimeEnvironment
	cache       map[string]*loaderEntry
	modules     []op.ProviderReceiverType
	predeclared starlark.StringDict
	registry    *op.ReceiverRegistry
}

// NewRuntime creates a fully initialized runtime that borrows the supplied [op.RuntimeEnvironment].
//
// The runtime does NOT own the env's lifetime — the caller (typically an [op.Plan] closure, a tool session-owner like
// [star.Application], or a wrapper that explicitly built the env) constructs the env, passes it here for the duration
// of starlark work, and is responsible for `defer env.Close()`. Providers are constructed and cached as the predeclared
// starlark globals from `env.Registry.Modules()`.
//
// Parameters:
//   - `env`: the runtime environment to borrow. Its Registry's full module set is exposed as starlark globals.
//
// Returns:
//   - *Runtime: the initialized runtime borrowing the supplied env.
func NewRuntime(env *op.RuntimeEnvironment) *Runtime {

	modules := env.Registry.Modules()

	runtime := &Runtime{
		environment: env,
		cache:       make(map[string]*loaderEntry),
		modules:     modules,
		registry:    env.Registry,
	}

	// Build predeclared globals from the selected modules. Registration branches on the access × root combination
	// declared by each provider (see phase-8 D12):
	//
	//   immediate, root=false → top-level global under the provider's name (status quo for plan, pkg, archive, …).
	//   immediate, root=true → each method installed as its own top-level predeclared entry; the provider instance
	//                          itself is not exposed. Reserved; no Phase 8 provider uses this row.
	//   planned, root=false → NOT registered; reached via plan.<provider>.<method> through plan.Provider's
	//                         sub-namespace dispatch (status quo for file, git, service, …).
	//   planned, root=true → NOT registered; plan.Provider discovers the provider via registry.RootProviders()
	//                        and hosts its methods flat at the plan namespace root via Tier 2 dispatch (flow).
	//
	// Providers that declare both RoleModule and RoleAction (access=both) register their module side per the
	// dispatch-zone rows above; their planned side is reached via plan.* regardless of placement.

	predeclared := starlark.StringDict{}

	for _, module := range modules {

		dispatch := module.Roles().Dispatch()
		isRoot := module.Roles().Placement()&op.RoleRoot != 0

		if dispatch&op.RoleModule == 0 {
			// No module-mode dispatch; provider is not addressable as a top-level global. Its planned side, if any,
			// is reached via plan.* dispatch at runtime.
			continue
		}

		if !isRoot {
			if sv := runtime.buildOne(module); sv != nil {
				predeclared[module.Name()] = sv
			}
			continue
		}

		// Immediate + root: install each method as its own top-level predeclared entry.

		sv := runtime.buildOne(module)
		if sv == nil {
			continue
		}

		hasAttrs, ok := sv.(starlark.HasAttrs)

		assert.Truef(ok, "provider %s wrapper (%T) does not implement starlark.HasAttrs",
			module.Name(),
			sv)

		for m := range module.Methods() {

			snake := op.CamelToSnake(m.Name())

			if existing, collides := predeclared[snake]; collides {
				assert.Failf("top-level global %q declared on both %s (root immediate) and existing predeclared (%T)",
					snake,
					module.Name(),
					existing)
			}

			attr, err := hasAttrs.Attr(snake)

			assert.Truef(err != nil,
				"provider %q: method %q (snake_case %q) registered in receiver type but Attr(%q) failed — registry/Attr mismatch: %v",
				module.Name(),
				m.Name(),
				snake,
				snake,
				err)

			predeclared[snake] = attr
		}
	}

	runtime.predeclared = predeclared
	return runtime
}

// region EXPORTED METHODS

// region State management

// Environment returns the runtime environment context.
//
// Returns:
//   - *op.RuntimeEnvironment: the environment.
func (rt *Runtime) Environment() *op.RuntimeEnvironment {
	return rt.environment
}

// Modules returns the selected modules.
//
// Returns:
//   - []op.ProviderReceiverType: the module list.
func (rt *Runtime) Modules() []op.ProviderReceiverType {

	return rt.modules
}

// Registry returns the receiver type registry.
//
// Returns:
//   - *op.ReceiverRegistry: the registry.
func (rt *Runtime) Registry() *op.ReceiverRegistry {

	return rt.registry
}

// Predeclared returns the cached predeclared starlark globals dict built from the selected modules.
//
// Returns:
//   - starlark.StringDict: the predeclared globals.
func (rt *Runtime) Predeclared() starlark.StringDict {

	return rt.predeclared
}

// endregion

// region Behaviors

// NewModule constructs a new starlark.Value for the named provider.
//
// Parameters:
//   - name: the provider name to build.
//
// Returns:
//   - starlark.Value: the constructed [starlark.Value], or nil if not found.
//   - bool: true if the provider was found in the selected modules.
func (rt *Runtime) NewModule(name string) (starlark.Value, bool) {

	for _, module := range rt.modules {

		if module.Name() != name {
			continue
		}

		if sv := rt.buildOne(module); sv != nil {
			return sv, true
		}

		return nil, false
	}

	return nil, false
}

// Invoke executes a starlark script.
//
// Script loading is confined to root via [os.OpenRoot] — relative load calls cannot escape. The `@devlore//` module
// loader resolves provider names from the registry. Dry-run mode is read from the tool's
// [application.Application] (carried on the shared [op.RuntimeEnvironment]); the caller does not pass it
// per-invocation.
//
// Parameters:
//   - `script`: path to the script file, relative to root.
//   - `root`: filesystem root for script loading (confined via [os.OpenRoot]).
//
// Returns:
//   - [starlark.StringDict]: the script's global bindings after execution.
//   - `error`: non-nil if the script fails to load or execute.
func (rt *Runtime) Invoke(script string, root string) (result starlark.StringDict, err error) {

	// Confine script loading to root.

	var scriptRoot *os.Root

	scriptRoot, err = os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("cannot open script root %q: %w", root, err)
	}
	defer iox.Close(&err, scriptRoot)

	// Read the script source.

	var source []byte

	source, err = scriptRoot.ReadFile(script)
	if err != nil {
		return nil, fmt.Errorf("cannot read script %q: %w", script, err)
	}

	// Dialect options.

	fileOpts := syntax.FileOptions{
		Set:             true,
		While:           true,
		TopLevelControl: true,
		GlobalReassign:  true,
		Recursion:       true,
	}

	// Module cache for relative load() calls within this invocation.

	moduleCache := map[string]starlark.StringDict{}

	// Create thread with loader.

	thread := &starlark.Thread{
		Name: script,
		Load: func(thread *starlark.Thread, module string) (starlark.StringDict, error) {

			// @devlore// modules resolve from the registry.

			if strings.HasPrefix(module, "@devlore//") {
				name := strings.TrimPrefix(module, "@devlore//")

				if e, ok := rt.cache[name]; ok {
					return e.globals, e.err
				}

				globals, loadErr := rt.resolveProvider(name)
				rt.cache[name] = &loaderEntry{globals, loadErr}
				return globals, loadErr
			}

			// Relative imports resolve from the confined root.

			if cached, ok := moduleCache[module]; ok {
				return cached, nil
			}

			moduleSrc, readErr := scriptRoot.ReadFile(module)
			if readErr != nil {
				return nil, fmt.Errorf("cannot load %q: %w", module, readErr)
			}

			globals, execErr := starlark.ExecFileOptions(&fileOpts, thread, module, moduleSrc, rt.predeclared)
			if execErr != nil {
				return nil, fmt.Errorf("cannot load %q: %w", module, execErr)
			}
			moduleCache[module] = globals
			return globals, nil
		},
	}

	return starlark.ExecFileOptions(&fileOpts, thread, script, source, rt.predeclared)
}

// endregion

// endregion

// region UNEXPORTED TYPES

// loaderEntry caches the result of resolving a provider module.
type loaderEntry struct {
	globals starlark.StringDict
	err     error
}

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// buildOne constructs a [starlark.Value] from a provider receiver type via the Environment provider cache.
//
// Parameters:
//   - prt: the provider receiver type.
//
// Returns:
//   - starlark.Value: the constructed [starlark.Value], or nil on failure.
func (rt *Runtime) buildOne(prt op.ProviderReceiverType) starlark.Value {

	raw, err := rt.environment.ModuleByName(prt.Name())
	if err != nil {
		return nil
	}

	instance, ok := raw.(op.Provider)
	if !ok {
		return nil
	}

	return newGoReceiver(prt, instance)
}

// resolveProvider creates a Starlark module dict for a single provider.
//
// Parameters:
//   - name: the provider name to resolve.
//
// Returns:
//   - starlark.StringDict: the module globals.
//   - error: non-nil if the provider is not found.
func (rt *Runtime) resolveProvider(name string) (starlark.StringDict, error) {

	sv, ok := rt.NewModule(name)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", name)
	}

	return starlark.StringDict{name: sv}, nil
}

// endregion

// endregion
