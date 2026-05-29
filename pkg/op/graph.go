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
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

const GraphKind = "com.noblefactor.DevLore.Graph"
const GraphSchemaVersion = 1

// Graph represents an execution graph containing nodes and edges.
//
// This is THE graph used by both writ and lore — they differ only in content. Graph is immutable: the plan is
// re-executable any number of times against any number of fresh [RuntimeEnvironment]s without carrying execution
// state across runs.
type Graph struct {

	// magicNumber identifies the graph format.
	kind string

	// schemaVersion is the graph format version.
	schemaVersion uint32

	// checksum is the git-style integrity hash.
	checksum string

	// signature contains the cryptographic signature (optional).
	signature *sops.Signature

	// timestamp is when the graph was created.
	timestamp time.Time

	// origin records the tool's stamp on the graph: identity, publisher context, and creation environment.
	origin Origin

	// resourceCatalog is the [ResourceCatalog] carried by the graph from planning into execution.
	//
	// Supplied at construction by [NewGraph]: the caller (typically [plan.Provider.Assemble]) hands in the planning
	// [RuntimeEnvironment]'s catalog — the one providers interned into during the .star script's execution — and nils
	// the env's reference before the call. From that point on the graph is self-contained: every later session-owner
	// (a Go-side [GraphExecutor.Run], a serializer, an inspector) reads from resourceCatalog rather than from the
	// long-gone planning environment. When [NewGraph] is called with a nil catalog, it defaults to a fresh empty
	// [ResourceCatalog].
	//
	// [GraphExecutor.Run] never mutates resourceCatalog directly — it [ResourceCatalog.Clone]s it onto a fresh per-run
	// [RuntimeEnvironment.ResourceCatalog] so each Run gets an independent working catalog and the graph's planning catalog
	// stays pristine across "plan once, run many" reuse.
	//
	// Not serialized — the catalog re-materializes when planning re-runs (or is reconstituted from execution
	// telemetry).
	resourceCatalog *ResourceCatalog

	// root is the graph's root subgraph. [NewGraph] constructs it from the supplied `units` (top-level children),
	// `retryPolicy`, `errorAction`, and `slots`, calling [Subgraph.AddChild] to attach each child and stamp its parent
	// pointer to the root (plan-doc D11). [GraphExecutor.Run] starts dispatch here. Set once at construction; never
	// replaced.
	root *Subgraph

	// unitsByID is the unit symbol table mapping each [ExecutableUnit.ID] to the materialized [*Node] or [*Subgraph].
	//
	// Populated at construction by [NewGraph], which walks the root subgraph's descendant nodes and subgraphs after
	// edges are materialized and indexes every reachable unit. The unmarshal path ([Graph.UnmarshalJSON] /
	// [Graph.UnmarshalYAML]) fills it as the wire form is reconstructed, then [Subgraph.linkChildren] resolves
	// placeholder child IDs against the table.
	unitsByID map[string]ExecutableUnit
}

