// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package deploy plans and executes writ deployments on the sealed graph model (phase-8 step 47 slice 1).
//
// A deployment walks the layered source tree (base → team → personal, segment/platform variants, cross-layer
// collision resolution — all owned by the tree package), plans one immutable graph per target scope through
// [plan.Provider] (file links, template render chains, sops decrypts, and manifest-resolved package units), and
// executes each graph under a confined root. The plan persists once via [cli.WriteGraph]; every run's trace
// persists via [cli.WriteTrace] win or lose; both writes append to the store's run index. The graph's origin
// annotations carry the writ metadata bag — including the per-unit file inventory the readback package folds.
package deploy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/lore/lore"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/snapshot"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// Config carries the resolved settings for one deploy operation.
type Config struct {

	// SourceRoot is the source directory for single-source mode; ignored when LayerSources is populated.
	SourceRoot string

	// TargetRoot is the target directory for single-source mode (e.g. $HOME).
	TargetRoot string

	// LayerSources are the layer sources for multi-source mode; each carries its own target scope and root.
	LayerSources []tree.LayerSource

	// Projects selects the projects to deploy (e.g. ["all", "noblefactor"]).
	Projects []string

	// Segments are the platform/custom segments for variant matching and template data.
	Segments segment.Segments

	// Vars are the user-configured template variables, merged over the builtin template data.
	Vars map[string]any

	// Conflict is the occupied-target policy (phase-8 step 49). The zero value is [op.ConflictStop] — the
	// ruled default: the pre-flight refuses foreign or locally-modified occupants (naming them and this flag)
	// while writ's own unmodified outputs are cleared for replacement, so redeploys flow. `skip` and `replace`
	// hand the per-target decision to the file provider's write seam.
	Conflict op.ConflictPolicy

	// ManifestPlanner resolves packages-manifest files into package units; nil skips manifest resolution
	// with a note.
	ManifestPlanner *lore.Planner

	// AllowDirty permits planning against layers with uncommitted changes.
	AllowDirty bool

	// DryRun serializes the planned graphs to stdout instead of executing them.
	DryRun bool

	// Verbose narrates planning context and per-run receipts via [cli.Note].
	Verbose bool
}

// Execute runs the full deploy operation: pin layers, build the per-scope graphs, and execute them.
//
// Multi-source mode pins every layer source to a git-worktree snapshot before planning (refusing dirty layers
// unless `AllowDirty`); the pin's commit hashes ride the graphs' origin annotations. Dry-run serializes the
// planned graphs to stdout and stops. Execution is fail-forward across scopes — System first, then Home — with
// each graph persisted via [cli.WriteGraph] before its run and each run's trace persisted via [cli.WriteTrace]
// win or lose.
//
// Parameters:
//   - `ctx`: the cancellation context for planning and execution.
//   - `cfg`: the resolved deploy configuration.
//
// Returns:
//   - `error`: non-nil when pinning or planning fails, or when one or more scopes fail to execute.
func Execute(ctx context.Context, cfg *Config) (err error) {

	pin := &PinInfo{}

	if len(cfg.LayerSources) > 0 {
		var cleanup func()
		pin, cleanup, err = pinLayers(cfg)
		if err != nil {
			return err
		}
		defer cleanup()
	}

	build, err := BuildGraphs(ctx, cfg, pin)
	if err != nil {
		return err
	}

	if len(build.Graphs) == 0 {
		cli.Note("No files to deploy")
		return nil
	}

	if cfg.Verbose {
		reportContext(cfg)
	}
	if len(build.Collisions) > 0 {
		reportCollisions(cfg, build.Collisions)
	}

	if cfg.DryRun {
		return op.SerializeGraphs(os.Stdout, build.Graphs)
	}

	sortGraphsByScope(build.Graphs)

	runPolicy, err := preflightConflicts(ctx, cfg, build.Graphs)
	if err != nil {
		return err
	}

	return runAll(ctx, cfg, build.Graphs, runPolicy)
}

