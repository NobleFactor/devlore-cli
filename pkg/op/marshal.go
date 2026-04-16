// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

// Custom marshalers for Graph, Node, and Subgraph.
//
// Graph has a Root *Subgraph; its Children / Edges live on Root but appear at
// the graph's top level in the wire format. The Graph marshalers project
// Root.Children / Root.Edges to top-level "children" / "edges" keys.
//
// Node and Subgraph have an unexported `id` (via embedded executableUnit);
// their marshalers project it to "id". The `parameters` field is not
// serialized — a plan-time computed surface rebuilt on load via Bind for
// Node and FinalizeParameters for Subgraph. See Fork E in
// docs/plans/extract-starlark-from-op/phase-7.md.

// region Graph marshalers

// graphPayload is the canonical wire shape for Graph. Used by both JSON and
// YAML marshalers; the tags apply to whichever encoder reads the struct.
type graphPayload struct {
	Children   []SubgraphChild `json:"children" yaml:"children"`
	Edges      []Edge          `json:"edges,omitempty" yaml:"edges,omitempty"`
	Checksum   string          `json:"checksum,omitempty" yaml:"checksum,omitempty"`
	Collisions []Collision     `json:"collisions,omitempty" yaml:"collisions,omitempty"`
	Provenance Provenance      `json:"provenance" yaml:"provenance"`
	Rollback   []RollbackEntry `json:"rollback,omitempty" yaml:"rollback,omitempty"`
	Signature  *sops.Signature `json:"signature,omitempty" yaml:"signature,omitempty"`
	State      GraphState      `json:"state" yaml:"state"`
	Timestamp  time.Time       `json:"timestamp" yaml:"timestamp"`
	Version    string          `json:"version" yaml:"version"`
}

func (g *Graph) MarshalJSON() ([]byte, error) {

	var children []SubgraphChild
	var edges []Edge
	if g.Root != nil {
		children = g.Root.Children
		edges = g.Root.Edges
	}
	return json.Marshal(graphPayload{
		Children:   children,
		Edges:      edges,
		Checksum:   g.Checksum,
		Collisions: g.Collisions,
		Provenance: g.Provenance,
		Rollback:   g.Rollback,
		Signature:  g.Signature,
		State:      g.State,
		Timestamp:  g.Timestamp,
		Version:    g.Version,
	})
}

func (g *Graph) UnmarshalJSON(data []byte) error {

	var p graphPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	g.applyPayload(&p)
	return nil
}

func (g *Graph) MarshalYAML() (any, error) {

	var children []SubgraphChild
	var edges []Edge
	if g.Root != nil {
		children = g.Root.Children
		edges = g.Root.Edges
	}
	return graphPayload{
		Children:   children,
		Edges:      edges,
		Checksum:   g.Checksum,
		Collisions: g.Collisions,
		Provenance: g.Provenance,
		Rollback:   g.Rollback,
		Signature:  g.Signature,
		State:      g.State,
		Timestamp:  g.Timestamp,
		Version:    g.Version,
	}, nil
}

func (g *Graph) UnmarshalYAML(unmarshal func(any) error) error {

	var p graphPayload
	if err := unmarshal(&p); err != nil {
		return err
	}
	g.applyPayload(&p)
	return nil
}

func (g *Graph) applyPayload(p *graphPayload) {

	if g.Root == nil {
		g.Root = NewSubgraph("root")
	}
	g.Root.Children = p.Children
	g.Root.Edges = p.Edges
	g.Checksum = p.Checksum
	g.Collisions = p.Collisions
	g.Provenance = p.Provenance
	g.Rollback = p.Rollback
	g.Signature = p.Signature
	g.State = p.State
	g.Timestamp = p.Timestamp
	g.Version = p.Version
}

// endregion

// region Node marshalers

func (n *Node) MarshalJSON() ([]byte, error) {

	return json.Marshal(struct {
		ID          string            `json:"id"`
		Receiver    string            `json:"receiver"`
		Status      NodeStatus        `json:"status"`
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
		Retry:       n.Retry,
		Slots:       n.Slots,
		Timestamp:   n.Timestamp,
	})
}

