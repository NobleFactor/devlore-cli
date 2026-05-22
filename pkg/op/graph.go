// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package op owns the concrete graph data model shared by the execution engine, Starlark layer, and CLI tools.
//
// # Core Types
//
//   - Graph: A directed graph of nodes and edges representing work to be done
//   - Node: A single unit of work with an action to execute
//   - Edge: A dependency relationship between nodes
//
// # Graph Lifecycle
//
// The Graph represents both plans (before execution) and receipts (after execution):
//   - Before Run(): State is "pending", nodes describe what will happen
//   - After Run(): State is "executed", nodes describe what happened
//   - Serialized before execution: "dry-run" or "purchase order"
//   - Serialized after execution: "receipt"
package op

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

// GraphFormatVersion is the current graph serialization format version.
const GraphFormatVersion = "6"

// Graph represents an execution graph containing nodes and edges.
// This is THE graph used by both writ and lore - they differ only in content.
//
// Before Run(): State is "pending", represents the plan
// After Run(): State is "executed", represents the receipt
type Graph struct {

	// Catalog is the append-only resource catalog for planning.
	//
	// One per Graph. Not serialized — planning-only state.
	Catalog *ResourceCatalog

	// Checksum is the git-style integrity hash.
	Checksum string

	// Collisions records source conflicts resolved during tree building (writ-specific).
	Collisions []Collision

	// Provenance records what was planned, from what sources, and for what scope.
	Provenance Provenance

	// Rollback records compensating actions executed during rollback (populated on failure).
	Rollback []RollbackEntry

	// Root is the graph's root subgraph. All top-level children and edges live here.
	//
	// [Graph.Edges] is the read accessor that returns Root's edges. Execution starts from [Graph.Execute](g.Root, nil).
	Root *Subgraph

	// Signature contains the cryptographic signature (optional).
	Signature *sops.Signature

	// State is the execution state (pending, executed, failed).
	State GraphState

	// Timestamp is when the graph was created/executed.
	Timestamp time.Time

	// Version is the graph format version.
	Version string

	// ctx is the execution environment for this graph. Replaced by Rebind at the planning-to-execution handoff.
	ctx *RuntimeEnvironment

	// Summary contains execution statistics (populated after Summary()).
	summary GraphExecutionSummary

	// unitsByID is the unit symbol table populated by [Graph.UnmarshalJSON] / [Graph.UnmarshalYAML] (and, in
	// Phase 5, by `plan.assemble`).
	//
	// Maps unit ID to the materialized [*Node] or [*Subgraph]. Empty for Graphs built directly via the planning API
	// ([Graph.AddNode] / [Graph.AddSubgraph]) — those callers walk the children tree directly. Used by
	// [Subgraph.linkChildren] at unmarshal to resolve child IDs.
	unitsByID map[string]ExecutableUnit
}

// NewGraph creates a Graph with no bound runtime environment.
//
// The graph is plan-structure data at construction — nodes, edges, slot values, Catalog, provenance, checksum.
//
// A [RuntimeEnvironment] is bound only when a session-owner (a planner via [Plan] or the [GraphExecutor] via
// [GraphExecutor.Run]) calls [Graph.Rebind] for the duration of that session, and [Graph.Unbind] when the session ends.
//
// Returns:
//   - `*Graph`: the freshly constructed, env-less graph.
func NewGraph() *Graph {

	return &Graph{
		Root:      NewSubgraph("root"),
		Catalog:   NewResourceCatalog(),
		Version:   GraphFormatVersion,
		Timestamp: time.Now(),
		State:     StatePending,
	}
}

// Parameters are the bubble-up variable surface of the graph.
//
// It is the deduplicated, type-checked set of [VariableValue] references walked across the root subgraph's children
// (plan-doc D3). It is consumed by the executor's preflight pass to drive [VariableResolver.Resolve].
//
// Returns:
//   - `[]Parameter`: the bubble-up surface, stable-sorted by Name.
func (g *Graph) Parameters() []Parameter { return g.Root.Parameters() }

// Edges returns the ordering edges at the root level.
func (g *Graph) Edges() []Edge { return g.Root.edges }

