// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"strings"
)

// errorType is cached for return-type classification.
var errorType = reflect.TypeOf((*error)(nil)).Elem()

// errFromValue extracts an error from a reflect.Value, returning nil when the value holds a nil interface.
func errFromValue(v reflect.Value) error {
	if v.IsNil() {
		return nil
	}
	return v.Interface().(error)
}

// MethodKind classifies a provider method by its return signature.
type MethodKind int

const (
	// MethodAction produces no result and is guaranteed not to fail. Return: ()
	MethodAction MethodKind = iota

	// MethodFallibleAction produces no result but may fail. Return: (error)
	MethodFallibleAction

	// MethodFunction produces a result and is guaranteed not to fail. Return: (T).
	MethodFunction

	// MethodFallibleFunction produces a result but may fail. Return: (T, error).
	MethodFallibleFunction

	// MethodCompensableFunction produces a result and complement or an error. Return: (T, U, error).
	MethodCompensableFunction
)

// Method describes a callable method on a provider or resource.
//
// It is shared metadata used by both action receiverTypes and starlark receivers. Actions wrap a Method for graph dispatch.
// Starlark receivers wrap a Method for immediate dispatch. Method itself is neither — it is the callable they both
// delegate to.
//
// A method may have up to two companion methods on the same receiver type, discovered by reflection:
//   - planned (<Name>Planned): plan-time output spec, computes the identity of the resource the method will
//     produce from the same inputs. Pure — no I/O.
//   - undo (Compensate<Name>): compensation companion for compensable methods, takes the complement returned by
//     the forward method and reverses its effect.
type Method struct {
	do         *reflect.Method // forward method
	kind       MethodKind      // classified from return signature
	parameters []Parameter     // named parameters (excluding receiver)
	planned    *reflect.Method // plan-time output spec companion; nil if the method has no plan-time output
	undo       *reflect.Method // compensation companion; nil unless compensable
}

