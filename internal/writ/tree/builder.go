// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package tree

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/NobleFactor/devlore-cli/internal/engine"
	"github.com/NobleFactor/devlore-cli/internal/writ/manifest"
	"github.com/NobleFactor/devlore-cli/internal/writ/segment"
)

// LayerSource represents a repository layer with its path and precedence order.
type LayerSource struct {
	Layer      string // "base", "team", or "personal"
	Path       string // Repo root path
	Order      int    // 0=base, 1=team, 2=personal (for precedence sorting)
	SourceRoot string // Full path to source directory (e.g., /path/to/repo/Home)
	TargetRoot string // Target root (e.g., $HOME or /)
	TargetName string // "System" or "Home"
}

// BuildResult contains the built execution graph and build-time metadata.
type BuildResult struct {
	// Graph is the execution graph ready for the engine.
	Graph *engine.Graph

	// SourceRoot is the source root directory (for single-source mode).
	SourceRoot string

	// TargetRoot is the target root directory.
	TargetRoot string

	// Sources are the layer sources processed (for multi-source mode).
	Sources []LayerSource

	// Projects included in this build.
	Projects []string

	// MatchedDirs are the directories that matched the segments.
	MatchedDirs []segment.MatchResult

	// Collisions are files where a more specific source overrode a less specific one.
	Collisions []Collision

	// NodeLayers tracks which layer each node came from (node.ID → layer name).
	NodeLayers map[string]string
}

// Collision records when a more specific file overrides a less specific one.
type Collision struct {
	// Target is the relative target path that had a collision.
	Target string

	// Winner is the source that won (more specific or higher layer).
	Winner string

	// WinnerSpecificity is the number of suffixes on the winning directory.
	WinnerSpecificity int

	// WinnerLayer is the layer of the winner (empty for single-source mode).
	WinnerLayer string

	// Loser is the source that was overridden (less specific or lower layer).
	Loser string

	// LoserSpecificity is the number of suffixes on the losing directory.
	LoserSpecificity int

	// LoserLayer is the layer of the loser (empty for single-source mode).
	LoserLayer string
}

// BuildConfig holds configuration for building a deployment graph.
type BuildConfig struct {
	// SourceRoot is the source directory for single-source mode.
	// Deprecated: Use Sources for multi-layer support.
	SourceRoot string

	// TargetRoot is the target directory (e.g., $HOME).
	// Used as default when Sources is empty.
	TargetRoot string

	// Sources are the layer sources for multi-source mode.
	// If empty, falls back to single-source mode using SourceRoot/TargetRoot.
	Sources []LayerSource

	// Projects to include (e.g., ["all", "noblefactor"]).
	Projects []string

	// Segments for platform matching.
	Segments segment.Segments
}

// nodeEntry tracks a node with its layer and specificity for collision detection.
type nodeEntry struct {
	node        *engine.Node
	specificity int
	layerOrder  int    // 0=base, 1=team, 2=personal
	layer       string // "base", "team", "personal", or "" for single-source mode
}

// Build creates an execution graph from the given configuration.
// Supports both single-source mode (SourceRoot) and multi-source mode (Sources).
func Build(cfg BuildConfig) (*BuildResult, error) {
	// Multi-source mode: process all layer sources
	if len(cfg.Sources) > 0 {
		return buildMultiSource(cfg)
	}

	// Single-source mode: backwards compatible
	return buildSingleSource(cfg)
}

