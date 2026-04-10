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

	// Catalog is the append-only resource catalog for planning.
	//
	// One per Graph. Not serialized — planning-only state.
	Catalog *ResourceCatalog `json:"-" yaml:"-"`

	// Checksum is the git-style integrity hash.
	Checksum string `json:"checksum,omitempty" yaml:"checksum,omitempty"`

	// Collisions records source conflicts resolved during tree building (writ-specific).
	Collisions []Collision `json:"collisions,omitempty" yaml:"collisions,omitempty"`

	// Children are the root-level nodes and subgraphs in declaration order.
	// This is the structural representation of the graph — a tree of nodes and subgraphs.
	// Edges at this level provide ordering constraints between root-level children.
	Children []SubgraphChild `json:"children" yaml:"children"`

	// Edges are ordering constraints between root-level children.
	// Each edge references children by ID (both node IDs and subgraph IDs).
	Edges []Edge `json:"edges,omitempty" yaml:"edges,omitempty"`

	// Provenance records what was planned, from what sources, and for what scope.
	Provenance Provenance `json:"provenance" yaml:"provenance"`

	// Rollback records compensating actions executed during rollback (populated on failure).
	Rollback []RollbackEntry `json:"rollback,omitempty" yaml:"rollback,omitempty"`

	// Signature contains the cryptographic signature (optional).
	Signature *sops.Signature `json:"signature,omitempty" yaml:"signature,omitempty"`

	// State is the execution state (pending, executed, failed).
	State GraphState `json:"state" yaml:"state"`

	// Summary contains execution statistics (populated after Summary()).
	summary GraphExecutionSummary

	// Timestamp is when the graph was created/executed.
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`

	// Version is the graph format version.
	Version string `json:"version" yaml:"version"`

	// ctx is the execution context for this graph. Replaced by Rebind at the planning-to-execution handoff.
	ctx *ExecutionContext
}

// NewGraph creates a Graph with the given execution context.
//
// Parameters:
//   - ctx: the execution context providing registry, root, platform, and other session state.
func NewGraph(ctx *ExecutionContext) *Graph {

	return &Graph{
		Version:   GraphFormatVersion,
		Timestamp: time.Now(),
		State:     StatePending,
		ctx:       ctx,
	}
}

// region EXPORTED METHODS

// region State management

// AddNode appends a node as a root-level child of the graph and sets the node's back-reference.
//
// Parameters:
//   - node: the node to append.
func (g *Graph) AddNode(node *Node) {
	node.graph = g
	g.Children = append(g.Children, SubgraphChild{Node: node})
}

// AddSubgraph appends a subgraph as a root-level child of the graph.
//
// Parameters:
//   - sg: the subgraph to append.
func (g *Graph) AddSubgraph(sg *Subgraph) {
	g.Children = append(g.Children, SubgraphChild{Subgraph: sg})
}

// Nodes returns all nodes in the graph by walking the tree recursively.
// The returned slice is in tree-walk order (depth-first, declaration order).
func (g *Graph) Nodes() []*Node {
	var nodes []*Node
	collectNodes(g.Children, &nodes)
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
	return findSubgraph(g.Children, id)
}

// findSubgraph recursively searches for a subgraph by ID.
func findSubgraph(children []SubgraphChild, id string) *Subgraph {
	for _, c := range children {
		if c.Subgraph != nil {
			if c.Subgraph.ID == id {
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
		Children:   g.Children,
		Collisions: g.Collisions,
		Context:    g.Provenance,
		Edges:      g.Edges,
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
	// Annotations holds extensible metadata (serialized to receipts).
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	// Error message if status is failed.
	Error string `json:"error,omitempty" yaml:"error,omitempty"`

	// ID is the unique identifier (typically relative target path or package name).
	ID string `json:"id" yaml:"id"`

	// Layer is the repository layer (base, team, personal).
	Layer string `json:"layer,omitempty" yaml:"layer,omitempty"`

	// Origin this node belongs to.
	Origin string `json:"origin,omitempty" yaml:"origin,omitempty"`

	// Receiver is the dotted receiver + method name (e.g., "flow.complete", "file.write_text").
	// At execution time, the executor splits this into receiver name and method name, looks up the
	// ProviderReceiverType from the registry, constructs the provider, and dispatches via Method.Do.
	Receiver string `json:"receiver" yaml:"receiver"`

	// Retry is the retry policy for this node (nil = no retry).
	Retry *RetryPolicy `json:"retry,omitempty" yaml:"retry,omitempty"`

	// Slots holds input values for this node. Each slot can be:
	// - Immediate: value known at analysis time
	// - Promise: reference to another node's output (creates edge)
	Slots map[string]SlotValue `json:"slots,omitempty" yaml:"slots,omitempty"`

	// Status of this node: pending, completed, skipped, failed.
	Status NodeStatus `json:"status" yaml:"status"`

	// Timestamp is when this action completed.
	Timestamp string `json:"timestamp,omitempty" yaml:"timestamp,omitempty"`

	graph  *Graph
	action Action // override for testing — bypasses Receiver lookup when set
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

// SlotByName returns the resolved value of a slot.
//
// If the slot is a promise, returns nil (must be resolved by executor).
func (n *Node) SlotByName(name string) any {

	if n.Slots != nil {
		if sv, ok := n.Slots[name]; ok {
			if sv.IsImmediate() {
				return sv.Immediate
			}
		}
	}

	return nil
}

// SetSlotImmediate sets a slot to an immediate value.
func (n *Node) SetSlotImmediate(name string, value any) {

	if n.Slots == nil {
		n.Slots = make(map[string]SlotValue)
	}
	n.Slots[name] = SlotValue{Immediate: value}
}

// SetSlotPromise sets a slot to a promise (reference to another node).
func (n *Node) SetSlotPromise(name, nodeRef, slot string) {

	if n.Slots == nil {
		n.Slots = make(map[string]SlotValue)
	}
	n.Slots[name] = SlotValue{NodeRef: nodeRef, Slot: slot}
}

// SetSlotProxy sets a slot to a gather proxy reference.
func (n *Node) SetSlotProxy(name, gatherRef, field string) {

	if n.Slots == nil {
		n.Slots = make(map[string]SlotValue)
	}
	n.Slots[name] = SlotValue{GatherRef: gatherRef, Field: field}
}

// endregion

// region Behaviors

// RequireStringSlot returns the string value of a required slot.
// Returns an error if the slot is not set, or holds a non-string value.
// An empty string is valid — use SlotByName for optional slots where zero value is acceptable.
func (n *Node) RequireStringSlot(name string) (string, error) {

	v := n.SlotByName(name)

	if v == nil {
		return "", fmt.Errorf("slot %q: not set", name)
	}

	s, ok := v.(string)

	if !ok {
		return "", fmt.Errorf("slot %q: expected string, got %T", name, v)
	}

	return s, nil
}

// ResolvedSlots returns all slot values as a flat map.
// Promise slots are resolved from the results map; immediate slots are returned
// directly. Proxy slots are resolved from the optional proxyCtx map (used by
// gather for per-iteration item binding).
// Pass nil for results when all slots are immediate (e.g., in tests).
func (n *Node) ResolvedSlots(results map[string]any, proxyCtx ...map[string]any) map[string]any {

	slots := make(map[string]any, len(n.Slots))
	for name, sv := range n.Slots {
		switch {
		case sv.IsProxy():
			if len(proxyCtx) > 0 && proxyCtx[0] != nil {
				if item, ok := proxyCtx[0][sv.GatherRef]; ok {
					slots[name] = fieldAccess(item, sv.Field)
				}
			}
		case sv.IsPromise():
			if results != nil {
				if val, ok := results[sv.NodeRef]; ok {
					slots[name] = val
				}
			}
		default:
			slots[name] = sv.Immediate
		}
	}
	return slots
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

// SlotValue represents a value that fills a slot in a node.
// Three variants, mutually exclusive:
//   - Immediate: value known at analysis time
//   - Promise: reference to another node's output (NodeRef)
//   - Proxy: reference to a gather iteration item (GatherRef + Field)
type SlotValue struct {
	// Field is the field name to access on the proxy item.
	Field string `json:"field,omitempty" yaml:"field,omitempty"`

	// GatherRef is the gather node ID for proxy resolution.
	GatherRef string `json:"gather_ref,omitempty" yaml:"gather_ref,omitempty"`

	// Immediate is the direct value (any type, known at analysis time).
	Immediate any `json:"immediate,omitempty" yaml:"immediate,omitempty"`

	// NodeRef is the ID of the node that produces this value (promise).
	NodeRef string `json:"node_ref,omitempty" yaml:"node_ref,omitempty"`

	// Slot is which output slot of the referenced node (empty = default output).
	Slot string `json:"slot,omitempty" yaml:"slot,omitempty"`
}

// IsImmediate returns true if this slot value is an immediate value.
func (s SlotValue) IsImmediate() bool {
	return !s.IsPromise() && !s.IsProxy()
}

// IsPromise returns true if this slot value is a promise (reference to another node).
func (s SlotValue) IsPromise() bool {
	return s.NodeRef != ""
}

// IsProxy returns true if this slot value is a gather proxy reference.
func (s SlotValue) IsProxy() bool {
	return s.GatherRef != ""
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

// fieldAccess extracts a named field from a value.
// Supports map[string]any for structured items.
func fieldAccess(item any, field string) any {

	if field == "" {
		return item
	}
	if m, ok := item.(map[string]any); ok {
		return m[field]
	}
	return nil
}

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
