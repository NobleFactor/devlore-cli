// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package adopt

import (
	"fmt"
	"os"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/flow"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
)

// gatherLimit bounds the adopt gather's per-iteration concurrency.
const gatherLimit = 4

// Item describes one file adoption: the source location and its plan-time-derived destinations.
//
// The inputs to `writ adopt` are the locations of the files to adopt; the tool derives each location's destination
// path and directory at plan time and feeds the batch to [BuildGraph] (the writ-adopt design,
// docs/plans/extract-starlark-from-op/phase-8/writ-adopt-command.md).
type Item struct {

	// Source is the absolute path of the file being adopted (the live location; the symlink lands here).
	Source string

	// RelPath is Source relative to the scope's target root — display/reporting only.
	RelPath string

	// DestDir is the destination directory (the parent of DestPath), created by the mkdir pre-stage.
	DestDir string

	// DestPath is the destination inside `<layer>/<scope>/<project>/`, preserving RelPath.
	DestPath string
}

// BuildGraph constructs the batch adopt graph for one scope group (phase-8 step 33 slice A).
//
// Shape (the settled gather + field-projection design): a deduplicated `file.mkdir` pre-stage — one node per unique
// destination directory, ahead of the gather because concurrent per-item creation of a shared directory would be
// same-resource production — followed by one `flow.gather` over the item records. Each iteration runs the in-graph
// destination guard and the adoption chain, all slots projected from the iteration item ([plan.Provider.Item]):
//
//	gather  items=[{source, dest_path}, …]  limit=4
//	└── choose( file.exists(dest_path) → flow.failed | default: file.move → file.link )
//
// Failure follows the policies as defined: a failed adoption fails the run, the executor unwinds, and completed
// iterations compensate (links removed, moves reversed, created directories pruned).
//
// Parameters:
//   - `env`: the planning runtime environment; supplies the receiver registry for provider-method lookup.
//   - `items`: the scope group's adoptions, destinations already derived.
//
// Returns:
//   - *op.Graph: the assembled batch graph.
//   - `error`: non-nil when planning any invocation or the assembly fails.
func BuildGraph(env *op.RuntimeEnvironment, items []Item) (*op.Graph, error) {

	planProvider := plan.NewProvider(env)

	// The deduplicated mkdir pre-stage.
	var invocations []*op.Invocation
	seenDirs := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, dup := seenDirs[item.DestDir]; dup {
			continue
		}
		seenDirs[item.DestDir] = struct{}{}

		mkdir, err := planProvider.Plan(file.Mkdir, nil, map[string]any{
			"path":  item.DestDir,
			"chmod": os.FileMode(0o755),
			"chown": "",
		})
		if err != nil {
			return nil, fmt.Errorf("adopt.BuildGraph: plan file.mkdir: %w", err)
		}
		invocations = append(invocations, mkdir)
	}

	// The gather items: one record per adoption.
	records := make([]any, 0, len(items))
	for _, item := range items {
		records = append(records, map[string]any{
			"source":    item.Source,
			"dest_path": item.DestPath,
		})
	}

	// The per-iteration body: the in-graph destination guard, then the move → link chain.
	existsInvocation, err := planProvider.Plan(file.Exists, nil, map[string]any{
		"path": planProvider.Item("dest_path"),
	})
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: plan file.exists: %w", err)
	}

	failedInvocation, err := planProvider.Plan(flow.Failed,
		[]any{"adopt: destination already exists: {{ .dest }}"},
		map[string]any{"dest": planProvider.Item("dest_path")})
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: plan flow.failed: %w", err)
	}

	moveInvocation, err := planProvider.Plan(file.Move, nil, map[string]any{
		"source_path":      planProvider.Item("source"),
		"destination_path": planProvider.Item("dest_path"),
	})
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: plan file.move: %w", err)
	}

	linkInvocation, err := planProvider.Plan(file.Link, nil, map[string]any{
		"source_path": planProvider.Item("dest_path"),
		"target_path": planProvider.Item("source"),
	})
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: plan file.link: %w", err)
	}

	guardCase, err := planProvider.Case(existsInvocation, failedInvocation)
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: plan the guard case: %w", err)
	}

	chooseInvocation, err := planProvider.Plan(flow.Choose,
		[]any{guardCase},
		map[string]any{"default": []any{moveInvocation, linkInvocation}})
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: plan flow.choose: %w", err)
	}

	gatherInvocation, err := planProvider.Plan(flow.Gather, nil, map[string]any{
		"items": records,
		"limit": gatherLimit,
		"body":  []any{chooseInvocation},
	})
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: plan flow.gather: %w", err)
	}

	graph, err := planProvider.AssembleDefinition(
		append(invocations, gatherInvocation),
		nil, nil, nil, nil, nil,
		planProvider.Origin("adopt"),
	)
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: assemble: %w", err)
	}

	return graph, nil
}
