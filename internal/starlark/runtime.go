// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Runtime is the devlore Starlark runtime. It embeds [op.StarlarkRuntime] for immediate receiver
// management and adds the plan namespace and @devlore// module loader used by the CLI tools.
type Runtime struct {
	*op.StarlarkRuntime
	cache map[string]*loaderEntry
}

// loaderEntry caches the result of resolving a provider module.
type loaderEntry struct {
	globals starlark.StringDict
	err     error
}

// NewRuntime creates a Runtime with the given configuration.
// Receivers listed in cfg.Receivers are included as pre-injected globals.
func NewRuntime(cfg op.BindingConfig) *Runtime {
	return &Runtime{
		StarlarkRuntime: op.NewStarlarkRuntime(cfg),
		cache:           make(map[string]*loaderEntry),
	}
}

// RegisterActions registers all providers' actions with the registry.
// All providers' actions are always registered regardless of Receivers selections — the action
// registry is for the executor, not the script environment.
func (rt *Runtime) RegisterActions(reg *op.ActionRegistry, ctx op.Context) {
	rt.Initialize(reg, ctx.ContextBase)
}

// NewPopulatedRegistry creates an ActionRegistry with all provider actions registered.
// Shorthand for NewActionRegistry() + RegisterActions().
func (rt *Runtime) NewPopulatedRegistry(ctx op.Context) *op.ActionRegistry {
	reg := op.NewActionRegistry()
	rt.RegisterActions(reg, ctx)
	return reg
}

// BuildGlobals constructs the Starlark globals dict for a consumer.
// Only providers listed in cfg.Receivers appear as globals.
// If "plan" is listed, a PlanRoot is built from all announced PlannedProvider implementations.
func (rt *Runtime) BuildGlobals(graph *op.Graph, project string, reg *op.ActionRegistry) starlark.StringDict {
	// Start with immediate receivers from the base runtime.
	globals := rt.BuildReceivers()

	// Build "plan" if requested.
	if rt.Included("plan") {
		planned := collectPlannedProviders()
		if len(planned) > 0 {
			globals["plan"] = NewPlanRootFromProviders(graph, project, reg, planned)
		}
	}

	return globals
}

// ConfigureThread sets thread.Load to the @devlore// module loader.
// The loader resolves provider names from the runtime and caches instances.
// Must be called before starlark.ExecFileOptions.
func (rt *Runtime) ConfigureThread(thread *starlark.Thread, graph *op.Graph, project string, reg *op.ActionRegistry) {
	thread.Load = rt.makeLoader(graph, project, reg)
}

// makeLoader creates the thread.Load function for @devlore// modules.
func (rt *Runtime) makeLoader(graph *op.Graph, project string, reg *op.ActionRegistry) func(*starlark.Thread, string) (starlark.StringDict, error) {
	return func(_ *starlark.Thread, module string) (starlark.StringDict, error) {
		if !strings.HasPrefix(module, "@devlore//") {
			return nil, fmt.Errorf("unknown module: %s (use @devlore// prefix)", module)
		}

		name := strings.TrimPrefix(module, "@devlore//")

		if e, ok := rt.cache[name]; ok {
			return e.globals, e.err
		}

		globals, err := rt.resolveProvider(name, graph, project, reg)
		rt.cache[name] = &loaderEntry{globals, err}
		return globals, err
	}
}

// resolveProvider creates a Starlark module dict for a single provider.
func (rt *Runtime) resolveProvider(name string, graph *op.Graph, project string, reg *op.ActionRegistry) (starlark.StringDict, error) {
	// Special case: plan aggregate
	if name == "plan" {
		return rt.buildPlanModule(graph, project, reg)
	}

	recv, ok := rt.BuildReceiver(name)
	if !ok {
		return nil, fmt.Errorf("provider %q not found or has no immediate factory", name)
	}
	return starlark.StringDict{name: recv}, nil
}

// buildPlanModule constructs a plan module from all announced PlannedProvider implementations.
func (rt *Runtime) buildPlanModule(graph *op.Graph, project string, reg *op.ActionRegistry) (starlark.StringDict, error) {
	planned := collectPlannedProviders()
	if len(planned) == 0 {
		return nil, fmt.Errorf("no planned providers registered")
	}
	plan := NewPlanRootFromProviders(graph, project, reg, planned)
	return starlark.StringDict{"plan": plan}, nil
}

// collectPlannedProviders returns all announced PlannedProvider implementations.
func collectPlannedProviders() map[string]op.PlannedProvider {
	planned := make(map[string]op.PlannedProvider)
	for _, p := range op.Providers() {
		if pp, ok := p.(op.PlannedProvider); ok {
			planned[p.Name()] = pp
		}
	}
	return planned
}
