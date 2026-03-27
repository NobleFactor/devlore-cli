// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// ThreadFrom extracts the *starlark.Thread from a Context.
// Returns nil if Thread is nil.
func ThreadFrom(ctx op.Context) *starlark.Thread {
	if ctx.Thread == nil {
		return nil
	}
	return ctx.Thread.(*starlark.Thread)
}

// loaderEntry caches the result of resolving a provider module.
type loaderEntry struct {
	globals starlark.StringDict
	err     error
}

// StarlarkRuntime manages a Starlark scripting runtime using announced receivers.
// Receivers listed in BindingConfig.Receivers are constructed as Starlark globals.
// External consumers (e.g., noblefactor-ops) use this to obtain framework-managed receivers
// without hand-coding receiver construction.
type StarlarkRuntime struct {
	cfg      *op.BindingConfig
	included map[string]bool
	ctx      op.Context // stored by Initialize for receiver context injection
	cache    map[string]*loaderEntry
}

// NewStarlarkRuntime creates a runtime with the given configuration.
// Receivers listed in cfg.Receivers are included when BuildReceivers is called.
//
// Parameters:
//   - cfg: configuration specifying which receivers to include.
//
// Returns:
//   - *StarlarkRuntime: the initialized runtime.
func NewStarlarkRuntime(cfg *op.BindingConfig) *StarlarkRuntime {

	included := make(map[string]bool, len(cfg.Receivers))
	for _, p := range cfg.Receivers {
		included[p.ReceiverName()] = true
	}
	return &StarlarkRuntime{
		cfg:      cfg,
		included: included,
		cache:    make(map[string]*loaderEntry),
	}
}

// region EXPORTED METHODS

// region State management

// Included reports whether the named provider is in the receiver set.
//
// Parameters:
//   - name: the provider name to check.
//
// Returns:
//   - bool: true if the provider is included in this runtime's receiver set.
func (rt *StarlarkRuntime) Included(name string) bool {

	return rt.included[name]
}

// HasGraphBuilder reports whether the plan.* graph namespace is enabled.
//
// Returns:
//   - bool: true if the graph builder is included.
func (rt *StarlarkRuntime) HasGraphBuilder() bool {

	return rt.cfg.GraphBuilder
}

// endregion

// region Behaviors

// Initialize registers all providers' actions with the registry and stores the context
// for later receiver initialization.
//
// Parameters:
//   - reg: the action registry to populate.
//   - ctx: the base execution context for provider initialization.
func (rt *StarlarkRuntime) Initialize(reg *op.ReceiverRegistry, ctx op.ContextBase) {
	rt.ctx = op.Context{ContextBase: ctx}
	if ctx.Root != nil {
		rt.ctx.RecoverySite = op.NewRecoverySite(rt.ctx)
	}
	op.InitAll(reg, rt.ctx)
}

// RegisterActions registers all providers' actions with the registry.
// All providers' actions are always registered regardless of Receivers selections — the action
// registry is for the executor, not the script environment.
//
// Parameters:
//   - reg: the action registry to populate.
//   - ctx: the execution context for provider initialization.
func (rt *StarlarkRuntime) RegisterActions(reg *op.ReceiverRegistry, ctx op.Context) {

	rt.Initialize(reg, ctx.ContextBase)
}

// BuildReceiver constructs a single immediate receiver by provider name.
//
// Parameters:
//   - name: the provider name to build.
//
// Returns:
//   - starlark.Value: the constructed receiver, or nil if not found.
//   - bool: true if the provider was found and implements ExecutingReceiverFactory.
func (rt *StarlarkRuntime) BuildReceiver(name string) (starlark.Value, bool) {

	for _, p := range op.Receivers() {
		if p.ReceiverName() != name {
			continue
		}
		if recv := rt.buildOne(p); recv != nil {
			return recv, true
		}
		return nil, false
	}
	return nil, false
}

