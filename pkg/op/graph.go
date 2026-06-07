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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

const GraphKind = "com.noblefactor.DevLore.Graph"
const GraphSchemaVersion = 1

// Graph represents an execution graph containing nodes and edges.
//
// This is THE graph used by both writ and lore — they differ only in content. Graph is immutable: the plan is
// re-executable any number of times against any number of fresh [RuntimeEnvironment]s without carrying execution state
// across runs.
type Graph struct {

	// kind is the canonical artifact-type identifier stamped from [GraphKind].
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
	//
	// Stored as the concrete [OriginBase] carrier (the only Origin implementation); [Graph.Origin] hands it back as
	// the [Origin] interface so tools can wrap it in a typed view.
	origin OriginBase

	// resourceCatalog is the [ResourceCatalog] carried by the graph from planning into execution.
	//
	// Supplied at construction by [NewGraph]: the caller (typically [plan.Provider.Assemble]) hands in the planning
	// [RuntimeEnvironment]'s catalog — the one providers interned into during the .star script's execution — and nils the
	// planning [RuntimeEnvironment]'s reference before the call. From that point on the graph is self-contained: every
	// later session-owner (a Go-side [GraphExecutor.Run], a serializer, an inspector) reads from resourceCatalog rather
	// than from the long-gone planning environment. When [NewGraph] is called with a nil catalog, it defaults to a fresh
	// empty [ResourceCatalog].
	//
	// [GraphExecutor.Run] never mutates resourceCatalog directly — it [ResourceCatalog.Clone]s it onto a fresh per-run
	// [RuntimeEnvironment.ResourceCatalog] so each Run gets an independent working catalog and the graph's planning
	// catalog stays pristine across "plan once, run many" reuse.
	//
	// Not serialized — the catalog re-materializes when planning re-runs (or is reconstituted from execution telemetry).
	resourceCatalog *ResourceCatalog

	// root is the graph's root subgraph. [NewGraph] constructs it from the supplied `units` (top-level children),
	// `retryPolicy`, `errorAction`, and `slots`, calling [Subgraph.AddChild] to attach each child and stamp its parent
	// pointer to the root (plan-doc D11). [GraphExecutor.Run] starts dispatch here. Set once at construction; never
	// replaced.
	root *Subgraph

	// unitsByID is the unit symbol table mapping each [ExecutableUnit.ID] to the materialized [*Node] or [*Subgraph].
	//
	// Populated at construction by [NewGraph], which walks the root subgraph's descendant nodes and subgraphs after edges
	// are materialized and indexes every reachable unit. The load path ([LoadGraph]) fills it as the wire form is
	// reconstructed, then [Subgraph.linkChildren] resolves placeholder child IDs against the table.
	unitsByID map[string]ExecutableUnit
}

