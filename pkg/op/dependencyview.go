// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"sort"
)

// DependencyView provides dependency analysis for a single execution graph.
// It indexes the graph's edges to enable efficient dependency queries.
type DependencyView struct {
	graph *Graph

	// Indexed lookups
	nodeByID   map[string]*Node
	dependsOn  map[string][]string // nodeID -> nodes it depends on (incoming edges)
	dependents map[string][]string // nodeID -> nodes that depend on it (outgoing edges)

	// Computed sets
	roots  []string // nodes with no dependencies
	leaves []string // nodes with no dependents
}

// NewDependencyView creates a DependencyView for the given graph.
func NewDependencyView(g *Graph) *DependencyView {
	v := &DependencyView{
		graph:      g,
		nodeByID:   make(map[string]*Node),
		dependsOn:  make(map[string][]string),
		dependents: make(map[string][]string),
	}

	// Index nodes
	for _, n := range g.Nodes() {
		v.nodeByID[n.ID()] = n
		// Initialize empty slices for all nodes
		v.dependsOn[n.ID()] = nil
		v.dependents[n.ID()] = nil
	}

	// Index edges
	// Edge semantics: From -> To means "From must complete before To"
	// So To depends on From
	for _, e := range g.Root.edges {
		v.dependsOn[e.To] = append(v.dependsOn[e.To], e.From)
		v.dependents[e.From] = append(v.dependents[e.From], e.To)
	}

	// Compute roots (no dependencies) and leaves (no dependents)
	for id := range v.nodeByID {
		if len(v.dependsOn[id]) == 0 {
			v.roots = append(v.roots, id)
		}
		if len(v.dependents[id]) == 0 {
			v.leaves = append(v.leaves, id)
		}
	}

	// Sort for consistent ordering
	sort.Strings(v.roots)
	sort.Strings(v.leaves)

	return v
}

// Graph returns the underlying graph.
func (v *DependencyView) Graph() *Graph {
	return v.graph
}

// Node returns the node with the given ID, or nil if not found.
func (v *DependencyView) Node(id string) *Node {
	return v.nodeByID[id]
}

// NodeCount returns the number of nodes in the graph.
func (v *DependencyView) NodeCount() int {
	return len(v.nodeByID)
}

// EdgeCount returns the number of edges in the graph.
func (v *DependencyView) EdgeCount() int {
	return len(v.graph.Root.edges)
}

// Roots returns nodes with no dependencies (can execute immediately).
func (v *DependencyView) Roots() []string {
	return v.roots
}

// Leaves returns nodes with no dependents (final nodes in the graph).
func (v *DependencyView) Leaves() []string {
	return v.leaves
}

// DependsOn returns the direct dependencies of a node (nodes that must complete first).
func (v *DependencyView) DependsOn(nodeID string) []string {
	deps := v.dependsOn[nodeID]
	if deps == nil {
		return nil
	}
	result := make([]string, len(deps))
	copy(result, deps)
	sort.Strings(result)
	return result
}

// Dependents returns the direct dependents of a node (nodes that wait for this one).
func (v *DependencyView) Dependents(nodeID string) []string {
	deps := v.dependents[nodeID]
	if deps == nil {
		return nil
	}
	result := make([]string, len(deps))
	copy(result, deps)
	sort.Strings(result)
	return result
}

