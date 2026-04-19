// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"testing"
)

// nodesGraph builds a Graph with the given node IDs as root-level children.
func nodesGraph(ids []string, edges []Edge) *Graph {
	root := NewSubgraph("root")
	for _, id := range ids {
		root.Children = append(root.Children, SubgraphChild{Node: NewNode(id)})
	}
	root.Edges = edges
	return &Graph{Root: root}
}

func TestDependencyViewEmpty(t *testing.T) {
	g := &Graph{Root: NewSubgraph("root")}
	v := NewDependencyView(g)

	if v.NodeCount() != 0 {
		t.Errorf("expected 0 nodes, got %d", v.NodeCount())
	}
	if v.EdgeCount() != 0 {
		t.Errorf("expected 0 edges, got %d", v.EdgeCount())
	}
	if len(v.Roots()) != 0 {
		t.Errorf("expected 0 roots, got %d", len(v.Roots()))
	}
	if len(v.Leaves()) != 0 {
		t.Errorf("expected 0 leaves, got %d", len(v.Leaves()))
	}
}

func TestDependencyViewSingleNode(t *testing.T) {
	g := nodesGraph([]string{"a"}, nil)
	v := NewDependencyView(g)

	if v.NodeCount() != 1 {
		t.Errorf("expected 1 node, got %d", v.NodeCount())
	}

	roots := v.Roots()
	if len(roots) != 1 || roots[0] != "a" {
		t.Errorf("expected roots [a], got %v", roots)
	}

	leaves := v.Leaves()
	if len(leaves) != 1 || leaves[0] != "a" {
		t.Errorf("expected leaves [a], got %v", leaves)
	}
}

func TestDependencyViewLinearChain(t *testing.T) {
	// a -> b -> c
	g := nodesGraph([]string{"a", "b", "c"}, []Edge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
	})
	v := NewDependencyView(g)

	if !reflect.DeepEqual(v.Roots(), []string{"a"}) {
		t.Errorf("expected roots [a], got %v", v.Roots())
	}
	if !reflect.DeepEqual(v.Leaves(), []string{"c"}) {
		t.Errorf("expected leaves [c], got %v", v.Leaves())
	}

	if !reflect.DeepEqual(v.DependsOn("b"), []string{"a"}) {
		t.Errorf("b should depend on [a], got %v", v.DependsOn("b"))
	}
	if !reflect.DeepEqual(v.DependsOn("c"), []string{"b"}) {
		t.Errorf("c should depend on [b], got %v", v.DependsOn("c"))
	}
	if v.DependsOn("a") != nil {
		t.Errorf("a should have no dependencies, got %v", v.DependsOn("a"))
	}

	if !reflect.DeepEqual(v.Dependents("a"), []string{"b"}) {
		t.Errorf("a should have dependents [b], got %v", v.Dependents("a"))
	}
	if !reflect.DeepEqual(v.Dependents("b"), []string{"c"}) {
		t.Errorf("b should have dependents [c], got %v", v.Dependents("b"))
	}

	if !reflect.DeepEqual(v.AllDependencies("c"), []string{"a", "b"}) {
		t.Errorf("c should have all deps [a, b], got %v", v.AllDependencies("c"))
	}

	if !reflect.DeepEqual(v.AllDependents("a"), []string{"b", "c"}) {
		t.Errorf("a should have all dependents [b, c], got %v", v.AllDependents("a"))
	}
}

func TestDependencyViewDiamond(t *testing.T) {
	//     a
	//    / \
	//   b   c
	//    \ /
	//     d
	g := nodesGraph([]string{"a", "b", "c", "d"}, []Edge{
		{From: "a", To: "b"},
		{From: "a", To: "c"},
		{From: "b", To: "d"},
		{From: "c", To: "d"},
	})
	v := NewDependencyView(g)

	if !reflect.DeepEqual(v.Roots(), []string{"a"}) {
		t.Errorf("expected roots [a], got %v", v.Roots())
	}
	if !reflect.DeepEqual(v.Leaves(), []string{"d"}) {
		t.Errorf("expected leaves [d], got %v", v.Leaves())
	}

	deps := v.DependsOn("d")
	if !reflect.DeepEqual(deps, []string{"b", "c"}) {
		t.Errorf("d should depend on [b, c], got %v", deps)
	}

	allDeps := v.AllDependencies("d")
	if !reflect.DeepEqual(allDeps, []string{"a", "b", "c"}) {
		t.Errorf("d all deps should be [a, b, c], got %v", allDeps)
	}
}

func TestDependencyViewTopologicalOrder(t *testing.T) {
	g := nodesGraph([]string{"a", "b", "c", "d"}, []Edge{
		{From: "a", To: "b"},
		{From: "a", To: "c"},
		{From: "b", To: "d"},
		{From: "c", To: "d"},
	})
	v := NewDependencyView(g)

	order := v.TopologicalOrder()
	if len(order) != 4 {
		t.Fatalf("expected 4 nodes in order, got %d", len(order))
	}

	if order[0] != "a" {
		t.Errorf("expected a first, got %s", order[0])
	}
	if order[3] != "d" {
		t.Errorf("expected d last, got %s", order[3])
	}

	middle := order[1:3]
	if !contains(middle, "b") || !contains(middle, "c") {
		t.Errorf("expected b and c in middle, got %v", middle)
	}
}

