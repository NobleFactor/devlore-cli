// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

var (
	_ starlark.Value    = (*ProviderNodeBuilder)(nil) // Interface Guard: ensures *ProviderNodeBuilder implements starlark.Value.
	_ starlark.HasAttrs = (*ProviderNodeBuilder)(nil) // Interface Guard: ensures *ProviderNodeBuilder implements starlark.HasAttrs.
)

// ProviderNodeBuilder wraps a provider's method signatures for plan-mode starlark use.
//
// It implements [starlark.Value] and [starlark.HasAttrs]. Each attribute access resolves a method on the underlying
// [op.ProviderReceiverType] and returns a [starlark.Builtin] bound to the plan adapter's dispatch method, which creates
// a graph node instead of executing the method directly.
//
// ProviderNodeBuilder does not embed [executingReceiver] — it operates on a graph, not a provider instance.
type ProviderNodeBuilder struct {
	receiverType op.ProviderReceiverType
	graph        *op.Graph
	methods      map[string]*op.Method // snake_name → *Method
	attrNames    []string              // sorted
}

// NewProviderNodeBuilder creates a [ProviderNodeBuilder] wrapping the given graph and receiver type.
//
// Parameters:
//   - rt: the provider receiver type descriptor.
//   - graph: the execution graph to append nodes to.
//
// Returns:
//   - *ProviderNodeBuilder: the starlark-ready wrapper.
func NewProviderNodeBuilder(rt op.ProviderReceiverType, graph *op.Graph) *ProviderNodeBuilder {

	methods := make(map[string]*op.Method)
	names := make([]string, 0)

	for method := range rt.Methods() {
		snake := camelToSnake(method.Name())
		methods[snake] = method
		names = append(names, snake)
	}

	sort.Strings(names)

	return &ProviderNodeBuilder{
		receiverType: rt,
		graph:        graph,
		methods:      methods,
		attrNames:    names,
	}
}

// region EXPORTED METHODS

// region State management

// String implements starlark.Value.
func (p *ProviderNodeBuilder) String() string { return "plan." + p.receiverType.Name() }

// Type implements starlark.Value.
func (p *ProviderNodeBuilder) Type() string { return "plan." + p.receiverType.Name() }

// Freeze implements starlark.Value.
func (p *ProviderNodeBuilder) Freeze() {}

// Truth implements starlark.Value.
func (p *ProviderNodeBuilder) Truth() starlark.Bool { return true }

// Hash implements starlark.Value.
func (p *ProviderNodeBuilder) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: plan.%s", p.receiverType.Name())
}

// endregion

// region Behaviors

// Attr implements starlark.HasAttrs.
//
// Parameters:
//   - name: the snake_case attribute name to look up.
//
// Returns:
//   - starlark.Value: a builtin bound to this plan adapter's dispatch method.
//   - error: non-nil if the attribute does not exist.
func (p *ProviderNodeBuilder) Attr(name string) (starlark.Value, error) {

	if _, ok := p.methods[name]; !ok {
		return nil, NoSuchAttrError("plan."+p.receiverType.Name(), name)
	}
	actionName := p.receiverType.Name() + "." + name
	return starlark.NewBuiltin(actionName, p.dispatch), nil
}

// AttrNames implements starlark.HasAttrs.
//
// Returns:
//   - []string: sorted list of available method names.
func (p *ProviderNodeBuilder) AttrNames() []string { return p.attrNames }

// endregion

// endregion

// region UNEXPORTED METHODS

