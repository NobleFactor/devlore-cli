// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package tree builds deployment trees from source and target specifications.
package tree

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/internal/manifest"
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

// FileEntry represents a file discovered during tree walking.
// This is pure file metadata - no execution state.
type FileEntry struct {
	// ID is the relative target path (unique identifier).
	ID string

	// Operations is the pipeline of operations to perform.
	// Examples: ["link"], ["decrypt", "render", "copy"].
	Operations []string

	// Source is the absolute path to the source file.
	Source string

	// Target is the absolute path to the target file.
	Target string

	// Project this file belongs to.
	Project string

	// Layer is the repository layer (base, team, personal).
	Layer string

	// TargetName is the target scope ("System" or "Home").
	// Set during multi-source builds from LayerSource.TargetName.
	TargetName string

	// Mode is the file permissions to set (0 means default 0644).
	Mode os.FileMode
}

// BuildResult contains the built file entries and build-time metadata.
type BuildResult struct {
	// Files are the file entries discovered.
	Files []*FileEntry

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
	// For multi-layer support, use Sources instead.
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

// fileEntryWithMeta tracks a file entry with its layer and specificity for collision detection.
type fileEntryWithMeta struct {
	entry       *FileEntry
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

	// Single-source mode
	return buildSingleSource(cfg)
}

// buildSingleSource builds from a single source root.
func buildSingleSource(cfg BuildConfig) (*BuildResult, error) {
	matches, err := segment.MatchDirectories(cfg.SourceRoot, cfg.Projects, cfg.Segments)
	if err != nil {
		return nil, err
	}

	result := &BuildResult{
		SourceRoot:  cfg.SourceRoot,
		TargetRoot:  cfg.TargetRoot,
		Projects:    cfg.Projects,
		MatchedDirs: matches,
	}

	entriesByTarget := make(map[string]fileEntryWithMeta)

	for _, match := range matches {
		entries, err := walkDirectory(match, cfg.TargetRoot)
		if err != nil {
			return nil, err
		}

		specificity := len(match.Suffixes)
		for _, entry := range entries {
			existing, exists := entriesByTarget[entry.ID]
			if !exists {
				entriesByTarget[entry.ID] = fileEntryWithMeta{entry: entry, specificity: specificity}
				continue
			}

			// Collision: more specific wins
			switch {
			case specificity > existing.specificity:
				result.Collisions = append(result.Collisions, Collision{
					Target:            entry.ID,
					Winner:            entry.Source,
					WinnerSpecificity: specificity,
					Loser:             existing.entry.Source,
					LoserSpecificity:  existing.specificity,
				})
				entriesByTarget[entry.ID] = fileEntryWithMeta{entry: entry, specificity: specificity}
			case specificity < existing.specificity:
				result.Collisions = append(result.Collisions, Collision{
					Target:            entry.ID,
					Winner:            existing.entry.Source,
					WinnerSpecificity: existing.specificity,
					Loser:             entry.Source,
					LoserSpecificity:  specificity,
				})
			default:
				// Same specificity — last wins
				result.Collisions = append(result.Collisions, Collision{
					Target:            entry.ID,
					Winner:            entry.Source,
					WinnerSpecificity: specificity,
					Loser:             existing.entry.Source,
					LoserSpecificity:  existing.specificity,
				})
				entriesByTarget[entry.ID] = fileEntryWithMeta{entry: entry, specificity: specificity}
			}
		}
	}

	// Convert map to sorted slice
	for _, meta := range entriesByTarget {
		result.Files = append(result.Files, meta.entry)
	}
	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].ID < result.Files[j].ID
	})

	return result, nil
}

