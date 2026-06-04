// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// Node represents a single unit of work in an execution graph.
type Node struct {
	executableUnit
}

// NewNode constructs a sealed [*Node] from a populated [*NodeSpec].
//
// Every Node dispatches to a method, so the spec's action must be non-nil — a nil action is a program-construction
// error and panics via the assert package. The spec's ID, action, annotations, slots, error action, and retry policy
// are applied here; the returned Node exposes no public setters and is immutable thereafter (the step-21 seal).
//
// Wire-form deserialization reaches the same result through [LoadGraph]: it decodes the stream into [nodeData] values
// and rebuilds each Node with its registry-resolved [Action], never leaving a Node in an action-less transient state.
//
// Parameters:
//   - `spec`: the populated node spec; must be non-nil and carry a non-nil action.
//
// Returns:
//   - `*Node`: the constructed node.
//   - `error`: reserved for future validation; nil today.
func NewNode(spec *NodeSpec) (*Node, error) {

	assert.NonZero("spec", spec)
	assert.NonZero("spec.Action", spec.Action)

	node := &Node{executableUnit: newExecutableUnit(spec.ID, spec.Action, spec.Annotations)}

	for name, value := range spec.Slots {
		node.setSlot(name, value)
	}

	if spec.RetryPolicy != nil {
		node.setRetryPolicy(spec.RetryPolicy)
	}

	if spec.ErrorAction != nil {
		node.setErrorAction(spec.ErrorAction)
	}

	return node, nil
}

// region EXPORTED METHODS

// region Behaviors

// Execute resolves slots, dispatches the action, and pushes a receipt at every exit.
//
// Entry checks are ordered: cancellation first (hard signal — `ctx.Err()` catches root/external cancel and any ancestor
// combinator's scoped cancel), then pause (soft signal — [GraphExecutor.Pause] sets a flag observed at this
// pause-point). A cancelled or paused check pushes its audit receipt and returns before the action runs.
//
// On a clean entry path, slots are resolved against the active stack (via [RecoveryStack.ResultByUnitID] for
// [PromiseValue] entries), the node-start hook fires, an [*ActivationRecord] is built, and the action's [Action.Do] is
// invoked. The audit trail — per-attempt history, outcome, captured slots, recovery state — lives on the receipt pushed
// onto `stack` at every exit. The return value is just control flow.
//
// Parameters:
//   - `ctx`: the cancellation context threaded from the parent dispatch.
//   - `executor`: the executor driving the run; provides hooks, the runtime environment, the audit-receipt helper, and
//     the pause-point hook.
//   - `stack`: the recovery stack the node's receipt pushes onto and that [PromiseValue.Resolve] queries via
//     [RecoveryStack.ResultByUnitID] for upstream unit results.
//   - `variables`: the per-call variable frame; resolves [VariableValue] slots and is stamped onto the activation for
//     the dispatched method.
//
// Returns:
//   - `any`: the dispatch's terminal result; nil on failure, cancellation, pause, or void return.
//   - `error`: non-nil on cancellation, pause ([ErrPaused]), missing action, or [Action.Do] error.
func (n *Node) Execute(
	ctx context.Context,
	executor *GraphExecutor,
	stack *RecoveryStack,
	variables map[string]Variable,
) (any, error) {

	nodeID := n.ID()

	// Exit 1: context cancelled before dispatch begins.
	if err := ctx.Err(); err != nil {
		executor.pushAuditReceipt(n, stack, nil, nil, nil, err, nil)
		return nil, fmt.Errorf("node %s: %w", nodeID, err)
	}

	// Exit 2: pause requested.
	if executor.pausePointObserved() {
		return nil, ErrPaused
	}

	// Every writer binds the Action at construction time; a nil Action here is a programming error.
	action := n.Action()
	if action == nil {
		err := fmt.Errorf("node %s: no Action bound", nodeID)
		executor.pushAuditReceipt(n, stack, nil, nil, nil, err, nil)
		return nil, err
	}

	runtimeEnvironment := executor.environment
	slots := n.ResolveSlots(variables, stack)
	executor.hooks.FireNodeStart(runtimeEnvironment, nodeID, slots)

	activationRecord := NewActivationRecord(executor.graph, n, runtimeEnvironment)
	activationRecord.Context = ctx
	activationRecord.Stack = stack
	activationRecord.Variables = variables
	activationRecord.Slots = slots
	result, complement, err := action.Do(activationRecord)

	// Exit 3: Do returned an error.
	if err != nil {
		executor.pushAuditReceipt(n, stack, slots, nil, complement, err, action)
		executor.hooks.FireNodeComplete(runtimeEnvironment, nodeID, nil, err)
		return nil, fmt.Errorf("%s: %w", action.Name(), err)
	}

	// Exit 4: successful dispatch.
	executor.pushAuditReceipt(n, stack, slots, result, complement, nil, action)
	executor.hooks.FireNodeComplete(runtimeEnvironment, nodeID, result, nil)

	return result, nil
}

