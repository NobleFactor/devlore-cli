// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

var (
	// activationRecordType is cached for detecting provider methods whose first parameter is an [*ActivationRecord].
	//
	// [Method.Invoke] autofills the [*ActivationRecord] with the per-dispatch record carrying the runtime environment,
	// producing-node identity, and per-call cancellation context. `context.Context` as a first parameter is NOT
	// supported. Methods that need cancellation access it via `activationRecord.Context`.
	activationRecordType = reflect.TypeFor[*ActivationRecord]()

	// errorType is cached for return-type classification.
	errorType = reflect.TypeFor[error]()

	// receiptType is cached for the [MethodCompensableFunction] complement-shape check.
	receiptType = reflect.TypeFor[Receipt]()

	// recoveryStackType is cached for the [MethodCompensableFunction] complement-shape check.
	//
	// [Complement] values typed as [*RecoveryStack] are recognized by [Method.Invoke] as engine-built sagas (e.g., the
	// value WalkTree returns) and spliced into the parent stack via [RecoveryStack.PushNested] rather than being
	// treated as a single [Receipt].
	recoveryStackType = reflect.TypeFor[*RecoveryStack]()
)

// Method describes a callable method on a provider or resource.
//
// It is shared metadata used by both action receiverTypes and starlark receivers. Actions wrap a Method for graph
// dispatch. Starlark receivers wrap a Method for immediate dispatch. Method itself is neither — it is the callable they
// both delegate to.
//
// Any method of a provider may have a plan companion; no method need have one. Companions are discovered by reflection
// on the receiver type, using a name-prefix convention:
//   - `plan (Plan<Name>)`: plan-time output spec, computes the identity of the resource the method will produce from
//     the same inputs. Pure — no I/O.
//   - `undo (Compensate<Name>)`: compensation companion for compensable methods, takes the complement returned by the
//     forward method and reverses its effect.
type Method struct {
	actionName                 string          // canonical <pkg-path>.<receiverName>.<methodName>; computed at NewMethod
	do                         *reflect.Method // forward method
	firstParamIsActivation     bool            // true when `do`'s first parameter (after receiver) is *ActivationRecord
	kind                       MethodKind      // classified from return signature
	modifiers                  MethodModifiers // surface modifiers (e.g. ModifierProperty), stamped at announcement
	parameters                 []Parameter     // named parameters (excluding receiver and any leading activation)
	plan                       *reflect.Method // plan-time output spec companion; nil if the method has no plan companion
	planner                    Planner         // plan-mode dispatch strategy; nil for resource methods; default ActionPlanner for provider methods
	undo                       *reflect.Method // compensation companion; nil unless compensable
	undoFirstParamIsActivation bool            // true when `undo`'s first parameter (after receiver) is *ActivationRecord
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
//   - `do`: the reflected Go method to wrap.
//   - `parameters`: parsed Parameter values matching the method's non-receiver parameters. Wire-form parsing happens
//     upstream in parseParameters at the announcement boundary; NewMethod consumes typed Parameters only.
//   - `plan`: the Plan<Name> companion method, or nil if the method has no plan companion.
//   - `undo`: the Compensate companion method, or nil for non-compensable methods.
//   - `enforceCompanions`: true if this method belongs to a provider; enables companion requirements.
//
// Returns:
//   - `*Method`: the classified method.
//   - `error`: non-nil if validation fails.
func NewMethod(
	do *reflect.Method,
	parameters []Parameter,
	plan *reflect.Method,
	undo *reflect.Method,
	enforceCompanions bool,
) (*Method, error) {

	methodType := do.Type

	// Detect whether the first Go parameter (after the receiver at index 0) is an [*ActivationRecord]. If so,
	// [Method.Invoke] autofills it with the per-dispatch record and the remaining Go parameters align with the caller
	// supplied parameter names. The `announce` map lists user-declared parameters only — the activation is implicit.

	firstParamIsActivation := methodType.NumIn() >= 2 && methodType.In(1) == activationRecordType

	expectedParams := methodType.NumIn() - 1
	if firstParamIsActivation {
		expectedParams--
	}

	if len(parameters) != expectedParams {
		names := make([]string, len(parameters))
		for i, p := range parameters {
			names[i] = p.Name
		}
		return nil, fmt.Errorf("expected %d parameter names for method %s, not %d: %s",
			expectedParams,
			do.Name,
			len(parameters),
			strings.Join(names, ", "))
	}

	// Validate variadic / kwargs position. Each flag implies the parameter sits in the last (or last-before- kwargs)
	// slot. The wire grammar already enforces that variadic / kwargs cannot also carry ?/=; here we only validate
	// cross-parameter position.

	for i, p := range parameters {

		if p.Kwargs && i != len(parameters)-1 {
			return nil, fmt.Errorf("keyword catch-all %q must be the last parameter of method %s",
				p.Name,
				do.Name)
		}

		if p.Variadic && i != len(parameters)-1 && !(i == len(parameters)-2 && parameters[i+1].Kwargs) {
			return nil, fmt.Errorf("variadic parameter %q must be the last or second-to-last (before **kwargs) parameter of method %s",
				p.Name,
				do.Name)
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

			// Complement must be a Receipt, []Receipt, or *RecoveryStack if it's to join a saga. We only enforce this
			// for providers where we expect compensation.

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
			do.Name,
			do.Name)
	}

	undoFirstParamIsActivation := false

	if undo != nil {

		if kind != MethodCompensableFunction {
			return nil, fmt.Errorf("compensation companion %s provided, but method %s is %v, not compensable",
				undo.Name,
				do.Name,
				kind)
		}

		undoType := undo.Type

		// Compensation companion accepts one of two shapes:
		//   (a) (receiver, complement)                    — NumIn == 2; no activation
		//   (b) (receiver, *ActivationRecord, complement) — NumIn == 3; activation is the first user-visible param
		// Method.Undo dispatches based on which shape was registered.

		switch undoType.NumIn() {
		case 2:
			// no activation
		case 3:
			if undoType.In(1) != activationRecordType {
				return nil, fmt.Errorf("compensation companion %s for method %s has an invalid signature: first parameter must be *ActivationRecord when 2 parameters are present, got %s",
					undo.Name,
					do.Name,
					undoType.In(1))
			}
			undoFirstParamIsActivation = true
		default:
			return nil, fmt.Errorf("compensation companion %s for method %s has an invalid signature: expected 1 parameter (complement) or 2 parameters (*ActivationRecord, complement), got %d",
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

	// The input parameters carry Name, Type, Optional, Variadic, Kwargs, and Default already; Type was set upstream by
	// parseParameters or by newReceiverType's auto-positional path. Defensive copy so the Method's internal slice is
	// independent of the caller's input.

	params := make([]Parameter, len(parameters))
	copy(params, parameters)

	receiverType := do.Type.In(0)

	if receiverType.Kind() == reflect.Pointer {
		receiverType = receiverType.Elem()
	}

	actionName := receiverType.PkgPath() + "." + receiverType.Name() + "." + do.Name

	return &Method{
		actionName:                 actionName,
		do:                         do,
		firstParamIsActivation:     firstParamIsActivation,
		kind:                       kind,
		parameters:                 params,
		plan:                       plan,
		undo:                       undo,
		undoFirstParamIsActivation: undoFirstParamIsActivation,
	}, nil
}

// region EXPORTED METHODS

// region State management

// ActionName returns the canonical action name for this method.
//
// Returns:
//   - `string`: the canonical `<pkg-path>.<receiver>.<method>` action name computed at construction.
func (m *Method) ActionName() string { return m.actionName }

// Kind returns the classification of this method's signature.
//
// Returns:
//   - `MethodKind`: the signature classification computed at construction.
func (m *Method) Kind() MethodKind { return m.kind }

// Modifiers returns the surface modifiers stamped on this method.
//
// Returns:
//   - `MethodModifiers`: the modifier set, or [ModifierNone] when none were declared.
func (m *Method) Modifiers() MethodModifiers { return m.modifiers }

// setModifiers stamps the surface modifiers on this method.
//
// Called by the announcement path; the modifier set originates in the codegen-emitted [MethodMetadata.Modifiers] for
// the method.
//
// Parameters:
//   - `modifiers`: the modifier set to stamp.
func (m *Method) setModifiers(modifiers MethodModifiers) { m.modifiers = modifiers }

// Name returns the short name of the method.
//
// Returns:
//   - `string`: the method's short Go name.
func (m *Method) Name() string { return m.do.Name }

// ParameterByName returns the Parameter with the given name, if any.
//
// Parameters:
//   - `name`: the parameter name to look up.
//
// Returns:
//   - `Parameter`: the matching parameter, or the zero `Parameter` when none matches.
//   - `bool`: true when a parameter with `name` exists.
func (m *Method) ParameterByName(name string) (Parameter, bool) {

	for _, p := range m.parameters {
		if p.Name == name {
			return p, true
		}
	}

	return Parameter{}, false
}

// Parameters returns the named parameters of the method, excluding the receiver and any leading context.Context.
//
// Returns:
//   - `[]Parameter`: the named parameters, excluding the receiver and any leading [*ActivationRecord].
func (m *Method) Parameters() []Parameter { return m.parameters }

// Planner returns the plan-mode dispatch strategy for this method.
//
// Nil for resource methods (resources are not plan-dispatchable). Provider methods carry the planner declared at
// announcement; absent declaration means [ActionPlanner].
//
// Returns:
//   - `Planner`: the dispatch strategy, or nil for resource methods.
func (m *Method) Planner() Planner { return m.planner }

// setPlanner stamps the plan-mode dispatch strategy on this method.
//
// Called by the receiver-type construction path at announcement time. Resource methods skip this call; provider methods
// receive either the announcement-declared planner or [ActionPlanner] by default.
//
// Parameters:
//   - `planner`: the dispatch strategy resolved at announcement.
func (m *Method) setPlanner(planner Planner) { m.planner = planner }

// ReceiverType returns the reflect.Type of the method's receiver.
//
// Returns:
//   - `reflect.Type`: the receiver's type, pointer or value as declared.
func (m *Method) ReceiverType() reflect.Type { return m.do.Type.In(0) }

// ResultType returns the reflect.Type of the method's first non-error result, or nil.
//
// Returns:
//   - `reflect.Type`: the first non-error result's type, or nil when the method returns nothing or only an error.
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

// Undo calls the compensation companion on the receiver with the given activation and complement.
//
// The companion's signature shape (with or without a leading *ActivationRecord parameter) is detected at registration
// time and stored on [Method.undoFirstParamIsActivation]; Undo passes activation only when the companion expects it.
//
// Parameters:
//   - `activation`: the per-dispatch record forwarded to the companion when its signature expects it.
//   - `receiver`: the provider value the companion is called on.
//   - `complement`: the complement the forward method returned, reversed by the companion.
//
// Returns:
//   - `error`: the companion's error, or non-nil when the method has no compensation companion.
func (m *Method) Undo(activation *ActivationRecord, receiver any, complement any) error {

	if m.undo == nil {
		return fmt.Errorf("method %s has no compensation companion", m.do.Name)
	}

	var goArgs []reflect.Value

	if m.undoFirstParamIsActivation {
		goArgs = []reflect.Value{
			reflect.ValueOf(receiver),
			reflect.ValueOf(activation),
			reflect.ValueOf(complement),
		}
	} else {
		goArgs = []reflect.Value{
			reflect.ValueOf(receiver),
			reflect.ValueOf(complement),
		}
	}

	results := m.undo.Func.Call(goArgs)
	return errorFromValue(results[0])
}

// endregion

// region Behaviors

// Invoke coerces slot values into Go arguments via [Convert] and dispatches to the wrapped method.
//
// Reads the resolved slot values from `activation.Slots` (stamped by the executor before the dispatch). Each
// parameter's value is looked up by name and converted to the parameter's declared Go type.
//
// Parameters:
//   - `activation`: the per-dispatch record carrying resolved slot values, runtime environment, and unit identity.
//   - `receiver`: the provider or resource value the wrapped method is called on.
//
// Returns:
//   - `Result`: the method's unwrapped return value, or nil for actions.
//   - `Complement`: the committed [Receipt] or spliced [*RecoveryStack], or nil when there is no complement.
//   - `error`: non-nil if slot conversion, dispatch, or receipt commit failed.
func (m *Method) Invoke(activation *ActivationRecord, receiver any) (Result, Complement, error) {

	params := m.Parameters()
	goArgs := make([]any, 0, len(params)+1)

	if m.firstParamIsActivation {
		goArgs = append(goArgs, activation)
	}

	for _, p := range params {

		value := activation.Slots[p.Name]

		val, err := Convert(activation.RuntimeEnvironment, value, p.Type)
		if err != nil {
			return nil, nil, fmt.Errorf("param %s: %w", p.Name, err)
		}

		goArgs = append(goArgs, val)
	}

	result, complement, err := m.Do(receiver, goArgs)

	if err != nil {
		return nil, nil, err
	}

	// Unwrap the reflected return once. m.Do hands back a reflect.Value; the receipt's stored result — and every
	// promise consumer that later reads it via [RecoveryStack.ResultByUnitID] — must see the underlying Go value, not
	// the reflect.Value wrapper. (A consumer slot binding `source=<upstream>` converts that stored value to its
	// parameter type, which fails on a raw reflect.Value.)

	unwrappedResult := resultOrNil(result)
	complementValue := complementOrNil(complement)

	switch v := complementValue.(type) {
	case nil:

		return unwrappedResult, nil, nil

	case Receipt:

		if err := v.Commit(activation.Unit, unwrappedResult, complementValue, err); err != nil {
			return nil, nil, fmt.Errorf("inflate %s receipt: %w", m.actionName, err)
		}

		return unwrappedResult, v, nil

	case *RecoveryStack:

		return unwrappedResult, v, nil

	default:

		recoveryStack, err := m.buildSubStackFromReceiptSlice(activation.Unit, unwrappedResult, complementValue, err)

		if err != nil {
			return nil, nil, err
		}

		if recoveryStack == nil {
			return unwrappedResult, v, nil
		}

		return unwrappedResult, recoveryStack, nil
	}
}

// buildSubStackFromReceiptSlice wraps a slice of [Receipt]-implementing values into a [RecoveryStack].
//
// Parameters:
//   - `unit`: the producing execution unit recorded on each spliced receipt.
//   - `result`: the forward method's unwrapped result, passed to each receipt's Commit.
//   - `complement`: the slice of [Receipt]-implementing values to wrap.
//   - `err`: the forward method's error, passed to each receipt's Commit.
//
// Returns:
//   - `*RecoveryStack`: the stack of committed receipts, or nil when `complement` is not a slice of receipts.
//   - `error`: non-nil if any receipt's Commit failed.
func (m *Method) buildSubStackFromReceiptSlice(
	unit ExecutableUnit,
	result any,
	complement any,
	err error,
) (*RecoveryStack, error) {

	rv := reflect.ValueOf(complement)

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

		if err := receipt.Commit(unit, result, complement, err); err != nil {
			return nil, fmt.Errorf("slice item %d: %w", i, err)
		}

		_ = stack.Push(receipt)
	}

	return stack, nil
}

// Do dispatches a method call directly with Go arguments, returning reflected values.
//
// Parameters:
//   - `receiver`: the provider or resource value the method is called on; auto-addressed when passed by value.
//   - `args`: the Go arguments in declaration order, excluding the receiver.
//
// Returns:
//   - `reflect.Value`: the method's first result, or the zero Value for actions.
//   - `reflect.Value`: the method's complement (compensable third return), or the zero Value.
//   - `error`: non-nil if the argument count is wrong or the method returned a non-nil error.
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
		return reflect.Value{}, reflect.Value{}, errorFromValue(results[0])
	case MethodFunction:
		return results[0], reflect.Value{}, nil
	case MethodFallibleFunction:
		return results[0], reflect.Value{}, errorFromValue(results[1])
	case MethodCompensableFunction:
		return results[0], results[1], errorFromValue(results[2])
	}

	assert.Unreachable("Method.Invoke: exhaustive switch on m.kind")
	return reflect.Value{}, reflect.Value{}, nil
}

// String returns the full Go method signature in human-readable form.
//
// Returns:
//   - `string`: the full Go method signature in human-readable form.
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

// region SUPPORTING TYPES

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

// MethodMetadata is the codegen-emitted record describing one method on a registered provider.
//
// Carries source-level information that Go reflection can't see: the starlark parameter spelling, any surface modifiers
// (e.g. eager property projection via [ModifierProperty]), and, optionally, the planner type that materializes the
// method's calls into an [ExecutableUnit]. Absent Planner means the method uses [ActionPlanner] — the default vanilla
// leaf-node dispatcher.
type MethodMetadata struct {
	ParameterNames []string        // starlark parameter name tokens, ordered to match the Go method's parameter slots
	Modifiers      MethodModifiers // surface modifiers (e.g. ModifierProperty); ModifierNone is the default
	Planner        reflect.Type    // optional; nil means default ActionPlanner
}

// MethodModifiers is a bit set of per-method surface modifiers.
//
// It is orthogonal to [MethodKind]: where MethodKind classifies a method's return signature (action vs. function), a
// modifier records how the method is projected onto a starlark surface. The set is codegen-emitted onto
// [MethodMetadata] and threaded onto the constructed [Method]; the zero value [ModifierNone] is the default callable
// projection.
type MethodModifiers uint

const (

	// ModifierProperty marks a zero-arg getter ([MethodFunction] or [MethodFallibleFunction]) for property projection.
	//
	// A starlark attribute access calls the method and yields its result instead of returning the builtin. The codegen
	// sets it from a `+devlore:property` directive; it is valid only on zero-arg, value-returning methods (an action
	// has no value to project). Subsequent flags double from here (2, 4, 8, …).
	ModifierProperty MethodModifiers = 1 << 0
)

// endregion

// region HELPER FUNCTIONS

// errorFromValue extracts an error from a reflect.Value, returning nil when the value holds a nil interface.
//
// Parameters:
//   - `v`: the reflected return value holding an error interface, possibly nil.
//
// Returns:
//   - `error`: the unwrapped error, or nil when `v` holds a nil interface.
func errorFromValue(v reflect.Value) error {
	if v.IsNil() {
		return nil
	}
	return v.Interface().(error)
}

// errorInvalidResultParameters returns a standard error for an unsupported return signature.
//
// Parameters:
//   - `do`: the reflected method whose return signature was rejected.
//
// Returns:
//   - `error`: a formatted error naming the method and its unsupported signature.
func errorInvalidResultParameters(do *reflect.Method) error {
	return fmt.Errorf("expected void, pure, fallible, or compensable result parameters for method %s, not: %s",
		do.Name,
		methodSignature(do))
}

// isLegalCompensableComplement returns true if t is a valid return type for a complement.
//
// Parameters:
//   - `t`: the candidate complement type to validate.
//
// Returns:
//   - `bool`: true when `t` is a [Receipt], a slice of [Receipt], or a [*RecoveryStack].
func isLegalCompensableComplement(t reflect.Type) bool {

	if t.Implements(receiptType) {
		return true
	}

	if t == recoveryStackType {
		return true
	}

	if t.Kind() == reflect.Slice {
		return t.Elem().Implements(receiptType)
	}

	return false
}

// methodSignature renders a reflect.Method as a human-readable Go function signature.
//
// Parameters:
//   - `m`: the reflected method to render.
//
// Returns:
//   - `string`: the method's Go signature in human-readable form.
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
