// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "github.com/NobleFactor/devlore-cli/pkg/assert"

// Binding is the value bound to a slot.
//
// It is sealed at three variants and the set is closed: [ImmediateBinding], [PromiseBinding], and [VariableBinding].
// Callers cannot extend it because the marker method [Binding.isBinding] is unexported.
//
// Resolve returns the slot's resolved Go value at execution time. The variables map carries the binding layer's
// resolved variables (one entry per [VariableBinding] slot the graph references). The recovery stack is queried by
// [PromiseBinding] to look up an upstream unit's output via [RecoveryStack.ResultByUnitID]. Variants ignore the
// parameters they do not need.
//
// Edge returns the producer→consumer dependency edge the binding induces, or nil when it induces none. Only
// [PromiseBinding] yields an edge: its producer is a unit in the graph. An immediate value has no producing
// unit, and a variable is injected from the [RuntimeEnvironment], so both return nil.
type Binding interface {
	isBinding()
	Edge(consumer string) *Edge
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

// Edge returns nil: an immediate value has no producing unit, so it induces no dependency edge.
//
// A [Resource] carried as an immediate value contributes its dependency through its own stamped producer
// ([Resource.ProducerID]), discovered wherever the resource flows — not through this binding.
//
// Parameters:
//   - `consumer`: the id of the consuming unit (ignored).
//
// Returns:
//   - `*Edge`: always nil.
func (b ImmediateBinding) Edge(_ string) *Edge {

	return nil
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

// Edge returns the dependency edge from this promise's producer unit to the consuming unit.
//
// Parameters:
//   - `consumer`: the id of the unit that consumes this binding — the edge's [Edge.To].
//
// Returns:
//   - `*Edge`: the producer→consumer dependency edge; the producer is this promise's referenced [ExecutableUnit].
func (b PromiseBinding) Edge(consumer string) *Edge {

	return &Edge{From: assert.Type[string]("promise unit ID", b.value), To: consumer}
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

	if result, ok := stack.ResultByUnitID(assert.Type[string]("promise unit ID", b.value)); ok {
		return result
	}

	return nil
}

// endregion

// VariableBinding references a [Variable] by name, optionally projecting one field of a record-valued variable.
//
// It is authored at plan time via plan.variable("name") — or, projected, via plan.variable("name", field=...) /
// plan.item(field) (phase-8 step 45) — and resolved at execution time via the variable map passed to
// [GraphExecutor.Run]. It is assembled by [VariableResolver] from layered sources: default => config => env => flag =>
// override.
type VariableBinding struct {
	binding

	// field optionally projects one field out of the resolved record; "" resolves the whole value.
	field string
}

// NewVariableBinding returns a [VariableBinding] referencing the [Variable] named `name`.
//
// Parameters:
//   - `name`: the variable name, resolved at execution time from the layered variable sources.
//
// Returns:
//   - `VariableBinding`: the binding.
func NewVariableBinding(name string) VariableBinding {

	return VariableBinding{binding: binding{value: name}}
}

// NewVariableBindingWithField returns a [VariableBinding] that resolves the [Variable] named `name` and then projects
// `field` from the record it holds (phase-8 step 45).
//
// Parameters:
//   - `name`: the variable name, resolved at execution time from the layered variable sources.
//   - `field`: the record field to project; "" resolves the whole value (equivalent to [NewVariableBinding]).
//
// Returns:
//   - `VariableBinding`: the binding.
func NewVariableBindingWithField(name, field string) VariableBinding {

	return VariableBinding{binding: binding{value: name}, field: field}
}

// region EXPORTED METHODS

// Edge returns nil: a variable is injected from the [RuntimeEnvironment] at execution time, not produced by a
// unit, so it induces no dependency edge.
//
// Parameters:
//   - `consumer`: the id of the consuming unit (ignored).
//
// Returns:
//   - `*Edge`: always nil.
func (b VariableBinding) Edge(_ string) *Edge {

	return nil
}

// Field returns the projected field name, or "" when the binding resolves the whole value.
//
// Returns:
//   - `string`: the projected field, or "".
func (b VariableBinding) Field() string {

	return b.field
}

// Name returns the referenced variable's name.
//
// Returns:
//   - `string`: the variable name.
func (b VariableBinding) Name() string {

	return assert.Type[string]("variable name", b.value)
}

// Resolve returns the value of the named variable from the supplied variable map, projected to the binding's field
// when one is set.
//
// The record arrives as the converted natural form (`map[string]any` for a .star dict). A projection against an
// absent field or a non-record value resolves nil — plan-time validation makes that unreachable for immediate
// gather items ([flow.GatherPlanner]); elsewhere the preflight required-parameter check surfaces the miss.
//
// Parameters:
//   - `variables`: the resolved variable map keyed by parameter name.
//   - `stack`: the recovery stack (ignored).
//
// Returns:
//   - `any`: the named variable's value (or its projected field), or nil if absent or the map is nil.
func (b VariableBinding) Resolve(variables map[string]Variable, _ *RecoveryStack) any {

	if variables == nil {
		return nil
	}

	value := variables[assert.Type[string]("variable name", b.value)].Value
	if b.field == "" {
		return value
	}

	record, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	return record[b.field]
}

// endregion
