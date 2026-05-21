// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestGraph_RoundTrip_WritAdopt_YAML and TestGraph_RoundTrip_WritAdopt_JSON build an adopt-shaped graph (two adopt
// subgraphs, each with mkdir → move → link), serialize via the named encoder, deserialize into a fresh Graph, and
// verify the containment round-trip preserves all IDs, edges, status fields, and parentID stamps.
//
// Slot values are intentionally omitted from the fixture — the [SlotValue] interface lacks marshalers (a pre-existing
// gap orthogonal to the wire-format refactor), so the test exercises only the symbol-table / containment layer: the
// surface 13.0(n)'s wire-format change is concerned with.

func TestGraph_RoundTrip_WritAdopt_YAML(t *testing.T) {

	runRoundTrip(t, "yaml", yaml.Marshal, yaml.Unmarshal)
}

func TestGraph_RoundTrip_WritAdopt_JSON(t *testing.T) {

	runRoundTrip(t, "json", json.Marshal, json.Unmarshal)
}

// TestGraph_Unmarshal_RejectsDanglingChild verifies that [Subgraph.linkChildren] surfaces a dangling child reference
// during Graph unmarshal.
func TestGraph_Unmarshal_RejectsDanglingChild(t *testing.T) {

	payload := []byte(`{
		"version": "6",
		"state": "pending",
		"timestamp": "2026-05-17T00:00:00Z",
		"children": ["does-not-exist"],
		"provenance": {}
	}`)

	var g Graph
	err := json.Unmarshal(payload, &g)
	if err == nil {
		t.Fatal("expected error for dangling child reference, got nil")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error %q does not name the dangling child", err.Error())
	}
}

// TestGraph_Unmarshal_RejectsDanglingEdge verifies that [Subgraph.validateEdges] surfaces a dangling Edge endpoint
// during Graph unmarshal.
func TestGraph_Unmarshal_RejectsDanglingEdge(t *testing.T) {

	payload := []byte(`{
		"version": "6",
		"state": "pending",
		"timestamp": "2026-05-17T00:00:00Z",
		"children": ["only-node"],
		"edges": [{"from": "only-node", "to": "missing-node"}],
		"nodes": [{"id": "only-node", "receiver": "file.Touch", "status": "pending"}],
		"provenance": {}
	}`)

	var g Graph
	err := json.Unmarshal(payload, &g)
	if err == nil {
		t.Fatal("expected error for dangling edge endpoint, got nil")
	}
	if !strings.Contains(err.Error(), "missing-node") {
		t.Errorf("error %q does not name the dangling endpoint", err.Error())
	}
}

// runRoundTrip marshals an adopt-shaped graph, unmarshals into a fresh Graph, and verifies the result.
func runRoundTrip(
	t *testing.T,
	name string,
	marshal func(any) ([]byte, error),
	unmarshal func([]byte, any) error,
) {

	t.Helper()

	original := buildWritAdoptFixture()

	data, err := marshal(original)
	if err != nil {
		t.Fatalf("[%s] marshal: %v", name, err)
	}

	loaded := &Graph{}
	if err := unmarshal(data, loaded); err != nil {
		t.Fatalf("[%s] unmarshal: %v", name, err)
	}

	expectRootContainment(t, name, loaded)
	expectSubgraph(t, name, loaded, "adopt-foo")
	expectSubgraph(t, name, loaded, "adopt-bar")
	expectParentIDStamps(t, name, loaded)
}

// buildWritAdoptFixture constructs an in-memory Graph modeling two adopt operations, each a mkdir → move → link
// sequence wrapped in a Subgraph. Nodes carry file.* receivers by name only; slots are left nil because the
// SlotValue interface has no marshalers today.
func buildWritAdoptFixture() *Graph {

	g := NewGraph()

	g.AddSubgraph(buildAdoptSubgraph("adopt-foo"))
	g.AddSubgraph(buildAdoptSubgraph("adopt-bar"))

	g.Root.edges = []Edge{{From: "adopt-foo", To: "adopt-bar"}}

	return g
}

// buildAdoptSubgraph constructs a single adopt-shaped Subgraph (mkdir → move → link) under the given ID.
func buildAdoptSubgraph(id string) *Subgraph {

	sg := NewSubgraph(id)
	sg.Name = "adopt"
	sg.Status = StatusPending

	mkdir := newAdoptNode(id+".mkdir", "file.Mkdir")
	move := newAdoptNode(id+".move", "file.Move")
	link := newAdoptNode(id+".link", "file.Link")

	sg.AddChild(mkdir)
	sg.AddChild(move)
	sg.AddChild(link)

	sg.edges = []Edge{
		{From: mkdir.ID(), To: move.ID()},
		{From: move.ID(), To: link.ID()},
	}

	return sg
}

