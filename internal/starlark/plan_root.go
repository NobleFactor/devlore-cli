// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"
	"sort"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/host"
	"github.com/NobleFactor/devlore-cli/pkg/projection"
)

// PlanRoot implements the top-level plan namespace using the slot-based model.
// Sub-namespaces are dynamically populated from the plan registry (each
// plan_*_gen.go registers via init()).
type PlanRoot struct {
	graph   *projection.Graph
	host    host.Host
	project string
	reg     *execution.ActionRegistry

	// Sub-namespaces built from planRegistry.
	plans map[string]starlark.Value
}

// NewPlanRoot creates a new PlanRoot for the given graph and host.
// Sub-namespaces are built dynamically from the plan registry.
func NewPlanRoot(graph *projection.Graph, h host.Host, project string, reg *execution.ActionRegistry) *PlanRoot {
	plans := make(map[string]starlark.Value, len(planRegistry))
	for name, factory := range planRegistry {
		plans[name] = factory(graph, h, project, reg)
	}
	return &PlanRoot{
		graph:   graph,
		host:    h,
		project: project,
		reg:     reg,
		plans:   plans,
	}
}

// Starlark Value interface
func (p *PlanRoot) String() string        { return "plan" }
func (p *PlanRoot) Type() string          { return "plan" }
func (p *PlanRoot) Freeze()               {}
func (p *PlanRoot) Truth() starlark.Bool  { return true }
func (p *PlanRoot) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: plan") }

// Starlark HasAttrs interface
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
	predOutput, ok := when.(*projection.Output)
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
	branchPhaseID := projection.GenerateNodeID("choose-branch")
	branchPhase := &projection.Phase{
		ID:     branchPhaseID,
		Name:   "choose-branch",
		Status: projection.PhasePending,
	}
	for i := nodesBefore; i < len(p.graph.Nodes); i++ {
		branchPhase.NodeIDs = append(branchPhase.NodeIDs, p.graph.Nodes[i].ID)
	}
	p.graph.Phases = append(p.graph.Phases, branchPhase)

	// Create the choose node. The "when" slot is a promise that resolves
	// to the predicate node's boolean result at execution time.
	chooseNode := &projection.Node{
		ID:      projection.GenerateNodeID("choose"),
		Action:  p.reg.MustGet("flow.choose"),
		Project: p.project,
	}

	// Wire predicate output → choose "when" slot via FillSlot (creates edge).
	if err := projection.FillSlot(chooseNode, p.graph, "when", predOutput); err != nil {
		return nil, fmt.Errorf("choose: when: %w", err)
	}
	chooseNode.SetSlotImmediate("then", branchPhaseID)

	p.graph.Nodes = append(p.graph.Nodes, chooseNode)
	return projection.NewOutput(chooseNode, p.graph, ""), nil
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

	node := &projection.Node{
		ID:      projection.GenerateNodeID("source"),
		Action:  p.reg.MustGet("file.source"),
		Project: p.project,
	}

	if err := projection.FillSlot(node, p.graph, "path", path); err != nil {
		return nil, fmt.Errorf("source: path: %w", err)
	}

	p.graph.Nodes = append(p.graph.Nodes, node)
	return projection.NewOutput(node, p.graph, ""), nil
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

	outputs := make([]*projection.Output, 0, len(args))
	for i, arg := range args {
		output, ok := arg.(*projection.Output)
		if !ok {
			return nil, fmt.Errorf("gather: argument %d must be an Output, got %s", i+1, arg.Type())
		}
		outputs = append(outputs, output)
	}

	return projection.NewGather(p.graph, outputs...), nil
}
