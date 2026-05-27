// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package op owns the concrete graph data model shared by the execution engine, Starlark layer, and CLI tools.
//
// # Core types
//
//   - Graph: a directed graph of nodes and edges representing work to be done.
//   - Node: a single unit of work with an action to execute.
//   - Edge: a dependency relationship between nodes.
//
// # Graph lifecycle
//
// Graph is immutable: a re-executable plan that carries no per-execution state. RuntimeEnvironment is the mutable
// counterpart, scoped to one execution; it owns every per-run mutation (catalog state, results, variable resolution,
// recovery stack, status). A run produces a receipt (*RecoveryStack) — the audit trail of dispatches and their
// compensations — that, paired with the graph, suffices to restart execution where it left off.
package op

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

// GraphSerialVersion is the current graph serialization format version.
const GraphSerialVersion = "1"

// Graph represents an execution graph containing nodes and edges.
//
// This is THE graph used by both writ and lore — they differ only in content. Graph is immutable: the plan is
// re-executable any number of times against any number of fresh [RuntimeEnvironment]s without carrying execution
// state across runs.
type Graph struct {

	// checksum is the git-style integrity hash.
	checksum string

	// serialVersion is the graph format version.
	serialVersion string

	// provenance records what was planned, from what sources, and for what scope.
	provenance Provenance

	// resourceCatalog is the [ResourceCatalog] carried by the graph from planning into execution.
	//
	// At Graph construction ([NewGraph]) resourceCatalog points at a fresh empty [ResourceCatalog]. At the tail of
	// [plan.Provider.Assemble] the planning [RuntimeEnvironment]'s catalog (the one providers interned into during the
	// .star script's execution) is handed off to resourceCatalog, and the runtime environment's catalog is nilled,
	// freezing it. From that point on, the graph is self-contained: every later session-owner (a Go-side
	// [GraphExecutor.Run], a serializer, an inspector) reads it from resourceCatalog rather than from the long-gone
	// planning environment.
	//
	// [GraphExecutor.Run] never mutates resourceCatalog directly — it [ResourceCatalog.Clone]s it onto a fresh per-run
	// [RuntimeEnvironment.Catalog] so each Run gets an independent working catalog and the graph's planning catalog
	// stays pristine across "plan once, run many" reuse.
	//
	// Not serialized — the catalog re-materializes when planning re-runs (or is reconstituted from execution
	// telemetry).
	resourceCatalog *ResourceCatalog

	// root is the graph's root subgraph. Every top-level [*Node] and [*Subgraph] attaches to it via
	// [Subgraph.AddChild]; [GraphExecutor.Run] starts dispatch here. Set once at [NewGraph]; never replaced.
	root *Subgraph

	// signature contains the cryptographic signature (optional).
	signature *sops.Signature

	// timestamp is when the graph was created.
	timestamp time.Time

	// unitsByID is the unit symbol table populated by [Graph.UnmarshalJSON] / [Graph.UnmarshalYAML] (and, in
	// Phase 5, by `plan.assemble`).
	//
	// Maps unit ID to the materialized [*Node] or [*Subgraph]. Empty for Graphs built directly via the planning API
	// ([Graph.AddNode] / [Graph.AddSubgraph]) — those callers walk the children tree directly. Used by
	// [Subgraph.linkChildren] at unmarshal to resolve child IDs.
	unitsByID map[string]ExecutableUnit
}

// NewGraph constructs a Graph with a fresh empty [ResourceCatalog] and a fresh root [*Subgraph].
//
// Returns:
//   - *Graph: the freshly constructed graph.
func NewGraph() *Graph {

	return &Graph{
		resourceCatalog: NewResourceCatalog(),
		root:            newRootSubgraph(),
		timestamp:       time.Now(),
		serialVersion:   GraphSerialVersion,
	}
}

// region EXPORTED METHODS

// region State management

// Checksum returns the git-style integrity hash.
//
// Returns:
//   - `string`: the canonical "sha256:<hex>" form, or empty when unset.
func (g *Graph) Checksum() string { return g.checksum }

// Edges returns the ordering edges at the root level.
//
// Returns:
//   - `[]Edge`: the root-level dependency edges in insertion order.
func (g *Graph) Edges() []Edge { return g.root.edges }