// newAdoptNode constructs a Node carrying the given ID and Receiver, in StatusPending.
func newAdoptNode(id, receiver string) *Node {

	n := NewNode(id)
	n.Receiver = receiver
	n.Status = StatusPending
	return n
}

// expectRootContainment verifies the loaded Graph's Root has the expected child IDs and edges.
func expectRootContainment(t *testing.T, name string, g *Graph) {

	t.Helper()

	wantChildren := []string{"adopt-foo", "adopt-bar"}
	if got := childIDsOf(g.Root); !reflect.DeepEqual(got, wantChildren) {
		t.Errorf("[%s] root children: got %v, want %v", name, got, wantChildren)
	}

	wantEdges := []Edge{{From: "adopt-foo", To: "adopt-bar"}}
	if !reflect.DeepEqual(g.Root.edges, wantEdges) {
		t.Errorf("[%s] root edges: got %v, want %v", name, g.Root.edges, wantEdges)
	}
}

// expectSubgraph verifies the loaded Graph's named Subgraph has the right name, status, children, and edges.
func expectSubgraph(t *testing.T, name string, g *Graph, sgID string) {

	t.Helper()

	unit, ok := g.unitsByID[sgID]
	if !ok {
		t.Fatalf("[%s] %s not in unit table", name, sgID)
	}

	sg, ok := unit.(*Subgraph)
	if !ok {
		t.Fatalf("[%s] %s is not a *Subgraph (got %T)", name, sgID, unit)
	}

	if sg.Name != "adopt" {
		t.Errorf("[%s] %s.Name = %q, want %q", name, sgID, sg.Name, "adopt")
	}
	if sg.Status != StatusPending {
		t.Errorf("[%s] %s.Status = %q, want %q", name, sgID, sg.Status, StatusPending)
	}

	wantChildren := []string{sgID + ".mkdir", sgID + ".move", sgID + ".link"}
	if got := childIDsOf(sg); !reflect.DeepEqual(got, wantChildren) {
		t.Errorf("[%s] %s children: got %v, want %v", name, sgID, got, wantChildren)
	}

	wantEdges := []Edge{
		{From: sgID + ".mkdir", To: sgID + ".move"},
		{From: sgID + ".move", To: sgID + ".link"},
	}
	if !reflect.DeepEqual(sg.edges, wantEdges) {
		t.Errorf("[%s] %s edges: got %v, want %v", name, sgID, sg.edges, wantEdges)
	}
}

// expectParentIDStamps verifies that every unit in the loaded Graph has its parentID re-stamped by
// [Subgraph.linkChildren] to the expected enclosing Subgraph's ID.
func expectParentIDStamps(t *testing.T, name string, g *Graph) {

	t.Helper()

	cases := []struct {
		unitID, wantParent string
	}{
		{"adopt-foo", "root"},
		{"adopt-bar", "root"},
		{"adopt-foo.mkdir", "adopt-foo"},
		{"adopt-foo.move", "adopt-foo"},
		{"adopt-foo.link", "adopt-foo"},
		{"adopt-bar.mkdir", "adopt-bar"},
		{"adopt-bar.move", "adopt-bar"},
		{"adopt-bar.link", "adopt-bar"},
	}

	for _, tc := range cases {
		unit, ok := g.unitsByID[tc.unitID]
		if !ok {
			t.Errorf("[%s] %s not in unit table", name, tc.unitID)
			continue
		}

		var gotParent string
		switch u := unit.(type) {
		case *Node:
			gotParent = u.ParentID()
		case *Subgraph:
			gotParent = u.ParentID()
		}

		if gotParent != tc.wantParent {
			t.Errorf("[%s] %s.ParentID() = %q, want %q", name, tc.unitID, gotParent, tc.wantParent)
		}
	}
}

// childIDsOf returns the IDs of the given Subgraph's direct children in order.
func childIDsOf(sg *Subgraph) []string {

	ids := make([]string, len(sg.executableUnits))
	for i, c := range sg.executableUnits {
		ids[i] = c.ID()
	}
	return ids
}