// NewMethod creates a [Method] from a reflected Go method, its parameter names, and its optional planned and undo
// companions.
//
// Classification rules:
//   - [MethodAction] returns nothing ()
//   - [MethodFallibleAction] returns an error or nil (error)
//   - [MethodFunction] returns a single result (T)
//   - [MethodFallibleFunction] returns a single result and an error (T, error)
//   - [MethodCompensableFunction] returns a single result, its complement, and an error (T, U, error)
//
// Returns an error if:
//   - paramNames length doesn't match method parameter count (excluding receiver)
//   - return signature does not match any known method kind
//   - planned companion provided for a method that produces no result
//   - planned companion parameter list differs from do
//   - planned companion return signature is not (T, error) where T matches do's first result
//   - compensable method has no Compensate companion
//   - Compensate companion provided for non-compensable method
//   - Compensate companion signature is not func(receiver, complement) error
//
// Parameters:
//   - do: the reflected Go method to wrap.
//   - parameters: parameter names matching the method's non-receiver parameters.
//   - planned: the Planned companion method, or nil if the method has no plan-time output spec.
//   - undo: the Compensate companion method, or nil for non-compensable methods.
//
// Returns:
//   - *Method: the classified method.
//   - error: non-nil if validation fails.
func NewMethod(do *reflect.Method, parameters []string, planned *reflect.Method, undo *reflect.Method) (*Method, error) {

	methodType := do.Type

	expectedParams := methodType.NumIn() - 1

	if len(parameters) != expectedParams {
		return nil, fmt.Errorf("expected %d parameter names for method %s, not %d: %s",
			expectedParams,
			do.Name,
			len(parameters),
			strings.Join(parameters, ", "))
	}

	// Classify by return signature

	numOut := methodType.NumOut()

	var kind MethodKind
	var err error

	switch {
	default:
		err = errorInvalidResultParameters(do)

	case numOut == 0:

		kind = MethodAction
		err = nil

	case numOut == 1:

		if methodType.Out(0).Implements(errorType) {
			kind = MethodFallibleAction
		} else {
			kind = MethodFunction
		}

	case numOut == 2:

		kind = MethodFallibleFunction

		if !methodType.Out(1).Implements(errorType) {
			err = errorInvalidResultParameters(do)
		}

	case numOut == 3:

		kind = MethodCompensableFunction

		if !methodType.Out(2).Implements(errorType) {
			err = errorInvalidResultParameters(do)
		}
	}

	if err != nil {
		return nil, err
	}

	// Cross-validate planned

	if planned != nil {

		if kind == MethodAction || kind == MethodFallibleAction {
			return nil, fmt.Errorf("planned companion %s provided for method %s which produces no result",
				planned.Name,
				do.Name)
		}

		plannedType := planned.Type

		if plannedType.NumIn() != methodType.NumIn() {
			return nil, fmt.Errorf("planned companion %s for method %s must accept %d parameters, got %d",
				planned.Name,
				do.Name,
				methodType.NumIn()-1,
				plannedType.NumIn()-1)
		}

		for i := 1; i < methodType.NumIn(); i++ {
			if plannedType.In(i) != methodType.In(i) {
				return nil, fmt.Errorf("planned companion %s for method %s: parameter %d type mismatch: got %s, want %s",
					planned.Name,
					do.Name,
					i-1,
					plannedType.In(i),
					methodType.In(i))
			}
		}

		if plannedType.NumOut() != 2 {
			return nil, fmt.Errorf("planned companion %s for method %s must return exactly 2 values (result, error), got %d",
				planned.Name,
				do.Name,
				plannedType.NumOut())
		}

		if plannedType.Out(0) != methodType.Out(0) {
			return nil, fmt.Errorf("planned companion %s for method %s: result type mismatch: got %s, want %s",
				planned.Name,
				do.Name,
				plannedType.Out(0),
				methodType.Out(0))
		}

		if !plannedType.Out(1).Implements(errorType) {
			return nil, fmt.Errorf("planned companion %s for method %s: second return value must implement error",
				planned.Name,
				do.Name)
		}
	}

	// Cross-validate undo

	if kind == MethodCompensableFunction && undo == nil {
		return nil, fmt.Errorf("compensable method %s requires a Compensate%s companion", do.Name, do.Name)
	}

	if undo != nil {

		if kind != MethodCompensableFunction {
			return nil, fmt.Errorf("compensation companion %s provided, but method %s is %v, not compensable",
				undo.Name,
				do.Name,
				kind)
		}

		undoType := undo.Type

		if undoType.NumIn() != 2 {
			return nil, fmt.Errorf("compensation companion %s for method %s must accept exactly 1 parameter, got %d",
				undo.Name,
				do.Name,
				undoType.NumIn()-1)
		}

		if undoType.NumOut() != 1 || !undoType.Out(0).Implements(errorType) {
			return nil, fmt.Errorf("compensation companion %s for method %s must return exactly one parameter (error), got %d returns",
				undo.Name,
				do.Name,
				undoType.NumOut())
		}
	}

	// Build parameters

	params := make([]Parameter, len(parameters))

	for i, name := range parameters {
		params[i] = Parameter{Name: name, Type: methodType.In(i + 1)}
	}

	return &Method{do: do, kind: kind, parameters: params, planned: planned, undo: undo}, nil
}

// region EXPORTED METHODS

// region State management

// Kind returns the method's classification.
//
// Returns:
//   - MethodKind: the kind (action, fallible action, function, fallible function, or compensable function).
func (m *Method) Kind() MethodKind { return m.kind }

// HasPlanned reports whether this method has a Planned companion.
//
// A non-nil planned companion means the method produces a resource whose identity can be computed at plan time
// from the method's input slot values. The planner calls [Method.Plan] to populate the catalog's pending entries.
//
// Returns:
//   - bool: true if the method has a Planned companion.
func (m *Method) HasPlanned() bool { return m.planned != nil }

// Name returns the Go method name (CamelCase).
//
// Returns:
//   - string: the method name (e.g., "WriteText").
func (m *Method) Name() string { return m.do.Name }

// Parameters returns the method's named parameters (excluding the receiver).
//
// Returns:
//   - []Parameter: the parameters with names and receiverTypes.
func (m *Method) Parameters() []Parameter { return m.parameters }

