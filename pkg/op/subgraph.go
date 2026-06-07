// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// Interface guard: *Subgraph completes the sealed [ExecutableUnit]
var _ ExecutableUnit = (*Subgraph)(nil)

// Subgraph is a subsystem of the graph — a functional, structural, and transactional boundary.
//
// Subgraphs are recursive: a subgraph contains nodes and child subgraphs, forming a tree. The graph is the root of the
// tree. All subgraphs participate in the saga pattern: retry, compensation. Nodes and subgraphs are peers at any level
// — both are vertices in the same topological sort.
//
// executableUnits is the in-memory containment list; each entry is an [ExecutableUnit] (a [*Node] or a nested
// [*Subgraph]). The field is unexported to make [Subgraph.AddChild] the only mutator — that's where parent-ID stamping
// and ordering invariants are enforced. Serialization emits child IDs (plus inline child data) via custom
// marshalers in `marshalers.go`; deserialization rebuilds the slice through [Subgraph.linkChildren] once the
// surrounding Graph's unit table is populated.
type Subgraph struct {
	executableUnit

	// edges are ordering constraints between children at this level.
	//
	// Each edge references children by ID (both node IDs and subgraph IDs). Exposed via the [Subgraph.Edges] accessor;
	// mutated within pkg/op via direct field access (same-package).
	edges []Edge

	// Name is the name of the subgraph (e.g., "install").
	Name string

	// executableUnits is the in-memory containment list; each entry is an [ExecutableUnit] (a [*Node] or a nested
	// [*Subgraph]). The field is unexported to make [Subgraph.AddChild] the only mutator — that's where parent-ID
	// stamping, the parallel by-ID index, and ordering invariants are enforced. Serialization emits child IDs
	// via custom marshalers in `marshalers.go`; deserialization populates the map with placeholder nil entries during
	// unmarshal and [Subgraph.linkChildren] resolves them once the surrounding Graph's unit table is built.
	executableUnits []ExecutableUnit

	// executableUnitsByID is the parallel index into executableUnits, keyed by [ExecutableUnit.ID]. Maintained by
	// [Subgraph.AddChild] in lockstep with executableUnits. Powers [Subgraph.ChildByID] in O(1) and edge-endpoint
	// resolution via [Subgraph.validateEdges] at unmarshal time. During unmarshal the map is pre-populated with `{id:
	// nil}` placeholder entries (one per declared child ID); [Subgraph.linkChildren] then walks the map, resolves each
	// ID through the Graph's unit table, and appends the populated unit to executableUnits in declaration order.
	executableUnitsByID map[string]ExecutableUnit
}

// NewSubgraph constructs a sealed [*Subgraph] from a populated [*SubgraphSpec].
//
// Every Subgraph binds a method, by a resolved [Action] (`spec.Action`) OR by name (`spec.ActionName`, resolved lazily
// at dispatch). At least one must be present — a spec with neither is a program-construction error and panics via the
// assert package, because there is no structural child-walk: every subgraph (the root included) dispatches through its
// bound action. The returned Subgraph carries no public setters: the spec's children, slots, retry policy, and
// error-action subgraph are applied here, edges are materialized from the children's promise / resource references, and
// the children are topologically sorted. Immutable thereafter (the step-21 seal). Mirrors the [NewGraph] shape one
// level down.
//
// Parameters:
//   - `spec`: the populated subgraph spec; must be non-nil and carry a non-nil action OR a non-empty action name.
//
// Returns:
//   - `*Subgraph`: the sealed subgraph.
//   - `error`: reserved for future validation; nil today.
func NewSubgraph(spec *SubgraphSpec) (*Subgraph, error) {

	assert.NonZero("spec", spec)
	assert.Truef(spec.Action != nil || spec.ActionName != "",
		"NewSubgraph: spec %q binds no action (neither spec.Action nor spec.ActionName set)", spec.ID)

	s := &Subgraph{executableUnit: newExecutableUnit(spec.ID, spec.Action, spec.Annotations)}
	s.populate(spec)

	return s, nil
}

// region EXPORTED METHODS

// region State management

// ChildByID returns this subgraph's direct child with the given ID, or nil when no direct child carries that ID.
//
// The lookup is local — it does not recurse into nested subgraphs.
//
// Parameters:
//   - `id`: the child unit's ID.
//
// Returns:
//   - `ExecutableUnit`: the matching direct child, or nil.
func (s *Subgraph) ChildByID(id string) ExecutableUnit {

	return s.executableUnitsByID[id]
}