// NewGraph constructs a sealed [*Graph] from a populated [*GraphSpec].
//
// Structural state is supplied at construction time; the returned Graph carries no public setters that mutate its
// fields. Per the phase-8 immutability invariant, every later session-owner (a [GraphExecutor.Run], a serializer, an
// inspector) reads from this Graph without changing it.
//
// Pipeline: build the root [*Subgraph] from the spec's units, slots, retry policy, and error action (which materializes
// edges and topologically sorts the children); assemble fresh [graphMetadata] (a now timestamp, the current schema
// version, the spec's origin and resource catalog — defaulting to a fresh empty [*ResourceCatalog] when nil); hand the
// root and metadata to the shared [buildGraph], which walks the unit table and computes the integrity checksum from
// [Graph.CanonicalContent]; and, when the spec carries a SOPS client, feed the canonical content to [sops.Client.Sign]
// for the signature (nil when no signing backend is configured). Construction signs after [buildGraph] because the
// signature is over the assembled canonical content; the load path preserves the document's signature instead.
//
// Parameters:
//   - `spec`: the populated graph spec. A zero `Origin` is permitted (graphs built outside a tooling context); a nil
//     `ResourceCatalog` defaults to a fresh empty catalog; a nil `SopsClient` leaves the graph unsigned.
//
// Returns:
//   - `*Graph`: the sealed graph, with checksum populated and signature populated when applicable.
//   - `error`: non-nil when canonical-content serialization or signing fails.
func NewGraph(spec *GraphSpec) (*Graph, error) {

	resourceCatalog := spec.ResourceCatalog

	if resourceCatalog == nil {
		resourceCatalog = NewResourceCatalog()
	}

	root, err := NewSubgraph(&spec.Root)

	if err != nil {
		return nil, fmt.Errorf("NewGraph: root subgraph: %w", err)
	}

	// spec.Origin is the op.Origin interface; the graph stores the concrete OriginBase carrier. Construction always
	// passes an OriginBase (tools build via NewOriginBase), so a nil / non-OriginBase value yields the zero origin.
	graphOrigin, _ := spec.Origin.(OriginBase)

	g, err := buildGraph(root, graphMetadata{
		schemaVersion:   GraphSchemaVersion,
		timestamp:       time.Now(),
		origin:          graphOrigin,
		resourceCatalog: resourceCatalog,
	})

	if err != nil {
		return nil, fmt.Errorf("NewGraph: %w", err)
	}

	// Signing runs after buildGraph: the signature is over the canonical content, which exists only once the graph is
	// assembled. Construction signs fresh via the spec's SOPS client (nil leaves the graph unsigned); the load path
	// preserves the document's signature through graphMetadata instead.
	if spec.SopsClient != nil {

		canonical, err := g.CanonicalContent()

		if err != nil {
			return nil, fmt.Errorf("NewGraph: canonical content: %w", err)
		}

		signature, err := spec.SopsClient.Sign(canonical)

		if err != nil {
			return nil, fmt.Errorf("NewGraph: sign: %w", err)
		}

		g.signature = signature
	}

	return g, nil
}

// NewGraphSpec returns a [*GraphSpec] whose root is seeded with the canonical root spec and is ready for fluent
// population via its With* setters.
//
// Seeding the root means every graph's root has ID "root" and binds "flow.subgraph" by name (resolved at dispatch) — the
// root runs through the same bound-action path as every other subgraph. This is the single root call site: inlining the
// spec here (rather than a shared factory) guarantees no other site can produce a divergent root. Because
// [SubgraphSpec.WithActionNamed] validates the action name against the global registry, NewGraphSpec requires the flow
// provider to be announced.
//
// Returns:
//   - `*GraphSpec`: a graph spec with its root pre-seeded.
func NewGraphSpec() *GraphSpec {
	return &GraphSpec{Root: *NewSubgraphSpec().WithID("root").WithActionNamed("flow.subgraph")}
}

// buildGraph assembles the single sealed [*Graph] from an already-prepared root and its [graphMetadata].
//
// This is the only place that hand-builds a [Graph] struct. Both preparers converge here: [NewGraph] derives the root's
// edges from slot-producers and assembles fresh metadata, while the load path preserves the document's edges and
// metadata. buildGraph is agnostic to edge provenance — it reads `root.edges` through [Graph.CanonicalContent]. It sets
// the struct fields from `metadata` and the root, derives `unitsByID` by walking the root's descendants, and recomputes
// the integrity checksum from the canonical content.
//
// The checksum is always recomputed here, never copied: a loaded document's recomputed checksum therefore equals the
// embedded one only when its edges and metadata round-trip intact, which makes the recomputation an implicit integrity
// check. The signature is taken verbatim from `metadata` (the load path's preserved document signature, or nil); fresh
// construction signing happens in [NewGraph] after this call, since signing needs the assembled canonical content.
//
// Parameters:
//   - `root`: the fully prepared root [*Subgraph]; its children must already be linked and its edges set before the
//     call, because buildGraph walks the descendants to build the unit table.
//   - `metadata`: the graph-level metadata (schema version, timestamp, signature, origin, resource catalog).
//
// Returns:
//   - `*Graph`: the sealed graph, with `unitsByID` and `checksum` populated.
//   - `error`: non-nil when canonical-content serialization fails.
func buildGraph(root *Subgraph, metadata graphMetadata) (*Graph, error) {

	g := &Graph{
		kind:            GraphKind,
		schemaVersion:   metadata.schemaVersion,
		signature:       metadata.signature,
		timestamp:       metadata.timestamp,
		origin:          metadata.origin,
		resourceCatalog: metadata.resourceCatalog,
		root:            root,
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
		return nil, fmt.Errorf("buildGraph: canonical content: %w", err)
	}

	g.checksum = GitStyleChecksum("graph", canonical)

	return g, nil
}

