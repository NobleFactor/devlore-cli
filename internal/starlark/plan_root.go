// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"sort"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// PlanRoot implements the top-level plan namespace using the slot-based model.
// Sub-namespaces are populated from PlannedProvider implementations selected by
// BindingSet. Flow actions (choose, source, gather) are built-in.
type PlanRoot struct {
	graph   *op.Graph
	project string
	reg     *op.ActionRegistry

	// Sub-namespaces built from announced PlannedProvider implementations.
	plans map[string]starlark.Value
}

// NewPlanRootFromProviders creates a PlanRoot from announced PlannedProvider
// implementations. Consumers select providers via BindingSet, which passes the
// filtered provider map here.
func NewPlanRootFromProviders(graph *op.Graph, project string, reg *op.ActionRegistry, providers map[string]op.PlannedProvider) *PlanRoot {
	plans := make(map[string]starlark.Value, len(providers))
	for name, p := range providers {
		plans[name] = p.NewPlanned(graph, project, reg)
	}
	return &PlanRoot{
		graph:   graph,
		project: project,
		reg:     reg,
		plans:   plans,
	}
}

// String implements starlark.Value.
func (p *PlanRoot) String() string { return "plan" }

// Type implements starlark.Value.
func (p *PlanRoot) Type() string { return "plan" }

// Freeze implements starlark.Value.
func (p *PlanRoot) Freeze() {}

// Truth implements starlark.Value.
func (p *PlanRoot) Truth() starlark.Bool { return true }

// Hash implements starlark.Value.
func (p *PlanRoot) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: plan") }

// Attr implements starlark.HasAttrs.
func (p *PlanRoot) Attr(name string) (starlark.Value, error) {
	// Dynamic sub-namespaces from registry
	if plan, ok := p.plans[name]; ok {
		return plan, nil
	}

	// Top-level bindings (graph construction primitives)
	switch name {
	case "choose":
		return starlark.NewBuiltin("plan.choose", p.choose), nil
	case "source":
		return starlark.NewBuiltin("plan.source", p.source), nil
	case "gather":
		return starlark.NewBuiltin("plan.gather", p.gather), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("plan has no attribute %q", name))
	}
}

// AttrNames implements starlark.HasAttrs.
func (p *PlanRoot) AttrNames() []string {
	names := make([]string, 0, len(p.plans)+3)
	for name := range p.plans {
		names = append(names, name)
	}
	names = append(names, "choose", "source", "gather")
	sort.Strings(names)
	return names
}

// choose creates a conditional branch in the execution graph.
// Usage: plan.choose(when=predicate_output, then=callback)
//
// The "when" argument is the output of a predicate action (a bool-returning
// provider method like plan.file.exists(path)). The predicate node runs first,
// producing a boolean that flows into Choose's "when" slot via an edge.
//
// Arguments:
//   - when: An Output from a predicate action (e.g., plan.file.exists(path))
//   - then: A callable that builds graph nodes for the true branch
//
// Returns: Output of the choose node
func (p *PlanRoot) choose(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var when starlark.Value
	var then starlark.Callable

	if err := starlark.UnpackArgs("choose", args, kwargs,
		"when", &when,
		"then", &then,
	); err != nil {
		return nil, err
	}

	// The "when" must be an Output — the promise of a boolean from a predicate node.
	predOutput, ok := when.(*op.Output)
	if !ok {
		return nil, fmt.Errorf("choose: when must be a predicate output (e.g., plan.file.exists(...)), got %s", when.Type())
	}

	// Snapshot current graph state to track nodes added by the callback.
	nodesBefore := len(p.graph.Nodes)

	// Execute the callback to build sub-graph nodes.
	_, err := starlark.Call(thread, then, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("choose: then callback: %w", err)
	}

	// Collect nodes added by the callback into a branch phase.
	branchPhaseID := op.GenerateNodeID("choose-branch")
	branchPhase := &op.Phase{
		ID:     branchPhaseID,
		Name:   "choose-branch",
		Status: op.PhasePending,
	}
	for i := nodesBefore; i < len(p.graph.Nodes); i++ {
		branchPhase.NodeIDs = append(branchPhase.NodeIDs, p.graph.Nodes[i].ID)
	}
	p.graph.Phases = append(p.graph.Phases, branchPhase)

	// Create the choose node. The "when" slot is a promise that resolves
	// to the predicate node's boolean result at execution time.
	chooseNode := &op.Node{
		ID:      op.GenerateNodeID("choose"),
		Action:  p.reg.MustGet("flow.choose"),
		Project: p.project,
	}

	// Wire predicate output → choose "when" slot via FillSlot (creates edge).
	if err := op.FillSlot(chooseNode, p.graph, "when", predOutput); err != nil {
		return nil, fmt.Errorf("choose: when: %w", err)
	}
	chooseNode.SetSlotImmediate("then", branchPhaseID)

	p.graph.Nodes = append(p.graph.Nodes, chooseNode)
	return op.NewOutput(chooseNode, p.graph, ""), nil
}

// source creates a source file artifact.
// Usage: plan.source(path)
//
// Slots:
//   - path: Path to existing source file (immediate only)
//
// Returns: Promise of the source file
func (p *PlanRoot) source(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path starlark.Value
	if err := starlark.UnpackArgs("source", args, kwargs, "path", &path); err != nil {
		return nil, err
	}

	node := &op.Node{
		ID:      op.GenerateNodeID("source"),
		Action:  p.reg.MustGet("file.read"),
		Project: p.project,
	}

	if err := op.FillSlot(node, p.graph, "path", path); err != nil {
		return nil, fmt.Errorf("source: path: %w", err)
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return op.NewOutput(node, p.graph, ""), nil
}

// gather creates a handle for parallel execution of multiple nodes.
// Usage: plan.gather(promise1, promise2, ...)
//
// This collects multiple promises into a single handle. When the handle is
// passed to another operation, it creates edges from ALL gathered nodes to
// the consumer, enabling parallel execution.
//
// Arguments:
//   - promises: Two or more Output values to gather
//
// Returns: Gather handle that can be passed to other operations
func (p *PlanRoot) gather(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("gather: expected at least 2 arguments, got %d", len(args))
	}

	outputs := make([]*op.Output, 0, len(args))
	for i, arg := range args {
		output, ok := arg.(*op.Output)
		if !ok {
			return nil, fmt.Errorf("gather: argument %d must be an Output, got %s", i+1, arg.Type())
		}
		outputs = append(outputs, output)
	}

	return op.NewGather(p.graph, outputs...), nil
}
