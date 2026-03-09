// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/cli"
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
// It writes progress to stderr using standard cli output functions.
func Execute(graph *op.Graph, analysis *MigrationAnalysis) error {
	// Find rename nodes in the graph
	var renameNodes []*op.Node
	for _, node := range graph.Nodes {
		if node.ActionName() == "file.move" {
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
	fp := &file.Provider{}
	op.InitProvider(fp, op.Context{Root: op.NewRootReaderWriter(analysis.SourceRoot)})
	for _, node := range renameNodes {
		source, err := node.RequireStringSlot("source")
		if err != nil {
			return fmt.Errorf("rename node %s: %w", node.ID, err)
		}
		target, err := node.RequireStringSlot("path")
		if err != nil {
			return fmt.Errorf("rename node %s: %w", node.ID, err)
		}
		if _, _, err := fp.Move(file.Resource{SourcePath: op.NewPath("", source)}, file.Resource{SourcePath: op.NewPath("", target)}); err != nil {
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
func WriteMigratedMarker(sourceRoot string, graph *op.Graph, analysis *MigrationAnalysis) error {
	var renames []Rename
	for _, node := range graph.Nodes {
		if node.ActionName() == "file.move" {
			source, _ := node.GetSlot("source").(string) //nolint:errcheck // zero value (empty) is acceptable
			target, _ := node.GetSlot("path").(string)   //nolint:errcheck // zero value (empty) is acceptable
			renames = append(renames, Rename{From: source, To: target})
		}
	}

	marker := MigratedMarker{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		System:    string(analysis.System),
		Renames:   renames,
	}
	markerPath := filepath.Join(sourceRoot, ".writ-migrated")
	data, err := yaml.Marshal(&marker)
	if err != nil {
		return fmt.Errorf("marshal marker: %w", err)
	}
	if err := os.WriteFile(markerPath, data, 0o600); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}
	return nil
}

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
