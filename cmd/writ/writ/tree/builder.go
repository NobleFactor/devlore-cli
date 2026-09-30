// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

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
	"github.com/NobleFactor/devlore-cli/pkg/selector"
)

// LayerSource represents a repository layer with its path and precedence order.
type LayerSource struct {
	Layer      string // "base", "team", or "personal"
	Path       string // Repo root path
	Order      int    // 0=base, 1=team, 2=personal (for precedence sorting)
	SourceRoot string // Full path to the directory planning reads (a pinned snapshot under layered deploys)
	OriginRoot string // Full path to the origin-repo directory links target and records name; SourceRoot when unpinned
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

	// Source is the absolute path to the source file planning and execution read — under a layered
	// deploy, inside the pinned snapshot worktree.
	Source string

	// Origin is the absolute origin-repo path for the same file. Links target it and records name it:
	// snapshots are removed when the run ends, so anything durable must point at the origin (ruled
	// 2026-08-08). Equals Source when planning reads the origin directly.
	Origin string

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

	// MatchedDirs are the directories selected, each layer's in its order of application, layer after layer.
	MatchedDirs []segment.MatchResult

	// Collisions are files a later directory overrode.
	Collisions []Collision

	// Manifests are the package manifests every selected directory contributed, in the order directories are applied:
	// layer (base → team → personal), then the selector's order within the layer (#944). They are NOT in
	// [BuildResult.Files] and never collide (#814): a manifest lands nowhere in the filesystem, so the overlay rule --
	// one target path, one source -- has nothing to arbitrate. Two sets of package claims combine. The order is total,
	// so the merged claims are deterministic, which a graph's checksum requires.
	Manifests []*FileEntry
}

// Collision records a file present in two selected directories, and which one the build took: the one applied later.
type Collision struct {
	// Target is the relative target path that had a collision.
	Target string

	// Winner is the source that won: applied later, in a later layer or later in the selector's order.
	Winner string

	// WinnerDir is the directory the winner was found in: common.Ubuntu.
	WinnerDir string

	// WinnerLayer is the layer of the winner (empty for single-source mode).
	WinnerLayer string

	// Loser is the source that was overridden.
	Loser string

	// LoserDir is the directory the loser was found in: common.Linux.
	LoserDir string

	// LoserLayer is the layer of the loser (empty for single-source mode).
	LoserLayer string

	// WinnerRank is the winner's directory's place in its layer's order of application.
	WinnerRank selector.Rank

	// LoserRank is the loser's directory's place in its layer's order of application.
	LoserRank selector.Rank

	// Reason names what put the winner after the loser: a later layer, a later project, a more specific OS word, the
	// architecture, or an extra segment.
	Reason string
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

	// Projects to include, in the order they're applied: the implicit ones, then the recorded, then the named.
	Projects []string

	// Segments for platform matching.
	Segments segment.Segments
}

// isManifest reports whether an entry is a package manifest -- an entry whose whole pipeline is `manifest.resolve`,
// as [processingPipeline] assigns to `packages-manifest.yaml` and `.json`.
//
// Parameters:
//   - `entry`: the walked entry.
//
// Returns:
//   - `bool`: true when the entry is a manifest and must bypass collision resolution (#814).
func isManifest(entry *FileEntry) bool {

	return len(entry.Operations) == 1 && entry.Operations[0] == manifestResolveOperation
}

// fileEntryWithMeta tracks a file entry with the directory and layer it came from, for collision records.
type fileEntryWithMeta struct {
	entry *FileEntry
	dir   string        // the selected directory: common.Ubuntu
	layer string        // "base", "team", "personal", or "" for single-source mode
	rank  selector.Rank // the directory's place in its layer's order of application
}

