// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"sort"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// Subgraph is a subsystem of the graph — a functional, structural, and transactional boundary.
//
// Subgraphs are recursive: a subgraph contains nodes and child subgraphs, forming a tree. The graph is the root of the
// tree. All subgraphs participate in the saga pattern: retry, compensation, status tracking. Nodes and subgraphs are
// peers at any level — both are vertices in the same topological sort.
type Subgraph struct {
	executableUnit

	// Name is the name of the subgraph (e.g., "install").
	Name string

	// Children are the nodes and child subgraphs in declaration order. Execution proceeds through this list after
	// topological sorting by edges at this level.
	Children []SubgraphChild

	// Edges are ordering constraints between children at this level. Each edge references children by ID (both node IDs
	// and subgraph IDs).
	Edges []Edge

	// Status of this subgraph: pending, completed, failed, rolled_back, skipped.
	Status SubgraphStatus

	// Items is the value passed via the reserved `items=` kwarg at construction. For containers that iterate (gather,
	// choose, wait_until), this is the iteration data; for plain Subgraph, it is typically nil. The executor resolves
	// the SlotValue against the parent frame at dispatch time to get the concrete `[]any` passed to the container
	// method's `items` parameter.
	Items SlotValue

	// FrameBindings are the kwarg-supplied bindings the executor uses to populate this subgraph's frame at dispatch
	// time. Each entry's name is a variable name; the SlotValue (ImmediateValue, PromiseValue, or VariableValue)
	// resolves against the parent frame to produce the value bound on the new frame.
	//
	// Reserved kwargs (body=, items=, error_action=, retry_policy=) are NOT in this map — they populate the equivalent
	// fields on the Subgraph. Only non-reserved kwargs end up here.
	FrameBindings map[string]SlotValue

	// Compensate is the ID of the compensating subgraph for rollback.
	Compensate string

	// Attempts records retry history (populated during execution).
	Attempts []Attempt

	// State holds execution metadata captured during the forward pass. The compensating subgraph reads this to know
	// what to undo.
	State map[string]any

	// Branch marks this subgraph as a conditional branch owned by a choose action. Branch subgraphs are not executed
	// directly by the top-level executor; they are dispatched by the choose action's Do method.
	Branch bool
}

// NewSubgraph constructs a [Subgraph] with the given identifier.
//
// Additional fields may be set on the returned pointer. The parameter surface is computed lazily by
// [Subgraph.Parameters] via a graph-walk over children's slots (plan-doc D3); no precomputation needed.
//
// Parameters:
//   - `id`: the subgraph's identifier; becomes the embedded executableUnit's ID.
//
// Returns:
//   - *Subgraph: the constructed subgraph with only its ID set and all other fields at their zero values.
func NewSubgraph(id string) *Subgraph {

	return &Subgraph{executableUnit: executableUnit{id: id}}
}

// region EXPORTED METHODS

// region State management

// AddChild appends an [ExecutableUnit] to this subgraph's Children and stamps its parentID to this subgraph's ID.
//
// Centralizing wiring through this method keeps ownership accurate (plan-doc D11) without callers having to remember to
// maintain the back-reference themselves.
//
// Idempotent on parentID under multi-Graph reuse: the same Invocation referenced from two different Graphs' assemblies
// both stamp `parentID = "root"` (constant Root ID) — silent success. Adding the same child to a Subgraph with a
// different ID causes a panic (a unit cannot belong to two different Subgraphs at the same time within a single Graph
// context).
//
// Parameters:
//   - `child`: the [SubgraphChild] variant to attach. Exactly one of Node or Subgraph must be set; AddChild stamps
//     parentID on whichever is present.
func (s *Subgraph) AddChild(child SubgraphChild) {

	s.Children = append(s.Children, child)

	if child.Node != nil {
		child.Node.stampParent(s.ID())
	}

	if child.Subgraph != nil {
		child.Subgraph.stampParent(s.ID())
	}
}

// endregion

// region Behaviors

