// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package plan provides graph-construction actions for the plan namespace.
// Its methods execute during script evaluation to create nodes in the
// operation graph. The plan Provider is an executing receiver — not a planning
// receiver — because its methods run immediately to build the graph.
package plan

import (
	"fmt"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
)

var _ op.ContextProvider = (*Provider)(nil) // Interface Guard

// Provider creates graph nodes for plan-time graph construction.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a plan Provider bound to the given context.
func NewProvider(ctx op.Context) *Provider {
	return &Provider{
		ProviderBase: op.NewProviderBase(ctx),
	}
}

// region EXPORTED METHODS

// region Behaviors

// Complete creates a terminal node representing healthy conclusion.
//
// +devlore:defaults output=None
//
// Parameters:
//   - output: optional output value (immediate or Promise).
//
// Returns:
//   - *bind.Promise: promise for the terminal node.
//   - error: any error from slot filling.
func (p *Provider) Complete(output any) (*bind.Promise, error) {
	ctx := p.Context()
	graph := ctx.Graph
	node := &op.Node{
		ID:      op.GenerateNodeID("complete"),
		Action:  p.actionRegistry().MustGet("flow.complete"),
		Project: p.project(),
	}

	p.fillSlot(node, "output", output)

	graph.Nodes = append(graph.Nodes, node)
	return bind.NewPromise(node, graph, ""), nil
}

// Degraded creates a terminal node representing degraded conclusion.
//
// Parameters:
//   - format: format string (immediate or Promise).
//   - args: positional format arguments (each may be immediate or Promise).
//   - kwargs: keyword arguments for template rendering (each may be immediate or Promise).
//
// Returns:
//   - *bind.Promise: promise for the terminal node.
//   - error: any error from slot filling.
func (p *Provider) Degraded(format any, args []any, kwargs map[string]any) (*bind.Promise, error) {
	ctx := p.Context()
	graph := ctx.Graph
	node := &op.Node{
		ID:      op.GenerateNodeID("degraded"),
		Action:  p.actionRegistry().MustGet("flow.degraded"),
		Project: p.project(),
	}

	p.fillSlot(node, "format", format)
	p.fillListSlot(node, "args", args)
	p.fillDictSlot(node, "kwargs", kwargs)

	graph.Nodes = append(graph.Nodes, node)
	return bind.NewPromise(node, graph, ""), nil
}

// Fatal creates a terminal node representing fatal conclusion.
//
// Parameters:
//   - format: format string (immediate or Promise).
//   - args: positional format arguments (each may be immediate or Promise).
//   - kwargs: keyword arguments for template rendering (each may be immediate or Promise).
//
// Returns:
//   - *bind.Promise: promise for the terminal node.
//   - error: any error from slot filling.
func (p *Provider) Fatal(format any, args []any, kwargs map[string]any) (*bind.Promise, error) {
	ctx := p.Context()
	graph := ctx.Graph
	node := &op.Node{
		ID:      op.GenerateNodeID("fatal"),
		Action:  p.actionRegistry().MustGet("flow.fatal"),
		Project: p.project(),
	}

	p.fillSlot(node, "format", format)
	p.fillListSlot(node, "args", args)
	p.fillDictSlot(node, "kwargs", kwargs)

	graph.Nodes = append(graph.Nodes, node)
	return bind.NewPromise(node, graph, ""), nil
}

// WaitUntil creates a synchronization node that polls a predicate.
//
// +devlore:defaults interval=0
//
// Parameters:
//   - target: the value to evaluate (typically a Promise).
//   - predicate: callable that takes the target and returns bool.
//   - timeout: maximum wait time.
//   - interval: poll interval (default 5s). Optional.
//
// Returns:
//   - *bind.Promise: promise for the wait node output.
//   - error: any error from slot filling.
func (p *Provider) WaitUntil(target, predicate any, timeout, interval time.Duration) (*bind.Promise, error) {
	ctx := p.Context()
	graph := ctx.Graph
	node := &op.Node{
		ID:      op.GenerateNodeID("wait-until"),
		Action:  p.actionRegistry().MustGet("flow.wait_until"),
		Project: p.project(),
	}

	p.fillSlot(node, "target", target)
	p.fillSlot(node, "predicate", predicate)
	p.fillSlot(node, "timeout", timeout)
	if interval > 0 {
		p.fillSlot(node, "interval", interval)
	}

	graph.Nodes = append(graph.Nodes, node)
	return bind.NewPromise(node, graph, ""), nil
}

