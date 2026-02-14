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
		// Get the value from the node's slots and convert to Starlark
		slotVal := o.node.GetSlot(name)
		if slotVal == nil {
			return nil, starlark.NoSuchAttrError(fmt.Sprintf("Output has no attribute %q", name))
		}
		sv, err := goToStarlarkValue(slotVal)
		if err != nil {
			return nil, fmt.Errorf("slot %q: %w", name, err)
		}
		return sv, nil
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

	// Create edge: producer must complete before consumer
	o.graph.Edges = append(o.graph.Edges, execution.Edge{
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
func FillSlot(node *execution.Node, graph *execution.Graph, slotName string, value starlark.Value) error {
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
		result, err := starlarkListToSlice(list)
		if err != nil {
			return fmt.Errorf("slot %q: %w", slotName, err)
		}
		node.SetSlotImmediate(slotName, result)
		return nil
	}

	// Dict: store as native map[string]any
	if dict, ok := value.(*starlark.Dict); ok {
		result, err := starlarkDictToMap(dict)
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
	path, _ := o.node.GetSlot("path").(string)
	return path
}

// DependOn creates an edge making the given node depend on this output's node.
func (o *Output) DependOn(consumer *execution.Node) {
	o.graph.Edges = append(o.graph.Edges, execution.Edge{
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

// goToStarlarkValue converts a native Go value to a Starlark value.
func goToStarlarkValue(v any) (starlark.Value, error) {
	switch val := v.(type) {
	case string:
		return starlark.String(val), nil
	case int:
		return starlark.MakeInt(val), nil
	case int64:
		return starlark.MakeInt64(val), nil
	case bool:
		return starlark.Bool(val), nil
	case float64:
		return starlark.Float(val), nil
	case []string:
		elems := make([]starlark.Value, len(val))
		for i, s := range val {
			elems[i] = starlark.String(s)
		}
		return starlark.NewList(elems), nil
	default:
		return starlark.String(fmt.Sprintf("%v", val)), nil
	}
}

// starlarkValueToGo converts a Starlark value to a native Go value.
func starlarkValueToGo(v starlark.Value) (any, error) {
	switch val := v.(type) {
	case starlark.String:
		return string(val), nil
	case starlark.Int:
		i, _ := val.Int64()
		return int(i), nil
	case starlark.Bool:
		return bool(val), nil
	case starlark.Float:
		return float64(val), nil
	case starlark.NoneType:
		return nil, nil
	case *starlark.List:
		return starlarkListToSlice(val)
	case *starlark.Dict:
		return starlarkDictToMap(val)
	default:
		return nil, fmt.Errorf("unsupported Starlark type %s", v.Type())
	}
}

// starlarkListToSlice converts a Starlark list to a Go slice.
// Returns []string if all elements are strings, []any otherwise.
func starlarkListToSlice(list *starlark.List) (any, error) {
	n := list.Len()
	if n == 0 {
		return []string{}, nil
	}

	// Try homogeneous []string first
	allStrings := true
	for i := 0; i < n; i++ {
		if _, ok := list.Index(i).(starlark.String); !ok {
			allStrings = false
			break
		}
	}

	if allStrings {
		result := make([]string, n)
		for i := 0; i < n; i++ {
			result[i] = string(list.Index(i).(starlark.String))
		}
		return result, nil
	}

	// Mixed types: []any
	result := make([]any, n)
	for i := 0; i < n; i++ {
		val, err := starlarkValueToGo(list.Index(i))
		if err != nil {
			return nil, fmt.Errorf("list element %d: %w", i, err)
		}
		result[i] = val
	}
	return result, nil
}

// starlarkDictToMap converts a Starlark dict to a Go map[string]any.
func starlarkDictToMap(dict *starlark.Dict) (map[string]any, error) {
	result := make(map[string]any, dict.Len())
	for _, item := range dict.Items() {
		key, ok := starlark.AsString(item[0])
		if !ok {
			return nil, fmt.Errorf("dict key must be string, got %s", item[0].Type())
		}
		val, err := starlarkValueToGo(item[1])
		if err != nil {
			return nil, fmt.Errorf("dict key %q: %w", key, err)
		}
		result[key] = val
	}
	return result, nil
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
	graph   *execution.Graph
}

// NewGather creates a new Gather from multiple outputs.
func NewGather(graph *execution.Graph, outputs ...*Output) *Gather {
	return &Gather{
		outputs: outputs,
		graph:   graph,
	}
}

// Starlark Value interface
func (g *Gather) String() string {
	ids := make([]string, len(g.outputs))
	for i, o := range g.outputs {
		ids[i] = o.node.ID
	}
	return fmt.Sprintf("Gather(%v)", ids)
}
func (g *Gather) Type() string          { return "Gather" }
func (g *Gather) Freeze()               {}
func (g *Gather) Truth() starlark.Bool  { return len(g.outputs) > 0 }
func (g *Gather) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: Gather") }

// Outputs returns the gathered outputs.
func (g *Gather) Outputs() []*Output {
	return g.outputs
}

// FillSlot fills a slot in the consumer node with all gathered promises,
// creating edges from each member. This enables parallel execution.
func (g *Gather) FillSlot(consumer *execution.Node, slotName string) {
	for i, output := range g.outputs {
		// Set slot reference for each output
		subSlot := fmt.Sprintf("%s[%d]", slotName, i)
		consumer.SetSlotPromise(subSlot, output.node.ID, output.slot)

		// Create edge: producer must complete before consumer
		g.graph.Edges = append(g.graph.Edges, execution.Edge{
			From: output.node.ID,
			To:   consumer.ID,
		})
	}
	// Store count for runtime
	consumer.SetSlotImmediate(slotName+".len", len(g.outputs))
}
