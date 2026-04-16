// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

// Promise represents the promise of an output that can flow through the graph to fill slots in other nodes.
//
// When passed to a plan function's slot, it creates an edge in a graph. The same promise can flow to multiple slots,
// thereby fanning-out to many nodes in the graph.
type Promise struct {
	// node is the action that produces this output
	node *op.Node

	// graph is the execution graph (for creating edges)
	graph *op.Graph

	// slot identifies which output of the node this represents (empty = default)
	slot string
}

// NewPromise creates a new Promise (promise) representing a node's output.
//
// Parameters:
//   - node: the producing node.
//   - graph: the execution graph.
//   - slot: which output slot this represents (empty for default).
//
// Returns:
//   - *Promise: the new promise handle.
func NewPromise(graph *op.Graph, node *op.Node, slot string) *Promise {

	return &Promise{
		graph: graph,
		node:  node,
		slot:  slot,
	}
}

// region EXPORTED METHODS

// region State management

// Graph returns the execution graph.
//
// Returns:
//   - *Graph: the graph this output belongs to.
func (p *Promise) Graph() *op.Graph {

	return p.graph
}

// Node returns the node that produces this output.
//
// Returns:
//   - *Node: the producing node.
func (p *Promise) Node() *op.Node {

	return p.node
}

// Path returns a path from the node's slots.
//
// Returns:
//   - string: the path slot value, or empty string if not present or not a string.
func (p *Promise) Path() string {

	path, ok := p.node.SlotByName("path").Immediate().(string)
	if !ok {
		return ""
	}
	return path
}