// dispatch dispatches a starlark builtin invocation to create a graph node for deferred execution.
//
// Parameters:
//   - thread: the starlark thread.
//   - builtin: the starlark builtin that triggered the dispatch.
//   - args: positional starlark arguments.
//   - kwargs: keyword starlark arguments.
//
// Returns:
//   - starlark.Value: a [Promise] representing the deferred result.
//   - error: non-nil if node creation fails.
func (p *ProviderNodeBuilder) dispatch(thread *starlark.Thread, builtin *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	actionName := builtin.Name()

	name := actionName[strings.LastIndex(actionName, ".")+1:]
	method := p.methods[name]
	params := method.Parameters()

	cleanName := func(raw string) string {
		return strings.TrimSuffix(strings.TrimPrefix(raw, "*"), "?")
	}

	// Single-pass classification: produce the slot sequence plus the scratch values slice and UnpackArgs pair list in
	// lockstep. Slots carry full Parameter identity (Name + Type); nothing else does.

	slots := make([]*op.Slot, 0, len(params))
	values := make([]starlark.Value, 0, len(params))
	pairs := make([]any, 0, len(params)*2)
	var kwargsSlot *op.Slot

	for _, param := range params {
		if strings.HasPrefix(param.Name, "**") {
			kwargsSlot = &op.Slot{Parameter: param}
			continue
		}
		slots = append(slots, &op.Slot{Parameter: param})
		values = append(values, nil)
		pairs = append(pairs, strings.TrimPrefix(param.Name, "*"), &values[len(values)-1])
	}

	// Split kwargs: known → UnpackArgs, unknown → **kwargs dict slots. The predicate scans the slot sequence — fine at
	// method-signature sizes, and avoids carrying a parallel lookup map.

	var filteredKwargs []starlark.Tuple
	var extraKwargs []starlark.Tuple

	if kwargsSlot != nil {
		for _, kv := range kwargs {
			key, _ := starlark.AsString(kv[0])
			known := false
			for _, slot := range slots {
				if cleanName(slot.Parameter.Name) == key {
					known = true
					break
				}
			}
			if known {
				filteredKwargs = append(filteredKwargs, kv)
			} else {
				extraKwargs = append(extraKwargs, kv)
			}
		}
	} else {
		filteredKwargs = kwargs
	}

	if err := starlark.UnpackArgs(actionName, args, filteredKwargs, pairs...); err != nil {
		return nil, err
	}

	// Create node.

	node := op.NewNode(op.GenerateNodeID(actionName))
	node.Receiver = actionName

	// Fill slots from starlark values via the unified FillSlot pipeline. Resource-typed targets are handled by
	// FillSlot's generic assign path (Unmarshaler + op.Convert) with link-time catalog resolution applied to any
	// op.Resource result.

	for i, slot := range slots {

		sv := values[i]

		if sv == nil {
			continue
		}

		if err := p.fillSlot(node, slot, sv); err != nil {
			return nil, fmt.Errorf("%s: %w", cleanName(slot.Parameter.Name), err)
		}
	}

	// Fill **kwargs as a single map slot matching the method's **kwargs parameter. The executing path (receiver.go)
	// consumes this as one map[string]any argument; packing extras into a starlark.Dict here lets the dict unmarshaler
	// project into the parameter's map target.

	if kwargsSlot != nil && len(extraKwargs) > 0 {
		dict := starlark.NewDict(len(extraKwargs))
		for _, kv := range extraKwargs {
			if err := dict.SetKey(kv[0], kv[1]); err != nil {
				return nil, fmt.Errorf("%s: kwargs: %w", actionName, err)
			}
		}
		if err := p.fillSlot(node, kwargsSlot, dict); err != nil {
			return nil, fmt.Errorf("%s: kwargs: %w", actionName, err)
		}
	}

	// Plan-time output shadowing. If this method has a Planned companion, call it with the filled slot values to
	// compute the pending resource the node will produce, and shadow it in the catalog. If the companion returns
	// KnownAtExecution, skip plan-time shadowing — the executor will shadow the real return value post-dispatch.
	if method.HasPlanned() {
		if err := p.shadowPendingOutput(node, method); err != nil {
			return nil, fmt.Errorf("%s: %w", actionName, err)
		}
	}

	// Append to graph and return promise.

	p.graph.AddNode(node)
	return NewPromise(p.graph, node, ""), nil
}

