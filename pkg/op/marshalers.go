// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

// Custom marshalers for Graph, Node, and Subgraph.
//
// The wire form is a symbol-table model. Graph's top-level `nodes` and `subgraphs` lists hold every executable unit in
// the graph, each sorted by ID for stable output. Each Subgraph's `children` field holds only the IDs of its direct
// children, in topological order. Edges within each Subgraph reference sibling IDs. The top-level `children` / `edges`
// keys project up from `Graph.Root`. References are validated at unmarshal time — dangling child IDs or edge endpoints
// surface as errors via [Graph.applyPayload].

// region Graph marshalers

// graphPayload is the canonical wire shape for Graph.
//
// Used by both JSON and YAML marshalers; the tags apply to whichever encoder reads the struct. Top-level `children` and
// `edges` project up from `Graph.Root`, mirroring Root's own wire shape. `subgraphs` and `nodes` are flat symbol tables
// — every non-root Subgraph and every Node in the graph, sorted by ID.
type graphPayload struct {
	Version    string            `json:"version"              yaml:"version"`
	State      GraphState        `json:"state"                yaml:"state"`
	Timestamp  time.Time         `json:"timestamp"            yaml:"timestamp"`
	Children   []string          `json:"children"             yaml:"children"`
	Edges      []Edge            `json:"edges,omitempty"      yaml:"edges,omitempty"`
	Subgraphs  []subgraphPayload `json:"subgraphs,omitempty"  yaml:"subgraphs,omitempty"`
	Nodes      []nodePayload     `json:"nodes,omitempty"      yaml:"nodes,omitempty"`
	Checksum   string            `json:"checksum,omitempty"   yaml:"checksum,omitempty"`
	Collisions []Collision       `json:"collisions,omitempty" yaml:"collisions,omitempty"`
	Provenance Provenance        `json:"provenance"           yaml:"provenance"`
	Rollback   []RollbackEntry   `json:"rollback,omitempty"   yaml:"rollback,omitempty"`
	Signature  *sops.Signature   `json:"signature,omitempty"  yaml:"signature,omitempty"`
}

func (g *Graph) MarshalJSON() ([]byte, error) { return json.Marshal(g.marshalPayload()) }

func (g *Graph) MarshalYAML() (any, error) { return g.marshalPayload(), nil }

// marshalPayload projects this Graph to its canonical wire shape.
//
// Each Node is projected to a [nodePayload] and each Subgraph to a [subgraphPayload] inline — the
// wire form is the payload structs themselves, never the in-memory unit types. Unmarshaling does the
// reverse via [LoadGraph], which goes through the [RuntimeEnvironment]'s registry to bind actions as
// units are reconstructed; there is no [json.Unmarshaler] on Graph, Node, or Subgraph because the
// stdlib decoder has no registry in scope.
//
// Returns:
//   - graphPayload: the projected payload.
func (g *Graph) marshalPayload() graphPayload {

	var rootEdges []Edge
	if g.Root != nil {
		rootEdges = g.Root.edges
	}

	descendants := g.Root.descendantSubgraphs()
	sort.Slice(descendants, func(i, j int) bool { return descendants[i].ID() < descendants[j].ID() })

	subgraphPayloads := make([]subgraphPayload, 0, len(descendants))
	for _, sg := range descendants {
		subgraphPayloads = append(subgraphPayloads, sg.marshalPayload())
	}

	descendantNodes := g.Root.descendantNodes()
	sort.Slice(descendantNodes, func(i, j int) bool { return descendantNodes[i].ID() < descendantNodes[j].ID() })

	nodePayloads := make([]nodePayload, 0, len(descendantNodes))
	for _, n := range descendantNodes {
		nodePayloads = append(nodePayloads, n.marshalPayload())
	}

	return graphPayload{
		Version:    g.Version,
		State:      g.State,
		Timestamp:  g.Timestamp,
		Children:   g.Root.childIDs(),
		Edges:      rootEdges,
		Subgraphs:  subgraphPayloads,
		Nodes:      nodePayloads,
		Checksum:   g.Checksum,
		Collisions: g.Collisions,
		Provenance: g.Provenance,
		Rollback:   g.Rollback,
		Signature:  g.Signature,
	}
}

