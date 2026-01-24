// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package engine

import "context"

// ExpandDelegates replaces delegate nodes in the graph with subgraphs
// produced by the given builder. Delegate nodes are identified by having
// "delegate" as their sole operation.
//
// The expanded subgraph's nodes and edges are appended to the parent graph.
// An ordering edge is added from the delegate node's predecessor (if any)
// to each root node of the subgraph.
//
// The original delegate node is removed from the graph after expansion.
func ExpandDelegates(ctx context.Context, graph *Graph, builder GraphBuilder, opts BuildOptions) error {
	var expanded []*Node
	var expandedEdges []Edge

	for _, node := range graph.Nodes {
		if !isDelegateNode(node) {
			expanded = append(expanded, node)
			continue
		}

		// Build subgraph from the manifest file
		sub, err := builder.BuildGraph(ctx, node.Source, opts)
		if err != nil {
			return err
		}

		// Append subgraph nodes
		expanded = append(expanded, sub.Nodes...)

		// Append subgraph edges
		expandedEdges = append(expandedEdges, sub.Edges...)

		// Add delegation edges: delegate node's project → each subgraph node
		for _, subNode := range sub.Nodes {
			expandedEdges = append(expandedEdges, Edge{
				From:     node.ID,
				To:       subNode.ID,
				Relation: "delegates",
			})
		}
	}

	graph.Nodes = expanded
	graph.Edges = append(graph.Edges, expandedEdges...)
	return nil
}

// isDelegateNode returns true if the node is a pure delegate operation.
func isDelegateNode(node *Node) bool {
	return len(node.Operations) == 1 && node.Operations[0] == "delegate"
}