// fillSlot populates a slot on a node from a starlark value.
//
// Graph-edge dispatch (Promise, list-of-Promises) comes first: these mutate graph structure. Other values flow through
// the Unmarshaler pipeline with a fall-back to [op.Convert] for target types Unmarshal cannot assign to directly
// (notably registered resource types constructed from primitives). After assignment, a Resource result is link-resolved
// against the graph catalog and, if the linked entry carries a producer origin, an implicit edge is added from the
// producer to this node.
//
// The caller supplies slot.Parameter pre-populated. fillSlot fills the slot's value and installs it on the node via
// [op.Node.SetSlot].
//
// Parameters:
//   - node: the node whose slot is being filled.
//   - slot: the slot (Parameter pre-populated; Value will be filled).
//   - value: the starlark value driving the fill.
//
// Returns:
//   - error: non-nil if the value cannot be assigned to the slot's target type.
func (p *ProviderNodeBuilder) fillSlot(node *op.Node, slot *op.Slot, value starlark.Value) error {

	name := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(slot.Parameter.Name, "**"), "*"), "?")

	// Promise: create edge, value flows at runtime.

	if promise, ok := value.(*Promise); ok {
		promise.FillSlot(node, name)
		return nil
	}

	// List of Promises: fan-in via indexed sub-slots.

	if list, ok := value.(*starlark.List); ok {
		if n := list.Len(); n > 0 {
			promises := make([]*Promise, n)
			allPromises := true
			for i := range n {
				promise, ok := list.Index(i).(*Promise)
				if !ok {
					allPromises = false
					break
				}
				promises[i] = promise
			}
			if allPromises {
				for i, promise := range promises {
					subSlot := fmt.Sprintf("%s[%d]", name, i)
					node.SetSlot(subSlot, op.PromiseValue{NodeRef: promise.node.ID(), Slot: promise.slot})
					p.graph.Root.Edges = append(p.graph.Root.Edges, op.Edge{From: promise.node.ID(), To: node.ID()})
				}
				node.SetSlot(name+".len", op.ImmediateValue{Value: n})
				return nil
			}
		}
	}

	// NoneType: skip optional parameter.

	if _, ok := value.(starlark.NoneType); ok {
		return nil
	}

	// *receiver: extract Go value directly.
	//
	// Preserves identity and origin through the planning layer without a marshal→unmarshal round-trip.
	if r, ok := value.(*receiver); ok {
		goVal := r.instance
		if originID, found := op.ExtractResource(goVal); found {
			p.graph.Root.Edges = append(p.graph.Root.Edges, op.Edge{From: originID, To: node.ID()})
		}
		node.SetSlot(name, op.ImmediateValue{Value: goVal})
		return nil
	}

	// Generic path: unmarshal into target type.
	//
	// On mismatch, fall back to unmarshal-into-any + op.Convert so registry-based target instantiation (e.g., string →
	// *file.Resource) takes over.

	final, err := p.assignTarget(value, slot.Parameter.Type)

	if err != nil {
		return fmt.Errorf("slot %q: %w", name, err)
	}

	// Result-based dispatch: if the value is a Resource, link it against the graph catalog and pick up any producer
	// origin as an implicit edge.

	if res, ok := final.(op.Resource); ok {
		final = p.linkResource(node, res, slot.Parameter.Type)
	}

	node.SetSlot(name, op.ImmediateValue{Value: final})
	return nil
}

