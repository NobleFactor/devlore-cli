// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// StarlarkRuntime manages a Starlark scripting runtime.
//
// It constructs providers as Starlark globals from the selected modules and provides the @devlore// module loader.
type StarlarkRuntime struct {
	ctx         *op.ExecutionContext
	cache       map[string]*loaderEntry
	modules     []op.ProviderReceiverType
	predeclared starlark.StringDict
	registry    *op.ReceiverRegistry
}

// NewStarlarkRuntime creates a fully initialized runtime from the given configuration.
//
// The ExecutionContext is built internally from the config. Providers are constructed and cached as the predeclared
// starlark globals. The config itself is not retained.
//
// Parameters:
//   - cfg: configuration specifying the registry, module selection, root, and runtime options.
//
// Returns:
//   - *StarlarkRuntime: the initialized runtime.
func NewStarlarkRuntime(cfg *op.RuntimeEnvironmentSpec) *StarlarkRuntime {

	ctx := op.ExecutionContext{
		Context:     context.Background(),
		ProgramName: cfg.ProgramName,
		Registry:    cfg.Registry,
		Writer:      cfg.Writer,
		Root:        cfg.Root,
		Data:        cfg.Data,
		DryRun:      cfg.DryRun,
		Platform:    op.NewPlatform(),
		Sops:        cfg.Sops,
	}

	if ctx.Root != nil {
		ctx.RecoverySite = op.NewRecoverySite(&ctx)
	}

	runtime := &StarlarkRuntime{
		ctx:      &ctx,
		cache:    make(map[string]*loaderEntry),
		modules:  cfg.Modules,
		registry: cfg.Registry,
	}

	// Build predeclared globals from the selected modules.

	predeclared := starlark.StringDict{}

	for _, module := range cfg.Modules {
		if receiver := runtime.buildOne(module); receiver != nil {
			predeclared[module.Name()] = receiver
		}
	}

	runtime.predeclared = predeclared
	return runtime
}

// region EXPORTED METHODS

// region State management

// Modules returns the selected modules.
//
// Returns:
//   - []op.ProviderReceiverType: the module list.
func (rt *StarlarkRuntime) Modules() []op.ProviderReceiverType {

	return rt.modules
}

// Registry returns the receiver type registry.
//
// Returns:
//   - *op.ReceiverRegistry: the registry.
func (rt *StarlarkRuntime) Registry() *op.ReceiverRegistry {

	return rt.registry
}

// Predeclared returns the cached predeclared starlark globals dict built from the selected modules.
//
// Returns:
//   - starlark.StringDict: the predeclared globals.
func (rt *StarlarkRuntime) Predeclared() starlark.StringDict {

	return rt.predeclared
}

// endregion

// region Behaviors

// BuildReceiver constructs a single immediate receiver by provider name.
//
// Parameters:
//   - name: the provider name to build.
//
// Returns:
//   - starlark.Value: the constructed receiver, or nil if not found.
//   - bool: true if the provider was found in the selected modules.
func (rt *StarlarkRuntime) BuildReceiver(name string) (starlark.Value, bool) {

	for _, mod := range rt.modules {
		if mod.Name() != name {
			continue
		}
		if recv := rt.buildOne(mod); recv != nil {
			return recv, true
		}
		return nil, false
	}
	return nil, false
}

// Invoke executes a starlark script with per-invocation settings.
//
// Script loading is confined to root via os.OpenRoot — relative load() calls cannot escape. The @devlore// module
// loader resolves provider names from the registry. DryRun and Data are set on the shared ExecutionContext for the
// duration of the invocation.
//
// Parameters:
//   - script: path to the script file, relative to root.
//   - root: filesystem root for script loading (confined via os.OpenRoot).
//   - data: per-invocation context data (replaces ExecutionContext.Data for this invocation).
//   - dryRun: per-invocation dry-run flag.
//
// Returns:
//   - starlark.StringDict: the script's global bindings after execution.
//   - error: non-nil if the script fails to load or execute.
func (rt *StarlarkRuntime) Invoke(script string, root string, data map[string]any, dryRun bool) (starlark.StringDict, error) {

	// Confine script loading to root.

	scriptRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open script root %s: %w", root, err)
	}
	defer scriptRoot.Close()

	// Read the script source.

	src, err := scriptRoot.ReadFile(script)
	if err != nil {
		return nil, fmt.Errorf("read script %s: %w", script, err)
	}

	// Set per-invocation state on the shared ExecutionContext.

	rt.ctx.Data = data
	rt.ctx.DryRun = dryRun

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
				return nil, fmt.Errorf("load %s: %w", module, readErr)
			}

			globals, execErr := starlark.ExecFileOptions(&fileOpts, thread, module, moduleSrc, rt.predeclared)
			if execErr != nil {
				return nil, fmt.Errorf("load %s: %w", module, execErr)
			}
			moduleCache[module] = globals
			return globals, nil
		},
	}

	return starlark.ExecFileOptions(&fileOpts, thread, script, src, rt.predeclared)
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

// buildOne constructs an immediate receiver from a module provider via the ExecutionContext provider cache.
//
// Parameters:
//   - prt: the provider receiver type to construct.
//
// Returns:
//   - starlark.Value: the constructed receiver, or nil on failure.
func (rt *StarlarkRuntime) buildOne(prt op.ProviderReceiverType) starlark.Value {

	raw, err := rt.ctx.ModuleByName(prt.Name())
	if err != nil {
		return nil
	}
	instance, ok := raw.(op.Provider)
	if !ok {
		return nil
	}
	return rt.newReceiver(instance)
}

// resolveProvider creates a Starlark module dict for a single provider.
//
// Parameters:
//   - name: the provider name to resolve.
//
// Returns:
//   - starlark.StringDict: the module globals.
//   - error: non-nil if the provider is not found.
func (rt *StarlarkRuntime) resolveProvider(name string) (starlark.StringDict, error) {

	receiver, ok := rt.BuildReceiver(name)
	if !ok {
		return nil, fmt.Errorf("provider %q not found or not in module selection", name)
	}
	return starlark.StringDict{name: receiver}, nil
}

// endregion

// endregion
