// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Plan implements the plan.flow namespace for Starlark scripts. Handwritten — flow actions have custom signatures that
// don't fit the reflection-based WrapProviderInPlanningReceiver model.
type Plan struct {
	graph   *op.Graph
	project string
	reg     *op.ActionRegistry
}

// NewFlowPlan creates a [Plan] bound to the given graph.
//
// Parameters:
//   - graph: the operation graph to populate
//   - project: the project identifier
//   - reg: the action registry for node creation
//
// Returns:
//   - *Plan: the plan.flow namespace
func NewFlowPlan(graph *op.Graph, project string, reg *op.ActionRegistry) *Plan {

	return &Plan{graph: graph, project: project, reg: reg}
}

// String implements [starlark.Value].
func (f *Plan) String() string { return "flow" }

// Type implements [starlark.Value].
func (f *Plan) Type() string { return "flow" }

// Freeze implements [starlark.Value].
func (f *Plan) Freeze() {}

// Truth implements [starlark.Value].
func (f *Plan) Truth() starlark.Bool { return true }

// Hash implements [starlark.Value].
func (f *Plan) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: flow") }

// Attr implements [starlark.HasAttrs].
//
// Parameters:
//   - name: the attribute name to look up
//
// Returns:
//   - starlark.Value: the builtin function for the attribute
//   - error: if the attribute does not exist
func (f *Plan) Attr(name string) (starlark.Value, error) {

	switch name {
	case "complete":
		return starlark.NewBuiltin("flow.complete", f.complete), nil
	case "degraded":
		return starlark.NewBuiltin("flow.degraded", f.degraded), nil
	case "fatal":
		return starlark.NewBuiltin("flow.fatal", f.fatal), nil
	default:
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("flow has no attribute %q", name))
	}
}

// AttrNames implements [starlark.HasAttrs].
//
// Returns:
//   - []string: the available attribute names
func (f *Plan) AttrNames() []string {

	return []string{"complete", "degraded", "fatal"}
}

// complete creates a flow.complete terminal node in the graph.
//
// Usage: plan.flow.complete() or plan.flow.complete(output=value)
//
// Parameters:
//   - args: positional arguments (unused)
//   - kwargs: optional "output" keyword argument
//
// Returns:
//   - starlark.Value: an [op.Output] promise for the terminal node
//   - error: any error from slot filling
func (f *Plan) complete(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var output starlark.Value = starlark.None

	if err := starlark.UnpackArgs("complete", args, kwargs, "output?", &output); err != nil {
		return nil, err
	}

	node := &op.Node{
		ID:      op.GenerateNodeID("complete"),
		Action:  f.reg.MustGet("flow.complete"),
		Project: f.project,
	}

	if err := op.FillSlot(node, f.graph, "output", output); err != nil {
		return nil, fmt.Errorf("complete: output: %w", err)
	}

	f.graph.Nodes = append(f.graph.Nodes, node)
	return op.NewOutput(node, f.graph, ""), nil
}

// degraded creates a flow.degraded terminal node in the graph.
//
// Usage: plan.flow.degraded(format, *args, **kwargs)
//
// First positional arg is the format string. Remaining positional args are packed into the "args" list slot. Keyword
// args are packed into the "kwargs" dict slot. Promise values in any position create edges.
//
// Parameters:
//   - args: format string (required) followed by optional positional arguments
//   - kwargs: optional keyword arguments for string formatting
//
// Returns:
//   - starlark.Value: an [op.Output] promise for the terminal node
//   - error: any error from slot filling
func (f *Plan) degraded(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("degraded: missing required argument: format")
	}

	node := &op.Node{
		ID:      op.GenerateNodeID("degraded"),
		Action:  f.reg.MustGet("flow.degraded"),
		Project: f.project,
	}

	// First positional arg is the format string.
	if err := op.FillSlot(node, f.graph, "format", args[0]); err != nil {
		return nil, fmt.Errorf("degraded: format: %w", err)
	}

	// Remaining positional args → "args" list slot.
	if err := fillListSlot(node, f.graph, "args", args[1:]); err != nil {
		return nil, fmt.Errorf("degraded: args: %w", err)
	}

	// Keyword args → "kwargs" dict slot.
	if err := fillDictSlot(node, f.graph, "kwargs", kwargs); err != nil {
		return nil, fmt.Errorf("degraded: kwargs: %w", err)
	}

	f.graph.Nodes = append(f.graph.Nodes, node)
	return op.NewOutput(node, f.graph, ""), nil
}

// fatal creates a flow.fatal terminal node in the graph.
//
// Usage: plan.flow.fatal(format, *args, **kwargs)
//
// Same signature as degraded. The action returns FatalError which halts graph execution.
//
// Parameters:
//   - args: format string (required) followed by optional positional arguments
//   - kwargs: optional keyword arguments for string formatting
//
// Returns:
//   - starlark.Value: an [op.Output] promise for the terminal node
//   - error: any error from slot filling
func (f *Plan) fatal(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fatal: missing required argument: format")
	}

	node := &op.Node{
		ID:      op.GenerateNodeID("fatal"),
		Action:  f.reg.MustGet("flow.fatal"),
		Project: f.project,
	}

	if err := op.FillSlot(node, f.graph, "format", args[0]); err != nil {
		return nil, fmt.Errorf("fatal: format: %w", err)
	}

	if err := fillListSlot(node, f.graph, "args", args[1:]); err != nil {
		return nil, fmt.Errorf("fatal: args: %w", err)
	}

	if err := fillDictSlot(node, f.graph, "kwargs", kwargs); err != nil {
		return nil, fmt.Errorf("fatal: kwargs: %w", err)
	}

	f.graph.Nodes = append(f.graph.Nodes, node)
	return op.NewOutput(node, f.graph, ""), nil
}

// fillListSlot packs Starlark values into indexed sub-slots on a node. Promise values create edges; immediates are
// stored directly.
//
// Parameters:
//   - node: the graph node to populate
//   - graph: the operation graph for edge creation
//   - slotName: base name for the indexed sub-slots (e.g., "args" → "args[0]", "args[1]")
//   - values: the Starlark values to pack
//
// Returns:
//   - error: any error from slot filling
func fillListSlot(node *op.Node, graph *op.Graph, slotName string, values starlark.Tuple) error {
	if len(values) == 0 {
		return nil
	}
	for i, v := range values {
		subSlot := fmt.Sprintf("%s[%d]", slotName, i)
		if err := op.FillSlot(node, graph, subSlot, v); err != nil {
			return err
		}
	}
	node.SetSlotImmediate(slotName+".len", len(values))
	return nil
}

// fillDictSlot packs Starlark keyword tuples into keyed sub-slots on a node. Promise values create edges; immediates
// are stored directly.
//
// Parameters:
//   - node: the graph node to populate
//   - graph: the operation graph for edge creation
//   - slotName: base name for the keyed sub-slots (e.g., "kwargs" → "kwargs.key")
//   - kwargs: the Starlark keyword tuples to pack
//
// Returns:
//   - error: any error from slot filling
func fillDictSlot(node *op.Node, graph *op.Graph, slotName string, kwargs []starlark.Tuple) error {
	if len(kwargs) == 0 {
		return nil
	}
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		subSlot := fmt.Sprintf("%s.%s", slotName, key)
		if err := op.FillSlot(node, graph, subSlot, kv[1]); err != nil {
			return err
		}
	}
	return nil
}
