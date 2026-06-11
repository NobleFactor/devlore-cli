// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/document"
	"github.com/NobleFactor/devlore-cli/pkg/op"
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
//   - analysis: the migration analysis with source fsroot and system info.
//
// Returns:
//   - error: non-nil if any rename fails or a target directory already exists.
func Execute(graph *op.Graph, analysis *MigrationAnalysis) error {

	// Find rename nodes in the graph

	var renameNodes []*op.Node

	for _, node := range graph.Nodes() {
		if actionName(node) == "file.move" {
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
		target, ok := op.ImmediateOf(node.Slots()["path"]).(string)
		if !ok || target == "" {
			return fmt.Errorf("rename node %s: path slot missing or not a string", node.ID())
		}
		if exists(target) {
			return fmt.Errorf("target directory %q already exists; aborting", target)
		}
	}

	// Perform renames. Phase 7: each Move routes through [Move] (in file_ops.go) so the call goes through the
	// binding-model path — single-node graph with VariableValue slot references, dispatched via
	// op.GraphExecutor.Run. The pre-Phase-7 nil-activation `fp.Move(nil, …)` is gone.
	for _, node := range renameNodes {
		source, ok := op.ImmediateOf(node.Slots()["source"]).(string)
		if !ok || source == "" {
			return fmt.Errorf("rename node %s: source slot missing or not a string", node.ID())
		}
		target, ok := op.ImmediateOf(node.Slots()["path"]).(string)
		if !ok || target == "" {
			return fmt.Errorf("rename node %s: path slot missing or not a string", node.ID())
		}
		if err := Move(context.Background(), analysis.SourceRoot, source, target); err != nil {
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
//   - sourceRoot: the fsroot directory where the marker file is written.
//   - graph: the execution graph containing completed rename nodes.
//   - analysis: the migration analysis with system metadata.
//
// Returns:
//   - error: non-nil if marshaling or writing the marker fails.
func WriteMigratedMarker(sourceRoot string, graph *op.Graph, analysis *MigrationAnalysis) error {

	var renames []Rename

	for _, node := range graph.Nodes() {
		if actionName(node) == "file.move" {
			source, _ := op.ImmediateOf(node.Slots()["source"]).(string) //nolint:errcheck // zero value (empty) is acceptable
			target, _ := op.ImmediateOf(node.Slots()["path"]).(string)   //nolint:errcheck // zero value (empty) is acceptable
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

// actionName returns the bound action's name, or empty string when no action is bound.
//
// Parameters:
//   - node: the node to read the action name from.
//
// Returns:
//   - string: the action name, or empty string.
func actionName(node *op.Node) string {

	action := node.Action()
	if action == nil {
		return ""
	}
	return action.Name()
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
