// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"reflect"
	"testing"
)

func TestDependencyViewEmpty(t *testing.T) {
	g := &Graph{}
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
	g := &Graph{
		Nodes: []*Node{{ID: "a"}},
	}
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
	g := &Graph{
		Nodes: []*Node{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
		},
	}
	v := NewDependencyView(g)

	// Roots and leaves
	if !reflect.DeepEqual(v.Roots(), []string{"a"}) {
		t.Errorf("expected roots [a], got %v", v.Roots())
	}
	if !reflect.DeepEqual(v.Leaves(), []string{"c"}) {
		t.Errorf("expected leaves [c], got %v", v.Leaves())
	}

	// Direct dependencies
	if !reflect.DeepEqual(v.DependsOn("b"), []string{"a"}) {
		t.Errorf("b should depend on [a], got %v", v.DependsOn("b"))
	}
	if !reflect.DeepEqual(v.DependsOn("c"), []string{"b"}) {
		t.Errorf("c should depend on [b], got %v", v.DependsOn("c"))
	}
	if v.DependsOn("a") != nil {
		t.Errorf("a should have no dependencies, got %v", v.DependsOn("a"))
	}

	// Direct dependents
	if !reflect.DeepEqual(v.Dependents("a"), []string{"b"}) {
		t.Errorf("a should have dependents [b], got %v", v.Dependents("a"))
	}
	if !reflect.DeepEqual(v.Dependents("b"), []string{"c"}) {
		t.Errorf("b should have dependents [c], got %v", v.Dependents("b"))
	}

	// Transitive dependencies
	if !reflect.DeepEqual(v.AllDependencies("c"), []string{"a", "b"}) {
		t.Errorf("c should have all deps [a, b], got %v", v.AllDependencies("c"))
	}

	// Transitive dependents
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
	g := &Graph{
		Nodes: []*Node{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "a", To: "c"},
			{From: "b", To: "d"},
			{From: "c", To: "d"},
		},
	}
	v := NewDependencyView(g)

	if !reflect.DeepEqual(v.Roots(), []string{"a"}) {
		t.Errorf("expected roots [a], got %v", v.Roots())
	}
	if !reflect.DeepEqual(v.Leaves(), []string{"d"}) {
		t.Errorf("expected leaves [d], got %v", v.Leaves())
	}

	// d depends on both b and c
	deps := v.DependsOn("d")
	if !reflect.DeepEqual(deps, []string{"b", "c"}) {
		t.Errorf("d should depend on [b, c], got %v", deps)
	}

	// All dependencies of d includes a, b, c
	allDeps := v.AllDependencies("d")
	if !reflect.DeepEqual(allDeps, []string{"a", "b", "c"}) {
		t.Errorf("d all deps should be [a, b, c], got %v", allDeps)
	}
}

func TestDependencyViewTopologicalOrder(t *testing.T) {
	// a -> b -> d
	// a -> c -> d
	g := &Graph{
		Nodes: []*Node{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "a", To: "c"},
			{From: "b", To: "d"},
			{From: "c", To: "d"},
		},
	}
	v := NewDependencyView(g)

	order := v.TopologicalOrder()
	if len(order) != 4 {
		t.Fatalf("expected 4 nodes in order, got %d", len(order))
	}

	// a must come first
	if order[0] != "a" {
		t.Errorf("expected a first, got %s", order[0])
	}
	// d must come last
	if order[3] != "d" {
		t.Errorf("expected d last, got %s", order[3])
	}

	// b and c can be in either order, but must be between a and d
	middle := order[1:3]
	if !contains(middle, "b") || !contains(middle, "c") {
		t.Errorf("expected b and c in middle, got %v", middle)
	}
}

func TestDependencyViewCycleDetection(t *testing.T) {
	// a -> b -> c -> a (cycle)
	g := &Graph{
		Nodes: []*Node{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
			{From: "c", To: "a"},
		},
	}
	v := NewDependencyView(g)

	if !v.HasCycle() {
		t.Error("expected cycle to be detected")
	}
	if v.TopologicalOrder() != nil {
		t.Error("expected nil topological order for cyclic graph")
	}
}

func TestDependencyViewParallelLevels(t *testing.T) {
	//   a     e
	//  / \    |
	// b   c   f
	//  \ /
	//   d
	g := &Graph{
		Nodes: []*Node{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"}, {ID: "f"}},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "a", To: "c"},
			{From: "b", To: "d"},
			{From: "c", To: "d"},
			{From: "e", To: "f"},
		},
	}
	v := NewDependencyView(g)

	levels := v.ParallelLevels()
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d: %v", len(levels), levels)
	}

	// Level 0: a, e (roots)
	if !reflect.DeepEqual(levels[0], []string{"a", "e"}) {
		t.Errorf("level 0 should be [a, e], got %v", levels[0])
	}

	// Level 1: b, c, f
	if !reflect.DeepEqual(levels[1], []string{"b", "c", "f"}) {
		t.Errorf("level 1 should be [b, c, f], got %v", levels[1])
	}

	// Level 2: d
	if !reflect.DeepEqual(levels[2], []string{"d"}) {
		t.Errorf("level 2 should be [d], got %v", levels[2])
	}
}

