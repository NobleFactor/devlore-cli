// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package adopt

import (
	"fmt"
	"os"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
)

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

	// Scope is the scope the item was inferred into (Home or System); the record names it.
	Scope string
}

// BuildGraph constructs the batch adopt graph for one scope group.
//
// Shape (ruled 2026-09-23, #931, superseding the gather of the writ-adopt design): a deduplicated `file.mkdir`
// pre-stage -- one node per unique destination directory -- followed by **one chain per item**, so every adopted
// file has its own `file.link` unit and the graph's `files` annotation can name it, as a deploy's does:
//
//	mkdir₁ … mkdir_k
//	per item: file.move → file.link      (the existing-destination guard runs before the graph, in RunBatches)
//
// The origin is a deployment record's: tool `writ`, the scope, `target_root`, and `files` keyed by each link
// unit's id with the target (the original location, now the link), the source (the project location, which is
// also what the run read), the action, the layer and the project. The fold reads an adoption as it reads a deploy.
//
// Failure follows the policies as defined: a failed adoption fails the run, the executor unwinds, and completed
// items compensate (links removed, moves reversed, created directories pruned).
//
// Parameters:
//   - `env`: the planning runtime environment; supplies the receiver registry for provider-method lookup.
//   - `cfg`: the adopt configuration; the layer and the project the record names.
//   - `targetRoot`: the scope's root, the annotation's `target_root`.
//   - `items`: the scope group's adoptions, destinations already derived.
//
// Returns:
//   - *op.Graph: the assembled batch graph.
//   - `error`: non-nil when planning any invocation or the assembly fails.
func BuildGraph(env *op.RuntimeEnvironment, cfg *Config, targetRoot string, items []Item) (*op.Graph, error) {

	planProvider := plan.NewProvider(env)

	var units []*op.Invocation
	seenDirs := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, dup := seenDirs[item.DestDir]; dup {
			continue
		}
		seenDirs[item.DestDir] = struct{}{}
		mkdir, err := planProvider.Plan(file.Mkdir, nil, map[string]any{
			"path": item.DestDir,
			"mode": os.FileMode(0o755),
			"user": "", "group": "",
		})
		if err != nil {
			return nil, fmt.Errorf("adopt.BuildGraph: plan file.mkdir: %w", err)
		}
		units = append(units, mkdir)
	}

	scope := ""
	fileMetas := make(map[string]any, len(items))
	for _, item := range items {
		scope = item.Scope
		chain, linkUnitID, err := planItemChain(env, planProvider, item)
		if err != nil {
			return nil, err
		}
		units = append(units, chain...)
		fileMetas[linkUnitID] = map[string]any{
			"target":    item.Source,
			"source":    item.DestPath,
			"read_from": item.DestPath,
			"project":   cfg.Project,
			"layer":     cfg.Layer,
			"action":    string(file.Link),
		}
	}

	origin := op.NewOriginBase("writ", scope, op.NewAnnotationMap(map[string]any{
		"target_root": targetRoot,
		"files":       fileMetas,
	}))

	graph, err := planProvider.AssembleDefinition(units, nil, nil, nil, nil, nil, origin)
	if err != nil {
		return nil, fmt.Errorf("adopt.BuildGraph: assemble: %w", err)
	}

	return graph, nil
}

// planItemChain plans one item's adoption: `file.move` then `file.link`.
//
// The existing-destination guard runs before the graph, in [RunBatches]: a `flow.choose` in the graph does not load
// back from the document (devlore-cli#939), and a record whose graph cannot be read is a finding, not a record.
//
// Parameters:
//   - `env`: the planning runtime environment, which claims the source.
//   - `planProvider`: the plan provider the chain registers into.
//   - `item`: the adoption.
//
// Returns:
//   - `[]*op.Invocation`: the move and the link, in order.
//   - `string`: the link unit's id, the `files` annotation's key.
//   - `error`: non-nil when claiming the source or planning either invocation fails.
func planItemChain(env *op.RuntimeEnvironment, planProvider *plan.Provider, item Item) ([]*op.Invocation, string, error) {

	source, err := file.DiscoverRegular(env, item.Source)
	if err != nil {
		return nil, "", fmt.Errorf("adopt.BuildGraph: claim source %s: %w", item.Source, err)
	}

	moveInvocation, err := planProvider.Plan(file.Move, nil, map[string]any{
		"source":           source,
		"destination_path": item.DestPath,
	})
	if err != nil {
		return nil, "", fmt.Errorf("adopt.BuildGraph: plan file.move: %w", err)
	}
	linkInvocation, err := planProvider.Plan(file.Link, nil, map[string]any{
		"source_path": item.DestPath,
		"target_path": item.Source,
	})
	if err != nil {
		return nil, "", fmt.Errorf("adopt.BuildGraph: plan file.link: %w", err)
	}

	return []*op.Invocation{moveInvocation, linkInvocation}, linkInvocation.Target.ID(), nil
}
