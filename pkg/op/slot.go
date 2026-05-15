// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Slot binds a Method parameter to its value in a Node.
type Slot struct {
	Parameter Parameter
	Value     SlotValue
}

// region EXPORTED METHODS

// region State management

// Immediate returns the unwrapped Go value if this slot holds an ImmediateValue, or nil otherwise. Nil-safe:
// returns nil on a nil receiver.
//
// Returns:
//   - `any`: the wrapped value when the slot's value is an ImmediateValue; nil for nil receiver or any other
//     SlotValue variant.
func (s *Slot) Immediate() any {

	if s == nil {
		return nil
	}
	if iv, ok := s.Value.(ImmediateValue); ok {
		return iv.Value
	}
	return nil
}

// endregion

// endregion

// SlotValue is the value bound to a Slot. Sealed at three variants — [ImmediateValue], [PromiseValue], and
// [VariableValue]. The set is closed; callers cannot extend it because the marker method isSlotValue is
// unexported.
//
// Resolve returns the slot's resolved Go value at execution time. The variables map carries the binding
// layer's resolved variables (one entry per [VariableValue] slot the graph references); the results map
// carries completed-node outputs for promise resolution. Variants ignore the parameters they do not need.
type SlotValue interface {
	isSlotValue()
	Resolve(variables map[string]Variable, results map[string]any) any
}

// ImmediateValue is a Go value known at plan time.
type ImmediateValue struct {
	Value any
}

// region EXPORTED METHODS

// region Behaviors

// Resolve returns the wrapped Go value verbatim. The variables and results maps are ignored.
//
// Parameters:
//   - variables: the resolved variable map (ignored).
//   - results: the completed-node results map (ignored).
//
// Returns:
//   - `any`: the wrapped value.
func (iv ImmediateValue) Resolve(_ map[string]Variable, _ map[string]any) any {

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
// execution time via the scope-chain results map.
type PromiseValue struct {
	NodeRef string
	Slot    string
}

// region EXPORTED METHODS

// region Behaviors

// Resolve returns the referenced producer's result from the results map.
//
// Parameters:
//   - variables: the resolved variable map (ignored).
//   - results: the completed-node results map.
//
// Returns:
//   - `any`: results[pv.NodeRef], or nil if results is nil.
func (pv PromiseValue) Resolve(_ map[string]Variable, results map[string]any) any {

	if results == nil {
		return nil
	}
	return results[pv.NodeRef]
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
//   - variables: the resolved variable map keyed by parameter name.
//   - results: the completed-node results map (ignored).
//
// Returns:
//   - `any`: variables[v.Name].Value, or nil if the variable is absent or the map is nil.
func (v VariableValue) Resolve(variables map[string]Variable, _ map[string]any) any {

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
