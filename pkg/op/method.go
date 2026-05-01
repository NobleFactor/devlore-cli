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
var contextType = reflect.TypeFor[context.Context]()

// errorType is cached for return-type classification.
var errorType = reflect.TypeFor[error]()

// receiptType is cached for the [MethodCompensableFunction] complement-shape check.
var receiptType = reflect.TypeFor[Receipt]()

// recoveryStackType is cached for the [MethodCompensableFunction] complement-shape check.
//
// Complement values typed as `*RecoveryStack` are recognized by [Method.Invoke] as engine-built sagas (e.g.,
// the value WalkTree returns) and spliced into the parent stack via PushNested rather than PushReceipt.
var recoveryStackType = reflect.TypeFor[*RecoveryStack]()

// errFromValue extracts an error from a reflect.Value, returning nil when the value holds a nil interface.
func errFromValue(v reflect.Value) error {
	if v.IsNil() {
		return nil
	}
	return v.Interface().(error)
}

// MethodKind identifies the signature and capabilities of a method.
type MethodKind int

const (
	// MethodAction produces no result and cannot fail. Return: ().
	MethodAction MethodKind = iota

	// MethodFallibleAction produces no result but may fail. Return: (error).
	MethodFallibleAction

	// MethodFunction produces a result and cannot fail. Return: (T).
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
//   - compensable method has no Compensate companion (if enforceCompanions is true)
//   - Compensate companion signature is invalid
//
// Parameters:
//   - do: the reflected Go method to wrap.
//   - parameters: parameter names matching the method's non-receiver parameters.
//   - plan: the Plan<Name> companion method, or nil if the method has no plan companion.
//   - undo: the Compensate companion method, or nil for non-compensable methods.
//   - enforceCompanions: true if this method belongs to a provider; enables companion requirements.
//
// Returns:
//   - *Method: the classified method.
//   - error: non-nil if validation fails.
func NewMethod(do *reflect.Method, parameters []string, plan *reflect.Method, undo *reflect.Method, enforceCompanions bool) (*Method, error) {

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

	// Validate variadic markers.
	for i, name := range parameters {
		if strings.HasPrefix(name, "**") {
			if i != len(parameters)-1 {
				return nil, fmt.Errorf("keyword catch-all %q must be the last parameter of method %s", name, do.Name)
			}
		} else if strings.HasPrefix(name, "*") {
			if i != len(parameters)-1 && !(i == len(parameters)-2 && strings.HasPrefix(parameters[i+1], "**")) {
				return nil, fmt.Errorf("variadic parameter %q must be the last or second-to-last (before **kwargs) parameter of method %s", name, do.Name)
			}
		}
	}

	// Classify by return signature

	numOut := methodType.NumOut()

	var kind MethodKind
	var err error

	switch numOut {
	default:
		err = errorInvalidResultParameters(do)

	case 0:

		kind = MethodAction
		err = nil

	case 1:

		if methodType.Out(0).Implements(errorType) {
			kind = MethodFallibleAction
		} else {
			kind = MethodFunction
		}

	case 2:

		kind = MethodFallibleFunction

		if !methodType.Out(1).Implements(errorType) {
			err = errorInvalidResultParameters(do)
		}

	case 3:

		kind = MethodCompensableFunction

		if !methodType.Out(2).Implements(errorType) {
			err = errorInvalidResultParameters(do)
		} else if !isLegalCompensableComplement(methodType.Out(1)) {
			// Complement must be a Receipt, []Receipt, or *RecoveryStack if it's to join a saga.
			// We only enforce this for providers where we expect compensation.
			if enforceCompanions {
				err = fmt.Errorf("compensable method %s: complement type %s must be Receipt, []Receipt (or a slice "+
					"whose element implements Receipt), or *RecoveryStack",
					do.Name,
					methodType.Out(1))
			}
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

	if enforceCompanions && kind == MethodCompensableFunction && undo == nil {
		return nil, fmt.Errorf("method %s appears to be compensable (returns 3 values) but no 'Compensate%s' companion was found",
			do.Name, do.Name)
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
			return nil, fmt.Errorf("compensation companion %s for method %s has an invalid signature: expected 1 parameter (the complement), got %d",
				undo.Name,
				do.Name,
				undoType.NumIn()-1)
		}

		if undoType.NumOut() != 1 || !undoType.Out(0).Implements(errorType) {
			return nil, fmt.Errorf("compensation companion %s for method %s has an invalid signature: must return exactly one parameter (error), got %d",
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

	if receiverType.Kind() == reflect.Pointer {
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
func (m *Method) ActionName() string { return m.actionName }

// Kind returns the classification of this method's signature.
func (m *Method) Kind() MethodKind { return m.kind }

// Name returns the short name of the method.
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

// Parameters returns the named parameters of the method, excluding the receiver and any leading context.Context.
func (m *Method) Parameters() []Parameter { return m.parameters }

// ReceiverType returns the reflect.Type of the method's receiver.
func (m *Method) ReceiverType() reflect.Type { return m.do.Type.In(0) }

// ResultType returns the reflect.Type of the method's first non-error result, or nil.
func (m *Method) ResultType() reflect.Type {
	t := m.do.Type
	if t.NumOut() == 0 {
		return nil
	}
	first := t.Out(0)
	if t.NumOut() == 1 && first.Implements(errorType) {
		return nil
	}
	return first
}

// Undo calls the compensation companion on the receiver with the given complement.
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

// HasUndo reports whether this method has a compensation companion.
func (m *Method) HasUndo() bool { return m.undo != nil }

// endregion

// region Behaviors

// Invoke coerces slot values into Go arguments via [Convert] and dispatches to the wrapped method.
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

		val, err := Convert(ctx, value, p.Type)
		if err != nil {
			return nil, nil, fmt.Errorf("param %s: %w", p.Name, err)
		}

		goArgs = append(goArgs, val)
	}

	result, complement, err := m.Do(receiver, goArgs)

	if err != nil {
		return nil, nil, err
	}

	complementValue := complementOrNil(complement)

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
func (m *Method) buildSubStackFromReceiptSlice(v any) (*RecoveryStack, error) {

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, nil
	}

	stack := NewRecoveryStack()
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i).Interface()
		receipt, ok := item.(Receipt)
		if !ok {
			return nil, nil
		}
		if err := receipt.Commit(m.actionName); err != nil {
			return nil, fmt.Errorf("slice item %d: %w", i, err)
		}
		_ = stack.PushReceipt(receipt, m.actionName)
	}

	return stack, nil
}

// Do dispatches a method call directly with Go arguments, returning reflected values.
func (m *Method) Do(receiver any, args []any) (reflect.Value, reflect.Value, error) {

	v := reflect.ValueOf(receiver)

	if v.Kind() != reflect.Pointer {
		ptr := reflect.New(v.Type())
		ptr.Elem().Set(v)
		v = ptr
	}

	numIn := m.do.Type.NumIn()
	if len(args)+1 != numIn {
		return reflect.Value{}, reflect.Value{}, fmt.Errorf("method %s: expected %d arguments (including receiver), got %d",
			m.do.Name,
			numIn,
			len(args)+1)
	}

	reflectArgs := make([]reflect.Value, len(args)+1)
	reflectArgs[0] = v

	for i, arg := range args {
		if arg == nil {
			reflectArgs[i+1] = reflect.Zero(m.do.Type.In(i + 1))
		} else {
			reflectArgs[i+1] = reflect.ValueOf(arg)
		}
	}

	var results []reflect.Value

	if m.do.Type.IsVariadic() {
		results = m.do.Func.CallSlice(reflectArgs)
	} else {
		results = m.do.Func.Call(reflectArgs)
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
func (m *Method) String() string {

	receiverType := m.ReceiverType()

	if receiverType.Kind() == reflect.Pointer {
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

// endregion

// endregion

// region HELPER FUNCTIONS

// errorInvalidResultParameters returns a standard error for an unsupported return signature.
func errorInvalidResultParameters(do *reflect.Method) error {
	return fmt.Errorf("expected void, pure, fallible, or compensable result parameters for method %s, not: %s",
		do.Name,
		methodSignature(do))
}

// isLegalCompensableComplement returns true if t is a valid return type for a complement.
func isLegalCompensableComplement(t reflect.Type) bool {

	receiptType := reflect.TypeFor[Receipt]()

	if t.Implements(receiptType) {
		return true
	}

	if t == reflect.TypeFor[*RecoveryStack]() {
		return true
	}

	if t.Kind() == reflect.Slice {
		return t.Elem().Implements(receiptType)
	}

	return false
}

// methodSignature renders a reflect.Method as a human-readable Go function signature.
func methodSignature(m *reflect.Method) string {

	mt := m.Type
	var b strings.Builder

	receiver := mt.In(0)
	if receiver.Kind() == reflect.Pointer {
		receiver = receiver.Elem()
	}

	b.WriteString("func (")
	b.WriteString(receiver.Name())
	b.WriteString(") ")
	b.WriteString(m.Name)

	b.WriteString("(")
	params := make([]string, mt.NumIn()-1)
	for i := range params {
		params[i] = mt.In(i + 1).String()
	}
	b.WriteString(strings.Join(params, ", "))
	b.WriteString(")")

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
