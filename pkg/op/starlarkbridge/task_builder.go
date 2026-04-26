// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
)

var (
	_ starlark.Value    = (*NodeBuilder)(nil) // Interface Guard: ensures *NodeBuilder implements starlark.Value.
	_ starlark.HasAttrs = (*NodeBuilder)(nil) // Interface Guard: ensures *NodeBuilder implements starlark.HasAttrs.
)

// executableUnitType is the [reflect.Type] of [op.ExecutableUnit], cached for fillSlot's target-type dispatch
// (phase-8 D2). When a slot's target type is assignable from ExecutableUnit, the slot receives the invocation's
// Target as an ImmediateValue (a unit reference); otherwise the slot receives a PromiseValue carrying the
// producer's NodeRef.
var executableUnitType = reflect.TypeFor[op.ExecutableUnit]()

// NodeBuilder wraps a provider's method signatures for plan-mode starlark use.
//
// It implements [starlark.Value] and [starlark.HasAttrs]. Each attribute access resolves a method on the underlying
// [op.ProviderReceiverType] and returns a [starlark.Builtin] bound to the plan adapter's dispatch method, which creates
// a graph node instead of executing the method directly.
//
// NodeBuilder is detached per phase-8 D5 — it does not hold a graph reference. Dispatch produces nodes that live on
// the returned [Invocation]; the [op.Graph] is materialized later by plan.run from the reachable invocation set.
//
// registry is shared across every NodeBuilder that participates in the same planning session. Dispatch registers
// one [Invocation] per method call under the effective label (user-supplied via [Options.Label] or auto-labeled
// via [InvocationRegistry.AutoLabel]).
//
// ctx is the ambient execution context (used by assignTarget for [op.Convert] and by shadowPendingOutput for
// provider construction). catalog is the session-scoped resource catalog (used by fillSlot via
// [op.ResourceCatalog.Link] for URI interning and by shadowPendingOutput for plan-time output shadowing). Both are
// owned by plan.Provider and shared across every NodeBuilder it constructs.
type NodeBuilder struct {
	receiverType op.ProviderReceiverType
	ctx          *op.ExecutionContext
	catalog      *op.ResourceCatalog
	registry     *InvocationRegistry
	methods      map[string]*op.Method // snake_name → *Method
	attrNames    []string              // sorted
}

// NewNodeBuilder creates a detached [NodeBuilder] for the given receiver type.
//
// Parameters:
//   - rt: the provider receiver type descriptor.
//   - ctx: the ambient execution context (for op.Convert, provider construction).
//   - catalog: the session-scoped resource catalog (for URI interning and plan-time output shadowing).
//   - registry: the shared invocation registry; every NodeBuilder in a planning session uses the same registry
//     so labels are session-unique and orphan detection can walk the full set.
//
// Returns:
//   - *NodeBuilder: the starlark-ready wrapper.
func NewNodeBuilder(rt op.ProviderReceiverType, ctx *op.ExecutionContext, catalog *op.ResourceCatalog, registry *InvocationRegistry) *NodeBuilder {

	methods := make(map[string]*op.Method)
	names := make([]string, 0)

	for method := range rt.Methods() {
		snake := camelToSnake(method.Name())
		methods[snake] = method
		names = append(names, snake)
	}

	sort.Strings(names)

	return &NodeBuilder{
		receiverType: rt,
		ctx:          ctx,
		catalog:      catalog,
		registry:     registry,
		methods:      methods,
		attrNames:    names,
	}
}

// region EXPORTED METHODS

// region State management

// String implements starlark.Value.
func (p *NodeBuilder) String() string { return "plan." + p.receiverType.Name() }

// Type implements starlark.Value.
func (p *NodeBuilder) Type() string { return "plan." + p.receiverType.Name() }

// Freeze implements starlark.Value.
func (p *NodeBuilder) Freeze() {}

// Truth implements starlark.Value.
func (p *NodeBuilder) Truth() starlark.Bool { return true }

// Hash implements starlark.Value.
func (p *NodeBuilder) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: plan.%s", p.receiverType.Name())
}

// endregion

// region Behaviors

// Attr implements starlark.HasAttrs.
//
// The builtin's label form depends on the receiver type's placement. Root providers (those with the RoleRoot placement
// bit; see D12) surface their methods flat at the plan namespace root and receive bare-name labels (e.g., "choose").
// Non-root providers keep the qualified "<provider>.<method>" form (e.g., "file.write_text"). The label is for display
// only — dispatch recovers the method by short name regardless of label form, and the executor resolves nodes via the
// always-dotted Node.Receiver written by dispatch.
//
// Parameters:
//   - name: the snake_case attribute name to look up.
//
// Returns:
//   - starlark.Value: a builtin bound to this plan adapter's dispatch method.
//   - error: non-nil if the attribute does not exist.
func (p *NodeBuilder) Attr(name string) (starlark.Value, error) {

	if _, ok := p.methods[name]; !ok {
		return nil, NoSuchAttrError("plan."+p.receiverType.Name(), name)
	}

	label := name
	if p.receiverType.Roles().Placement()&op.RoleRoot == 0 {
		label = p.receiverType.Name() + "." + name
	}

	return starlark.NewBuiltin(label, p.dispatch), nil
}