// region EXPORTED METHODS

// region State management

// AddNode appends a node as a root-level child of the graph.
//
// Routing through [Subgraph.AddChild] stamps the node's parent pointer to the graph's Root (plan-doc D11).
//
// Parameters:
//   - `node`: the node to append.
func (g *Graph) AddNode(node *Node) {
	g.Root.AddChild(node)
}

// AddSubgraph appends a subgraph as a root-level child of the graph.
//
// Routing through [Subgraph.AddChild] stamps the subgraph's parent pointer to the graph's Root (plan-doc D11).
//
// Parameters:
//   - `sg`: the subgraph to append.
func (g *Graph) AddSubgraph(sg *Subgraph) {
	g.Root.AddChild(sg)
}

// Nodes returns all nodes in the graph by walking the tree recursively.
//
// The returned slice is in tree-walk order (depth-first, declaration order).
//
// Returns:
//   - []*Node: the flat node list in tree-walk order; nil when no nodes are present.
func (g *Graph) Nodes() []*Node { return g.Root.descendantNodes() }

// Subgraphs returns every [*Subgraph] descendant of [Graph.Root].
//
// The result does NOT include the Root subgraph itself — it lists only authored / planner-emitted
// container units below it. Used by [Graph.UnitCount] and by harness assertions that want to count
// or inspect every executable unit produced by `plan.assemble`.
//
// Returns:
//   - []*Subgraph: the descendant subgraphs in tree-walk order.
func (g *Graph) Subgraphs() []*Subgraph { return g.Root.descendantSubgraphs() }

// UnitCount returns the total count of [ExecutableUnit] descendants of [Graph.Root] — both [*Node]
// and [*Subgraph] children. Excludes the Root itself.
//
// This is the count the harness asserts against via `t.expect_unit_count(n)`: a `plan.choose`
// container materializes as a Subgraph that holds its branch's children, so a script with
// `write_text` + `exists` + `choose(then=remove)` produces unit count 4 (3 Nodes + 1 Subgraph),
// not 3.
//
// Returns:
//   - int: the total descendant-unit count.
func (g *Graph) UnitCount() int { return len(g.Nodes()) + len(g.Subgraphs()) }

// RuntimeEnvironment returns a point to the graph's execution context.
func (g *Graph) RuntimeEnvironment() *RuntimeEnvironment { return g.ctx }

// Filename returns the standard filename for this graph.
//
// Format: "<timestamp>.yaml" or "<scope>-<timestamp>.yaml" when scoped.
func (g *Graph) Filename() string {

	ts := g.Timestamp.Format("2006-01-02T15-04-05")

	if g.Provenance.Scope != "" {
		return fmt.Sprintf("%s-%s.yaml", g.Provenance.Scope, ts)
	}

	return fmt.Sprintf("%s.yaml", ts)
}

// SubgraphByID returns the descendant subgraph with the given ID, or nil if no descendant has that ID.
//
// Searches the tree recursively; ID never returns the graph root.
//
// Parameters:
//   - `id`: the Subgraph ID to find.
//
// Returns:
//   - *Subgraph: the matching descendant, or nil.
func (g *Graph) SubgraphByID(id string) *Subgraph { return g.Root.descendantSubgraphByID(id) }

// ResolveExecutable returns the executable unit with the given ID, or an error if no such unit exists.
//
// Nodes and subgraphs share one ID space (Phase 7 invariant); ResolveExecutable is the single lookup gather, choose,
// and other combinators use to resolve a body reference.
func (g *Graph) ResolveExecutable(id string) (ExecutableUnit, error) {
	if g.Root != nil && g.Root.ID() == id {
		return g.Root, nil
	}
	if sub := g.SubgraphByID(id); sub != nil {
		return sub, nil
	}
	for _, node := range g.Nodes() {
		if node.ID() == id {
			return node, nil
		}
	}
	return nil, fmt.Errorf("no executable unit with ID %q", id)
}

