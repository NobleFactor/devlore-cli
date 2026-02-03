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
		Nodes: make([]*execution.Node, 0, len(manifest.Packages)),
	}

	for _, pkg := range manifest.Packages {
		node := b.buildPackageNode(pkg, opts)
		graph.Nodes = append(graph.Nodes, node)
	}

	// Add dependency edges between packages if needed
	// (For now, packages are independent; registry resolution may add deps later)

	return graph, nil
}

// buildPackageNode creates an execution.Node for a single package entry.
func (b *Builder) buildPackageNode(pkg PackageEntry, opts execution.BuildOptions) *execution.Node {
	// Build the lore pipeline operations
	// The four-phase lore pipeline: prepare → install → provision → verify
	operations := []string{"prepare", "install", "provision", "verify"}

	node := &execution.Node{
		ID:         pkg.Name,
		Operations: operations,
	}

	// Store package name for registry lookup
	node.SetSlotImmediate("package", pkg.Name)

	// Store enabled features
	if len(pkg.With) > 0 {
		for i, feature := range pkg.With {
			node.SetSlotImmediate(fmt.Sprintf("feature.%d", i), feature)
		}
		node.SetSlotImmediate("feature_count", fmt.Sprintf("%d", len(pkg.With)))
	}

	return node
}

// Ensure Builder implements execution.SubgraphBuilder.
var _ execution.SubgraphBuilder = (*Builder)(nil)
