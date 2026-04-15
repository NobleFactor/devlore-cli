// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"strings"
)

// action wraps a Method for graph execution. Infallible — no error, no undo.
type action struct {
	receiverType ProviderReceiverType
	method       *Method
	name         string
}

// Name returns the action name (e.g., "file.join").
func (a *action) Name() string { return a.name }

// Params returns the method's parameters.
func (a *action) Params() []Parameter { return a.method.Parameters() }

// Do constructs a provider, coerces slots to args, and calls the method.
//
// Parameters:
//   - ctx: the execution context.
//   - slots: named slot values from the graph node.
//
// Returns:
//   - Result: the method's return value, or nil.
//   - Complement: always nil.
//   - error: always nil.
func (a *action) Do(ctx *ExecutionContext, slots map[string]any) (Result, Complement, error) {

	provider, goArgs, err := prepareCall(a.receiverType, a.method, ctx, slots)
	if err != nil {
		panic(fmt.Sprintf("%s: %v", a.name, err))
	}

	if ctx.DryRun {
		dryRunLog(a.name, a.method, ctx, slots)
		return nil, nil, nil
	}

	result, _, err := a.method.Do(provider, goArgs)
	if err != nil {
		panic(fmt.Sprintf("%s: unexpected error from infallible method: %v", a.name, err))
	}

	return resultOrNil(result), nil, nil
}

// fallibleAction wraps a Method for graph execution. May fail — returns error.
type fallibleAction struct {
	receiverType ProviderReceiverType
	method       *Method
	name         string
}

// Name returns the action name.
func (a *fallibleAction) Name() string { return a.name }

// Params returns the method's parameters.
func (a *fallibleAction) Params() []Parameter { return a.method.Parameters() }

// Do constructs a provider, coerces slots to args, and calls the method.
//
// Parameters:
//   - ctx: the execution context.
//   - slots: named slot values from the graph node.
//
// Returns:
//   - Result: the method's return value, or nil.
//   - Complement: always nil.
//   - error: non-nil if the method fails.
func (a *fallibleAction) Do(ctx *ExecutionContext, slots map[string]any) (Result, Complement, error) {

	provider, goArgs, err := prepareCall(a.receiverType, a.method, ctx, slots)
	if err != nil {
		return nil, nil, err
	}

	if ctx.DryRun {
		dryRunLog(a.name, a.method, ctx, slots)
		return nil, nil, nil
	}

	result, _, err := a.method.Do(provider, goArgs)
	if err != nil {
		return nil, nil, err
	}

	return resultOrNil(result), nil, nil
}

// compensableAction wraps a Method for graph execution. May fail, supports undo.
type compensableAction struct {
	receiverType ProviderReceiverType
	method       *Method
	name         string
}

// Name returns the action name.
func (a *compensableAction) Name() string { return a.name }

// Params returns the method's parameters.
func (a *compensableAction) Params() []Parameter { return a.method.Parameters() }

// Do constructs a provider, coerces slots to args, and calls the method.
//
// Parameters:
//   - ctx: the execution context.
//   - slots: named slot values from the graph node.
//
// Returns:
//   - Result: the method's return value, or nil.
//   - Complement: the undo state for compensation.
//   - error: non-nil if the method fails.
func (a *compensableAction) Do(ctx *ExecutionContext, slots map[string]any) (Result, Complement, error) {

	provider, goArgs, err := prepareCall(a.receiverType, a.method, ctx, slots)
	if err != nil {
		return nil, nil, err
	}

	if ctx.DryRun {
		dryRunLog(a.name, a.method, ctx, slots)
		return nil, nil, nil
	}

	result, complement, err := a.method.Do(provider, goArgs)
	if err != nil {
		return nil, nil, err
	}

	return resultOrNil(result), complementOrNil(complement), nil
}

// Undo constructs a provider and calls the method's compensation companion.
//
// Parameters:
//   - ctx: the execution context.
//   - complement: the undo state from Do.
//
// Returns:
//   - error: non-nil if compensation fails.
func (a *compensableAction) Undo(ctx *ExecutionContext, complement Complement) error {

	if complement == nil {
		return nil
	}

	provider, err := ctx.cachedProvider(a.receiverType)
	if err != nil {
		return fmt.Errorf("%s: undo: %w", a.name, err)
	}

	return a.method.Undo(provider, complement)
}