// NewGraph constructs a sealed [*Graph] from its constituent parts.
//
// Structural state is supplied at construction time; the returned Graph carries no public setters that mutate its
// fields. Per the phase-8 immutability invariant, every later session-owner (a [GraphExecutor.Run], a serializer, an
// inspector) reads from this Graph without changing it.
//
// Pipeline:
//
//  1. Build the root [*Subgraph] from `units`, `retryPolicy`, `errorAction`, and `slots`. Children are appended via
//     [Subgraph.AddChild] in order; the slot map populates via [Subgraph.SetSlot].
//  2. [Subgraph.MaterializeEdges] computes the edge set from PromiseValue UnitRefs and Resource producerIDs.
//  3. [Subgraph.SortAll] topologically sorts each reachable Subgraph's children.
//  4. The Graph value is built with `origin`, `resourceCatalog` (defaulting to a fresh empty [*ResourceCatalog] when
//     nil), the root, the serialization version, the current timestamp, and a walked `unitsByID` table.
//  5. [Graph.CanonicalContent] is computed and used as input to:
//     - [GitStyleChecksum] for the graph's integrity hash.
//     - [sops.Client.Sign] (when `sopsClient` is non-nil) for the cryptographic signature. If no signing backend is
//     configured, the returned signature is nil — by design.
//
// Parameters:
//   - `origin`: the tool's stamp on the graph: identity, publisher context, and creation environment. Zero value is
//     permitted for graphs built outside a tooling context.
//   - `units`: the top-level [ExecutableUnit] children of the graph's root subgraph, in their planned order.
//     Empty or nil → a graph with an empty root.
//   - `slots`: frame-level slot values to seed on the root subgraph. Nil treated as the empty map.
//   - `resourceCatalog`: the [*ResourceCatalog] the graph carries from planning into execution. Nil → a fresh empty
//     catalog. Non-nil → the caller's catalog (typically the planning [*RuntimeEnvironment.ResourceCatalog], transferred onto
//     the graph at the end of [plan.Provider.Assemble]).
//   - `errorAction`: the pre-built error-action subgraph (typically the output of plan-side
//     `subgraphFromInvocations`), or nil for "no error action".
//   - `retryPolicy`: the resolved retry policy for the root subgraph, or nil for "no retry".
//   - `sopsClient`: the SOPS client used to sign the canonical content. Nil → unsigned (the signature field remains
//     nil); a non-nil client with no signing backends produces a nil signature gracefully (per [sops.Client.Sign]).
//
// Returns:
//   - `*Graph`: the sealed graph, with checksum populated and signature populated when applicable.
//   - `error`: non-nil when canonical-content serialization or signing fails.
func NewGraph(
	origin Origin,
	units []ExecutableUnit,
	slots map[string]SlotValue,
	resourceCatalog *ResourceCatalog,
	errorAction *Subgraph,
	retryPolicy *RetryPolicy,
	sopsClient *sops.Client,
) (*Graph, error) {

	if resourceCatalog == nil {
		resourceCatalog = NewResourceCatalog()
	}

	root := newRootSubgraph(units, slots, retryPolicy, errorAction)

	g := &Graph{
		kind:            GraphKind,
		schemaVersion:   GraphSchemaVersion,
		origin:          origin,
		resourceCatalog: resourceCatalog,
		root:            root,
		timestamp:       time.Now(),
	}

	g.unitsByID = make(map[string]ExecutableUnit)

	for _, n := range g.root.descendantNodes() {
		g.unitsByID[n.ID()] = n
	}

	for _, sg := range g.root.descendantSubgraphs() {
		g.unitsByID[sg.ID()] = sg
	}

	canonical, err := g.CanonicalContent()

	if err != nil {
		return nil, fmt.Errorf("NewGraph: canonical content: %w", err)
	}

	g.checksum = GitStyleChecksum("graph", canonical)

	if sopsClient != nil {
		signature, err := sopsClient.Sign(canonical)
		if err != nil {
			return nil, fmt.Errorf("NewGraph: sign: %w", err)
		}
		g.signature = signature
	}

	return g, nil
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
// Format: "<timestamp>.yaml", or "<scope>-<timestamp>.yaml" when [Origin.Scope] is set.
//
// Returns:
//   - `string`: the formatted filename.
func (g *Graph) Filename() string {

	ts := g.timestamp.Format("2006-01-02T15-04-05")

	if g.origin.Scope != "" {
		return fmt.Sprintf("%s-%s.yaml", g.origin.Scope, ts)
	}

	return fmt.Sprintf("%s.yaml", ts)
}

// Kind returns the canonical identifier of this graph's artifact type.
//
// Stamped at construction from [GraphKind]. Paired with [Graph.SerialVersion] (the numeric schema version), it serves
// as the wire-format discriminator that distinguishes a Devlore Graph from other YAML/JSON artifacts that might share a
// stream or path, and lets readers reject payloads of the wrong shape before attempting to decode them.
//
// Returns:
//   - `string`: the value of [GraphKind] at the time the graph was constructed.
func (g *Graph) Kind() string { return g.kind }

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
//   - `[]Parameter`: the bubble-up surface, stable-sorted by Name. Returned even when `error` is non-nil, so callers
//     can render a best-effort surface alongside the diagnostic.
//   - `error`: an [errors.Join] of any same-name-different-type collisions detected during the walk; nil when the walk
//     succeeded without violations.
func (g *Graph) Parameters() ([]Parameter, error) { return g.root.Parameters() }

// Origin returns the tool-stamped graph metadata as a shallow value copy.
//
// The struct's scalar fields (Scope, SourceRoot, TargetPlatform, Tool, TargetRoot) are copy-safe. Its map and slice
// fields (CommitHashes, DirtyLayers, Features, Layers, Packages, Projects, Segments, Settings) share underlying
// storage with the original — mutations to those reference-typed children would reach back. Callers must treat the
// returned value as read-only.
//
// Returns:
//   - `Origin`: the tool-stamped metadata.
func (g *Graph) Origin() Origin { return g.origin }

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
//   - `string`: the value of [GraphSchemaVersion] at the time the graph was constructed.
func (g *Graph) SerialVersion() uint32 { return g.schemaVersion }

// Signature returns the cryptographic signature or nil when the graph is unsigned.
//
// Returns:
//   - `*sops.Signature`: the signature pointer, or nil.
func (g *Graph) Signature() *sops.Signature { return g.signature }

// Subgraphs returns every [*Subgraph] descendant of the graph's root.
//
// The result does NOT include the root subgraph itself — it lists only authored / planner-emitted container units below
// it. Used by [Graph.UnitCount] and by harness assertions that want to count or inspect every executable unit produced
// by `plan.assemble`.
//
// Returns:
//   - `[]*Subgraph`: the descendant subgraphs in tree-walk order.
func (g *Graph) Subgraphs() []*Subgraph { return g.root.descendantSubgraphs() }

// Timestamp returns when the graph was created.
//
// Returns:
//   - `time.Time`: the construction timestamp set at [NewGraph].
func (g *Graph) Timestamp() time.Time { return g.timestamp }

// UnitCount returns the total count of [ExecutableUnit] descendants of the graph's root.
//
// Both [*Node] and [*Subgraph] are children. The count excludes the root itself.
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
		Kind          string      `yaml:"kind"`
		SchemaVersion uint32      `yaml:"schema_version"`
		Timestamp     string      `yaml:"timestamp"`
		Children      []string    `yaml:"children"`
		Edges         []Edge      `yaml:"edges,omitempty"`
		Subgraphs     []*Subgraph `yaml:"subgraphs,omitempty"`
		Nodes         []*Node     `yaml:"nodes,omitempty"`
		Origin        Origin      `yaml:"origin"`
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
		Kind:          g.kind,
		SchemaVersion: g.schemaVersion,
		Timestamp:     g.timestamp.Format(time.RFC3339),
		Children:      g.root.childIDs(),
		Edges:         rootEdges,
		Subgraphs:     subgraphs,
		Nodes:         nodes,
		Origin:        g.origin,
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

func (g *Graph) MarshalJSON() ([]byte, error) { return json.Marshal(g.marshalData()) }

func (g *Graph) MarshalYAML() (any, error) { return g.marshalData(), nil }

// endregion

// endregion

// graphData is the canonical wire shape for Graph.
//
// Used by both JSON and YAML marshalers; the tags apply to whichever encoder reads the struct. Top-level `children` and
// `edges` project up from `Graph.Root`, mirroring Root's own wire shape. `subgraphs` and `nodes` are flat symbol tables
// — every non-root Subgraph and every Node in the graph, sorted by ID.
type graphData struct {

	// Identity
	Kind          string    `json:"kind"                 yaml:"kind"`
	SchemaVersion uint32    `json:"schema_version"       yaml:"schema_version"`
	Timestamp     time.Time `json:"timestamp"            yaml:"timestamp"`
	Origin        Origin    `json:"origin"               yaml:"origin"`

	// Integrity
	Checksum  string          `json:"checksum,omitempty"   yaml:"checksum,omitempty"`
	Signature *sops.Signature `json:"signature,omitempty"  yaml:"signature,omitempty"`

	// Content
	Children  []string       `json:"children"             yaml:"children"`
	Edges     []Edge         `json:"edges,omitempty"      yaml:"edges,omitempty"`
	Nodes     []nodeData     `json:"nodes,omitempty"      yaml:"nodes,omitempty"`
	Subgraphs []subgraphData `json:"subgraphs,omitempty"  yaml:"subgraphs,omitempty"`
}

// region UNEXPORTED METHODS

// region Behaviors

// marshalData projects this Graph to its canonical wire shape.
//
// Each Node is projected to a [nodePayload] and each Subgraph to a [subgraphPayload] inline — the
// wire form is the payload structs themselves, never the in-memory unit types. Unmarshaling does the
// reverse via [LoadGraph], which goes through the [RuntimeEnvironment]'s registry to bind actions as
// units are reconstructed; there is no [json.Unmarshaler] on Graph, Node, or Subgraph because the
// stdlib decoder has no registry in scope.
//
// Returns:
//   - `graphPayload`: the projected payload.
func (g *Graph) marshalData() graphData {

	var edges []Edge

	if g.root != nil {
		edges = g.root.edges
	}

	descendants := g.root.descendantSubgraphs()
	sort.Slice(descendants, func(i, j int) bool { return descendants[i].ID() < descendants[j].ID() })

	subgraphPayloads := make([]subgraphData, 0, len(descendants))
	for _, sg := range descendants {
		subgraphPayloads = append(subgraphPayloads, sg.marshalData())
	}

	descendantNodes := g.root.descendantNodes()
	sort.Slice(descendantNodes, func(i, j int) bool { return descendantNodes[i].ID() < descendantNodes[j].ID() })

	nodePayloads := make([]nodeData, 0, len(descendantNodes))
	for _, n := range descendantNodes {
		nodePayloads = append(nodePayloads, n.marshalPayload())
	}

	return graphData{

		// Identity
		Kind:          g.kind,
		SchemaVersion: g.schemaVersion,
		Timestamp:     g.timestamp,
		Origin:        g.origin,

		// Integrity
		Checksum:  g.checksum,
		Signature: g.signature,

		// Content
		Children:  g.root.childIDs(),
		Edges:     edges,
		Nodes:     nodePayloads,
		Subgraphs: subgraphPayloads,
	}
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

func (n *Node) MarshalJSON() ([]byte, error) { return json.Marshal(n.marshalPayload()) }

func (n *Node) MarshalYAML() (any, error) { return n.marshalPayload(), nil }

// Node intentionally has no [json.Unmarshaler] / yaml.Unmarshaler. The wire form decodes into
// [nodePayload] structs (held inside [graphPayload]); [LoadGraph] then walks those payloads and
// constructs each Node via [NewNode] with the registry-resolved [Action] in one pass — so a
// Node never exists in an action-less transient state outside [LoadGraph]'s internals.

// endregion

// endregion

// nodeData is the canonical wire shape for Node.
//
// ActionName is the sole identity field — sourced from `unit.Action().Name()` at marshal; consumed by
// the post-load Action-rebind link pass via `env.ActionByName(name)` at Rebind. Status / Error /
// Timestamp do not round-trip — they live on the recovery-stack receipts at execution time.
// Slots serialize as an object/dict keyed by parameter name; values are the sealed [SlotValue]
// variants ([ImmediateValue], [PromiseValue], [VariableValue]).
type nodeData struct {
	ID          string               `json:"id"                     yaml:"id"`
	ActionName  string               `json:"action_name,omitempty"  yaml:"action_name,omitempty"`
	Annotations map[string]string    `json:"annotations,omitempty"  yaml:"annotations,omitempty"`
	Layer       string               `json:"layer,omitempty"        yaml:"layer,omitempty"`
	Origin      string               `json:"origin,omitempty"       yaml:"origin,omitempty"`
	Retry       *RetryPolicy         `json:"retry,omitempty"        yaml:"retry,omitempty"`
	Slots       map[string]SlotValue `json:"slots,omitempty"        yaml:"slots,omitempty"`
}

// region UNEXPORTED METHODS

// region Behaviors

// marshalPayload projects this Node to its canonical wire shape.
//
// Returns:
//   - `nodePayload`: the projected payload.
func (n *Node) marshalPayload() nodeData {
	var actionName string
	if a := n.Action(); a != nil {
		actionName = a.Name()
	}
	return nodeData{
		ID:          n.id,
		ActionName:  actionName,
		Annotations: n.annotations,
		Layer:       n.Layer,
		Origin:      n.Origin,
		Retry:       n.RetryPolicy(),
		Slots:       n.slots,
	}
}

// endregion

// endregion

// region SUPPORTING TYPES

// Origin is the tool-stamped graph metadata: tool identity, publisher context, and creation environment
// (e.g., tool, scope, source root, target root, commit hashes, dirty flag, layers, packages, features)
// stored directly on the graph.
//
// Plan-time-written by tools, tool-read at runtime and beyond, never inspected by the framework. Sits
// strictly above the graph's internal structural content (nodes, subgraphs, edges) — not a manifest or
// inventory, but an immutable record of who produced the graph and under what conditions. See plan-doc
// D15 for the full role description.
type Origin struct {

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
