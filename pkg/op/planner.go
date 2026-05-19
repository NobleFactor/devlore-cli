// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
	"reflect"
)

// executableUnitType caches the reflect.Type of [ExecutableUnit] for [Planner] implementations that need
// to decide between ImmediateValue (unit reference) and PromiseValue (value-side output) at slot-fill time.
var executableUnitType = reflect.TypeFor[ExecutableUnit]()

// Plan runs a planning session bounded by spec and fn. The session-shape is:
//
//  1. Build a planning [RuntimeEnvironment] from spec.
//  2. Construct a fresh [Graph] via [NewGraph].
//  3. Bind the env to the graph via [Graph.Rebind].
//  4. Call fn with the graph; the caller does the work of populating it (loading a starlark script,
//     adding nodes, populating the catalog, etc.).
//  5. Unbind the graph from the planning env.
//  6. Close the planning env.
//
// Steps 5 and 6 fire via defer, so a panic inside fn still leaves the graph unbound and the env closed.
//
// The returned graph leaves the planning session unbound — its `ctx` field is nil. The next session-owner
// (typically a [GraphExecutor]) Rebinds during its own Run.
//
// Parameters:
//   - `ctx`: the parent context whose cancellation / values flow into the planning env.
//   - `spec`: the planning-environment configuration.
//   - `fn`: the caller-supplied planning routine; receives the freshly-bound graph.
//
// Returns:
//   - *Graph: the planned graph, unbound from the planning env.
//   - `error`: non-nil if fn returned an error or the planning env's [RuntimeEnvironment.Close] failed.
func Plan(ctx context.Context, spec *RuntimeEnvironmentSpec, fn func(*Graph) error) (*Graph, error) {

	env := NewRuntimeEnvironment(ctx, spec)
	defer func() { _ = env.Close() }()

	graph := NewGraph()
	graph.Rebind(env)
	defer graph.Unbind()

	if err := fn(graph); err != nil {
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
	//
	// Returns:
	//   - ExecutableUnit: the assembled unit; Label / RetryPolicy / ErrorAction unset.
	//   - `error`: non-nil on missing required parameter, projection failure, or unit construction error.
	Plan(
		invocator PlanInvocator,
		receiverType ProviderReceiverType,
		method *Method,
		args []any,
		kwargs map[string]any,
	) (ExecutableUnit, error)
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
//
// Returns:
//   - ExecutableUnit: the constructed [*Node].
//   - `error`: non-nil if a required parameter is missing.
func (ActionPlanner) Plan(
	_ PlanInvocator,
	receiverType ProviderReceiverType,
	method *Method,
	args []any,
	kwargs map[string]any,
) (ExecutableUnit, error) {

	if receiverType == nil {
		return nil, fmt.Errorf("op.ActionPlanner.Plan: nil receiverType")
	}
	if method == nil {
		return nil, fmt.Errorf("op.ActionPlanner.Plan: nil method")
	}

	actionName := receiverType.Name() + "." + CamelToSnake(method.Name())

	node := NewNode(GenerateNodeID(actionName))
	node.Receiver = actionName
	node.Bind(method)

	params := method.Parameters()
	consumed := make(map[string]bool, len(kwargs))
	positional := 0

	for _, param := range params {

		if param.Variadic {
			rest := make([]any, 0, max(0, len(args)-positional))
			for ; positional < len(args); positional++ {
				rest = append(rest, args[positional])
			}
			node.SetSlot(param.Name, ImmediateValue{Value: rest})
			continue
		}

		if param.Kwargs {
			remaining := make(map[string]any, len(kwargs))
			for k, v := range kwargs {
				if !consumed[k] {
					remaining[k] = v
				}
			}
			node.SetSlot(param.Name, ImmediateValue{Value: remaining})
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
				node.SetSlot(param.Name, ImmediateValue{Value: param.Default})
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
				node.SetSlot(param.Name, ImmediateValue{Value: v.Target})
			} else {
				v.FillSlot(node, param.Name)
			}
		case *Promise:
			v.FillSlot(node, param.Name)
		case *Variable:
			node.SetSlot(param.Name, VariableValue{Name: v.Name})
		default:
			node.SetSlot(param.Name, ImmediateValue{Value: value})
		}
	}

	return node, nil
}

// endregion

// endregion
