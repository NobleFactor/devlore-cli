// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
)

// executableUnitType caches the reflect.Type of [ExecutableUnit] for [Planner] implementations that need
// to decide between ImmediateBinding (unit reference) and PromiseBinding (value-side output) at slot-fill time.
var executableUnitType = reflect.TypeFor[ExecutableUnit]()

// Plan runs a planning session bounded by spec and fn.
//
// The session-shape is:
//
//  1. Build a planning [RuntimeEnvironment] from spec, minting its [fsroot.Dir] from the spec's anchor.
//  2. Call fn with the runtime environment; the caller drives planning (loading a starlark script,
//     calling plan.assemble_definition, etc.) and returns the assembled [*Graph] (or nil if the script did not
//     assemble a graph).
//  3. Close the planning runtime environment.
//
// Step 3 fires via defer, so a panic inside fn still leaves the runtime environment closed. The returned
// [*Graph] is immutable and holds no reference to the planning environment; the next session-owner
// (typically a [GraphExecutor]) executes it under a fresh environment of its own.
//
// Parameters:
//   - `ctx`: the parent context whose cancellation / values flow into the planning runtime environment.
//   - `spec`: the planning-environment configuration.
//   - `fn`: the caller-supplied planning routine; receives the runtime environment and returns the
//     assembled graph.
//
// Returns:
//   - *Graph: the assembled graph (nil if fn did not assemble one).
//   - `error`: non-nil if the planning runtime environment cannot be built, fn returned an error, or the
//     planning runtime environment's [RuntimeEnvironment.Close] failed.
func Plan(
	ctx context.Context,
	spec *RuntimeEnvironmentSpec,
	fn func(*RuntimeEnvironment) (*Graph, error),
) (graph *Graph, err error) {

	runtimeEnvironment, buildErr := NewRuntimeEnvironment(ctx, spec)
	if buildErr != nil {
		return nil, fmt.Errorf("op.Plan: build runtime environment: %w", buildErr)
	}
	defer iox.Close(&err, runtimeEnvironment)

	graph, err = fn(runtimeEnvironment)
	if err != nil {
		return nil, err
	}

	return graph, nil
}

// PlanInvocator is the contract a [Planner] consumes to reach plan-time session state.
//
// plan.Provider satisfies this interface.
type PlanInvocator interface {

	// InvocationRegistry returns the session-scoped ledger of constructed invocations.
	//
	// Returns:
	//   - *InvocationRegistry: the session ledger; never nil during planning.
	InvocationRegistry() *InvocationRegistry

	// RuntimeEnvironment returns the session environment the planner uses for plan-time [Convert] calls that
	// resolve immediate arguments to their parameter types (e.g. a string to a *file.Resource).
	//
	// Returns:
	//   - *RuntimeEnvironment: the session environment; never nil during planning.
	RuntimeEnvironment() *RuntimeEnvironment
}