// shadowPendingOutput invokes the method's Planned companion on a freshly-constructed provider instance with the
// node's filled slot values, and shadows the resulting pending resource in the catalog.
//
// The Planned companion is pure — it constructs the resource identity without I/O. The returned resource is
// registered via [ResourceCatalog.Shadow] with the node's ID as its origin, creating a pending entry that
// pre-flight skips and post-dispatch shadowing transitions to resolved.
//
// If the companion returns [op.KnownAtExecution], the method's output identity depends on runtime values and
// cannot be shadowed at plan time. The function returns nil; the executor will shadow the real return value after
// the forward method runs.
//
// Parameters:
//   - node: the node whose output is being shadowed.
//   - method: the method whose Planned companion computes the identity.
//
// Returns:
//   - error: non-nil if provider construction, the Planned call, or catalog shadowing fails.
func (p *ProviderNodeBuilder) shadowPendingOutput(node *op.Node, method *op.Method) error {

	// Build positional args from node slots in parameter order. Unresolved slots pass through as nil; Method.Plan
	// substitutes zero values and the Planned method must tolerate them (or return KnownAtExecution if it cannot
	// compute without them).
	params := method.Parameters()
	args := make([]any, len(params))
	for i, param := range params {
		cleanName := strings.TrimSuffix(strings.TrimPrefix(param.Name, "*"), "?")
		if slot := node.SlotByName(cleanName); slot != nil {
			if iv, ok := slot.Value.(op.ImmediateValue); ok {
				args[i] = iv.Value
			}
		}
	}

	// Construct a provider instance so the Planned method has its receiver context available. Planned is pure — the
	// context is used only for identity construction (e.g., resolving paths under the confined root), not for I/O.
	receiver, err := p.receiverType.Construct()(p.graph.ExecutionContext())
	if err != nil {
		return fmt.Errorf("construct receiver: %w", err)
	}

	result, err := method.Plan(receiver, args)
	if err != nil {
		return fmt.Errorf("planned: %w", err)
	}
	if !result.IsValid() {
		return nil
	}
	if kind := result.Kind(); (kind == reflect.Ptr || kind == reflect.Interface) && result.IsNil() {
		return nil
	}

	pending, ok := result.Interface().(op.Resource)
	if !ok {
		return fmt.Errorf("planned companion for %s did not return op.Resource", method.Name())
	}
	if op.IsKnownAtExecution(pending) {
		return nil
	}

	if p.graph.Catalog == nil {
		p.graph.Catalog = op.NewResourceCatalog()
	}
	if _, err := p.graph.Catalog.Shadow(pending, node.ID()); err != nil {
		return fmt.Errorf("shadow output: %w", err)
	}
	return nil
}

// assignTarget unmarshals value into the target Go type with fallback to [op.Convert] when the target is unreachable.
//
// The direct path preserves typed fidelity for structural types (e.g., maps keyed by string, slices of typed elements)
// that the unmarshalers project directly. The fallback path unmarshals into an interface target and lets op.Convert
// handle target-type instantiation — notably registry-based construction of Resource types from primitive sources
// (e.g., string → *file.Resource).
func (p *ProviderNodeBuilder) assignTarget(value starlark.Value, target reflect.Type) (any, error) {

	u, err := ToUnmarshaler(value)
	if err != nil {
		return nil, err
	}
	rv := reflect.New(target).Elem()
	if err := u.Unmarshal(rv); err == nil {
		return rv.Interface(), nil
	}
	var raw any
	if err := u.Unmarshal(reflect.ValueOf(&raw).Elem()); err != nil {
		return nil, err
	}
	return op.Convert(p.graph.ExecutionContext(), raw, target)
}

// linkResource performs link-time resolution of a [op.Resource] against the graph catalog and wires the
// producer→consumer edge if the catalog hands back an entry stamped with a producer origin.
//
// The catalog is an intern table keyed by URI. The first resource seen for a URI is kept as a discovery entry;
// subsequent values for the same URI are discarded in favor of the existing entry — which may already carry an originID
// stamped by a producer node's Planned companion. When an origin is present, an A→node edge wires the producer-consumer
// relationship the author expressed by URI alone.
//
// If the slot's target type is a value (not a pointer), the linked entry is dereferenced for storage; pointer targets
// store the linked entry directly so all holders observe the same instance.
func (p *ProviderNodeBuilder) linkResource(node *op.Node, res op.Resource, target reflect.Type) any {

	if p.graph.Catalog == nil {
		p.graph.Catalog = op.NewResourceCatalog()
	}
	linked, _ := p.graph.Catalog.Resolve(res)

	if originID, found := op.ExtractResource(linked); found {
		p.graph.Root.Edges = append(p.graph.Root.Edges, op.Edge{From: originID, To: node.ID()})
	}

	if target.Kind() == reflect.Ptr {
		return linked
	}
	rv := reflect.ValueOf(linked)
	if rv.Kind() == reflect.Ptr {
		return rv.Elem().Interface()
	}
	return linked
}

// endregion