// Children returns this subgraph's direct children in their assembled order.
//
// Topological order once [Subgraph.sortChildren] has run; otherwise declaration order. The returned slice aliases the
// unexported storage; callers must not mutate it.
//
// Returns:
//   - `[]ExecutableUnit`: the direct children.
func (s *Subgraph) Children() []ExecutableUnit {

	return s.executableUnits
}

// Edges returns this subgraph's edges.
//
// The returned slice aliases the underlying storage; callers must not mutate it directly.
//
// Returns:
//   - `[]Edge`: the edges (it may be nil).
func (s *Subgraph) Edges() []Edge {

	return s.edges
}

// endregion

// region Behaviors

// Execute dispatches this subgraph through its bound action.
//
// Every subgraph — including the graph root — binds an action: a resolved [Action] (`subgraph.Action() != nil`,
// the planner-built shape) or a name (`subgraph.ActionName() != ""`, the root's shape) resolved lazily at dispatch via
// [RuntimeEnvironment.ActionByName]. flow.Subgraph / flow.Gather / flow.Choose / flow.WaitUntil all reach this path:
// the subgraph's own slots are resolved (matching the bound method's parameter list); the activation is built with the
// subgraph as `Unit`; the action's [Action.Do] is invoked. The flow method's body orchestrates the children walk + any
// per-iteration semantics (retry, errorAction, frame minting). There is no structural child-walk — a subgraph that
// binds neither shape is a construction error ([NewSubgraph] rejects it) and surfaces as a no-Action-bound failure.
//
// Entry checks mirror [Node.Execute]: cancellation first (hard signal), then pause (soft signal via
// [GraphExecutor.Pause]). The audit-receipt push happens at every exit (canceled, paused, action error, success),
// stamped with the subgraph's ID. Subgraph hooks ([HookRegistry.FireSubgraphStart] /
// [HookRegistry.FireSubgraphComplete]) fire around the dispatch.
//
// Parameters:
//   - `ctx`: the cancellation context threaded from the parent dispatch.
//   - `executor`: the executor driving the run; provides hooks, the runtime environment, the audit-receipt helper, and
//     the pause-point hook.
//   - `stack`: the recovery stack child compensations push onto and that [PromiseValue.Resolve] query via
//     [RecoveryStack.ResultByUnitID] for upstream unit results.
//   - `variables`: the per-call variable frame; passed through to child dispatches and stamped onto the activation for
//     the bound flow method.
//
// Returns:
//   - `any`: the subgraph's terminal result, or nil for dispatches whose action produces no output, or on
//     pause/failure.
//   - `error`: non-nil on cancellation, pause ([ErrPaused]), an unbound subgraph, or a bound action's failure.
func (s *Subgraph) Execute(
	ctx context.Context,
	executor *GraphExecutor,
	stack *RecoveryStack,
	variables map[string]Variable,
) (any, error) {

	subgraphID := s.ID()
	runtimeEnvironment := executor.environment

	// Exit 1: context canceled before dispatch begins.

	if err := ctx.Err(); err != nil {
		executor.pushAuditReceipt(s, stack, nil, nil, nil, err, nil)
		return nil, fmt.Errorf("subgraph %s: %w", subgraphID, err)
	}

	// Exit 2: pause requested.

	if executor.pausePointObserved() {
		return nil, ErrPaused
	}

	action := s.Action()

	// A unit may bind its action by name (no resolved Action in scope at construction); resolve it now via the
	// runtime environment. A resolved Action (action != nil) wins and is used as-is. Both empty falls through to
	// the no-Action-bound error guard below — there is no structural child-walk.

	if action == nil && s.ActionName() != "" {

		resolved, err := runtimeEnvironment.ActionByName(s.ActionName())
		if err != nil {
			executor.pushAuditReceipt(s, stack, nil, nil, nil, err, nil)
			return nil, fmt.Errorf("subgraph %s: resolve action %q: %w", subgraphID, s.ActionName(), err)
		}

		action = resolved
	}

	// A subgraph bound neither by a resolved Action nor by a resolvable name is a program-construction error
	// ([NewSubgraph] rejects both-empty). Mirrors [Node.Execute]'s no-action guard — there is no structural
	// child-walk anymore; every subgraph dispatches through its bound action.

	if action == nil {
		err := fmt.Errorf("subgraph %s: no Action bound", subgraphID)
		executor.pushAuditReceipt(s, stack, nil, nil, nil, err, nil)
		return nil, err
	}

	slots := s.ResolveSlots(variables, stack)
	executor.hooks.FireSubgraphStart(runtimeEnvironment, subgraphID)

	activationRecord := NewActivationRecord(executor.graph, s, runtimeEnvironment)
	activationRecord.Context = ctx
	activationRecord.Stack = stack
	activationRecord.Variables = variables
	activationRecord.Slots = slots
	activationRecord.dispatchChild = func(
		childCtx context.Context,
		child ExecutableUnit,
		subStack *RecoveryStack,
		childVars map[string]Variable,
	) (any, error) {
		return child.Execute(childCtx, executor, subStack, childVars)
	}
	result, complement, err := action.Do(activationRecord)

	// Exit 3: Do returned an error.

	if err != nil {
		executor.pushAuditReceipt(s, stack, slots, nil, complement, err, action)
		executor.hooks.FireSubgraphComplete(runtimeEnvironment, subgraphID, err)
		return nil, fmt.Errorf("subgraph %s: %s: %w", subgraphID, action.Name(), err)
	}

	// Exit 4: successful dispatch.

	executor.pushAuditReceipt(s, stack, slots, result, complement, nil, action)
	executor.hooks.FireSubgraphComplete(runtimeEnvironment, subgraphID, nil)

	return result, nil
}

