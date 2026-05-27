// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// SlotValue is the value bound to a slot. Sealed at three variants — [ImmediateValue], [PromiseValue], and
// [VariableValue]. The set is closed; callers cannot extend it because the marker method isSlotValue is
// unexported.
//
// Resolve returns the slot's resolved Go value at execution time. The variables map carries the binding
// layer's resolved variables (one entry per [VariableValue] slot the graph references); the recovery
// stack is queried by [PromiseValue] to look up an upstream unit's output via
// [RecoveryStack.ResultByUnitID]. Variants ignore the parameters they do not need.
type SlotValue interface {
	isSlotValue()
	Resolve(variables map[string]Variable, stack *RecoveryStack) any
}

// region HELPER FUNCTIONS

// ImmediateOf returns the wrapped Go value when value is an [ImmediateValue]; nil otherwise.
//
// Helper for callers that hold a bare [SlotValue] and need the immediate-value short-circuit pattern.
//
// Parameters:
//   - `value`: the slot value to inspect; nil is acceptable and yields nil.
//
// Returns:
//   - `any`: the wrapped value when value is an [ImmediateValue]; nil for nil or any other variant.
func ImmediateOf(value SlotValue) any {

	if iv, ok := value.(ImmediateValue); ok {
		return iv.Value
	}
	return nil
}

// ProducerIDOf returns the ID of the unit producing the given slot's value, or empty string when the
// slot has no implicit producer dependency.
//
// Resolution by SlotValue variant:
//   - [PromiseValue]: the producer is the unit named by [PromiseValue.UnitRef].
//   - [ImmediateValue] whose Value is an [op.Resource] with a non-empty [Resource.ProducerID]: the producer
//     is the catalog-stamped producer node.
//   - [VariableValue] or any other shape: no producer dependency — returns empty.
//
// Consumed by [Subgraph.MaterializeEdges] during graph assembly to emit sibling-level edges.
//
// Parameters:
//   - `value`: the slot value to inspect; nil is acceptable and yields empty string.
//
// Returns:
//   - `string`: the producer's ID, or empty string when none.
func ProducerIDOf(value SlotValue) string {

	switch v := value.(type) {
	case PromiseValue:
		return v.UnitRef
	case ImmediateValue:
		if r, ok := v.Value.(Resource); ok {
			return r.ProducerID()
		}
	}
	return ""
}

// endregion

// ImmediateValue is a Go value known at plan time.
type ImmediateValue struct {
	Value any
}

// region EXPORTED METHODS

// region Behaviors

// Resolve returns the wrapped Go value verbatim. Both inputs are ignored.
//
// Parameters:
//   - `variables`: the resolved variable map (ignored).
//   - `stack`: the recovery stack (ignored).
//
// Returns:
//   - `any`: the wrapped value.
func (iv ImmediateValue) Resolve(_ map[string]Variable, _ *RecoveryStack) any {

	return iv.Value
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// isSlotValue marks [ImmediateValue] as a sealed [SlotValue] implementation.
func (ImmediateValue) isSlotValue() {}

// endregion

// endregion

// PromiseValue references the output of another executable unit (node or subgraph), resolved to a Go value at
// execution time via [RecoveryStack.ResultByUnitID] against the active recovery stack.
type PromiseValue struct {
	UnitRef string
	Slot    string
}

// region EXPORTED METHODS

// region Behaviors

// Resolve returns the referenced producer's result by querying the recovery stack.
//
// Parameters:
//   - `variables`: the resolved variable map (ignored).
//   - `stack`: the recovery stack carrying per-dispatch receipts.
//
// Returns:
//   - `any`: the receipt's stored result for `pv.UnitRef`, or nil when `stack` is nil or no matching
//     receipt exists.
func (pv PromiseValue) Resolve(_ map[string]Variable, stack *RecoveryStack) any {

	if stack == nil {
		return nil
	}
	if v, ok := stack.ResultByUnitID(pv.UnitRef); ok {
		return v
	}
	return nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// isSlotValue marks [PromiseValue] as a sealed [SlotValue] implementation.
func (PromiseValue) isSlotValue() {}

// endregion

// endregion

// VariableValue references a [Variable] by name. Authored at plan time via plan.variable("name"); resolved
// at execution time via the variable map passed to [GraphExecutor.Run] (assembled by [VariableResolver]
// from layered sources — override / flag / env / config / default).
type VariableValue struct {
	Name string
}

// region EXPORTED METHODS

// region Behaviors

// Resolve returns the value of the named variable from the supplied variable map.
//
// Parameters:
//   - `variables`: the resolved variable map keyed by parameter name.
//   - `stack`: the recovery stack (ignored).
//
// Returns:
//   - `any`: variables[v.Name].Value, or nil if the variable is absent or the map is nil.
func (v VariableValue) Resolve(variables map[string]Variable, _ *RecoveryStack) any {

	if variables == nil {
		return nil
	}
	return variables[v.Name].Value
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// isSlotValue marks [VariableValue] as a sealed [SlotValue] implementation.
func (VariableValue) isSlotValue() {}

// endregion

// endregion
