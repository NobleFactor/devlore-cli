// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// region ExecutableUnit interface

// ExecutableUnit is anything the executor can dispatch — a Node or a Subgraph.
//
// Every unit carries an [Action] (the dispatch surface), an annotation map (extensible plan-time
// metadata), a slot map (parameter-name → [SlotValue] bindings), an optional retry policy, and an
// optional error-handler [*Subgraph]. Both Node and Subgraph dispatch through the same path:
// `unit.Action() → action.Do(activationRecord, slots)`. Parameters reports the unit's input surface
// (the method's parameters for Node; the bubble-up variable surface for Subgraph).
//
// Setters on the interface (SetAction, SetAnnotation, SetSlot, SetRetryPolicy, SetErrorAction) let
// plan-time machinery stamp the kwarg payload on any planner's return value without a *Node /
// *Subgraph type-switch.
//
// stampParent is package-internal — exposed on the interface so [Subgraph.AddChild] and
// [Subgraph.SetErrorAction] can stamp ownership without a *Node / *Subgraph type-switch. Because the
// method is unexported, the interface is closed to same-package implementations — only [*Node] and
// [*Subgraph] satisfy it.
//
// Step 11 (Foundation) note: the new accessors (Action, Annotations, Slots, …) coexist with the
// transitional fields on [*Node] / [*Subgraph] (e.g., `Node.Receiver`, `Node.Slots`, `Subgraph.Items`).
// Removal of those transitional fields lands in step 15.
type ExecutableUnit interface {
	SetAction(a Action)
	SetAnnotation(key, value string)
	ErrorAction() *Subgraph
	SetErrorAction(ea *Subgraph)
	ID() string
	Parameters() []Parameter
	ParentID() string
	RetryPolicy() *RetryPolicy
	SetRetryPolicy(p *RetryPolicy)
	SetSlot(name string, value SlotValue)
	stampParent(parentID string)
}

// Step 11 deferrals:
//   - `Action()` collides with [*Node]'s existing `Action() (Action, error)` method (registry lookup);
//     [Step 12 — Executor + RecoveryStack + Receipt generalization] migrates the executor to the base
//     accessor and adds `Action()` to the interface.
//   - `Annotations()` and `Slots()` collide with [*Node]'s exported `Annotations` and `Slots` fields
//     (field shadows promoted method); [Step 15 — Strip transitional shims + Slot type collapse]
//     removes the fields and adds the read accessors to the interface.

// endregion

// region executableUnit base

// executableUnit is the shared state embedded by Node and Subgraph.
//
// All fields are unexported; callers read through accessors and write through constructors,
// plan-time hooks ([Node.Bind] and friends), and the parent-stamping methods.
//
// action is the bound dispatch [Action] — what this unit invokes when the executor reaches it. Carries
// the receiver type, the [*Method], and the canonical short name. Set via [executableUnit.SetAction]
// at plan time.
//
// annotations holds extensible plan-time metadata (tags, labels, source positions). The receipt
// machinery copies these into the audit snapshot at dispatch time.
//
// slots is the unified slot map keyed by parameter name. For Node, the names match the bound
// [*Method]'s parameters; for Subgraph, names match the flow combinator's parameters with
// non-matching entries treated as frame bindings (method-signature-driven discriminator).
//
// parameters is a transitional field (removed in step 15). [Parameters] still reads it; once
// slot data lives entirely on `slots` and consumers reach the parameter surface via
// `unit.Action().Method().Parameters()`, `parameters` goes away.
//
// parentID identifies the enclosing Subgraph by ID rather than pointer (plan-doc D11). By-ID rather
// than by-pointer because containment must round-trip through plan.save / plan.load — pointers don't
// serialize. The graph's Root subgraph has parentID == "" (the only unit with an empty parentID after
// it has been wired into a Graph). Cross-Graph reuse works because graph.Root.ID is the constant
// "root" across all Graphs, so an Invocation that's a root child in two different Graphs stamps the
// same parentID value both times — idempotent.
//
// retryPolicy and errorAction are plan-doc D5 / D11 fields — every executable unit can carry them.
// Nil retryPolicy means no retry; nil errorAction defaults to the flow.Provider.Failed sentinel at
// dispatch time. errorAction is always a [*Subgraph]; authoring grammar at the .star surface is
// `error_action=[invocation, ...]`.
type executableUnit struct {
	action      Action
	annotations map[string]string
	errorAction *Subgraph
	id          string
	parameters  []Parameter
	parentID    string
	retryPolicy *RetryPolicy
	slots       map[string]SlotValue
}

// Action returns the bound dispatch [Action], or nil when this unit has not been bound.
//
// Returns:
//   - `Action`: the bound action, or nil.
func (e *executableUnit) Action() Action { return e.action }

