// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

// contextType is cached for detecting provider methods whose first parameter (after the receiver) is a context.Context
// handle, which [Method.Invoke] autofills with the ambient cancellation context.
var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

// errorType is cached for return-type classification.
var errorType = reflect.TypeOf((*error)(nil)).Elem()

// receiptType is cached for the [MethodCompensableFunction] complement-shape check.
var receiptType = reflect.TypeOf((*Receipt)(nil)).Elem()

// recoveryStackType is cached for the [MethodCompensableFunction] complement-shape check.
//
// Complement values typed as `*RecoveryStack` are recognized by [Method.Invoke] as engine-built sagas (e.g.,
// the value WalkTree returns) and spliced into the parent stack via PushNested rather than PushReceipt.
var recoveryStackType = reflect.TypeOf((*RecoveryStack)(nil))

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
// Any method of a provider may have a plan companion; no method need have one. Companions are discovered by
// reflection on the receiver type, using a name-prefix convention:
//   - plan (Plan<Name>): plan-time output spec, computes the identity of the resource the method will produce
//     from the same inputs. Pure — no I/O.
//   - undo (Compensate<Name>): compensation companion for compensable methods, takes the complement returned by
//     the forward method and reverses its effect.
type Method struct {
	actionName      string          // canonical <pkg-path>.<receiverName>.<methodName>; computed at NewMethod
	do              *reflect.Method // forward method
	firstParamIsCtx bool            // true when `do`'s first parameter (after receiver) is context.Context
	kind            MethodKind      // classified from return signature
	parameters      []Parameter     // named parameters (excluding receiver and any leading ctx)
	plan            *reflect.Method // plan-time output spec companion; nil if the method has no plan companion
	undo            *reflect.Method // compensation companion; nil unless compensable
}

