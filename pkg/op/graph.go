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
	"time"

	"gopkg.in/yaml.v3"

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

	// Root is the graph's root subgraph. All top-level children and edges live
	// here. Graph.Children() / Graph.Edges() are read accessors that return
	// Root's fields. Execution starts from Graph.Execute(g.Root, nil).
	Root *Subgraph

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

	// Signature contains the cryptographic signature (optional).
	Signature *sops.Signature

	// State is the execution state (pending, executed, failed).
	State GraphState

	// Summary contains execution statistics (populated after Summary()).
	summary GraphExecutionSummary

	// Timestamp is when the graph was created/executed.
	Timestamp time.Time

	// Version is the graph format version.
	Version string

	// ctx is the execution context for this graph. Replaced by Rebind at the planning-to-execution handoff.
	ctx *ExecutionContext
}

// NewGraph creates a Graph with the given execution context.
//
// Parameters:
//   - ctx: the execution context providing registry, root, platform, and other session state.
func NewGraph(ctx *ExecutionContext) *Graph {

	return &Graph{
		Root:      NewSubgraph("root"),
		Version:   GraphFormatVersion,
		Timestamp: time.Now(),
		State:     StatePending,
		ctx:       ctx,
	}
}

// Children returns the root-level children of the graph.
func (g *Graph) Children() []SubgraphChild { return g.Root.Children }

// Edges returns the ordering edges at the root level.
func (g *Graph) Edges() []Edge { return g.Root.Edges }

// region EXPORTED METHODS

// region State management

// AddNode appends a node as a root-level child of the graph and sets the node's back-reference.
//
// Parameters:
//   - node: the node to append.
func (g *Graph) AddNode(node *Node) {
	node.graph = g
	g.Root.Children = append(g.Root.Children, SubgraphChild{Node: node})
}

// AddSubgraph appends a subgraph as a root-level child of the graph.
//
// Parameters:
//   - sg: the subgraph to append.
func (g *Graph) AddSubgraph(sg *Subgraph) {
	g.Root.Children = append(g.Root.Children, SubgraphChild{Subgraph: sg})
}

// Nodes returns all nodes in the graph by walking the tree recursively.
// The returned slice is in tree-walk order (depth-first, declaration order).
func (g *Graph) Nodes() []*Node {
	var nodes []*Node
	collectNodes(g.Root.Children, &nodes)
	return nodes
}

// collectNodes recursively collects all nodes from a children list.
func collectNodes(children []SubgraphChild, out *[]*Node) {
	for _, c := range children {
		if c.Node != nil {
			*out = append(*out, c.Node)
		}
		if c.Subgraph != nil {
			collectNodes(c.Subgraph.Children, out)
		}
	}
}

// ExecutionContext returns a point to the graph's execution context.
func (g *Graph) ExecutionContext() *ExecutionContext { return g.ctx }

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

// SubgraphByID returns the subgraph with the given ID, or nil if not found.
// Searches the tree recursively.
func (g *Graph) SubgraphByID(id string) *Subgraph {
	return findSubgraph(g.Root.Children, id)
}

// ResolveExecutable returns the executable unit with the given ID, or an
// error if no such unit exists. Nodes and subgraphs share one ID space
// (Phase 7 invariant); ResolveExecutable is the single lookup gather,
// choose, and other combinators use to resolve a body reference.
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

// findSubgraph recursively searches for a subgraph by ID.
func findSubgraph(children []SubgraphChild, id string) *Subgraph {
	for _, c := range children {
		if c.Subgraph != nil {
			if c.Subgraph.ID() == id {
				return c.Subgraph
			}
			if found := findSubgraph(c.Subgraph.Children, id); found != nil {
				return found
			}
		}
	}
	return nil
}

// Rebind replaces the graph's execution context and clears the action cache.
//
// This is the handoff point between planning and execution. During planning, the graph holds the StarlarkRuntime's
// context (read-only root, no recovery site). At execution time, the executor creates a new ExecutionContext with a
// confined root, recovery site, and execution-specific settings, then calls Rebind to switch the graph over.
//
// Parameters:
//   - ctx: the new execution context.
func (g *Graph) Rebind(ctx *ExecutionContext) {
	g.ctx = ctx
	for _, n := range g.Nodes() {
		n.graph = g
	}
}

