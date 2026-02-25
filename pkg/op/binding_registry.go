// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"sort"
	"sync"

	"go.starlark.net/starlark"
)

// PlannedFactory creates a plan sub-namespace (e.g., plan.file) for a given
// graph context. Returned by generated planned_gen.go init() functions.
type PlannedFactory func(graph *Graph, project string, reg *ActionRegistry) starlark.Value

// ImmediateFactory creates an immediate receiver (e.g., ui) for direct calls
// during plan construction. Returned by generated immediate_gen.go init() functions.
type ImmediateFactory func(cfg BindingConfig) starlark.Value

// ProviderBinding collects all registration data for a single provider.
// Each generated file's init() calls RegisterBinding with its subset of fields;
// the registry merges them by Name.
type ProviderBinding struct {
	// Name is the provider identifier (e.g., "file", "ui", "host").
	Name string

	// Access defines when this provider's methods are available.
	Access AccessType

	// ActionRegistrar registers graph actions from actions_gen.go.
	ActionRegistrar ProviderRegistrar

	// PlannedFactory creates the plan sub-namespace from planned_gen.go.
	PlannedFactory PlannedFactory

	// ImmediateFactory creates the immediate receiver from immediate_gen.go.
	ImmediateFactory ImmediateFactory
}

var (
	bindingMu       sync.Mutex
	bindingRegistry = map[string]*ProviderBinding{}
)

// RegisterBinding merges a partial ProviderBinding into the registry.
// Multiple init() functions may register for the same provider name;
// non-nil fields are merged into the existing entry.
func RegisterBinding(b *ProviderBinding) {
	bindingMu.Lock()
	defer bindingMu.Unlock()

	existing, ok := bindingRegistry[b.Name]
	if !ok {
		bindingRegistry[b.Name] = b
		return
	}

	// Merge non-zero fields.
	if b.Access != "" {
		existing.Access = b.Access
	}
	if b.ActionRegistrar != nil {
		existing.ActionRegistrar = b.ActionRegistrar
	}
	if b.PlannedFactory != nil {
		existing.PlannedFactory = b.PlannedFactory
	}
	if b.ImmediateFactory != nil {
		existing.ImmediateFactory = b.ImmediateFactory
	}
}

// AllBindings returns all registered provider bindings, sorted by name.
func AllBindings() []*ProviderBinding {
	bindingMu.Lock()
	defer bindingMu.Unlock()

	bindings := make([]*ProviderBinding, 0, len(bindingRegistry))
	for _, b := range bindingRegistry {
		bindings = append(bindings, b)
	}
	sort.Slice(bindings, func(i, j int) bool {
		return bindings[i].Name < bindings[j].Name
	})
	return bindings
}

// BindingByName returns the provider binding for the given name.
func BindingByName(name string) (*ProviderBinding, bool) {
	bindingMu.Lock()
	defer bindingMu.Unlock()

	b, ok := bindingRegistry[name]
	return b, ok
}
