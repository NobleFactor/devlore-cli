// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// Subgraph is a subsystem of the graph — a functional, structural, and transactional boundary.
//
// Subgraphs are recursive: a subgraph contains nodes and child subgraphs, forming a tree. The graph is the root of the
// tree. All subgraphs participate in the saga pattern: retry, compensation. Nodes and subgraphs are
// peers at any level — both are vertices in the same topological sort.
//
// executableUnits is the in-memory containment list; each entry is an [ExecutableUnit] (a [*Node] or a nested
// [*Subgraph]). The field is unexported to make [Subgraph.AddChild] the only mutator — that's where parent-ID stamping
// and ordering invariants are enforced. Wire-form serialization emits child IDs (plus inline child data) via custom
// marshalers in `marshalers.go`; deserialization rebuilds the slice through [Subgraph.linkChildren] once the
// surrounding Graph's unit table is populated.
type Subgraph struct {
	executableUnit

	// edges are ordering constraints between children at this level.
	//
	// Each edge references children by ID (both node IDs and subgraph IDs). Exposed via the
	// [Subgraph.Edges] accessor; mutated within pkg/op via direct field access (same-package).
	edges []Edge

	// Name is the name of the subgraph (e.g., "install").
	Name string

	// executableUnits is the in-memory containment list; each entry is an [ExecutableUnit] (a [*Node] or a
	// nested [*Subgraph]). The field is unexported to make [Subgraph.AddChild] the only mutator — that's
	// where parent-ID stamping, the parallel by-ID index, and ordering invariants are enforced. Wire-form
	// serialization emits child IDs via custom marshalers in `marshalers.go`; deserialization populates
	// the map with placeholder nil entries during unmarshal and [Subgraph.linkChildren] resolves them
	// once the surrounding Graph's unit table is built.
	executableUnits []ExecutableUnit

	// executableUnitsByID is the parallel index into executableUnits, keyed by [ExecutableUnit.ID].
	// Maintained by [Subgraph.AddChild] in lockstep with executableUnits. Powers [Subgraph.ChildByID]
	// in O(1) and edge-endpoint resolution via [Subgraph.validateEdges] at unmarshal time. During
	// unmarshal the map is pre-populated with `{id: nil}` placeholder entries (one per declared child
	// ID); [Subgraph.linkChildren] then walks the map, resolves each ID through the Graph's unit
	// table, and appends the populated unit to executableUnits in declaration order.
	executableUnitsByID map[string]ExecutableUnit
}

// NewSubgraph constructs a [Subgraph] with the given identifier.
//
// Every Subgraph must dispatch to a method, so construction requires a non-nil action — passing nil is
// a program-construction error and asserts via the assert package. Wire-form deserialization does NOT
// go through this constructor; the JSON / YAML decoder produces a zero-value Subgraph and
// [Subgraph.applyPayload] fills it from the payload, with the eventual [Graph.Rebind] resolving the
// cached action name through the registry.
//
// The parameter surface is computed lazily by [Subgraph.Parameters] via a graph-walk over children's
// slots (plan-doc D3); no precomputation needed.
//
// Parameters:
//   - `id`: the subgraph's identifier; becomes the embedded executableUnit's ID.
//   - `action`: the dispatch action; must be non-nil.
//
// Returns:
//   - *Subgraph: the constructed subgraph with `id` and `action` set; other fields at their zero values.
func NewSubgraph(id string, action Action) *Subgraph {

	assert.NonZero("op.NewSubgraph", action)
	return &Subgraph{executableUnit: executableUnit{id: id, action: action}}
}

// newRootSubgraph constructs the structural root Subgraph of a [Graph]. The root is a containment
// artifact, not a user-constructed dispatch site — it holds the top-level children and edges of the
// graph and is never invoked as a leaf. Unlike [NewSubgraph], it does not require a bound action;
// the action (if any) is supplied later by the planner / [Assemble] when the graph is materialized
// for execution.
//
// Package-internal; the only caller is [NewGraph].
//
// Parameters:
//   - `id`: the root identifier; conventionally "root".
//
// Returns:
//   - *Subgraph: the constructed root subgraph, with action initially nil.
func newRootSubgraph(id string) *Subgraph {

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

	s.executableUnits = append(s.executableUnits, child)

	if s.executableUnitsByID == nil {
		s.executableUnitsByID = make(map[string]ExecutableUnit)
	}
	s.executableUnitsByID[child.ID()] = child

	child.stampParent(s.ID())
}