// Rebind binds the graph to a runtime environment for the duration of a session.
//
// Both planning ([Plan]) and execution ([GraphExecutor.Run]) call Rebind on entry and [Graph.Unbind] on exit, so the
// graph's `ctx` field is authoritative only inside the active session. Between sessions (after Unbind, before the next
// Rebind) the field is nil.
//
// The two-session split is structural — a graph planned in one environment may be executed in another
// (different machine, different time). Each session-owner installs its own env via Rebind for the duration
// of the work it controls.
//
// Parameters:
//   - `runtimeEnvironment`: the env to bind for the duration of the active session.
func (g *Graph) Rebind(runtimeEnvironment *RuntimeEnvironment) {

	g.ctx = runtimeEnvironment
	g.linkActions(runtimeEnvironment)
}

// linkActions walks every executable unit in this graph and rebinds its [Action] from the
// transient `pendingAction` stash populated by the marshalers. This is the post-load Action-rebind
// link pass: when a graph round-trips through wire form, units carry their action name as a string;
// linkActions resolves each name through `env.Registry.ActionByName(name)` and stamps the live
// [Action] via [executableUnit.SetAction]. Units whose action name cannot be resolved leave their
// Action nil — the executor reports the missing binding when it reaches that unit.
//
// Parameters:
//   - `env`: the runtime environment whose registry resolves action names.
func (g *Graph) linkActions(env *RuntimeEnvironment) {

	rebind := func(unit ExecutableUnit) {
		bu, ok := unit.(interface {
			PendingAction() string
			SetPendingAction(string)
			SetAction(Action)
		})
		if !ok {
			return
		}
		name := bu.PendingAction()
		if name == "" {
			return
		}
		if action, err := env.ActionByName(name); err == nil {
			bu.SetAction(action)
		}
		bu.SetPendingAction("")
	}

	if g.Root != nil {
		rebind(g.Root)
	}
	for _, n := range g.Nodes() {
		rebind(n)
	}
	for _, sg := range g.Subgraphs() {
		rebind(sg)
	}
}

// Unbind clears the graph's bound runtime environment.
//
// Called by session-owners ([Plan], [GraphExecutor.Run]) when their session ends, so the graph carries no stale env
// reference across the handoff to a later session.
func (g *Graph) Unbind() {
	g.ctx = nil
}

// Execute runs an executable unit (Node or Subgraph) with caller-supplied slot overrides.
//
// Execute is the unified entry point for every top-level execution path: top-level graph runs, subgraph invocations,
// gather iterations, choose branches, and test harnesses all funnel through here. Resolution order per slot is
// overrides[param.Name] when present, then the unit's baked-in Slot.Value (ImmediateValue, PromiseValue, or
// Variable) resolved via its Resolve(variables, results) method.
//
// Overrides route to topological root children only — non-root children receive inputs via promises, not from outside
// the subgraph. Overrides are runtime-only; they do not serialize into the graph.
//
// Execute creates a fresh recovery stack scoped to this call; compensation on failure unwinds only actions executed
// during this invocation. Nested combinators that need to own their own stack call [Graph.ExecuteWithStack] instead.
//
// Parameters:
//   - `exec`: the executable unit to run.
//   - `overrides`: caller-supplied slot overrides; nil for none.
//
// Returns:
//   - `any`: the unit's terminal result, or nil when it produces no output.
//   - `error`: non-nil if the unit fails or the graph is not yet rebound.
func (g *Graph) Execute(exec ExecutableUnit, overrides map[string]SlotValue) (any, error) {

	if exec == nil {
		return nil, fmt.Errorf("execute: exec is nil")
	}

	if g.ctx == nil {
		return nil, fmt.Errorf("execute: graph has no execution context; call Rebind first")
	}

	if g.ctx.Results == nil {
		g.ctx.Results = make(map[string]any)
	}

	e := &GraphExecutor{hooks: NewHookRegistry()}
	stack := NewRecoveryStack()

	result, err := g.dispatch(g.ctx.Context, e, stack, exec, g.ctx.Results, overrides)

	if err != nil {
		if unwindErr := stack.Unwind(); unwindErr != nil {
			err = fmt.Errorf("%w; compensation: %w", err, unwindErr)
		}
	}

	return result, err
}

