// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"strings"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// BindingSet selects which provider bindings a consumer uses and builds Starlark globals from them.
// Consumers call With() to include specific providers as pre-injected globals.
// All other providers remain available via load("@devlore//name", "name") in scripts.
type BindingSet struct {
	cfg      op.BindingConfig
	included map[string]bool
	cache    map[string]*loaderEntry
}

// loaderEntry caches the result of resolving a provider module.
type loaderEntry struct {
	globals starlark.StringDict
	err     error
}

// NewBindingSet creates a BindingSet with the given configuration.
// No providers are included as globals by default.
func NewBindingSet(cfg op.BindingConfig) *BindingSet {
	return &BindingSet{
		cfg:      cfg,
		included: make(map[string]bool),
		cache:    make(map[string]*loaderEntry),
	}
}

// With includes one or more providers as pre-injected globals.
// "plan" is a special name that includes the PlanRoot aggregate.
// Returns the BindingSet for chaining.
func (bs *BindingSet) With(names ...string) *BindingSet {
	for _, name := range names {
		bs.included[name] = true
	}
	return bs
}

// RegisterActions registers all providers' actions with the registry.
// All providers' actions are always registered regardless of With() selections — the action registry is for the
// executor, not the script environment.
func (bs *BindingSet) RegisterActions(reg *op.ActionRegistry, ctx op.Context) {
	for _, b := range op.AllBindings() {
		if b.ActionRegistrar != nil {
			b.ActionRegistrar(reg, ctx)
		}
	}
}

// NewPopulatedRegistry creates an ActionRegistry with all provider actions registered.
// Shorthand for NewActionRegistry() + RegisterActions().
func (bs *BindingSet) NewPopulatedRegistry(ctx op.Context) *op.ActionRegistry {
	reg := op.NewActionRegistry()
	bs.RegisterActions(reg, ctx)
	return reg
}

// BuildGlobals constructs the Starlark globals dict for a consumer.
// Only providers named in With() appear as globals.
// If "plan" was included via With(), a PlanRoot is built from all registered PlannedFactory bindings.
func (bs *BindingSet) BuildGlobals(graph *op.Graph, project string, reg *op.ActionRegistry) starlark.StringDict {
	globals := starlark.StringDict{}

	// Build "plan" if requested via With("plan").
	if bs.included["plan"] {
		factories := collectPlannedFactories()
		if len(factories) > 0 {
			globals["plan"] = NewPlanRootFromFactories(graph, project, reg, factories)
		}
	}

	// Add immediate receivers for explicitly included providers.
	for _, b := range op.AllBindings() {
		if !bs.included[b.Name] {
			continue
		}
		if b.ImmediateFactory != nil {
			globals[b.Name] = b.ImmediateFactory(bs.cfg)
		}
	}

	return globals
}

// ConfigureThread sets thread.Load to the @devlore// module loader.
// The loader resolves provider names from the binding registry and caches instances on the BindingSet.
// Must be called before starlark.ExecFileOptions.
func (bs *BindingSet) ConfigureThread(thread *starlark.Thread, graph *op.Graph, project string, reg *op.ActionRegistry) {
	thread.Load = bs.makeLoader(graph, project, reg)
}

// makeLoader creates the thread.Load function for @devlore// modules.
func (bs *BindingSet) makeLoader(graph *op.Graph, project string, reg *op.ActionRegistry) func(*starlark.Thread, string) (starlark.StringDict, error) {
	return func(_ *starlark.Thread, module string) (starlark.StringDict, error) {
		if !strings.HasPrefix(module, "@devlore//") {
			return nil, fmt.Errorf("unknown module: %s (use @devlore// prefix)", module)
		}

		name := strings.TrimPrefix(module, "@devlore//")

		if e, ok := bs.cache[name]; ok {
			return e.globals, e.err
		}

		globals, err := bs.resolveProvider(name, graph, project, reg)
		bs.cache[name] = &loaderEntry{globals, err}
		return globals, err
	}
}

// resolveProvider creates a Starlark module dict for a single provider.
func (bs *BindingSet) resolveProvider(name string, graph *op.Graph, project string, reg *op.ActionRegistry) (starlark.StringDict, error) {
	// Special case: plan aggregate
	if name == "plan" {
		return bs.buildPlanModule(graph, project, reg)
	}

	binding, ok := op.BindingByName(name)
	if !ok {
		return nil, fmt.Errorf("no provider %q registered", name)
	}

	if binding.ImmediateFactory == nil {
		return nil, fmt.Errorf("provider %q has no immediate factory", name)
	}

	value := binding.ImmediateFactory(bs.cfg)
	return starlark.StringDict{name: value}, nil
}

// buildPlanModule constructs a plan module from all registered PlannedFactory bindings.
func (bs *BindingSet) buildPlanModule(graph *op.Graph, project string, reg *op.ActionRegistry) (starlark.StringDict, error) {
	factories := collectPlannedFactories()
	if len(factories) == 0 {
		return nil, fmt.Errorf("no planned providers registered")
	}
	plan := NewPlanRootFromFactories(graph, project, reg, factories)
	return starlark.StringDict{"plan": plan}, nil
}

// collectPlannedFactories returns all registered PlannedFactory bindings.
func collectPlannedFactories() map[string]op.PlannedFactory {
	factories := make(map[string]op.PlannedFactory)
	for _, b := range op.AllBindings() {
		if b.PlannedFactory != nil {
			factories[b.Name] = b.PlannedFactory
		}
	}
	return factories
}