func TestDependencyViewCriticalPath(t *testing.T) {
	// a -> b -> c -> d (critical path = 4)
	// a -> e -> d (shorter path = 3)
	g := &Graph{
		Nodes: []*Node{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"}},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
			{From: "c", To: "d"},
			{From: "a", To: "e"},
			{From: "e", To: "d"},
		},
	}
	v := NewDependencyView(g)

	path := v.CriticalPath()
	if !reflect.DeepEqual(path, []string{"a", "b", "c", "d"}) {
		t.Errorf("critical path should be [a, b, c, d], got %v", path)
	}
}

func TestDependencyViewIndependentSets(t *testing.T) {
	// Two disconnected components:
	// Component 1: a -> b
	// Component 2: c -> d
	// Component 3: e (standalone)
	g := &Graph{
		Nodes: []*Node{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"}},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "c", To: "d"},
		},
	}
	v := NewDependencyView(g)

	sets := v.IndependentSets()
	if len(sets) != 3 {
		t.Fatalf("expected 3 independent sets, got %d: %v", len(sets), sets)
	}

	// Check each set (order may vary but should be sorted)
	foundAB := false
	foundCD := false
	foundE := false
	for _, set := range sets {
		if reflect.DeepEqual(set, []string{"a", "b"}) {
			foundAB = true
		} else if reflect.DeepEqual(set, []string{"c", "d"}) {
			foundCD = true
		} else if reflect.DeepEqual(set, []string{"e"}) {
			foundE = true
		}
	}
	if !foundAB || !foundCD || !foundE {
		t.Errorf("expected sets {a,b}, {c,d}, {e}, got %v", sets)
	}
}

func TestDependencyViewPathBetween(t *testing.T) {
	// a -> b -> c -> d
	//      |
	//      v
	//      e
	g := &Graph{
		Nodes: []*Node{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"}},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
			{From: "c", To: "d"},
			{From: "b", To: "e"},
		},
	}
	v := NewDependencyView(g)

	// Path from a to d
	path := v.PathBetween("a", "d")
	if !reflect.DeepEqual(path, []string{"a", "b", "c", "d"}) {
		t.Errorf("path a->d should be [a, b, c, d], got %v", path)
	}

	// Path from a to e
	path = v.PathBetween("a", "e")
	if !reflect.DeepEqual(path, []string{"a", "b", "e"}) {
		t.Errorf("path a->e should be [a, b, e], got %v", path)
	}

	// Path from c to a (no path - wrong direction)
	path = v.PathBetween("c", "a")
	if path != nil {
		t.Errorf("no path c->a should exist, got %v", path)
	}

	// Path to self
	path = v.PathBetween("b", "b")
	if !reflect.DeepEqual(path, []string{"b"}) {
		t.Errorf("path b->b should be [b], got %v", path)
	}
}

func TestDependencyViewSubgraph(t *testing.T) {
	// Original: a -> b -> c -> d
	g := &Graph{
		Nodes: []*Node{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}},
		Edges: []Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
			{From: "c", To: "d"},
		},
	}
	v := NewDependencyView(g)

	// Subgraph with just b and c
	sub := v.Subgraph([]string{"b", "c"})

	if sub.NodeCount() != 2 {
		t.Errorf("expected 2 nodes in subgraph, got %d", sub.NodeCount())
	}
	if sub.EdgeCount() != 1 {
		t.Errorf("expected 1 edge in subgraph, got %d", sub.EdgeCount())
	}

	// b is now a root (no dependency on a in subgraph)
	if !reflect.DeepEqual(sub.Roots(), []string{"b"}) {
		t.Errorf("subgraph roots should be [b], got %v", sub.Roots())
	}
	// c is now a leaf
	if !reflect.DeepEqual(sub.Leaves(), []string{"c"}) {
		t.Errorf("subgraph leaves should be [c], got %v", sub.Leaves())
	}
}

func TestDependencyViewNode(t *testing.T) {
	g := &Graph{
		Nodes: []*Node{
			{ID: "a", Operations: []string{"link"}},
			{ID: "b", Operations: []string{"copy"}},
		},
	}
	v := NewDependencyView(g)

	nodeA := v.Node("a")
	if nodeA == nil {
		t.Fatal("expected to find node a")
	}
	if nodeA.Operations[0] != "link" {
		t.Errorf("expected node a to have link operation")
	}

	nodeC := v.Node("c")
	if nodeC != nil {
		t.Error("expected node c to not exist")
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
