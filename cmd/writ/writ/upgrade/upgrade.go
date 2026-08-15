// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package upgrade regenerates deployed copied files (templates, secrets, copies) from their current sources
// (phase-8 step 47 slice 2).
//
// Upgrade is a pure readback consumer: the copied inventory comes from the store fold (symlinks are never
// touched — a link already reflects its source). Each candidate is classified before anything runs:
//
//   - missing — the target is gone; regenerating is safe and needs no flag.
//   - up-to-date — the target's content equals a fresh in-process render/copy of the current source; nothing
//     to do.
//   - stale — the target is unchanged since deployment (its digest equals the run's recorded as-deployed
//     identity, step 48) and the source moved; regenerates freely.
//   - modified — the target was edited locally (digest differs from the recorded identity); skipped with a
//     warning, regenerates only under --force.
//   - differing / unverifiable — runs traced before the step-48 capture (or encrypted chains without a
//     cataloged source) cannot attribute; they skip and follow the --force rule.
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

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/encryption"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/template"
)

// Config carries the resolved settings for one upgrade operation.
type Config struct {

	// Projects filters the copied inventory; empty upgrades every project.
	Projects []string

	// Force regenerates locally-modified and indeterminate targets (without it they skip with a warning;
	// stale targets — source moved, target untouched per the recorded identity — regenerate freely).
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

// Execute runs the full upgrade operation.
//
// Fold the copied inventory, classify each entry, and regenerate what the classification allows.
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
	reportSkipped(skipped)

	if len(regenerate) == 0 {
		cli.Success("Nothing to regenerate.")
		return nil
	}

	graphs, err := buildScopeGraphs(ctx, cfg, regenerate, data)
	if err != nil {
		return err
	}

	if cfg.DryRun {
		return op.SerializeGraphs(os.Stdout, graphs)
	}

	regenerated, err := runAll(ctx, cfg, graphs)
	if err != nil {
		return err
	}

	if len(skipped) > 0 {
		cli.Success("%d file(s) regenerated, %d skipped", regenerated, len(skipped))
	} else {
		cli.Success("%d file(s) regenerated", regenerated)
	}

	return nil
}

// region HELPER FUNCTIONS

// reportSkipped notes the skipped targets and the --force hint; an empty slice notes nothing.
//
// Parameters:
//   - `skipped`: the skipped target paths from classification.
func reportSkipped(skipped []string) {

	if len(skipped) == 0 {
		return
	}

	cli.Note("Skipped %d file(s):", len(skipped))
	for _, target := range skipped {
		cli.Note("  %s", target)
	}
	cli.Note("Use --force to overwrite. (\"indeterminate\" entries predate the recorded content identity or are" +
		" encrypted without a cataloged source.)")
}

// buildScopeGraphs groups the entries by scope and assembles one regeneration graph per scope, in
// sorted scope order.
//
// Parameters:
//   - `ctx`: the planning context.
//   - `cfg`: the upgrade configuration.
//   - `regenerate`: the entries to regenerate.
//   - `data`: the render data for the chains.
//
// Returns:
//   - `[]*op.Graph`: one assembled graph per scope, in sorted scope order.
//   - `error`: non-nil when any scope's planning or assembly fails.
func buildScopeGraphs(
	ctx context.Context, cfg *Config, regenerate []readback.Entry, data map[string]any,
) ([]*op.Graph, error) {

	byScope := make(map[string][]readback.Entry)
	for i := range regenerate {
		entry := &regenerate[i]
		byScope[entry.Scope] = append(byScope[entry.Scope], *entry)
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
			return nil, err
		}
		graphs = append(graphs, graph)
	}

	return graphs, nil
}