// LoadGraph decodes a wire-form graph (JSON or YAML) into a fully action-bound in-memory [*Graph].
//
// The decode path is registry-aware end-to-end: payload bytes are first decoded into the wire-form payload structs
// ([graphData], [nodeData], [subgraphData]); LoadGraph then hands the payload to [assembleGraph], which resolves each
// unit's action by short name through `env.Registry` and constructs each [*Node] / [*Subgraph] via [NewNode] /
// [NewSubgraph] with the resolved action — so no unit ever exists in a transient action-less state outside the load
// internals.
//
// After unit construction the load path rebuilds containment (child IDs → child pointers, topological order per subgraph
// edges) and validates edge endpoints. The returned graph holds no reference to the supplied env; pass it to
// [NewGraphExecutor] to execute.
//
// Parameters:
//   - `env`: the runtime environment whose registry resolves action names. Must be non-nil; the registry must contain
//     every action referenced in the wire form.
//   - `data`: the encoded bytes.
//   - `format`: "json" or "yaml" (or "yml") — case-insensitive.
//
// Returns:
//   - `*Graph`: the constructed graph with every unit's action bound.
//   - `error`: non-nil if decoding fails, the format is unsupported, any action name is unknown to the registry, any
//     child ID is dangling, or any edge endpoint fails to resolve.
func LoadGraph(env *RuntimeEnvironment, data []byte, format string) (*Graph, error) {

	if env == nil {
		return nil, fmt.Errorf("op.LoadGraph: nil environment")
	}

	var p graphData
	switch strings.ToLower(format) {
	case "json":
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("op.LoadGraph: json decode: %w", err)
		}
	case "yaml", "yml":
		if err := yaml.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("op.LoadGraph: yaml decode: %w", err)
		}
	default:
		return nil, fmt.Errorf("op.LoadGraph: unsupported format %q (use json, yaml, or yml)", format)
	}

	return assembleGraph(env, &p)
}