// runAll executes every graph under the run policy, collecting per-scope failures.
//
// Parameters:
//   - `ctx`: the execution context.
//   - `cfg`: the deploy configuration.
//   - `graphs`: the assembled graphs, in run order.
//   - `runPolicy`: the conflict policy resolved by the preflight pass.
//
// Returns:
//   - `error`: the joined per-scope failures, or nil when every scope succeeds.
func runAll(ctx context.Context, cfg *Config, graphs []*op.Graph, runPolicy op.ConflictPolicy) error {

	var failures []error

	for _, graph := range graphs {
		if runErr := runGraph(ctx, cfg, graph, runPolicy); runErr != nil {
			scope := scopeLabel(graph)
			cli.Warn("scope %s failed: %v", scope, runErr)
			failures = append(failures, fmt.Errorf("scope %s: %w", scope, runErr))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d scope(s) failed: %w", len(failures), errors.Join(failures...))
	}

	return nil
}

// region HELPER FUNCTIONS

// pinLayers checks the layer sources for uncommitted changes and pins each to a git-worktree snapshot.
//
// Parameters:
//   - `cfg`: the deploy configuration; its LayerSources are rewritten onto the snapshot paths.
//
// Returns:
//   - `*PinInfo`: the pinned commit hashes and the dirty-layer names.
//   - `func()`: the snapshot cleanup, to be deferred by the caller.
//   - `error`: non-nil when the dirty check fails, dirty layers exist without AllowDirty, or pinning fails.
func pinLayers(cfg *Config) (*PinInfo, func(), error) {

	pin := &PinInfo{}

	dirty, err := snapshot.CheckClean(cfg.LayerSources)
	if err != nil {
		return nil, nil, fmt.Errorf("check dirty: %w", err)
	}

	if len(dirty) > 0 {
		if !cfg.AllowDirty {
			return nil, nil, fmt.Errorf(
				"layers have uncommitted changes: %v\nCommit your changes or use --allow-dirty to plan against HEAD",
				dirty)
		}
		cli.Warn("Planning against dirty layers (uncommitted changes): %v", dirty)
		pin.DirtyLayers = dirty
	}

	snapshots, cleanup, err := snapshot.PinAll(cfg.LayerSources)
	if err != nil {
		return nil, nil, fmt.Errorf("pin layers: %w", err)
	}

	cfg.LayerSources = snapshot.RewriteSources(cfg.LayerSources, snapshots)
	pin.CommitHashes = snapshot.Hashes(snapshots)

	if cfg.Verbose {
		for _, s := range snapshots {
			cli.Note("Pinned %s → %s (%s)", s.Layer, s.CommitHash[:12], s.WorktreePath)
		}
	}

	return pin, cleanup, nil
}

// runGraph executes one scope's graph: persist the plan, run, persist the trace, and report the summary.
//
// Parameters:
//   - `ctx`: the cancellation context for the run.
//   - `cfg`: the deploy configuration (dry-run flag, verbosity).
//   - `graph`: the scope graph to execute.
//   - `runPolicy`: the write-seam conflict policy the pre-flight resolved for this run.
//
// Returns:
//   - `error`: non-nil when the spec cannot be configured, the plan cannot persist, or the run fails.
func runGraph(ctx context.Context, cfg *Config, graph *op.Graph, runPolicy op.ConflictPolicy) error {

	spec, err := runSpec(graph, cfg.DryRun, runPolicy)
	if err != nil {
		return err
	}

	if _, err := cli.WriteGraph(graph); err != nil {
		return fmt.Errorf("persist graph: %w", err)
	}

	executor := op.NewGraphExecutor(graph, spec)
	_, runErr := executor.Run(ctx, nil)

	if trace := executor.Trace(); trace != nil {
		if receiptPath, writeErr := cli.WriteTrace(trace); writeErr != nil {
			cli.Warn("failed to write receipt: %v", writeErr)
		} else if cfg.Verbose {
			cli.Note("Receipt: %s", receiptPath)
		}

		if runErr == nil {
			if scope := graph.Origin().Scope(); scope != "" {
				cli.Success("Deployed %s [%s]", formatSummary(trace.Summarize(graph)), scope)
			} else {
				cli.Success("Deployed %s", formatSummary(trace.Summarize(graph)))
			}
		}
	}

	return runErr
}

// preflightConflicts gates the default conflict policy against the planned targets (phase-8 step 49, the
// layered-enforcement ruling).
//
// Under the default `stop`, every planned target that is occupied on disk is classified through the readback:
// writ's own unmodified outputs — a symlink resolving to its recorded source, or a file whose digest equals the
// run's recorded as-deployed identity — are cleared for replacement (redeploys flow); anything foreign or
// locally modified is a violation, and the deploy refuses listing them and naming the flag. A cleared run (or an
// explicit `skip` / `replace`) hands the resolved policy to the file provider's write seam, which enforces it
// per target. A missing run index reads as zero knowledge (every occupant is foreign) — first deploys onto a
// clean machine have no occupants, so nothing refuses.
//
// Parameters:
//   - `ctx`: the context for the readback fold.
//   - `cfg`: the deploy configuration (the flag policy).
//   - `graphs`: the planned scope graphs.
//
// Returns:
//   - `op.ConflictPolicy`: the policy the runs execute under (`replace` for a cleared default-stop run).
//   - `error`: the refusal when default-stop finds foreign or modified occupants, or a fold failure.
func preflightConflicts(ctx context.Context, cfg *Config, graphs []*op.Graph) (op.ConflictPolicy, error) {

	if cfg.Conflict != op.ConflictStop {
		return cfg.Conflict, nil
	}

	inventory, err := readback.Fold(ctx)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			inventory = &readback.Inventory{Entries: map[string]readback.Entry{}}
		} else {
			return op.ConflictStop, err
		}
	}

	var violations []string

	for _, graph := range graphs {
		for target := range plannedTargets(graph) {

			if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
				continue
			}

			if entry, known := inventory.Entries[target]; known && occupantIsOurs(entry) {
				continue
			}

			violations = append(violations, target)
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		return op.ConflictStop, fmt.Errorf(
			"refusing to deploy over %d occupied target(s) not recognized as writ's own unmodified output:\n  %s\n"+
				"use --conflict=replace to archive-and-overwrite them, or --conflict=skip to leave them",
			len(violations), strings.Join(violations, "\n  "))
	}

	// Every occupant is ours and unmodified: the run may replace them (the redeploy flow).
	return op.ConflictReplace, nil
}