// newAction creates the appropriate concrete action type based on the method's kind.
//
// Parameters:
//   - rt: the provider receiver type.
//   - method: the method descriptor.
//   - name: the action name (e.g., "file.copy").
//
// Returns:
//   - Action: the concrete action.
func newAction(rt ProviderReceiverType, method *Method, name string) Action {

	switch method.Kind() {
	case MethodAction, MethodFunction:
		return &action{receiverType: rt, method: method, name: name}
	case MethodFallibleAction, MethodFallibleFunction:
		return &fallibleAction{receiverType: rt, method: method, name: name}
	case MethodCompensableFunction:
		return &compensableAction{receiverType: rt, method: method, name: name}
	default:
		panic(fmt.Sprintf("newAction: unknown method kind %d for %s", method.Kind(), name))
	}
}

// prepareCall constructs the provider and coerces slot values to Go args.
//
// Parameters:
//   - rt: the provider receiver type.
//   - method: the method descriptor.
//   - ctx: the execution context.
//   - slots: named slot values from the graph node.
//
// Returns:
//   - any: the constructed provider instance.
//   - []any: the coerced Go arguments.
//   - error: non-nil if construction or coercion fails.
func prepareCall(rt ProviderReceiverType, method *Method, ctx *ExecutionContext, slots map[string]any) (any, []any, error) {

	provider, err := ctx.cachedProvider(rt)
	if err != nil {
		return nil, nil, err
	}

	params := method.Parameters()
	goArgs := make([]any, len(params))

	for i, p := range params {
		// Slots can be keyed either by the parameter's full name (with markers
		// like "boundary?") or by its clean name (without markers, "boundary").
		// The planner uses clean names; imperative graph builders may use
		// either. Check both.
		sv, ok := slots[p.Name]
		if !ok {
			cleanName := strings.TrimSuffix(p.Name, "?")
			cleanName = strings.TrimPrefix(cleanName, "*")
			sv = slots[cleanName]
		}
		val, err := coerceSlotValue(ctx, sv, p.Type)
		if err != nil {
			return nil, nil, fmt.Errorf("param %s: %w", p.Name, err)
		}
		goArgs[i] = val
	}

	return provider, goArgs, nil
}

// coerceSlotValue converts a slot value to the target Go type.
//
// Coercion path:
//  1. nil → zero value
//  2. Direct assignment — already the right type
//  3. reflect.Convert — Go built-in conversion (e.g., int64 → int)
//  4. Convertible.Convert — domain-specific conversion (e.g., mem.Function → func)
//
// Parameters:
//   - slotValue: the raw value from the slot.
//   - targetType: the expected Go type.
//
// Returns:
//   - any: the coerced value.
//   - error: non-nil if coercion fails.
func coerceSlotValue(ctx *ExecutionContext, slotValue any, targetType reflect.Type) (any, error) {

	if slotValue == nil {
		return reflect.Zero(targetType).Interface(), nil
	}

	sv := reflect.ValueOf(slotValue)

	if sv.Type().AssignableTo(targetType) {
		return slotValue, nil
	}

	if sv.Type().ConvertibleTo(targetType) {
		return sv.Convert(targetType).Interface(), nil
	}

	if c, ok := slotValue.(Convertible); ok {
		return c.ConvertTo(targetType)
	}

	// Try resource constructor coercion (e.g., string → *file.Resource).
	if s, ok := slotValue.(string); ok {
		elemType := targetType
		isPtr := false
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
			isPtr = true
		}
		if ctx != nil && ctx.Registry != nil {
			if rt, ok := ctx.Registry.TypeByReflection(elemType); ok {
				if rrt, ok := rt.(ResourceReceiverType); ok {
					resource, err := rrt.Construct()(ctx, s)
					if err != nil {
						return nil, err
					}
					if !isPtr {
						return reflect.ValueOf(resource).Elem().Interface(), nil
					}
					return resource, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("cannot coerce %T to %s", slotValue, targetType)
}

// resultOrNil extracts the interface value from a reflect.Value, or nil if invalid.
func resultOrNil(v reflect.Value) Result {

	if !v.IsValid() {
		return nil
	}
	return v.Interface()
}

// complementOrNil extracts the interface value from a reflect.Value, or nil if invalid.
func complementOrNil(v reflect.Value) Complement {

	if !v.IsValid() {
		return nil
	}
	return v.Interface()
}

// dryRunLog writes dry-run output to the context writer.
func dryRunLog(name string, method *Method, ctx *ExecutionContext, slots map[string]any) {

	if ctx.Writer == nil {
		return
	}
	_, _ = fmt.Fprintf(ctx.Writer, "[dry-run] %s", name)
	for _, p := range method.Parameters() {
		_, _ = fmt.Fprintf(ctx.Writer, " %v", slots[p.Name])
	}
	_, _ = fmt.Fprintln(ctx.Writer)
}
