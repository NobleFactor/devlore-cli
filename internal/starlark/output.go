// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Output represents a promise - a handle to a node's output that can flow
// through the graph to fill slots in other nodes.
//
// When passed to a plan function's slot, it creates an edge in the graph.
// The same promise can flow to multiple slots (fan-out).
type Output struct {
	// node is the action that produces this output
	node *execution.Node

	// graph is the execution graph (for creating edges)
	graph *execution.Graph

	// slot identifies which output of the node this represents (empty = default)
	slot string
}

// NewOutput creates a new Output (promise) representing a node's output.
func NewOutput(node *execution.Node, graph *execution.Graph, slot string) *Output {
	return &Output{
		node:  node,
		graph: graph,
		slot:  slot,
	}
}

// Starlark Value interface
func (o *Output) String() string        { return fmt.Sprintf("Output(%s)", o.node.ID) }
func (o *Output) Type() string          { return "Output" }
func (o *Output) Freeze()               {}
func (o *Output) Truth() starlark.Bool  { return true }
func (o *Output) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: Output") }

// Starlark HasAttrs interface
func (o *Output) Attr(name string) (starlark.Value, error) {
	switch name {
	case "node_id":
		return starlark.String(o.node.ID), nil
	case "slot":
		return starlark.String(o.slot), nil
	default:
		// Get the value from the node's slots
		if value := o.node.GetSlot(name); value != "" {
			return starlark.String(value), nil
		}
		return nil, starlark.NoSuchAttrError(fmt.Sprintf("Output has no attribute %q", name))
	}
}

func (o *Output) AttrNames() []string {
	names := []string{"node_id", "slot"}
	// Add slot names from the node
	if o.node.Slots != nil {
		for name := range o.node.Slots {
			names = append(names, name)
		}
	}
	return names
}

// Node returns the execution node that produces this output.
func (o *Output) Node() *execution.Node {
	return o.node
}

// Graph returns the execution graph.
func (o *Output) Graph() *execution.Graph {
	return o.graph
}

// Slot returns which output slot this represents.
func (o *Output) Slot() string {
	return o.slot
}

// FillSlot fills a slot in the consumer node with this promise, creating an edge.
// This is called when a promise is passed to a plan function.
func (o *Output) FillSlot(consumer *execution.Node, slotName string) {
	// Set the slot to reference this output's node
	consumer.SetSlotPromise(slotName, o.node.ID, o.slot)

	// Create edge: consumer depends_on producer
	o.graph.Edges = append(o.graph.Edges, execution.Edge{
		From:     o.node.ID, // producer
		To:       consumer.ID,
		Relation: "depends_on",
	})
}

// FillSlot fills a slot in a node from a Starlark value.
//
// Any slot accepts either:
//   - A promise (Output): creates an edge, value flows at runtime
//   - An immediate value: stored directly, known at analysis time
func FillSlot(node *execution.Node, graph *execution.Graph, slotName string, value starlark.Value) error {
	// Promise: create edge, value flows at runtime
	if output, ok := value.(*Output); ok {
		output.FillSlot(node, slotName)
		return nil
	}

	// Immediate: store directly
	if str, ok := starlark.AsString(value); ok {
		node.SetSlotImmediate(slotName, str)
		return nil
	}

	// List: recurse for each element
	if list, ok := value.(*starlark.List); ok {
		iter := list.Iterate()
		defer iter.Done()
		var v starlark.Value
		i := 0
		for iter.Next(&v) {
			subSlot := fmt.Sprintf("%s[%d]", slotName, i)
			if err := FillSlot(node, graph, subSlot, v); err != nil {
				return fmt.Errorf("list element %d: %w", i, err)
			}
			i++
		}
		node.SetSlotImmediate(slotName+".len", fmt.Sprintf("%d", i))
		return nil
	}

	// Other immediate types
	switch v := value.(type) {
	case starlark.Int:
		i, _ := v.Int64()
		node.SetSlotImmediate(slotName, fmt.Sprintf("%d", i))
		return nil
	case starlark.Bool:
		node.SetSlotImmediate(slotName, fmt.Sprintf("%t", v))
		return nil
	case starlark.Float:
		node.SetSlotImmediate(slotName, fmt.Sprintf("%f", v))
		return nil
	case starlark.NoneType:
		return nil
	}

	return fmt.Errorf("unsupported value type %s for slot %q", value.Type(), slotName)
}


// Path returns a path from the node's slots.
func (o *Output) Path() string {
	return o.node.GetSlot("path")
}
