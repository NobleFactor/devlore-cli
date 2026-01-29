// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package graph provides the Package Graph Builder for lore.
//
// # Architecture
//
// The Package Graph Builder produces execution graph nodes for software package
// operations. It shares the execution engine with writ's File Tree Builder:
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                    Execution Engine                         │
//	│                  (internal/engine)                          │
//	├─────────────────────────────────────────────────────────────┤
//	│                                                             │
//	│  ┌─────────────────┐          ┌─────────────────┐          │
//	│  │  File Tree      │          │  Package Graph  │          │
//	│  │  Builder        │          │  Builder        │          │
//	│  │  (writ/tree)    │          │  (lore/graph)   │          │
//	│  └────────┬────────┘          └────────┬────────┘          │
//	│           │                            │                    │
//	│           │    ┌──────────────────┐    │                    │
//	│           └───►│ Execution Graph  │◄───┘                    │
//	│                │ (engine.Graph)   │                         │
//	│                └────────┬─────────┘                         │
//	│                         │                                   │
//	│                         ▼                                   │
//	│                ┌──────────────────┐                         │
//	│                │   Engine.Run()   │                         │
//	│                └──────────────────┘                         │
//	│                                                             │
//	└─────────────────────────────────────────────────────────────┘
//
// # Usage
//
// When writ encounters a packages-manifest.yaml file, it calls BuildFromManifest
// to produce package nodes that are merged into the execution graph:
//
//	// In writ deploy:
//	fileResult, _ := tree.Build(fileCfg)
//
//	// For each packages-manifest.yaml found:
//	pkgResult, _ := graph.BuildFromManifest(manifestPath, platform)
//
//	// Merge into single graph:
//	combinedGraph := engine.MergeGraphs(fileResult.Graph, pkgResult.Graph)
//
//	// Run unified graph:
//	results, _ := eng.Run(ctx, combinedGraph)
//
// When lore is invoked directly, it uses the same builder:
//
//	// In lore deploy:
//	pkgResult, _ := graph.BuildFromPackages(packageNames, platform)
//	results, _ := eng.Run(ctx, pkgResult.Graph)
//
// # NOT YET IMPLEMENTED
//
// This package is a stub. The following must be implemented:
//
//   - BuildFromManifest: Parse packages-manifest.yaml, resolve packages from
//     registry, produce install/configure/verify nodes
//
//   - BuildFromPackages: Resolve package names to registry entries, produce
//     install/configure/verify nodes
//
//   - Package operations: install, configure, verify (as engine.Direct operations)
//
//   - Registry integration: Resolve package names to installation instructions
//     for the current platform
package graph

import (
	"errors"

	"github.com/NobleFactor/devlore-cli/internal/engine"
)

// ErrNotImplemented is returned by stub functions that are not yet implemented.
var ErrNotImplemented = errors.New("Package Graph Builder not yet implemented")

// BuildResult contains the built execution graph and metadata for packages.
type BuildResult struct {
	// Graph is the execution graph ready for the engine.
	Graph *engine.Graph

	// Packages lists the resolved package names.
	Packages []string

	// Platform is the detected or specified platform.
	Platform string
}

// BuildConfig holds configuration for building a package graph.
type BuildConfig struct {
	// ManifestPath is the path to a packages-manifest.yaml file.
	// Mutually exclusive with Packages.
	ManifestPath string

	// Packages is a list of package names to install.
	// Mutually exclusive with ManifestPath.
	Packages []string

	// Platform is the target platform (e.g., "darwin-arm64").
	// If empty, auto-detected.
	Platform string

	// Features are optional feature flags to enable.
	Features []string

	// DryRun prevents actual installation when true.
	DryRun bool
}

// Build creates an execution graph from the given configuration.
//
// NOT YET IMPLEMENTED: Returns ErrNotImplemented.
//
// When implemented, this function will:
//  1. Parse the manifest or resolve package names
//  2. Query the registry for installation instructions
//  3. Produce install/configure/verify nodes for each package
//  4. Add dependency edges between nodes
func Build(cfg BuildConfig) (*BuildResult, error) {
	return nil, ErrNotImplemented
}

// BuildFromManifest creates an execution graph from a packages-manifest.yaml file.
//
// NOT YET IMPLEMENTED: Returns ErrNotImplemented.
//
// This is the entry point used by writ when it encounters a packages-manifest.yaml
// file during deployment. The resulting graph is merged with writ's file graph
// and executed by the shared engine.
func BuildFromManifest(manifestPath, platform string) (*BuildResult, error) {
	return nil, ErrNotImplemented
}

// BuildFromPackages creates an execution graph from a list of package names.
//
// NOT YET IMPLEMENTED: Returns ErrNotImplemented.
//
// This is the entry point used by "lore deploy <package>..." when installing
// packages directly without a manifest file.
func BuildFromPackages(packages []string, platform string) (*BuildResult, error) {
	return nil, ErrNotImplemented
}