// BuildReceivers constructs immediate receivers for factories listed in cfg.Receivers.
//
// Returns:
//   - starlark.StringDict: receiver map suitable for merging into Starlark globals.
func (rt *StarlarkRuntime) BuildReceivers() starlark.StringDict {

	globals := starlark.StringDict{}
	for _, p := range op.Receivers() {
		if !rt.included[p.ReceiverName()] {
			continue
		}
		if recv := rt.buildOne(p); recv != nil {
			globals[p.ReceiverName()] = recv
		}
	}
	return globals
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
func (rt *StarlarkRuntime) BuildGlobals(graph *op.Graph, project string, reg *op.ReceiverRegistry) starlark.StringDict {

	// Start with immediate receivers from the base runtime.
	globals := rt.BuildReceivers()

	// Build "plan" if requested. The plan provider is an ExecutingReceiver
	// whose methods create graph nodes. It uses DynamicReceiver to route
	// sub-namespace lookups (plan.file, plan.git) to PlanningReceiverFactories.
	if rt.HasGraphBuilder() {
		if recv := rt.buildPlanReceiver(graph, project, reg); recv != nil {
			globals["plan"] = recv
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
func (rt *StarlarkRuntime) ConfigureThread(thread *starlark.Thread, graph *op.Graph, project string, reg *op.ReceiverRegistry) {

	thread.Load = rt.makeLoader(graph, project, reg)
}

// NewPopulatedRegistry creates an ReceiverRegistry with all provider actions registered.
// Shorthand for NewActionRegistry() + RegisterActions().
//
// Parameters:
//   - ctx: the execution context for provider initialization.
//
// Returns:
//   - *ReceiverRegistry: the populated registry.
func (rt *StarlarkRuntime) NewPopulatedRegistry(ctx op.Context) *op.ReceiverRegistry {

	reg := op.NewActionRegistry()
	rt.RegisterActions(reg, ctx)
	return reg
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// buildOne constructs an immediate receiver from a provider, injecting context if available.
//
// Parameters:
//   - p: the provider to construct a receiver from.
//
// Returns:
//   - starlark.Value: the constructed receiver, or nil if the provider is not immediate.
func (rt *StarlarkRuntime) buildOne(p op.ReceiverFactory) starlark.Value {

	ip, ok := p.(ExecutingReceiverFactory)
	if !ok {
		return nil
	}
	recv := ip.NewExecuting(rt.ctx)
	if rr, ok := recv.(*ExecutingReceiver); ok && rt.ctx.Root != nil {
		rr.SetContext(rt.ctx)
	}
	return recv
}

// makeLoader creates the thread.Load function for @devlore// modules.
//
// Parameters:
//   - graph: the execution graph for provider resolution.
//   - project: the project name for provider resolution.
//   - reg: the action registry for provider resolution.
//
// Returns:
//   - func: a Starlark module loader function.
func (rt *StarlarkRuntime) makeLoader(graph *op.Graph, project string, reg *op.ReceiverRegistry) func(*starlark.Thread, string) (starlark.StringDict, error) {

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
func (rt *StarlarkRuntime) resolveProvider(name string, graph *op.Graph, project string, reg *op.ReceiverRegistry) (starlark.StringDict, error) {

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

// buildPlanModule constructs a plan module for @devlore//plan import.
func (rt *StarlarkRuntime) buildPlanModule(graph *op.Graph, project string, reg *op.ReceiverRegistry) (starlark.StringDict, error) {
	recv := rt.buildPlanReceiver(graph, project, reg)
	if recv == nil {
		return nil, fmt.Errorf("no plan provider registered")
	}
	return starlark.StringDict{"plan": recv}, nil
}

// buildPlanReceiver creates the plan provider's ExecutingReceiver.
// The plan provider is found by name ("plan") in the announced receivers.
// Context is enriched with graph, project, and action_registry so the
// provider can access them.
func (rt *StarlarkRuntime) buildPlanReceiver(graph *op.Graph, project string, reg *op.ReceiverRegistry) starlark.Value {
	rt.ctx.Graph = graph
	if rt.ctx.Data == nil {
		rt.ctx.Data = make(map[string]any)
	}
	rt.ctx.Data["project"] = project
	rt.ctx.Data["action_registry"] = reg

	recv, ok := rt.BuildReceiver("plan")
	if !ok {
		return nil
	}
	return recv
}

// endregion

// endregion
