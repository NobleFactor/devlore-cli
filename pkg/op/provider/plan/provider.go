// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package plan provides graph-construction actions for the plan namespace.
// Its methods execute during script evaluation to create nodes in the
// operation graph. The plan Provider is an executing receiver — not a planning
// receiver — because its methods run immediately to build the graph.
package plan

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var _ op.ContextProvider = (*Provider)(nil) // Interface Guard

// Provider creates graph nodes for plan-time graph construction.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase

	graph   *op.Graph
	project string
	reg     *op.ActionRegistry
}

// NewProvider creates a plan Provider bound to the given graph.
func NewProvider(graph *op.Graph, project string, reg *op.ActionRegistry) *Provider {
	return &Provider{
		graph:   graph,
		project: project,
		reg:     reg,
	}
}

// region EXPORTED METHODS

// region Behaviors

// Complete creates a terminal node representing healthy conclusion.
//
// Parameters:
//   - output: optional output value (immediate or Promise).
//
// Returns:
//   - *op.Promise: promise for the terminal node.
//   - error: any error from slot filling.
func (p *Provider) Complete(output any) (*op.Promise, error) {
	node := &op.Node{
		ID:      op.GenerateNodeID("complete"),
		Action:  p.reg.MustGet("flow.complete"),
		Project: p.project,
	}

	p.fillSlot(node, "output", output)

	p.graph.Nodes = append(p.graph.Nodes, node)
	return op.NewPromise(node, p.graph, ""), nil
}

// Degraded creates a terminal node representing degraded conclusion.
//
// Parameters:
//   - format: format string (immediate or Promise).
//   - args: positional format arguments (each may be immediate or Promise).
//
// Returns:
//   - *op.Promise: promise for the terminal node.
//   - error: any error from slot filling.
func (p *Provider) Degraded(format any, args ...any) (*op.Promise, error) {
	node := &op.Node{
		ID:      op.GenerateNodeID("degraded"),
		Action:  p.reg.MustGet("flow.degraded"),
		Project: p.project,
	}

	p.fillSlot(node, "format", format)
	p.fillListSlot(node, "args", args)

	p.graph.Nodes = append(p.graph.Nodes, node)
	return op.NewPromise(node, p.graph, ""), nil
}

// Fatal creates a terminal node representing fatal conclusion.
//
// Parameters:
//   - format: format string (immediate or Promise).
//   - args: positional format arguments (each may be immediate or Promise).
//
// Returns:
//   - *op.Promise: promise for the terminal node.
//   - error: any error from slot filling.
func (p *Provider) Fatal(format any, args ...any) (*op.Promise, error) {
	node := &op.Node{
		ID:      op.GenerateNodeID("fatal"),
		Action:  p.reg.MustGet("flow.fatal"),
		Project: p.project,
	}

	p.fillSlot(node, "format", format)
	p.fillListSlot(node, "args", args)

	p.graph.Nodes = append(p.graph.Nodes, node)
	return op.NewPromise(node, p.graph, ""), nil
}

// WaitUntil creates a synchronization node that polls a predicate.
//
// Parameters:
//   - target: the value to evaluate (typically a Promise).
//   - predicate: callable that takes the target and returns bool.
//   - timeout: maximum wait time (Go duration string, e.g. "5m").
//   - interval: poll interval (Go duration string, default "5s"). Optional.
//
// Returns:
//   - *op.Promise: promise for the wait node output.
//   - error: any error from slot filling.
func (p *Provider) WaitUntil(target, predicate any, timeout, interval string) (*op.Promise, error) {

	node := &op.Node{
		ID:      op.GenerateNodeID("wait-until"),
		Action:  p.reg.MustGet("flow.wait_until"),
		Project: p.project,
	}

	p.fillSlot(node, "target", target)
	p.fillSlot(node, "predicate", predicate)
	p.fillSlot(node, "timeout", timeout)
	if interval != "" {
		p.fillSlot(node, "interval", interval)
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return op.NewPromise(node, p.graph, ""), nil
}

// Gather collects promises for fan-in parallel execution.
//
// Parameters:
//   - promises: two or more Promise values to gather.
//
// Returns:
//   - []*op.Promise: the gathered promises.
//   - error: if fewer than 2 promises provided.
func (p *Provider) Gather(promises ...*op.Promise) ([]*op.Promise, error) {
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
//   - *op.Promise: promise for the choose node.
//   - error: any error during branch construction.
func (p *Provider) Choose(when *op.Promise, then func() error) (*op.Promise, error) {
	// Snapshot current graph state to track nodes added by the callback.
	nodesBefore := len(p.graph.Nodes)

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
	for i := nodesBefore; i < len(p.graph.Nodes); i++ {
		branchPhase.NodeIDs = append(branchPhase.NodeIDs, p.graph.Nodes[i].ID)
	}
	p.graph.Phases = append(p.graph.Phases, branchPhase)

	// Create the choose node.
	chooseNode := &op.Node{
		ID:      op.GenerateNodeID("choose"),
		Action:  p.reg.MustGet("flow.choose"),
		Project: p.project,
	}

	// Wire predicate output → choose "when" slot (creates edge).
	when.FillSlot(chooseNode, "when")
	chooseNode.SetSlotImmediate("then", branchPhaseID)

	p.graph.Nodes = append(p.graph.Nodes, chooseNode)
	return op.NewPromise(chooseNode, p.graph, ""), nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// fillSlot fills a slot on a node from a Go value.
// Handles Promise, nil, and immediate values.
func (p *Provider) fillSlot(node *op.Node, slotName string, value any) {
	if value == nil {
		return
	}
	if promise, ok := value.(*op.Promise); ok {
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

// endregion

// endregion
