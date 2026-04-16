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
	_ starlark.Value    = (*Planner)(nil) // Interface Guard: ensures *Planner implements starlark.Value.
	_ starlark.HasAttrs = (*Planner)(nil) // Interface Guard: ensures *Planner implements starlark.HasAttrs.
)

// Planner wraps a provider's method signatures for plan-mode starlark use.
//
// It implements [starlark.Value] and [starlark.HasAttrs]. Each attribute access resolves a method on the underlying
// [op.ProviderReceiverType] and returns a [starlark.Builtin] bound to the planner's dispatch method, which creates a
// graph node instead of executing the method directly.
//
// Planner does not embed [executingReceiver] — it operates on a graph, not a provider instance.
type Planner struct {
	receiverType op.ProviderReceiverType
	graph        *op.Graph
	methods      map[string]*op.Method // snake_name → *Method
	attrNames    []string              // sorted
}

// NewPlanner creates a [Planner] wrapping the given graph and receiver type.
//
// Parameters:
//   - rt: the provider receiver type descriptor.
//   - graph: the execution graph to append nodes to.
//
// Returns:
//   - *Planner: the starlark-ready wrapper.
func NewPlanner(rt op.ProviderReceiverType, graph *op.Graph) *Planner {

	methods := make(map[string]*op.Method)
	names := make([]string, 0)
	for method := range rt.Methods() {
		snake := camelToSnake(method.Name())
		methods[snake] = method
		names = append(names, snake)
	}
	sort.Strings(names)

	return &Planner{
		receiverType: rt,
		graph:        graph,
		methods:      methods,
		attrNames:    names,
	}
}

// region EXPORTED METHODS

// region State management

// String implements starlark.Value.
func (p *Planner) String() string { return "plan." + p.receiverType.Name() }

// Type implements starlark.Value.
func (p *Planner) Type() string { return "plan." + p.receiverType.Name() }

// Freeze implements starlark.Value.
func (p *Planner) Freeze() {}

// Truth implements starlark.Value.
func (p *Planner) Truth() starlark.Bool { return true }

