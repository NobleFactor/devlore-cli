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
	Version    string          `json:"version"              yaml:"version"`
	State      GraphState      `json:"state"                yaml:"state"`
	Timestamp  time.Time       `json:"timestamp"            yaml:"timestamp"`
	Children   []string        `json:"children"             yaml:"children"`
	Edges      []Edge          `json:"edges,omitempty"      yaml:"edges,omitempty"`
	Subgraphs  []*Subgraph     `json:"subgraphs,omitempty"  yaml:"subgraphs,omitempty"`
	Nodes      []*Node         `json:"nodes,omitempty"      yaml:"nodes,omitempty"`
	Checksum   string          `json:"checksum,omitempty"   yaml:"checksum,omitempty"`
	Collisions []Collision     `json:"collisions,omitempty" yaml:"collisions,omitempty"`
	Provenance Provenance      `json:"provenance"           yaml:"provenance"`
	Rollback   []RollbackEntry `json:"rollback,omitempty"   yaml:"rollback,omitempty"`
	Signature  *sops.Signature `json:"signature,omitempty"  yaml:"signature,omitempty"`
}

func (g *Graph) MarshalJSON() ([]byte, error) { return json.Marshal(g.marshalPayload()) }

func (g *Graph) MarshalYAML() (any, error) { return g.marshalPayload(), nil }

func (g *Graph) UnmarshalJSON(data []byte) error {

	var p graphPayload

	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}

	return g.applyPayload(&p)
}

func (g *Graph) UnmarshalYAML(unmarshal func(any) error) error {

	var p graphPayload

	if err := unmarshal(&p); err != nil {
		return err
	}

	return g.applyPayload(&p)
}

// applyPayload populates this Graph from a parsed [graphPayload].
//
// Builds the unit symbol table from the flat `Nodes` and `Subgraphs` lists, then resolves each Subgraph's stashed child
// IDs via [Subgraph.linkChildren] and validates every edge endpoint via [Subgraph.validateEdges].
//
// Parameters:
//   - `p`: the parsed payload.
//
// Returns:
//   - `error`: non-nil if any child ID or edge endpoint fails to resolve in the unit table.
func (g *Graph) applyPayload(p *graphPayload) error {

	g.Version = p.Version
	g.State = p.State
	g.Timestamp = p.Timestamp
	g.Checksum = p.Checksum
	g.Collisions = p.Collisions
	g.Provenance = p.Provenance
	g.Rollback = p.Rollback
	g.Signature = p.Signature

	g.Root = NewSubgraph("root")
	g.Root.edges = p.Edges
	g.Root.pendingChildren = p.Children

	g.unitsByID = make(map[string]ExecutableUnit, len(p.Nodes)+len(p.Subgraphs))

	for _, n := range p.Nodes {
		g.unitsByID[n.ID()] = n
	}

	for _, sg := range p.Subgraphs {
		g.unitsByID[sg.ID()] = sg
	}

	if err := g.Root.linkChildren(g.unitsByID); err != nil {
		return err
	}

	for _, sg := range p.Subgraphs {
		if err := sg.linkChildren(g.unitsByID); err != nil {
			return err
		}
	}

	errs := []error{g.Root.validateEdges()}

	for _, sg := range p.Subgraphs {
		errs = append(errs, sg.validateEdges())
	}

	return errors.Join(errs...)
}

// marshalPayload projects this Graph to its canonical wire shape.
//
// Returns:
//   - graphPayload: the projected payload.
func (g *Graph) marshalPayload() graphPayload {

	var rootEdges []Edge
	if g.Root != nil {
		rootEdges = g.Root.edges
	}

	subgraphs := g.Root.descendantSubgraphs()
	sort.Slice(subgraphs, func(i, j int) bool { return subgraphs[i].ID() < subgraphs[j].ID() })

	nodes := g.Root.descendantNodes()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID() < nodes[j].ID() })

	return graphPayload{
		Version:    g.Version,
		State:      g.State,
		Timestamp:  g.Timestamp,
		Children:   g.Root.childIDs(),
		Edges:      rootEdges,
		Subgraphs:  subgraphs,
		Nodes:      nodes,
		Checksum:   g.Checksum,
		Collisions: g.Collisions,
		Provenance: g.Provenance,
		Rollback:   g.Rollback,
		Signature:  g.Signature,
	}
}

// endregion

// region Node marshalers

