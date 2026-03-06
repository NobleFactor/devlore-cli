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
// Receivers listed in BindingConfig.Receivers are included as pre-injected globals.
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
// Receivers listed in cfg.Receivers are included as pre-injected globals.
func NewBindingSet(cfg op.BindingConfig) *BindingSet {
	included := make(map[string]bool, len(cfg.Receivers))
	for _, name := range cfg.Receivers {
		included[name] = true
	}
	return &BindingSet{
		cfg:      cfg,
		included: included,
		cache:    make(map[string]*loaderEntry),
	}
}

// RegisterActions registers all providers' actions with the registry.
// All providers' actions are always registered regardless of With() selections — the action registry is for the
// executor, not the script environment.
func (bs *BindingSet) RegisterActions(reg *op.ActionRegistry, ctx op.Context) {
	op.InitAll(reg, ctx)
}

// NewPopulatedRegistry creates an ActionRegistry with all provider actions registered.
// Shorthand for NewActionRegistry() + RegisterActions().
func (bs *BindingSet) NewPopulatedRegistry(ctx op.Context) *op.ActionRegistry {
	reg := op.NewActionRegistry()
	bs.RegisterActions(reg, ctx)
	return reg
}

// BuildGlobals constructs the Starlark globals dict for a consumer.
// Only providers listed in cfg.Receivers appear as globals.
// If "plan" is listed, a PlanRoot is built from all announced PlannedProvider implementations.
func (bs *BindingSet) BuildGlobals(graph *op.Graph, project string, reg *op.ActionRegistry) starlark.StringDict {
	globals := starlark.StringDict{}

	// Build "plan" if requested via With("plan").
	if bs.included["plan"] {
		planned := collectPlannedProviders()
		if len(planned) > 0 {
			globals["plan"] = NewPlanRootFromProviders(graph, project, reg, planned)
		}
	}

	// Add immediate receivers for explicitly included providers.
	for _, p := range op.Providers() {
		if !bs.included[p.Name()] {
			continue
		}
		if ip, ok := p.(op.ImmediateProvider); ok {
			globals[p.Name()] = ip.NewImmediate(bs.cfg)
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

	for _, p := range op.Providers() {
		if p.Name() != name {
			continue
		}
		ip, ok := p.(op.ImmediateProvider)
		if !ok {
			return nil, fmt.Errorf("provider %q has no immediate factory", name)
		}
		value := ip.NewImmediate(bs.cfg)
		return starlark.StringDict{name: value}, nil
	}
	return nil, fmt.Errorf("no provider %q registered", name)
}

// buildPlanModule constructs a plan module from all announced PlannedProvider implementations.
func (bs *BindingSet) buildPlanModule(graph *op.Graph, project string, reg *op.ActionRegistry) (starlark.StringDict, error) {
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
