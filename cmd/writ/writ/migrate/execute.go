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
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
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

// Execute runs the assembled migration graph and writes the .writ-migrated marker.
//
// The graph executes as a graph — one [op.GraphExecutor.Run] over the whole assembly (the step-33 rewrite;
// formerly each file.move node was strip-mined into a fresh one-node graph and dispatched separately, so the
// assembled graph was never run). Before the run, the rename targets are conflict-checked: file.move would
// archive-and-overwrite an existing target, but a pre-existing target here means the tree is not in the expected
// pre-migration shape, so the run is refused before any node dispatches. Progress goes to stderr via the standard
// cli output functions.
//
// Parameters:
//   - `ctx`: the cancellation context for the run.
//   - `graph`: the assembled migration graph.
//   - `analysis`: the migration analysis; supplies the source root and system label.
//
// Returns:
//   - `*op.Trace`: the run's execution trace — the migration receipt, which the caller persists (nil when the
//     graph was empty). Non-nil even when the run failed, so a failed run's journal survives.
//   - `error`: non-nil on a conflict, an open-root failure, a run failure, or a marker-write failure.
func Execute(ctx context.Context, graph *op.Graph, analysis *MigrationAnalysis) (*op.Trace, error) {

	if len(graph.Nodes()) == 0 {
		cli.Note("No changes needed.")
		return nil, nil
	}

	renameNodes := filterNodesByAction(graph, file.Move)

	// Conflict check: refuse to start when a rename target already exists. The slots are plan-time immediates; the
	// read goes through the reporting helper, not the executor's resolution path.
	for _, node := range renameNodes {
		target := immediateString(node, "destination_path")
		if target == "" {
			return nil, fmt.Errorf("rename node %s: destination_path slot missing or not a string", node.ID())
		}
		if exists(target) {
			return nil, fmt.Errorf("target directory %q already exists; aborting", target)
		}
	}

	cli.Note("Migrating: %s -> writ (%d nodes, %d directory renames)",
		analysis.System, len(graph.Nodes()), len(renameNodes))

	root, err := fsroot.OpenConfined(analysis.SourceRoot)
	if err != nil {
		return nil, fmt.Errorf("open root %s: %w", analysis.SourceRoot, err)
	}

	executor := op.NewGraphExecutor(graph, op.NewRuntimeEnvironmentSpec("writ").WithStatus(cli.UI()).WithRoot(root))

	if _, err := executor.Run(ctx, nil); err != nil {
		return executor.Trace(), fmt.Errorf("migration run: %w", err)
	}

	for _, node := range renameNodes {
		cli.Success("  %s -> %s",
			filepath.Base(immediateString(node, "source_path")),
			filepath.Base(immediateString(node, "destination_path")))
	}

	if err := WriteMigratedMarker(analysis.SourceRoot, graph, analysis); err != nil {
		return executor.Trace(), err
	}

	cli.Success("Wrote .writ-migrated marker.")
	cli.Note("Migration complete. Next steps:")
	cli.Note("  git add -A && git commit -m \"Migrate to writ naming conventions\"")
	if analysis.Structure != nil && len(analysis.Structure.Groups) > 0 {
		cli.Note("  writ deploy %s", joinWords(analysis.Structure.Groups))
	}

	return executor.Trace(), nil
}

// WriteMigratedMarker writes the .writ-migrated marker file.
//
// The marker is the human-facing record of what moved — the one legitimate graph-slot inspection left in this
// package's execution path (reporting; the executor owns slot resolution for dispatch).
//
// Parameters:
//   - `sourceRoot`: the root directory where the marker file is written.
//   - `graph`: the executed migration graph; its file.move nodes supply the rename list.
//   - `analysis`: the migration analysis with system metadata.
//
// Returns:
//   - `error`: non-nil if writing the marker fails.
func WriteMigratedMarker(sourceRoot string, graph *op.Graph, analysis *MigrationAnalysis) error {

	var renames []Rename

	for _, node := range filterNodesByAction(graph, file.Move) {
		renames = append(renames, Rename{
			From: immediateString(node, "source_path"),
			To:   immediateString(node, "destination_path"),
		})
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
//   - `words`: the strings to join.
//
// Returns:
//   - `string`: the space-separated result.
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