// Execute runs an executable unit (Node or Subgraph) with caller-supplied slot overrides.
//
// Execute is the unified entry point for every top-level execution path: top-level graph runs, subgraph invocations,
// gather iterations, choose branches, and test harnesses all funnel through here. Resolution order per slot is
// overrides[param.Name] when present, then the unit's baked-in Slot.Value (ImmediateValue, PromiseValue, or
// EnvironmentValue) resolved via its Resolve(env, results) method.
//
// Overrides route to topological root children only — non-root children receive inputs via promises, not from outside
// the subgraph. Overrides are runtime-only; they do not serialize into the graph.
//
// Execute creates a fresh recovery stack scoped to this call; compensation on failure unwinds only actions executed
// during this invocation. Nested combinators that need to own their own stack call [Graph.ExecuteWithStack] instead.
//
// Parameters:
//   - exec: the executable unit to run.
//   - overrides: caller-supplied slot overrides; nil for none.
//
// Returns:
//   - any: the unit's terminal result, or nil when it produces no output.
//   - error: non-nil if the unit fails or the graph is not yet rebound.
func (g *Graph) Execute(exec ExecutableUnit, overrides map[string]SlotValue) (any, error) {

	if exec == nil {
		return nil, fmt.Errorf("Execute: exec is nil")
	}

	if g.ctx == nil {
		return nil, fmt.Errorf("Execute: graph has no execution context; call Rebind first")
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
//   - ctx: the combinator's scoped cancellation context; propagates to iteration bodies via dispatch.
//   - exec: the executable unit to run for this iteration.
//   - stack: the caller-owned recovery stack that collects the iteration's compensations.
//   - overrides: caller-supplied slot overrides for the iteration, typically the bound iteration input.
//
// Returns:
//   - any: the unit's terminal result, or nil when it produces no output.
//   - error: non-nil if the unit fails or the graph is not yet rebound; the stack is returned to the caller intact.
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
// state and cancellation scope thread from the top-level entry down through recursive executeChildren calls. This is
// the hook site: observability hooks and cancellation checks attached here (the ctx.Err() check happens in
// executeNode) see every unit dispatch regardless of nesting depth.
//
// [Graph.Execute] and [GraphExecutor.Run] are the two external bootstraps that seed a fresh executor and stack
// before calling dispatch; [Graph.ExecuteWithStack] is the combinator entry (e.g. gather) that brings its own
// stack and scoped ctx. executeChildren reuses its caller's executor, stack, and ctx so compensation unwinding and
// cancellation propagation see the entire chain.
//
// Parameters:
//   - ctx: the cancellation context threaded from the nearest entry point.
//   - e: the live executor whose hooks and state persist across the dispatch chain.
//   - stack: the active recovery stack compensations are pushed onto.
//   - exec: the executable unit to dispatch; must be a *Node or *Subgraph.
//   - results: the accumulated node results for promise resolution.
//   - overrides: caller-supplied slot overrides, if any.
//
// Returns:
//   - any: the unit's terminal result, or nil when it produces no output.
//   - error: non-nil if the unit fails or the exec type is unrecognized.
func (g *Graph) dispatch(ctx context.Context, e *GraphExecutor, stack *RecoveryStack, exec ExecutableUnit, results map[string]any, overrides map[string]SlotValue) (any, error) {

	switch unit := exec.(type) {

	case *Node:
		result := e.executeNode(ctx, unit, results, stack, overrides)
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
// This is used for computing checksums and verifying signatures.
func (g *Graph) CanonicalContent() ([]byte, error) {

	type canonicalGraph struct {
		Children   []SubgraphChild `yaml:"children"`
		Collisions []Collision     `yaml:"collisions,omitempty"`
		Context    Provenance      `yaml:"context"`
		Edges      []Edge          `yaml:"edges,omitempty"`
		State      GraphState      `yaml:"state"`
		Timestamp  string          `yaml:"timestamp"`
		Version    string          `yaml:"version"`
	}

	canonical := canonicalGraph{
		Children:   g.Root.Children,
		Collisions: g.Collisions,
		Context:    g.Provenance,
		Edges:      g.Root.Edges,
		State:      g.State,
		Timestamp:  g.Timestamp.Format(time.RFC3339),
		Version:    g.Version,
	}

	return yaml.Marshal(canonical)
}

// Summary calculates execution statistics from nodes.
//
// Returns:
//   - GraphExecutionSummary: the computed summary.
func (g *Graph) Summary() GraphExecutionSummary {

	g.summary = newGraphExecutionSummary(g.Nodes())
	return g.summary
}

// SortChildren sorts a children list by the given edges using Kahn's algorithm.
// Nodes and subgraphs are peers — both are vertices referenced by ID.
// Returns the children in topological order. If no edges, returns declaration order.
func SortChildren(children []SubgraphChild, edges []Edge) []SubgraphChild {

	if len(edges) == 0 || len(children) <= 1 {
		return children
	}

	// Build ID → index map and extract node pointers for topological sort.
	idToChild := make(map[string]SubgraphChild, len(children))
	for _, c := range children {
		idToChild[c.ChildID()] = c
	}

	return topologicalSortChildren(children, edges)
}

// Serialize writes the graph to the given encoder.
//
// The checksum is computed before encoding.
//
// Usage:
//
//	enc := yaml.NewEncoder(file)
//	enc.SetIndent(2)
//	defer enc.Close()
//	g.Serialize(enc)
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

	// Annotations holds extensible metadata (serialized to receipts).
	Annotations map[string]string

	// Error message if status is failed.
	Error string

	// Layer is the repository layer (base, team, personal).
	Layer string

	// Origin this node belongs to.
	Origin string

	// Receiver is the dotted receiver + method name (e.g., "flow.complete", "file.write_text").
	// At execution time, the executor splits this into receiver name and method name, looks up the
	// ProviderReceiverType from the registry, constructs the provider, and dispatches via Method.Do.
	Receiver string

	// Retry is the retry policy for this node (nil = no retry).
	Retry *RetryPolicy

	// Slots holds input values for this node, ordered by method parameter position.
	Slots []*Slot

	// Status of this node: pending, completed, skipped, failed.
	Status NodeStatus

	// Timestamp is when this action completed.
	Timestamp string

	graph       *Graph
	action      Action // override for testing — bypasses Receiver lookup when set
	method      *Method
	slotsByName map[string]*Slot
}

// NewNode constructs a Node with the given identifier. Additional fields may
// be set on the returned pointer; the identifier is immutable after this call.
func NewNode(id string) *Node {
	return &Node{executableUnit: executableUnit{id: id}}
}

// region EXPORTED METHODS

// region State management

// SetAction sets an action override on this node. When set, Action() returns this directly
// instead of resolving via Receiver name and registry. Used by tests that inject mock actions.
func (n *Node) SetAction(a Action) { n.action = a }

// Action returns the resolved action for this node.
//
// If an action override is set (via SetAction), it is returned directly. Otherwise, the action
// is looked up by name through the graph's execution context registry.
//
// Returns:
//   - Action: the resolved action.
//   - error: non-nil if the receiver name is invalid or the provider cannot be constructed.
func (n *Node) Action() (Action, error) {
	if n.action != nil {
		return n.action, nil
	}
	return n.graph.ctx.ActionByName(n.Receiver)
}

// ExecutionContext returns the execution context from this node's parent graph.
//
// Returns:
//   - *ExecutionContext: the graph's execution context.
func (n *Node) ExecutionContext() *ExecutionContext { return n.graph.ctx }

// Bind associates this node with its resolved Method. Must be called before
// SetSlot. Populates the node's parameter surface from the method.
func (n *Node) Bind(method *Method) {

	n.method = method
	n.parameters = method.Parameters()
	n.rebuildSlotsByName()
}

// Method returns the bound method, or nil if unbound.
func (n *Node) Method() *Method { return n.method }

// SlotByName returns the Slot with the given parameter name, or nil if absent.
func (n *Node) SlotByName(name string) *Slot {

	return n.slotsByName[name]
}

// SetSlot sets a slot's value. Node must be bound to a method.
// The value may be any of ImmediateValue, PromiseValue, or EnvironmentValue.
func (n *Node) SetSlot(name string, value SlotValue) {

	param := n.requireParam(name, "SetSlot")
	n.setSlot(&Slot{Parameter: param, Value: value})
}

// endregion

// region Behaviors

// ResolvedSlots returns all slot values as a flat map, resolving promises and environment bindings.
func (n *Node) ResolvedSlots(props Properties, results map[string]any) map[string]any {

	return n.ResolveSlots(env, results, nil)
}

// ResolveSlots returns all slot values with caller-supplied overrides applied.
// For each slot, if overrides contains an entry keyed by the parameter name,
// that entry's Resolve is used; otherwise the baked-in Slot.Value is resolved.
// Overrides whose keys do not match any slot parameter are silently ignored —
// the caller-facing parameter surface is the authority.
func (n *Node) ResolveSlots(env RuntimeEnvironment, results map[string]any, overrides map[string]SlotValue) map[string]any {

	out := make(map[string]any, len(n.Slots))
	for _, slot := range n.Slots {
		name := slot.Parameter.Name
		if ov, ok := overrides[name]; ok {
			out[name] = ov.Resolve(env, results)
			continue
		}
		out[name] = slot.Value.Resolve(env, results)
	}
	return out
}

// endregion

// region UNEXPORTED NODE METHODS

func (n *Node) requireParam(name, caller string) Parameter {

	if n.method == nil {
		panic(fmt.Sprintf("%s: node %q is not bound to a method", caller, n.ID()))
	}
	param, ok := n.method.ParameterByName(name)
	if !ok {
		panic(fmt.Sprintf("%s: parameter %q not found on method %s", caller, name, n.method.Name()))
	}
	return param
}

func (n *Node) setSlot(slot *Slot) {

	for i, existing := range n.Slots {
		if existing.Parameter.Name == slot.Parameter.Name {
			n.Slots[i] = slot
			n.slotsByName[slot.Parameter.Name] = slot
			return
		}
	}
	n.Slots = append(n.Slots, slot)
	if n.slotsByName == nil {
		n.slotsByName = make(map[string]*Slot)
	}
	n.slotsByName[slot.Parameter.Name] = slot
}

func (n *Node) rebuildSlotsByName() {

	n.slotsByName = make(map[string]*Slot, len(n.Slots))
	for _, slot := range n.Slots {
		n.slotsByName[slot.Parameter.Name] = slot
	}
}

// endregion

// endregion

// NodeStatus represents the execution status of a node.
type NodeStatus string

// NodeStatus constants define the possible statuses of a node.
const (
	// StatusCompleted indicates the node executed successfully.
	StatusCompleted NodeStatus = "completed"
	// StatusFailed indicates the node failed during execution.
	StatusFailed NodeStatus = "failed"
	// StatusPending indicates the node has not yet been executed.
	StatusPending NodeStatus = "pending"
	// StatusSkipped indicates the node was skipped.
	StatusSkipped NodeStatus = "skipped"
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
func newGraphExecutionSummary(nodes []*Node) GraphExecutionSummary {

	s := &graphExecutionSummary{
		byAction: make(map[string]*actionExecutionSummary),
	}
	s.total = len(nodes)

	for _, n := range nodes {

		action, ok := s.byAction[n.Receiver]
		if !ok {
			action = &actionExecutionSummary{}
			s.byAction[n.Receiver] = action
		}
		action.total++

		switch n.Status {
		case StatusCompleted:
			s.completed++
			action.completed++
		case StatusFailed:
			s.failed++
			action.failed++
		case StatusSkipped:
			s.skipped++
			action.skipped++
		}
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
	Completed int                          `json:"completed" yaml:"completed"`
	Failed    int                          `json:"failed,omitempty" yaml:"failed,omitempty"`
	Skipped   int                          `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	Total     int                          `json:"total" yaml:"total"`
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


// topologicalSortChildren orders children (nodes and subgraphs) respecting edge constraints (Kahn's algorithm).
// Nodes and subgraphs are peers — both are vertices referenced by ChildID().
//
// Parameters:
//   - children: the children to sort.
//   - edges: the directed edges expressing ordering constraints.
//
// Returns:
//   - []SubgraphChild: the topologically sorted children (cycles broken by appending unsorted children).
func topologicalSortChildren(children []SubgraphChild, edges []Edge) []SubgraphChild { //nolint:gocognit // complexity is inherent to the algorithm

	childMap := make(map[string]SubgraphChild, len(children))
	inDegree := make(map[string]int, len(children))
	adj := make(map[string][]string)

	for _, c := range children {
		id := c.ChildID()
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
	for _, c := range children {
		id := c.ChildID()
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}

	var sorted []SubgraphChild
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

	if len(sorted) < len(children) {
		visited := make(map[string]bool, len(sorted))
		for _, c := range sorted {
			visited[c.ChildID()] = true
		}
		for _, c := range children {
			if !visited[c.ChildID()] {
				sorted = append(sorted, c)
			}
		}
	}

	return sorted
}

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
