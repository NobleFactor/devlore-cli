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

// FullName returns the canonical action name in fully-qualified form.
//
// The name has the form <pkg-path>.<receiverName>.<methodName> and is sourced from
// [Method.ActionName] — the same value [ReceiptBase.Action] stores once a receipt is committed by
// [Method.Invoke]. Used at saga-stack push sites to stamp action identity onto receipts; distinct from [Name],
// which returns the short starlark form (e.g., "file.join").
//
// Returns:
//   - string: the canonical action name.
func (a *action) FullName() string { return a.method.ActionName() }

// Name returns the action name (e.g., "file.join").
func (a *action) Name() string { return a.name }

// Params returns the method's parameters.
func (a *action) Params() []Parameter { return a.method.Parameters() }

// Do constructs a provider and delegates to [Method.Invoke]. Infallible —
// coercion or dispatch errors become panics.
//
// Parameters:
//   - activationRecord: the per-dispatch record carrying the runtime environment and producing-node identity.
//   - slots: named slot values from the graph node.
//
// Returns:
//   - Result: the method's return value, or nil.
//   - Complement: always nil.
//   - error: always nil.
func (a *action) Do(activationRecord *ActivationRecord, slots map[string]any) (Result, Complement, error) {

	runtimeEnvironment := activationRecord.Runtime

	provider, err := runtimeEnvironment.cachedProvider(a.receiverType)
	if err != nil {
		panic(fmt.Sprintf("%s: %v", a.name, err))
	}

	if runtimeEnvironment.DryRun {
		dryRunLog(a.name, a.method, runtimeEnvironment, slots)
		return nil, nil, nil
	}

	result, _, err := a.method.Invoke(activationRecord, provider, slots)
	if err != nil {
		panic(fmt.Sprintf("%s: unexpected error from infallible method: %v", a.name, err))
	}
	return result, nil, nil
}

// fallibleAction wraps a Method for graph execution. May fail — returns error.
type fallibleAction struct {
	receiverType ProviderReceiverType
	method       *Method
	name         string
}

// FullName returns the canonical action name in fully-qualified form.
//
// The name has the form <pkg-path>.<receiverName>.<methodName> and is sourced from
// [Method.ActionName] — the same value [ReceiptBase.Action] stores once a receipt is committed by
// [Method.Invoke]. Used at saga-stack push sites to stamp action identity onto receipts; distinct from [Name],
// which returns the short starlark form.
//
// Returns:
//   - string: the canonical action name.
func (a *fallibleAction) FullName() string { return a.method.ActionName() }

// Name returns the action name.
func (a *fallibleAction) Name() string { return a.name }

// Params returns the method's parameters.
func (a *fallibleAction) Params() []Parameter { return a.method.Parameters() }

// Do constructs a provider and delegates to [Method.Invoke]. Fallible —
// coercion or dispatch errors are returned to the caller.
//
// Parameters:
//   - activationRecord: the per-dispatch record carrying the runtime environment and producing-node identity.
//   - slots: named slot values from the graph node.
//
// Returns:
//   - Result: the method's return value, or nil.
//   - Complement: always nil.
//   - error: non-nil if the method fails.
func (a *fallibleAction) Do(activationRecord *ActivationRecord, slots map[string]any) (Result, Complement, error) {

	runtimeEnvironment := activationRecord.Runtime

	provider, err := runtimeEnvironment.cachedProvider(a.receiverType)
	if err != nil {
		return nil, nil, err
	}

	if runtimeEnvironment.DryRun {
		dryRunLog(a.name, a.method, runtimeEnvironment, slots)
		return nil, nil, nil
	}

	result, _, err := a.method.Invoke(activationRecord, provider, slots)
	return result, nil, err
}

// compensableAction wraps a Method for graph execution. May fail, supports undo.
type compensableAction struct {
	receiverType ProviderReceiverType
	method       *Method
	name         string
}

// FullName returns the canonical action name in fully-qualified form.
//
// The name has the form <pkg-path>.<receiverName>.<methodName> and is sourced from
// [Method.ActionName] — the same value [ReceiptBase.Action] stores once a receipt is committed by
// [Method.Invoke]. Used at saga-stack push sites to stamp action identity onto receipts; distinct from [Name],
// which returns the short starlark form.
//
// Returns:
//   - string: the canonical action name.
func (a *compensableAction) FullName() string { return a.method.ActionName() }

// Name returns the action name.
func (a *compensableAction) Name() string { return a.name }

// Params returns the method's parameters.
func (a *compensableAction) Params() []Parameter { return a.method.Parameters() }

// Do constructs a provider and delegates to [Method.Invoke]. Compensable —
// returns the complement value alongside the result for later undo.
//
// Parameters:
//   - activationRecord: the per-dispatch record carrying the runtime environment and producing-node identity.
//   - slots: named slot values from the graph node.
//
// Returns:
//   - Result: the method's return value, or nil.
//   - Complement: the undo state for compensation.
//   - error: non-nil if the method fails.
func (a *compensableAction) Do(activationRecord *ActivationRecord, slots map[string]any) (Result, Complement, error) {

	runtimeEnvironment := activationRecord.Runtime

	provider, err := runtimeEnvironment.cachedProvider(a.receiverType)
	if err != nil {
		return nil, nil, err
	}

	if runtimeEnvironment.DryRun {
		dryRunLog(a.name, a.method, runtimeEnvironment, slots)
		return nil, nil, nil
	}

	return a.method.Invoke(activationRecord, provider, slots)
}

// Undo constructs a provider and calls the method's compensation companion.
//
// Parameters:
//   - activationRecord: the per-dispatch record. Carries the runtime environment for provider construction;
//     `NodeID` is typically empty during compensation since the original producing node has already executed.
//   - complement: the undo state from Do.
//
// Returns:
//   - error: non-nil if compensation fails.
func (a *compensableAction) Undo(activationRecord *ActivationRecord, complement Complement) error {

	if complement == nil {
		return nil
	}

	runtimeEnvironment := activationRecord.Runtime

	provider, err := runtimeEnvironment.cachedProvider(a.receiverType)
	if err != nil {
		return fmt.Errorf("%s: undo: %w", a.name, err)
	}

	return a.method.Undo(activationRecord, provider, complement)
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

// dryRunLog writes dry-run output to the context status UI.
func dryRunLog(name string, method *Method, ctx *RuntimeEnvironment, slots map[string]any) {

	if ctx.Status == nil {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[dry-run] %s", name)
	for _, p := range method.Parameters() {
		fmt.Fprintf(&b, " %v", slots[p.Name])
	}
	ctx.Status.Note(b.String())
}
