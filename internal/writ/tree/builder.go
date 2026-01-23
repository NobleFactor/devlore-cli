// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package tree

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/NobleFactor/devlore-cli/internal/writ/segment"
)

// Tree represents the full deployment tree.
type Tree struct {
	// SourceRoot is the source root directory (e.g., ~/dotfiles/Home/Configs).
	SourceRoot string `json:"source_root"`

	// TargetRoot is the target root directory (e.g., $HOME).
	TargetRoot string `json:"target_root"`

	// Projects included in this tree.
	Projects []string `json:"projects"`

	// MatchedDirs are the directories that matched the segments.
	MatchedDirs []segment.MatchResult `json:"matched_dirs"`

	// Nodes are all file nodes in the tree.
	Nodes []*Node `json:"nodes"`

	// Collisions are files where a more specific source overrode a less specific one.
	Collisions []Collision `json:"collisions,omitempty"`
}

// Collision records when a more specific file overrides a less specific one.
type Collision struct {
	// Target is the relative target path that had a collision.
	Target string `json:"target"`

	// Winner is the source that won (more specific).
	Winner string `json:"winner"`

	// WinnerSpecificity is the number of suffixes on the winning directory.
	WinnerSpecificity int `json:"winner_specificity"`

	// Loser is the source that was overridden (less specific).
	Loser string `json:"loser"`

	// LoserSpecificity is the number of suffixes on the losing directory.
	LoserSpecificity int `json:"loser_specificity"`
}

// BuildConfig holds configuration for building a deployment tree.
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

// Build creates a deployment tree from the given configuration.
func Build(cfg BuildConfig) (*Tree, error) {
	// Match directories for the requested projects
	matches, err := segment.MatchDirectories(cfg.SourceRoot, cfg.Projects, cfg.Segments)
	if err != nil {
		return nil, err
	}

	tree := &Tree{
		SourceRoot:  cfg.SourceRoot,
		TargetRoot:  cfg.TargetRoot,
		Projects:    cfg.Projects,
		MatchedDirs: matches,
	}

	// Collect all nodes, tracking by target path for collision detection
	nodesByTarget := make(map[string]*Node)

	// Walk each matched directory and collect files
	for _, match := range matches {
		nodes, err := walkDirectory(match, cfg.TargetRoot)
		if err != nil {
			return nil, err
		}

		for _, node := range nodes {
			existing, exists := nodesByTarget[node.RelTarget]
			if !exists {
				nodesByTarget[node.RelTarget] = node
				continue
			}

			// Collision detected - more specific wins
			existingSpec := len(existing.Suffixes)
			newSpec := len(node.Suffixes)

			if newSpec > existingSpec {
				// New node is more specific, it wins
				tree.Collisions = append(tree.Collisions, Collision{
					Target:            node.RelTarget,
					Winner:            node.Source,
					WinnerSpecificity: newSpec,
					Loser:             existing.Source,
					LoserSpecificity:  existingSpec,
				})
				nodesByTarget[node.RelTarget] = node
			} else if newSpec < existingSpec {
				// Existing is more specific, it stays
				tree.Collisions = append(tree.Collisions, Collision{
					Target:            node.RelTarget,
					Winner:            existing.Source,
					WinnerSpecificity: existingSpec,
					Loser:             node.Source,
					LoserSpecificity:  newSpec,
				})
			} else {
				// Same specificity - last one wins (arbitrary but deterministic based on match order)
				// This shouldn't happen in practice if directories are properly organized
				tree.Collisions = append(tree.Collisions, Collision{
					Target:            node.RelTarget,
					Winner:            node.Source,
					WinnerSpecificity: newSpec,
					Loser:             existing.Source,
					LoserSpecificity:  existingSpec,
				})
				nodesByTarget[node.RelTarget] = node
			}
		}
	}

	// Convert map to slice
	for _, node := range nodesByTarget {
		tree.Nodes = append(tree.Nodes, node)
	}

	// Sort nodes by target path for consistent output
	sort.Slice(tree.Nodes, func(i, j int) bool {
		return tree.Nodes[i].RelTarget < tree.Nodes[j].RelTarget
	})

	return tree, nil
}

// walkDirectory walks a matched directory and returns nodes for all files.
func walkDirectory(match segment.MatchResult, targetRoot string) ([]*Node, error) {
	var nodes []*Node

	err := filepath.WalkDir(match.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories (we only care about files)
		if d.IsDir() {
			return nil
		}

		// Note: We include hidden files (like .bashrc) - they're valid dotfiles

		// Get relative path from the matched directory
		relPath, err := filepath.Rel(match.Path, path)
		if err != nil {
			return err
		}

		// Determine target name and operations from filename
		dir := filepath.Dir(relPath)
		targetName, ops := ProcessingPipeline(d.Name())

		// Build relative target path
		var relTarget string
		if dir == "." {
			relTarget = targetName
		} else {
			relTarget = filepath.Join(dir, targetName)
		}

		// Determine file mode
		var mode os.FileMode
		for _, op := range ops {
			if op == OpDecrypt {
				mode = 0600 // Secrets get restricted permissions
				break
			}
		}

		node := &Node{
			Source:     path,
			Target:     filepath.Join(targetRoot, relTarget),
			RelSource:  relPath,
			RelTarget:  relTarget,
			Operations: ops,
			Mode:       mode,
			Project:    match.Project,
			Suffixes:   match.Suffixes,
		}

		nodes = append(nodes, node)
		return nil
	})

	return nodes, err
}

// FileCount returns the number of files in the tree.
func (t *Tree) FileCount() int {
	return len(t.Nodes)
}

// SecretCount returns the number of encrypted files.
func (t *Tree) SecretCount() int {
	count := 0
	for _, n := range t.Nodes {
		if n.IsSecret() {
			count++
		}
	}
	return count
}

// TemplateCount returns the number of template files.
func (t *Tree) TemplateCount() int {
	count := 0
	for _, n := range t.Nodes {
		if n.IsTemplate() {
			count++
		}
	}
	return count
}

// LinkCount returns the number of simple symlink files.
func (t *Tree) LinkCount() int {
	count := 0
	for _, n := range t.Nodes {
		if n.IsLink() {
			count++
		}
	}
	return count
}

// HasCollisions returns true if there were file collisions during build.
func (t *Tree) HasCollisions() bool {
	return len(t.Collisions) > 0
}

// CollisionCount returns the number of file collisions.
func (t *Tree) CollisionCount() int {
	return len(t.Collisions)
}