// Filename returns the standard filename for this graph.
//
// Format: "<timestamp>.yaml", or "<scope>-<timestamp>.yaml" when [Provenance.Scope] is set.
//
// Returns:
//   - `string`: the formatted filename.
func (g *Graph) Filename() string {

	ts := g.timestamp.Format("2006-01-02T15-04-05")

	if g.provenance.Scope != "" {
		return fmt.Sprintf("%s-%s.yaml", g.provenance.Scope, ts)
	}

	return fmt.Sprintf("%s.yaml", ts)
}

// Nodes returns all nodes in the graph by walking the tree recursively.
//
// The returned slice is in tree-walk order (depth-first, declaration order).
//
// Returns:
//   - `[]*Node`: the flat node list in tree-walk order; nil when no nodes are present.
func (g *Graph) Nodes() []*Node { return g.root.descendantNodes() }

// Parameters returns the bubble-up variable surface of the graph.
//
// It is the deduplicated, type-checked set of [VariableValue] references walked across the root subgraph's children
// (plan-doc D3). It is consumed by the executor's preflight pass to drive [VariableResolver.Resolve].
//
// Returns:
//   - `[]Parameter`: the bubble-up surface, stable-sorted by Name. Returned even when error is non-nil, so callers can
//     render a best-effort surface alongside the diagnostic.
//   - `error`: an [errors.Join] of any same-name-different-type collisions detected during the walk; nil when the
//     walk succeeded without violations.
func (g *Graph) Parameters() ([]Parameter, error) { return g.root.Parameters() }

// Provenance returns the tool-specific planning metadata as a shallow value copy.
//
// The struct's scalar fields (Scope, SourceRoot, TargetPlatform, Tool, TargetRoot) are copy-safe. Its map and slice
// fields (CommitHashes, DirtyLayers, Features, Layers, Packages, Projects, Segments, Settings) share underlying
// storage with the original — mutations to those reference-typed children would reach back. Callers must treat the
// returned value as read-only.
//
// Returns:
//   - `Provenance`: the planning-metadata snapshot.
func (g *Graph) Provenance() Provenance { return g.provenance }

// ResourceCatalog returns the [ResourceCatalog] carried by the graph from planning into execution.
//
// Returns:
//   - `*ResourceCatalog`: the catalog pointer; callers must not mutate the catalog after graph construction.
func (g *Graph) ResourceCatalog() *ResourceCatalog { return g.resourceCatalog }

// Root returns the graph's root subgraph.
//
// Returns:
//   - `*Subgraph`: the root subgraph pointer; callers must not mutate the subgraph after graph construction.
func (g *Graph) Root() *Subgraph { return g.root }

// SerialVersion returns the graph format version stamped at construction.
//
// Returns:
//   - `string`: the value of [GraphSerialVersion] at the time the graph was constructed.
func (g *Graph) SerialVersion() string { return g.serialVersion }

// Signature returns the cryptographic signature or nil when the graph is unsigned.
//
// Returns:
//   - `*sops.Signature`: the signature pointer, or nil.
func (g *Graph) Signature() *sops.Signature { return g.signature }

// Subgraphs returns every [*Subgraph] descendant of the graph's root.
//
// The result does NOT include the root subgraph itself — it lists only authored / planner-emitted container units
// below it. Used by [Graph.UnitCount] and by harness assertions that want to count or inspect every executable unit
// produced by `plan.assemble`.
//
// Returns:
//   - `[]*Subgraph`: the descendant subgraphs in tree-walk order.
func (g *Graph) Subgraphs() []*Subgraph { return g.root.descendantSubgraphs() }

// Timestamp returns when the graph was created.
//
// Returns:
//   - `time.Time`: the construction timestamp set at [NewGraph].
func (g *Graph) Timestamp() time.Time { return g.timestamp }