// Planner builds an [ExecutableUnit] for one plan-mode method call.
//
// Each [*Method] in the receiver registry carries a Planner — either the default [ActionPlanner] or a
// specialized planner named by reflect.Type in the method's announcement. plan.Provider.Invocation
// delegates the structural shape of the call to the method's planner; plan.Provider then stamps Label /
// RetryPolicy / OnError on the returned unit, wraps it in an [Invocation], and registers it.
//
// Planners are stateless and constructed once per planner type at announcement time.
type Planner interface {

	// Plan builds the [ExecutableUnit] for one plan-mode method call.
	//
	// The unit's slots are filled from `args` / `kwargs` against the method's declared parameters; declared defaults
	// fill any parameter the call omits; `onError` / `onRetry` / `retryPolicy` / `transitionPolicy` are stamped at
	// construction. A required parameter (non-optional, no default) with no value is an error. Implementations leave
	// Label unset — the caller stamps it when wrapping the unit in an [Invocation] and registering it. [ActionPlanner]
	// is the default implementation.
	//
	// Parameters:
	//   - `invocator`: the planning host; supplies the session [*InvocationRegistry] and the [*RuntimeEnvironment] for
	//     plan-time [Convert] calls.
	//   - `receiverType`: the planning provider whose method is being called; must be non-nil.
	//   - `method`: the registered method descriptor; must be non-nil.
	//   - `args`: positional arguments, already converted starlark → Go, in call order.
	//   - `kwargs`: keyword arguments by parameter name, already converted (reserved entries removed).
	//   - `annotations`: tool-specific annotations stamped onto the unit; nil for none.
	//   - `onError`: the failure-handler [*Subgraph] stamped onto the unit, or nil.
	//   - `onRetry`: the retry-handler [*Subgraph] stamped onto the unit, or nil.
	//   - `retryPolicy`: the [*RetryPolicy] stamped onto the unit, or nil.
	//   - `transitionPolicy`: the [*TransitionPolicy] stamped onto the unit, or nil.
	//
	// Returns:
	//   - `ExecutableUnit`: the assembled unit with `onError` / `onRetry` / `retryPolicy` / `transitionPolicy`
	//     applied and Label unset.
	//   - `error`: non-nil on a missing required parameter, a slot-value projection failure, or unit construction error.
	Plan(
		invocator PlanInvocator,
		receiverType ProviderReceiverType,
		method *Method,
		args []any,
		kwargs map[string]any,
		annotations map[string]any,
		onError *Subgraph,
		onRetry *Subgraph,
		retryPolicy *RetryPolicy,
		transitionPolicy *TransitionPolicy,
	) (ExecutableUnit, error)
}

// plannerForType resolves a reflect.Type declared in [MethodMetadata.Planner] to its singleton [Planner]
// instance. Nil yields the default [ActionPlanner]. Handles both value-receiver and pointer-receiver
// planner implementations: tries the zero-value-of-`t` shape first (value-receiver methods), then the
// pointer-to-zero-value shape (pointer-receiver methods).
//
// Parameters:
//   - `t`: the planner type declared in announcement metadata, or nil for the default planner.
//
// Returns:
//   - Planner: the resolved planner instance.
func plannerForType(t reflect.Type) Planner {

	if t == nil {
		return ActionPlanner{}
	}

	val := reflect.New(t).Elem().Interface()
	if p, ok := val.(Planner); ok {
		return p
	}

	val = reflect.New(t).Interface()
	if p, ok := val.(Planner); ok {
		return p
	}

	assert.Failf("op.plannerForType: %s does not implement Planner", t)
	return nil
}

// ActionPlanner is the default vanilla planner — one starlark call produces one leaf [*Node].
type ActionPlanner struct{}

// region EXPORTED METHODS

// region Behaviors