// ReceiverType returns the reflect.Type of the method's receiver.
//
// Returns:
//   - reflect.Type: the receiver type (pointer type if the method uses a pointer receiver).
func (m *Method) ReceiverType() reflect.Type { return m.do.Type.In(0) }

// ResultType returns the reflect.Type of the method's first non-error result, or nil if the method has no results
// other than error.
//
// Used by the planner to determine whether a method produces a resource: if ResultType is a registered resource type,
// the method's last resource parameter is treated as the output destination and shadowed in the catalog.
//
// Returns:
//   - reflect.Type: the first non-error result type, or nil.
func (m *Method) ResultType() reflect.Type {
	t := m.do.Type
	if t.NumOut() == 0 {
		return nil
	}
	first := t.Out(0)
	// If the only result is error, there's no real result.
	if t.NumOut() == 1 && first.Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return nil
	}
	return first
}

// endregion

// region Behaviors

// Do calls the forward method on the receiver with the given arguments.
//
// The result and complement are returned as [reflect.Value]. Callers are responsible for marshaling arguments to the
// correct receiverTypes before calling Do. A zero [reflect.Value] indicates no value (void result or absent complement).
//
// Parameters:
//   - receiver: the provider or resource instance to call the method on.
//   - args: the method arguments (excluding the receiver), one per parameter.
//
// Returns:
//   - reflect.Value: the result value, or zero Value for void methods.
//   - reflect.Value: the complement value, or zero Value for non-compensable methods.
//   - error: the error return, or nil on success.
func (m *Method) Do(receiver any, args []any) (reflect.Value, reflect.Value, error) {

	goArgs := make([]reflect.Value, len(args)+1)
	goArgs[0] = reflect.ValueOf(receiver)

	for i, arg := range args {
		if arg == nil {
			goArgs[i+1] = reflect.Zero(m.do.Type.In(i + 1))
		} else {
			goArgs[i+1] = reflect.ValueOf(arg)
		}
	}

	var results []reflect.Value

	if m.do.Type.IsVariadic() {
		results = m.do.Func.CallSlice(goArgs)
	} else {
		results = m.do.Func.Call(goArgs)
	}

	switch m.kind {
	case MethodAction:
		return reflect.Value{}, reflect.Value{}, nil
	case MethodFallibleAction:
		return reflect.Value{}, reflect.Value{}, errFromValue(results[0])
	case MethodFunction:
		return results[0], reflect.Value{}, nil
	case MethodFallibleFunction:
		return results[0], reflect.Value{}, errFromValue(results[1])
	case MethodCompensableFunction:
		return results[0], results[1], errFromValue(results[2])
	}

	panic("unreachable")
}

// String returns the full Go method signature in human-readable form.
//
// Returns:
//   - string: e.g., "func (Provider) WriteText(destination string, content string) (Resource, map[string]any, error)".
func (m *Method) String() string {

	receiverType := m.ReceiverType()

	if receiverType.Kind() == reflect.Ptr {
		receiverType = receiverType.Elem()
	}

	var b strings.Builder

	b.WriteString("func (")
	b.WriteString(receiverType.Name())
	b.WriteString(") ")
	b.WriteString(m.do.Name)
	b.WriteString("(")

	params := make([]string, len(m.parameters))
	for i, p := range m.parameters {
		params[i] = p.Name + " " + p.Type.String()
	}
	b.WriteString(strings.Join(params, ", "))

	b.WriteString(")")

	numOut := m.do.Type.NumOut()

	if numOut > 0 {
		b.WriteString(" ")
		if numOut > 1 {
			b.WriteString("(")
		}

		results := make([]string, numOut)
		for i := range results {
			results[i] = m.do.Type.Out(i).String()
		}
		b.WriteString(strings.Join(results, ", "))

		if numOut > 1 {
			b.WriteString(")")
		}
	}

	return b.String()
}

