// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadGraph decodes a wire-form graph (JSON or YAML) into an in-memory [*Graph] with every unit
// already bound to its [Action] through the supplied environment's registry.
//
// The decode path is registry-aware end-to-end: payload bytes are first decoded into the wire-form
// payload structs ([graphPayload], [nodePayload], [subgraphPayload]); LoadGraph then walks those
// payloads, resolves each unit's action by short name through `env.Registry`, and constructs each
// [*Node] / [*Subgraph] via [NewNode] / [NewSubgraph] with the resolved action — so no unit ever
// exists in a transient action-less state outside this function's internals.
//
// After unit construction LoadGraph rebuilds containment (child IDs → child pointers, topological
// order per subgraph edges) and validates edge endpoints. The returned graph holds no reference to the
// supplied env; pass it to [NewGraphExecutor] to execute.
//
// Parameters:
//   - `env`: the runtime environment whose registry resolves action names. Must be non-nil; the
//     registry must contain every action referenced in the wire form.
//   - `data`: the encoded bytes.
//   - `format`: "json" or "yaml" (or "yml") — case-insensitive.
//
// Returns:
//   - *Graph: the constructed graph with every unit's action bound.
//   - `error`: non-nil if decoding fails, the format is unsupported, any action name is unknown to
//     the registry, any child ID is dangling, or any edge endpoint fails to resolve.
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

	return buildGraphFromPayload(env, &p)
}

// region UNEXPORTED FUNCTIONS

// region Behaviors

// buildGraphFromPayload constructs a [*Graph] from a decoded [graphPayload]. The dual to
// [Graph.marshalPayload]. Resolves each unit's action through env.Registry and constructs the
// concrete [*Node] / [*Subgraph] values via [NewNode] / [NewSubgraph]; rebuilds the unit table and
// per-subgraph containment; validates edges.
//
// Parameters:
//   - `env`: the runtime environment whose registry resolves action names.
//   - `p`: the decoded payload.
//
// Returns:
//   - *Graph: the constructed graph.
//   - `error`: non-nil on unresolved action name, dangling child ID, or invalid edge endpoint.
func buildGraphFromPayload(env *RuntimeEnvironment, p *graphData) (*Graph, error) {

	g := &Graph{
		kind:            p.Kind,
		schemaVersion:   p.SchemaVersion,
		checksum:        p.Checksum,
		origin:          p.Origin,
		resourceCatalog: NewResourceCatalog(),
		root:            newRootSubgraph(nil, nil, nil, nil),
		signature:       p.Signature,
		timestamp:       p.Timestamp,
	}
	g.root.edges = p.Edges

	var violations []error

	// Build the unit symbol table from the flat payload lists. Each unit comes into existence with
	// its action already bound — NewNode / NewSubgraph's assert.NonZero invariant holds.
	g.unitsByID = make(map[string]ExecutableUnit, len(p.Nodes)+len(p.Subgraphs))

	for i := range p.Nodes {
		node, err := buildNodeFromPayload(env, &p.Nodes[i])
		if err != nil {
			violations = append(violations, err)
			continue
		}
		g.unitsByID[node.ID()] = node
	}

	for i := range p.Subgraphs {
		sg, err := buildSubgraphFromPayload(env, &p.Subgraphs[i])
		if err != nil {
			violations = append(violations, err)
			continue
		}
		g.unitsByID[sg.ID()] = sg
	}

	if len(violations) > 0 {
		return nil, errors.Join(violations...)
	}

	// Wire root's children + the per-subgraph child links. Each Subgraph's executableUnitsByID was
	// pre-populated with placeholder nil entries by buildSubgraphFromPayload from its Children list;
	// linkChildren resolves each placeholder against the now-complete unit table and populates
	// executableUnits in topological order per edges.
	if len(p.Children) > 0 {
		g.root.executableUnitsByID = make(map[string]ExecutableUnit, len(p.Children))
		for _, id := range p.Children {
			g.root.executableUnitsByID[id] = nil
		}
	}
	if err := g.root.linkChildren(g.unitsByID); err != nil {
		violations = append(violations, err)
	}
	for _, sg := range g.Subgraphs() {
		if err := sg.linkChildren(g.unitsByID); err != nil {
			violations = append(violations, err)
		}
	}

	violations = append(violations, g.root.validateEdges())
	for _, sg := range g.Subgraphs() {
		violations = append(violations, sg.validateEdges())
	}

	if err := errors.Join(violations...); err != nil {
		return nil, err
	}

	return g, nil
}