// buildSingleSource builds from a single source root (backwards compatible).
func buildSingleSource(cfg BuildConfig) (*BuildResult, error) {
	matches, err := segment.MatchDirectories(cfg.SourceRoot, cfg.Projects, cfg.Segments)
	if err != nil {
		return nil, err
	}

	result := &BuildResult{
		Graph:       &engine.Graph{},
		SourceRoot:  cfg.SourceRoot,
		TargetRoot:  cfg.TargetRoot,
		Projects:    cfg.Projects,
		MatchedDirs: matches,
		NodeLayers:  make(map[string]string),
	}

	nodesByTarget := make(map[string]nodeEntry)

	for _, match := range matches {
		nodes, err := walkDirectory(match, cfg.TargetRoot)
		if err != nil {
			return nil, err
		}

		specificity := len(match.Suffixes)
		for _, node := range nodes {
			existing, exists := nodesByTarget[node.ID]
			if !exists {
				nodesByTarget[node.ID] = nodeEntry{node: node, specificity: specificity}
				continue
			}

			// Collision: more specific wins
			if specificity > existing.specificity {
				result.Collisions = append(result.Collisions, Collision{
					Target:            node.ID,
					Winner:            node.Source,
					WinnerSpecificity: specificity,
					Loser:             existing.node.Source,
					LoserSpecificity:  existing.specificity,
				})
				nodesByTarget[node.ID] = nodeEntry{node: node, specificity: specificity}
			} else if specificity < existing.specificity {
				result.Collisions = append(result.Collisions, Collision{
					Target:            node.ID,
					Winner:            existing.node.Source,
					WinnerSpecificity: existing.specificity,
					Loser:             node.Source,
					LoserSpecificity:  specificity,
				})
			} else {
				// Same specificity — last wins
				result.Collisions = append(result.Collisions, Collision{
					Target:            node.ID,
					Winner:            node.Source,
					WinnerSpecificity: specificity,
					Loser:             existing.node.Source,
					LoserSpecificity:  existing.specificity,
				})
				nodesByTarget[node.ID] = nodeEntry{node: node, specificity: specificity}
			}
		}
	}

	// Convert map to sorted slice
	for _, entry := range nodesByTarget {
		result.Graph.Nodes = append(result.Graph.Nodes, entry.node)
	}
	sort.Slice(result.Graph.Nodes, func(i, j int) bool {
		return result.Graph.Nodes[i].ID < result.Graph.Nodes[j].ID
	})

	return result, nil
}

// buildMultiSource builds from multiple layer sources with precedence.
// Layers are processed in order (base → team → personal).
// Higher-order layers override lower-order layers for the same target.
func buildMultiSource(cfg BuildConfig) (*BuildResult, error) {
	result := &BuildResult{
		Graph:      &engine.Graph{},
		Sources:    cfg.Sources,
		TargetRoot: cfg.TargetRoot,
		Projects:   cfg.Projects,
		NodeLayers: make(map[string]string),
	}

	// Set SourceRoot to first source for backwards compatibility
	if len(cfg.Sources) > 0 {
		result.SourceRoot = cfg.Sources[0].SourceRoot
	}

	nodesByTarget := make(map[string]nodeEntry)

	// Process sources in order (base → team → personal)
	for _, source := range cfg.Sources {
		matches, err := segment.MatchDirectories(source.SourceRoot, cfg.Projects, cfg.Segments)
		if err != nil {
			return nil, fmt.Errorf("layer %s: %w", source.Layer, err)
		}

		result.MatchedDirs = append(result.MatchedDirs, matches...)

		for _, match := range matches {
			nodes, err := walkDirectory(match, source.TargetRoot)
			if err != nil {
				return nil, fmt.Errorf("layer %s: %w", source.Layer, err)
			}

			specificity := len(match.Suffixes)
			for _, node := range nodes {
				// Store layer in node metadata
				node.Metadata["layer"] = source.Layer

				existing, exists := nodesByTarget[node.ID]
				if !exists {
					nodesByTarget[node.ID] = nodeEntry{
						node:        node,
						specificity: specificity,
						layerOrder:  source.Order,
						layer:       source.Layer,
					}
					result.NodeLayers[node.ID] = source.Layer
					continue
				}

				// Collision resolution: layer takes precedence, then specificity
				newWins := false
				if source.Order > existing.layerOrder {
					// Higher layer always wins (personal > team > base)
					newWins = true
				} else if source.Order == existing.layerOrder {
					// Same layer: specificity wins, or last if equal
					if specificity >= existing.specificity {
						newWins = true
					}
				}

				if newWins {
					result.Collisions = append(result.Collisions, Collision{
						Target:            node.ID,
						Winner:            node.Source,
						WinnerSpecificity: specificity,
						WinnerLayer:       source.Layer,
						Loser:             existing.node.Source,
						LoserSpecificity:  existing.specificity,
						LoserLayer:        existing.layer,
					})
					nodesByTarget[node.ID] = nodeEntry{
						node:        node,
						specificity: specificity,
						layerOrder:  source.Order,
						layer:       source.Layer,
					}
					result.NodeLayers[node.ID] = source.Layer
				} else {
					result.Collisions = append(result.Collisions, Collision{
						Target:            node.ID,
						Winner:            existing.node.Source,
						WinnerSpecificity: existing.specificity,
						WinnerLayer:       existing.layer,
						Loser:             node.Source,
						LoserSpecificity:  specificity,
						LoserLayer:        source.Layer,
					})
				}
			}
		}
	}

	// Convert map to sorted slice
	for _, entry := range nodesByTarget {
		result.Graph.Nodes = append(result.Graph.Nodes, entry.node)
	}
	sort.Slice(result.Graph.Nodes, func(i, j int) bool {
		return result.Graph.Nodes[i].ID < result.Graph.Nodes[j].ID
	})

	return result, nil
}

