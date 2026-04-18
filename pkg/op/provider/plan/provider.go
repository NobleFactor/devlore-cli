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
	mutex    sync.Mutex               // guards planners
	planners map[string]*bind.Planner // cached planners by receiver name
}

// NewProvider creates a plan Provider bound to the given context.
func NewProvider(ctx *op.ExecutionContext) *Provider {
	return &Provider{
		ProviderBase: op.NewProviderBase(ctx),
		Graph:        op.NewGraph(ctx),
		planners:     make(map[string]*bind.Planner),
	}
}

// region EXPORTED METHODS

// ResolveAttr implements op.AttributeResolver.
//
// It routes sub-namespace lookups (e.g., plan.file, plan.git) to the corresponding [bind.Planner]. Planners are
// constructed once per receiver name and cached for the lifetime of this provider.
func (p *Provider) ResolveAttr(name string) any {

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if planner, ok := p.planners[name]; ok {
		return planner
	}

	prt, ok := p.ExecutionContext().Registry.PlannerByName(name)

	if !ok {
		return nil
	}

	planner := bind.NewPlanner(prt, p.Graph)
	p.planners[name] = planner

	return planner
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
