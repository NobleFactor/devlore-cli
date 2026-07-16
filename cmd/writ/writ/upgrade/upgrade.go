// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package upgrade regenerates deployed copied files (templates, secrets, copies) from their current sources
// (phase-8 step 47 slice 2).
//
// Upgrade is a pure readback consumer: the copied inventory comes from the store fold (symlinks are never
// touched — a link already reflects its source). Each candidate is classified before anything runs:
//
//   - missing — the target is gone; regenerating is safe and needs no flag.
//   - up-to-date — the target's content equals a fresh in-process render/copy of the current source; nothing
//     to do.
//   - differing — the target differs from the fresh result. Until step 48 records as-deployed content
//     identity, source-changed cannot be distinguished from target-modified, so differing entries are skipped
//     with a warning and regenerate only under --force (the conservative interim the family design settled).
//   - unverifiable — sops-encrypted entries cannot be compared without decrypting outside the graph; they
//     follow the differing rule.
//
// Regeneration re-plans the same chains deploy plans ([deploy.PlanFileChain]) into one graph per scope; plans
// and traces persist to the store, so the fold reflects the regeneration.
package upgrade

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/template"
)

// Config carries the resolved settings for one upgrade operation.
type Config struct {

	// Projects filters the copied inventory; empty upgrades every project.
	Projects []string

	// Force regenerates differing and unverifiable targets (without it they skip with a warning — the
	// conservative interim until step 48 lands attribution).
	Force bool

	// Segments are the platform/custom segments for the render data.
	Segments segment.Segments

	// Vars are the user-configured template variables, merged over the builtin render data.
	Vars map[string]any

	// DryRun serializes the planned regeneration graphs to stdout instead of executing them.
	DryRun bool

	// Verbose narrates classifications and per-run receipts via [cli.Note].
	Verbose bool
}

