// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"encoding/json"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// ExecutableUnit is anything the executor can dispatch: a Node or a Subgraph.
//
// Every unit carries an [Action] (the dispatch surface), an annotation map (extensible plan-time metadata), a slot map
// (parameter-name → [SlotValue] bindings), an optional retry policy, and an optional error-handler [*Subgraph]. Both
// Node and Subgraph dispatch through the same path: `unit.Action() → action.Do(activationRecord)`. Parameters reports
// the unit's input surface (the method's parameters for Node; the bubble-up variable surface for Subgraph).
//
// The interface exposes read-only accessors and the dispatch entry point only. Mutation is package-internal: the
// lowercase setters on the embedded [executableUnit] are visible to in-package builders ([NewSubgraph], [NewNode],
// [Subgraph.addChild]'s parent-stamp, the planner's slot fill, the promise resolver's slot fill, the load path's child
// linkage) but invisible across the package boundary. The construction surface ([NewGraph] / [NewSubgraph] / [NewNode])
// is the only public path for producing a fully-formed unit.
//
// stampParentID is also package-internal — exposed on the interface so the in-package mutators can stamp ownership
// without a *Node / *Subgraph type-switch. Because both setters and stampParentID are unexported, the interface is
// closed to same-package implementations — only [*Node] and [*Subgraph] satisfy it.
type ExecutableUnit interface {

	// State management

	Action() Action
	Annotations() AnnotationMap
	ErrorAction() *Subgraph
	ID() string
	Parameters() ([]Parameter, error)
	ParentID() string
	RetryPolicy() *RetryPolicy
	Slots() map[string]SlotValue

	// Behaviors

	Execute(
		ctx context.Context,
		executor *GraphExecutor,
		stack *RecoveryStack,
		variables map[string]Variable,
	) (any, error)

	// Unexported state management

	setAction(a Action)
	setErrorAction(ea *Subgraph)
	setRetryPolicy(p *RetryPolicy)
	setSlot(name string, value SlotValue)
	stampParentID(parentID string)
}

// executableUnit is the shared state embedded by Node and Subgraph.
//
// All fields are unexported; callers read through accessors and write through constructors and plan-time setters.
type executableUnit struct {
	action      Action
	annotations AnnotationMap
	errorAction *Subgraph
	id          string
	parentID    string
	retryPolicy *RetryPolicy
	slots       map[string]SlotValue
}

// newExecutableUnit builds the embedded [executableUnit] base shared by [NewNode] and [NewSubgraph].
//
// Annotations are a construction-time input — tool-specific provenance fixed at birth; there is no post-construction
// annotation mutator. The supplied map is copied via [NewAnnotationMap], detaching it from the caller.
//
// Parameters:
//   - `id`: the unit identifier.
//   - `action`: the bound dispatch action, or nil for structural units (e.g., the root subgraph).
//   - `annotations`: tool-specific annotations to stamp; nil yields the zero [AnnotationMap].
//
// Returns:
//   - `executableUnit`: the initialized base.
func newExecutableUnit(id string, action Action, annotations map[string]any) executableUnit {
	return executableUnit{id: id, action: action, annotations: NewAnnotationMap(annotations)}
}

// region State management

// Action returns the bound dispatch [Action], or nil when this unit has not been bound.
//
// Returns:
//   - `Action`: the bound action, or nil.
func (e *executableUnit) Action() Action { return e.action }

// setAction binds the dispatch [Action] on this unit.
//
// Package-internal mutator used by the construction surface and the load path.
//
// Parameters:
//   - `a`: the action to bind. Pass nil to clear.
func (e *executableUnit) setAction(a Action) { e.action = a }

// Annotations returns this unit's annotation map.
//
// Returns:
//   - `AnnotationMap`: the annotation map wrapper.
func (e *executableUnit) Annotations() AnnotationMap { return e.annotations }

// ErrorAction returns the failure-handler subgraph for this unit, or nil when no error action is configured.
//
// Returns:
//   - `*Subgraph`: the configured failure-handler subgraph, or nil. Nil defaults to the
//     flow.Provider.Failed sentinel at dispatch time.
func (e *executableUnit) ErrorAction() *Subgraph { return e.errorAction }

// setErrorAction sets the failure-handler subgraph and stamps it as a child of this unit.
//
// Stamping the `parentID` to this unit's ID ensures the post-assembly orphan scan covers `error_action=` assignments.
// Package-internal mutator used by the construction surface.
//
// Parameters:
//   - `ea`: the failure-handling subgraph, or nil to clear (no stamping when nil).
func (e *executableUnit) setErrorAction(ea *Subgraph) {

	if ea != nil {
		ea.stampParentID(e.ID())
	}

	e.errorAction = ea
}

// ID returns the identifier.
//
// Returns:
//   - `string`: the unit identifier.
func (e *executableUnit) ID() string { return e.id }

