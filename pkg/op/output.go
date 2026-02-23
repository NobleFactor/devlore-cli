// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"

	"go.starlark.net/starlark"
)

// Output represents a promise - a handle to a node's output that can flow
// through the graph to fill slots in other nodes.
//
// When passed to a plan function's slot, it creates an edge in the graph.
// The same promise can flow to multiple slots (fan-out).
type Output struct {
	// node is the action that produces this output
	node *Node

	// graph is the execution graph (for creating edges)
	graph *Graph

	// slot identifies which output of the node this represents (empty = default)
	slot string
}

// NewOutput creates a new Output (promise) representing a node's output.
func NewOutput(node *Node, graph *Graph, slot string) *Output {
	return &Output{
		node:  node,
		graph: graph,
		slot:  slot,
	}
}

// String implements starlark.Value.
func (o *Output) String() string { return fmt.Sprintf("Output(%s)", o.node.ID) }

// Type implements starlark.Value.
func (o *Output) Type() string { return "Output" }

// Freeze implements starlark.Value.
func (o *Output) Freeze() {}

// Truth implements starlark.Value.
func (o *Output) Truth() starlark.Bool { return true }

// Hash implements starlark.Value.
func (o *Output) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: Output") }

// Attr implements starlark.HasAttrs.
func (o *Output) Attr(name string) (starlark.Value, error) {
	switch name {
	case "node_id":
		return starlark.String(o.node.ID), nil
	case "slot":
		return starlark.String(o.slot), nil
	case "retry":
		return starlark.NewBuiltin("output.retry", o.retryBuiltin), nil
	default:
		// Get the value from the node's slots and convert to Starlark
		slotVal := o.node.GetSlot(name)
		if slotVal == nil {
			return nil, starlark.NoSuchAttrError(fmt.Sprintf("Output has no attribute %q", name))
		}
		sv, err := GoToStarlarkValue(slotVal)
		if err != nil {
			return nil, fmt.Errorf("slot %q: %w", name, err)
		}
		return sv, nil
	}
}

// AttrNames implements starlark.HasAttrs.
func (o *Output) AttrNames() []string {
	names := []string{"node_id", "retry", "slot"}
	// Add slot names from the node
	if o.node.Slots != nil {
		for name := range o.node.Slots {
			names = append(names, name)
		}
	}
	return names
}

// Node returns the node that produces this output.
func (o *Output) Node() *Node {
	return o.node
}

// Graph returns the execution graph.
func (o *Output) Graph() *Graph {
	return o.graph
}

// Slot returns which output slot this represents.
func (o *Output) Slot() string {
	return o.slot
}

// FillSlot fills a slot in the consumer node with this promise, creating an edge.
// This is called when a promise is passed to a plan function.
func (o *Output) FillSlot(consumer *Node, slotName string) {
	// Set the slot to reference this output's node
	consumer.SetSlotPromise(slotName, o.node.ID, o.slot)

	// Create edge: producer must complete before consumer
	o.graph.Edges = append(o.graph.Edges, Edge{
		From: o.node.ID,
		To:   consumer.ID,
	})
}

// FillSlot fills a slot in a node from a Starlark value.
//
// Any slot accepts:
//   - A promise (Output): creates an edge, value flows at runtime
//   - A gather (Gather): creates edges from all members (parallel execution)
//   - An immediate value: stored directly, known at analysis time
func FillSlot(node *Node, graph *Graph, slotName string, value starlark.Value) error {
	// Promise: create edge, value flows at runtime
	if output, ok := value.(*Output); ok {
		output.FillSlot(node, slotName)
		return nil
	}

	// Gather: create edges from all members (parallel execution)
	if gather, ok := value.(*Gather); ok {
		gather.FillSlot(node, slotName)
		return nil
	}

	// Immediate: store directly
	if str, ok := starlark.AsString(value); ok {
		node.SetSlotImmediate(slotName, str)
		return nil
	}

	// List: store as native []string (homogeneous) or []any (mixed)
	if list, ok := value.(*starlark.List); ok {
		result, err := StarlarkListToSlice(list)
		if err != nil {
			return fmt.Errorf("slot %q: %w", slotName, err)
		}
		node.SetSlotImmediate(slotName, result)
		return nil
	}

	// Dict: store as native map[string]any
	if dict, ok := value.(*starlark.Dict); ok {
		result, err := StarlarkDictToMap(dict)
		if err != nil {
			return fmt.Errorf("slot %q: %w", slotName, err)
		}
		node.SetSlotImmediate(slotName, result)
		return nil
	}

	// Other immediate types — store as native Go types
	switch v := value.(type) {
	case starlark.Int:
		i, _ := v.Int64()
		node.SetSlotImmediate(slotName, int(i))
		return nil
	case starlark.Bool:
		node.SetSlotImmediate(slotName, bool(v))
		return nil
	case starlark.Float:
		node.SetSlotImmediate(slotName, float64(v))
		return nil
	case starlark.NoneType:
		return nil
	}

	return fmt.Errorf("unsupported value type %s for slot %q", value.Type(), slotName)
}