// AllDependencies returns the transitive closure of dependencies for a node.
// This includes all nodes that must complete before this node can start.
func (v *DependencyView) AllDependencies(nodeID string) []string {
	visited := make(map[string]bool)
	v.collectDependencies(nodeID, visited)
	delete(visited, nodeID) // Don't include self

	result := make([]string, 0, len(visited))
	for id := range visited {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func (v *DependencyView) collectDependencies(nodeID string, visited map[string]bool) {
	if visited[nodeID] {
		return
	}
	visited[nodeID] = true

	for _, dep := range v.dependsOn[nodeID] {
		v.collectDependencies(dep, visited)
	}
}

// AllDependents returns the transitive closure of dependents for a node.
// This includes all nodes that directly or indirectly depend on this node.
func (v *DependencyView) AllDependents(nodeID string) []string {
	visited := make(map[string]bool)
	v.collectDependents(nodeID, visited)
	delete(visited, nodeID) // Don't include self

	result := make([]string, 0, len(visited))
	for id := range visited {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func (v *DependencyView) collectDependents(nodeID string, visited map[string]bool) {
	if visited[nodeID] {
		return
	}
	visited[nodeID] = true

	for _, dep := range v.dependents[nodeID] {
		v.collectDependents(dep, visited)
	}
}

// TopologicalOrder returns nodes in a valid execution order.
// Nodes appear after all their dependencies.
// Returns nil if the graph has a cycle.
func (v *DependencyView) TopologicalOrder() []string {
	// Kahn's algorithm
	inDegree := make(map[string]int)
	for id := range v.nodeByID {
		inDegree[id] = len(v.dependsOn[id])
	}

	// Start with roots (in-degree 0)
	queue := make([]string, 0, len(v.roots))
	queue = append(queue, v.roots...)

	var result []string
	for len(queue) > 0 {
		// Sort queue for deterministic order among nodes at same level
		sort.Strings(queue)

		// Take first node
		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		// Reduce in-degree of dependents
		for _, dep := range v.dependents[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	// Check for cycle
	if len(result) != len(v.nodeByID) {
		return nil // Cycle detected
	}

	return result
}

// HasCycle returns true if the graph contains a cycle.
func (v *DependencyView) HasCycle() bool {
	return v.TopologicalOrder() == nil
}

// ParallelLevels returns nodes grouped by execution level.
// Level 0 contains roots (can execute immediately).
// Level N contains nodes whose dependencies are all in levels < N.
// Within each level, nodes can execute in parallel.
func (v *DependencyView) ParallelLevels() [][]string {
	if v.HasCycle() {
		return nil
	}

	level := make(map[string]int)

	// BFS from roots
	for _, root := range v.roots {
		level[root] = 0
	}

	order := v.TopologicalOrder()
	for _, nodeID := range order {
		maxDepLevel := -1
		for _, dep := range v.dependsOn[nodeID] {
			if level[dep] > maxDepLevel {
				maxDepLevel = level[dep]
			}
		}
		level[nodeID] = maxDepLevel + 1
	}

	// Group by level
	maxLevel := 0
	for _, l := range level {
		if l > maxLevel {
			maxLevel = l
		}
	}

	levels := make([][]string, maxLevel+1)
	for nodeID, l := range level {
		levels[l] = append(levels[l], nodeID)
	}

	// Sort within each level
	for i := range levels {
		sort.Strings(levels[i])
	}

	return levels
}

// CriticalPath returns the longest dependency chain in the graph.
// This represents the minimum sequential execution path.
func (v *DependencyView) CriticalPath() []string {
	if v.HasCycle() {
		return nil
	}

	// Dynamic programming: longest path to each node
	dist := make(map[string]int)
	prev := make(map[string]string)

	order := v.TopologicalOrder()
	for _, nodeID := range order {
		maxDist := 0
		maxPrev := ""
		for _, dep := range v.dependsOn[nodeID] {
			if dist[dep]+1 > maxDist {
				maxDist = dist[dep] + 1
				maxPrev = dep
			}
		}
		dist[nodeID] = maxDist
		prev[nodeID] = maxPrev
	}

	// Find the leaf with maximum distance
	maxDist := 0
	var endNode string
	for _, leaf := range v.leaves {
		if dist[leaf] >= maxDist {
			maxDist = dist[leaf]
			endNode = leaf
		}
	}

	if endNode == "" && len(v.nodeByID) > 0 {
		// No leaves found (shouldn't happen in DAG), pick any node with max dist
		for nodeID, d := range dist {
			if d >= maxDist {
				maxDist = d
				endNode = nodeID
			}
		}
	}

	// Reconstruct path
	var path []string
	for node := endNode; node != ""; node = prev[node] {
		path = append([]string{node}, path...)
	}

	return path
}

// IndependentSets returns groups of nodes that have no dependencies between them.
// Each set can be executed fully in parallel with other sets.
func (v *DependencyView) IndependentSets() [][]string {
	if len(v.nodeByID) == 0 {
		return nil
	}

	// Use Union-Find to group connected nodes
	parent := make(map[string]string)
	for id := range v.nodeByID {
		parent[id] = id
	}

	var find func(string) string
	find = func(x string) string {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}

	union := func(x, y string) {
		px, py := find(x), find(y)
		if px != py {
			parent[px] = py
		}
	}

	// Union nodes connected by edges
	for _, e := range v.graph.Root.edges {
		union(e.From, e.To)
	}

	// Group by root
	groups := make(map[string][]string)
	for id := range v.nodeByID {
		root := find(id)
		groups[root] = append(groups[root], id)
	}

	// Convert to slice
	result := make([][]string, 0, len(groups))
	for _, group := range groups {
		sort.Strings(group)
		result = append(result, group)
	}

	// Sort groups by first element for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i][0] < result[j][0]
	})

	return result
}

// PathBetween returns the shortest path from source to target, or nil if none exists.
func (v *DependencyView) PathBetween(source, target string) []string {
	if source == target {
		return []string{source}
	}

	// BFS from source
	visited := map[string]bool{source: true}
	prev := make(map[string]string)
	queue := []string{source}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, next := range v.dependents[current] {
			if !visited[next] {
				visited[next] = true
				prev[next] = current
				if next == target {
					// Reconstruct path
					var path []string
					for node := target; node != ""; node = prev[node] {
						path = append([]string{node}, path...)
					}
					return path
				}
				queue = append(queue, next)
			}
		}
	}

	return nil // No path exists
}

// Subgraph returns a new DependencyView containing only the specified nodes
// and the edges between them.
func (v *DependencyView) Subgraph(nodeIDs []string) *DependencyView {
	nodeSet := make(map[string]bool)
	for _, id := range nodeIDs {
		nodeSet[id] = true
	}

	// Build subgraph
	subgraph := &Graph{
		Root:       NewSubgraph("root"),
		Version:    v.graph.Version,
		Timestamp:  v.graph.Timestamp,
		State:      v.graph.State,
		Provenance: v.graph.Provenance,
	}

	for _, n := range v.graph.Nodes() {
		if nodeSet[n.ID()] {
			subgraph.AddNode(n)
		}
	}

	for _, e := range v.graph.Root.edges {
		if nodeSet[e.From] && nodeSet[e.To] {
			subgraph.Root.edges = append(subgraph.Root.edges, e)
		}
	}

	return NewDependencyView(subgraph)
}