// Plan builds the leaf [*Node] for one vanilla `plan.<provider>.<method>(...)` call.
//
// The action name is `<receiverType.Name>.<snake(method.Name)>`; the node binds a resolved [Action] built from
// `receiverType` + `method` directly (the planner holds both, so no by-name deferral). Every field is gathered into a
// [NodeSpec] and the node is constructed once via [NewNode] — no post-construction mutation (the graph-immutability
// seal).
//
// Slot fill walks the method's declared parameters in order, taking each value positionally from `args` first, then by
// name from `kwargs`. A parameter the call omits takes its declared default when one exists; a required parameter
// (non-optional, no default) with no value is an error. Each value is projected to the [Binding] variant matching
// the argument kind:
//
//   - variadic parameter — the remaining positional `args`, as an [ImmediateBinding] slice.
//   - kwargs parameter — the still-unconsumed `kwargs`, as an [ImmediateBinding] map.
//   - [*Invocation] — the referenced unit itself ([ImmediateBinding] of its `Target`) when the parameter type is
//     [ExecutableUnit]-assignable; otherwise the invocation's value-side output (a [PromiseBinding]).
//   - [*Variable] — a [VariableBinding], which bubbles up as a caller-supplied graph parameter.
//   - [Resource], content-addressed — validated against [SourceConverter] now, conversion deferred to runtime
//     ([ImmediateBinding] of the resource); location-addressed — converted now via [Convert].
//   - any other value — converted toward the parameter type now via [Convert] ([ImmediateBinding]).
//
// Parameters:
//   - `invocator`: the planning host; supplies the session [*InvocationRegistry] and the [*RuntimeEnvironment] for
//     plan-time [Convert] calls.
//   - `receiverType`: the planning provider whose method is being called; must be non-nil.
//   - `method`: the registered method descriptor; must be non-nil.
//   - `args`: positional arguments, already converted starlark → Go, in call order.
//   - `kwargs`: keyword arguments by parameter name, already converted (reserved entries removed).
//   - `annotations`: tool-specific annotations stamped onto the unit; nil for none.
//   - `onError`: the failure-handler [*Subgraph] stamped onto the unit, or nil.
//   - `onRetry`: the retry-handler [*Subgraph] stamped onto the unit, or nil.
//   - `retryPolicy`: the [*RetryPolicy] stamped onto the unit, or nil.
//   - `transitionPolicy`: the [*TransitionPolicy] stamped onto the unit, or nil.
//
// Returns:
//   - `ExecutableUnit`: the sealed [*Node] with `onError` / `onRetry` / `retryPolicy` / `transitionPolicy` applied
//     and Label unset.
//   - `error`: non-nil on nil `receiverType` / `method`, a missing required parameter, or a slot-value conversion
//     failure.
func (ActionPlanner) Plan(
	invocator PlanInvocator,
	receiverType ProviderReceiverType,
	method *Method,
	args []any,
	kwargs map[string]any,
	annotations map[string]any,
	onError *Subgraph,
	onRetry *Subgraph,
	retryPolicy *RetryPolicy,
	transitionPolicy *TransitionPolicy,
) (ExecutableUnit, error) {

	if receiverType == nil {
		return nil, fmt.Errorf("op.ActionPlanner.Plan: nil receiverType")
	}
	if method == nil {
		return nil, fmt.Errorf("op.ActionPlanner.Plan: nil method")
	}

	actionName := receiverType.Name() + "." + CamelToSnake(method.Name())

	// Gather every field into the spec, then construct once — no post-construction node mutation.

	spec := NewNodeSpec().
		WithID(GenerateNodeID(actionName)).
		WithAction(NewAction(receiverType, method, ActionName(actionName))).
		WithAnnotations(annotations).
		WithOnError(onError).
		WithOnRetry(onRetry).
		WithRetryPolicy(retryPolicy).
		WithTransitionPolicy(transitionPolicy)

	if err := bindParameterSlots(invocator, spec, actionName, method.Parameters(), args, kwargs); err != nil {
		return nil, err
	}

	return NewNode(spec)
}

// bindParameterSlots binds the call's positional and keyword arguments to the method's parameters:
// the variadic sink absorbs positional overflow, the kwargs sink the unconsumed keywords, defaults
// fill absences, a missing required parameter is a plan error, and every supplied value routes
// through [bindPresentValue].
//
// Parameters:
//   - `invocator`: the planning invocator, for the runtime environment.
//   - `spec`: the node spec under construction.
//   - `actionName`: the action name, for error messages.
//   - `params`: the method's declared parameters.
//   - `args`: the positional arguments.
//   - `kwargs`: the keyword arguments.
//
// Returns:
//   - `error`: non-nil for a missing required parameter or a binding failure.
func bindParameterSlots(
	invocator PlanInvocator, spec *NodeSpec, actionName string, params []Parameter, args []any, kwargs map[string]any,
) error {

	consumed := make(map[string]bool, len(kwargs))
	positional := 0

	for _, param := range params {

		if param.Variadic {
			var rest []any
			rest, positional = positionalRest(args, positional)
			spec.WithSlot(param.Name, NewImmediateBinding(rest))
			continue
		}

		if param.Kwargs {
			spec.WithSlot(param.Name, NewImmediateBinding(remainingKwargs(kwargs, consumed)))
			continue
		}

		var value any
		var present bool

		if positional < len(args) {
			value = args[positional]
			positional++
			present = true
		} else if v, ok := kwargs[param.Name]; ok {
			value = v
			consumed[param.Name] = true
			present = true
		}

		if !present {
			if param.Default != nil {
				spec.WithSlot(param.Name, NewImmediateBinding(param.Default))
				continue
			}
			if !param.Optional {
				return fmt.Errorf("op.ActionPlanner.Plan: %s: missing required parameter %q", actionName, param.Name)
			}
			continue
		}

		if err := bindPresentValue(invocator, spec, actionName, param, value); err != nil {
			return err
		}
	}

	return nil
}

