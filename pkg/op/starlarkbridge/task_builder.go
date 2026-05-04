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
// ctx is the ambient execution context, used by assignTarget for [op.Convert] when arg-to-parameter conversion
// requires it. catalog is the session-scoped resource catalog, used by fillSlot via [op.ResourceCatalog.Link]
// for URI interning. Both are owned by plan.Provider and shared across every NodeBuilder it constructs.
type NodeBuilder struct {
	receiverType op.ProviderReceiverType
	ctx          *op.RuntimeEnvironment
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
//   - *NodeBuilder: the starlark-ready goReceiver.
func NewNodeBuilder(rt op.ProviderReceiverType, ctx *op.RuntimeEnvironment, catalog *op.ResourceCatalog, registry *InvocationRegistry) *NodeBuilder {

	methods := make(map[string]*op.Method)
	names := make([]string, 0)

	for method := range rt.Methods() {
		snake := op.CamelToSnake(method.Name())
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

// dispatch invokes a Starlark builtin to create a detached graph node for deferred execution.
//
// It registers the invocation in the session registry and returns it to the Starlark caller.
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
	// op.RuntimeEnvironment.ActionByName. Display-side concerns (error messages, auto-labels) use the builtin's label,
	// which reflects the receiver's placement (bare for root, dotted for non-root).
	actionName := p.receiverType.Name() + "." + name

	// Extract the reserved `options` kwarg before UnpackArgs sees it. Options is a cross-cutting per-invocation
	// concern (label + retry policy) reserved by the planner (D7). Method registration rejects any provider that
	// declares options as a parameter name, so it never collides with method-level kwargs.
	opts, kwargs, err := extractOptionsKwarg(kwargs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	// Single-pass classification: produce the slot sequence plus the scratch values slice and UnpackArgs pair list
	// in lockstep. Parameters arrive cooked (Name clean, Variadic / Kwargs flags set); no decoration trimming
	// needed at this layer.

	slots := make([]*op.Slot, 0, len(params))
	values := make([]starlark.Value, 0, len(params))
	pairs := make([]any, 0, len(params)*2)
	var kwargsSlot *op.Slot

	for _, param := range params {
		if param.Kwargs {
			kwargsSlot = &op.Slot{Parameter: param}
			continue
		}
		slots = append(slots, &op.Slot{Parameter: param})
		values = append(values, nil)
		// starlark.UnpackArgs uses a trailing "?" on the pair name to mark a kwarg optional. p.Name is clean post-
		// parseParameterToken; reconstruct the suffix here so UnpackArgs sees the optional convention.
		unpackName := param.Name
		if param.Optional {
			unpackName += "?"
		}
		pairs = append(pairs, unpackName, &values[len(values)-1])
	}

	// Split kwargs: known → UnpackArgs, unknown → **kwargs dict slots. The predicate scans the slot sequence —
	// fine at method-signature sizes, and avoids carrying a parallel lookup map.

	var filteredKwargs []starlark.Tuple
	var extraKwargs []starlark.Tuple

	if kwargsSlot != nil {
		for _, kv := range kwargs {
			key, _ := starlark.AsString(kv[0])
			known := false
			for _, slot := range slots {
				if slot.Parameter.Name == key {
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

	// Create node and bind the method so SetSlot can validate slot names against the parameter list.

	node := op.NewNode(op.GenerateNodeID(actionName))
	node.Bind(method)
	node.Receiver = actionName

	// Fill slots from starlark values via the unified fillSlot pipeline. Resource-typed targets are handled by
	// fillSlot's generic path (starlarkToGoTyped → toGo + op.Convert) with link-time catalog resolution applied
	// to any op.Resource result.

	for i, slot := range slots {

		sv := values[i]

		if sv == nil {
			// Truly absent kwarg — fill from the parameter's directive default if one exists. Default values
			// arrive at slot-fill time at the parameter's Go type exactly (parseDefaultExpression widens via
			// reflect.Value.Convert), so bypass fillSlot's starlark conversion and write op.ImmediateValue
			// directly. Explicit starlark.None has its own None-skip path through fillSlot below — defaults
			// never override caller intent.
			if slot.Parameter.Default != nil {
				node.SetSlot(slot.Parameter.Name, op.ImmediateValue{Value: slot.Parameter.Default})
			}
			continue
		}

		if err := p.fillSlot(node, slot, sv); err != nil {
			return nil, fmt.Errorf("%s: %w", slot.Parameter.Name, err)
		}
	}

	// Fill **kwargs as a single map slot matching the method's **kwargs parameter. The executing path (wrapper.go)
	// consumes this as one map[string]any argument; packing extras into a starlark.Dict here lets the dict unmarshaler
	// project into the parameter's map target.

	if kwargsSlot != nil && len(extraKwargs) > 0 {
		dict := starlark.NewDict(len(extraKwargs))
		for _, kv := range extraKwargs {
			if err := dict.SetKey(kv[0], kv[1]); err != nil {
				return nil, fmt.Errorf("%s(): kwargs: %w", label, err)
			}
		}
		if err := p.fillSlot(node, kwargsSlot, dict); err != nil {
			return nil, fmt.Errorf("%s: kwargs: %w", label, err)
		}
	}

	// Apply the retry policy from options (if supplied) to the node before it joins the graph.
	if opts != nil && opts.RetryPolicy != nil {
		node.Retry = opts.RetryPolicy
	}

	// Register an Invocation under the effective label.
	//
	// User-supplied Options.Label wins; otherwise the registry auto-labels as "<label>#<N>" where <label> is the
	// builtin's label form (bare for root providers, dotted otherwise — matching D7's label examples). Label collisions
	// fail plan-time.
	//
	// The node is NOT added to any graph here — Nodes are detached until plan.run materializes the graph from the
	// reachable invocation set (phase-8 D5). Producer→consumer edges live implicitly in each consumer node's slot
	// (PromiseValue's NodeRef + Resource producerIDs in ImmediateValue); plan.run extracts them at materialization.
	promise := NewPromise(node, "")

	invLabel := label
	if opts != nil && opts.Label != "" {
		invLabel = opts.Label
	} else {
		invLabel = p.registry.AutoLabel(label)
	}

	inv := &Invocation{Label: invLabel, Target: node, Promise: promise}
	if err := p.registry.Register(invLabel, inv); err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}

	return inv, nil
}

// extractOptionsKwarg scans kwargs for the "options" keyword and, if found, unwraps the value to an [Options] pointer.
//
// The reserved name is guarded at method registration ([newReceiverType] rejects any provider method that declares
// options as a parameter name), so this function cannot collide with a method-level kwarg. The unwrapped value flows
// into the dispatch site's invocation-registration and retry-application steps.
//
// Accepted value shapes for the kwarg:
//
//   - A *goReceiver around a *Options (produced by plan.options(...)). Returns the *Options.
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

		case *goReceiver:

			o, ok := v.instance.(*Options)
			if !ok {
				return nil, nil, fmt.Errorf("options: expected plan.options(...) result, got starlark receiver around %T", v.instance)
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
// Graph-edge dispatch (Invocation, list-of-Invocations) comes first: these mutate graph structure. NoneType skips
// the optional slot. Projector lets the starlark value choose its own Go projection. The generic path routes
// through [starlarkToGoTyped] — the same toGo + [op.Convert] pipeline immediate-mode dispatch reaches via
// [op.Method.Invoke] — covering registered Resource construction from primitives, slice/map element recursion,
// and the source/target converter opt-ins. After assignment, a Resource result is link-resolved against the
// session catalog and, if the linked entry carries a producer origin, an implicit edge is added from the
// producer to this node.
//
// The caller supplies slot.Parameter pre-populated. fillSlot fills the slot's value and installs it on the node via
// [op.Node.SetSlot].
//
// Parameters:
//   - node: the node whose slot is being filled.
//   - slot: the slot (Parameter pre-populated; Value will be filled).
//   - sv: the starlark value driving the fill.
//
// Returns:
//   - error: non-nil if the value cannot be assigned to the slot's target type.
func (p *NodeBuilder) fillSlot(node *op.Node, slot *op.Slot, sv starlark.Value) error {

	name := slot.Parameter.Name

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

	if inv, ok := sv.(*Invocation); ok {
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

	if list, ok := sv.(*starlark.List); ok {
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
				wantsUnit := slot.Parameter.Type.Kind() == reflect.Slice && executableUnitType.AssignableTo(slot.Parameter.Type.Elem())
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

	if _, ok := sv.(starlark.NoneType); ok {
		return nil
	}

	// Projector: extract the Go value the slot's parameter type wants, via the implementer's own projection
	// logic (op.Convert cascade for goReceiver, target-type switch for Promise/Invocation). Preserves identity and
	// producer through the planning layer without a starlark round-trip. If the extracted value carries a
	// producerID (resource produced by a prior node), the edge is implicit in the Resource stored in this node's
	// slot; plan.run extracts it at materialization (phase-8 D5).
	if proj, ok := sv.(Projector); ok {
		goVal, err := proj.Project(slot.Parameter.Type)
		if err != nil {
			return fmt.Errorf("slot %q: %w", name, err)
		}
		node.SetSlot(name, op.ImmediateValue{Value: goVal})
		return nil
	}

	// Generic path: route to the bridge's single starlark→typed-Go entry point. starlarkToGoTyped runs the
	// toGo unpacker (target=any) then the op.Convert cascade — covering assignability, ConvertibleTo,
	// source/target converter opt-ins, registered Resource construction (string → *file.Resource), and
	// slice/map element recursion. Identical to the path immediate-mode dispatch takes via Method.Invoke.

	final, err := starlarkToGoTyped(p.ctx, sv, slot.Parameter.Type)

	if err != nil {
		return fmt.Errorf("slot %q: %w", name, err)
	}

	// Resource-typed values intern against the session catalog so the consumer slot ends up holding the canonical
	// entry. Pointer slot targets store the linked Resource directly; value slot targets store the dereferenced
	// inner value so all holders observe the same instance. The producer→consumer edge is implicit in the linked
	// Resource's producerID (extractable via op.ExtractResource at plan.run materialization, phase-8 D5).

	if resource, ok := final.(op.Resource); ok {

		linked := p.catalog.Link(resource)

		if slot.Parameter.Type.Kind() == reflect.Pointer {
			final = linked
		} else {
			rv := reflect.ValueOf(linked)
			if rv.Kind() == reflect.Pointer {
				final = rv.Elem().Interface()
			} else {
				final = linked
			}
		}
	}

	node.SetSlot(name, op.ImmediateValue{Value: final})
	return nil
}

// endregion