// MarshalJSON encodes this subgraph to its canonical JSON document via [Subgraph.marshalData].
//
// Returns:
//   - `[]byte`: the encoded JSON payload.
//   - `error`: any error reported by [json.Marshal].
func (s *Subgraph) MarshalJSON() ([]byte, error) { return json.Marshal(s.marshalData()) }

// MarshalYAML projects this subgraph to its canonical YAML value via [Subgraph.marshalData].
//
// Returns:
//   - `any`: the [subgraphData] projection handed to the YAML encoder.
//   - `error`: always nil; present to satisfy the yaml.Marshaler interface.
func (s *Subgraph) MarshalYAML() (any, error) { return s.marshalData(), nil }

// Parameters are the exposed bubble-up variable surface of this subgraph.
//
// The deduplicated set of [VariableValue] references walked across every child's slots, recursing into nested subgraphs
// (plan-doc D3), MINUS the variables this subgraph binds locally as frame bindings. The exposed surface is what a
// parent caller must supply when invoking this subgraph: variables already bound locally are resolved within this
// subgraph's frame at dispatch time and do not propagate up. `*Subgraph` supplies this as its own implementation of
// [ExecutableUnit.Parameters] — the embedded [executableUnit] base provides none — so it is the surface seen by both
// direct `*Subgraph` callers and interface dispatch through [ExecutableUnit].
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
//   - `[]Parameter`: the exposed, deduplicated bubble-up surface, in stable order by Name. Returned even when error is
//     non-nil, so callers can render a best-effort surface alongside the diagnostic.
//   - `error`: an [errors.Join] of every same-name-different-type collision detected during the walk plus any errors
//     returned by child [ExecutableUnit.Parameters] calls; nil when the walk succeeded without violations.
func (s *Subgraph) Parameters() ([]Parameter, error) {

	seen := make(map[string]Parameter)
	var violations []error

	violations = append(violations, s.bubbleChildParameters(seen)...)
	violations = append(violations, s.bubbleOwnSlots(seen)...)

	names := make([]string, 0, len(seen))

	for name := range seen {
		names = append(names, name)
	}

	sort.Strings(names)
	out := make([]Parameter, 0, len(seen))

	for _, name := range names {
		out = append(out, seen[name])
	}

	return out, errors.Join(violations...)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region State management

// addChild appends `child` to this subgraph and stamps the parent back-reference.
//
// This is a package-internal mutator used by the construction surface ([NewSubgraph]'s populate body) and by the load
// path's child linkage.
//
// Also registers the child in the [Subgraph.ChildByID] index. Centralizing wiring through this method keeps the
// children slice, the by-ID index, and the child's parentID in lockstep for both [*Node] and nested [*Subgraph]
// children (plan-doc D11) — callers never need to update the index directly. Idempotent on parentID under multi-Graph
// reuse: the same Invocation referenced from two different Graphs' assemblies both stamp `parentID = "root"`
// (constant Root ID) — silent success. Adding the same child to a Subgraph with a different ID causes a panic (a unit
// cannot belong to two different Subgraphs at the same time within a single Graph context).
//
// Parameters:
//   - `child`: the [ExecutableUnit] to attach.
func (s *Subgraph) addChild(child ExecutableUnit) {

	s.executableUnits = append(s.executableUnits, child)

	if s.executableUnitsByID == nil {
		s.executableUnitsByID = make(map[string]ExecutableUnit)
	}

	s.executableUnitsByID[child.ID()] = child
	child.stampParentID(s.ID())
}

// addEdge appends `edge` to this subgraph's edge list.
//
// Package-internal mutator used by the construction surface and by the load path's edge restoration.
//
// Parameters:
//   - `edge`: the edge to append.
func (s *Subgraph) addEdge(edge Edge) {

	s.edges = append(s.edges, edge)
}

// setEdges replaces this subgraph's edge list with `edges`.
//
// Package-internal mutator used by the load path to restore edges from the serialized payload.
//
// Parameters:
//   - `edges`: the new edge list. Pass nil to clear.
func (s *Subgraph) setEdges(edges []Edge) {

	s.edges = edges
}

// endregion

// region Behaviors

// bubbleChildParameters folds each child's bubble-up surface into `seen`.
//
// Slot names on this Subgraph define the local-frame variable names it publishes to its dispatched children: every
// kwarg on the originating `plan.subgraph(name=…, …)` call lands in the slot map and binds `name` in the new frame at
// dispatch. Children that reference a name defined here are satisfied locally and do not bubble up.
//
// Walks every child via [ExecutableUnit.Parameters], skips any name present in the locally-bound set, and routes each
// remaining parameter through [Subgraph.mergeBubbled]. Each child's own error is collected verbatim; each collision
// detected by mergeBubbled is collected too — the walk continues past every collision so the partial bubble-up surface
// and the full violation list surface in a single [Subgraph.Parameters] call.
//
// Parameters:
//   - `seen`: the accumulating surface map (mutated in place on hit).
//
// Returns:
//   - `[]error`: child Parameters() errors and merge collisions in encounter order; nil on clean walk.
func (s *Subgraph) bubbleChildParameters(seen map[string]Parameter) []error {

	locals := make(map[string]bool, len(s.Slots()))

	for name := range s.Slots() {
		locals[name] = true
	}

	var violations []error

	for _, child := range s.executableUnits {

		bubbled, err := child.Parameters()
		if err != nil {
			violations = append(violations, err)
		}

		for _, p := range bubbled {
			if locals[p.Name] {
				continue
			}
			if mergeErr := s.mergeBubbled(seen, p); mergeErr != nil {
				violations = append(violations, mergeErr)
			}
		}
	}

	return violations
}

// bubbleOwnSlots contributes the Subgraph's own [VariableValue] slot references to `seen`.
//
// Slot values that are [ImmediateValue] or [PromiseValue] are self-contained (literal or sibling-resolved) and
// contribute nothing. Only [VariableValue] slot fills name an outer variable that the parent must resolve. The type and
// default for each contributed Parameter come from [Subgraph.slotParameterType].
//
// Parameters:
//   - `seen`: the accumulating surface map (mutated in place on hit).
//
// Returns:
//   - `[]error`: every merge collision detected by [Subgraph.mergeBubbled]; nil on clean walk.
func (s *Subgraph) bubbleOwnSlots(seen map[string]Parameter) []error {

	var violations []error

	for name, value := range s.Slots() {

		vv, ok := value.(VariableValue)
		if !ok {
			continue
		}

		typ, def := s.slotParameterType(name)

		mergeErr := s.mergeBubbled(seen, Parameter{Name: vv.Name, Type: typ, Default: def})
		if mergeErr != nil {
			violations = append(violations, mergeErr)
		}
	}

	return violations
}

// childIDs returns the IDs of this subgraph's direct children in the same order as [Subgraph.Children].
//
// Nil-safe on a nil receiver; returns nil for an empty `children` slice.
//
// Returns:
//   - `[]string`: the IDs in containment order; nil when no children are present.
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

// descendantNodes walks this subgraph's tree and returns every [*Node] found at any depth.
//
// Tree-walk order: depth-first, declaration order.
//
// Nil-safe on a nil receiver.
//
// Returns:
//   - `[]*Node`: the flat node list in tree-walk order; nil when no nodes are present.
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
//   - `*Subgraph`: the matching descendant, or nil when no descendant has that ID.
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
//   - `[]*Subgraph`: the flat subgraph list in tree-walk order; nil when no nested subgraphs are present.
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

// linkChildren populates [Subgraph.executableUnits] from placeholder child IDs, in topological order.
//
// Each placeholder entry in [Subgraph.executableUnitsByID] is resolved against the unit table built by [assembleGraph];
// the resolved children are then ordered per [Subgraph.Edges].
//
// Map iteration order is unstable, so the final slice order is established by Kahn's topological sort over the local edge
// set. Ties between roots are broken by ID for determinism.
//
// Parameters:
//   - `unitsByID`: the Graph's unit symbol table, keyed by [ExecutableUnit.ID].
//
// Returns:
//   - `error`: non-nil if any placeholder ID is missing from `unitsByID`.
func (s *Subgraph) linkChildren(unitsByID map[string]ExecutableUnit) error {

	if len(s.executableUnitsByID) == 0 {
		return nil
	}

	ids := make([]string, 0, len(s.executableUnitsByID))

	for id := range s.executableUnitsByID {
		ids = append(ids, id)
	}

	sort.Strings(ids)
	resolved := make([]ExecutableUnit, 0, len(ids))

	for _, id := range ids {

		child, ok := unitsByID[id]
		if !ok {
			return fmt.Errorf("subgraph %q: child %q not in unit table", s.ID(), id)
		}

		resolved = append(resolved, child)
		s.executableUnitsByID[id] = child
		child.stampParentID(s.ID())
	}

	s.executableUnits = topologicallySorted(resolved, s.edges)
	return nil
}

// marshalData projects this Subgraph to its canonical serialized shape.
//
// Returns:
//   - `subgraphData`: the projected payload.
func (s *Subgraph) marshalData() subgraphData {

	var actionName string

	if a := s.Action(); a != nil {
		actionName = a.Name()
	}

	return subgraphData{
		ID:          s.id,
		Name:        s.Name,
		ActionName:  actionName,
		Annotations: s.annotations,
		Children:    s.childIDs(),
		Edges:       s.edges,
		Retry:       s.RetryPolicy(),
	}
}

// materializeEdges walks this subgraph's tree and emits an [Edge] for each producer→consumer slot link.
//
// Package-internal; called by [NewSubgraph]'s populate body during construction. For every direct
// [*Node] child encountered, inspects every slot and emits an [Edge] from the slot's producer (via [ProducerIDOf]) to
// the consumer node on the enclosing subgraph's [Subgraph.Edges] list. Recurses into nested subgraphs so the whole tree
// is covered in one call.
func (s *Subgraph) materializeEdges() {

	for _, child := range s.executableUnits {
		switch t := child.(type) {
		case *Node:
			for _, value := range t.Slots() {
				if pid := ProducerIDOf(value); pid != "" {
					s.edges = append(s.edges, Edge{From: pid, To: t.ID()})
				}
			}
		case *Subgraph:
			t.materializeEdges()
		}
	}
}

// mergeBubbled merges a single bubbled [Parameter] into the `seen` map.
//
// Same-name + same-type entries dedup silently. Same-name + different-type entries consult [typesAreInterconvertible]
// before declaring a violation: when a registered conversion bridges the two types (in either direction), slot-fill at
// dispatch time performs the projection — there is no real collision. Only genuinely irreconcilable type pairs surface
// as errors; the caller appends them to its violation list and continues the walk so every real collision in the graph
// surfaces in one [Subgraph.Parameters] call (plan-doc D3 plan-time error).
//
// Variable-type selection when types differ but are interconvertible: prefer the source-side type — the shape a CLI
// flag, env var, or config value naturally produces — over framework abstractions constructed downstream from those
// primitives. Concretely: prefer types that do NOT implement [Resource] (or do not point to a [Resource] implementer)
// over Resource-typed slots, so a path-like variable's reported type settles to `string` rather than `*Resource`. The
// resolver then stores the raw source-side value; slot fill at dispatch converts outward to whatever each slot expects
// via [Convert]'s cascade.
//
// Parameters:
//   - `seen`: the accumulating dedup map keyed by variable name.
//   - `bubbled`: the candidate entry to merge.
//
// Returns:
//   - `error`: non-nil when `bubbled.Name` is already in `seen` with a type that neither matches nor bridges via the
//     conversion cascade. The seen map is not mutated in the error case.
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

// populate is the shared construction body for [NewSubgraph].
//
// Attaches children, stamps the per-unit policy triplet (elevation / retry / error) and slots, materializes edges,
// and sorts.
//
// Parameters:
//   - `spec`: the subgraph's [*SubgraphSpec]; supplies children, slots, and the per-unit policy triplet.
func (s *Subgraph) populate(spec *SubgraphSpec) {

	s.Name = spec.Name

	if spec.ActionName != "" {
		s.setActionName(spec.ActionName)
	}

	for _, child := range spec.Children {
		s.addChild(child)
	}

	if spec.ElevationOffer != nil {
		s.setElevationOffer(spec.ElevationOffer)
	}

	if spec.RetryPolicy != nil {
		s.setRetryPolicy(spec.RetryPolicy)
	}

	if spec.ErrorAction != nil {
		s.setErrorAction(spec.ErrorAction)
	}

	for name, value := range spec.Slots {
		s.setSlot(name, value)
	}

	s.materializeEdges()
	s.sortAll()
}

// slotParameterType returns the declared type and default of the bound method's `name` parameter.
//
// Walks Subgraph.Action() → action.Method() → method.ParameterByName(name); returns (nil, nil) when any link is absent.
// Used by [Subgraph.bubbleOwnSlots] to source type info for VariableValue slot fills.
//
// Parameters:
//   - `name`: the slot name to look up on the bound method.
//
// Returns:
//   - `reflect.Type`: the declared type of the named parameter, or nil when no bound action / method / matching
//     parameter exists.
//   - `any`: the parameter's default value, or nil under the same conditions.
func (s *Subgraph) slotParameterType(name string) (reflect.Type, any) {

	action := s.Action()
	if action == nil {
		return nil, nil
	}

	method := action.Method()
	if method == nil {
		return nil, nil
	}

	param, present := method.ParameterByName(name)
	if !present {
		return nil, nil
	}

	return param.Type, param.Default
}

// sortAll sorts this subgraph's children topologically and recurses into every nested [*Subgraph].
//
// Package-internal; called by [NewSubgraph]'s populate body during construction. Each Subgraph in
// the tree ends up with its `children` slice in topological order per [Subgraph.Edges], so the executor iterates
// without re-sorting.
func (s *Subgraph) sortAll() {

	s.sortChildren()

	for _, child := range s.executableUnits {
		if sub, ok := child.(*Subgraph); ok {
			sub.sortAll()
		}
	}
}

// sortChildren replaces this subgraph's children slice with the topologically sorted result.
//
// Called at assembly time so the in-memory order — and the serialized form — reflect the execution order. The executor
// iterates children in this order without re-sorting.
func (s *Subgraph) sortChildren() {

	s.executableUnits = topologicallySorted(s.executableUnits, s.edges)
}

// validateEdges checks that every entry in this subgraph's [Subgraph.Edges] references direct children by their IDs.
//
// Sibling-level edges are local — they don't cross subgraph boundaries.
//
// Returns:
//   - `error`: the joined error envelope (one entry per dangling endpoint), or nil on success.
func (s *Subgraph) validateEdges() error {

	var errs []error

	for _, e := range s.edges {
		if s.ChildByID(e.From) == nil {
			errs = append(errs, fmt.Errorf("subgraph %q: edge.From %q not a direct child", s.ID(), e.From))
		}
		if s.ChildByID(e.To) == nil {
			errs = append(errs, fmt.Errorf("subgraph %q: edge.To %q not a direct child", s.ID(), e.To))
		}
	}

	return errors.Join(errs...)
}

// endregion

// endregion

// region UNEXPORTED FUNCTIONS

// region Behaviors

// assembleSubgraph constructs a [*Subgraph] from a [subgraphData] payload during deserialization.
//
// Resolves the subgraph's action through the environment's registry. The load path builds the Subgraph empty and lets
// [Subgraph.linkChildren] resolve children later via the unit symbol table: the sealed [NewSubgraph] all-args
// constructor would materialize edges from children at construction, which can't happen here — children aren't
// instantiated yet. So the constructor is invoked with empty children and the wire-supplied name, annotations, edges, and
// placeholder child IDs are patched in through package-internal mutators. Called by [assembleGraph] once per subgraph in
// the decoded payload's flat subgraph list.
//
// Parameters:
//   - `env`: the runtime environment whose registry resolves action names.
//   - `p`: the decoded subgraph payload.
//
// Returns:
//   - `*Subgraph`: the constructed subgraph, with action bound and placeholder children.
//   - `error`: non-nil if the action name cannot be resolved.
func assembleSubgraph(env *RuntimeEnvironment, p *subgraphData) (*Subgraph, error) {

	action, err := resolvePayloadAction(env, p.ActionName, "subgraph", p.ID)
	if err != nil {
		return nil, err
	}

	sg, err := NewSubgraph(NewSubgraphSpec().
		WithID(p.ID).
		WithAction(action).
		WithRetryPolicy(p.Retry))
	if err != nil {
		return nil, fmt.Errorf("op.LoadGraph: subgraph %q: %w", p.ID, err)
	}
	sg.Name = p.Name
	sg.annotations = p.Annotations
	sg.setEdges(p.Edges)

	if len(p.Children) > 0 {
		sg.executableUnitsByID = make(map[string]ExecutableUnit, len(p.Children))
		for _, id := range p.Children {
			sg.executableUnitsByID[id] = nil
		}
	}
	return sg, nil
}

// endregion

// endregion

// region SUPPORTING TYPES

// SubgraphSpec is the fluent builder for a [*Subgraph].
//
// It embeds [ExecutableUnitSpec] and adds a child list, re-declaring each inherited With* to return `*SubgraphSpec` so
// the chain — including its own `WithChildren` — stays on the concrete type. Hand a populated spec to [NewSubgraph].
type SubgraphSpec struct {
	ExecutableUnitSpec
	Children []ExecutableUnit
	Name     string
}

// NewSubgraphSpec returns an empty [*SubgraphSpec] ready for fluent population via its With* setters.
//
// Returns:
//   - `*SubgraphSpec`: a zero-valued subgraph spec.
func NewSubgraphSpec() *SubgraphSpec {
	return &SubgraphSpec{}
}

// WithAction sets the dispatch [Action] and returns the spec for chaining.
//
// Callers that hold only a name bind via [SubgraphSpec.WithActionNamed] instead; every subgraph must end up bound one
// way or the other (there is no structural, action-less subgraph).
//
// Parameters:
//   - `action`: the [Action] to bind.
//
// Returns:
//   - `*SubgraphSpec`: the receiver, for chaining.
func (s *SubgraphSpec) WithAction(action Action) *SubgraphSpec {
	s.ExecutableUnitSpec.WithAction(action)
	return s
}

// WithActionNamed binds the dispatch action by its registry name and returns the spec for chaining.
//
// Validates the name against the global receiver registry and panics on an un-resolvable name (see
// [ExecutableUnitSpec.WithActionNamed]).
//
// Parameters:
//   - `name`: the dotted registry name (e.g. "flow.subgraph").
//
// Returns:
//   - `*SubgraphSpec`: the receiver, for chaining.
func (s *SubgraphSpec) WithActionNamed(name string) *SubgraphSpec {
	s.ExecutableUnitSpec.WithActionNamed(name)
	return s
}

// WithAnnotations sets the tool-specific annotations and returns the spec for chaining.
//
// Parameters:
//   - `annotations`: the raw `map[string]any` to stamp; nil for none.
//
// Returns:
//   - `*SubgraphSpec`: the receiver, for chaining.
func (s *SubgraphSpec) WithAnnotations(annotations map[string]any) *SubgraphSpec {
	s.ExecutableUnitSpec.WithAnnotations(annotations)
	return s
}

// WithChildren sets the subgraph's child units and returns the spec for chaining.
//
// Parameters:
//   - `children`: the [ExecutableUnit] children, in planned order; replaces any prior set.
//
// Returns:
//   - `*SubgraphSpec`: the receiver, for chaining.
func (s *SubgraphSpec) WithChildren(children ...ExecutableUnit) *SubgraphSpec {
	s.Children = children
	return s
}

// WithElevationOffer sets the [ElevationOffer] and returns the spec for chaining.
//
// Parameters:
//   - `elevationOffer`: the [ElevationOffer], or nil to run unprivileged.
//
// Returns:
//   - `*SubgraphSpec`: the receiver, for chaining.
func (s *SubgraphSpec) WithElevationOffer(elevationOffer *ElevationOffer) *SubgraphSpec {
	s.ExecutableUnitSpec.WithElevationOffer(elevationOffer)
	return s
}

// WithErrorAction sets the failure-handler [Subgraph] and returns the spec for chaining.
//
// Parameters:
//   - `errorAction`: the handler [Subgraph], or nil for no error action.
//
// Returns:
//   - `*SubgraphSpec`: the receiver, for chaining.
func (s *SubgraphSpec) WithErrorAction(errorAction *Subgraph) *SubgraphSpec {
	s.ExecutableUnitSpec.WithErrorAction(errorAction)
	return s
}

// WithID sets the unit identifier and returns the spec for chaining.
//
// Parameters:
//   - `id`: the unit identifier.
//
// Returns:
//   - `*SubgraphSpec`: the receiver, for chaining.
func (s *SubgraphSpec) WithID(id string) *SubgraphSpec {
	s.ExecutableUnitSpec.WithID(id)
	return s
}

// WithName sets the subgraph's display name and returns the spec for chaining.
//
// Parameters:
//   - `name`: the subgraph name (e.g. a lore phase name like "install").
//
// Returns:
//   - `*SubgraphSpec`: the receiver, for chaining.
func (s *SubgraphSpec) WithName(name string) *SubgraphSpec {
	s.Name = name
	return s
}

// WithRetryPolicy sets the [RetryPolicy] and returns the spec for chaining.
//
// Parameters:
//   - `retryPolicy`: the [RetryPolicy], or nil for no retry.
//
// Returns:
//   - `*SubgraphSpec`: the receiver, for chaining.
func (s *SubgraphSpec) WithRetryPolicy(retryPolicy *RetryPolicy) *SubgraphSpec {
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
//   - `*SubgraphSpec`: the receiver, for chaining.
func (s *SubgraphSpec) WithSlot(name string, value SlotValue) *SubgraphSpec {
	s.ExecutableUnitSpec.WithSlot(name, value)
	return s
}

// subgraphData is the canonical serialized shape for Subgraph.
//
// `Children` holds direct-child IDs in topological order; the actual units are looked up in the surrounding Graph's
// unit table via [Subgraph.linkChildren] during unmarshal. Used by both JSON and YAML marshalers.
type subgraphData struct {
	ID          string        `json:"id"                    yaml:"id"`
	Name        string        `json:"name"                  yaml:"name"`
	ActionName  string        `json:"action_name,omitempty" yaml:"action_name,omitempty"`
	Annotations AnnotationMap `json:"annotations,omitempty"  yaml:"annotations,omitempty"`
	Children    []string      `json:"children"              yaml:"children"`
	Edges       []Edge        `json:"edges,omitempty"       yaml:"edges,omitempty"`
	Retry       *RetryPolicy  `json:"retry,omitempty"       yaml:"retry,omitempty"`
}

// endregion

// region HELPERS

// preferSourceSide reports whether `candidate` is more source-side than `incumbent`.
//
// "Source-side" means closer to the shape a CLI flag / env var / config value naturally produces. Used by
// [Subgraph.mergeBubbled] to pick a stable, source-friendly type for a variable bound to
// differently-typed-but-interconvertible slots.
//
// Rule: a type that does NOT implement [Resource] (and is not a pointer to one) is preferred over a type that does.
// This codifies "primitives are source-side, Resources are framework-side": CLI strings, env strings, config values
// flow as their natural Go shapes; Resources are constructed downstream via the catalog. Among two types of equal
// Resource-ness, the incumbent wins (no change).
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

// typeImplementsResource reports whether `t` is a [Resource]-implementing type.
//
// Accounts for the conventional pattern of declaring Resource methods on the pointer receiver (so `*file.Resource`
// implements Resource while `file.Resource` does not). Returns true for both `Resource` and `*Resource`-shaped types.
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

// endregion