// NewMethod creates a [Method] from a reflected Go method, its parameter names, and its optional plan and undo
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
//   - plan companion provided for a method that produces no result
//   - plan companion parameter list differs from do
//   - plan companion return signature is not (T, error) where T matches `do`'s first result
//   - compensable method has no Compensate companion
//   - Compensate companion provided for non-compensable method
//   - Compensate companion signature is not func(receiver, complement) error
//
// Parameters:
//   - do: the reflected Go method to wrap.
//   - parameters: parameter names matching the method's non-receiver parameters.
//   - plan: the Plan<Name> companion method, or nil if the method has no plan companion.
//   - undo: the Compensate companion method, or nil for non-compensable methods.
//
// Returns:
//   - *Method: the classified method.
//   - error: non-nil if validation fails.
func NewMethod(do *reflect.Method, parameters []string, plan *reflect.Method, undo *reflect.Method) (*Method, error) {

	methodType := do.Type

	// Detect whether the first Go parameter (after the receiver at index 0) is a context.Context. If so, Method.Invoke
	// autofills it with the ambient cancellation ctx and the remaining Go parameters align with the caller supplied
	// parameter names. The `announce` map lists user-declared parameters only — ctx is implicit.
	firstParamIsCtx := methodType.NumIn() >= 2 && methodType.In(1) == contextType

	expectedParams := methodType.NumIn() - 1
	if firstParamIsCtx {
		expectedParams--
	}

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
		} else if !isLegalCompensableComplement(methodType.Out(1)) {
			err = fmt.Errorf("compensable method %s: complement type %s must be Receipt, []Receipt (or a slice "+
				"whose element implements Receipt), or *RecoveryStack",
				do.Name,
				methodType.Out(1))
		}
	}

	if err != nil {
		return nil, err
	}

	// Cross-validate plan

	if plan != nil {

		if kind == MethodAction || kind == MethodFallibleAction {
			return nil, fmt.Errorf("plan companion %s provided for method %s which produces no result",
				plan.Name,
				do.Name)
		}

		planType := plan.Type

		if planType.NumIn() != methodType.NumIn() {
			return nil, fmt.Errorf("plan companion %s for method %s must accept %d parameters, got %d",
				plan.Name,
				do.Name,
				methodType.NumIn()-1,
				planType.NumIn()-1)
		}

		for i := 1; i < methodType.NumIn(); i++ {
			if planType.In(i) != methodType.In(i) {
				return nil, fmt.Errorf("plan companion %s for method %s: parameter %d type mismatch: got %s, want %s",
					plan.Name,
					do.Name,
					i-1,
					planType.In(i),
					methodType.In(i))
			}
		}

		if planType.NumOut() != 2 {
			return nil, fmt.Errorf("plan companion %s for method %s must return exactly 2 values (result, error), got %d",
				plan.Name,
				do.Name,
				planType.NumOut())
		}

		if planType.Out(0) != methodType.Out(0) {
			return nil, fmt.Errorf("plan companion %s for method %s: result type mismatch: got %s, want %s",
				plan.Name,
				do.Name,
				planType.Out(0),
				methodType.Out(0))
		}

		if !planType.Out(1).Implements(errorType) {
			return nil, fmt.Errorf("plan companion %s for method %s: second return value must implement error",
				plan.Name,
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

	// Build parameters. If the Go method declares a leading context.Context, it
	// sits at In(1) and user parameters start at In(2); otherwise user
	// parameters start at In(1).
	paramOffset := 1
	if firstParamIsCtx {
		paramOffset = 2
	}

	params := make([]Parameter, len(parameters))

	for i, name := range parameters {
		params[i] = Parameter{Name: name, Type: methodType.In(i + paramOffset)}
	}

	receiverType := do.Type.In(0)

	if receiverType.Kind() == reflect.Ptr {
		receiverType = receiverType.Elem()
	}

	actionName := receiverType.PkgPath() + "." + receiverType.Name() + "." + do.Name

	return &Method{
		actionName:      actionName,
		do:              do,
		firstParamIsCtx: firstParamIsCtx,
		kind:            kind,
		parameters:      params,
		plan:            plan,
		undo:            undo,
	}, nil
}

// region EXPORTED METHODS

// region State management

// ActionName returns the canonical action name for this method.
//
// The name has the form <pkg-path>.<receiverName>.<methodName> and is computed once at [NewMethod]
// construction. Callers should prefer this over ad-hoc composition from reflect metadata.
//
// Returns:
//   - string: the canonical action name.
func (m *Method) ActionName() string { return m.actionName }

// Kind returns the method's classification.
//
// Returns:
//   - MethodKind: the kind (action, fallible action, function, fallible function, or compensable function).
func (m *Method) Kind() MethodKind { return m.kind }

// HasPlan reports whether this method has a Plan<Name> companion.
//
// A non-nil plan companion means the method produces a resource whose identity can be computed at plan time
// from the method's input slot values. The planner calls [Method.Plan] to populate the catalog's pending entries.
//
// Returns:
//   - bool: true if the method has a Plan<Name> companion.
func (m *Method) HasPlan() bool { return m.plan != nil }

// Name returns the Go method name (CamelCase).
//
// Returns:
//   - string: the method name (e.g., "WriteText").
func (m *Method) Name() string { return m.do.Name }

// ParameterByName returns the Parameter with the given name, if any.
func (m *Method) ParameterByName(name string) (Parameter, bool) {

	for _, p := range m.parameters {
		if p.Name == name {
			return p, true
		}
	}
	return Parameter{}, false
}

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

// Invoke coerces slot values into Go arguments via [Convert] and dispatches to the wrapped method on the given
// receiver.
//
// Invoke is the single dispatch entry point for already-resolved slot values — the plan path
// ([starlarkbridge.NodeBuilder.FillSlot]) and the execute path ([starlarkbridge.receiver.dispatch]) both project starlark values down to Go
// values first; Invoke then projects those Go values into the method signature using the same [Convert] cascade both
// paths share. Results are unpacked from [reflect.Value] into the Action-layer shape (Result, Complement, error).
//
// When the underlying Go method declares a leading context.Context, Invoke autofills it from ctx.Context; provider
// methods that do not declare it are unaffected and their user-declared parameters remain the full Go argument list.
//
// Parameters:
//   - ctx: the execution context carrying registry, catalog, and the ambient cancellation context.
//   - receiver: the provider or resource instance to dispatch on.
//   - slots: named slot values from the graph node; keys may be the parameter's raw name (e.g., "boundary?") or its
//     clean form ("boundary"). Both are accepted for compatibility with imperative graph builders.
//
// Returns:
//   - Result: the method's return value, or nil for void methods.
//   - Complement: the undo state for compensable methods, or nil.
//   - error: non-nil on coercion failure or a fallible method's error.
func (m *Method) Invoke(ctx *ExecutionContext, receiver any, slots map[string]any) (Result, Complement, error) {

	params := m.Parameters()
	goArgs := make([]any, 0, len(params)+1)

	if m.firstParamIsCtx {
		goArgs = append(goArgs, ctx.Context)
	}

	for _, p := range params {

		value, ok := slots[p.Name]

		if !ok {
			cleanName := strings.TrimSuffix(strings.TrimLeft(p.Name, "*"), "?")
			value = slots[cleanName]
		}

		var val any
		var err error

		switch {
		case value == nil:
			val = reflect.Zero(p.Type).Interface()
		case reflect.TypeOf(value).AssignableTo(p.Type):
			val = value
		default:
			val, err = value.(Converter).Convert(p.Type)
			if err != nil {
				return nil, nil, fmt.Errorf("param %s: %w", p.Name, err)
			}
		}

		goArgs = append(goArgs, val)
	}

	result, complement, err := m.Do(receiver, goArgs)

	if err != nil {
		return nil, nil, err
	}

	complementValue := complementOrNil(complement)

	// Reshape the complement on the return path. The classifier guarantees one of three shapes for
	// MethodCompensableFunction: a Receipt, a slice of Receipt-implementing values, or *RecoveryStack. The
	// executor's push site dispatches per shape; this method's job is to commit any receipts and (for the
	// slice case) build the engine-side sub-stack that the executor will splice via PushNested.
	switch v := complementValue.(type) {
	case nil:
		return resultOrNil(result), nil, nil
	case Receipt:
		if commitErr := v.Commit(m.actionName); commitErr != nil {
			return nil, nil, fmt.Errorf("inflate %s receipt: %w", m.actionName, commitErr)
		}
		return resultOrNil(result), v, nil
	case *RecoveryStack:
		return resultOrNil(result), v, nil
	default:
		sub, buildErr := m.buildSubStackFromReceiptSlice(v)
		if buildErr != nil {
			return nil, nil, buildErr
		}
		if sub == nil {
			return resultOrNil(result), v, nil
		}
		return resultOrNil(result), sub, nil
	}
}

// buildSubStackFromReceiptSlice wraps a slice of [Receipt]-implementing values into a [RecoveryStack].
//
// Returns (nil, nil) when complement is not a slice of receipts — caller falls through to its own handling.
// Each receipt is pushed via [RecoveryStack.PushReceipt] under m.actionName, which commits in place.
//
// Parameters:
//   - complement: a candidate value the type-switch fell through to; expected to be a slice whose element
//     implements [Receipt] (e.g., []*file.Receipt, []op.Receipt).
//
// Returns:
//   - *RecoveryStack: a fresh sub-stack with one entry per receipt, in slice order; nil when complement is
//     not a recognized receipt slice.
//   - error: any error from [RecoveryStack.PushReceipt] (commit failure, missing resource context).
func (m *Method) buildSubStackFromReceiptSlice(complement any) (*RecoveryStack, error) {

	v := reflect.ValueOf(complement)
	if v.Kind() != reflect.Slice {
		return nil, nil
	}

	if !v.Type().Elem().Implements(receiptType) && !reflect.PointerTo(v.Type().Elem()).Implements(receiptType) {
		return nil, nil
	}

	sub := NewRecoveryStack()
	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i).Interface()
		receipt, ok := elem.(Receipt)
		if !ok {
			return nil, fmt.Errorf("inflate %s receipt slice: element %d does not implement Receipt", m.actionName, i)
		}
		if pushErr := sub.PushReceipt(receipt, m.actionName); pushErr != nil {
			return nil, fmt.Errorf("inflate %s receipt slice: push element %d: %w", m.actionName, i, pushErr)
		}
	}

	return sub, nil
}

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

