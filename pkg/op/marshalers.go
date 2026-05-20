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
	g.Root.Edges = p.Edges
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
		rootEdges = g.Root.Edges
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

func (n *Node) MarshalJSON() ([]byte, error) {

	return json.Marshal(struct {
		ID          string            `json:"id"`
		Receiver    string            `json:"receiver"`
		Status      Status        `json:"status"`
		Annotations map[string]string `json:"annotations,omitempty"`
		Error       string            `json:"error,omitempty"`
		Layer       string            `json:"layer,omitempty"`
		Origin      string            `json:"origin,omitempty"`
		Retry       *RetryPolicy      `json:"retry,omitempty"`
		Slots       []*Slot           `json:"slots,omitempty"`
		Timestamp   string            `json:"timestamp,omitempty"`
	}{
		ID:          n.id,
		Receiver:    n.Receiver,
		Status:      n.Status,
		Annotations: n.Annotations,
		Error:       n.Error,
		Layer:       n.Layer,
		Origin:      n.Origin,
		Retry:       n.RetryPolicy(),
		Slots:       n.Slots,
		Timestamp:   n.Timestamp,
	})
}

func (n *Node) UnmarshalJSON(data []byte) error {

	var payload struct {
		ID          string            `json:"id"`
		Receiver    string            `json:"receiver"`
		Status      Status        `json:"status"`
		Annotations map[string]string `json:"annotations,omitempty"`
		Error       string            `json:"error,omitempty"`
		Layer       string            `json:"layer,omitempty"`
		Origin      string            `json:"origin,omitempty"`
		Retry       *RetryPolicy      `json:"retry,omitempty"`
		Slots       []*Slot           `json:"slots,omitempty"`
		Timestamp   string            `json:"timestamp,omitempty"`
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	n.executableUnit = executableUnit{id: payload.ID}
	n.Receiver = payload.Receiver
	n.Status = payload.Status
	n.Annotations = payload.Annotations
	n.Error = payload.Error
	n.Layer = payload.Layer
	n.Origin = payload.Origin
	n.SetRetryPolicy(payload.Retry)
	n.Slots = payload.Slots
	n.Timestamp = payload.Timestamp

	return nil
}

func (n *Node) MarshalYAML() (any, error) {

	return struct {
		ID          string            `yaml:"id"`
		Receiver    string            `yaml:"receiver"`
		Status      Status        `yaml:"status"`
		Annotations map[string]string `yaml:"annotations,omitempty"`
		Error       string            `yaml:"error,omitempty"`
		Layer       string            `yaml:"layer,omitempty"`
		Origin      string            `yaml:"origin,omitempty"`
		Retry       *RetryPolicy      `yaml:"retry,omitempty"`
		Slots       []*Slot           `yaml:"slots,omitempty"`
		Timestamp   string            `yaml:"timestamp,omitempty"`
	}{
		ID:          n.id,
		Receiver:    n.Receiver,
		Status:      n.Status,
		Annotations: n.Annotations,
		Error:       n.Error,
		Layer:       n.Layer,
		Origin:      n.Origin,
		Retry:       n.RetryPolicy(),
		Slots:       n.Slots,
		Timestamp:   n.Timestamp,
	}, nil
}

func (n *Node) UnmarshalYAML(unmarshal func(any) error) error {

	var payload struct {
		ID          string            `yaml:"id"`
		Receiver    string            `yaml:"receiver"`
		Status      Status        `yaml:"status"`
		Annotations map[string]string `yaml:"annotations,omitempty"`
		Error       string            `yaml:"error,omitempty"`
		Layer       string            `yaml:"layer,omitempty"`
		Origin      string            `yaml:"origin,omitempty"`
		Retry       *RetryPolicy      `yaml:"retry,omitempty"`
		Slots       []*Slot           `yaml:"slots,omitempty"`
		Timestamp   string            `yaml:"timestamp,omitempty"`
	}

	if err := unmarshal(&payload); err != nil {
		return err
	}

	n.executableUnit = executableUnit{id: payload.ID}
	n.Receiver = payload.Receiver
	n.Status = payload.Status
	n.Annotations = payload.Annotations
	n.Error = payload.Error
	n.Layer = payload.Layer
	n.Origin = payload.Origin
	n.SetRetryPolicy(payload.Retry)
	n.Slots = payload.Slots
	n.Timestamp = payload.Timestamp

	return nil
}

// endregion

// region Subgraph marshalers

// subgraphPayload is the canonical wire shape for Subgraph.
//
// `Children` holds direct-child IDs in topological order; the actual units are looked up in the surrounding Graph's
// unit table via [Subgraph.linkChildren] during unmarshal. Used by both JSON and YAML marshalers.
type subgraphPayload struct {
	ID         string         `json:"id"                   yaml:"id"`
	Name       string         `json:"name"                 yaml:"name"`
	Status     Status `json:"status"               yaml:"status"`
	Children   []string       `json:"children"             yaml:"children"`
	Edges      []Edge         `json:"edges,omitempty"      yaml:"edges,omitempty"`
	Retry      *RetryPolicy   `json:"retry,omitempty"      yaml:"retry,omitempty"`
	Compensate string         `json:"compensate,omitempty" yaml:"compensate,omitempty"`
	Attempts   []Attempt      `json:"attempts,omitempty"   yaml:"attempts,omitempty"`
	State      map[string]any `json:"state,omitempty"      yaml:"state,omitempty"`
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
	s.Status = p.Status
	s.Edges = p.Edges
	s.SetRetryPolicy(p.Retry)
	s.Compensate = p.Compensate
	s.Attempts = p.Attempts
	s.State = p.State

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

	return subgraphPayload{
		ID:         s.id,
		Name:       s.Name,
		Status:     s.Status,
		Children:   s.childIDs(),
		Edges:      s.Edges,
		Retry:      s.RetryPolicy(),
		Compensate: s.Compensate,
		Attempts:   s.Attempts,
		State:      s.State,
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

	for _, e := range s.Edges {
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
