// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
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

// Method returns the underlying [*Method].
func (a *action) Method() *Method { return a.method }

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

	runtimeEnvironment := activationRecord.RuntimeEnvironment

	provider, err := runtimeEnvironment.cachedProvider(a.receiverType)
	assert.NoError(a.name, err)

	if runtimeEnvironment.Application.DryRun() {
		dryRunLog(runtimeEnvironment, a.method, a.name, slots)
		return nil, nil, nil
	}

	result, _, err := a.method.Invoke(activationRecord, provider, slots)
	assert.NoError(a.name+": unexpected error from infallible method", err)
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

// Method returns the underlying [*Method].
func (a *fallibleAction) Method() *Method { return a.method }

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

	runtimeEnvironment := activationRecord.RuntimeEnvironment

	provider, err := runtimeEnvironment.cachedProvider(a.receiverType)
	if err != nil {
		return nil, nil, err
	}

	if runtimeEnvironment.Application.DryRun() {
		dryRunLog(runtimeEnvironment, a.method, a.name, slots)
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

// Method returns the underlying [*Method].
func (a *compensableAction) Method() *Method { return a.method }

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

	runtimeEnvironment := activationRecord.RuntimeEnvironment

	provider, err := runtimeEnvironment.cachedProvider(a.receiverType)
	if err != nil {
		return nil, nil, err
	}

	if runtimeEnvironment.Application.DryRun() {
		dryRunLog(runtimeEnvironment, a.method, a.name, slots)
		return nil, nil, nil
	}

	return a.method.Invoke(activationRecord, provider, slots)
}

// Undo constructs a provider and calls the method's compensation companion.
//
// Parameters:
//   - activationRecord: the per-dispatch record. Carries the runtime environment for provider construction;
//     `SiteID` is typically empty during compensation since the original producing dispatch has already executed.
//   - complement: the undo state from Do.
//
// Returns:
//   - error: non-nil if compensation fails.
func (a *compensableAction) Undo(activationRecord *ActivationRecord, complement Complement) error {

	if complement == nil {
		return nil
	}

	runtimeEnvironment := activationRecord.RuntimeEnvironment

	provider, err := runtimeEnvironment.cachedProvider(a.receiverType)
	if err != nil {
		return fmt.Errorf("%s: undo: %w", a.name, err)
	}

	return a.method.Undo(activationRecord, provider, complement)
}

// NewAction creates the appropriate concrete [Action] from a receiver type, method, and short label.
//
// Plan-time callers (planners, writ / lore graph builders, migration plan builders) that hold the
// [ProviderReceiverType] and [*Method] directly use this to bind an Action onto a fresh Node without
// re-walking the registry. Callers that only know the action's short name use
// [ReceiverRegistry.BuildAction] instead.
//
// Parameters:
//   - rt: the provider receiver type.
//   - method: the method descriptor.
//   - name: the action's short label (e.g., "file.copy").
//
// Returns:
//   - Action: the concrete action (one of [action], [fallibleAction], [compensableAction] per
//     [Method.Kind]).
func NewAction(rt ProviderReceiverType, method *Method, name string) Action { return newAction(rt, method, name) }

// newAction is the internal constructor — exported via [NewAction].
func newAction(rt ProviderReceiverType, method *Method, name string) Action {

	switch method.Kind() {
	case MethodAction, MethodFunction:
		return &action{receiverType: rt, method: method, name: name}
	case MethodFallibleAction, MethodFallibleFunction:
		return &fallibleAction{receiverType: rt, method: method, name: name}
	case MethodCompensableFunction:
		return &compensableAction{receiverType: rt, method: method, name: name}
	default:
		assert.Failf("newAction: unknown method kind %d for %s", method.Kind(), name)
		return nil
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
func dryRunLog(runtimeEnvironment *RuntimeEnvironment, method *Method, name string, slots map[string]any) {

	if runtimeEnvironment.Status == nil {
		return
	}

	var builder strings.Builder
	_, _ = fmt.Fprintf(&builder, "[dry-run] %s", name)

	for _, p := range method.Parameters() {
		_, _ = fmt.Fprintf(&builder, " %v", slots[p.Name])
	}

	runtimeEnvironment.Status.Note(builder.String())
}