// AttrNames implements starlark.HasAttrs.
//
// Returns:
//   - []string: sorted list of available method names.
func (p *NodeBuilder) AttrNames() []string { return p.attrNames }

// endregion

// endregion

// region UNEXPORTED METHODS

// dispatch dispatches a starlark builtin invocation to create a detached graph node for deferred execution,
// registers the resulting invocation in the session registry, and returns the invocation to the starlark caller.
//
// Parameters:
//   - thread: the starlark thread.
//   - builtin: the starlark builtin that triggered the dispatch.
//   - args: positional starlark arguments.
//   - kwargs: keyword starlark arguments.
//
// Returns:
//   - starlark.Value: the newly-registered *Invocation, or an error value if dispatch fails.
//   - error: non-nil if node creation, slot filling, planned shadowing, or registration fails.
func (p *NodeBuilder) dispatch(_ *starlark.Thread, builtin *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	// The builtin's label may be bare (root providers per D12) or dotted ("<provider>.<method>"). Recover the method
	// name either way by taking the substring after the last dot — for bare labels, strings.LastIndex returns -1 and
	// the slice trims to the whole string. The always-dotted form computed below is what the executor uses to resolve
	// the node at execute time.
	label := builtin.Name()
	name := label[strings.LastIndex(label, ".")+1:]
	method := p.methods[name]
	params := method.Parameters()

	// actionName is the always-dotted "<provider>.<method>" form written onto the node for execute-time lookup via
	// op.ExecutionContext.ActionByName. Display-side concerns (error messages, auto-labels) use the builtin's label,
	// which reflects the receiver's placement (bare for root, dotted for non-root).
	actionName := p.receiverType.Name() + "." + name

	// Extract the reserved `options` kwarg before UnpackArgs sees it. Options is a cross-cutting per-invocation
	// concern (label + retry policy) reserved by the planner (D7). Method registration rejects any provider that
	// declares options as a parameter name, so it never collides with method-level kwargs.
	opts, kwargs, err := extractOptionsKwarg(kwargs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

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

	if err := starlark.UnpackArgs(label, args, filteredKwargs, pairs...); err != nil {
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
				return nil, fmt.Errorf("%s: kwargs: %w", label, err)
			}
		}
		if err := p.fillSlot(node, kwargsSlot, dict); err != nil {
			return nil, fmt.Errorf("%s: kwargs: %w", label, err)
		}
	}

	// Plan-time output shadowing. If this method has a Planned companion, call it with the filled slot values to
	// compute the pending resource the node will produce, and shadow it in the catalog. If the companion returns
	// KnownAtExecution, skip plan-time shadowing — the executor will shadow the real return value post-dispatch.
	if method.HasPlanned() {
		if err := p.shadowPendingOutput(node, method); err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
	}

	// Apply the retry policy from options (if supplied) to the node before it joins the graph.
	if opts != nil && opts.RetryPolicy != nil {
		node.Retry = opts.RetryPolicy
	}

	// Register an Invocation under the effective label. User-supplied Options.Label wins; otherwise the registry
	// auto-labels as "<label>#<N>" where <label> is the builtin's label form (bare for root receivers, dotted
	// otherwise — matching D7's label examples). Label collisions fail plan-time.
	//
	// The node is NOT added to any graph here — Nodes are detached until plan.run materializes the graph from the
	// reachable invocation set (phase-8 D5). Producer→consumer edges live implicitly in each consumer node's slot
	// (PromiseValue's NodeRef + Resource originIDs in ImmediateValue); plan.run extracts them at materialization.
	promise := NewPromise(node, "")

	invLabel := label
	if opts != nil && opts.Label != "" {
		invLabel = opts.Label
	} else {
		invLabel = p.registry.AutoLabel(label)
	}

	inv := &Invocation{Label: invLabel, Target: node, Result: promise}
	if err := p.registry.Register(invLabel, inv); err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	return inv, nil
}

