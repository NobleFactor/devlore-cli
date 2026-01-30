// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/execution"
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
// The w parameter is kept for API compatibility but is not used.
func Execute(w io.Writer, graph *execution.Graph, analysis *MigrationAnalysis) error {
	_ = w // kept for API compatibility

	// Find rename nodes in the graph
	var renameNodes []*execution.Node
	for _, node := range graph.Nodes {
		for _, op := range node.Operations {
			if op == "rename" {
				renameNodes = append(renameNodes, node)
				break
			}
		}
	}

	if len(renameNodes) == 0 {
		cli.Note("No renames needed.")
		return nil
	}

	cli.Note("Migrating: %s -> writ (%d directory renames)", analysis.System, len(renameNodes))

	// Verify no target conflicts before starting
	for _, node := range renameNodes {
		if exists(node.Target) {
			return fmt.Errorf("target directory %q already exists; aborting", node.Target)
		}
	}

	// Perform renames
	var renames []Rename
	for _, node := range renameNodes {
		if err := os.Rename(node.Source, node.Target); err != nil {
			cli.Error("  %s -> %s", filepath.Base(node.Source), filepath.Base(node.Target))
			return fmt.Errorf("rename %s -> %s: %w", node.Source, node.Target, err)
		}
		cli.Success("  %s -> %s", filepath.Base(node.Source), filepath.Base(node.Target))
		renames = append(renames, Rename{From: node.Source, To: node.Target})
	}

	// Write marker file
	marker := MigratedMarker{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		System:    string(analysis.System),
		Renames:   renames,
	}
	markerPath := filepath.Join(analysis.SourceRoot, ".writ-migrated")
	data, err := yaml.Marshal(&marker)
	if err != nil {
		return fmt.Errorf("marshal marker: %w", err)
	}
	if err := os.WriteFile(markerPath, data, 0644); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}

	cli.Success("Wrote .writ-migrated marker.")
	cli.Note("Migration complete. Next steps:")
	cli.Note("  git add -A && git commit -m \"Migrate to writ naming conventions\"")
	if len(analysis.Projects) > 0 {
		cli.Note("  writ deploy %s", joinWords(analysis.Projects))
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