// Edges returns this subgraph's edges. The returned slice aliases the underlying storage; callers
// must not mutate it directly.
//
// Returns:
//   - []Edge: the edges (may be nil).
func (s *Subgraph) Edges() []Edge { return s.edges }

// AddEdge appends an [Edge] to this subgraph's edge list.
//
// Parameters:
//   - `edge`: the edge to append.
func (s *Subgraph) AddEdge(edge Edge) { s.edges = append(s.edges, edge) }

// SetEdges replaces this subgraph's edge list.
//
// Parameters:
//   - `edges`: the new edge list. Pass nil to clear.
func (s *Subgraph) SetEdges(edges []Edge) { s.edges = edges }

// ChildByID returns this subgraph's direct child with the given ID, or nil when no direct child carries that ID.
//
// The lookup is local — it does not recurse into nested subgraphs.
//
// Parameters:
//   - `id`: the child unit's ID.
//
// Returns:
//   - `ExecutableUnit`: the matching direct child, or nil.
func (s *Subgraph) ChildByID(id string) ExecutableUnit { return s.executableUnitsByID[id] }

// Children returns this subgraph's direct children in their assembled order — topological order once
// [Subgraph.sortChildren] has run; otherwise declaration order. The returned slice aliases the unexported
// storage; callers must not mutate it.
//
// Returns:
//   - []ExecutableUnit: the direct children.
func (s *Subgraph) Children() []ExecutableUnit { return s.executableUnits }

