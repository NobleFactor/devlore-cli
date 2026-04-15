// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/document"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// MigratedMarker records what was done during execution.
type MigratedMarker struct {
	Timestamp string   `yaml:"timestamp"`
	System    string   `yaml:"system"`
	Renames   []Rename `yaml:"renames"`
}

// Rename records a single directory rename.
type Rename struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// Execute performs the directory renames specified in the execution graph.
//
// It writes progress to stderr using standard cli output functions.
//
// Parameters:
//   - graph: the execution graph containing rename nodes.
//   - analysis: the migration analysis with source root and system info.
//
// Returns:
//   - error: non-nil if any rename fails or a target directory already exists.
func Execute(graph *op.Graph, analysis *MigrationAnalysis) error {

	// Find rename nodes in the graph

	var renameNodes []*op.Node

	for _, node := range graph.Nodes() {
		if node.Receiver == "file.move" {
			renameNodes = append(renameNodes, node)
		}
	}

	if len(renameNodes) == 0 {
		cli.Note("No renames needed.")
		return nil
	}

	cli.Note("Migrating: %s -> writ (%d directory renames)", analysis.System, len(renameNodes))

	// Verify no target conflicts before starting
	for _, node := range renameNodes {
		target, err := node.RequireStringSlot("path")
		if err != nil {
			return fmt.Errorf("rename node %s: %w", node.ID, err)
		}
		if exists(target) {
			return fmt.Errorf("target directory %q already exists; aborting", target)
		}
	}

	// Perform renames
	fp := file.NewProvider(&op.ExecutionContext{ProgramName: "writ", Root: op.NewRootReaderWriter(analysis.SourceRoot), Registry: op.NewReceiverRegistry()})
	for _, node := range renameNodes {
		source, err := node.RequireStringSlot("source")
		if err != nil {
			return fmt.Errorf("rename node %s: %w", node.ID, err)
		}
		target, err := node.RequireStringSlot("path")
		if err != nil {
			return fmt.Errorf("rename node %s: %w", node.ID, err)
		}
		if _, _, err := fp.Move(&file.Resource{SourcePath: op.NewPath("", source)}, target); err != nil {
			cli.Error("  %s -> %s", filepath.Base(source), filepath.Base(target))
			return fmt.Errorf("rename %s -> %s: %w", source, target, err)
		}
		cli.Success("  %s -> %s", filepath.Base(source), filepath.Base(target))
	}

	// Write marker file
	if err := WriteMigratedMarker(analysis.SourceRoot, graph, analysis); err != nil {
		return err
	}

	cli.Success("Wrote .writ-migrated marker.")
	cli.Note("Migration complete. Next steps:")
	cli.Note("  git add -A && git commit -m \"Migrate to writ naming conventions\"")
	if analysis.Structure != nil && len(analysis.Structure.Groups) > 0 {
		cli.Note("  writ deploy %s", joinWords(analysis.Structure.Groups))
	}

	return nil
}

// WriteMigratedMarker writes the .writ-migrated marker file.
//
// Parameters:
//   - sourceRoot: the root directory where the marker file is written.
//   - graph: the execution graph containing completed rename nodes.
//   - analysis: the migration analysis with system metadata.
//
// Returns:
//   - error: non-nil if marshaling or writing the marker fails.
func WriteMigratedMarker(sourceRoot string, graph *op.Graph, analysis *MigrationAnalysis) error {

	var renames []Rename

	for _, node := range graph.Nodes() {
		if node.Receiver == "file.move" {
			source, _ := node.SlotByName("source").(string) //nolint:errcheck // zero value (empty) is acceptable
			target, _ := node.SlotByName("path").(string)   //nolint:errcheck // zero value (empty) is acceptable
			renames = append(renames, Rename{From: source, To: target})
		}
	}

	marker := MigratedMarker{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		System:    string(analysis.System),
		Renames:   renames,
	}
	markerPath := filepath.Join(sourceRoot, ".writ-migrated")
	return document.Write(markerPath, &marker)
}

// joinWords concatenates words with spaces.
//
// Parameters:
//   - words: the strings to join.
//
// Returns:
//   - string: the space-separated result.
func joinWords(words []string) string {

	result := ""

	for i, w := range words {
		if i > 0 {
			result += " "
		}
		result += w
	}

	return result
}