// Execute runs the full upgrade operation: fold the copied inventory, classify each entry, and regenerate what
// the classification allows.
//
// Parameters:
//   - `ctx`: the cancellation context for the fold, planning, and execution.
//   - `cfg`: the resolved upgrade configuration.
//
// Returns:
//   - `error`: non-nil when the fold fails, planning fails, or a regeneration run fails.
func Execute(ctx context.Context, cfg *Config) (err error) {

	inventory, err := readback.Fold(ctx)
	if err != nil {
		return err
	}

	copied := selectCopied(inventory, cfg.Projects)
	if len(copied) == 0 {
		cli.Note("No copied files to upgrade.")
		return nil
	}

	data := deploy.RenderData(cfg.Segments, cfg.Vars)

	regenerate, skipped := classify(cfg, copied, data)

	if len(skipped) > 0 {
		cli.Note("Skipped %d differing or unverifiable file(s) (local modification cannot be ruled out):", len(skipped))
		for _, target := range skipped {
			cli.Note("  %s", target)
		}
		cli.Note("Use --force to overwrite; full attribution arrives with the recorded content identity (step 48).")
	}

	if len(regenerate) == 0 {
		cli.Success("Nothing to regenerate.")
		return nil
	}

	byScope := make(map[string][]readback.Entry)
	for _, entry := range regenerate {
		byScope[entry.Scope] = append(byScope[entry.Scope], entry)
	}

	scopes := make([]string, 0, len(byScope))
	for scope := range byScope {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)

	var graphs []*op.Graph
	for _, scope := range scopes {
		graph, err := buildScopeGraph(ctx, cfg, scope, byScope[scope], data)
		if err != nil {
			return err
		}
		graphs = append(graphs, graph)
	}

	if cfg.DryRun {
		encoder := yaml.NewEncoder(os.Stdout)
		defer func() {
			if closeErr := encoder.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}()
		encoder.SetIndent(2)
		for _, graph := range graphs {
			if err = graph.Serialize(encoder); err != nil {
				return err
			}
		}
		return nil
	}

	var failures []error
	regenerated := 0
	for _, graph := range graphs {
		count, runErr := runGraph(ctx, cfg, graph)
		regenerated += count
		if runErr != nil {
			scope := graph.Origin().Scope()
			if scope == "" {
				scope = "default"
			}
			cli.Warn("upgrade scope %s failed: %v", scope, runErr)
			failures = append(failures, fmt.Errorf("scope %s: %w", scope, runErr))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d scope(s) failed: %w", len(failures), errors.Join(failures...))
	}

	if len(skipped) > 0 {
		cli.Success("%d file(s) regenerated, %d skipped", regenerated, len(skipped))
	} else {
		cli.Success("%d file(s) regenerated", regenerated)
	}

	return nil
}

// region HELPER FUNCTIONS

// classify partitions the copied entries into regeneration candidates and force-gated skips.
//
// Parameters:
//   - `cfg`: the upgrade configuration (force flag, verbosity).
//   - `copied`: the copied inventory entries.
//   - `data`: the render data for fresh-content comparison.
//
// Returns:
//   - `[]readback.Entry`: the entries to regenerate.
//   - `[]string`: the skipped targets (differing or unverifiable, without --force).
func classify(cfg *Config, copied []readback.Entry, data map[string]any) ([]readback.Entry, []string) {

	var regenerate []readback.Entry
	var skipped []string

	for _, entry := range copied {

		switch classifyEntry(entry, data) {

		case classMissing:
			regenerate = append(regenerate, entry)

		case classUpToDate:
			if cfg.Verbose {
				cli.Note("%s: up to date", entry.Target)
			}

		case classSourceGone:
			cli.Warn("%s: source %s no longer exists; skipping", entry.Target, entry.Source)

		default: // classDiffering, classUnverifiable
			if cfg.Force {
				regenerate = append(regenerate, entry)
			} else {
				skipped = append(skipped, entry.Target)
			}
		}
	}

	sort.Slice(regenerate, func(i, j int) bool { return regenerate[i].Target < regenerate[j].Target })
	sort.Strings(skipped)

	return regenerate, skipped
}

// classification is the per-entry upgrade decision.
type classification int

const (
	// classUpToDate means the target equals a fresh result; nothing to do.
	classUpToDate classification = iota

	// classMissing means the target is gone; regenerating is safe without --force.
	classMissing

	// classDiffering means the target differs from a fresh result; force-gated until step 48 attributes.
	classDiffering

	// classUnverifiable means the entry cannot be compared without decrypting (sops); force-gated.
	classUnverifiable

	// classSourceGone means the source no longer exists; the entry cannot regenerate at all.
	classSourceGone
)

// classifyEntry compares one copied entry's target against a fresh in-process result of its current source.
//
// Template chains render through the same [template.Provider] the graph uses (a pure function — including the
// render-time `Env` lookup); plain copies compare bytes; sops chains are unverifiable without decrypting
// outside the graph.
//
// Parameters:
//   - `entry`: the copied inventory entry.
//   - `data`: the render data for template comparison.
//
// Returns:
//   - `classification`: the upgrade decision for this entry.
func classifyEntry(entry readback.Entry, data map[string]any) classification {

	if _, err := os.Lstat(entry.Target); errors.Is(err, os.ErrNotExist) {
		if _, srcErr := os.Lstat(entry.Source); errors.Is(srcErr, os.ErrNotExist) {
			return classSourceGone
		}
		return classMissing
	}

	source, err := os.ReadFile(entry.Source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return classSourceGone
		}
		return classUnverifiable
	}

	_, operations := tree.ProcessingPipeline(filepath.Base(entry.Source))
	pipeline := strings.Join(operations, "+")

	var fresh []byte
	switch pipeline {
	case "template.render_bytes+file.copy":
		provider := &template.Provider{}
		rendered, err := provider.RenderText(string(source), data)
		if err != nil {
			return classUnverifiable
		}
		fresh = []byte(rendered)
	case "file.link":
		// A source with no processing suffix deployed as a plain copy entry: compare bytes directly.
		fresh = source
	default:
		// Sops-encrypted chains cannot be compared without decrypting outside the graph.
		return classUnverifiable
	}

	current, err := os.ReadFile(entry.Target)
	if err != nil {
		return classDiffering
	}

	if bytes.Equal(current, fresh) {
		return classUpToDate
	}
	return classDiffering
}