// MaterializeEdges walks this subgraph and every nested [*Subgraph] descendant. For each direct [*Node]
// child encountered, it inspects every slot and emits an [Edge] from the slot's producer (via
// [ProducerIDOf]) to the consumer node on the enclosing subgraph's [Subgraph.Edges] list.
//
// Called by [plan.Provider.Assemble] post-rooting so each Subgraph's local edge constraints reflect the
// slot-level dependencies (PromiseValue UnitRefs and Resource producerIDs) baked into its children.
// Recurses into nested subgraphs so the whole tree is covered in one call.
func (s *Subgraph) MaterializeEdges() {

	for _, child := range s.executableUnits {
		switch t := child.(type) {
		case *Node:
			for _, value := range t.Slots() {
				if pid := ProducerIDOf(value); pid != "" {
					s.edges = append(s.edges, Edge{From: pid, To: t.ID()})
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

	for _, child := range s.executableUnits {
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
// (plan-doc D3), MINUS the variables this subgraph binds locally as frame bindings. The exposed surface is what a
// parent caller must supply when invoking this subgraph: variables already bound locally are resolved within this
// subgraph's frame at dispatch time and do not propagate up. This shadows the embedded [executableUnit.Parameters] for
// *Subgraph callers and for interface dispatch through [ExecutableUnit] on *Subgraph.
//
// Discovery is a graph-walk: for each child node, iterate its slots; for each slot whose value is a [VariableValue],
// contribute a [Parameter] under the variable's Name, sourcing Type and Default from the child's bound method via
// [Method.ParameterByName] keyed on the slot name. For each child subgraph, recurse — its [Subgraph.Parameters] already
// returns its own deduped, locally-filtered exposed surface; merge those entries into the parent's working set.
// [ImmediateValue] and [PromiseValue] slot fills do not contribute (they are intrinsically resolved at execution time).
//
// Parameters with the same name and same type collapse to one entry. Parameters with the same name and different types
// are reported as plan-time errors via [Subgraph.mergeBubbled] and joined into the returned error, because the variable
// map at runtime is keyed by name and carries one value.
//
// Method-signature-driven frame-binding filter: a Subgraph's own slot entries split into method parameters and frame
// bindings according to whether the slot name matches a parameter of the Subgraph's bound flow method. Any slot whose
// name is NOT a method parameter is a frame binding — its name is removed from the bubble-up result so callers of this
// Subgraph never need to supply it. When the Subgraph has no bound Action, every slot is treated as a frame binding
// (filter applies). An empty or nil slot map produces no filtering.
//
// Returns:
//   - []Parameter: the exposed, deduplicated bubble-up surface, in stable order by Name. Returned
//     even when error is non-nil, so callers can render a best-effort surface alongside the
//     diagnostic.
//   - `error`: an [errors.Join] of every same-name-different-type collision detected during the walk
//     plus any errors returned by child [ExecutableUnit.Parameters] calls; nil when the walk
//     succeeded without violations.
func (subgraph *Subgraph) Parameters() ([]Parameter, error) {

	seen := make(map[string]Parameter)
	var violations []error

	// Slot names on this Subgraph define the local-frame variable names this Subgraph publishes to its
	// dispatched children: every kwarg on the originating `plan.subgraph(name=…, …)` call lands in the
	// slot map and binds `name` in the new frame at dispatch. Children that reference a name defined
	// here are satisfied locally and do not bubble up.
	locals := make(map[string]bool, len(subgraph.Slots()))
	for name := range subgraph.Slots() {
		locals[name] = true
	}

	// Compose each child's bubble-up surface, masking any name defined locally by this Subgraph.
	// Each child's accumulated error (if any) is appended to violations. Each [Subgraph.mergeBubbled]
	// collision is appended too — the walk continues past every collision so both the partial bubble-
	// up surface and the full list of violations surface in a single call.
	for _, child := range subgraph.executableUnits {
		bubbled, err := child.Parameters()
		if err != nil {
			violations = append(violations, err)
		}
		for _, p := range bubbled {
			if locals[p.Name] {
				continue
			}
			if mergeErr := subgraph.mergeBubbled(seen, p); mergeErr != nil {
				violations = append(violations, mergeErr)
			}
		}
	}

	// The Subgraph's own [VariableValue] slot values contribute upward — the Subgraph itself needs
	// those outer variables resolved by its caller. Slot values that are [ImmediateValue] or
	// [PromiseValue] are self-contained (literal or sibling-resolved) and contribute nothing.
	for name, value := range subgraph.Slots() {
		vv, ok := value.(VariableValue)
		if !ok {
			continue
		}
		// Type info comes from the slot's declared parameter on the bound method, when present. The
		// slot's parameter declares the type the bound value must satisfy; the outer variable
		// referenced by `vv.Name` must therefore resolve to that type at dispatch.
		var typ reflect.Type
		var def any
		if action := subgraph.Action(); action != nil {
			if method := action.Method(); method != nil {
				if param, present := method.ParameterByName(name); present {
					typ = param.Type
					def = param.Default
				}
			}
		}
		if mergeErr := subgraph.mergeBubbled(seen, Parameter{Name: vv.Name, Type: typ, Default: def}); mergeErr != nil {
			violations = append(violations, mergeErr)
		}
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

	return out, errors.Join(violations...)
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

	if s == nil || len(s.executableUnits) == 0 {
		return nil
	}

	out := make([]string, len(s.executableUnits))

	for i, c := range s.executableUnits {
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
		for _, c := range parent.executableUnits {
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

	for _, c := range s.executableUnits {
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
		for _, c := range parent.executableUnits {
			if sg, ok := c.(*Subgraph); ok {
				out = append(out, sg)
				walk(sg)
			}
		}
	}

	walk(s)
	return out
}

// mergeBubbled merges a single bubbled [Parameter] into the seen map. Same-name + same-type entries
// dedup silently. Same-name + different-type entries consult [typesAreInterconvertible] before declaring
// a violation: when a registered conversion bridges the two types (in either direction), slot-fill at
// dispatch time performs the projection — there is no real collision. Only genuinely irreconcilable type
// pairs surface as errors; the caller appends them to its violation list and continues the walk so every
// real collision in the graph surfaces in one [Subgraph.Parameters] call (plan-doc D3 plan-time error).
//
// Variable-type selection when types differ but are interconvertible: prefer the source-side type — the
// shape a CLI flag, env var, or config value naturally produces — over framework abstractions constructed
// downstream from those primitives. Concretely: prefer types that do NOT implement [Resource] (or do not
// point to a [Resource] implementer) over Resource-typed slots, so a path-like variable's reported type
// settles to `string` rather than `*Resource`. The resolver then stores the raw source-side value; slot
// fill at dispatch converts outward to whatever each slot expects via [Convert]'s cascade.
//
// Parameters:
//   - `seen`: the accumulating dedup map keyed by variable name.
//   - `bubbled`: the candidate entry to merge.
//
// Returns:
//   - `error`: non-nil when `bubbled.Name` is already in `seen` with a type that neither matches nor
//     bridges via the conversion cascade. The seen map is not mutated in the error case.
func (s *Subgraph) mergeBubbled(seen map[string]Parameter, bubbled Parameter) error {

	existing, dup := seen[bubbled.Name]
	if !dup {
		seen[bubbled.Name] = bubbled
		return nil
	}

	if existing.Type == bubbled.Type {
		return nil
	}

	if typesAreInterconvertible(existing.Type, bubbled.Type) {
		if preferSourceSide(bubbled.Type, existing.Type) {
			seen[bubbled.Name] = bubbled
		}
		return nil
	}

	return fmt.Errorf("subgraph %q: variable %q declared with incompatible types %s and %s across slots",
		s.ID(), bubbled.Name, existing.Type, bubbled.Type)
}

// preferSourceSide reports whether `candidate` is more source-side than `incumbent` — i.e., closer to the
// shape a CLI flag / env var / config value naturally produces. Used by [Subgraph.mergeBubbled] to pick a
// stable, source-friendly type for a variable bound to differently-typed-but-interconvertible slots.
//
// Rule: a type that does NOT implement [Resource] (and is not a pointer to one) is preferred over a type
// that does. This codifies "primitives are source-side, Resources are framework-side": CLI strings, env
// strings, config values flow as their natural Go shapes; Resources are constructed downstream via the
// catalog. Among two types of equal Resource-ness, the incumbent wins (no change).
//
// Parameters:
//   - `candidate`: the bubbled type being considered.
//   - `incumbent`: the type currently held in the seen map.
//
// Returns:
//   - `bool`: true when `candidate` should replace `incumbent`.
func preferSourceSide(candidate, incumbent reflect.Type) bool {

	candidateIsResource := typeImplementsResource(candidate)
	incumbentIsResource := typeImplementsResource(incumbent)

	if incumbentIsResource && !candidateIsResource {
		return true
	}

	return false
}

// typeImplementsResource reports whether `t` is a [Resource]-implementing type, accounting for the
// conventional pattern of declaring Resource methods on the pointer receiver (so `*file.Resource`
// implements Resource while `file.Resource` does not). Returns true for both `Resource` and `*Resource`-
// shaped types.
func typeImplementsResource(t reflect.Type) bool {

	if t == nil {
		return false
	}

	if t.Implements(resourceInterfaceType) {
		return true
	}

	if t.Kind() != reflect.Pointer {
		if reflect.PointerTo(t).Implements(resourceInterfaceType) {
			return true
		}
	}

	return false
}

// sortChildren replaces this subgraph's children slice with the topologically sorted result.
//
// Called at assembly time so the in-memory order — and the serialized form — reflect the execution order. The executor
// iterates children in this order without re-sorting.
func (s *Subgraph) sortChildren() {

	s.executableUnits = topologicallySorted(s.executableUnits, s.edges)
}

// topologicallySorted returns `units` ordered topologically per the supplied `edges` using Kahn's
// algorithm. Both Nodes and Subgraphs are vertices referenced by ID. On cycles, the subset that can be
// sorted is placed first; the remaining children are appended in their original input order so dispatch
// makes forward progress. The cycle itself surfaces as a separate validation error rather than blocking
// the sort.
//
// Used by both [Subgraph.sortChildren] (post-assembly sort) and [Subgraph.linkChildren] (post-unmarshal
// sort over the placeholder symbol table once each ID has been resolved to a unit).
//
// Parameters:
//   - `units`: the unit slice to sort.
//   - `edges`: the local edge constraints.
//
// Returns:
//   - []ExecutableUnit: the topologically sorted slice.
func topologicallySorted(units []ExecutableUnit, edges []Edge) []ExecutableUnit { //nolint:gocognit // complexity is inherent to the algorithm

	if len(edges) == 0 || len(units) <= 1 {
		return units
	}

	childMap := make(map[string]ExecutableUnit, len(units))
	inDegree := make(map[string]int, len(units))
	adj := make(map[string][]string)

	for _, c := range units {
		id := c.ID()
		childMap[id] = c
		inDegree[id] = 0
	}

	for _, edge := range edges {
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

	for _, c := range units {
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

	if len(sorted) < len(units) {
		visited := make(map[string]bool, len(sorted))
		for _, c := range sorted {
			visited[c.ID()] = true
		}
		for _, c := range units {
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