// Build creates an execution graph from the given configuration.
// Supports both single-source mode (SourceRoot) and multi-source mode (Sources).
//
// Every layer's directories are selected first, and a directory name that breaks the selector grammar in any of them
// refuses the build before a file is read, with every such name listed (#944). Then the directories are applied in
// order, layer after layer (base → team → personal) and, within a layer, in the selector's order, and a file a later
// directory also holds is taken from the later one.
//
// Parameters:
//   - `cfg`: the build's configuration.
//
// Returns:
//   - `*BuildResult`: the files, the manifests, and the collisions.
//   - `error`: a [*segment.Refusal] listing every malformed directory name, or the error reading a layer.
func Build(cfg BuildConfig) (*BuildResult, error) {

	result := &BuildResult{TargetRoot: cfg.TargetRoot, Projects: cfg.Projects}

	sources := cfg.Sources
	if len(sources) == 0 {
		// Single-source mode reads the origin directly — one LayerSource with OriginRoot defaulting.
		sources = []LayerSource{{SourceRoot: cfg.SourceRoot, TargetRoot: cfg.TargetRoot}}
		result.SourceRoot = cfg.SourceRoot
	} else {
		result.Sources = cfg.Sources
	}

	selections, err := selectDirectories(sources, cfg)
	if err != nil {
		return nil, err
	}

	entriesByTarget := make(map[string]fileEntryWithMeta)
	extras := cfg.Segments.Extras()

	for i, source := range sources {
		result.MatchedDirs = append(result.MatchedDirs, selections[i]...)

		for _, match := range selections[i] {
			entries, err := walkDirectory(match, source)
			if err != nil {
				return nil, layerError(source, err)
			}

			result.apply(entriesByTarget, entries, match, source, extras)
		}
	}

	for _, meta := range entriesByTarget {
		result.Files = append(result.Files, meta.entry)
	}
	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].ID < result.Files[j].ID
	})

	return result, nil
}

// apply takes a selected directory's entries in, after every directory applied before it: a manifest joins the
// manifests, and a file a directory applied earlier also holds is taken from this one, the collision recorded.
//
// Parameters:
//   - `entriesByTarget`: the files taken so far, by target.
//   - `entries`: the directory's entries.
//   - `match`: the directory, with its rank.
//   - `source`: the layer the directory is in.
//   - `extras`: the extra segments, in configured order, which name the extras a rank records.
func (r *BuildResult) apply(entriesByTarget map[string]fileEntryWithMeta, entries []*FileEntry,
	match segment.MatchResult, source LayerSource, extras []selector.Segment) {

	taken := fileEntryWithMeta{dir: filepath.Base(match.Path), layer: source.Layer, rank: match.Rank}
	for _, entry := range entries {
		entry.Layer = source.Layer
		entry.TargetName = source.TargetName

		// Manifests combine; only files collide (#814).
		if isManifest(entry) {
			r.Manifests = append(r.Manifests, entry)
			continue
		}

		taken.entry = entry
		if existing, exists := entriesByTarget[entry.ID]; exists {
			r.Collisions = append(r.Collisions, Collision{
				Target:      entry.ID,
				Winner:      entry.Source,
				WinnerDir:   taken.dir,
				WinnerLayer: taken.layer,
				Loser:       existing.entry.Source,
				LoserDir:    existing.dir,
				LoserLayer:  existing.layer,
				WinnerRank:  taken.rank,
				LoserRank:   existing.rank,
				Reason:      collisionReason(taken, existing, extras),
			})
		}
		entriesByTarget[entry.ID] = taken
	}
}

// collisionReason names what put a collision's winner after its loser.
//
// Parameters:
//   - `winner`: the directory applied later.
//   - `loser`: the directory applied earlier.
//   - `extras`: the extra segments, in configured order.
//
// Returns:
//   - `string`: a later layer, a later project, a more specific OS word, the architecture, or an extra segment.
func collisionReason(winner, loser fileEntryWithMeta, extras []selector.Segment) string {

	switch {
	case winner.layer != loser.layer:
		return "a later layer"
	case winner.rank.Project != loser.rank.Project:
		return "a later project"
	case winner.rank.Link != loser.rank.Link:
		return "a more specific OS word"
	case winner.rank.Arch != loser.rank.Arch:
		return "it names the architecture"
	}
	for i := 0; i < len(winner.rank.Extras) && i < len(loser.rank.Extras) && i < len(extras); i++ {
		if winner.rank.Extras[i] != loser.rank.Extras[i] {
			return fmt.Sprintf("it names the %s segment", extras[i].Name)
		}
	}
	return "applied later"
}

