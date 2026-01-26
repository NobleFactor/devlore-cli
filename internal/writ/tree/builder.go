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

// BuildResult contains the built execution graph and build-time metadata.
type BuildResult struct {
	// Graph is the execution graph ready for the engine.
	Graph *engine.Graph

	// SourceRoot is the source root directory.
	SourceRoot string

	// TargetRoot is the target root directory.
	TargetRoot string

	// Projects included in this build.
	Projects []string

	// MatchedDirs are the directories that matched the segments.
	MatchedDirs []segment.MatchResult

	// Collisions are files where a more specific source overrode a less specific one.
	Collisions []Collision
}

// Collision records when a more specific file overrides a less specific one.
type Collision struct {
	// Target is the relative target path that had a collision.
	Target string

	// Winner is the source that won (more specific).
	Winner string

	// WinnerSpecificity is the number of suffixes on the winning directory.
	WinnerSpecificity int

	// Loser is the source that was overridden (less specific).
	Loser string

	// LoserSpecificity is the number of suffixes on the losing directory.
	LoserSpecificity int
}

// BuildConfig holds configuration for building a deployment graph.
type BuildConfig struct {
	// SourceRoot is the source directory (e.g., ~/dotfiles/Home/Configs).
	SourceRoot string

	// TargetRoot is the target directory (e.g., $HOME).
	TargetRoot string

	// Projects to include (e.g., ["all", "noblefactor"]).
	Projects []string

	// Segments for platform matching.
	Segments segment.Segments
}

// Build creates an execution graph from the given configuration.
func Build(cfg BuildConfig) (*BuildResult, error) {
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
	}

	// Collect nodes, tracking by target path for collision detection
	type nodeEntry struct {
		node        *engine.Node
		specificity int
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