// Gather collects promises for fan-in parallel execution.
//
// Parameters:
//   - promises: two or more Promise values to gather.
//
// Returns:
//   - []*bind.Promise: the gathered promises.
//   - error: if fewer than 2 promises provided.
func (p *Provider) Gather(promises ...*bind.Promise) ([]*bind.Promise, error) {
	if len(promises) < 2 {
		return nil, fmt.Errorf("gather: expected at least 2 arguments, got %d", len(promises))
	}

	return promises, nil
}

// Choose creates a conditional branch in the execution graph.
//
// Parameters:
//   - when: Promise from a predicate action (bool-returning).
//   - then: callable that builds graph nodes for the true branch.
//
// Returns:
//   - *bind.Promise: promise for the choose node.
//   - error: any error during branch construction.
func (p *Provider) Choose(when *bind.Promise, then func() error) (*bind.Promise, error) {
	ctx := p.Context()
	graph := ctx.Graph

	// Snapshot current graph state to track nodes added by the callback.
	nodesBefore := len(graph.Nodes)

	// Execute the callback to build sub-graph nodes.
	if err := then(); err != nil {
		return nil, fmt.Errorf("choose: then callback: %w", err)
	}

	// Collect nodes added by the callback into a branch phase.
	branchPhaseID := op.GenerateNodeID("choose-branch")
	branchPhase := &op.Phase{
		ID:     branchPhaseID,
		Name:   "choose-branch",
		Status: op.PhasePending,
		Branch: true,
	}
	for i := nodesBefore; i < len(graph.Nodes); i++ {
		branchPhase.NodeIDs = append(branchPhase.NodeIDs, graph.Nodes[i].ID)
	}
	graph.Phases = append(graph.Phases, branchPhase)

	// Create the choose node.
	chooseNode := &op.Node{
		ID:      op.GenerateNodeID("choose"),
		Action:  p.actionRegistry().MustGet("flow.choose"),
		Project: p.project(),
	}

	// Wire predicate output → choose "when" slot (creates edge).
	when.FillSlot(chooseNode, "when")
	chooseNode.SetSlotImmediate("then", branchPhaseID)

	graph.Nodes = append(graph.Nodes, chooseNode)
	return bind.NewPromise(chooseNode, graph, ""), nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// actionRegistry returns the ActionRegistry from the context.
func (p *Provider) actionRegistry() *op.ActionRegistry {
	return p.Context().Data["action_registry"].(*op.ActionRegistry)
}

// project returns the project name from the context.
func (p *Provider) project() string {
	return p.Context().Data["project"].(string)
}

// ResolveAttr implements op.AttributeResolver. It routes sub-namespace lookups
// (e.g., plan.file, plan.git) to the corresponding PlanningReceiverFactory.
func (p *Provider) ResolveAttr(name string) any {
	ctx := p.Context()
	for _, r := range op.Receivers() {
		if r.ReceiverName() != name {
			continue
		}
		if pf, ok := r.(op.PlanningReceiverFactory); ok {
			return pf.NewPlanning(
				ctx.Graph,
				p.project(),
				p.actionRegistry(),
			)
		}
	}
	return nil
}

// fillSlot fills a slot on a node from a Go value.
// Handles Promise, nil, and immediate values.
func (p *Provider) fillSlot(node *op.Node, slotName string, value any) {
	if value == nil {
		return
	}
	if promise, ok := value.(*bind.Promise); ok {
		promise.FillSlot(node, slotName)
		return
	}
	node.SetSlotImmediate(slotName, value)
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
	node.SetSlotImmediate(slotName+".len", len(values))
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