// Plan calls the Plan<Name> companion on the receiver with the given arguments.
//
// The Plan companion must be pure — no I/O, no target-machine state, no mutation. The planner calls it at plan
// time to compute the identity of the resource the forward method will produce. The returned [reflect.Value] holds
// the resource (typed as the method's first return value). Callers that need a strongly-typed [Resource] assert the
// value's Interface() to op.Resource.
//
// A zero-value slot is passed as the parameter type's zero value (the Plan method must tolerate missing inputs,
// or return [KnownAtExecution] when it cannot compute an identity without them).
//
// Parameters:
//   - receiver: the provider instance to call the Plan method on.
//   - args: positional arguments matching the method's non-receiver parameters (nil entries become zero values).
//
// Returns:
//   - reflect.Value: the resource value returned by the Plan method.
//   - error: the Plan method's error return, or a lookup error if the method has no Plan companion.
func (m *Method) Plan(receiver any, args []any) (reflect.Value, error) {

	if m.plan == nil {
		return reflect.Value{}, fmt.Errorf("method %s has no plan companion", m.do.Name)
	}

	goArgs := make([]reflect.Value, len(args)+1)
	goArgs[0] = reflect.ValueOf(receiver)

	for i, arg := range args {

		paramType := m.plan.Type.In(i + 1)

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
				"method %s plan: arg %d: cannot convert %T to %s",
				m.do.Name, i, arg, paramType,
			)
		}
	}

	results := m.plan.Func.Call(goArgs)
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

// isLegalCompensableComplement reports whether reflectType is a legal complement type for [MethodCompensableFunction].
//
// Three shapes are accepted: a [Receipt]-implementing type (single-output compensable), a slice whose element
// implements [Receipt] (multi-output compensable; engine wraps the slice into a sub-stack at [Method.Invoke]
// time), and `*RecoveryStack` (action returns a fully-built saga that the engine splices via PushNested).
//
// Parameters:
//   - reflectType: the method's second return type ([reflect.Type] of `Out(1)`).
//
// Returns:
//   - bool: true when reflectType is one of the three legal complement shapes.
func isLegalCompensableComplement(reflectType reflect.Type) bool {

	if reflectType.Implements(receiptType) {
		return true
	}

	if reflectType.Kind() == reflect.Slice && reflectType.Elem().Implements(receiptType) {
		return true
	}

	if reflectType == recoveryStackType {
		return true
	}

	return false
}

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