// ParentID returns the ID of the enclosing Subgraph, or the empty string when this unit has no parent.
//
// A unit has no parent when it is the graph root or has not yet been added to any Subgraph.
//
// Returns:
//   - `string`: the parent Subgraph's ID, or "".
func (e *executableUnit) ParentID() string { return e.parentID }

// stampParentID sets this unit's parentID with idempotency.
//
// First stamp (existing `parentID == ""`) and re-stamp with the same parentID both succeed silently; re-stamp with a
// different non-empty parentID panics — within a single Graph context, a unit can be a child of only one Subgraph at a
// time. Cross-graph reuse via the constant "root" ID for graph.Root is the use case the idempotency permits.
//
// Parameters:
//   - `newParentID`: the parent Subgraph's ID to stamp. Must not be empty (asserted).
func (e *executableUnit) stampParentID(newParentID string) {

	assert.NonZero("newParentID", newParentID)

	assert.Truef(e.parentID == "" || e.parentID == newParentID,
		"executableUnit %q already has parentID %q; cannot re-parent to %q",
		e.id,
		e.parentID,
		newParentID)

	e.parentID = newParentID
}

// RetryPolicy returns this unit's retry policy, or nil when no policy is configured.
//
// Returns:
//   - `*RetryPolicy`: the configured retry policy, or nil.
func (e *executableUnit) RetryPolicy() *RetryPolicy { return e.retryPolicy }

// setRetryPolicy sets this unit's retry policy.
//
// Package-internal mutator used by the construction surface and the promise builder's options-kwarg projection.
//
// Parameters:
//   - `p`: the retry policy to set. Pass nil to disable retry.
func (e *executableUnit) setRetryPolicy(p *RetryPolicy) { e.retryPolicy = p }

// Parameters on the executableUnit base is intentionally not implemented.
//
// Both [*Node] and [*Subgraph] override Parameters to return their own bubble-up variable surface; the embedded base
// has no usable default — leaf vs. composite need different walks. The [ExecutableUnit] interface declares Parameters,
// and both concrete types satisfy it via their overrides.

// Slots returns this unit's slot map, keyed by parameter name.
//
// The map aliases the unit's storage; callers must not mutate it directly — use [executableUnit.setSlot] instead.
//
// Returns:
//   - `map[string]SlotValue`: the slot map (may be nil).
func (e *executableUnit) Slots() map[string]SlotValue { return e.slots }

// setSlot sets a single slot entry on this unit.
//
// Package-internal mutator used by the construction surface, the planner's parameter-binding pass, and the promise
// resolver's slot fill.
//
// Parameters:
//   - `name`: the parameter name (or frame-binding name for non-matching slots on a Subgraph).
//   - `value`: the [SlotValue] to bind.
func (e *executableUnit) setSlot(name string, value SlotValue) {

	if e.slots == nil {
		e.slots = make(map[string]SlotValue)
	}

	e.slots[name] = value
}

// endregion

// region Behaviors

// ResolveSlots returns all slot values resolved against the per-dispatch `variables` frame.
//
// Each slot's [SlotValue.Resolve] is called with the supplied `variables` map and `stack`: [VariableValue] entries look
// up `variables[name]`; [PromiseValue] entries look up the producer's result via [RecoveryStack.ResultByUnitID];
// [ImmediateValue] entries return their stored value.
//
// Shared by [*Node] and [*Subgraph] dispatch paths in [GraphExecutor]. The `variables` map is the per-call frame
// threaded through dispatch — at top level it's the session-resolved variables; for combinator-driven sub-dispatches
// (gather's per-iteration body) it's a per-iteration frame the combinator built.
//
// Parameters:
//   - `variables`: the variable frame in scope for this dispatch.
//   - `stack`: the recovery stack; [PromiseValue.Resolve] queries it for upstream unit results.
//
// Returns:
//   - `map[string]any`: the resolved slot values, keyed by slot name.
func (e *executableUnit) ResolveSlots(variables map[string]Variable, stack *RecoveryStack) map[string]any {

	out := make(map[string]any, len(e.slots))

	for name, value := range e.slots {
		out[name] = value.Resolve(variables, stack)
	}

	return out
}

// endregion

// region SUPPORTING TYPES

// AnnotationMap is a read-only wrapper around tool-specific unit metadata.
//
// It encapsulates the raw map to ensure the immutability of a unit's annotations after construction. Serializes to
// the same JSON/YAML shape as the underlying map.
type AnnotationMap struct {
	values map[string]any
}

// NewAnnotationMap returns an [AnnotationMap] holding a detached, read-only copy of `values`.
//
// Later mutations of the source map do not bleed into the constructed map, and callers read through [AnnotationMap.Get]
// but cannot mutate it. An empty or nil `values` yields the zero [AnnotationMap].
//
// Parameters:
//   - `values`: the name → value annotations to capture.
//
// Returns:
//   - `AnnotationMap`: an immutable wrapper over a fresh copy of `values`.
func NewAnnotationMap(values map[string]any) AnnotationMap {

	if len(values) == 0 {
		return AnnotationMap{}
	}

	cp := make(map[string]any, len(values))

	for name, value := range values {
		cp[name] = value
	}

	return AnnotationMap{values: cp}
}