// buildNodeFromPayload constructs a [*Node] from a [nodePayload], resolving its action through the
// environment's registry.
//
// Parameters:
//   - `env`: the runtime environment whose registry resolves action names.
//   - `p`: the decoded node payload.
//
// Returns:
//   - *Node: the constructed node, with action bound.
//   - `error`: non-nil if the action name cannot be resolved.
func buildNodeFromPayload(env *RuntimeEnvironment, p *nodeData) (*Node, error) {

	action, err := resolvePayloadAction(env, p.ActionName, "node", p.ID)
	if err != nil {
		return nil, err
	}

	node := NewNode(p.ID, action, p.Annotations)
	node.setRetryPolicy(p.Retry)
	node.slots = p.Slots
	return node, nil
}

// buildSubgraphFromPayload constructs a [*Subgraph] from a [subgraphPayload], resolving its action
// through the environment's registry. The Subgraph's executableUnitsByID is pre-populated with
// placeholder nil entries keyed by child ID; [Subgraph.linkChildren] resolves them in the caller
// once the full unit table is built.
//
// Parameters:
//   - `env`: the runtime environment whose registry resolves action names.
//   - `p`: the decoded subgraph payload.
//
// Returns:
//   - *Subgraph: the constructed subgraph, with action bound and placeholder children.
//   - `error`: non-nil if the action name cannot be resolved.
func buildSubgraphFromPayload(env *RuntimeEnvironment, p *subgraphData) (*Subgraph, error) {

	action, err := resolvePayloadAction(env, p.ActionName, "subgraph", p.ID)
	if err != nil {
		return nil, err
	}

	// Load path builds the Subgraph empty and lets linkChildren resolve children later via the unit
	// symbol table. The sealed [NewSubgraph] all-args constructor would materialize edges from
	// children at construction, which we can't do here — children aren't instantiated yet. So we
	// invoke the constructor with empty children and then patch wire-supplied edges / retry /
	// placeholder child IDs through package-internal mutators.
	sg, err := NewSubgraph(p.ID, action, nil, nil, nil, nil, p.Retry)
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

// resolvePayloadAction resolves an action name from the wire payload through env's registry and
// returns the bound [Action], or an error if the name is empty or unknown.
//
// Parameters:
//   - `env`: the runtime environment whose registry resolves action names.
//   - `name`: the short dotted action name from the payload.
//   - `kind`: a label used in error messages — "node" or "subgraph".
//   - `id`: the unit's ID, included in error messages for context.
//
// Returns:
//   - `Action`: the resolved action.
//   - `error`: non-nil if `name` is empty or the registry does not recognize it.
func resolvePayloadAction(env *RuntimeEnvironment, name, kind, id string) (Action, error) {

	if name == "" {
		return nil, fmt.Errorf("op.LoadGraph: %s %q has no action_name in wire form", kind, id)
	}
	action, err := env.ReceiverRegistry.BuildAction(name)
	if err != nil {
		return nil, fmt.Errorf("op.LoadGraph: %s %q: action %q: %w", kind, id, name, err)
	}
	return action, nil
}

// linkChildren resolves the placeholder entries in [Subgraph.executableUnitsByID] against the unit
// table built by [buildGraphFromPayload] and populates [Subgraph.executableUnits] in topological
// order per [Subgraph.Edges].
//
// Map iteration order is unstable, so the final slice order is established by Kahn's topological sort
// over the local edge set. Ties between roots are broken by ID for determinism.
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
		child.stampParent(s.ID())
	}

	s.executableUnits = topologicallySorted(resolved, s.edges)
	return nil
}

// endregion

// endregion