// Hash implements starlark.Value.
func (p *Planner) Hash() (uint32, error) {
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
//   - starlark.Value: a builtin bound to this planner's dispatch method.
//   - error: non-nil if the attribute does not exist.
func (p *Planner) Attr(name string) (starlark.Value, error) {

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
func (p *Planner) AttrNames() []string { return p.attrNames }

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

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
func (p *Planner) dispatch(thread *starlark.Thread, builtin *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	actionName := builtin.Name()

	name := actionName[strings.LastIndex(actionName, ".")+1:]
	method := p.methods[name]
	params := method.Parameters()

	// Classify parameters.
	//
	// We build a clean-name → type map so we can coerce strings to typed resources at plan time. Every resource-typed
	// parameter is an INPUT (discovery entry in the catalog); the method's output spec handles the output shadowing
	// separately, after all slots are filled.

	var kwargsName string
	knownKwargs := make(map[string]bool, len(params))
	regularParams := make([]string, 0, len(params))
	paramTypes := make(map[string]reflect.Type, len(params))

	for _, param := range params {
		if strings.HasPrefix(param.Name, "**") {
			kwargsName = strings.TrimPrefix(param.Name, "**")
		} else {
			regularParams = append(regularParams, param.Name)
			clean := strings.TrimPrefix(param.Name, "*")
			clean = strings.TrimSuffix(clean, "?")
			knownKwargs[clean] = true
			paramTypes[clean] = param.Type
		}
	}

	// Unpack args as raw starlark values.
	//
	// Slots store Go types, not Starlark values.

	values := make([]starlark.Value, len(regularParams))
	pairs := make([]any, 0, len(regularParams)*2)
	for i, paramName := range regularParams {
		pairs = append(pairs, strings.TrimPrefix(paramName, "*"), &values[i])
	}

	// Split kwargs: known → UnpackArgs, unknown → **kwargs dict slots.

	var filteredKwargs []starlark.Tuple
	var extraKwargs []starlark.Tuple

	if kwargsName != "" {
		for _, kv := range kwargs {
			key, _ := starlark.AsString(kv[0])
			if knownKwargs[key] {
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

	// Fill slots from starlark values. Every resource-typed parameter is
	// coerced from a string to a typed resource and routed through the
	// catalog as a discovery entry (input). The method's output spec
	// handles shadowing the output after the loop.

	for i, paramName := range regularParams {

		cleanName := strings.TrimSuffix(paramName, "?")
		cleanName = strings.TrimPrefix(cleanName, "*")
		sv := values[i]

		if sv == nil {
			continue
		}

		if targetType, ok := paramTypes[cleanName]; ok {
			if err := p.fillResourceSlot(node, cleanName, targetType, sv); err != nil {
				return nil, fmt.Errorf("%s: %w", cleanName, err)
			}
			if node.SlotByName(cleanName) != nil {
				continue
			}
		}

		if err := fillSlot(p.graph, node, cleanName, sv); err != nil {
			return nil, fmt.Errorf("%s: %w", cleanName, err)
		}
	}

	// Fill **kwargs as dict sub-slots.

	if kwargsName != "" {
		for _, kv := range extraKwargs {
			key, _ := starlark.AsString(kv[0])
			subSlot := fmt.Sprintf("%s.%s", kwargsName, key)
			if err := fillSlot(p.graph, node, subSlot, kv[1]); err != nil {
				return nil, fmt.Errorf("%s: kwarg %s: %w", actionName, key, err)
			}
		}
	}

	// Plan-time output shadowing. If this method has a Planned companion,
	// call it with the filled slot values to compute the pending resource
	// the node will produce, and shadow it in the catalog.
	//
	// If the companion returns KnownAtExecution, skip plan-time shadowing —
	// the executor will shadow the real return value post-dispatch.
	if method.HasPlanned() {
		if err := p.shadowPendingOutput(node, method); err != nil {
			return nil, fmt.Errorf("%s: %w", actionName, err)
		}
	}

	// Append to graph and return promise.

	p.graph.AddNode(node)
	return NewPromise(p.graph, node, ""), nil
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
func (p *Planner) shadowPendingOutput(node *op.Node, method *op.Method) error {

	// Build positional args from node slots in parameter order. Unresolved
	// slots pass through as nil; Method.Plan substitutes zero values and the
	// Planned method must tolerate them (or return KnownAtExecution if it
	// cannot compute without them).
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

	// Construct a provider instance so the Planned method has its receiver
	// context available. Planned is pure — the context is used only for
	// identity construction (e.g., resolving paths under the confined root),
	// not for I/O.
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

// isResourceType returns true if the given type is a registered resource type.
func (p *Planner) isResourceType(t reflect.Type) bool {
	if t == nil {
		return false
	}
	elemType := t
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
	}
	if p.graph == nil || p.graph.ExecutionContext() == nil || p.graph.ExecutionContext().Registry == nil {
		return false
	}
	rt, found := p.graph.ExecutionContext().Registry.TypeByReflection(elemType)
	if !found {
		return false
	}
	_, isResourceType := rt.(op.ResourceReceiverType)
	return isResourceType
}

// fillResourceSlot coerces a starlark string argument to a typed resource and routes it through the catalog as
// an input (discovery entry).
//
// Every resource-typed parameter of a method is an input from the planner's perspective. Outputs are handled
// separately by the method's output spec, called after all slots are filled in [Planner.shadowPendingOutput].
//
// The string is passed to the resource type's registered constructor (pure — no I/O) to build the typed resource.
// If the URI is already cataloged (e.g., by an earlier node that shadowed it as its output), the canonical
// shadowed entry is returned from [ResourceCatalog.Resolve] and an implicit edge is created from that origin to
// the current node. Otherwise the resource is cataloged as a new discovery entry.
//
// Parameters:
//   - node: the node whose slot is being filled.
//   - slotName: the slot name.
//   - targetType: the Go reflect.Type of the method parameter.
//   - sv: the starlark value being assigned.
//
// Returns:
//   - error: non-nil if coercion or catalog registration fails. The slot is left unset on error so the caller
//     can fall through to [fillSlot] for normal handling.
func (p *Planner) fillResourceSlot(node *op.Node, slotName string, targetType reflect.Type, sv starlark.Value) error {

	// Only strings need coercion — other starlark values route through FillSlot as before.
	if _, isString := sv.(starlark.String); !isString {
		return nil
	}
	s, _ := starlark.AsString(sv)

	// Check if the target type is a registered resource type.
	elemType := targetType
	isPtr := false
	if elemType.Kind() == reflect.Ptr {
		elemType = elemType.Elem()
		isPtr = true
	}

	if p.graph == nil || p.graph.ExecutionContext() == nil || p.graph.ExecutionContext().Registry == nil {
		return nil
	}

	ctx := p.graph.ExecutionContext()

	rt, found := ctx.Registry.TypeByReflection(elemType)
	if !found {
		return nil
	}
	rrt, isResourceType := rt.(op.ResourceReceiverType)
	if !isResourceType {
		return nil
	}

	// Plan-time construction: pure, no I/O. The constructor must tolerate a
	// context that has no Root/Platform/etc. populated — it is only used for
	// identity and type, not for filesystem access.
	constructed, err := rrt.Construct()(ctx, s)
	if err != nil {
		return fmt.Errorf("construct %s from %q: %w", elemType.Name(), s, err)
	}
	res, ok := constructed.(op.Resource)
	if !ok {
		return fmt.Errorf("constructor for %s did not return op.Resource", elemType.Name())
	}

	if p.graph.Catalog == nil {
		p.graph.Catalog = op.NewResourceCatalog()
	}

	// Resolve as a discovery entry. If the URI was already shadowed by an
	// earlier node's output spec, the canonical entry is returned with its
	// originID set, and an implicit edge is created below.
	canonical, _ := p.graph.Catalog.Resolve(res)

	if originID, hasOrigin := op.ExtractResource(canonical); hasOrigin {
		p.graph.Root.Edges = append(p.graph.Root.Edges, op.Edge{From: originID, To: node.ID()})
	}

	// Store the canonical typed resource in the slot. If the target type is a
	// value (not pointer), dereference.
	var slotValue any = canonical
	if !isPtr {
		rv := reflect.ValueOf(canonical)
		if rv.Kind() == reflect.Ptr {
			slotValue = rv.Elem().Interface()
		}
	}
	node.SetSlot(slotName, op.ImmediateValue{Value: slotValue})
	return nil
}

// endregion

// endregion
