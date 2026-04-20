// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"

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
		return starlark.String(p.node.ID()), nil
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

	p.graph.Root.Edges = append(p.graph.Root.Edges, op.Edge{
		From: p.node.ID(),
		To:   consumer.ID(),
	})
}

// FillSlot fills a slot in the consumer node with this promise, creating the
// producer→consumer edge that will carry the value at runtime. Called from
// [NodeBuilder.fillSlot] when a promise is passed to a plan function.
//
// Parameters:
//   - consumer: the node receiving the promise.
//   - slot: the slot name to fill.
func (p *Promise) FillSlot(consumer *op.Node, slot string) {

	// Set the slot to reference this output's node
	consumer.SetSlot(slot, op.PromiseValue{NodeRef: p.node.ID(), Slot: p.slot})

	// Create edge: producer must complete before consumer
	p.graph.Root.Edges = append(p.graph.Root.Edges, op.Edge{
		From: p.node.ID(),
		To:   consumer.ID(),
	})
}

// Unmarshal projects this promise onto a Go target. For a *Promise target
// the pointer is stored directly; for a PromiseValue target the slot-ref
// shape is stored; for any other target it errors — promises are not
// directly resolvable to Go scalar types at plan time.
func (p *Promise) Unmarshal(target reflect.Value) error {

	promiseType := reflect.TypeOf((*Promise)(nil))
	promiseValueType := reflect.TypeOf(op.PromiseValue{})

	if target.Kind() == reflect.Interface {
		target.Set(reflect.ValueOf(p))
		return nil
	}
	if target.Type() == promiseType {
		target.Set(reflect.ValueOf(p))
		return nil
	}
	if target.Type() == promiseValueType {
		target.Set(reflect.ValueOf(op.PromiseValue{NodeRef: p.node.ID(), Slot: p.slot}))
		return nil
	}
	return fmt.Errorf("unmarshal: cannot assign Promise to %s (promises resolve at execute time)", target.Type())
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
func (p *Promise) String() string { return fmt.Sprintf("Promise(%s)", p.node.ID()) }

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