// assembleGraph constructs a [*Graph] from a decoded [graphData] payload — the dual to [Graph.marshalData].
//
// It prepares the root the load way: each unit's action is resolved through env.Registry and the concrete
// [*Node] / [*Subgraph] values are constructed via [assembleNode] / [assembleSubgraph]; the document's edges are set on
// the root directly (set and order preserved); the per-subgraph containment is rebuilt and edges validated. It then
// assembles the document's [graphMetadata] (schema version, timestamp, and the preserved signature from the payload) and
// hands the linked root plus that metadata to the shared [buildGraph] — never hand-building a [Graph] itself.
//
// Because the document's edges are preserved rather than re-derived, [buildGraph]'s recomputed checksum equals the
// payload's embedded checksum whenever the document round-trips intact; assembleGraph compares the two and rejects a
// mismatch as an integrity check against post-write alteration.
//
// Parameters:
//   - `env`: the runtime environment whose registry resolves action names.
//   - `p`: the decoded payload.
//
// Returns:
//   - `*Graph`: the constructed graph.
//   - `error`: non-nil on unresolved action name, dangling child ID, invalid edge endpoint, or a checksum mismatch.
func assembleGraph(env *RuntimeEnvironment, p *graphData) (*Graph, error) {

	root, err := NewSubgraph(NewSubgraphSpec().WithID("root").WithActionNamed("flow.subgraph"))
	if err != nil {
		return nil, fmt.Errorf("assembleGraph: root subgraph: %w", err)
	}

	// Preserve the document's root edges verbatim (set and order). buildGraph reads them through CanonicalContent, so the
	// recomputed checksum matches the document's; re-deriving here would drop hand-authored, non-slot-producer edges.
	root.edges = p.Edges

	var violations []error

	// Build the unit symbol table from the flat payload lists. Each unit comes into existence with its action already
	// bound — NewNode / NewSubgraph's assert.NonZero invariant holds.
	unitsByID := make(map[string]ExecutableUnit, len(p.Nodes)+len(p.Subgraphs))

	for i := range p.Nodes {
		node, err := assembleNode(env, &p.Nodes[i])
		if err != nil {
			violations = append(violations, err)
			continue
		}
		unitsByID[node.ID()] = node
	}

	for i := range p.Subgraphs {
		sg, err := assembleSubgraph(env, &p.Subgraphs[i])
		if err != nil {
			violations = append(violations, err)
			continue
		}
		unitsByID[sg.ID()] = sg
	}

	if len(violations) > 0 {
		return nil, errors.Join(violations...)
	}

	// Wire root's children + the per-subgraph child links. Each Subgraph's executableUnitsByID was pre-populated with
	// placeholder nil entries by assembleSubgraph from its Children list; linkChildren resolves each placeholder against
	// the now-complete unit table and populates executableUnits in topological order per edges.
	if len(p.Children) > 0 {
		root.executableUnitsByID = make(map[string]ExecutableUnit, len(p.Children))
		for _, id := range p.Children {
			root.executableUnitsByID[id] = nil
		}
	}
	if err := root.linkChildren(unitsByID); err != nil {
		violations = append(violations, err)
	}
	for _, sg := range root.descendantSubgraphs() {
		if err := sg.linkChildren(unitsByID); err != nil {
			violations = append(violations, err)
		}
	}

	violations = append(violations, root.validateEdges())
	for _, sg := range root.descendantSubgraphs() {
		violations = append(violations, sg.validateEdges())
	}

	if err := errors.Join(violations...); err != nil {
		return nil, err
	}

	g, err := buildGraph(root, graphMetadata{
		schemaVersion:   p.SchemaVersion,
		timestamp:       p.Timestamp,
		signature:       p.Signature,
		origin:          p.Origin,
		resourceCatalog: NewResourceCatalog(),
	})

	if err != nil {
		return nil, err
	}

	// Integrity check: buildGraph recomputed the checksum from the canonical content; it must equal the value the
	// document carried. A mismatch means the document was altered after it was written (or produced by an incompatible
	// canonicalization), so reject it rather than silently accept the recomputed value.
	if g.Checksum() != p.Checksum {
		return nil, fmt.Errorf("op.LoadGraph: checksum mismatch: document %q, recomputed %q", p.Checksum, g.Checksum())
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
//   - []Edge: the root-level dependency edges in insertion order.
func (g *Graph) Edges() []Edge { return g.root.edges }

// Filename returns the standard filename for this graph.
//
// Format: "<timestamp>.yaml", or "<scope>-<timestamp>.yaml" when [Origin.Scope] is set.
//
// Returns:
//   - `string`: the formatted filename.
func (g *Graph) Filename() string {

	ts := g.timestamp.Format("2006-01-02T15-04-05")

	if g.origin.Scope() != "" {
		return fmt.Sprintf("%s-%s.yaml", g.origin.Scope(), ts)
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
//   - []*Node: the flat node list in tree-walk order; nil when no nodes are present.
func (g *Graph) Nodes() []*Node { return g.root.descendantNodes() }

// Origin returns the tool-stamped graph metadata as a shallow value copy.
//
// The struct's scalar fields (Scope, SourceRoot, TargetPlatform, Tool, TargetRoot) are copy-safe. Its map and slice
// fields (CommitHashes, DirtyLayers, Features, Layers, Packages, Projects, Segments, Settings) share underlying storage
// with the original — mutations to those reference-typed children would reach back. Callers must treat the returned
// value as read-only.
//
// Returns:
//   - `Origin`: the tool-stamped metadata.
func (g *Graph) Origin() Origin { return g.origin }

// ResourceCatalog returns the [ResourceCatalog] carried by the graph from planning into execution.
//
// Returns:
//   - *ResourceCatalog: the catalog pointer; callers must not mutate the catalog after graph construction.
func (g *Graph) ResourceCatalog() *ResourceCatalog { return g.resourceCatalog }

// Root returns the graph's root subgraph.
//
// Returns:
//   - *Subgraph: the root subgraph pointer; callers must not mutate the subgraph after graph construction.
func (g *Graph) Root() *Subgraph { return g.root }

// SerialVersion returns the graph format version stamped at construction.
//
// Returns:
//   - `uint32`: the value of [GraphSchemaVersion] at the time the graph was constructed.
func (g *Graph) SerialVersion() uint32 { return g.schemaVersion }

// Signature returns the cryptographic signature or nil when the graph is unsigned.
//
// Returns:
//   - *sops.Signature: the signature pointer, or nil.
func (g *Graph) Signature() *sops.Signature { return g.signature }

// Subgraphs returns every [*Subgraph] descendant of the graph's root.
//
// The result does NOT include the root subgraph itself — it lists only authored / planner-emitted container units below
// it. Used by [Graph.UnitCount] and by harness assertions that want to count or inspect every executable unit produced
// by `plan.assemble`.
//
// Returns:
//   - []*Subgraph: the descendant subgraphs in tree-walk order.
func (g *Graph) Subgraphs() []*Subgraph { return g.root.descendantSubgraphs() }

// Timestamp returns when the graph was created.
//
// Returns:
//   - time.Time: the construction timestamp set at [NewGraph].
func (g *Graph) Timestamp() time.Time { return g.timestamp }

// UnitCount returns the total count of [ExecutableUnit] descendants of the graph's root.
//
// Both [*Node] and [*Subgraph] are children. The count excludes the root itself.
//
// This is the count the harness asserts against via `ctx.assert_equal(graph.unit_count(), n)`: a `plan.choose`
// container materializes as a Subgraph that holds its branch's children, so a script with `write_text` + `exists` +
// `choose(then=remove)` produces unit count 4 (3 Nodes + 1 Subgraph), not 3.
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
//   - []byte: the canonical YAML bytes.
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
		Origin        OriginBase  `yaml:"origin"`
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

// MarshalJSON projects the graph to its [graphData] wire shape and JSON-encodes it.
//
// Returns:
//   - []byte: the JSON encoding of the graph's wire form.
//   - `error`: non-nil if JSON marshaling fails.
func (g *Graph) MarshalJSON() ([]byte, error) { return json.Marshal(g.marshalData()) }

// MarshalYAML returns the graph's [graphData] wire shape for the YAML encoder to serialize.
//
// Returns:
//   - `any`: the [graphData] wire-form value.
//   - `error`: always nil; present only to satisfy the yaml.Marshaler signature.
func (g *Graph) MarshalYAML() (any, error) { return g.marshalData(), nil }

// Parameters returns the bubble-up variable surface of the graph.
//
// It is the deduplicated, type-checked set of [VariableValue] references walked across the root subgraph's children
// (plan-doc D3). It is consumed by the executor's preflight pass to drive [VariableResolver.Resolve].
//
// Returns:
//   - []Parameter: the bubble-up surface, stable-sorted by Name. Returned even when `error` is non-nil, so callers can
//     render a best-effort surface alongside the diagnostic.
//   - `error`: an [errors.Join] of any same-name-different-type collisions detected during the walk; nil when the walk
//     succeeded without violations.
func (g *Graph) Parameters() ([]Parameter, error) { return g.root.Parameters() }

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

// Serialize writes this graph through `encoder`, selecting JSON or YAML by the encoder's concrete type.
//
// Dispatches to [Graph.MarshalJSON] or [Graph.MarshalYAML]. The result is the symbol-table wire form: top-level
// `children` IDs from Root, plus the flat `subgraphs` and `nodes` lists sorted by ID.
//
// Whatever value is currently in [Graph.Checksum] is emitted as-is; this method does not (re)compute it. Callers that
// want a fresh checksum compute it from [Graph.CanonicalContent] and assign before calling.
//
// Usage:
//
//	encoder := yaml.NewEncoder(file)
//	encoder.SetIndent(2)
//	defer encoder.Close()
//	g.Serialize(encoder)
//
// Parameters:
//   - `encoder`: the destination encoder; both *json.Encoder and *yaml.Encoder satisfy [Encoder].
//
// Returns:
//   - `error`: the encoder's error, or nil on success.
func (g *Graph) Serialize(encoder Encoder) error {

	return encoder.Encode(g)
}

// SubgraphByID returns the descendant subgraph with the given ID, or nil if no descendant has that ID.
//
// Searches the tree recursively; the graph root is never returned.
//
// Parameters:
//   - `id`: the Subgraph ID to find.
//
// Returns:
//   - *Subgraph: the matching descendant, or nil.
func (g *Graph) SubgraphByID(id string) *Subgraph { return g.root.descendantSubgraphByID(id) }

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// marshalData projects this Graph to its canonical wire shape.
//
// Each Node is projected to a [nodeData] and each Subgraph to a [subgraphData] inline — the wire form is the data
// structs themselves, never the in-memory unit types. Unmarshaling does the reverse via [LoadGraph], which goes through
// the [RuntimeEnvironment]'s registry to bind actions as units are reconstructed; there is no [json.Unmarshaler] on
// Graph, Node, or Subgraph because the stdlib decoder has no registry in scope.
//
// Returns:
//   - `graphData`: the projected wire-form value.
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
		nodePayloads = append(nodePayloads, n.marshalData())
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

// region SUPPORTING TYPES

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
//
// Both *json.Encoder and *yaml.Encoder satisfy this interface.
type Encoder interface {
	Encode(v any) error
}

// GraphSpec is the fluent builder for a [*Graph]. A Graph is a document container, not an [ExecutableUnit], so the spec
// has no ID / action / annotations of its own; instead it carries the root subgraph's spec ([GraphSpec.Root]) plus
// graph-level metadata (origin, resource catalog, SOPS client). The root-shaped `With*` setters delegate to Root, and
// [NewGraph] hands `&spec.Root` to [NewSubgraph]. The root spec is seeded by [NewGraphSpec] (ID "root", binding
// "flow.subgraph" by name). Hand a populated spec to [NewGraph].
type GraphSpec struct {
	Root            SubgraphSpec
	Origin          Origin
	ResourceCatalog *ResourceCatalog
	SopsClient      *sops.Client
}

// WithElevationOffer sets the root subgraph's [ElevationOffer] and returns the spec for chaining.
//
// Parameters:
//   - `elevationOffer`: the [ElevationOffer], or nil to run unprivileged.
//
// Returns:
//   - `*GraphSpec`: the receiver, for chaining.
func (s *GraphSpec) WithElevationOffer(elevationOffer *ElevationOffer) *GraphSpec {
	s.Root.WithElevationOffer(elevationOffer)
	return s
}

// WithErrorAction sets the root subgraph's failure-handler and returns the spec for chaining.
//
// Parameters:
//   - `errorAction`: the handler [Subgraph], or nil for no error action.
//
// Returns:
//   - `*GraphSpec`: the receiver, for chaining.
func (s *GraphSpec) WithErrorAction(errorAction *Subgraph) *GraphSpec {
	s.Root.WithErrorAction(errorAction)
	return s
}

// WithOrigin sets the tool-stamp [Origin] and returns the spec for chaining.
//
// Parameters:
//   - `origin`: the graph's [Origin]; the zero value is permitted.
//
// Returns:
//   - `*GraphSpec`: the receiver, for chaining.
func (s *GraphSpec) WithOrigin(origin Origin) *GraphSpec {
	s.Origin = origin
	return s
}

// WithResourceCatalog sets the [*ResourceCatalog] the graph carries from planning into execution.
//
// Parameters:
//   - `catalog`: the [*ResourceCatalog]; nil defaults to a fresh empty catalog at construction.
//
// Returns:
//   - `*GraphSpec`: the receiver, for chaining.
func (s *GraphSpec) WithResourceCatalog(catalog *ResourceCatalog) *GraphSpec {
	s.ResourceCatalog = catalog
	return s
}

// WithRetryPolicy sets the root subgraph's [RetryPolicy] and returns the spec for chaining.
//
// Parameters:
//   - `retryPolicy`: the [RetryPolicy], or nil for no retry.
//
// Returns:
//   - `*GraphSpec`: the receiver, for chaining.
func (s *GraphSpec) WithRetryPolicy(retryPolicy *RetryPolicy) *GraphSpec {
	s.Root.WithRetryPolicy(retryPolicy)
	return s
}

// WithSlot binds one root-subgraph slot value by name and returns the spec for chaining.
//
// Parameters:
//   - `name`: the slot (frame-binding) name.
//   - `value`: the [SlotValue] to bind.
//
// Returns:
//   - `*GraphSpec`: the receiver, for chaining.
func (s *GraphSpec) WithSlot(name string, value SlotValue) *GraphSpec {
	s.Root.WithSlot(name, value)
	return s
}

// WithSopsClient sets the SOPS client used to sign the graph's canonical content.
//
// Parameters:
//   - `client`: the [*sops.Client]; nil leaves the graph unsigned.
//
// Returns:
//   - `*GraphSpec`: the receiver, for chaining.
func (s *GraphSpec) WithSopsClient(client *sops.Client) *GraphSpec {
	s.SopsClient = client
	return s
}

// WithUnits sets the top-level [ExecutableUnit] children of the graph's root subgraph.
//
// Parameters:
//   - `units`: the units, in planned order; replaces any prior set.
//
// Returns:
//   - `*GraphSpec`: the receiver, for chaining.
func (s *GraphSpec) WithUnits(units ...ExecutableUnit) *GraphSpec {
	s.Root.WithChildren(units...)
	return s
}

// graphMetadata carries the graph-level fields each preparer hands to [buildGraph].
//
// It decouples the single [Graph] build from how its non-structural fields are sourced. [NewGraph] fills it fresh
// (current schema version, now timestamp, a nil signature it sets afterward, the spec's origin and resource catalog);
// the load path fills it from the decoded document (the payload's schema version, timestamp, and preserved signature,
// the payload's origin, and a fresh resource catalog). The `kind` field is not carried — it is always [GraphKind] —
// and `checksum` / `unitsByID` are derived inside [buildGraph].
type graphMetadata struct {
	schemaVersion   uint32
	timestamp       time.Time
	signature       *sops.Signature
	origin          OriginBase
	resourceCatalog *ResourceCatalog
}

// graphData is the canonical wire shape for Graph.
//
// Used by both JSON and YAML marshalers; the tags apply to whichever encoder reads the struct. Top-level `children` and
// `edges` project up from `Graph.Root`, mirroring Root's own wire shape. `subgraphs` and `nodes` are flat symbol tables
// — every non-root Subgraph and every Node in the graph, sorted by ID.
type graphData struct {

	// Identity
	Kind          string     `json:"kind"                 yaml:"kind"`
	SchemaVersion uint32     `json:"schema_version"       yaml:"schema_version"`
	Timestamp     time.Time  `json:"timestamp"            yaml:"timestamp"`
	Origin        OriginBase `json:"origin"               yaml:"origin"`

	// Integrity
	Checksum  string          `json:"checksum,omitempty"   yaml:"checksum,omitempty"`
	Signature *sops.Signature `json:"signature,omitempty"  yaml:"signature,omitempty"`

	// Content
	Children  []string       `json:"children"             yaml:"children"`
	Edges     []Edge         `json:"edges,omitempty"      yaml:"edges,omitempty"`
	Nodes     []nodeData     `json:"nodes,omitempty"      yaml:"nodes,omitempty"`
	Subgraphs []subgraphData `json:"subgraphs,omitempty"  yaml:"subgraphs,omitempty"`
}

// endregion