// Parameters are the exposed bubble-up variable surface of this subgraph.
//
// The deduplicated set of [VariableValue] references walked across every child's slots, recursing into nested subgraphs
// (plan-doc D3), MINUS the variables this subgraph binds locally via [Subgraph.FrameBindings]. The exposed surface is
// what a parent caller must supply when invoking this subgraph: variables already bound locally are resolved within
// this subgraph's frame at dispatch time and do not propagate up. This shadows the embedded [executableUnit.Parameters]
// for *Subgraph callers and for interface dispatch through [ExecutableUnit] on *Subgraph.
//
// Discovery is a graph-walk: for each child node, iterate its slots; for each slot whose Value is a [VariableValue],
// contribute a [Parameter] under the variable's Name, carrying the slot's declared Type and Default. For each child
// subgraph, recurse — its [Subgraph.Parameters] already returns its own deduped, locally-filtered exposed surface;
// merge those entries into the parent's working set. [ImmediateValue] and [PromiseValue] slot fills do not contribute
// (they are intrinsically resolved at execution time).
//
// Parameters with the same name and same type collapse to one entry. Parameters with the same name and different types
// are caught as plan-time errors (panic via [assert.Failf]) because the variable map at runtime is keyed by name and
// carries one value.
//
// After the bubble-up walk completes, any entry whose Name is a key in [Subgraph.FrameBindings] is removed from the
// result — those variables are bound locally and not part of the caller's interface. An empty or nil FrameBindings
// map produces no filtering.
//
// Returns:
//   - []Parameter: the exposed, deduplicated bubble-up surface, in stable order by Name.
func (s *Subgraph) Parameters() []Parameter {

	seen := make(map[string]Parameter)

	for _, child := range s.Children {

		if child.Node != nil {
			for _, slot := range child.Node.Slots {

				vv, ok := slot.Value.(VariableValue)
				if !ok {
					continue
				}

				bubbled := Parameter{
					Name:    vv.Name,
					Type:    slot.Parameter.Type,
					Default: slot.Parameter.Default,
				}

				s.mergeBubbled(seen, bubbled)
			}
			continue
		}

		if child.Subgraph != nil {
			for _, bubbled := range child.Subgraph.Parameters() {
				s.mergeBubbled(seen, bubbled)
			}
		}
	}

	for name := range s.FrameBindings {
		delete(seen, name)
	}

	out := make([]Parameter, 0, len(seen))
	names := make([]string, 0, len(seen))

	for name := range seen {
		names = append(names, name)
	}

	sort.Strings(names)
	for _, name := range names {
		out = append(out, seen[name])
	}

	return out
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// mergeBubbled merges a single bubbled [Parameter] into the seen map.
//
// This method panics via [assert.Failf] on a same-name but different-type collision (plan-doc D3 plan-time error).
//
// Parameters:
//   - `seen`: the accumulating dedup map keyed by variable name.
//   - `bubbled`: the candidate entry to merge.
func (s *Subgraph) mergeBubbled(seen map[string]Parameter, bubbled Parameter) {

	existing, dup := seen[bubbled.Name]
	if !dup {
		seen[bubbled.Name] = bubbled
		return
	}

	if existing.Type != bubbled.Type {
		assert.Failf("subgraph %q: variable %q declared with incompatible types %s and %s across slots",
			s.ID(),
			bubbled.Name,
			existing.Type,
			bubbled.Type)
	}
}

// endregion

// endregion

// Attempt records one execution attempt of a subgraph.
type Attempt struct {

	// Number is the 1-based attempt number.
	Number int `json:"number" yaml:"number"`

	// Status is "completed" or "failed".
	Status string `json:"status" yaml:"status"`

	// Error is the error message if the attempt failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	// Timestamp is when this attempt completed (RFC3339).
	Timestamp string `json:"timestamp" yaml:"timestamp"`
}

// RollbackEntry records a compensating action executed during rollback.
type RollbackEntry struct {

	// Subgraph is the name of the subgraph that was rolled back.
	Subgraph string `json:"subgraph" yaml:"subgraph"`

	// Compensate is the ID of the compensating subgraph.
	Compensate string `json:"compensate" yaml:"compensate"`

	// Status is "completed" or "failed".
	Status string `json:"status" yaml:"status"`

	// Error is the error message if the compensating action failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`
}

// SubgraphChild is a child of a subgraph or graph — either a node or a nested subgraph.
//
// It is eitherExactly one field is set.
type SubgraphChild struct {

	// Node is the [Node] this child wraps; nil when this child wraps a Subgraph.
	Node *Node `json:"node,omitempty" yaml:"node,omitempty"`

	// Subgraph is the [Subgraph] this child wraps; nil when this child wraps a Node.
	Subgraph *Subgraph `json:"subgraph,omitempty" yaml:"subgraph,omitempty"`
}

// region EXPORTED METHODS

// region Behaviors

// ChildID returns the identifier of the underlying unit, whether it is a [Node] or a [Subgraph].
//
// Returns:
//   - `string`: the underlying unit's ID, or the empty string when neither field is set.
func (c SubgraphChild) ChildID() string {

	if c.Node != nil {
		return c.Node.ID()
	}

	if c.Subgraph != nil {
		return c.Subgraph.ID()
	}

	return ""
}

// endregion

// endregion

// SubgraphStatus represents the execution state of a subgraph.
type SubgraphStatus string

// SubgraphStatus constants define the possible states of a subgraph.
const (
	// SubgraphPending indicates the subgraph has not yet been executed.
	SubgraphPending SubgraphStatus = "pending"

	// SubgraphCompleted indicates the subgraph executed successfully.
	SubgraphCompleted SubgraphStatus = "completed"

	// SubgraphFailed indicates the subgraph failed during execution.
	SubgraphFailed SubgraphStatus = "failed"

	// SubgraphRolledBack indicates the subgraph was rolled back after failure.
	SubgraphRolledBack SubgraphStatus = "rolled_back"

	// SubgraphSkipped indicates the subgraph was skipped.
	SubgraphSkipped SubgraphStatus = "skipped"
)