// Slot returns which output slot this represents.
//
// Returns:
//   - string: the slot identifier.
func (p *Promise) Slot() string {

	return p.slot
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
func (p *Promise) Attr(name string) (starlark.Value, error) {

	switch name {
	case "node_id":
		return starlark.String(p.node.ID), nil
	case "slot":
		return starlark.String(p.slot), nil
	case "retry":
		return starlark.NewBuiltin("output.retry", p.retryBuiltin), nil
	default:
		// Get the value from the node's slots and convert to Starlark.

		slot := p.node.SlotByName(name)

		if slot == nil {
			return nil, starlark.NoSuchAttrError(fmt.Sprintf("Promise has no attribute %q", name))
		}

		slotVal := slot.Immediate()

		if slotVal == nil {
			return nil, fmt.Errorf("slot %q: not an immediate value", name)
		}

		switch v := slotVal.(type) {
		case string:
			return starlark.String(v), nil
		case int:
			return starlark.MakeInt(v), nil
		case int64:
			return starlark.MakeInt64(v), nil
		case bool:
			return starlark.Bool(v), nil
		case float64:
			return starlark.Float(v), nil
		case []byte:
			return starlark.Bytes(v), nil
		case starlark.Value:
			return v, nil
		default:
			return nil, fmt.Errorf("slot %q: unsupported Go type %T", name, slotVal)
		}
	}
}

// AttrNames implements starlark.HasAttrs.
//
// Returns:
//   - []string: all available attribute names.
func (p *Promise) AttrNames() []string {

	names := []string{"node_id", "retry", "slot"}
	for _, slot := range p.node.Slots {
		names = append(names, slot.Parameter.Name)
	}
	return names
}

// DependOn creates an edge making the given node depend on this output's node.
//
// Parameters:
//   - consumer: the node that should depend on this output's producer.
func (p *Promise) DependOn(consumer *op.Node) {

	p.graph.Edges = append(p.graph.Edges, op.Edge{
		From: p.node.ID,
		To:   consumer.ID,
	})
}

// fillSlot fills a slot in the consumer node with this promise, creating an edge.
// This is called when a promise is passed to a plan function.
//
// Parameters:
//   - consumer: the node receiving the promise.
//   - slotName: the slot to fill.
func (p *Promise) FillSlot(consumer *op.Node, slot string) {

	// Set the slot to reference this output's node
	consumer.SetSlot(slot, op.PromiseValue{NodeRef: p.node.ID, Slot: p.slot})

	// Create edge: producer must complete before consumer
	p.graph.Edges = append(p.graph.Edges, op.Edge{
		From: p.node.ID,
		To:   consumer.ID,
	})
}

// Freeze implements starlark.Value.
func (p *Promise) Freeze() {}

// Hash implements starlark.Value.
//
// Returns:
//   - uint32: unused, always 0.
//   - error: always non-nil (Promise is unhashable).
func (p *Promise) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: Promise") }

// String implements starlark.Value.
//
// Returns:
//   - string: human-readable representation.
func (p *Promise) String() string { return fmt.Sprintf("Promise(%s)", p.node.ID) }

// Truth implements starlark.Value.
//
// Returns:
//   - starlark.Bool: always true.
func (p *Promise) Truth() starlark.Bool { return true }

// Type implements starlark.Value.
//
// Returns:
//   - string: the type name "Promise".
func (p *Promise) Type() string { return "Promise" }

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
//   - starlark.Value: this Promise (for chaining).
//   - error: non-nil if arguments are invalid.
func (p *Promise) retryBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

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

	policy := &op.RetryPolicy{
		MaxAttempts: maxAttempts,
	}

	if backoff != "" {
		switch backoff {
		case "none":
			policy.Backoff = op.BackoffNone
		case "linear":
			policy.Backoff = op.BackoffLinear
		case "exponential":
			policy.Backoff = op.BackoffExponential
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

	p.node.Retry = policy
	return p, nil
}

// endregion

// endregion

// fillSlot fills a slot in a node of a graph with a Starlark value.
//
// Any slot accepts:
//   - A promise (Promise): creates an edge, value flows at runtime
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
func fillSlot(graph *op.Graph, node *op.Node, slot string, value starlark.Value) error {

	// Promise: create edge, value flows at runtime

	if output, ok := value.(*Promise); ok {
		output.FillSlot(node, slot)
		return nil
	}

	if list, ok := value.(*starlark.List); ok {
		if fillOutputList(node, graph, slot, list) {
			// Fan-in by creating edges from all members. Each Promise forms an indexed sub-slot and a dependency edge.
			return nil
		}
		// Not a list of Outputs — fall through to immediate handling.
	}

	if _, ok := value.(starlark.NoneType); ok {
		return nil // skip an optional parameter that was not provided.
	}

	if v, ok := value.(*receiver); ok {
		// The starlark value wraps a Go resource so extract it directly. This preserves identity and origin through the
		// planning layer without a lossy marshal→unmarshal round-trip.
		goVal := v.instance
		if originID, found := op.ExtractResource(goVal); found {
			graph.Edges = append(graph.Edges, op.Edge{From: originID, To: node.ID})
		}
		node.SetSlot(slot, op.ImmediateValue{Value: goVal})
		return nil
	}

	// Immediate: convert starlark primitive to its native Go equivalent.

	var goVal any

	switch v := value.(type) {
	case starlark.String:
		goVal = string(v)
	case starlark.Int:
		i, ok := v.Int64()
		if !ok {
			return fmt.Errorf("slot %q: int value out of range", slot)
		}
		goVal = int(i)
	case starlark.Bool:
		goVal = bool(v)
	case starlark.Float:
		goVal = float64(v)
	case starlark.Bytes:
		goVal = []byte(v)
	default:
		return fmt.Errorf("slot %q: unsupported starlark type %s", slot, value.Type())
	}

	node.SetSlot(slot, op.ImmediateValue{Value: goVal})
	return nil
}

// fillOutputList checks if a starlark.List contains only *Promise values.
// If so, it creates edges and indexed sub-slots for each element (fan-in)
// and returns true. If any element is not an *Promise, it returns false
// without modifying the node or graph.
func fillOutputList(node *op.Node, graph *op.Graph, slotName string, list *starlark.List) bool {
	n := list.Len()
	if n == 0 {
		return false
	}

	// Validate all elements are *Promise before mutating anything.
	outputs := make([]*Promise, n)
	for i := range n {
		output, ok := list.Index(i).(*Promise)
		if !ok {
			return false
		}
		outputs[i] = output
	}

	// All elements are promises — create edges and indexed sub-slots.
	for i, output := range outputs {
		subSlot := fmt.Sprintf("%s[%d]", slotName, i)
		node.SetSlot(subSlot, op.PromiseValue{NodeRef: output.node.ID, Slot: output.slot})
		graph.Edges = append(graph.Edges, op.Edge{
			From: output.node.ID,
			To:   node.ID,
		})
	}
	node.SetSlot(slotName+".len", op.ImmediateValue{Value: n})
	return true
}
