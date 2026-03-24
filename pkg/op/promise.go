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
//   - A list of promises: creates edges from all members (fan-in)
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

	// List of promises: create edges from all members (fan-in).
	// Each Output element creates an indexed sub-slot and a dependency edge.
	if list, ok := value.(*starlark.List); ok {
		if fillOutputList(node, graph, slotName, list) {
			return nil
		}
		// Not a list of Outputs — fall through to immediate handling.
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

// fillOutputList checks if a starlark.List contains only *Output values.
// If so, it creates edges and indexed sub-slots for each element (fan-in)
// and returns true. If any element is not an *Output, it returns false
// without modifying the node or graph.
func fillOutputList(node *Node, graph *Graph, slotName string, list *starlark.List) bool {
	n := list.Len()
	if n == 0 {
		return false
	}

	// Validate all elements are *Output before mutating anything.
	outputs := make([]*Output, n)
	for i := range n {
		output, ok := list.Index(i).(*Output)
		if !ok {
			return false
		}
		outputs[i] = output
	}

	// All elements are promises — create edges and indexed sub-slots.
	for i, output := range outputs {
		subSlot := fmt.Sprintf("%s[%d]", slotName, i)
		node.SetSlotPromise(subSlot, output.node.ID, output.slot)
		graph.Edges = append(graph.Edges, Edge{
			From: output.node.ID,
			To:   node.ID,
		})
	}
	node.SetSlotImmediate(slotName+".len", n)
	return true
}