// positionalRest collects the positional overflow from `positional` onward.
//
// Parameters:
//   - `args`: the positional arguments.
//   - `positional`: the first unconsumed index.
//
// Returns:
//   - `[]any`: the overflow, in order.
//   - `int`: the advanced positional index (len(args)).
func positionalRest(args []any, positional int) (rest []any, next int) {

	rest = make([]any, 0, max(0, len(args)-positional))
	for ; positional < len(args); positional++ {
		rest = append(rest, args[positional])
	}

	return rest, positional
}

// remainingKwargs collects the keywords not yet consumed by named parameters.
//
// Parameters:
//   - `kwargs`: the keyword arguments.
//   - `consumed`: the consumed-key set.
//
// Returns:
//   - `map[string]any`: the remaining keywords.
func remainingKwargs(kwargs map[string]any, consumed map[string]bool) map[string]any {

	remaining := make(map[string]any, len(kwargs))
	for k, v := range kwargs {
		if !consumed[k] {
			remaining[k] = v
		}
	}

	return remaining
}

// bindPresentValue binds one supplied argument value into the parameter's slot: an Invocation binds
// its promise (or its target unit for ExecutableUnit-typed parameters), a Variable binds by name, and
// everything else routes through resource construction and the addressing rules.
//
// Parameters:
//   - `invocator`: the planning invocator, for the runtime environment.
//   - `spec`: the node spec under construction.
//   - `actionName`: the action name, for error messages.
//   - `param`: the parameter being bound.
//   - `value`: the supplied value.
//
// Returns:
//   - `error`: non-nil on a construction or conversion failure.
func bindPresentValue(invocator PlanInvocator, spec *NodeSpec, actionName string, param Parameter, value any) error {

	switch v := value.(type) {
	case *Invocation:

		if param.Type != nil && executableUnitType.AssignableTo(param.Type) {
			spec.WithSlot(param.Name, NewImmediateBinding(v.Target))
		} else {
			spec.WithSlot(param.Name, v.Binding())
		}

	case *Variable:

		spec.WithSlot(param.Name, NewVariableBindingWithField(v.Name, v.Field))

	default:

		// An authored string cannot bind an interface-typed resource slot (4-resource-management.md §5.7
		// rule 6, ruled 2026-08-22): a claim asserts a kind — "claims are true when made" needs a kind to
		// be true about — and an interface asserts none. The author states the kind, or feeds a
		// discovery's promise; mechanically, plan-time claiming constructs the parameter's type, and an
		// interface cannot be instantiated.
		if param.Type != nil && param.Type.Kind() == reflect.Interface && param.Type.Implements(resourceInterfaceType) {
			if _, isString := value.(string); isString {
				return fmt.Errorf(
					"op.ActionPlanner.Plan: %s: param %q: an authored string cannot bind the interface-typed "+
						"resource slot %s — a claim asserts a kind and an interface asserts none; state the kind, "+
						"or feed a discovery (4-resource-management.md §5.7)",
					actionName, param.Name, param.Type)
			}
		}

		value, err := normalizePlanSpaceValue(actionName, param, value)
		if err != nil {
			return err
		}

		// A builtin value may be the Go form of a content resource (e.g. a *starlark.Function). The provider
		// keys its constructor by the source type; if the registry has one, build the resource here so the
		// addressing switch below takes over — naming only op + reflect, never the provider.
		if _, isResource := value.(Resource); !isResource {
			if construct, ok := ReceiverRegistry().ConstructorForSource(reflect.TypeOf(value)); ok {
				built, err := construct(invocator.RuntimeEnvironment(), value)
				if err != nil {
					return fmt.Errorf("op.ActionPlanner.Plan: %s: param %q: %w", actionName, param.Name, err)
				}
				value = built
			}
		}

		if r, ok := value.(Resource); ok {
			return bindResourceValue(invocator, spec, actionName, param, r)
		}

		// A plain value: convert toward the parameter type now.

		converted, err := Convert(invocator.RuntimeEnvironment(), value, param.Type)
		if err != nil {
			return fmt.Errorf("op.ActionPlanner.Plan: %s: param %q: %w", actionName, param.Name, err)
		}

		spec.WithSlot(param.Name, NewImmediateBinding(converted))
	}

	return nil
}