// Get returns the annotation value for the given name and a boolean indicating if it was present.
//
// Returns:
//   - `any`: the value associated with the name, or nil if the annotation is missing.
//   - `bool`: true if the annotation was present (even if the value is nil).
func (a AnnotationMap) Get(name string) (any, bool) {

	if a.values == nil {
		return nil, false
	}

	val, ok := a.values[name]
	return val, ok
}

// MarshalJSON ensures the wrapper serializes as a plain map.
//
// Returns:
//   - `[]byte`: the JSON encoding of the underlying annotation map.
//   - `error`: any error returned by [json.Marshal] over the underlying map.
func (a AnnotationMap) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.values)
}

// MarshalYAML ensures the wrapper serializes as a plain map.
//
// Returns:
//   - `any`: the underlying annotation map, emitted in place of the wrapper.
//   - `error`: always nil; the signature satisfies the [yaml.Marshaler] contract.
func (a AnnotationMap) MarshalYAML() (any, error) {
	return a.values, nil
}

// ExecutableUnitSpec is the construction payload shared by [NodeSpec] and [SubgraphSpec].
//
// It carries the fields common to every [ExecutableUnit] — identity, action, annotations, slot bindings, an
// optional error-action subgraph, and an optional retry policy — and exposes one fluent `With*` setter per field.
// [NodeSpec] and [SubgraphSpec] embed it and re-declare each setter to return their own type; a populated spec
// feeds [NewNode] / [NewSubgraph], which produce the sealed unit. The setters mutate the builder, never a
// constructed unit — the seal forbids post-construction mutation.
type ExecutableUnitSpec struct {
	Action      Action
	Annotations map[string]any
	ErrorAction *Subgraph
	ID          string
	RetryPolicy *RetryPolicy
	Slots       map[string]SlotValue
}

// WithAction sets the dispatch [Action] for the unit.
//
// Parameters:
//   - `action`: the [Action] to bind; nil for a structural unit (e.g. a root subgraph).
//
// Returns:
//   - `*ExecutableUnitSpec`: the receiver, for chaining.
func (s *ExecutableUnitSpec) WithAction(action Action) *ExecutableUnitSpec {
	s.Action = action
	return s
}

// WithAnnotations sets the tool-specific annotations stamped on the unit at construction.
//
// Parameters:
//   - `annotations`: the raw `map[string]any` to stamp. [NewNode] / [NewSubgraph] wrap it via
//     [NewAnnotationMap]; nil for none.
//
// Returns:
//   - `*ExecutableUnitSpec`: the receiver, for chaining.
func (s *ExecutableUnitSpec) WithAnnotations(annotations map[string]any) *ExecutableUnitSpec {
	s.Annotations = annotations
	return s
}

// WithErrorAction sets the failure-handler [Subgraph] for the unit.
//
// Parameters:
//   - `errorAction`: the handler [Subgraph], or nil for no error action.
//
// Returns:
//   - `*ExecutableUnitSpec`: the receiver, for chaining.
func (s *ExecutableUnitSpec) WithErrorAction(errorAction *Subgraph) *ExecutableUnitSpec {
	s.ErrorAction = errorAction
	return s
}

// WithID sets the unit identifier.
//
// Parameters:
//   - `id`: the unit identifier; immutable once the unit is constructed.
//
// Returns:
//   - `*ExecutableUnitSpec`: the receiver, for chaining.
func (s *ExecutableUnitSpec) WithID(id string) *ExecutableUnitSpec {
	s.ID = id
	return s
}

// WithRetryPolicy sets the [RetryPolicy] for the unit.
//
// Parameters:
//   - `retryPolicy`: the [RetryPolicy], or nil for no retry.
//
// Returns:
//   - `*ExecutableUnitSpec`: the receiver, for chaining.
func (s *ExecutableUnitSpec) WithRetryPolicy(retryPolicy *RetryPolicy) *ExecutableUnitSpec {
	s.RetryPolicy = retryPolicy
	return s
}

// WithSlot binds one slot value by parameter name, allocating the slot map on first use.
//
// Parameters:
//   - `name`: the parameter name (or frame-binding name) the slot fills.
//   - `value`: the [SlotValue] to bind.
//
// Returns:
//   - `*ExecutableUnitSpec`: the receiver, for chaining.
func (s *ExecutableUnitSpec) WithSlot(name string, value SlotValue) *ExecutableUnitSpec {

	if s.Slots == nil {
		s.Slots = make(map[string]SlotValue)
	}

	s.Slots[name] = value

	return s
}

// endregion