// nodePayload is the canonical wire shape for Node post-step-14.
//
// Status / Error / Timestamp dropped in step 13 (now audit-trail state on the recovery stack's
// receipts). Receiver dropped in step 14 (every writer binds the Action via SetAction at construction).
// ActionName is the sole identity field — sourced from `unit.Action().Name()` at marshal; consumed by
// the post-load Action-rebind link pass via `env.ActionByName(name)` at Rebind.
//
// The Receiver field on the payload remains ON UNMARSHAL only — for backward compatibility with
// graphs serialized before step 13 added action_name. Pre-step-13 graphs have receiver but no
// action_name; applyPayload stashes whichever is present in pendingAction.
type nodePayload struct {
	ID          string            `json:"id"                     yaml:"id"`
	ActionName  string            `json:"action_name,omitempty"  yaml:"action_name,omitempty"`
	Receiver    string            `json:"receiver,omitempty"     yaml:"receiver,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"  yaml:"annotations,omitempty"`
	Layer       string            `json:"layer,omitempty"        yaml:"layer,omitempty"`
	Origin      string            `json:"origin,omitempty"       yaml:"origin,omitempty"`
	Retry       *RetryPolicy      `json:"retry,omitempty"        yaml:"retry,omitempty"`
	Slots       []*Slot           `json:"slots,omitempty"        yaml:"slots,omitempty"`
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
		Slots:       n.Slots,
	}
}

func (n *Node) applyPayload(p *nodePayload) {
	n.executableUnit = executableUnit{id: p.ID}
	n.annotations = p.Annotations
	n.Layer = p.Layer
	n.Origin = p.Origin
	n.SetRetryPolicy(p.Retry)
	n.Slots = p.Slots

	// Stash the action name for the post-load Action-rebind link pass. Falls back to the legacy
	// receiver field when the wire payload predates the action_name field (pre-step-13 graphs).
	if p.ActionName != "" {
		n.SetPendingAction(p.ActionName)
	} else if p.Receiver != "" {
		n.SetPendingAction(p.Receiver)
	}
}

func (n *Node) MarshalJSON() ([]byte, error) { return json.Marshal(n.marshalPayload()) }

func (n *Node) MarshalYAML() (any, error) { return n.marshalPayload(), nil }

func (n *Node) UnmarshalJSON(data []byte) error {
	var p nodePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	n.applyPayload(&p)
	return nil
}

func (n *Node) UnmarshalYAML(unmarshal func(any) error) error {
	var p nodePayload
	if err := unmarshal(&p); err != nil {
		return err
	}
	n.applyPayload(&p)
	return nil
}

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

func (s *Subgraph) UnmarshalJSON(data []byte) error {

	var p subgraphPayload

	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}

	s.applyPayload(&p)
	return nil
}

func (s *Subgraph) UnmarshalYAML(unmarshal func(any) error) error {

	var p subgraphPayload

	if err := unmarshal(&p); err != nil {
		return err
	}

	s.applyPayload(&p)
	return nil
}

// applyPayload populates this Subgraph from a parsed [subgraphPayload].
//
// Child IDs are stashed in `pendingChildren` for later resolution by [Subgraph.linkChildren] (called from
// [Graph.applyPayload] once the Graph's unit table is populated). Resets `children` and `childrenByID` to nil so a
// re-unmarshal onto a populated Subgraph produces a coherent state rather than accumulating entries.
//
// Parameters:
//   - `p`: the parsed payload.
func (s *Subgraph) applyPayload(p *subgraphPayload) {

	s.executableUnit = executableUnit{id: p.ID}
	s.Name = p.Name
	s.edges = p.Edges
	s.SetRetryPolicy(p.Retry)

	if p.ActionName != "" {
		s.SetPendingAction(p.ActionName)
	}

	s.executableUnits = nil
	s.executableUnitsByID = nil
	s.pendingChildren = p.Children
}

// linkChildren resolves the child IDs stashed in `pendingChildren` against the Graph's unit table, wiring each through
// [Subgraph.AddChild]. `pendingChildren` is cleared on success.
//
// Parameters:
//   - `unitsByID`: the Graph's unit symbol table, keyed by [ExecutableUnit.ID].
//
// Returns:
//   - `error`: non-nil if any pending child ID is missing from `unitsByID`.
func (s *Subgraph) linkChildren(unitsByID map[string]ExecutableUnit) error {

	for _, id := range s.pendingChildren {
		child, ok := unitsByID[id]
		if !ok {
			return fmt.Errorf("subgraph %q: child %q not in unit table", s.ID(), id)
		}
		s.AddChild(child)
	}

	s.pendingChildren = nil
	return nil
}

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
