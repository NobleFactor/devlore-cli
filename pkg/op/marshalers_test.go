// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/application"
)

// TestGraph_Marshal_WritAdopt_YAML and TestGraph_Marshal_WritAdopt_JSON build an adopt-shaped graph
// (two adopt subgraphs, each with mkdir → move → link), serialize via the named encoder, decode the
// bytes back into a [graphPayload] directly, and verify the wire-form structure preserves all IDs,
// edges, names, and action labels.
//
// Under step 18.b, Graph / Node / Subgraph no longer implement [json.Unmarshaler] — the wire form
// decodes into payload structs, and [LoadGraph] is the registry-aware path that converts payloads
// to in-memory units. This test asserts on the payload structure directly, which doesn't require a
// registry; coverage of the registry-aware decode path lives alongside [plan.Provider.Load].

func TestGraph_Marshal_WritAdopt_YAML(t *testing.T) {

	runMarshalRoundTrip(t, "yaml", yaml.Marshal, yaml.Unmarshal)
}

func TestGraph_Marshal_WritAdopt_JSON(t *testing.T) {

	runMarshalRoundTrip(t, "json", json.Marshal, json.Unmarshal)
}

// runMarshalRoundTrip marshals an adopt-shaped graph, decodes the bytes into a [graphPayload], and
// verifies the projected wire structure.
func runMarshalRoundTrip(
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

	var loaded graphData
	if err := unmarshal(data, &loaded); err != nil {
		t.Fatalf("[%s] unmarshal: %v", name, err)
	}

	expectPayloadRootContainment(t, name, &loaded)
	expectPayloadSubgraph(t, name, &loaded, "adopt-foo")
	expectPayloadSubgraph(t, name, &loaded, "adopt-bar")
}

// TestGraph_SaveLoad_RoundTrip_ChecksumPreserved proves the save → load round-trip is integrity-preserving.
//
// It builds a registry-resolvable graph (root bound to flow.subgraph by name; two flow.complete leaves), sets a
// hand-authored root edge that NewGraph would never derive (neither leaf produces a slot the other consumes), recomputes
// the document checksum from the canonical content, marshals to YAML, and loads through LoadGraph. The reload must yield
// a graph whose recomputed checksum equals the document's — the implicit integrity check that buildGraph performs by
// recomputing rather than copying — with the hand edge, timestamp, and schema version intact.
func TestGraph_SaveLoad_RoundTrip_ChecksumPreserved(t *testing.T) {

	registry := NewReceiverRegistry()

	completeAction, err := registry.BuildAction("flow.complete")
	if err != nil {
		t.Fatalf("BuildAction(flow.complete): %v", err)
	}

	leafA, err := NewNode(NewNodeSpec().WithID("leaf-a").WithAction(completeAction))
	if err != nil {
		t.Fatalf("NewNode(leaf-a): %v", err)
	}

	leafB, err := NewNode(NewNodeSpec().WithID("leaf-b").WithAction(completeAction))
	if err != nil {
		t.Fatalf("NewNode(leaf-b): %v", err)
	}

	original, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(leafA, leafB))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}

	// A hand edge NewGraph never derives: neither leaf produces a slot the other consumes. Recompute the document
	// checksum so the serialized form carries a checksum that reflects the edge.
	handEdge := Edge{From: "leaf-a", To: "leaf-b"}
	original.root.edges = []Edge{handEdge}

	canonical, err := original.CanonicalContent()
	if err != nil {
		t.Fatalf("CanonicalContent: %v", err)
	}
	original.checksum = GitStyleChecksum("graph", canonical)

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}

	environment := NewRuntimeEnvironment(context.Background(),
		NewRuntimeEnvironmentSpec("test", registry).WithApplication(&application.Application{Name: "test"}))

	loaded, err := LoadGraph(environment, data, "yaml")
	if err != nil {
		t.Fatalf("LoadGraph: %v", err)
	}

	if loaded.Checksum() != original.Checksum() {
		t.Errorf("recomputed checksum: got %q, want %q (round-trip integrity broken)",
			loaded.Checksum(), original.Checksum())
	}

	if !reflect.DeepEqual(loaded.Edges(), []Edge{handEdge}) {
		t.Errorf("root edges: got %v, want %v (hand edge not preserved)", loaded.Edges(), []Edge{handEdge})
	}

	if loaded.SerialVersion() != original.SerialVersion() {
		t.Errorf("schema version: got %d, want %d", loaded.SerialVersion(), original.SerialVersion())
	}

	if !loaded.Timestamp().Equal(original.Timestamp()) {
		t.Errorf("timestamp: got %v, want %v", loaded.Timestamp(), original.Timestamp())
	}
}