// ExecuteWithStack runs an ExecutableUnit against a caller-supplied recovery stack and cancellation context.
//
// The caller owns the stack end-to-end: ExecuteWithStack neither initializes it nor unwinds it on error, so the
// caller can inspect its contents after the call and decide whether to compose them into a parent-scope complement
// or unwind them locally. Each call builds a fresh GraphExecutor and a fresh results map, so observability hooks
// on the parent execution do not fire on the unit's body (a known limitation addressed in a later step) and the
// results map reflects the D6 per-invocation scope rule.
//
// Intended for combinators that manage their own compensation and cancellation scope — currently
// [flow.Provider.Gather], whose concurrent iterations each build a stack that becomes part of gather's compensable
// complement on total success or is unwound locally on failure. The ctx argument carries the combinator's scoped
// cancellation; [Graph.dispatch] threads it down to executeNode's entry check so cancelled iterations bail
// cooperatively.
//
// Parameters:
//   - `ctx`: the combinator's scoped cancellation context; propagates to iteration bodies via dispatch.
//   - `exec`: the executable unit to run for this iteration.
//   - `stack`: the caller-owned recovery stack that collects the iteration's compensations.
//   - `overrides`: caller-supplied slot overrides for the iteration, typically the bound iteration input.
//
// Returns:
//   - `any`: the unit's terminal result, or nil when it produces no output.
//   - `error`: non-nil if the unit fails or the graph is not yet rebound; the stack is returned to the caller intact.
func (g *Graph) ExecuteWithStack(ctx context.Context, exec ExecutableUnit, stack *RecoveryStack, overrides map[string]SlotValue) (any, error) {

	if exec == nil {
		return nil, fmt.Errorf("ExecuteWithStack: exec is nil")
	}

	if g.ctx == nil {
		return nil, fmt.Errorf("ExecuteWithStack: graph has no execution context; call Rebind first")
	}

	if stack == nil {
		return nil, fmt.Errorf("ExecuteWithStack: stack is nil")
	}

	e := &GraphExecutor{hooks: NewHookRegistry()}
	results := make(map[string]any)

	return g.dispatch(ctx, e, stack, exec, results, overrides)
}

// dispatch is the single Node/Subgraph dispatch site that every unit invocation flows through.
//
// Callers supply the live [GraphExecutor], [RecoveryStack], results map, and [context.Context], so the same executor
// state and cancellation scope thread from the top-level entry down through recursive child dispatch (Phase 5 wires
// that recursion via the rewritten [GraphExecutor.executeSubgraph]). This is the hook site: observability hooks and
// cancellation checks attached here (the ctx.Err() check happens in executeNode) see every unit dispatch regardless of
// nesting depth.
//
// [Graph.Execute] and [GraphExecutor.Run] are the two external bootstraps that seed a fresh executor and stack before
// calling dispatch; [Graph.ExecuteWithStack] is the combinator entry (e.g., gather) that brings its own stack and
// scoped ctx.
//
// Parameters:
//   - `ctx`: the cancellation context threaded from the nearest entry point.
//   - `e`: the live executor whose hooks and state persist across the dispatch chain.
//   - `stack`: the active recovery stack compensations are pushed onto.
//   - `exec`: the executable unit to dispatch; must be a *Node or *Subgraph.
//   - `results`: the accumulated node results for promise resolution.
//   - `overrides`: caller-supplied slot overrides, if any.
//
// Returns:
//   - `any`: the unit's terminal result, or nil when it produces no output.
//   - `error`: non-nil if the unit fails or the exec type is unrecognized.
func (g *Graph) dispatch(ctx context.Context, e *GraphExecutor, stack *RecoveryStack, exec ExecutableUnit, results map[string]any, overrides map[string]SlotValue) (any, error) {

	switch unit := exec.(type) {

	case *Node:
		result := e.executeNode(ctx, g, unit, results, stack, overrides)
		if result.Status == ResultFailed {
			return nil, result.Error
		}
		return results[unit.ID()], nil

	case *Subgraph:
		return e.executeSubgraph(ctx, g, unit, results, stack, overrides)

	default:
		return nil, fmt.Errorf("dispatch: unknown ExecutableUnit type %T", exec)
	}
}