// walkDirectory walks a matched directory and returns engine nodes for all files.
func walkDirectory(match segment.MatchResult, targetRoot string) ([]*engine.Node, error) {
	var nodes []*engine.Node

	err := filepath.WalkDir(match.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(match.Path, path)
		if err != nil {
			return err
		}

		dir := filepath.Dir(relPath)
		targetName, ops := ProcessingPipeline(d.Name())

		var relTarget string
		if dir == "." {
			relTarget = targetName
		} else {
			relTarget = filepath.Join(dir, targetName)
		}

		// Secrets get restricted permissions
		var mode os.FileMode
		for _, op := range ops {
			if op == OpDecrypt {
				mode = 0600
				break
			}
		}

		node := &engine.Node{
			ID:         relTarget,
			Operations: ops.Strings(),
			Source:     path,
			Target:     filepath.Join(targetRoot, relTarget),
			Project:    match.Project,
			Mode:       mode,
			Metadata:   make(map[string]string),
		}

		if ops.HasDelegate() {
			node.DelegateTo = "lore"

			// Validate packages-manifest files
			if manifest.IsManifestFile(d.Name()) {
				if err := manifest.Validate(path); err != nil {
					return fmt.Errorf("invalid %s: %w", relPath, err)
				}
			}
		}

		nodes = append(nodes, node)
		return nil
	})

	return nodes, err
}

// HasCollisions returns true if there were file collisions during build.
func (r *BuildResult) HasCollisions() bool {
	return len(r.Collisions) > 0
}

// FileCount returns the number of files in the graph.
func (r *BuildResult) FileCount() int {
	return len(r.Graph.Nodes)
}

// SecretCount returns the number of encrypted files.
func (r *BuildResult) SecretCount() int {
	count := 0
	for _, n := range r.Graph.Nodes {
		for _, op := range n.Operations {
			if op == "decrypt" {
				count++
				break
			}
		}
	}
	return count
}

// TemplateCount returns the number of template files.
func (r *BuildResult) TemplateCount() int {
	count := 0
	for _, n := range r.Graph.Nodes {
		for _, op := range n.Operations {
			if op == "expand" {
				count++
				break
			}
		}
	}
	return count
}

// LinkCount returns the number of simple symlink files.
func (r *BuildResult) LinkCount() int {
	count := 0
	for _, n := range r.Graph.Nodes {
		if len(n.Operations) == 1 && n.Operations[0] == "link" {
			count++
		}
	}
	return count
}

// DelegateCount returns the number of delegate nodes.
func (r *BuildResult) DelegateCount() int {
	count := 0
	for _, n := range r.Graph.Nodes {
		if n.DelegateTo != "" {
			count++
		}
	}
	return count
}