// buildWritAdoptFixture constructs an in-memory Graph modeling two adopt operations, each a mkdir →
// move → link sequence wrapped in a Subgraph. Nodes carry a stub action; slots are left nil because
// the [SlotValue] interface has no marshalers today.
func buildWritAdoptFixture() *Graph {

	g, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(
		buildAdoptSubgraph("adopt-foo"),
		buildAdoptSubgraph("adopt-bar"),
	))
	if err != nil {
		panic("buildWritAdoptFixture: " + err.Error())
	}

	g.Root().edges = []Edge{{From: "adopt-foo", To: "adopt-bar"}}

	return g
}

// buildAdoptSubgraph constructs a single adopt-shaped Subgraph (mkdir → move → link) under the given ID.
func buildAdoptSubgraph(id string) *Subgraph {

	sg := stubSubgraph(id)
	sg.Name = "adopt"

	mkdir := newAdoptNode(id + ".mkdir")
	move := newAdoptNode(id + ".move")
	link := newAdoptNode(id + ".link")

	sg.addChild(mkdir)
	sg.addChild(move)
	sg.addChild(link)

	sg.edges = []Edge{
		{From: mkdir.ID(), To: move.ID()},
		{From: move.ID(), To: link.ID()},
	}

	return sg
}

// newAdoptNode constructs a Node carrying the given ID, bound to a stub [Action]. The wire-format
// round-trip test exercises only the symbol-table / containment layer (IDs, children, edges).
func newAdoptNode(id string) *Node {

	n, err := NewNode(NewNodeSpec().WithID(id).WithAction(&action{name: "adopt"}))
	if err != nil {
		panic("newAdoptNode: " + err.Error())
	}
	return n
}

// expectPayloadRootContainment verifies the decoded payload's Root-level containment.
func expectPayloadRootContainment(t *testing.T, name string, p *graphData) {

	t.Helper()

	wantChildren := []string{"adopt-foo", "adopt-bar"}
	if got := p.Children; !reflect.DeepEqual(got, wantChildren) {
		t.Errorf("[%s] root children: got %v, want %v", name, got, wantChildren)
	}

	wantEdges := []Edge{{From: "adopt-foo", To: "adopt-bar"}}
	if !reflect.DeepEqual(p.Edges, wantEdges) {
		t.Errorf("[%s] root edges: got %v, want %v", name, p.Edges, wantEdges)
	}
}

// expectPayloadSubgraph verifies the named Subgraph appears in the decoded payload's Subgraphs list
// with the expected name, children, edges, and action label.
func expectPayloadSubgraph(t *testing.T, name string, p *graphData, sgID string) {

	t.Helper()

	var sg *subgraphData
	for i := range p.Subgraphs {
		if p.Subgraphs[i].ID == sgID {
			sg = &p.Subgraphs[i]
			break
		}
	}
	if sg == nil {
		t.Fatalf("[%s] %s not in payload subgraphs", name, sgID)
	}

	if sg.Name != "adopt" {
		t.Errorf("[%s] %s.Name = %q, want %q", name, sgID, sg.Name, "adopt")
	}
	if sg.ActionName != "stub" {
		t.Errorf("[%s] %s.ActionName = %q, want %q", name, sgID, sg.ActionName, "stub")
	}

	wantChildren := []string{sgID + ".mkdir", sgID + ".move", sgID + ".link"}
	gotChildren := append([]string(nil), sg.Children...)
	sort.Strings(gotChildren)
	sort.Strings(wantChildren)
	if !reflect.DeepEqual(gotChildren, wantChildren) {
		t.Errorf("[%s] %s children: got %v, want %v", name, sgID, gotChildren, wantChildren)
	}

	wantEdges := []Edge{
		{From: sgID + ".mkdir", To: sgID + ".move"},
		{From: sgID + ".move", To: sgID + ".link"},
	}
	if !reflect.DeepEqual(sg.Edges, wantEdges) {
		t.Errorf("[%s] %s edges: got %v, want %v", name, sgID, sg.Edges, wantEdges)
	}

	// Every node in the subgraph should appear in the payload's Nodes list with action_name "adopt".
	for _, childID := range wantChildren {
		var found *nodeData
		for i := range p.Nodes {
			if p.Nodes[i].ID == childID {
				found = &p.Nodes[i]
				break
			}
		}
		if found == nil {
			t.Errorf("[%s] node %s not in payload nodes", name, childID)
			continue
		}
		if found.ActionName != "adopt" {
			t.Errorf("[%s] node %s.ActionName = %q, want %q", name, childID, found.ActionName, "adopt")
		}
	}
}