// SetAction binds the dispatch [Action] on this unit. Plan-time mutator.
//
// Parameters:
//   - `a`: the action to bind. Pass nil to clear.
func (e *executableUnit) SetAction(a Action) { e.action = a }

// Annotations returns this unit's annotation map, or nil if no annotations are set. The returned map
// aliases the unit's storage; callers must not mutate it directly — use [executableUnit.SetAnnotation]
// instead.
//
// Returns:
//   - `map[string]string`: the annotation map (may be nil).
func (e *executableUnit) Annotations() map[string]string { return e.annotations }

// SetAnnotation sets a single annotation entry on this unit. Idempotent on (key, value) pairs.
//
// Parameters:
//   - `key`: the annotation name.
//   - `value`: the annotation value.
func (e *executableUnit) SetAnnotation(key, value string) {

	if e.annotations == nil {
		e.annotations = make(map[string]string)
	}
	e.annotations[key] = value
}

// ID returns the identifier.
func (e *executableUnit) ID() string { return e.id }

// Parameters returns the precomputed parameter surface.
//
// For Node, this is the full Go-method signature (populated by Node.Bind from method.Parameters());
// for Subgraph, this is shadowed by *Subgraph.Parameters() which computes the bubble-up variable
// surface dynamically via a graph-walk (plan-doc D3).
//
// Step 11 (Foundation) note: still reads the transitional `parameters` field. Step 15 switches the
// body to read from `e.action.Method().Parameters()` and drops the field.
func (e *executableUnit) Parameters() []Parameter { return e.parameters }

// Slots returns this unit's slot map, keyed by parameter name. The map aliases the unit's storage;
// callers must not mutate it directly — use [executableUnit.SetSlot] instead.
//
// Returns:
//   - `map[string]SlotValue`: the slot map (may be nil).
func (e *executableUnit) Slots() map[string]SlotValue { return e.slots }

// SetSlot sets a single slot entry on this unit. Plan-time mutator.
//
// Parameters:
//   - `name`: the parameter name (or frame-binding name for non-matching slots on a Subgraph).
//   - `value`: the [SlotValue] to bind.
func (e *executableUnit) SetSlot(name string, value SlotValue) {

	if e.slots == nil {
		e.slots = make(map[string]SlotValue)
	}
	e.slots[name] = value
}

// ParentID returns the ID of the enclosing Subgraph, or the empty string for the graph root (or for a
// unit that has not yet been added to any parent).
//
// Returns:
//   - string: the parent Subgraph's ID, or "".
func (e *executableUnit) ParentID() string { return e.parentID }

// RetryPolicy returns this unit's retry policy, or nil when no policy is configured.
//
// Returns:
//   - *RetryPolicy: the configured retry policy, or nil.
func (e *executableUnit) RetryPolicy() *RetryPolicy { return e.retryPolicy }

// SetRetryPolicy sets this unit's retry policy.
//
// Parameters:
//   - p: the retry policy to set. Pass nil to disable retry.
func (e *executableUnit) SetRetryPolicy(p *RetryPolicy) { e.retryPolicy = p }

// ErrorAction returns the failure-handler subgraph for this unit, or nil when no error action is
// configured.
//
// Returns:
//   - *Subgraph: the configured failure-handler subgraph, or nil. Nil defaults to the
//     flow.Provider.Failed sentinel at dispatch time.
func (e *executableUnit) ErrorAction() *Subgraph { return e.errorAction }

// SetErrorAction sets the failure-handler subgraph.
//
// The base implementation is a plain field `write`. [Subgraph.SetErrorAction] shadows it to additionally stamp
// `parentID` on the handler so the post-assembly orphan scan covers `error_action=` assignments.
//
// Parameters:
//   - `ea`: the failure-handling subgraph. Pass nil to use the default flow.Provider.Failed sentinel.
func (e *executableUnit) SetErrorAction(ea *Subgraph) { e.errorAction = ea }

// stampParent sets this unit's parentID with idempotency.
//
// Calling again with the same parentID succeeds silently; calling with a different non-empty parentID panics — within a
// single Graph context, a unit can be a child of only one Subgraph at a time. Cross-graph reuse via the constant
// "root" ID for graph.Root is the use case the idempotency permits.
//
// Parameters:
//   - `newParentID`: the parent Subgraph's ID to stamp. Must not be empty (asserted).
func (e *executableUnit) stampParent(newParentID string) {

	assert.True("newParentID not empty", newParentID != "")

	assert.Truef(e.parentID == "" || e.parentID == newParentID,
		"executableUnit %q already has parentID %q; cannot re-parent to %q",
		e.id,
		e.parentID,
		newParentID)

	e.parentID = newParentID
}

// endregion