// Plan calls the Planned companion on the receiver with the given arguments.
//
// The Planned companion must be pure — no I/O, no target-machine state, no mutation. The planner calls it at plan
// time to compute the identity of the resource the forward method will produce. The returned [reflect.Value] holds
// the resource (typed as the method's first return value). Callers that need a strongly-typed [Resource] assert the
// value's Interface() to op.Resource.
//
// A zero-value slot is passed as the parameter type's zero value (the Planned method must tolerate missing inputs,
// or return [KnownAtExecution] when it cannot compute an identity without them).
//
// Parameters:
//   - receiver: the provider instance to call the Planned method on.
//   - args: positional arguments matching the method's non-receiver parameters (nil entries become zero values).
//
// Returns:
//   - reflect.Value: the resource value returned by the Planned method.
//   - error: the Planned method's error return, or a lookup error if the method has no Planned companion.
func (m *Method) Plan(receiver any, args []any) (reflect.Value, error) {

	if m.planned == nil {
		return reflect.Value{}, fmt.Errorf("method %s has no planned companion", m.do.Name)
	}

	goArgs := make([]reflect.Value, len(args)+1)
	goArgs[0] = reflect.ValueOf(receiver)

	for i, arg := range args {

		paramType := m.planned.Type.In(i + 1)

		if arg == nil {
			goArgs[i+1] = reflect.Zero(paramType)
			continue
		}

		argVal := reflect.ValueOf(arg)

		switch {
		case argVal.Type().AssignableTo(paramType):
			goArgs[i+1] = argVal
		case argVal.Type().ConvertibleTo(paramType):
			goArgs[i+1] = argVal.Convert(paramType)
		default:
			return reflect.Value{}, fmt.Errorf(
				"method %s planned: arg %d: cannot convert %T to %s",
				m.do.Name, i, arg, paramType,
			)
		}
	}

	results := m.planned.Func.Call(goArgs)
	return results[0], errFromValue(results[1])
}

// Undo calls the compensation companion on the receiver with the given complement.
//
// Parameters:
//   - receiver: the provider instance to call the compensation method on.
//   - complement: the undo state produced by the forward method's second return value.
//
// Returns:
//   - error: the compensation result.
func (m *Method) Undo(receiver any, complement any) error {

	if m.undo == nil {
		return fmt.Errorf("method %s has no compensation companion", m.do.Name)
	}

	goArgs := make([]reflect.Value, 2)
	goArgs[0] = reflect.ValueOf(receiver)
	goArgs[1] = reflect.ValueOf(complement)

	results := m.undo.Func.Call(goArgs)
	return errFromValue(results[0])
}

// endregion

// endregion

// region UNEXPORTED FUNCTIONS

// errorInvalidResultParameters returns an error describing an unsupported return signature.
//
// Parameters:
//   - do: the reflected method with the invalid signature.
//
// Returns:
//   - error: the formatted error with the full method signature.
func errorInvalidResultParameters(do *reflect.Method) error {

	return fmt.Errorf("expected void, pure, fallible, or compensable result parameters for method %s, not: %s",
		do.Name,
		methodSignature(do))
}

// methodSignature renders a reflect.Method as a human-readable Go function signature.
//
// Unlike [Method.String], this operates on a raw [reflect.Method] and does not include parameter names.
//
// Parameters:
//   - m: the reflected method to render.
//
// Returns:
//   - string: the rendered signature.
func methodSignature(m *reflect.Method) string {

	mt := m.Type
	var b strings.Builder

	// Receiver

	receiver := mt.In(0)

	if receiver.Kind() == reflect.Ptr {
		receiver = receiver.Elem()
	}

	b.WriteString("func (")
	b.WriteString(receiver.Name())
	b.WriteString(") ")
	b.WriteString(m.Name)

	// Parameters (skip receiver at In(0))

	b.WriteString("(")

	params := make([]string, mt.NumIn()-1)
	for i := range params {
		params[i] = mt.In(i + 1).String()
	}
	b.WriteString(strings.Join(params, ", "))

	b.WriteString(")")

	// Result parameters

	if mt.NumOut() > 0 {
		b.WriteString(" ")

		if mt.NumOut() > 1 {
			b.WriteString("(")
		}

		results := make([]string, mt.NumOut())
		for i := range results {
			results[i] = mt.Out(i).String()
		}
		b.WriteString(strings.Join(results, ", "))

		if mt.NumOut() > 1 {
			b.WriteString(")")
		}
	}

	return b.String()
}

// endregion