// UnitCount returns the total count of [ExecutableUnit] descendants of the graph's root — both [*Node] and
// [*Subgraph] children. Excludes the root itself.
//
// This is the count the harness asserts against via `ctx.assert_equal(graph.unit_count(), n)`: a `plan.choose`
// container materializes as a Subgraph that holds its branch's children, so a script with `write_text` + `exists`
// + `choose(then=remove)` produces unit count 4 (3 Nodes + 1 Subgraph), not 3.
//
// Returns:
//   - `int`: the total descendant-unit count.
func (g *Graph) UnitCount() int { return len(g.Nodes()) + len(g.Subgraphs()) }

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
		Version   string      `yaml:"version"`
		Timestamp string      `yaml:"timestamp"`
		Children  []string    `yaml:"children"`
		Edges     []Edge      `yaml:"edges,omitempty"`
		Subgraphs []*Subgraph `yaml:"subgraphs,omitempty"`
		Nodes     []*Node     `yaml:"nodes,omitempty"`
		Context   Provenance  `yaml:"context"`
	}

	var rootEdges []Edge

	if g.root != nil {
		rootEdges = g.root.edges
	}

	subgraphs := g.root.descendantSubgraphs()
	sort.Slice(subgraphs, func(i, j int) bool { return subgraphs[i].ID() < subgraphs[j].ID() })

	nodes := g.root.descendantNodes()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID() < nodes[j].ID() })

	canonical := canonicalGraph{
		Version:   g.serialVersion,
		Timestamp: g.timestamp.Format(time.RFC3339),
		Children:  g.root.childIDs(),
		Edges:     rootEdges,
		Subgraphs: subgraphs,
		Nodes:     nodes,
		Context:   g.provenance,
	}

	return yaml.Marshal(canonical)
}

