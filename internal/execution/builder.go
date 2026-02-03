// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import "context"

// GraphBuilder is the interface for building execution graphs.
// Implementations are provided by tools (writ, lore) and create graphs
// from their respective inputs (file trees, package manifests, etc.).
type GraphBuilder interface {
	// Build creates an execution graph.
	// Implementations hold their configuration internally (set at construction).
	Build(ctx context.Context) (*Graph, error)
}

// SubgraphBuilder builds subgraphs from manifest files during delegate expansion.
// This is used when a node with operation "delegate" is encountered, causing
// the referenced manifest to be expanded into a subgraph.
type SubgraphBuilder interface {
	// BuildSubgraph creates a graph from a manifest file path.
	BuildSubgraph(ctx context.Context, manifestPath string, opts BuildOptions) (*Graph, error)
}

// BuildOptions configures subgraph building behavior.
type BuildOptions struct {
	// DryRun prevents the builder from making filesystem queries with side effects.
	DryRun bool

	// Features lists enabled features (e.g., "rootless", "compose").
	Features []string

	// Data holds tool context: platform info, environment, segments.
	Data map[string]any
}

// ExpandDelegates replaces delegate nodes in the graph with subgraphs
// produced by the given builder. Delegate nodes are identified by having
// "delegate" as their sole operation.
//
// The expanded subgraph's nodes and edges are appended to the parent graph.
// An ordering edge is added from the delegate node to each root node of
// the subgraph.
//
// The original delegate node is removed from the graph after expansion.
func ExpandDelegates(ctx context.Context, graph *Graph, builder SubgraphBuilder, opts BuildOptions) error {
	var expanded []*Node
	var expandedEdges []Edge

	for _, node := range graph.Nodes {
		if !isDelegateNode(node) {
			expanded = append(expanded, node)
			continue
		}

		// Build subgraph from the manifest file
		sub, err := builder.BuildSubgraph(ctx, node.GetSlot("source"), opts)
		if err != nil {
			return err
		}

		// Append subgraph nodes
		expanded = append(expanded, sub.Nodes...)

		// Append subgraph edges
		expandedEdges = append(expandedEdges, sub.Edges...)

		// Add edges: delegate node → each subgraph node
		for _, subNode := range sub.Nodes {
			expandedEdges = append(expandedEdges, Edge{
				From: node.ID,
				To:   subNode.ID,
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