// endregion

// region Node marshalers

// nodePayload is the canonical wire shape for Node.
//
// ActionName is the sole identity field — sourced from `unit.Action().Name()` at marshal; consumed by
// the post-load Action-rebind link pass via `env.ActionByName(name)` at Rebind. Status / Error /
// Timestamp do not round-trip — they live on the recovery-stack receipts at execution time.
// Slots serialize as an object/dict keyed by parameter name; values are the sealed [SlotValue]
// variants ([ImmediateValue], [PromiseValue], [VariableValue]).
type nodePayload struct {
	ID          string               `json:"id"                     yaml:"id"`
	ActionName  string               `json:"action_name,omitempty"  yaml:"action_name,omitempty"`
	Annotations map[string]string    `json:"annotations,omitempty"  yaml:"annotations,omitempty"`
	Layer       string               `json:"layer,omitempty"        yaml:"layer,omitempty"`
	Origin      string               `json:"origin,omitempty"       yaml:"origin,omitempty"`
	Retry       *RetryPolicy         `json:"retry,omitempty"        yaml:"retry,omitempty"`
	Slots       map[string]SlotValue `json:"slots,omitempty"        yaml:"slots,omitempty"`
}

func (n *Node) marshalPayload() nodePayload {
	var actionName string
	if a := n.Action(); a != nil {
		actionName = a.Name()
	}
	return nodePayload{
		ID:          n.id,
		ActionName:  actionName,
		Annotations: n.annotations,
		Layer:       n.Layer,
		Origin:      n.Origin,
		Retry:       n.RetryPolicy(),
		Slots:       n.slots,
	}
}

func (n *Node) MarshalJSON() ([]byte, error) { return json.Marshal(n.marshalPayload()) }

func (n *Node) MarshalYAML() (any, error) { return n.marshalPayload(), nil }

// Node intentionally has no [json.Unmarshaler] / yaml.Unmarshaler. The wire form decodes into
// [nodePayload] structs (held inside [graphPayload]); [LoadGraph] then walks those payloads and
// constructs each Node via [NewNode] with the registry-resolved [Action] in one pass — so a
// Node never exists in an action-less transient state outside [LoadGraph]'s internals.

// endregion

// region Subgraph marshalers

// subgraphPayload is the canonical wire shape for Subgraph.
//
// `Children` holds direct-child IDs in topological order; the actual units are looked up in the surrounding Graph's
// unit table via [Subgraph.linkChildren] during unmarshal. Used by both JSON and YAML marshalers.
type subgraphPayload struct {
	ID         string       `json:"id"                    yaml:"id"`
	Name       string       `json:"name"                  yaml:"name"`
	ActionName string       `json:"action_name,omitempty" yaml:"action_name,omitempty"`
	Children   []string     `json:"children"              yaml:"children"`
	Edges      []Edge       `json:"edges,omitempty"       yaml:"edges,omitempty"`
	Retry      *RetryPolicy `json:"retry,omitempty"       yaml:"retry,omitempty"`
}

func (s *Subgraph) MarshalJSON() ([]byte, error) { return json.Marshal(s.marshalPayload()) }

func (s *Subgraph) MarshalYAML() (any, error) { return s.marshalPayload(), nil }

// Subgraph intentionally has no [json.Unmarshaler] / yaml.Unmarshaler. See the analogous comment on
// [Node] — [LoadGraph] is the registry-aware path that decodes payloads and constructs Subgraphs via
// [NewSubgraph] with bound actions; the stdlib decoder is not allowed to produce action-less
// Subgraphs that would have to be linked up by a later pass.

// marshalPayload projects this Subgraph to its canonical wire shape.
//
// Returns:
//   - subgraphPayload: the projected payload.
func (s *Subgraph) marshalPayload() subgraphPayload {

	var actionName string
	if a := s.Action(); a != nil {
		actionName = a.Name()
	}

	return subgraphPayload{
		ID:         s.id,
		Name:       s.Name,
		ActionName: actionName,
		Children:   s.childIDs(),
		Edges:      s.edges,
		Retry:      s.RetryPolicy(),
	}
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