func (n *Node) UnmarshalJSON(data []byte) error {

	var payload struct {
		ID          string            `json:"id"`
		Receiver    string            `json:"receiver"`
		Status      NodeStatus        `json:"status"`
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
	n.Retry = payload.Retry
	n.Slots = payload.Slots
	n.Timestamp = payload.Timestamp
	return nil
}

func (n *Node) MarshalYAML() (any, error) {

	return struct {
		ID          string            `yaml:"id"`
		Receiver    string            `yaml:"receiver"`
		Status      NodeStatus        `yaml:"status"`
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
		Retry:       n.Retry,
		Slots:       n.Slots,
		Timestamp:   n.Timestamp,
	}, nil
}

func (n *Node) UnmarshalYAML(unmarshal func(any) error) error {

	var payload struct {
		ID          string            `yaml:"id"`
		Receiver    string            `yaml:"receiver"`
		Status      NodeStatus        `yaml:"status"`
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
	n.Retry = payload.Retry
	n.Slots = payload.Slots
	n.Timestamp = payload.Timestamp
	return nil
}

// endregion

// region Subgraph marshalers

func (s *Subgraph) MarshalJSON() ([]byte, error) {

	return json.Marshal(struct {
		ID         string           `json:"id"`
		Name       string           `json:"name"`
		Status     SubgraphStatus   `json:"status"`
		Children   []SubgraphChild  `json:"children"`
		Edges      []Edge           `json:"edges,omitempty"`
		Retry      *RetryPolicy     `json:"retry,omitempty"`
		Compensate string           `json:"compensate,omitempty"`
		Attempts   []Attempt        `json:"attempts,omitempty"`
		State      map[string]any   `json:"state,omitempty"`
		Branch     bool             `json:"branch,omitempty"`
	}{
		ID:         s.id,
		Name:       s.Name,
		Status:     s.Status,
		Children:   s.Children,
		Edges:      s.Edges,
		Retry:      s.Retry,
		Compensate: s.Compensate,
		Attempts:   s.Attempts,
		State:      s.State,
		Branch:     s.Branch,
	})
}

func (s *Subgraph) UnmarshalJSON(data []byte) error {

	var payload struct {
		ID         string           `json:"id"`
		Name       string           `json:"name"`
		Status     SubgraphStatus   `json:"status"`
		Children   []SubgraphChild  `json:"children"`
		Edges      []Edge           `json:"edges,omitempty"`
		Retry      *RetryPolicy     `json:"retry,omitempty"`
		Compensate string           `json:"compensate,omitempty"`
		Attempts   []Attempt        `json:"attempts,omitempty"`
		State      map[string]any   `json:"state,omitempty"`
		Branch     bool             `json:"branch,omitempty"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	s.executableUnit = executableUnit{id: payload.ID}
	s.Name = payload.Name
	s.Status = payload.Status
	s.Children = payload.Children
	s.Edges = payload.Edges
	s.Retry = payload.Retry
	s.Compensate = payload.Compensate
	s.Attempts = payload.Attempts
	s.State = payload.State
	s.Branch = payload.Branch
	return nil
}

func (s *Subgraph) MarshalYAML() (any, error) {

	return struct {
		ID         string           `yaml:"id"`
		Name       string           `yaml:"name"`
		Status     SubgraphStatus   `yaml:"status"`
		Children   []SubgraphChild  `yaml:"children"`
		Edges      []Edge           `yaml:"edges,omitempty"`
		Retry      *RetryPolicy     `yaml:"retry,omitempty"`
		Compensate string           `yaml:"compensate,omitempty"`
		Attempts   []Attempt        `yaml:"attempts,omitempty"`
		State      map[string]any   `yaml:"state,omitempty"`
		Branch     bool             `yaml:"branch,omitempty"`
	}{
		ID:         s.id,
		Name:       s.Name,
		Status:     s.Status,
		Children:   s.Children,
		Edges:      s.Edges,
		Retry:      s.Retry,
		Compensate: s.Compensate,
		Attempts:   s.Attempts,
		State:      s.State,
		Branch:     s.Branch,
	}, nil
}

func (s *Subgraph) UnmarshalYAML(unmarshal func(any) error) error {

	var payload struct {
		ID         string           `yaml:"id"`
		Name       string           `yaml:"name"`
		Status     SubgraphStatus   `yaml:"status"`
		Children   []SubgraphChild  `yaml:"children"`
		Edges      []Edge           `yaml:"edges,omitempty"`
		Retry      *RetryPolicy     `yaml:"retry,omitempty"`
		Compensate string           `yaml:"compensate,omitempty"`
		Attempts   []Attempt        `yaml:"attempts,omitempty"`
		State      map[string]any   `yaml:"state,omitempty"`
		Branch     bool             `yaml:"branch,omitempty"`
	}
	if err := unmarshal(&payload); err != nil {
		return err
	}

	s.executableUnit = executableUnit{id: payload.ID}
	s.Name = payload.Name
	s.Status = payload.Status
	s.Children = payload.Children
	s.Edges = payload.Edges
	s.Retry = payload.Retry
	s.Compensate = payload.Compensate
	s.Attempts = payload.Attempts
	s.State = payload.State
	s.Branch = payload.Branch
	return nil
}

// endregion