// endregion

// region Behaviors

// CanonicalContent returns the graph serialized as YAML without checksum and signature.
//
// Used for computing checksums and verifying signatures. The output mirrors the symbol-table wire form: top-level
// `children` (root's children IDs in topological order), `subgraphs` (every non-root Subgraph sorted by ID), and
// `nodes` (every Node sorted by ID).
//
// Returns:
//   - `[]byte`: the canonical YAML bytes.
//   - `error`: non-nil if YAML marshaling fails.
func (g *Graph) CanonicalContent() ([]byte, error) {

	type canonicalGraph struct {
		Version    string      `yaml:"version"`
		State      GraphState  `yaml:"state"`
		Timestamp  string      `yaml:"timestamp"`
		Children   []string    `yaml:"children"`
		Edges      []Edge      `yaml:"edges,omitempty"`
		Subgraphs  []*Subgraph `yaml:"subgraphs,omitempty"`
		Nodes      []*Node     `yaml:"nodes,omitempty"`
		Collisions []Collision `yaml:"collisions,omitempty"`
		Context    Provenance  `yaml:"context"`
	}

	var rootEdges []Edge

	if g.Root != nil {
		rootEdges = g.Root.edges
	}

	subgraphs := g.Root.descendantSubgraphs()
	sort.Slice(subgraphs, func(i, j int) bool { return subgraphs[i].ID() < subgraphs[j].ID() })

	nodes := g.Root.descendantNodes()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID() < nodes[j].ID() })

	canonical := canonicalGraph{
		Version:    g.Version,
		State:      g.State,
		Timestamp:  g.Timestamp.Format(time.RFC3339),
		Children:   g.Root.childIDs(),
		Edges:      rootEdges,
		Subgraphs:  subgraphs,
		Nodes:      nodes,
		Collisions: g.Collisions,
		Context:    g.Provenance,
	}

	return yaml.Marshal(canonical)
}

// Summary calculates execution statistics from nodes.
//
// Returns:
//   - `GraphExecutionSummary`: the computed summary.
func (g *Graph) Summary() GraphExecutionSummary {

	g.summary = newGraphExecutionSummary(g.Nodes())
	return g.summary
}

// Serialize writes this graph through `enc`, dispatching to [Graph.MarshalJSON] or [Graph.MarshalYAML]
// per the encoder's concrete type. The result is the symbol-table wire form: top-level `children` IDs
// from Root, plus the flat `subgraphs` and `nodes` lists sorted by ID.
//
// Whatever value is currently in [Graph.Checksum] is emitted as-is; this method does not (re)compute it.
// Callers that want a fresh checksum compute it from [Graph.CanonicalContent] and assign before calling.
//
// Usage:
//
//	enc := yaml.NewEncoder(file)
//	enc.SetIndent(2)
//	defer enc.Close()
//	g.Serialize(enc)
//
// Parameters:
//   - `enc`: the destination encoder; both `*json.Encoder` and `*yaml.Encoder` satisfy [Encoder].
//
// Returns:
//   - `error`: the encoder's error, or nil on success.
func (g *Graph) Serialize(enc Encoder) error {

	return enc.Encode(g)
}

// endregion

// endregion

// Collision records a source conflict resolved during tree building (writ-specific).
type Collision struct {
	Loser             string `json:"loser" yaml:"loser"`
	LoserLayer        string `json:"loser_layer,omitempty" yaml:"loser_layer,omitempty"`
	LoserSpecificity  int    `json:"loser_specificity,omitempty" yaml:"loser_specificity,omitempty"`
	Target            string `json:"target" yaml:"target"`
	Winner            string `json:"winner" yaml:"winner"`
	WinnerLayer       string `json:"winner_layer,omitempty" yaml:"winner_layer,omitempty"`
	WinnerSpecificity int    `json:"winner_specificity,omitempty" yaml:"winner_specificity,omitempty"`
}

