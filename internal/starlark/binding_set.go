// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// BindingSet selects which provider bindings a consumer uses and builds
// Starlark globals from them. Consumers call Without() to exclude providers
// they don't need.
type BindingSet struct {
	cfg      op.BindingConfig
	excluded map[string]bool
}

// NewBindingSet creates a BindingSet with the given configuration.
// All registered providers are included by default.
func NewBindingSet(cfg op.BindingConfig) *BindingSet {
	return &BindingSet{
		cfg:      cfg,
		excluded: make(map[string]bool),
	}
}

// Without excludes one or more providers by name.
// Returns the BindingSet for chaining.
func (bs *BindingSet) Without(names ...string) *BindingSet {
	for _, name := range names {
		bs.excluded[name] = true
	}
	return bs
}

// RegisterActions registers all included providers' actions with the registry.
func (bs *BindingSet) RegisterActions(reg *op.ActionRegistry) {
	for _, b := range op.AllBindings() {
		if bs.excluded[b.Name] {
			continue
		}
		if b.ActionRegistrar != nil {
			b.ActionRegistrar(reg)
		}
	}
}

// BuildGlobals constructs the Starlark globals dict for a consumer.
// The "plan" global is a PlanRoot built from included PlannedFactory bindings.
// Each included ImmediateFactory binding becomes a top-level global (e.g., "ui").
func (bs *BindingSet) BuildGlobals(graph *op.Graph, project string, reg *op.ActionRegistry) starlark.StringDict {
	globals := starlark.StringDict{}

	// Collect planned factories for PlanRoot sub-namespaces.
	factories := make(map[string]op.PlannedFactory)
	for _, b := range op.AllBindings() {
		if bs.excluded[b.Name] {
			continue
		}
		if b.PlannedFactory != nil {
			factories[b.Name] = b.PlannedFactory
		}
	}

	if len(factories) > 0 {
		globals["plan"] = NewPlanRootFromFactories(graph, project, reg, factories)
	}

	// Add immediate receivers as top-level globals.
	for _, b := range op.AllBindings() {
		if bs.excluded[b.Name] {
			continue
		}
		if b.ImmediateFactory != nil {
			globals[b.Name] = b.ImmediateFactory(bs.cfg)
		}
	}

	return globals
}