// Path returns a path from the node's slots.
func (o *Output) Path() string {
	path, ok := o.node.GetSlot("path").(string)
	if !ok {
		return ""
	}
	return path
}

// retryBuiltin sets the retry policy on this output's node.
// Usage: node = plan.net.download(...); node.retry(max_attempts=5, backoff="linear")
func (o *Output) retryBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var maxAttempts int
	var backoff, initialDelay, maxDelay string

	if err := starlark.UnpackArgs("retry", args, kwargs,
		"max_attempts", &maxAttempts,
		"backoff?", &backoff,
		"initial_delay?", &initialDelay,
		"max_delay?", &maxDelay,
	); err != nil {
		return nil, err
	}

	if maxAttempts < 0 {
		return nil, fmt.Errorf("retry: max_attempts must be non-negative, got %d", maxAttempts)
	}

	policy := &RetryPolicy{
		MaxAttempts: maxAttempts,
	}

	if backoff != "" {
		switch backoff {
		case "none":
			policy.Backoff = BackoffNone
		case "linear":
			policy.Backoff = BackoffLinear
		case "exponential":
			policy.Backoff = BackoffExponential
		default:
			return nil, fmt.Errorf("retry: unknown backoff %q (use none, linear, exponential)", backoff)
		}
	}

	if initialDelay != "" {
		policy.InitialDelay = initialDelay
	}
	if maxDelay != "" {
		policy.MaxDelay = maxDelay
	}

	o.node.Retry = policy
	return o, nil
}

// DependOn creates an edge making the given node depend on this output's node.
func (o *Output) DependOn(consumer *Node) {
	o.graph.Edges = append(o.graph.Edges, Edge{
		From: o.node.ID,
		To:   consumer.ID,
	})
}

// ResolveInput extracts an *Output from a Starlark value.
// Returns an error if the value is not an Output.
func ResolveInput(value starlark.Value) (*Output, error) {
	if output, ok := value.(*Output); ok {
		return output, nil
	}
	return nil, fmt.Errorf("expected Output, got %s", value.Type())
}

// Gather represents a collection of outputs that can run in parallel.
// When used as a slot input, it creates edges from ALL members to the consumer,
// enabling parallel execution of the gathered nodes.
//
// Usage in Starlark:
//
//	a = plan.file.copy(src1, dst1)
//	b = plan.file.copy(src2, dst2)
//	c = plan.file.copy(src3, dst3)
//	group = plan.gather(a, b, c)
//	d = plan.whatever(group)  # d waits for a, b, c (which run in parallel)
type Gather struct {
	outputs []*Output
	graph   *Graph
}

// NewGather creates a new Gather from multiple outputs.
func NewGather(graph *Graph, outputs ...*Output) *Gather {
	return &Gather{
		outputs: outputs,
		graph:   graph,
	}
}

// String implements starlark.Value.
func (g *Gather) String() string {
	ids := make([]string, len(g.outputs))
	for i, o := range g.outputs {
		ids[i] = o.node.ID
	}
	return fmt.Sprintf("Gather(%v)", ids)
}

// Type implements starlark.Value.
func (g *Gather) Type() string { return "Gather" }

// Freeze implements starlark.Value.
func (g *Gather) Freeze() {}

// Truth implements starlark.Value.
func (g *Gather) Truth() starlark.Bool { return len(g.outputs) > 0 }

// Hash implements starlark.Value.
func (g *Gather) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: Gather") }

// Outputs returns the gathered outputs.
func (g *Gather) Outputs() []*Output {
	return g.outputs
}

// FillSlot fills a slot in the consumer node with all gathered promises,
// creating edges from each member. This enables parallel execution.
func (g *Gather) FillSlot(consumer *Node, slotName string) {
	for i, output := range g.outputs {
		// Set slot reference for each output
		subSlot := fmt.Sprintf("%s[%d]", slotName, i)
		consumer.SetSlotPromise(subSlot, output.node.ID, output.slot)

		// Create edge: producer must complete before consumer
		g.graph.Edges = append(g.graph.Edges, Edge{
			From: output.node.ID,
			To:   consumer.ID,
		})
	}
	// Store count for runtime
	consumer.SetSlotImmediate(slotName+".len", len(g.outputs))
}