// Edge represents a dependency relationship between two nodes.
// From must complete before To can begin execution.
type Edge struct {
	From string `json:"from" yaml:"from"`
	To   string `json:"to" yaml:"to"`
}

// Encoder is the interface for graph serialization.
// Both *json.Encoder and *yaml.Encoder satisfy this interface.
type Encoder interface {
	Encode(v any) error
}

// GraphState represents the execution state of the graph.
type GraphState string

// GraphState constants define the possible states of a graph.
const (
	// StateExecuted indicates the graph executed successfully.
	StateExecuted GraphState = "executed"
	// StateFailed indicates the graph failed during execution.
	StateFailed GraphState = "failed"
	// StatePending indicates the graph has not yet been executed.
	StatePending GraphState = "pending"
)

// Node represents a single unit of work in an execution graph.
type Node struct {
	executableUnit

	// Layer is the repository layer (base, team, personal).
	Layer string

	// Origin this node belongs to.
	Origin string
}

// NewNode constructs a Node bound to the supplied [Action].
//
// Every Node must dispatch to a method, so construction requires a non-nil action — passing nil is a
// program-construction error and panics via the assert package. Wire-form deserialization does NOT go
// through this constructor; the JSON / YAML decoder produces a zero-value Node and [Node.applyPayload]
// fills it from the payload, with the eventual [Graph.Rebind] resolving the cached action name through
// the registry.
//
// Parameters:
//   - `id`: the node identifier; immutable after this call.
//   - `action`: the dispatch action; must be non-nil.
//
// Returns:
//   - *Node: the constructed node, with `id` and `action` set.
func NewNode(id string, action Action) *Node {

	assert.Truef(action != nil, "op.NewNode(%q): action must be non-nil", id)
	return &Node{executableUnit: executableUnit{id: id, action: action}}
}

// region EXPORTED METHODS

// region Behaviors

// Parameters returns this node's variable bubble-up surface — one [Parameter] per slot whose value is a
// [VariableValue]. Each returned entry carries the value-side variable name (the variable a caller of this
// node's containing [Subgraph] must supply) and the type / default sourced from the bound action's method
// signature via [Method.ParameterByName] on the slot name.
//
// This shadows the embedded [executableUnit.Parameters] so that [Subgraph.Parameters] composes its bubble-up
// surface uniformly via [ExecutableUnit.Parameters] across both Node and Subgraph children, without a
// per-child type switch. Callers that want the method's declared parameter list (the slot names / types
// the method expects to receive) read [Action.Method].Parameters() directly.
//
// Returns:
//   - []Parameter: the variable bubble-up surface; nil when no slot carries a [VariableValue].
func (n *Node) Parameters() []Parameter {

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

	return out
}

// ResolvedSlots returns all slot values as a flat map, resolving promises and variable bindings.
func (n *Node) ResolvedSlots(variables map[string]Variable, results map[string]any) map[string]any {
	return n.ResolveSlots(variables, results, nil)
}

// ResolveSlots returns all slot values with caller-supplied overrides applied.
//
// For each slot, if overrides contains an entry keyed by the parameter name, that entry's Resolve is used; otherwise
// the baked-in [SlotValue] is resolved. Overrides whose keys do not match any slot are silently ignored — the slot map
// is the authority.
func (n *Node) ResolveSlots(variables map[string]Variable, results map[string]any, overrides map[string]SlotValue) map[string]any {

	out := make(map[string]any, len(n.slots))
	for name, value := range n.slots {
		if ov, ok := overrides[name]; ok {
			out[name] = ov.Resolve(variables, results)
			continue
		}
		out[name] = value.Resolve(variables, results)
	}
	return out
}

// endregion

// endregion

// Status is the unified execution status for [ExecutableUnit]s. Both Nodes and Subgraphs use the same
// status set — a rollback at a Subgraph boundary cascades to every Node inside it, so there's no
// kind-specific status that's not shared.
type Status string

