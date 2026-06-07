// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"sort"
)

// DependencyView provides dependency analysis for a single execution graph by indexing the graph's
// edges into incoming and outgoing adjacency maps and pre-computing root and leaf sets.
type DependencyView struct {
	graph *Graph

	nodeByID   map[string]*Node    // node ID -> node
	dependsOn  map[string][]string // nodeID -> nodes it depends on (incoming edges)
	dependents map[string][]string // nodeID -> nodes that depend on it (outgoing edges)

	roots  []string // nodes with no dependencies
	leaves []string // nodes with no dependents
}

// region EXPORTED METHODS

// region State management

// NewDependencyView constructs a [DependencyView] over `g` by indexing its edges into incoming and
// outgoing adjacency maps and pre-computing the root and leaf sets.
//
// Parameters:
//   - `g`: the graph to analyze.
//
// Returns:
//   - *DependencyView: the constructed view.
func NewDependencyView(g *Graph) *DependencyView {

	v := &DependencyView{
		graph:      g,
		nodeByID:   make(map[string]*Node),
		dependsOn:  make(map[string][]string),
		dependents: make(map[string][]string),
	}

	for _, n := range g.Nodes() {
		v.nodeByID[n.ID()] = n
		v.dependsOn[n.ID()] = nil
		v.dependents[n.ID()] = nil
	}

	// Edge semantics: From -> To means "From must complete before To" so To depends on From.
	for _, e := range g.Root().edges {
		v.dependsOn[e.To] = append(v.dependsOn[e.To], e.From)
		v.dependents[e.From] = append(v.dependents[e.From], e.To)
	}

	for id := range v.nodeByID {
		if len(v.dependsOn[id]) == 0 {
			v.roots = append(v.roots, id)
		}
		if len(v.dependents[id]) == 0 {
			v.leaves = append(v.leaves, id)
		}
	}

	sort.Strings(v.roots)
	sort.Strings(v.leaves)

	return v
}

// EdgeCount returns the number of edges in the underlying graph.
//
// Returns:
//   - `int`: the edge count.
func (v *DependencyView) EdgeCount() int {

	return len(v.graph.Root().edges)
}

// Graph returns the underlying graph this view analyzes.
//
// Returns:
//   - *Graph: the graph.
func (v *DependencyView) Graph() *Graph {

	return v.graph
}

// Leaves returns the IDs of nodes with no dependents.
//
// Returns:
//   - []string: leaf node IDs, sorted ascending.
func (v *DependencyView) Leaves() []string {

	return v.leaves
}

// Node returns the node with `id`, or nil when no such node exists in the graph.
//
// Parameters:
//   - `id`: the node ID to look up.
//
// Returns:
//   - *Node: the node, or nil if not found.
func (v *DependencyView) Node(id string) *Node {

	return v.nodeByID[id]
}

// NodeCount returns the number of nodes in the underlying graph.
//
// Returns:
//   - `int`: the node count.
func (v *DependencyView) NodeCount() int {

	return len(v.nodeByID)
}

// Roots returns the IDs of nodes with no dependencies (executable immediately).
//
// Returns:
//   - []string: root node IDs, sorted ascending.
func (v *DependencyView) Roots() []string {

	return v.roots
}

// endregion

// region Behaviors

// Actions

