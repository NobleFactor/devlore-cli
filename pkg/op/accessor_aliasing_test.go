// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"testing"
)

// region TEST FUNCTIONS

// TestSliceAccessors_DoNotAliasInternalStorage pins that the graph's slice accessors return copies
// (#532): a caller outside pkg/op could otherwise rewrite which edges and children the graph records,
// leaving the construction checksum describing contents that no longer exist.
//
// Each accessor is written through with a deliberately wrong value; the graph must be unmoved. The
// elements themselves stay shared — that is the sharing the rule permits, and only the slice header is
// defended.
func TestSliceAccessors_DoNotAliasInternalStorage(t *testing.T) {

	first, err := NewNode(NewNodeSpec().WithID("first").WithAction(&action{name: "file.copy"}))
	if err != nil {
		t.Fatalf("NewNode(first): %v", err)
	}
	second, err := NewNode(NewNodeSpec().WithID("second").WithAction(&action{name: "file.copy"}))
	if err != nil {
		t.Fatalf("NewNode(second): %v", err)
	}
	intruder, err := NewNode(NewNodeSpec().WithID("intruder").WithAction(&action{name: "file.copy"}))
	if err != nil {
		t.Fatalf("NewNode(intruder): %v", err)
	}

	graph, err := NewGraph(NewGraphSpec().WithOrigin(OriginBase{}).WithUnits(first, second))
	if err != nil {
		t.Fatalf("NewGraph: %v", err)
	}
	root := graph.Root()
	root.edges = []Edge{{From: "first", To: "second"}}

	t.Run("Graph.Edges", func(t *testing.T) {
		edges := graph.Edges()
		if len(edges) == 0 {
			t.Fatal("no edges to write through")
		}
		edges[0] = Edge{From: "intruder", To: "intruder"}

		if got := graph.Edges()[0]; got.From != "first" || got.To != "second" {
			t.Errorf("the graph's edge became %+v; the accessor aliases its storage", got)
		}
	})

	t.Run("Subgraph.Edges", func(t *testing.T) {
		edges := root.Edges()
		if len(edges) == 0 {
			t.Fatal("no edges to write through")
		}
		edges[0] = Edge{From: "intruder", To: "intruder"}

		if got := root.Edges()[0]; got.From != "first" || got.To != "second" {
			t.Errorf("the subgraph's edge became %+v; the accessor aliases its storage", got)
		}
	})

	t.Run("Subgraph.Children", func(t *testing.T) {
		children := root.Children()
		if len(children) < 2 {
			t.Fatalf("children = %d, want at least 2 to write through", len(children))
		}
		children[0] = intruder

		if got := root.Children()[0]; got.ID() == "intruder" {
			t.Error("the subgraph was re-parented through Children(); the accessor aliases its storage")
		}
	})

	t.Run("the elements stay shared", func(t *testing.T) {
		if got := root.Children()[0]; got != root.Children()[0] {
			t.Error("successive calls yield different units; only the slice header should be copied")
		}
	})
}

// endregion