// Status constants define the possible statuses of an executable unit.
const (
	// StatusCompleted indicates the unit executed successfully.
	StatusCompleted Status = "completed"
	// StatusFailed indicates the unit failed during execution.
	StatusFailed Status = "failed"
	// StatusPending indicates the unit has not yet been executed.
	StatusPending Status = "pending"
	// StatusRolledBack indicates the unit was rolled back after failure.
	StatusRolledBack Status = "rolled_back"
	// StatusSkipped indicates the unit was skipped.
	StatusSkipped Status = "skipped"
)

// Provenance contains tool-specific metadata stored in the graph.
// Both writ and lore populate this with their relevant context.
type Provenance struct {

	// CommitHashes records the git commit hash for each layer source (writ-specific).
	// Keys are layer names ("base", "team", "personal"); values are full commit hashes.
	CommitHashes map[string]string `json:"commit_hashes,omitempty" yaml:"commit_hashes,omitempty"`

	// DirtyLayers lists layer names that had uncommitted changes at planning time (writ-specific).
	// Present only when --allow-dirty was used; empty means all layers were clean.
	DirtyLayers []string `json:"dirty_layers,omitempty" yaml:"dirty_layers,omitempty"`

	// Features enabled for package installation (lore-specific).
	Features []string `json:"features,omitempty" yaml:"features,omitempty"`

	// Layers lists repository layers used (writ-specific).
	Layers []string `json:"layers,omitempty" yaml:"layers,omitempty"`

	// Packages lists the packages included (lore-specific).
	Packages []string `json:"packages,omitempty" yaml:"packages,omitempty"`

	// Projects lists the projects included (writ-specific).
	Projects []string `json:"projects,omitempty" yaml:"projects,omitempty"`

	// Scope identifies the planning scope for this graph.
	// For writ: target scope ("system", "home").
	// For lore: package cache scope (package name or names).
	Scope string `json:"scope,omitempty" yaml:"scope,omitempty"`

	// Segments contains platform segment values (writ-specific).
	Segments map[string]string `json:"segments,omitempty" yaml:"segments,omitempty"`

	// Settings for package installation (lore-specific).
	Settings map[string]string `json:"settings,omitempty" yaml:"settings,omitempty"`

	// SourceRoot is the source directory (writ: repo path, lore: registry cache).
	SourceRoot string `json:"source_root,omitempty" yaml:"source_root,omitempty"`

	// TargetPlatform is the target platform string (lore-specific, e.g., "Darwin", "Linux.Debian").
	TargetPlatform string `json:"target_platform,omitempty" yaml:"target_platform,omitempty"`

	// Tool identifies which program produced this graph ("writ", "lore").
	Tool string `json:"tool,omitempty" yaml:"tool,omitempty"`

	// TargetRoot is the target directory (typically $HOME).
	TargetRoot string `json:"target_root,omitempty" yaml:"target_root,omitempty"`
}

// ActionExecutionSummary provides execution statistics for a set of actions.
type ActionExecutionSummary interface {
	json.Marshaler
	yaml.Marshaler
	Completed() int
	Failed() int
	Skipped() int
	Total() int
}

// GraphExecutionSummary extends [ActionExecutionSummary] with per-action breakdowns.
type GraphExecutionSummary interface {
	ActionExecutionSummary
	ByAction() map[string]ActionExecutionSummary
}

// actionExecutionSummary is the concrete implementation of [ActionExecutionSummary].
type actionExecutionSummary struct {
	completed int
	failed    int
	skipped   int
	total     int
}

func (s *actionExecutionSummary) Completed() int { return s.completed }
func (s *actionExecutionSummary) Failed() int    { return s.failed }
func (s *actionExecutionSummary) Skipped() int   { return s.skipped }
func (s *actionExecutionSummary) Total() int     { return s.total }