// buildScopeGraph re-plans the regeneration chains for one scope's entries.
//
// Parameters:
//   - `ctx`: the planning context.
//   - `cfg`: the upgrade configuration.
//   - `scope`: the target scope.
//   - `entries`: the scope's entries to regenerate.
//   - `data`: the render data for the chains.
//
// Returns:
//   - `*op.Graph`: the assembled regeneration graph.
//   - `error`: non-nil when planning or assembly fails.
func buildScopeGraph(
	ctx context.Context, cfg *Config, scope string, entries []readback.Entry, data map[string]any,
) (*op.Graph, error) {

	runRoot := entries[0].TargetRoot
	if runRoot == "" {
		return nil, fmt.Errorf("scope %q: deployed entries carry no target root; cannot confine the upgrade", scope)
	}
	targetRoot := runRoot
	for _, entry := range entries {
		runRoot = deploy.CommonAncestor(runRoot, filepath.Dir(entry.Source))
	}

	spec, err := upgradeSpec(runRoot, cfg.DryRun)
	if err != nil {
		return nil, err
	}

	return op.Plan(ctx, spec, func(env *op.RuntimeEnvironment) (*op.Graph, error) {

		provider := plan.NewProvider(env)
		fileMetas := make(map[string]any, len(entries))

		for _, entry := range entries {

			_, operations := tree.ProcessingPipeline(filepath.Base(entry.Source))

			finalInvocation, action, err := deploy.PlanFileChain(provider, &tree.FileEntry{
				ID:         entry.Target,
				Operations: operations,
				Source:     entry.Source,
				Target:     entry.Target,
				Project:    entry.Project,
				Layer:      entry.Layer,
			}, data)
			if err != nil {
				return nil, fmt.Errorf("plan %s: %w", entry.Target, err)
			}

			fileMetas[finalInvocation.Target.ID()] = map[string]any{
				"target":  entry.Target,
				"source":  entry.Source,
				"project": entry.Project,
				"layer":   entry.Layer,
				"action":  action,
			}
		}

		var units []op.ExecutableUnit
		for _, invocation := range provider.InvocationRegistry().All() {
			if invocation.Target.ParentID() == "" {
				units = append(units, invocation.Target)
			}
		}

		origin := op.NewOriginBase("writ", scope, op.NewAnnotationMap(map[string]any{
			"target_root": targetRoot,
			"run_root":    runRoot,
			"projects":    cfg.Projects,
			"files":       fileMetas,
		}))

		graph, err := op.NewGraph(op.NewGraphSpec().WithOrigin(origin).WithUnits(units...))
		if err != nil {
			return nil, fmt.Errorf("assemble scope %q: %w", scope, err)
		}
		return graph, nil
	})
}

// runGraph executes one scope's regeneration graph and reports how many files landed.
//
// Parameters:
//   - `ctx`: the cancellation context for the run.
//   - `cfg`: the upgrade configuration (dry-run flag, verbosity).
//   - `graph`: the scope graph to execute.
//
// Returns:
//   - `int`: the number of regenerated files (target-producing completions).
//   - `error`: non-nil when the spec cannot be configured, the plan cannot persist, or the run fails.
func runGraph(ctx context.Context, cfg *Config, graph *op.Graph) (int, error) {

	value, _ := graph.Origin().Annotations().Get("run_root")
	runRoot, _ := value.(string)

	spec, err := upgradeSpec(runRoot, cfg.DryRun)
	if err != nil {
		return 0, err
	}

	if _, err := cli.WriteGraph(graph); err != nil {
		return 0, fmt.Errorf("persist graph: %w", err)
	}

	executor := op.NewGraphExecutor(graph, spec)
	_, runErr := executor.Run(ctx, nil)

	regenerated := 0
	if trace := executor.Trace(); trace != nil {
		if receiptPath, writeErr := cli.WriteTrace(trace); writeErr != nil {
			cli.Warn("failed to write receipt: %v", writeErr)
		} else if cfg.Verbose {
			cli.Note("Receipt: %s", receiptPath)
		}

		byAction := trace.Summarize(graph).ByAction()
		regenerated = byAction["file.write_text"].Completed() +
			byAction["file.write_bytes"].Completed() +
			byAction["file.copy"].Completed() +
			byAction["encryption.decrypt_sops_file"].Completed()
	}

	return regenerated, runErr
}

// upgradeSpec constructs a fresh [op.RuntimeEnvironmentSpec] confined at `root` for one phase of an upgrade.
//
// Parameters:
//   - `root`: the deepest common ancestor of the scope's target root and every source (regeneration reads
//     sources and writes targets).
//   - `dryRun`: forwarded to the application flag map.
//
// Returns:
//   - `*op.RuntimeEnvironmentSpec`: the constructed spec.
//   - `error`: non-nil when [fsroot.OpenConfined] fails.
func upgradeSpec(root string, dryRun bool) (*op.RuntimeEnvironmentSpec, error) {

	confined, err := fsroot.OpenConfined(root)
	if err != nil {
		return nil, fmt.Errorf("open root %s: %w", root, err)
	}

	return op.NewRuntimeEnvironmentSpec("writ").
		WithRoot(confined).
		WithApplication(&application.Application{
			Name:  "writ",
			Flags: map[string]any{"dry-run": dryRun},
		}), nil
}

// selectCopied filters the folded inventory to copied (non-link) entries, optionally by project.
//
// Parameters:
//   - `inventory`: the readback fold.
//   - `projects`: the projects to include; empty includes all.
//
// Returns:
//   - `[]readback.Entry`: the copied entries, unordered.
func selectCopied(inventory *readback.Inventory, projects []string) []readback.Entry {

	wanted := make(map[string]bool, len(projects))
	for _, p := range projects {
		wanted[p] = true
	}

	var copied []readback.Entry
	for _, entry := range inventory.Entries {
		if entry.Action == "file.link" {
			continue
		}
		if len(wanted) > 0 && !wanted[entry.Project] {
			continue
		}
		copied = append(copied, entry)
	}
	return copied
}

// endregion
