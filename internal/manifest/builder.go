// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package manifest

import (
	"context"
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Builder implements execution.SubgraphBuilder for packages-manifest files.
// It translates a packages-manifest into an execution graph that lore
// can process to install software.
type Builder struct{}

// NewBuilder creates a new packages-manifest graph builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// BuildSubgraph loads a packages-manifest file and builds an execution graph.
// This is the execution.SubgraphBuilder interface implementation.
//
// Entry point 1: Load, validate, and build from file path.
func (b *Builder) BuildSubgraph(ctx context.Context, manifestPath string, opts execution.BuildOptions) (*execution.Graph, error) {
	// Load and validate the manifest
	manifest, err := Load(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	// Validate against schema
	if err := Validate(manifestPath); err != nil {
		return nil, fmt.Errorf("validate manifest: %w", err)
	}

	return b.BuildGraphFromManifest(ctx, manifest, opts)
}

// BuildGraphFromManifest builds an execution graph from an already-parsed manifest.
//
// Entry point 2: Build from pre-parsed manifest (for callers who already have the data).
func (b *Builder) BuildGraphFromManifest(ctx context.Context, manifest *PackagesManifest, opts execution.BuildOptions) (*execution.Graph, error) {
	graph := &execution.Graph{
		Nodes: make([]*execution.Node, 0, len(manifest.Packages)*4),
	}

	for _, pkg := range manifest.Packages {
		nodes, edges := b.buildPackageNodes(pkg, opts)
		graph.Nodes = append(graph.Nodes, nodes...)
		graph.Edges = append(graph.Edges, edges...)
	}

	// Add dependency edges between packages if needed
	// (For now, packages are independent; registry resolution may add deps later)

	return graph, nil
}

// buildPackageNodes creates a chain of execution.Nodes for a single package entry.
// The four-phase lore pipeline: prepare → install → provision → verify
// Returns the nodes and edges for the chain.
func (b *Builder) buildPackageNodes(pkg PackageEntry, opts execution.BuildOptions) ([]*execution.Node, []execution.Edge) {
	phases := []string{"prepare", "install", "provision", "verify"}

	var nodes []*execution.Node
	var edges []execution.Edge
	var prevNode *execution.Node

	for i, phase := range phases {
		isLast := (i == len(phases) - 1)
		nodeID := pkg.Name
		if !isLast {
			nodeID = pkg.Name + ":" + phase
		}

		node := &execution.Node{
			ID:        nodeID,
			Operation: phase,
		}

		// Store package name for registry lookup on all nodes
		node.SetSlotImmediate("package", pkg.Name)

		// Store enabled features on the first node
		if i == 0 && len(pkg.With) > 0 {
			for j, feature := range pkg.With {
				node.SetSlotImmediate(fmt.Sprintf("feature.%d", j), feature)
			}
			node.SetSlotImmediate("feature_count", fmt.Sprintf("%d", len(pkg.With)))
		}

		nodes = append(nodes, node)

		if prevNode != nil {
			edges = append(edges, execution.Edge{
				From: prevNode.ID, To: node.ID,
			})
		}
		prevNode = node
	}

	return nodes, edges
}

// Ensure Builder implements execution.SubgraphBuilder.
var _ execution.SubgraphBuilder = (*Builder)(nil)
