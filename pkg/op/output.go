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
//
// Parameters:
//   - node: the producing node.
//   - graph: the execution graph.
//   - slot: which output slot this represents (empty for default).
//
// Returns:
//   - *Output: the new promise handle.
func NewOutput(node *Node, graph *Graph, slot string) *Output {

	return &Output{
		node:  node,
		graph: graph,
		slot:  slot,
	}
}

// region EXPORTED METHODS

// region State management

// Graph returns the execution graph.
//
// Returns:
//   - *Graph: the graph this output belongs to.
func (o *Output) Graph() *Graph {

	return o.graph
}

// Node returns the node that produces this output.
//
// Returns:
//   - *Node: the producing node.
func (o *Output) Node() *Node {

	return o.node
}

// Path returns a path from the node's slots.
//
// Returns:
//   - string: the path slot value, or empty string if not present or not a string.
func (o *Output) Path() string {

	path, ok := o.node.GetSlot("path").(string)
	if !ok {
		return ""
	}
	return path
}

// Slot returns which output slot this represents.
//
// Returns:
//   - string: the slot identifier.
func (o *Output) Slot() string {

	return o.slot
}

// endregion

// region Behaviors

// Actions

// Attr implements starlark.HasAttrs.
//
// Parameters:
//   - name: the attribute name to look up.
//
// Returns:
//   - starlark.Value: the attribute value.
//   - error: non-nil if the attribute does not exist.
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
		sv, err := Marshal(slotVal)
		if err != nil {
			return nil, fmt.Errorf("slot %q: %w", name, err)
		}
		return sv, nil
	}
}

// AttrNames implements starlark.HasAttrs.
//
// Returns:
//   - []string: all available attribute names.
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

// DependOn creates an edge making the given node depend on this output's node.
//
// Parameters:
//   - consumer: the node that should depend on this output's producer.
func (o *Output) DependOn(consumer *Node) {

	o.graph.Edges = append(o.graph.Edges, Edge{
		From: o.node.ID,
		To:   consumer.ID,
	})
}

// FillSlot fills a slot in the consumer node with this promise, creating an edge.
// This is called when a promise is passed to a plan function.
//
// Parameters:
//   - consumer: the node receiving the promise.
//   - slotName: the slot to fill.
func (o *Output) FillSlot(consumer *Node, slotName string) {

	// Set the slot to reference this output's node
	consumer.SetSlotPromise(slotName, o.node.ID, o.slot)

	// Create edge: producer must complete before consumer
	o.graph.Edges = append(o.graph.Edges, Edge{
		From: o.node.ID,
		To:   consumer.ID,
	})
}

// Freeze implements starlark.Value.
func (o *Output) Freeze() {}

// Hash implements starlark.Value.
//
// Returns:
//   - uint32: unused, always 0.
//   - error: always non-nil (Output is unhashable).
func (o *Output) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: Output") }

// String implements starlark.Value.
//
// Returns:
//   - string: human-readable representation.
func (o *Output) String() string { return fmt.Sprintf("Output(%s)", o.node.ID) }

// Truth implements starlark.Value.
//
// Returns:
//   - starlark.Bool: always true.
func (o *Output) Truth() starlark.Bool { return true }

// Type implements starlark.Value.
//
// Returns:
//   - string: the type name "Output".
func (o *Output) Type() string { return "Output" }

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// retryBuiltin sets the retry policy on this output's node.
// Usage: node = plan.appnet.download(...); node.retry(max_attempts=5, backoff="linear")
//
// Parameters:
//   - thread: the Starlark thread (unused).
//   - b: the builtin (unused).
//   - args: positional arguments.
//   - kwargs: keyword arguments (max_attempts, backoff?, initial_delay?, max_delay?).
//
// Returns:
//   - starlark.Value: this Output (for chaining).
//   - error: non-nil if arguments are invalid.
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

// endregion

// endregion

// FillSlot fills a slot in a node from a Starlark value.
//
// Any slot accepts:
//   - A promise (Output): creates an edge, value flows at runtime
//   - A gather (Gather): creates edges from all members (parallel execution)
//   - An immediate value: stored directly, known at analysis time
//
// Parameters:
//   - node: the node whose slot to fill.
//   - graph: the execution graph (for edge creation).
//   - slotName: the slot to fill.
//   - value: the Starlark value to assign.
//
// Returns:
//   - error: non-nil if the value cannot be unmarshaled.
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

	// None: skip (optional parameter not provided)
	if _, ok := value.(starlark.NoneType); ok {
		return nil
	}

	// Immediate: unmarshal Starlark value to native Go type
	var goVal any
	if err := unmarshal(value, &goVal); err != nil {
		return fmt.Errorf("slot %q: %w", slotName, err)
	}

	// Resource identity: if the immediate value carries resource origin,
	// create an implicit edge from the origin node to the consumer. This
	// enables automatic dependency ordering when a resource produced by
	// one node flows to another.
	if originID, ok := extractResource(goVal); ok {
		graph.Edges = append(graph.Edges, Edge{From: originID, To: node.ID})
	}

	node.SetSlotImmediate(slotName, goVal)
	return nil
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
//
// Parameters:
//   - graph: the execution graph.
//   - outputs: the outputs to gather.
//
// Returns:
//   - *Gather: the new gather handle.
func NewGather(graph *Graph, outputs ...*Output) *Gather {

	return &Gather{
		outputs: outputs,
		graph:   graph,
	}
}

// region EXPORTED METHODS

// region State management

// Outputs returns the gathered outputs.
//
// Returns:
//   - []*Output: the collected output promises.
func (g *Gather) Outputs() []*Output {

	return g.outputs
}

// endregion

// region Behaviors

// Actions

// FillSlot fills a slot in the consumer node with all gathered promises,
// creating edges from each member. This enables parallel execution.
//
// Parameters:
//   - consumer: the node receiving the gathered promises.
//   - slotName: the slot to fill.
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

// Freeze implements starlark.Value.
func (g *Gather) Freeze() {}

// Hash implements starlark.Value.
//
// Returns:
//   - uint32: unused, always 0.
//   - error: always non-nil (Gather is unhashable).
func (g *Gather) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: Gather") }

// String implements starlark.Value.
//
// Returns:
//   - string: human-readable representation.
func (g *Gather) String() string {

	ids := make([]string, len(g.outputs))
	for i, o := range g.outputs {
		ids[i] = o.node.ID
	}
	return fmt.Sprintf("Gather(%v)", ids)
}

// Truth implements starlark.Value.
//
// Returns:
//   - starlark.Bool: true if the gather contains any outputs.
func (g *Gather) Truth() starlark.Bool { return len(g.outputs) > 0 }

// Type implements starlark.Value.
//
// Returns:
//   - string: the type name "Gather".
func (g *Gather) Type() string { return "Gather" }

// endregion

// endregion