// plannedTargets reads the target set from a scope graph's `files` origin annotation.
//
// Parameters:
//   - `graph`: the planned scope graph.
//
// Returns:
//   - `map[string]bool`: the absolute planned target paths.
func plannedTargets(graph *op.Graph) map[string]bool {

	targets := make(map[string]bool)

	value, ok := graph.Origin().Annotations().Get("files")
	if !ok {
		return targets
	}
	raw, ok := value.(map[string]any)
	if !ok {
		return targets
	}
	for _, entry := range raw {
		fields, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if target, ok := fields["target"].(string); ok && target != "" {
			targets[target] = true
		}
	}
	return targets
}

// occupantIsOurs reports whether an occupied target is writ's own unmodified output.
//
// A linked entry is ours when the on-disk symlink resolves to the entry's recorded source; a copied entry is
// ours when its content digest equals the run's recorded as-deployed identity (step 48). Entries without a
// recorded identity (pre-capture runs) are NOT cleared — indeterminate occupants stay policy-gated.
//
// Parameters:
//   - `entry`: the readback inventory entry for the occupied target.
//
// Returns:
//   - `bool`: true when the occupant is writ's own unmodified output.
func occupantIsOurs(entry readback.Entry) bool {

	if entry.Action == string(file.Link) {
		resolvedTarget, err := filepath.EvalSymlinks(entry.Target)
		if err != nil {
			return false
		}
		resolvedSource, err := filepath.EvalSymlinks(entry.Source)
		if err != nil {
			return false
		}
		return resolvedTarget == resolvedSource
	}

	if entry.RecordedDigest == "" {
		return false
	}
	current, err := os.ReadFile(entry.Target)
	if err != nil {
		return false
	}
	return readback.ContentDigest(current) == entry.RecordedDigest
}

// scopeLabel returns the graph's scope for reporting, or "default" when unscoped.
//
// Parameters:
//   - `graph`: the graph whose scope to label.
//
// Returns:
//   - `string`: the scope, or "default".
func scopeLabel(graph *op.Graph) string {

	if origin := graph.Origin(); origin != nil && origin.Scope() != "" {
		return origin.Scope()
	}
	return "default"
}

// scopeOrder defines the execution priority for target scopes: System first (root-confined), then Home.
var scopeOrder = map[string]int{
	"system": 0,
	"home":   1,
}

// sortGraphsByScope sorts graphs into deterministic execution order: system, then home, then unscoped.
//
// Parameters:
//   - `graphs`: the graphs to sort in place.
func sortGraphsByScope(graphs []*op.Graph) {

	sort.SliceStable(graphs, func(i, j int) bool {
		oi, ok := scopeOrder[graphs[i].Origin().Scope()]
		if !ok {
			oi = len(scopeOrder)
		}
		oj, ok := scopeOrder[graphs[j].Origin().Scope()]
		if !ok {
			oj = len(scopeOrder)
		}
		return oi < oj
	})
}

// endregion
