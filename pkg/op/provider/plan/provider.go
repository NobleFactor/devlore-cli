// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package plan provides graph-construction actions for the plan namespace.
//
// Its methods execute during script evaluation to create nodes in the operation graph. The plan Provider is an
// executing receiver — not a planning receiver — because its methods run immediately to build the graph.
package plan

import (
	"fmt"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
)

var _ op.Provider = (*Provider)(nil) // Interface Guard

// Provider creates graph nodes for plan-time graph construction.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
	Graph    *op.Graph
	mutex    sync.Mutex                   // guards adapters
	adapters map[string]*bind.ProviderNodeBuilder // cached plan adapters by receiver name
}

// NewProvider creates a plan Provider bound to the given context.
func NewProvider(ctx *op.ExecutionContext) *Provider {
	return &Provider{
		ProviderBase: op.NewProviderBase(ctx),
		Graph:        op.NewGraph(ctx),
		adapters:     make(map[string]*bind.ProviderNodeBuilder),
	}
}

// region EXPORTED METHODS

// Options constructs a [bind.Options] value for use as the reserved `options` kwarg on any plan-mode dispatch.
//
// Exposed to starlark as `plan.options(label="...", retry_policy=...)`. Both parameters are optional: an empty label
// triggers auto-labeling at dispatch time (format `<provider>.<method>#<N>`), and a nil retry policy means no retry
// for the underlying node or subgraph.
//
// Parameters:
//   - label: the user-supplied invocation label; empty triggers auto-labeling.
//   - retryPolicy: the retry policy to apply to the invocation's node; nil means no retry.
//
// Returns:
//   - *bind.Options: the constructed options value.
func (p *Provider) Options(label string, retryPolicy *op.RetryPolicy) *bind.Options {

	return &bind.Options{
		Label:       label,
		RetryPolicy: retryPolicy,
	}
}

// ResolveAttr implements op.AttributeResolver.
//
// It routes sub-namespace lookups (e.g., plan.file, plan.git) to the corresponding [bind.ProviderNodeBuilder]. Adapters are
// constructed once per receiver name and cached for the lifetime of this provider.
func (p *Provider) ResolveAttr(name string) any {

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if adapter, ok := p.adapters[name]; ok {
		return adapter
	}

	prt, ok := p.ExecutionContext().Registry.PlannerByName(name)

	if !ok {
		return nil
	}

	adapter := bind.NewProviderNodeBuilder(prt, p.Graph)
	p.adapters[name] = adapter

	return adapter
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// fillSlot fills a slot on a node from a Go value.
//
// Handles Promise, nil, and immediate values.
func (p *Provider) fillSlot(node *op.Node, slotName string, value any) {

	if value == nil {
		return
	}

	if promise, ok := value.(*bind.Promise); ok {
		promise.FillSlot(node, slotName)
		return
	}

	node.SetSlot(slotName, op.ImmediateValue{Value: value})
}

// fillListSlot packs values into indexed sub-slots on a node.
func (p *Provider) fillListSlot(node *op.Node, slotName string, values []any) {

	if len(values) == 0 {
		return
	}

	for i, v := range values {
		subSlot := fmt.Sprintf("%s[%d]", slotName, i)
		p.fillSlot(node, subSlot, v)
	}

	node.SetSlot(slotName+".len", op.ImmediateValue{Value: len(values)})
}

// fillDictSlot packs key-value pairs into keyed sub-slots on a node.
func (p *Provider) fillDictSlot(node *op.Node, slotName string, kwargs map[string]any) {
	for key, v := range kwargs {
		subSlot := fmt.Sprintf("%s.%s", slotName, key)
		p.fillSlot(node, subSlot, v)
	}
}

// endregion

// endregion