// AllDependencies returns the transitive closure of dependencies for `nodeID` — every node that
// must complete before `nodeID` can start.
//
// Parameters:
//   - `nodeID`: the node whose ancestor closure is requested.
//
// Returns:
//   - []string: dependency IDs, sorted ascending, excluding `nodeID` itself.
func (v *DependencyView) AllDependencies(nodeID string) []string {

	visited := make(map[string]bool)
	v.collectDependencies(nodeID, visited)
	delete(visited, nodeID)

	result := make([]string, 0, len(visited))
	for id := range visited {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

// AllDependents returns the transitive closure of dependents for `nodeID` — every node that
// directly or indirectly depends on `nodeID`.
//
// Parameters:
//   - `nodeID`: the node whose descendant closure is requested.
//
// Returns:
//   - []string: dependent IDs, sorted ascending, excluding `nodeID` itself.
func (v *DependencyView) AllDependents(nodeID string) []string {

	visited := make(map[string]bool)
	v.collectDependents(nodeID, visited)
	delete(visited, nodeID)

	result := make([]string, 0, len(visited))
	for id := range visited {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

// CriticalPath returns the longest dependency chain in the graph — the minimum sequential
// execution path. Returns nil when the graph contains a cycle.
//
// Returns:
//   - []string: node IDs along the longest chain, ordered earliest to latest; nil when the graph
//     contains a cycle.
func (v *DependencyView) CriticalPath() []string {

	if v.HasCycle() {
		return nil
	}

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

	maxDist := 0
	var endNode string
	for _, leaf := range v.leaves {
		if dist[leaf] >= maxDist {
			maxDist = dist[leaf]
			endNode = leaf
		}
	}

	if endNode == "" && len(v.nodeByID) > 0 {
		for nodeID, d := range dist {
			if d >= maxDist {
				maxDist = d
				endNode = nodeID
			}
		}
	}

	var path []string
	for node := endNode; node != ""; node = prev[node] {
		path = append([]string{node}, path...)
	}

	return path
}

// DependsOn returns the direct dependencies of `nodeID` — the nodes that must complete first.
//
// Parameters:
//   - `nodeID`: the node whose direct dependencies are requested.
//
// Returns:
//   - []string: direct dependency IDs, sorted ascending.
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

// Dependents returns the direct dependents of `nodeID` — the nodes that wait for this one.
//
// Parameters:
//   - `nodeID`: the node whose direct dependents are requested.
//
// Returns:
//   - []string: direct dependent IDs, sorted ascending.
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

// HasCycle reports whether the underlying graph contains a cycle.
//
// Returns:
//   - `bool`: true when a cycle exists.
func (v *DependencyView) HasCycle() bool {

	return v.TopologicalOrder() == nil
}

// IndependentSets returns groups of nodes that have no dependencies between them; each set can be
// executed fully in parallel with the others.
//
// Returns:
//   - [][]string: independent groups; each inner slice is sorted ascending and the outer slice is
//     sorted by first element for deterministic ordering.
func (v *DependencyView) IndependentSets() [][]string {

	if len(v.nodeByID) == 0 {
		return nil
	}

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

	for _, e := range v.graph.Root().edges {
		union(e.From, e.To)
	}

	groups := make(map[string][]string)
	for id := range v.nodeByID {
		root := find(id)
		groups[root] = append(groups[root], id)
	}

	result := make([][]string, 0, len(groups))
	for _, group := range groups {
		sort.Strings(group)
		result = append(result, group)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i][0] < result[j][0]
	})

	return result
}

// ParallelLevels returns nodes grouped by execution level: level 0 contains roots, and level N
// contains nodes whose dependencies are all in levels < N. Within each level, nodes execute in
// parallel.
//
// Returns:
//   - [][]string: per-level node groupings; nil when the graph contains a cycle.
func (v *DependencyView) ParallelLevels() [][]string {

	if v.HasCycle() {
		return nil
	}

	level := make(map[string]int)
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

	for i := range levels {
		sort.Strings(levels[i])
	}

	return levels
}

// PathBetween returns the shortest path from `source` to `target`, or nil when none exists.
//
// Parameters:
//   - `source`: the starting node ID.
//   - `target`: the destination node ID.
//
// Returns:
//   - []string: the path from `source` to `target` inclusive, or nil when no path exists.
func (v *DependencyView) PathBetween(source, target string) []string {

	if source == target {
		return []string{source}
	}

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

	return nil
}

// Subgraph returns a new [DependencyView] containing only the specified nodes and the edges
// between them.
//
// Parameters:
//   - `nodeIDs`: the IDs of nodes to include in the subgraph.
//
// Returns:
//   - *DependencyView: a view over the restricted subgraph.
func (v *DependencyView) Subgraph(nodeIDs []string) *DependencyView {

	nodeSet := make(map[string]bool)

	for _, id := range nodeIDs {
		nodeSet[id] = true
	}

	children := make([]ExecutableUnit, 0, len(nodeIDs))

	for _, n := range v.graph.Nodes() {
		if nodeSet[n.ID()] {
			children = append(children, n)
		}
	}

	// The root spec is valid by construction (NewRootSubgraphSpec binds flow.subgraph by name), so NewSubgraph cannot
	// fail here; a non-nil error is a program-construction defect.
	root, err := NewSubgraph(NewRootSubgraphSpec().WithName("root").WithChildren(children...))

	if err != nil {
		panic(fmt.Sprintf("dependency view: root subgraph: %v", err))
	}

	// Replace the materialized edges (which may include cross-set producer IDs from filtered-out nodes' slot bindings)
	// with the strict subset of the source graph's edges where BOTH endpoints survive the filter. This is the
	// dependency-view semantic — preserve only the edges that connect nodes in the chosen subset.

	var filteredEdges []Edge

	for _, e := range v.graph.Root().edges {
		if nodeSet[e.From] && nodeSet[e.To] {
			filteredEdges = append(filteredEdges, e)
		}
	}

	root.setEdges(filteredEdges)

	subgraph := &Graph{
		root:          root,
		kind:          v.graph.Kind(),
		schemaVersion: v.graph.SerialVersion(),
		timestamp:     v.graph.Timestamp(),
		origin:        v.graph.origin,
	}

	return NewDependencyView(subgraph)
}

// TopologicalOrder returns nodes in a valid execution order — every node appears after all its
// dependencies. Returns nil when the graph contains a cycle.
//
// Returns:
//   - []string: node IDs in execution-safe order, or nil when a cycle exists.
func (v *DependencyView) TopologicalOrder() []string {

	inDegree := make(map[string]int)
	for id := range v.nodeByID {
		inDegree[id] = len(v.dependsOn[id])
	}

	queue := make([]string, 0, len(v.roots))
	queue = append(queue, v.roots...)

	var result []string
	for len(queue) > 0 {
		sort.Strings(queue)

		node := queue[0]
		queue = queue[1:]
		result = append(result, node)

		for _, dep := range v.dependents[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(result) != len(v.nodeByID) {
		return nil
	}

	return result
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// collectDependencies recursively visits each ancestor of `nodeID` via the `dependsOn` adjacency,
// marking it in `visited`.
//
// Parameters:
//   - `nodeID`: the node whose ancestors are being collected.
//   - `visited`: the set of node IDs visited so far; mutated in place.
func (v *DependencyView) collectDependencies(nodeID string, visited map[string]bool) {

	if visited[nodeID] {
		return
	}
	visited[nodeID] = true

	for _, dep := range v.dependsOn[nodeID] {
		v.collectDependencies(dep, visited)
	}
}

// collectDependents recursively visits each descendant of `nodeID` via the `dependents` adjacency,
// marking it in `visited`.
//
// Parameters:
//   - `nodeID`: the node whose descendants are being collected.
//   - `visited`: the set of node IDs visited so far; mutated in place.
func (v *DependencyView) collectDependents(nodeID string, visited map[string]bool) {

	if visited[nodeID] {
		return
	}
	visited[nodeID] = true

	for _, dep := range v.dependents[nodeID] {
		v.collectDependents(dep, visited)
	}
}

// endregion

// endregion