// MarshalJSON projects the node to its [nodeData] wire shape and JSON-encodes it.
//
// Returns:
//   - []byte: the JSON encoding of the node's wire form.
//   - `error`: non-nil if JSON marshaling fails.
func (n *Node) MarshalJSON() ([]byte, error) { return json.Marshal(n.marshalData()) }

// MarshalYAML returns the node's [nodeData] wire shape for the YAML encoder to serialize.
//
// Returns:
//   - `any`: the [nodeData] wire-form value.
//   - `error`: always nil; present only to satisfy the yaml.Marshaler signature.
func (n *Node) MarshalYAML() (any, error) { return n.marshalData(), nil }

// Parameters returns this node's variable bubble-up surface — one [Parameter] per slot whose value is a
// [VariableValue]. Each returned entry carries the value-side variable name (the variable a caller of this node's
// containing [Subgraph] must supply) and the type / default sourced from the bound action's method signature via
// [Method.ParameterByName] on the slot name.
//
// Implements [ExecutableUnit.Parameters] so that [Subgraph.Parameters] composes its bubble-up surface uniformly via
// [ExecutableUnit.Parameters] across both Node and Subgraph children, without a per-child type switch. Callers that
// want the method's declared parameter list (the slot names / types the method expects to receive) read
// [Action.Method].Parameters() directly.
//
// Node never produces a non-nil error — there's no merging at the leaf — so the second return value exists purely for
// [ExecutableUnit.Parameters] signature alignment with [Subgraph.Parameters].
//
// Returns:
//   - []Parameter: the variable bubble-up surface; nil when no slot carries a [VariableValue].
//   - `error`: always nil for Node.
func (n *Node) Parameters() ([]Parameter, error) {

	var out []Parameter

	method := n.action.Method()

	for name, value := range n.slots {
		vv, ok := value.(VariableValue)
		if !ok {
			continue
		}
		param, _ := method.ParameterByName(name)
		out = append(out, Parameter{
			Name:    vv.Name,
			Type:    param.Type,
			Default: param.Default,
		})
	}

	return out, nil
}

// Node intentionally has no [json.Unmarshaler] / yaml.Unmarshaler. The wire form decodes into [nodeData] structs (held
// inside [graphData]); [LoadGraph] then walks those payloads and constructs each Node via [NewNode] with the
// registry-resolved [Action] in one pass — so a Node never exists in an action-less transient state outside
// [LoadGraph]'s internals.

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// marshalData projects this Node to its canonical wire shape.
//
// Returns:
//   - `nodeData`: the projected wire-form value.
func (n *Node) marshalData() nodeData {
	var actionName string
	if a := n.Action(); a != nil {
		actionName = a.Name()
	}
	return nodeData{
		ID:          n.id,
		ActionName:  actionName,
		Annotations: n.annotations.values,
		Retry:       n.RetryPolicy(),
		Slots:       n.slots,
	}
}

// endregion

// endregion

// region SUPPORTING TYPES

