// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// executableUnitType caches the reflect.Type of [ExecutableUnit] for [Planner] implementations that need
// to decide between ImmediateValue (unit reference) and PromiseValue (value-side output) at slot-fill time.
var executableUnitType = reflect.TypeFor[ExecutableUnit]()

// Plan runs a planning session bounded by spec and fn.
//
// The session-shape is:
//
//  1. Build a planning [RuntimeEnvironment] from spec.
//  2. Call fn with the runtime environment; the caller drives planning (loading a starlark script,
//     calling plan.assemble, etc.) and returns the assembled [*Graph] (or nil if the script did not
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
//   - `error`: non-nil if fn returned an error or the planning runtime environment's
//     [RuntimeEnvironment.Close] failed.
func Plan(ctx context.Context, spec *RuntimeEnvironmentSpec, fn func(*RuntimeEnvironment) (*Graph, error)) (*Graph, error) {

	runtimeEnvironment := NewRuntimeEnvironment(ctx, spec)
	defer func() { _ = runtimeEnvironment.Close() }()

	graph, err := fn(runtimeEnvironment)
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
}

// Planner builds an [ExecutableUnit] for one plan-mode method call.
//
// Each [*Method] in the receiver registry carries a Planner — either the default [ActionPlanner] or a
// specialized planner named by reflect.Type in the method's announcement. plan.Provider.Invocation
// delegates the structural shape of the call to the method's planner; plan.Provider then stamps Label /
// RetryPolicy / ErrorAction on the returned unit, wraps it in an [Invocation], and registers it.
//
// Planners are stateless and constructed once per planner type at announcement time.
type Planner interface {

	// Plan builds the executable unit for one call.
	//
	// Parameters:
	//   - `invocator`: the host; gives access to the session invocation registry.
	//   - `receiverType`: the planning provider being routed for.
	//   - `method`: the registered method descriptor.
	//   - `args`: positional arguments converted starlark → Go.
	//   - `kwargs`: keyword arguments converted starlark → Go (reserved kwargs already removed).
	//   - `annotations`: tool-specific annotations stamped onto the unit at construction; nil for none.
	//   - `errorAction`: the failure-handler subgraph applied to the unit at construction, or nil.
	//   - `retryPolicy`: the retry policy applied to the unit at construction, or nil.
	//
	// Returns:
	//   - ExecutableUnit: the assembled unit with `errorAction` / `retryPolicy` applied; Label unset.
	//   - `error`: non-nil on missing required parameter, projection failure, or unit construction error.
	Plan(
		invocator PlanInvocator,
		receiverType ProviderReceiverType,
		method *Method,
		args []any,
		kwargs map[string]any,
		annotations map[string]any,
		errorAction *Subgraph,
		retryPolicy *RetryPolicy,
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

// Fallible actions

// Plan implements [Planner] for vanilla action methods.
//
// Allocates a fresh [*Node] with action name `<provider>.<snake_method>`, binds the method, fills slots
// from `args` / `kwargs` against the method's parameter list, and applies declared defaults to any
// parameter not supplied by the call. Required parameters with no value produce an error.
//
// Parameters:
//   - `invocator`: unused.
//   - `receiverType`: the planning provider.
//   - `method`: the registered method descriptor.
//   - `args`: positional arguments converted starlark → Go.
//   - `kwargs`: keyword arguments converted starlark → Go (reserved entries already removed).
//   - `annotations`: tool-specific annotations stamped onto the node at construction; nil for none.
//   - `errorAction`: the failure-handler subgraph stamped onto the node, or nil.
//   - `retryPolicy`: the retry policy stamped onto the node, or nil.
//
// Returns:
//   - ExecutableUnit: the constructed [*Node] with `errorAction` / `retryPolicy` applied.
//   - `error`: non-nil if a required parameter is missing.
func (ActionPlanner) Plan(
	_ PlanInvocator,
	receiverType ProviderReceiverType,
	method *Method,
	args []any,
	kwargs map[string]any,
	annotations map[string]any,
	errorAction *Subgraph,
	retryPolicy *RetryPolicy,
) (ExecutableUnit, error) {

	if receiverType == nil {
		return nil, fmt.Errorf("op.ActionPlanner.Plan: nil receiverType")
	}
	if method == nil {
		return nil, fmt.Errorf("op.ActionPlanner.Plan: nil method")
	}

	actionName := receiverType.Name() + "." + CamelToSnake(method.Name())

	node := NewNode(GenerateNodeID(actionName), NewAction(receiverType, method, actionName), annotations)

	if errorAction != nil {
		node.setErrorAction(errorAction)
	}
	if retryPolicy != nil {
		node.setRetryPolicy(retryPolicy)
	}

	params := method.Parameters()
	consumed := make(map[string]bool, len(kwargs))
	positional := 0

	for _, param := range params {

		if param.Variadic {
			rest := make([]any, 0, max(0, len(args)-positional))
			for ; positional < len(args); positional++ {
				rest = append(rest, args[positional])
			}
			node.setSlot(param.Name, ImmediateValue{Value: rest})
			continue
		}

		if param.Kwargs {
			remaining := make(map[string]any, len(kwargs))
			for k, v := range kwargs {
				if !consumed[k] {
					remaining[k] = v
				}
			}
			node.setSlot(param.Name, ImmediateValue{Value: remaining})
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
				node.setSlot(param.Name, ImmediateValue{Value: param.Default})
				continue
			}
			if !param.Optional {
				return nil, fmt.Errorf("op.ActionPlanner.Plan: %s: missing required parameter %q", actionName, param.Name)
			}
			continue
		}

		switch v := value.(type) {
		case *Invocation:
			if param.Type != nil && executableUnitType.AssignableTo(param.Type) {
				node.setSlot(param.Name, ImmediateValue{Value: v.Target})
			} else {
				v.FillSlot(node, param.Name)
			}
		case *Promise:
			v.FillSlot(node, param.Name)
		case *Variable:
			node.setSlot(param.Name, VariableValue{Name: v.Name})
		default:
			node.setSlot(param.Name, ImmediateValue{Value: value})
		}
	}

	return node, nil
}

// endregion

// endregion