// extractOptionsKwarg scans kwargs for the reserved "options" key and, if found, removes it and unwraps the value
// to a *Options.
//
// The reserved name is guarded at method registration ([newReceiverType] rejects any provider method that declares
// options as a parameter name), so this function cannot collide with a method-level kwarg. The unwrapped value
// flows into the dispatch site's invocation-registration and retry-application steps.
//
// Accepted value shapes for the kwarg:
//
//   - A *receiver wrapping a *Options (produced by plan.options(...)). Returns the *Options.
//   - starlark.None. Returns nil *Options; treated as "no options."
//   - Anything else. Returns a descriptive error.
//
// Parameters:
//   - kwargs: the caller-supplied keyword arguments.
//
// Returns:
//   - *Options: the unwrapped options, or nil if the kwarg was absent or None.
//   - []starlark.Tuple: kwargs with the "options" entry removed.
//   - error: non-nil if the "options" value is of an unexpected type.
func extractOptionsKwarg(kwargs []starlark.Tuple) (*Options, []starlark.Tuple, error) {

	for i, kv := range kwargs {

		key, _ := starlark.AsString(kv[0])
		if key != "options" {
			continue
		}

		var opts *Options

		switch v := kv[1].(type) {

		case *receiver:
			o, ok := v.instance.(*Options)
			if !ok {
				return nil, nil, fmt.Errorf("options: expected *Options (from plan.options(...)), got %T", v.instance)
			}
			opts = o

		case starlark.NoneType:
			// explicit None — treated as no options

		default:
			return nil, nil, fmt.Errorf("options: expected value from plan.options(...), got %s", kv[1].Type())
		}

		filtered := make([]starlark.Tuple, 0, len(kwargs)-1)
		filtered = append(filtered, kwargs[:i]...)
		filtered = append(filtered, kwargs[i+1:]...)
		return opts, filtered, nil
	}

	return nil, kwargs, nil
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
func (p *NodeBuilder) fillSlot(node *op.Node, slot *op.Slot, value starlark.Value) error {

	name := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(slot.Parameter.Name, "**"), "*"), "?")

	// Invocation: the value returned by every plan.* call. The slot's target type determines whether the consumer
	// wants the unit reference itself (Target) or a value-side promise (Result), per phase-8 D2:
	//
	//   target type assignable from op.ExecutableUnit  → ImmediateValue{inv.Target}  (no promise; unit reference)
	//   any other target type                          → PromiseValue via Result      (value flows at execute time)
	//
	// The unit-reference path is what container methods (subgraph, choose branches, gather body, wait_until
	// predicate) consume: their parameter type is op.ExecutableUnit, and they need the structural reference, not a
	// resolved value. Everything else stays on the PromiseValue path so the consumer's slot encodes the
	// producer→consumer edge for plan.run to materialize.

	if inv, ok := value.(*Invocation); ok {
		if executableUnitType.AssignableTo(slot.Parameter.Type) {
			node.SetSlot(name, op.ImmediateValue{Value: inv.Target})
		} else {
			inv.FillSlot(node, name)
		}
		return nil
	}

	// List of Invocations: fan-in via indexed sub-slots. Each element contributes its own slot value ("<name>[i]")
	// chosen by the same target-type rule as the scalar path above; the slot's element type drives the choice when
	// the target is a slice. "<name>.len" holds the count. plan.run flattens these at materialization.

	if list, ok := value.(*starlark.List); ok {
		if n := list.Len(); n > 0 {
			invocations := make([]*Invocation, n)
			allInvocations := true
			for i := range n {
				inv, ok := list.Index(i).(*Invocation)
				if !ok {
					allInvocations = false
					break
				}
				invocations[i] = inv
			}
			if allInvocations {
				wantsUnit := slot.Parameter.Type.Kind() == reflect.Slice &&
					executableUnitType.AssignableTo(slot.Parameter.Type.Elem())
				for i, inv := range invocations {
					subSlot := fmt.Sprintf("%s[%d]", name, i)
					if wantsUnit {
						node.SetSlot(subSlot, op.ImmediateValue{Value: inv.Target})
					} else {
						inv.FillSlot(node, subSlot)
					}
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
	// Preserves identity and origin through the planning layer without a marshal→unmarshal round-trip. If the
	// extracted value carries an originID (resource produced by a prior node), the edge is implicit in the
	// Resource stored in this node's slot; plan.run extracts it at materialization (phase-8 D5).
	if r, ok := value.(*receiver); ok {
		goVal := r.instance
		node.SetSlot(name, op.ImmediateValue{Value: goVal})
		return nil
	}

	// Generic path: unmarshal into target type.
	//
	// On mismatch, fall back to unmarshal-into-any + op.Convert so registry-based target instantiation (e.g., string →
	// *file.Resource) takes over.

	final, err := p.assignTarget(slot.Parameter, value)

	if err != nil {
		return fmt.Errorf("slot %q: %w", name, err)
	}

	// Resource-typed values intern against the session catalog so the consumer slot ends up holding the canonical
	// entry. Pointer slot targets store the linked Resource directly; value slot targets store the dereferenced
	// inner value so all holders observe the same instance. The producer→consumer edge is implicit in the linked
	// Resource's originID (extractable via op.ExtractResource at plan.run materialization, phase-8 D5).

	if resource, ok := final.(op.Resource); ok {

		linked := p.catalog.Link(resource)

		if slot.Parameter.Type.Kind() == reflect.Ptr {
			final = linked
		} else {
			rv := reflect.ValueOf(linked)
			if rv.Kind() == reflect.Ptr {
				final = rv.Elem().Interface()
			} else {
				final = linked
			}
		}
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
func (p *NodeBuilder) shadowPendingOutput(node *op.Node, method *op.Method) error {

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

	receiver, err := p.receiverType.Construct()(p.ctx)

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

	if _, err := p.catalog.Shadow(pending, node.ID()); err != nil {
		return fmt.Errorf("shadow output: %w", err)
	}

	return nil
}

// assignTarget converts a starlark value into the Go type declared by parameter.Type.
//
// When the target type implements [Unmarshaler], a fresh zero instance is allocated and UnmarshalStarlark is
// invoked on it. Otherwise, the starlark value is unpacked to its natural Go equivalent and aligned with the
// target type using Go's conversion rules (reflect.Value.AssignableTo → reflect.Value.ConvertibleTo). Unpacking
// a list/dict/tuple recurses into any-typed slots, so collections naturally unpack as []any or map[any]any; the
// trailing alignment succeeds only when the target type accepts that shape per the Go spec. parameter.Name is
// threaded into error messages so callers see which argument failed.
func (p *NodeBuilder) assignTarget(parameter op.Parameter, value starlark.Value) (any, error) {

	target := parameter.Type

	if target.Implements(reflect.TypeFor[Unmarshaler]()) {

		v := reflect.New(target.Elem()).Interface().(Unmarshaler)

		if err := v.UnmarshalStarlark(value); err != nil {
			return nil, fmt.Errorf("%s: %w", parameter.Name, err)
		}

		return v, nil
	}

	anyType := reflect.TypeFor[any]()

	var unpacked any

	switch v := value.(type) {

	case starlark.Bool:

		unpacked = bool(v)

	case starlark.Bytes:

		unpacked = []byte(v)

	case *starlark.Dict:

		m := make(map[any]any, v.Len())

		for _, item := range v.Items() {

			key, err := p.assignTarget(op.Parameter{Name: parameter.Name + " key", Type: anyType}, item[0])
			if err != nil {
				return nil, err
			}

			val, err := p.assignTarget(op.Parameter{Name: fmt.Sprintf("%s[%v]", parameter.Name, key), Type: anyType}, item[1])
			if err != nil {
				return nil, err
			}

			m[key] = val
		}

		unpacked = m

	case starlark.Float:

		unpacked = float64(v)

	case *starlark.Function:

		unpacked = v

	case starlark.Int:

		i, ok := v.Int64()
		if !ok {
			return nil, fmt.Errorf("%s: int overflow: %s", parameter.Name, v)
		}
		unpacked = i

	case *starlark.List:

		n := v.Len()
		result := make([]any, n)

		for i := range n {

			elem, err := p.assignTarget(op.Parameter{Name: fmt.Sprintf("%s[%d]", parameter.Name, i), Type: anyType}, v.Index(i))
			if err != nil {
				return nil, err
			}

			result[i] = elem
		}

		unpacked = result

	case starlark.NoneType:

		return nil, nil

	case *starlark.Set:

		result := make([]any, 0, v.Len())
		iter := v.Iterate()
		defer iter.Done()

		var item starlark.Value
		for i := 0; iter.Next(&item); i++ {

			elem, err := p.assignTarget(op.Parameter{Name: fmt.Sprintf("%s[%d]", parameter.Name, i), Type: anyType}, item)
			if err != nil {
				return nil, err
			}

			result = append(result, elem)
		}

		unpacked = result

	case starlark.String:

		unpacked = string(v)

	case starlark.Tuple:

		n := v.Len()
		result := make([]any, n)

		for i := range n {

			elem, err := p.assignTarget(op.Parameter{Name: fmt.Sprintf("%s[%d]", parameter.Name, i), Type: anyType}, v.Index(i))
			if err != nil {
				return nil, err
			}

			result[i] = elem
		}

		unpacked = result

	default:

		return nil, fmt.Errorf("%s: cannot convert starlark %s to %s", parameter.Name, value.Type(), target)
	}

	rv := reflect.ValueOf(unpacked)

	if rv.Type().AssignableTo(target) {
		return rv.Interface(), nil
	}

	if rv.Type().ConvertibleTo(target) {
		return rv.Convert(target).Interface(), nil
	}

	return nil, fmt.Errorf("%s: cannot convert %s to %s", parameter.Name, rv.Type(), target)
}

// endregion