func TestDependencyViewCycleDetection(t *testing.T) {
	g := nodesGraph([]string{"a", "b", "c"}, []Edge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "c", To: "a"},
	})
	v := NewDependencyView(g)

	if !v.HasCycle() {
		t.Error("expected cycle to be detected")
	}
	if v.TopologicalOrder() != nil {
		t.Error("expected nil topological order for cyclic graph")
	}
}

func TestDependencyViewParallelLevels(t *testing.T) {
	g := nodesGraph([]string{"a", "b", "c", "d", "e", "f"}, []Edge{
		{From: "a", To: "b"},
		{From: "a", To: "c"},
		{From: "b", To: "d"},
		{From: "c", To: "d"},
		{From: "e", To: "f"},
	})
	v := NewDependencyView(g)

	levels := v.ParallelLevels()
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d: %v", len(levels), levels)
	}

	if !reflect.DeepEqual(levels[0], []string{"a", "e"}) {
		t.Errorf("level 0 should be [a, e], got %v", levels[0])
	}
	if !reflect.DeepEqual(levels[1], []string{"b", "c", "f"}) {
		t.Errorf("level 1 should be [b, c, f], got %v", levels[1])
	}
	if !reflect.DeepEqual(levels[2], []string{"d"}) {
		t.Errorf("level 2 should be [d], got %v", levels[2])
	}
}

func TestDependencyViewCriticalPath(t *testing.T) {
	g := nodesGraph([]string{"a", "b", "c", "d", "e"}, []Edge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "c", To: "d"},
		{From: "a", To: "e"},
		{From: "e", To: "d"},
	})
	v := NewDependencyView(g)

	path := v.CriticalPath()
	if !reflect.DeepEqual(path, []string{"a", "b", "c", "d"}) {
		t.Errorf("critical path should be [a, b, c, d], got %v", path)
	}
}

func TestDependencyViewIndependentSets(t *testing.T) {
	g := nodesGraph([]string{"a", "b", "c", "d", "e"}, []Edge{
		{From: "a", To: "b"},
		{From: "c", To: "d"},
	})
	v := NewDependencyView(g)

	sets := v.IndependentSets()
	if len(sets) != 3 {
		t.Fatalf("expected 3 independent sets, got %d: %v", len(sets), sets)
	}

	foundAB := false
	foundCD := false
	foundE := false
	for _, set := range sets {
		switch {
		case reflect.DeepEqual(set, []string{"a", "b"}):
			foundAB = true
		case reflect.DeepEqual(set, []string{"c", "d"}):
			foundCD = true
		case reflect.DeepEqual(set, []string{"e"}):
			foundE = true
		}
	}
	if !foundAB || !foundCD || !foundE {
		t.Errorf("expected sets {a,b}, {c,d}, {e}, got %v", sets)
	}
}

func TestDependencyViewPathBetween(t *testing.T) {
	g := nodesGraph([]string{"a", "b", "c", "d", "e"}, []Edge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "c", To: "d"},
		{From: "b", To: "e"},
	})
	v := NewDependencyView(g)

	path := v.PathBetween("a", "d")
	if !reflect.DeepEqual(path, []string{"a", "b", "c", "d"}) {
		t.Errorf("path a->d should be [a, b, c, d], got %v", path)
	}

	path = v.PathBetween("a", "e")
	if !reflect.DeepEqual(path, []string{"a", "b", "e"}) {
		t.Errorf("path a->e should be [a, b, e], got %v", path)
	}

	path = v.PathBetween("c", "a")
	if path != nil {
		t.Errorf("no path c->a should exist, got %v", path)
	}

	path = v.PathBetween("b", "b")
	if !reflect.DeepEqual(path, []string{"b"}) {
		t.Errorf("path b->b should be [b], got %v", path)
	}
}

func TestDependencyViewSubgraph(t *testing.T) {
	g := nodesGraph([]string{"a", "b", "c", "d"}, []Edge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "c", To: "d"},
	})
	v := NewDependencyView(g)

	sub := v.Subgraph([]string{"b", "c"})

	if sub.NodeCount() != 2 {
		t.Errorf("expected 2 nodes in subgraph, got %d", sub.NodeCount())
	}
	if sub.EdgeCount() != 1 {
		t.Errorf("expected 1 edge in subgraph, got %d", sub.EdgeCount())
	}
	if !reflect.DeepEqual(sub.Roots(), []string{"b"}) {
		t.Errorf("subgraph roots should be [b], got %v", sub.Roots())
	}
	if !reflect.DeepEqual(sub.Leaves(), []string{"c"}) {
		t.Errorf("subgraph leaves should be [c], got %v", sub.Leaves())
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
