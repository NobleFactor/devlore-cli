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
//
// children is the in-memory containment list; each entry is an [ExecutableUnit] (a [*Node] or a nested [*Subgraph]).
// The field is unexported to make [Subgraph.AddChild] the only mutator — that's where parent-ID stamping and ordering
// invariants are enforced. Wire-form serialization emits child IDs (plus inline child data) via custom marshalers in
// `marshalers.go`; deserialization rebuilds the slice through [Subgraph.AddChild].
type Subgraph struct {
	executableUnit

	// Attempts records retry history (populated during execution).
	Attempts []Attempt

	// Branch marks this subgraph as a conditional branch owned by a choose action.
	//
	// Branch subgraphs are not executed directly by the top-level executor; they are dispatched by the choose action's
	// Do method.
	Branch bool

	// Compensate is the ID of the compensating subgraph for rollback.
	Compensate string

	// Edges are ordering constraints between children at this level.
	//
	// Each edge references children by ID (both node IDs and subgraph IDs).
	Edges []Edge

	// FrameBindings are the kwarg-supplied bindings the executor uses to populate this subgraph's frame at run time.
	//
	// Each entry's name is a variable name; the SlotValue (ImmediateValue, PromiseValue, or VariableValue) resolves
	// against the parent frame to produce the value bound on the new frame. Reserved kwargs (body=, items=,
	// error_action=, retry_policy=) are NOT in this map — they populate the equivalent fields on the Subgraph. Only
	// non-reserved kwargs end up here.
	FrameBindings map[string]SlotValue

	// Items is the value passed via the reserved `items=` kwarg at construction.
	//
	// For containers that iterate (gather, choose, wait_until), this is the iteration data; for plain Subgraph, it is
	// typically nil. The executor resolves the SlotValue against the parent frame at dispatch time to get the concrete
	// `[]any` passed to the container method's `items` parameter.
	Items SlotValue

	// Name is the name of the subgraph (e.g., "install").
	Name string

	// State holds execution metadata captured during the forward pass.
	//
	// The compensating subgraph reads this to know what to undo.
	State map[string]any

	// Status of this subgraph: pending, completed, failed, rolled_back, skipped.
	Status SubgraphStatus

	// children is the in-memory containment list; each entry is an [ExecutableUnit] (a [*Node] or a nested
	// [*Subgraph]). The field is unexported to make [Subgraph.AddChild] the only mutator — that's where parent-ID
	// stamping, the parallel by-ID index, and ordering invariants are enforced. Wire-form serialization emits child
	// IDs via custom marshalers in `marshalers.go`; deserialization stashes the IDs in [Subgraph.pendingChildren] and
	// rebuilds the slice through [Subgraph.linkChildren] once the surrounding Graph's unit table is populated.
	children []ExecutableUnit

	// childrenByID is the parallel index into children, keyed by [ExecutableUnit.ID]. Maintained by
	// [Subgraph.AddChild] in lockstep with children. Powers [Subgraph.ChildByID] in O(1) and edge-endpoint
	// resolution via [Subgraph.validateEdges] at unmarshal time. Slice reordering by [Subgraph.sortChildren]
	// doesn't invalidate the map — entries are keyed by ID, not by position.
	childrenByID map[string]ExecutableUnit

	// pendingChildren holds child IDs stashed by [Subgraph.UnmarshalJSON] / [Subgraph.UnmarshalYAML] before the
	// surrounding Graph has built its unit table. [Graph.applyPayload] calls [Subgraph.linkChildren] once the
	// table is populated; that method resolves each ID through [Subgraph.AddChild] and nils this slice. Always
	// nil on a fully assembled Subgraph.
	pendingChildren []string
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

// AddChild appends `child` to this subgraph, registers it in the [Subgraph.ChildByID] index, and stamps its parent
// back-reference.
//
// Centralizing wiring through this method keeps the children slice, the by-ID index, and the child's parentID in
// lockstep for both [*Node] and nested [*Subgraph] children (plan-doc D11) — callers never need to update the index
// directly. Idempotent on parentID under multi-Graph reuse: the same Invocation referenced from two different Graphs'
// assemblies both stamp `parentID = "root"` (constant Root ID) — silent success. Adding the same child to a Subgraph
// with a different ID causes a panic (a unit cannot belong to two different Subgraphs at the same time within a
// single Graph context).
//
// Parameters:
//   - `child`: the [ExecutableUnit] to attach.
func (s *Subgraph) AddChild(child ExecutableUnit) {

	s.children = append(s.children, child)

	if s.childrenByID == nil {
		s.childrenByID = make(map[string]ExecutableUnit)
	}
	s.childrenByID[child.ID()] = child

	child.stampParent(s.ID())
}

// ChildByID returns this subgraph's direct child with the given ID, or nil when no direct child carries that ID.
//
// The lookup is local — it does not recurse into nested subgraphs.
//
// Parameters:
//   - `id`: the child unit's ID.
//
// Returns:
//   - `ExecutableUnit`: the matching direct child, or nil.
func (s *Subgraph) ChildByID(id string) ExecutableUnit { return s.childrenByID[id] }

// Children returns this subgraph's direct children in their assembled order — topological order once
// [Subgraph.sortChildren] has run; otherwise declaration order. The returned slice aliases the unexported
// storage; callers must not mutate it.
//
// Returns:
//   - []ExecutableUnit: the direct children.
func (s *Subgraph) Children() []ExecutableUnit { return s.children }

// MaterializeEdges walks this subgraph and every nested [*Subgraph] descendant. For each direct [*Node]
// child encountered, it inspects every slot and emits an [Edge] from the slot's producer (via
// [Slot.ProducerID]) to the consumer node on the enclosing subgraph's [Subgraph.Edges] list.
//
// Called by [plan.Provider.Assemble] post-rooting so each Subgraph's local edge constraints reflect the
// slot-level dependencies (PromiseValue NodeRefs and Resource producerIDs) baked into its children.
// Recurses into nested subgraphs so the whole tree is covered in one call.
func (s *Subgraph) MaterializeEdges() {

	for _, child := range s.children {
		switch t := child.(type) {
		case *Node:
			for _, slot := range t.Slots {
				if pid := slot.ProducerID(); pid != "" {
					s.Edges = append(s.Edges, Edge{From: pid, To: t.ID()})
				}
			}
		case *Subgraph:
			t.MaterializeEdges()
		}
	}
}

// SetErrorAction sets this subgraph's failure-handler [*Subgraph] and stamps its parentID to this
// subgraph's ID, shadowing the embedded [executableUnit.SetErrorAction] so the post-assemble
// invariant — every Invocation Target has a non-empty parentID — covers `error_action=` assignments.
//
// The handler belongs to the Subgraph it handles errors for; stamping parent here makes it
// discoverable by the registry's orphan scan via the same parentID chain as ordinary children.
// Passing nil clears the field without stamping. Re-assignment on a non-nil handler with a
// conflicting parentID panics through [executableUnit.stampParent] (a Subgraph cannot belong to two
// different parents at the same time).
//
// Authoring shapes that supply a single non-Subgraph unit (e.g., a bare Node from
// `error_action=plan.file.notify(...)` in `.star`) are auto-wrapped into a single-child Subgraph by
// the bridge before reaching this method, so the field type stays uniform.
//
// Parameters:
//   - `ea`: the failure-handler subgraph, or nil to clear.
func (s *Subgraph) SetErrorAction(ea *Subgraph) {
	if ea != nil {
		ea.stampParent(s.ID())
	}
	s.errorAction = ea
}

// SortAll sorts this subgraph's children topologically per [Subgraph.Edges] and recurses into every
// nested [*Subgraph] child. Each Subgraph in the tree ends up with its `children` slice in topological
// order, so the executor iterates without re-sorting.
//
// Called by [plan.Provider.Assemble] post-edge-materialization so the in-memory and serialized order
// match the execution order.
func (s *Subgraph) SortAll() {

	s.sortChildren()

	for _, child := range s.children {
		if sub, ok := child.(*Subgraph); ok {
			sub.SortAll()
		}
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
// are caught as plan-time errors (panic via [assert.Truef]) because the variable map at runtime is keyed by name and
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

	for _, child := range s.children {

		switch t := child.(type) {

		case *Node:
			for _, slot := range t.Slots {

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

		case *Subgraph:
			for _, bubbled := range t.Parameters() {
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

// childIDs returns the IDs of this subgraph's direct children in the same order as [Subgraph.Children].
// Nil-safe on a nil receiver; returns nil for an empty `children` slice.
//
// Returns:
//   - []string: the IDs in containment order; nil when no children are present.
func (s *Subgraph) childIDs() []string {

	if s == nil || len(s.children) == 0 {
		return nil
	}

	out := make([]string, len(s.children))

	for i, c := range s.children {
		out[i] = c.ID()
	}

	return out
}

// descendantNodes walks this subgraph's tree and returns every [*Node] found at any depth in tree-walk
// order (depth-first, declaration order).
//
// Nil-safe on a nil receiver.
//
// Returns:
//   - []*Node: the flat node list in tree-walk order; nil when no nodes are present.
func (s *Subgraph) descendantNodes() []*Node {

	if s == nil {
		return nil
	}

	var walk func(*Subgraph)
	var out []*Node

	walk = func(parent *Subgraph) {
		for _, c := range parent.children {
			switch t := c.(type) {
			case *Node:
				out = append(out, t)
			case *Subgraph:
				walk(t)
			}
		}
	}

	walk(s)
	return out
}

// descendantSubgraphByID searches this subgraph's tree for a nested [*Subgraph] with the given ID.
//
// The receiver itself is not considered. This method is nil-safe on a nil receiver.
//
// Parameters:
//   - `id`: the Subgraph ID to find.
//
// Returns:
//   - *Subgraph: the matching descendant, or nil when no descendant has that ID.
func (s *Subgraph) descendantSubgraphByID(id string) *Subgraph {

	if s == nil {
		return nil
	}

	for _, c := range s.children {
		sg, ok := c.(*Subgraph)
		if !ok {
			continue
		}
		if sg.ID() == id {
			return sg
		}
		if found := sg.descendantSubgraphByID(id); found != nil {
			return found
		}
	}

	return nil
}

// descendantSubgraphs walks this subgraph's tree and returns every nested [*Subgraph] found at any depth.
//
// The walk excludes the receiver itself. This method is nil-safe on a nil receiver.
//
// Returns:
//   - []*Subgraph: the flat subgraph list in tree-walk order; nil when no nested subgraphs are present.
func (s *Subgraph) descendantSubgraphs() []*Subgraph {

	if s == nil {
		return nil
	}

	var walk func(*Subgraph)
	var out []*Subgraph

	walk = func(parent *Subgraph) {
		for _, c := range parent.children {
			if sg, ok := c.(*Subgraph); ok {
				out = append(out, sg)
				walk(sg)
			}
		}
	}

	walk(s)
	return out
}

// mergeBubbled merges a single bubbled [Parameter] into the seen map.
//
// This method panics via [assert.Truef] on a same-name but different-type collision (plan-doc D3 plan-time error).
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

	assert.Truef(
		"subgraph %q: variable %q declared with incompatible types %s and %s across slots",
		existing.Type == bubbled.Type,
		s.ID(),
		bubbled.Name,
		existing.Type,
		bubbled.Type)
}

// sortChildren replaces this subgraph's children slice with the result of [Subgraph.topologicallySortedChildren].
//
// Called at assembly time so the in-memory order — and the serialized form — reflect the execution order. The executor
// iterates children in this order without re-sorting.
func (s *Subgraph) sortChildren() {

	s.children = s.topologicallySortedChildren()
}

// topologicallySortedChildren returns `s.children` ordered topologically per [Subgraph.Edges] using Kahn's algorithm.
//
// Nodes and Subgraphs are peers. Both are vertices referenced by ID. On cycles, the subset that can be sorted is placed
// first; the remaining children are appended in their original declaration order so dispatch makes forward progress.
// The cycle itself will surface as a separate validation error rather than blocking the sort.
//
// Returns:
//   - []ExecutableUnit: the topologically sorted children.
func (s *Subgraph) topologicallySortedChildren() []ExecutableUnit { //nolint:gocognit // complexity is inherent to the algorithm

	if len(s.Edges) == 0 || len(s.children) <= 1 {
		return s.children
	}

	childMap := make(map[string]ExecutableUnit, len(s.children))
	inDegree := make(map[string]int, len(s.children))
	adj := make(map[string][]string)

	for _, c := range s.children {
		id := c.ID()
		childMap[id] = c
		inDegree[id] = 0
	}

	for _, edge := range s.Edges {
		if _, fromOK := childMap[edge.From]; !fromOK {
			continue
		}
		if _, toOK := childMap[edge.To]; !toOK {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To)
		inDegree[edge.To]++
	}

	var queue []string

	for _, c := range s.children {
		id := c.ID()
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}

	var sorted []ExecutableUnit

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		sorted = append(sorted, childMap[id])

		for _, neighbor := range adj[id] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(sorted) < len(s.children) {
		visited := make(map[string]bool, len(sorted))
		for _, c := range sorted {
			visited[c.ID()] = true
		}
		for _, c := range s.children {
			if !visited[c.ID()] {
				sorted = append(sorted, c)
			}
		}
	}

	return sorted
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

	// Subgraph is the subgraph name that was rolled back.
	Subgraph string `json:"subgraph" yaml:"subgraph"`

	// Compensate is the ID of the compensating subgraph.
	Compensate string `json:"compensate" yaml:"compensate"`

	// Status is "completed" or "failed".
	Status string `json:"status" yaml:"status"`

	// Error is the error message if the compensating action failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`
}

// SubgraphStatus represents the execution state of a subgraph.
type SubgraphStatus string

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