// runAll executes every graph, collecting per-scope failures; the regenerated count accumulates even
// when scopes fail.
//
// Parameters:
//   - `ctx`: the execution context.
//   - `cfg`: the upgrade configuration.
//   - `graphs`: the assembled graphs, in run order.
//
// Returns:
//   - `regenerated`: the number of files regenerated across all scopes.
//   - `err`: the joined per-scope failures, or nil when every scope succeeds.
func runAll(ctx context.Context, cfg *Config, graphs []*op.Graph) (regenerated int, err error) {

	var failures []error

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
		return regenerated, fmt.Errorf("%d scope(s) failed: %w", len(failures), errors.Join(failures...))
	}

	return regenerated, nil
}

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
func classify(
	cfg *Config, copied []readback.Entry, data map[string]any,
) (regenerate []readback.Entry, skipped []string) {

	for i := range copied {

		entry := &copied[i]

		switch classifyEntry(*entry, data) {

		case classMissing:
			regenerate = append(regenerate, *entry)

		case classStale:
			if cfg.Verbose {
				cli.Note("%s: source changed, target unmodified — regenerating", entry.Target)
			}
			regenerate = append(regenerate, *entry)

		case classUpToDate:
			if cfg.Verbose {
				cli.Note("%s: up to date", entry.Target)
			}

		case classSourceGone:
			cli.Warn("%s: source %s no longer exists; skipping", entry.Target, entry.Source)

		case classModified:
			if cfg.Force {
				regenerate = append(regenerate, *entry)
			} else {
				skipped = append(skipped, entry.Target+" (locally modified)")
			}

		default: // classDiffering, classUnverifiable — indeterminate
			if cfg.Force {
				regenerate = append(regenerate, *entry)
			} else {
				skipped = append(skipped, entry.Target+" (indeterminate)")
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

	// classStale means the target is unchanged since deployment (its digest equals the recorded as-deployed
	// identity) and the source moved — regenerating is safe without --force (step 48 attribution).
	classStale

	// classModified means the target was edited locally after deployment (its digest differs from the
	// recorded identity); force-gated.
	classModified

	// classDiffering means the target differs from a fresh result and the run predates the step-48 recorded
	// identity — source change and local edits are indistinguishable; force-gated.
	classDiffering

	// classUnverifiable means the entry cannot be attributed (an encrypted chain without enough recorded
	// identity); force-gated.
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

	current, err := os.ReadFile(entry.Target)
	if err != nil {
		return classDiffering
	}

	// Local-edit attribution (step 48): the recorded as-deployed digest tells target-modified apart from
	// source-changed. Absent identity (a pre-capture run) leaves the pre-48 indeterminate behavior.
	targetUnchanged := false
	if entry.RecordedDigest != "" {
		if readback.ContentDigest(current) != entry.RecordedDigest {
			return classModified
		}
		targetUnchanged = true
	}

	_, operations := tree.ProcessingPipeline(filepath.Base(entry.Source))
	pipeline := strings.Join(operations, "+")

	var fresh []byte
	switch pipeline {
	case "template.render_bytes+file.copy":
		rendered, ok := renderedFresh(source, data)
		if !ok {
			return classUnverifiable
		}
		fresh = rendered
	case "file.link":
		// A source with no processing suffix deployed as a plain copy entry: compare bytes directly.
		fresh = source
	default:
		return classifyEncryptedChain(entry, source, targetUnchanged)
	}

	if bytes.Equal(current, fresh) {
		return classUpToDate
	}
	if targetUnchanged {
		return classStale
	}
	return classDiffering
}

// renderedFresh recomputes a templated entry's fresh content from its source and the render data.
//
// Parameters:
//   - `source`: the template source bytes.
//   - `data`: the render data.
//
// Returns:
//   - `[]byte`: the rendered content.
//   - `bool`: false when rendering fails (the entry is unverifiable).
func renderedFresh(source []byte, data map[string]any) ([]byte, bool) {

	provider := &template.Provider{}
	rendered, err := provider.RenderText(string(source), data)
	if err != nil {
		return nil, false
	}

	return []byte(rendered), true
}

// classifyEncryptedChain classifies an entry whose fresh result is not computable without decrypting.
//
// The ENCRYPTED source's bytes are hashable — when the run cataloged the source, source movement
// attributes without any decryption.
//
// Parameters:
//   - `entry`: the deployed entry under classification.
//   - `source`: the encrypted source bytes.
//   - `targetUnchanged`: whether the target matches its recorded as-deployed digest.
//
// Returns:
//   - `classification`: up-to-date, stale, or unverifiable.
func classifyEncryptedChain(entry readback.Entry, source []byte, targetUnchanged bool) classification {

	if targetUnchanged && entry.RecordedSourceDigest != "" {
		if readback.ContentDigest(source) == entry.RecordedSourceDigest {
			return classUpToDate
		}
		return classStale
	}

	return classUnverifiable
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
	for i := range entries {
		runRoot = deploy.CommonAncestor(runRoot, filepath.Dir(entries[i].Source))
	}

	return op.Plan(ctx, upgradeSpec(runRoot, cfg.DryRun), func(environment *op.RuntimeEnvironment) (*op.Graph, error) {

		provider := plan.NewProvider(environment)
		fileMetas := make(map[string]any, len(entries))

		for i := range entries {

			entry := &entries[i]
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

	runRoot := ""
	if value, ok := graph.Origin().Annotations().Get("run_root"); ok {
		runRoot = assert.Type[string]("run_root annotation", value)
	}

	spec := upgradeSpec(runRoot, cfg.DryRun)

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
		regenerated = byAction[string(file.WriteText)].Completed() +
			byAction[string(file.WriteBytes)].Completed() +
			byAction[string(file.Copy)].Completed() +
			byAction[string(encryption.DecryptSopsFile)].Completed()
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
func upgradeSpec(root string, dryRun bool) *op.RuntimeEnvironmentSpec {

	return op.NewRuntimeEnvironmentSpec("writ").
		WithStatus(cli.UI()).
		WithRoot(root, fsroot.ModeConfined).
		WithApplication(&application.Application{
			Name: "writ",
			// The regeneration set is classification-cleared (missing / stale / force-approved), so the runs
			// execute under replace at the write seam (phase-8 step 49): overwriting these targets is the point.
			Flags: map[string]any{"dry-run": dryRun, "conflict": op.ConflictReplace},
		})
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
	//nolint:gocritic // rangeValCopy: map values are unaddressable; the per-iteration copy is the read.
	for _, entry := range inventory.Entries {
		if entry.Action == string(file.Link) {
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
