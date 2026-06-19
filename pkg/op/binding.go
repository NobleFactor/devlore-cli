// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Binding is the value bound to a slot.
//
// It is sealed at three variants and the set is closed: [ImmediateBinding], [PromiseBinding], and [VariableBinding].
// Callers cannot extend it because the marker method [Binding.isBinding] is unexported.
//
// Resolve returns the slot's resolved Go value at execution time. The variables map carries the binding layer's
// resolved variables (one entry per [VariableBinding] slot the graph references). The recovery stack is queried by
// [PromiseBinding] to look up an upstream unit's output via [RecoveryStack.ResultByUnitID]. Variants ignore the
// parameters they do not need.
type Binding interface {
	isBinding()
	ProducerID() string
	Resolve(variables map[string]Variable, stack *RecoveryStack) any
}

type binding struct {
	value any
}

// region UNEXPORTED METHODS

func (binding) isBinding() {}

// endregion

// ImmediateBinding is a Go value known at plan time.
type ImmediateBinding struct {
	binding
}

// NewImmediateBinding returns an [ImmediateBinding] wrapping a Go value known at plan time.
//
// Parameters:
//   - `value`: the plan-time value to bind (any Go value, including a [Resource]).
//
// Returns:
//   - `ImmediateBinding`: the binding.
func NewImmediateBinding(value any) ImmediateBinding {

	return ImmediateBinding{binding{value: value}}
}

// region EXPORTED METHODS

// ProducerID returns the identity of the executable unit that produced the immediate value referenced by this binding.
//
// Only [Resource] instances are stamped with a Producer ID. An empty string is returned for all other immediate values.
//
// Returns:
//   - `string`: the identity of the executable unit that produced the immediate value referenced by this binding, or an
//     empty string.
func (b ImmediateBinding) ProducerID() string {

	if r, ok := b.value.(Resource); ok {
		return r.ProducerID()
	}

	return ""
}

// Resolve returns the wrapped Go value verbatim.
//
// Both inputs are ignored.
//
// Parameters:
//   - `variables`: the resolved variable map (ignored).
//   - `stack`: the recovery stack (ignored).
//
// Returns:
//   - `any`: the wrapped value.
func (b ImmediateBinding) Resolve(_ map[string]Variable, _ *RecoveryStack) any {

	return b.value
}

// endregion

// PromiseBinding references the output of another executable unit (node or subgraph)
//
// It is resolved to a Go value at execution time via [RecoveryStack.ResultByUnitID] against the active recovery stack.
type PromiseBinding struct {
	binding
}

// NewPromiseBinding returns a [PromiseBinding] that resolves, at execution time, to the output of the producer
// identified by `unitID`.
//
// Parameters:
//   - `unitID`: the ID of the producing [ExecutableUnit].
//
// Returns:
//   - `PromiseBinding`: the binding.
func NewPromiseBinding(unitID string) PromiseBinding {

	return PromiseBinding{binding{value: unitID}}
}

// region EXPORTED METHODS

// ProducerID returns the identity of the [ExecutableUnit] that produces the result promised by this binding.
//
// Returns:
//   - `string`: the identity of the [ExecutableUnit] that produces the result promised by this binding.
func (b PromiseBinding) ProducerID() string {

	return b.value.(string)
}

// Resolve returns the referenced producer's result by querying the recovery stack.
//
// Parameters:
//   - `variables`: the resolved variable map (ignored).
//   - `stack`: the recovery stack carrying per-dispatch receipts.
//
// Returns:
//   - `any`: the referenced producer's stored result, or nil when `stack` is nil or no matching receipt exists.
func (b PromiseBinding) Resolve(_ map[string]Variable, stack *RecoveryStack) any {

	if stack == nil {
		return nil
	}

	if result, ok := stack.ResultByUnitID(b.value.(string)); ok {
		return result
	}

	return nil
}

// endregion

// VariableBinding references a [Variable] by name.
//
// It is authored at plan time via plan.variable("name"); resolved at execution time via the variable map passed to
// [GraphExecutor.Run]. It is assembled by [VariableResolver] from layered sources: default => config => env => flag =>
// override.
type VariableBinding struct {
	binding
}

// NewVariableBinding returns a [VariableBinding] referencing the [Variable] named `name`.
//
// Parameters:
//   - `name`: the variable name, resolved at execution time from the layered variable sources.
//
// Returns:
//   - `VariableBinding`: the binding.
func NewVariableBinding(name string) VariableBinding {

	return VariableBinding{binding{value: name}}
}

// region EXPORTED METHODS

// ProducerID returns an empty string.
//
// Variable bindings do not track producers.
//
// Returns:
//   - `string`: An empty string.
func (b VariableBinding) ProducerID() string {

	return ""
}

// Resolve returns the value of the named variable from the supplied variable map.
//
// Parameters:
//   - `variables`: the resolved variable map keyed by parameter name.
//   - `stack`: the recovery stack (ignored).
//
// Returns:
//   - `any`: the named variable's value, or nil if the variable is absent or the map is nil.
func (b VariableBinding) Resolve(variables map[string]Variable, _ *RecoveryStack) any {

	if variables == nil {
		return nil
	}

	return variables[b.value.(string)].Value
}

// endregion