// ResolveExecutable returns the executable unit with the given ID, or an error if no such unit exists.
//
// Nodes and subgraphs share one ID space (Phase 7 invariant); ResolveExecutable is the single lookup gather, choose,
// and other combinators use to resolve a body reference.
//
// Parameters:
//   - `id`: the executable unit identifier to resolve.
//
// Returns:
//   - `ExecutableUnit`: the resolved unit (Root, a Subgraph descendant, or a Node).
//   - `error`: non-nil when no descendant or root matches `id`.
func (g *Graph) ResolveExecutable(id string) (ExecutableUnit, error) {

	if g.root != nil && g.root.ID() == id {
		return g.root, nil
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

// Serialize writes this graph through `enc`, dispatching to [Graph.MarshalJSON] or [Graph.MarshalYAML] per the
// encoder's concrete type. The result is the symbol-table wire form: top-level `children` IDs from Root, plus the
// flat `subgraphs` and `nodes` lists sorted by ID.
//
// Whatever value is currently in [Graph.Checksum] is emitted as-is; this method does not (re)compute it. Callers
// that want a fresh checksum compute it from [Graph.CanonicalContent] and assign before calling.
//
// Usage:
//
//	enc := yaml.NewEncoder(file)
//	enc.SetIndent(2)
//	defer enc.Close()
//	g.Serialize(enc)
//
// Parameters:
//   - `enc`: the destination encoder; both *json.Encoder and *yaml.Encoder satisfy [Encoder].
//
// Returns:
//   - `error`: the encoder's error, or nil on success.
func (g *Graph) Serialize(enc Encoder) error {

	return enc.Encode(g)
}

// AddNode appends a node as a root-level child of the graph.
//
// Routing through [Subgraph.AddChild] stamps the node's parent pointer to the graph's Root (plan-doc D11).
//
// Parameters:
//   - `node`: the node to append.
func (g *Graph) AddNode(node *Node) {

	g.root.AddChild(node)
}

// AddSubgraph appends a subgraph as a root-level child of the graph.
//
// Routing through [Subgraph.AddChild] stamps the subgraph's parent pointer to the graph's Root (plan-doc D11).
//
// Parameters:
//   - `sg`: the subgraph to append.
func (g *Graph) AddSubgraph(sg *Subgraph) {

	g.root.AddChild(sg)
}

// SubgraphByID returns the descendant subgraph with the given ID, or nil if no descendant has that ID.
//
// Searches the tree recursively; the graph root is never returned.
//
// Parameters:
//   - `id`: the Subgraph ID to find.
//
// Returns:
//   - `*Subgraph`: the matching descendant, or nil.
func (g *Graph) SubgraphByID(id string) *Subgraph { return g.root.descendantSubgraphByID(id) }

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
//
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
// program-construction error and panics via the assert package. Wire-form deserialization does NOT go through this
// constructor; the JSON / YAML decoder produces a zero-value Node and [Node.applyPayload] fills it from the payload,
// with the eventual registry-aware [LoadGraph] resolving the cached action name through the registry.
//
// Parameters:
//   - `id`: the node identifier; immutable after this call.
//   - `action`: the dispatch action; must be non-nil.
//
// Returns:
//   - `*Node`: the constructed node, with `id` and `action` set.
func NewNode(id string, action Action) *Node {

	assert.Truef(action != nil, "op.NewNode(%q): action must be non-nil", id)
	return &Node{executableUnit: executableUnit{id: id, action: action}}
}

// region EXPORTED METHODS

// region Behaviors

// Execute resolves slots, dispatches the action, and pushes a receipt at every exit.
//
// Entry checks are ordered: cancellation first (hard signal — `ctx.Err()` catches root/external cancel
// and any ancestor combinator's scoped cancel), then pause (soft signal — [GraphExecutor.Pause] sets
// a flag observed at this pause-point). A cancelled or paused check pushes its audit receipt and
// returns before the action runs.
//
// On a clean entry path, slots are resolved against the active stack (via [RecoveryStack.ResultByUnitID]
// for [PromiseValue] entries), the node-start hook fires, an [*ActivationRecord] is built, and the
// action's [Action.Do] is invoked. The audit trail — per-attempt history, outcome, captured slots,
// recovery state — lives on the receipt pushed onto `stack` at every exit. The return value is just
// control flow.
//
// Parameters:
//   - `ctx`: the cancellation context threaded from the parent dispatch.
//   - `executor`: the executor driving the run; provides hooks, the runtime environment, the
//     audit-receipt helper, and the pause-point hook.
//   - `stack`: the recovery stack the node's receipt pushes onto and that [PromiseValue.Resolve]
//     queries via [RecoveryStack.ResultByUnitID] for upstream unit results.
//   - `variables`: the per-call variable frame; resolves [VariableValue] slots and is stamped onto
//     the activation for the dispatched method.
//
// Returns:
//   - `any`: the dispatch's terminal result; nil on failure, cancellation, pause, or void return.
//   - `error`: non-nil on cancellation, pause ([ErrPaused]), missing action, or [Action.Do] error.
func (n *Node) Execute(ctx context.Context, executor *GraphExecutor, stack *RecoveryStack, variables map[string]Variable) (any, error) {

	nodeID := n.ID()

	// Exit 1: context cancelled before dispatch begins.
	if err := ctx.Err(); err != nil {
		executor.pushAuditReceipt(nodeID, stack, nil, nil, nil, err, "")
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
		executor.pushAuditReceipt(nodeID, stack, nil, nil, nil, err, "")
		return nil, err
	}

	runtimeEnvironment := executor.environment
	slots := n.ResolveSlots(variables, stack)
	executor.hooks.FireNodeStart(runtimeEnvironment, nodeID, slots)

	activationRecord := NewActivationRecord(executor.graph, n, runtimeEnvironment)
	activationRecord.Context = ctx
	activationRecord.Stack = stack
	activationRecord.Variables = variables
	result, complement, err := action.Do(activationRecord, slots)

	// Exit 3: Do returned an error.
	if err != nil {
		executor.pushAuditReceipt(nodeID, stack, slots, nil, complement, err, action.FullName())
		executor.hooks.FireNodeComplete(runtimeEnvironment, nodeID, nil, err)
		return nil, fmt.Errorf("%s: %w", action.Name(), err)
	}

	// Exit 4: successful dispatch.
	executor.pushAuditReceipt(nodeID, stack, slots, result, complement, nil, action.FullName())
	executor.hooks.FireNodeComplete(runtimeEnvironment, nodeID, result, nil)

	return result, nil
}

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
// Node never produces a non-nil error — there's no merging at the leaf — so the second return value exists purely
// for [ExecutableUnit.Parameters] signature alignment with [Subgraph.Parameters].
//
// Returns:
//   - `[]Parameter`: the variable bubble-up surface; nil when no slot carries a [VariableValue].
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

// endregion

// endregion

// region SUPPORTING TYPES

// Provenance contains tool-specific metadata stored in the graph.
//
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

// endregion