// actionSummaryPayload is the serialization shape for [actionExecutionSummary].
type actionSummaryPayload struct {
	Completed int `json:"completed" yaml:"completed"`
	Failed    int `json:"failed,omitempty" yaml:"failed,omitempty"`
	Skipped   int `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	Total     int `json:"total" yaml:"total"`
}

func (s *actionExecutionSummary) MarshalJSON() ([]byte, error) {

	return json.Marshal(actionSummaryPayload{
		Completed: s.completed,
		Failed:    s.failed,
		Skipped:   s.skipped,
		Total:     s.total,
	})
}

func (s *actionExecutionSummary) MarshalYAML() (any, error) {

	return actionSummaryPayload{
		Completed: s.completed,
		Failed:    s.failed,
		Skipped:   s.skipped,
		Total:     s.total,
	}, nil
}

// graphExecutionSummary is the concrete implementation of [GraphExecutionSummary].
type graphExecutionSummary struct {
	actionExecutionSummary
	byAction map[string]*actionExecutionSummary
}

// newGraphExecutionSummary creates a [GraphExecutionSummary] from a slice of nodes.
//
// Post-step-15, per-node Status is no longer derivable from the Node itself — it lives on the
// recovery-stack receipts at execution time. This summary therefore reports only totals (overall and
// per-action). The completed / failed / skipped counters remain zero until a future step rewires them
// off the receipt stack (tracked as a follow-on).
func newGraphExecutionSummary(nodes []*Node) GraphExecutionSummary {

	s := &graphExecutionSummary{
		byAction: make(map[string]*actionExecutionSummary),
	}
	s.total = len(nodes)

	for _, n := range nodes {

		var actionName string
		if a := n.Action(); a != nil {
			actionName = a.Name()
		}
		action, ok := s.byAction[actionName]
		if !ok {
			action = &actionExecutionSummary{}
			s.byAction[actionName] = action
		}
		action.total++
	}

	return s
}

// ByAction returns per-action summaries. The returned map is a copy.
func (s *graphExecutionSummary) ByAction() map[string]ActionExecutionSummary {

	out := make(map[string]ActionExecutionSummary, len(s.byAction))
	for k, v := range s.byAction {
		out[k] = v
	}
	return out
}

// graphSummaryPayload is the serialization shape for [graphExecutionSummary].
type graphSummaryPayload struct {
	Completed int                             `json:"completed" yaml:"completed"`
	Failed    int                             `json:"failed,omitempty" yaml:"failed,omitempty"`
	Skipped   int                             `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	Total     int                             `json:"total" yaml:"total"`
	ByAction  map[string]actionSummaryPayload `json:"by_action,omitempty" yaml:"by_action,omitempty"`
}

func (s *graphExecutionSummary) MarshalJSON() ([]byte, error) {

	p := graphSummaryPayload{
		Completed: s.completed,
		Failed:    s.failed,
		Skipped:   s.skipped,
		Total:     s.total,
		ByAction:  make(map[string]actionSummaryPayload, len(s.byAction)),
	}
	for k, v := range s.byAction {
		p.ByAction[k] = actionSummaryPayload{
			Completed: v.completed,
			Failed:    v.failed,
			Skipped:   v.skipped,
			Total:     v.total,
		}
	}
	return json.Marshal(p)
}

func (s *graphExecutionSummary) MarshalYAML() (any, error) {

	p := graphSummaryPayload{
		Completed: s.completed,
		Failed:    s.failed,
		Skipped:   s.skipped,
		Total:     s.total,
		ByAction:  make(map[string]actionSummaryPayload, len(s.byAction)),
	}
	for k, v := range s.byAction {
		p.ByAction[k] = actionSummaryPayload{
			Completed: v.completed,
			Failed:    v.failed,
			Skipped:   v.skipped,
			Total:     v.total,
		}
	}
	return p, nil
}

type NodeResult struct {
	NodeID  string
	Status  ResultStatus
	Error   error
	Message string
}

type ResultStatus int

const (
	ResultPending ResultStatus = iota
	ResultRunning
	ResultCompleted
	ResultFailed
	ResultSkipped
)

// region HELPER FUNCTIONS

// GitStyleChecksum computes a git-style checksum.
//
// Format: SHA256("<type> <basename> <len>\0<content>")
func GitStyleChecksum(objectType string, basename string, content []byte) string {

	header := fmt.Sprintf("%s %s %d\x00", objectType, basename, len(content))
	hash := sha256.New()
	hash.Write([]byte(header))
	hash.Write(content)

	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// endregion
