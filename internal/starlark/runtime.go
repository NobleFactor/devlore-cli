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
//
// Parameters:
//   - cfg: configuration specifying providers, writer, and program name.
//
// Returns:
//   - *Runtime: the initialized runtime.
func NewRuntime(cfg *op.BindingConfig) *Runtime {

	return &Runtime{
		StarlarkRuntime: op.NewStarlarkRuntime(cfg),
		cache:           make(map[string]*loaderEntry),
	}
}

// region EXPORTED METHODS

// region Behaviors

// RegisterActions registers all providers' actions with the registry.
// All providers' actions are always registered regardless of Receivers selections — the action
// registry is for the executor, not the script environment.
//
// Parameters:
//   - reg: the action registry to populate.
//   - ctx: the execution context for provider initialization.
func (rt *Runtime) RegisterActions(reg *op.ActionRegistry, ctx op.Context) {

	rt.Initialize(reg, ctx.ContextBase)
}

// BuildGlobals constructs the Starlark globals dict for a consumer.
// Only receivers listed in cfg.Receivers appear as globals.
// If GraphBuilder is enabled, a PlanRoot is built from all announced PlanningReceiverFactory implementations.
//
// Parameters:
//   - graph: the execution graph for plan namespace construction.
//   - project: the project name passed to planned providers.
//   - reg: the action registry for plan namespace construction.
//
// Returns:
//   - starlark.StringDict: the complete globals dict for Starlark execution.
func (rt *Runtime) BuildGlobals(graph *op.Graph, project string, reg *op.ActionRegistry) starlark.StringDict {

	// Start with immediate receivers from the base runtime.
	globals := rt.BuildReceivers()

	// Build "plan" if requested.
	if rt.HasGraphBuilder() {
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
//
// Parameters:
//   - thread: the Starlark thread to configure.
//   - graph: the execution graph for module resolution.
//   - project: the project name for module resolution.
//   - reg: the action registry for module resolution.
func (rt *Runtime) ConfigureThread(thread *starlark.Thread, graph *op.Graph, project string, reg *op.ActionRegistry) {

	thread.Load = rt.makeLoader(graph, project, reg)
}

// NewPopulatedRegistry creates an ActionRegistry with all provider actions registered.
// Shorthand for NewActionRegistry() + RegisterActions().
//
// Parameters:
//   - ctx: the execution context for provider initialization.
//
// Returns:
//   - *op.ActionRegistry: the populated registry.
func (rt *Runtime) NewPopulatedRegistry(ctx op.Context) *op.ActionRegistry {

	reg := op.NewActionRegistry()
	rt.RegisterActions(reg, ctx)
	return reg
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// makeLoader creates the thread.Load function for @devlore// modules.
//
// Parameters:
//   - graph: the execution graph for provider resolution.
//   - project: the project name for provider resolution.
//   - reg: the action registry for provider resolution.
//
// Returns:
//   - func: a Starlark module loader function.
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
//
// Parameters:
//   - name: the provider name to resolve.
//   - graph: the execution graph for plan module construction.
//   - project: the project name for plan module construction.
//   - reg: the action registry for plan module construction.
//
// Returns:
//   - starlark.StringDict: the module globals.
//   - error: non-nil if the provider is not found.
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

// buildPlanModule constructs a plan module from all announced PlanningReceiverFactory implementations.
//
// Parameters:
//   - graph: the execution graph for plan construction.
//   - project: the project name for plan construction.
//   - reg: the action registry for plan construction.
//
// Returns:
//   - starlark.StringDict: the plan module globals.
//   - error: non-nil if no planned providers are registered.
func (rt *Runtime) buildPlanModule(graph *op.Graph, project string, reg *op.ActionRegistry) (starlark.StringDict, error) {

	planned := collectPlannedProviders()
	if len(planned) == 0 {
		return nil, fmt.Errorf("no planned providers registered")
	}
	plan := NewPlanRootFromProviders(graph, project, reg, planned)
	return starlark.StringDict{"plan": plan}, nil
}

// endregion

// endregion

// collectPlannedProviders returns all announced PlanningReceiverFactory implementations.
//
// Returns:
//   - map[string]op.PlanningReceiverFactory: provider name to PlanningReceiverFactory mapping.
func collectPlannedProviders() map[string]op.PlanningReceiverFactory {

	planned := make(map[string]op.PlanningReceiverFactory)
	for _, p := range op.Receivers() {
		if pp, ok := p.(op.PlanningReceiverFactory); ok {
			planned[p.ReceiverName()] = pp
		}
	}
	return planned
}