// selectDirectories selects every layer's directories, and gathers every directory name that breaks the selector
// grammar in any of them.
//
// Parameters:
//   - `sources`: the layers, in order.
//   - `cfg`: the build's projects and segments.
//
// Returns:
//   - `[][]segment.MatchResult`: each layer's selected directories, in order of application.
//   - `error`: a [*segment.Refusal] naming every malformed directory name, or the error reading a layer.
func selectDirectories(sources []LayerSource, cfg BuildConfig) ([][]segment.MatchResult, error) {

	selections := make([][]segment.MatchResult, len(sources))
	refusal := &segment.Refusal{}

	for i, source := range sources {
		matches, grammarErrors, err := segment.MatchDirectories(source.SourceRoot, cfg.Projects, cfg.Segments)
		if err != nil {
			return nil, layerError(source, err)
		}
		selections[i] = matches

		root := source.OriginRoot
		if root == "" {
			root = source.SourceRoot
		}
		for _, grammarError := range grammarErrors {
			refusal.Entries = append(refusal.Entries, segment.RefusalEntry{Root: root, Err: grammarError})
		}
	}

	if len(refusal.Entries) > 0 {
		return nil, refusal
	}
	return selections, nil
}

// layerError names the layer an error came from, in multi-source mode.
//
// Parameters:
//   - `source`: the layer.
//   - `err`: the error.
//
// Returns:
//   - `error`: the error, prefixed with the layer's name when it has one.
func layerError(source LayerSource, err error) error {

	if source.Layer == "" {
		return err
	}
	return fmt.Errorf("layer %s: %w", source.Layer, err)
}

// walkDirectory walks a matched directory and returns file entries for all files, each carrying both the
// read path (Source, the pinned snapshot under layered deploys) and the durable origin path (Origin).
func walkDirectory(match segment.MatchResult, source LayerSource) ([]*FileEntry, error) {

	originRoot := source.OriginRoot
	if originRoot == "" {
		originRoot = source.SourceRoot
	}
	var entries []*FileEntry

	err := filepath.WalkDir(match.Path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		entry, err := fileEntryAt(match, source, originRoot, path, d.Name())
		if err != nil {
			return err
		}

		entries = append(entries, entry)
		return nil
	})

	return entries, err
}

// fileEntryAt builds one walked file's [FileEntry] — target and pipeline from the name, restricted mode for
// secrets, the Origin mapped onto `originRoot` — validating packages-manifest files as they surface.
func fileEntryAt(match segment.MatchResult, source LayerSource, originRoot, path, name string) (*FileEntry, error) {

	relPath, err := filepath.Rel(match.Path, path)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(relPath)
	targetName, actions := ProcessingPipeline(name)

	relTarget := targetName
	if dir != "." {
		relTarget = filepath.Join(dir, targetName)
	}

	// Secrets get restricted permissions
	var mode os.FileMode
	if hasAction(actions, "encryption.decrypt") {
		mode = 0o600
	}

	sourceRelative, err := filepath.Rel(source.SourceRoot, path)
	if err != nil {
		return nil, err
	}

	if hasAction(actions, "manifest.resolve") && manifest.IsManifestFile(name) {
		if err := manifest.Validate(path); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", relPath, err)
		}
	}

	return &FileEntry{
		ID:         relTarget,
		Operations: actions,
		Source:     path,
		Origin:     filepath.Join(originRoot, sourceRelative),
		Target:     filepath.Join(source.TargetRoot, relTarget),
		Project:    match.Project,
		Mode:       mode,
	}, nil
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
