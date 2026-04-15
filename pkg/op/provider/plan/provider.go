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
	"time"

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
	mu       sync.Mutex               // guards planners
	planners map[string]*bind.Planner // cached planners by receiver name
}

// NewProvider creates a plan Provider bound to the given context.
//
// The graph is obtained from ctx.Data["graph"]. If no graph is provided, a new one is created.
func NewProvider(ctx *op.ExecutionContext, value any) *Provider {

	graph, _ := ctx.Data["graph"].(*op.Graph)

	if graph == nil {
		graph = op.NewGraph(ctx)
	}

	return &Provider{
		ProviderBase: op.NewProviderBase(ctx),
		Graph:        graph,
		planners:     make(map[string]*bind.Planner),
	}
}

// region EXPORTED METHODS

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

	// Snapshot current children to capture nodes added by the callback.

	childrenBefore := len(p.Graph.Children)

	// Execute the callback to build sub-graph nodes.

	if err := then(); err != nil {
		return nil, fmt.Errorf("choose: then callback: %w", err)
	}

	// Move newly added children into a branch subgraph.

	branchID := op.GenerateNodeID("choose-branch")

	newChildren := p.Graph.Children[childrenBefore:]

	branchSG := &op.Subgraph{
		ID:       branchID,
		Name:     "choose-branch",
		Children: append([]op.SubgraphChild{}, newChildren...),
		Status:   op.SubgraphPending,
		Branch:   true,
	}

	p.Graph.Children = p.Graph.Children[:childrenBefore]

	// Move edges whose endpoints are both in this branch subgraph.
	childIDs := make(map[string]bool, len(newChildren))
	for _, c := range newChildren {
		childIDs[c.ChildID()] = true
	}

	var kept []op.Edge
	for _, e := range p.Graph.Edges {
		if childIDs[e.From] && childIDs[e.To] {
			branchSG.Edges = append(branchSG.Edges, e)
		} else {
			kept = append(kept, e)
		}
	}
	p.Graph.Edges = kept

	p.Graph.AddSubgraph(branchSG)

	// Create the choose node.

	chooseNode := &op.Node{
		ID:       op.GenerateNodeID("choose"),
		Receiver: "flow.choose",
	}

	// Wire predicate output → choose "when" slot (creates edge).

	when.FillSlot(chooseNode, "when")
	chooseNode.SetSlotImmediate("then", branchID)

	p.Graph.AddNode(chooseNode)

	return bind.NewPromise(p.Graph, chooseNode, ""), nil
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
// Is
// Returns:
//   - *bind.Promise: promise for the wait node output.
//   - error: any error from slot filling.
func (p *Provider) WaitUntil(target, predicate any, timeout, interval time.Duration) (*bind.Promise, error) {

	node := &op.Node{
		ID:       op.GenerateNodeID("wait-until"),
		Receiver: "flow.wait_until",
	}

	p.fillSlot(node, "target", target)
	p.fillSlot(node, "predicate", predicate)
	p.fillSlot(node, "timeout", timeout)

	if interval > 0 {
		p.fillSlot(node, "interval", interval)
	}

	p.Graph.AddNode(node)
	return bind.NewPromise(p.Graph, node, ""), nil
}

// ResolveAttr implements op.AttributeResolver.
//
// It routes sub-namespace lookups (e.g., plan.file, plan.git) to the corresponding [bind.Planner]. Planners are
// constructed once per receiver name and cached for the lifetime of this provider.
func (p *Provider) ResolveAttr(name string) any {

	p.mu.Lock()
	defer p.mu.Unlock()

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