// normalizePlanSpaceValue renders an authored string bound to a resource-typed parameter through the
// owning scheme's plan-space grammar — the claiming seam of the little language (§5.2).
//
// Non-string values and parameter types without a registered normalizer pass through untouched: the grammar
// governs what a plan may say, not what a session may do.
//
// Parameters:
//   - `actionName`: the action name, for error messages.
//   - `param`: the parameter being bound.
//   - `value`: the supplied value.
//
// Returns:
//   - `any`: the canonical value (normalized when the grammar applied; the input otherwise).
//   - `error`: non-nil when the grammar refuses the authored path.
func normalizePlanSpaceValue(actionName string, param Parameter, value any) (any, error) {

	s, isString := value.(string)
	if !isString || param.Type == nil {
		return value, nil
	}

	normalize, ok := planPathNormalizerFor(param.Type)
	if !ok {
		return value, nil
	}

	normalized, err := normalize(s)
	if err != nil {
		return nil, fmt.Errorf("op.ActionPlanner.Plan: %s: param %q: %w", actionName, param.Name, err)
	}

	return normalized, nil
}

// bindResourceValue binds a resource-valued argument per its addressing.
//
// Content addressing validates the conversion now and defers it to runtime (e.g. function.Resource →
// Go function pointer, deferred because pointers are ephemeral); location addressing converts
// immediately (e.g. file.Resource → string, immediate because path strings are serializable and
// stable).
//
// Parameters:
//   - `invocator`: the planning invocator, for the runtime environment.
//   - `spec`: the node spec under construction.
//   - `actionName`: the action name, for error messages.
//   - `param`: the parameter being bound.
//   - `r`: the resource value.
//
// Returns:
//   - `error`: non-nil when the resource cannot reach the parameter type.
func bindResourceValue(invocator PlanInvocator, spec *NodeSpec, actionName string, param Parameter, r Resource) error {

	switch r.Addressing() {

	case AddressingContent:

		sc, ok := r.(SourceConverter)
		if !ok || !sc.CanConvertTo(param.Type) {
			return fmt.Errorf("op.ActionPlanner.Plan: %s: param %q: %T has no conversion to %s",
				actionName, param.Name, r, param.Type)
		}

		spec.WithSlot(param.Name, NewImmediateBinding(r))

	case AddressingLocation:

		converted, err := Convert(invocator.RuntimeEnvironment(), r, param.Type)
		if err != nil {
			return fmt.Errorf("op.ActionPlanner.Plan: %s: param %q: %w", actionName, param.Name, err)
		}

		spec.WithSlot(param.Name, NewImmediateBinding(converted))

	default:
		assert.Unreachablef(
			"op.ActionPlanner.Plan: %s: param %q: resource %T has addressing %v; "+
				"want AddressingContent or AddressingLocation",
			actionName,
			param.Name,
			r,
			r.Addressing())
	}

	return nil
}

// endregion

// endregion