// buildMultiSource builds from multiple layer sources with precedence.
// Layers are processed in order (base → team → personal).
// Higher-order layers override lower-order layers for the same target.
func buildMultiSource(cfg BuildConfig) (*BuildResult, error) { //nolint:gocognit
	result := &BuildResult{
		Sources:    cfg.Sources,
		TargetRoot: cfg.TargetRoot,
		Projects:   cfg.Projects,
	}

	entriesByTarget := make(map[string]fileEntryWithMeta)

	// Process sources in order (base → team → personal)
	for _, source := range cfg.Sources {
		matches, err := segment.MatchDirectories(source.SourceRoot, cfg.Projects, cfg.Segments)
		if err != nil {
			return nil, fmt.Errorf("layer %s: %w", source.Layer, err)
		}

		result.MatchedDirs = append(result.MatchedDirs, matches...)

		for _, match := range matches {
			entries, err := walkDirectory(match, source.TargetRoot)
			if err != nil {
				return nil, fmt.Errorf("layer %s: %w", source.Layer, err)
			}

			specificity := len(match.Suffixes)
			for _, entry := range entries {
				// Store layer and target scope in entry
				entry.Layer = source.Layer
				entry.TargetName = source.TargetName

				existing, exists := entriesByTarget[entry.ID]
				if !exists {
					entriesByTarget[entry.ID] = fileEntryWithMeta{
						entry:       entry,
						specificity: specificity,
						layerOrder:  source.Order,
						layer:       source.Layer,
					}
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
						Target:            entry.ID,
						Winner:            entry.Source,
						WinnerSpecificity: specificity,
						WinnerLayer:       source.Layer,
						Loser:             existing.entry.Source,
						LoserSpecificity:  existing.specificity,
						LoserLayer:        existing.layer,
					})
					entriesByTarget[entry.ID] = fileEntryWithMeta{
						entry:       entry,
						specificity: specificity,
						layerOrder:  source.Order,
						layer:       source.Layer,
					}
				} else {
					result.Collisions = append(result.Collisions, Collision{
						Target:            entry.ID,
						Winner:            existing.entry.Source,
						WinnerSpecificity: existing.specificity,
						WinnerLayer:       existing.layer,
						Loser:             entry.Source,
						LoserSpecificity:  specificity,
						LoserLayer:        source.Layer,
					})
				}
			}
		}
	}

	// Convert map to sorted slice
	for _, meta := range entriesByTarget {
		result.Files = append(result.Files, meta.entry)
	}
	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].ID < result.Files[j].ID
	})

	return result, nil
}

// walkDirectory walks a matched directory and returns file entries for all files.
func walkDirectory(match segment.MatchResult, targetRoot string) ([]*FileEntry, error) {
	var entries []*FileEntry

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
		targetName, actions := ProcessingPipeline(d.Name())

		var relTarget string
		if dir == "." {
			relTarget = targetName
		} else {
			relTarget = filepath.Join(dir, targetName)
		}

		// Secrets get restricted permissions
		var mode os.FileMode
		if hasAction(actions, "encryption.decrypt") {
			mode = 0o600
		}

		entry := &FileEntry{
			ID:         relTarget,
			Operations: actions,
			Source:     path,
			Target:     filepath.Join(targetRoot, relTarget),
			Project:    match.Project,
			Mode:       mode,
		}

		// Validate packages-manifest files
		if hasAction(actions, "manifest.resolve") {
			if manifest.IsManifestFile(d.Name()) {
				if err := manifest.Validate(path); err != nil {
					return fmt.Errorf("invalid %s: %w", relPath, err)
				}
			}
		}

		entries = append(entries, entry)
		return nil
	})

	return entries, err
}

// hasAction returns true if the actions slice contains the given name.
func hasAction(actions []string, name string) bool {
	for _, a := range actions {
		if a == name {
			return true
		}
	}
	return false
}

// HasCollisions returns true if there were file collisions during build.
func (r *BuildResult) HasCollisions() bool {
	return len(r.Collisions) > 0
}

// FileCount returns the number of files discovered.
func (r *BuildResult) FileCount() int {
	return len(r.Files)
}

// SecretCount returns the number of encrypted files.
func (r *BuildResult) SecretCount() int {
	count := 0
	for _, f := range r.Files {
		for _, action := range f.Operations {
			if action == "encryption.decrypt" {
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
	for _, f := range r.Files {
		for _, action := range f.Operations {
			if action == "template.render_bytes" {
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
	for _, f := range r.Files {
		if len(f.Operations) == 1 && f.Operations[0] == "file.link" {
			count++
		}
	}
	return count
}

// PackagesCount returns the number of packages-manifest entries.
func (r *BuildResult) PackagesCount() int {
	count := 0
	for _, f := range r.Files {
		for _, action := range f.Operations {
			if action == "manifest.resolve" {
				count++
				break
			}
		}
	}
	return count
}