// NodeSpec is the fluent builder for a [*Node]. It embeds [ExecutableUnitSpec] and adds nothing — a node is a leaf
// unit — re-declaring each inherited With* to return `*NodeSpec` so the builder chain stays on the concrete type.
// Hand a populated spec to [NewNode].
type NodeSpec struct {
	ExecutableUnitSpec
}

// NewNodeSpec returns an empty [*NodeSpec] ready for fluent population via its With* setters.
//
// Returns:
//   - `*NodeSpec`: a zero-valued node spec.
func NewNodeSpec() *NodeSpec {
	return &NodeSpec{}
}

// WithAction sets the dispatch [Action] and returns the spec for chaining.
//
// Parameters:
//   - `action`: the [Action] to bind; nil for a structural unit.
//
// Returns:
//   - `*NodeSpec`: the receiver, for chaining.
func (s *NodeSpec) WithAction(action Action) *NodeSpec {
	s.ExecutableUnitSpec.WithAction(action)
	return s
}

// WithAnnotations sets the tool-specific annotations and returns the spec for chaining.
//
// Parameters:
//   - `annotations`: the raw `map[string]any` to stamp; nil for none.
//
// Returns:
//   - `*NodeSpec`: the receiver, for chaining.
func (s *NodeSpec) WithAnnotations(annotations map[string]any) *NodeSpec {
	s.ExecutableUnitSpec.WithAnnotations(annotations)
	return s
}

// WithErrorAction sets the failure-handler [Subgraph] and returns the spec for chaining.
//
// Parameters:
//   - `errorAction`: the handler [Subgraph], or nil for no error action.
//
// Returns:
//   - `*NodeSpec`: the receiver, for chaining.
func (s *NodeSpec) WithErrorAction(errorAction *Subgraph) *NodeSpec {
	s.ExecutableUnitSpec.WithErrorAction(errorAction)
	return s
}

// WithID sets the unit identifier and returns the spec for chaining.
//
// Parameters:
//   - `id`: the unit identifier.
//
// Returns:
//   - `*NodeSpec`: the receiver, for chaining.
func (s *NodeSpec) WithID(id string) *NodeSpec {
	s.ExecutableUnitSpec.WithID(id)
	return s
}

// WithRetryPolicy sets the [RetryPolicy] and returns the spec for chaining.
//
// Parameters:
//   - `retryPolicy`: the [RetryPolicy], or nil for no retry.
//
// Returns:
//   - `*NodeSpec`: the receiver, for chaining.
func (s *NodeSpec) WithRetryPolicy(retryPolicy *RetryPolicy) *NodeSpec {
	s.ExecutableUnitSpec.WithRetryPolicy(retryPolicy)
	return s
}

// WithSlot binds one slot value by parameter name and returns the spec for chaining.
//
// Parameters:
//   - `name`: the parameter name (or frame-binding name) the slot fills.
//   - `value`: the [SlotValue] to bind.
//
// Returns:
//   - `*NodeSpec`: the receiver, for chaining.
func (s *NodeSpec) WithSlot(name string, value SlotValue) *NodeSpec {
	s.ExecutableUnitSpec.WithSlot(name, value)
	return s
}

// nodeData is the canonical wire shape for Node.
//
// ActionName is the sole identity field — sourced from `unit.Action().Name()` at marshal; consumed by the post-load
// Action-rebind link pass via [RuntimeEnvironment.ActionByName] at Rebind. Status / Error / Timestamp do not round-trip
// — they live on the recovery-stack receipts at execution time. Slots serialize as an object/dict keyed by parameter
// name; values are the sealed [SlotValue] variants ([ImmediateValue], [PromiseValue], [VariableValue]).
type nodeData struct {
	ID          string               `json:"id"                     yaml:"id"`
	ActionName  string               `json:"action_name,omitempty"  yaml:"action_name,omitempty"`
	Annotations map[string]any       `json:"annotations,omitempty"  yaml:"annotations,omitempty"`
	Retry       *RetryPolicy         `json:"retry,omitempty"        yaml:"retry,omitempty"`
	Slots       map[string]SlotValue `json:"slots,omitempty"        yaml:"slots,omitempty"`
}

// endregion
