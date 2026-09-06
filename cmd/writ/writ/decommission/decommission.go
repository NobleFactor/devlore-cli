// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package decommission removes deployed files through the sealed graph model (phase-8 step 47 slice 2).
//
// Decommission is a pure readback consumer: the deployed inventory comes from the store fold — never from a
// directory scan — so it removes exactly what writ's runs put into effect. Symlinked entries plan `file.unlink`
// (which refuses a non-symlink squatter, so a target the user replaced is never deleted); copied entries plan
// `file.remove`. One graph per scope, System first, fail-forward; each run's plan and trace persist to the
// store with the removal inventory in the origin annotations, so the next fold clears the removed targets.
// The signed/unsigned safety gate from the old surface arrives with step 46 (nothing signs graphs today).
package decommission

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
)

// Config carries the resolved settings for one decommission operation.
type Config struct {

	// Projects selects the projects whose deployed files are removed; empty removes nothing (the command
	// requires at least one project).
	Projects []string

	// Prune removes now-empty parent directories after file removal, bounded at each scope's target root.
	Prune bool

	// DryRun serializes the planned removal graphs to stdout instead of executing them.
	DryRun bool

	// Verbose narrates per-run receipts via [cli.Note].
	Verbose bool
}

// Execute runs the full decommission operation.
//
// Fold the deployed inventory, plan one removal graph per scope, and execute them.
//
// The inventory is the store readback (a missing run index is an error; zero deployed knowledge for the
// selected projects is a refusal — decommission never guesses from a directory scan). Execution is
// fail-forward across scopes, System first; each graph persists via [cli.WriteGraph] before its run and each
// run's trace persists via [cli.WriteTrace] win or lose.
//
// Parameters:
//   - `ctx`: the cancellation context for the fold, planning, and execution.
//   - `cfg`: the resolved decommission configuration.
//
// Returns:
//   - `graphs`: the plan, under --dry-run; nil when the run executed (or there was nothing to do).
//   - `err`: non-nil when the fold fails, nothing is deployed for the selection, planning fails, or one or
//     more scopes fail to execute.
func Execute(ctx context.Context, cfg *Config) (graphs []*op.Graph, err error) {

	inventory, err := readback.Fold(ctx)
	if err != nil {
		return nil, err
	}

	selected := selectEntries(inventory, cfg.Projects)
	if len(selected) == 0 {
		return nil, fmt.Errorf(
			"no deployed files found for projects %v; cannot decommission without deployment history", cfg.Projects)
	}

	graphs, err = buildScopeGraphs(ctx, cfg, selected)
	if err != nil {
		return nil, err
	}

	if cfg.DryRun {
		return graphs, nil
	}

	return nil, runAll(ctx, cfg, graphs)
}

// buildScopeGraphs groups the entries by scope and assembles one removal graph per scope, in removal
// priority order.
//
// Parameters:
//   - `ctx`: the planning context.
//   - `cfg`: the decommission configuration.
//   - `selected`: the deployed entries selected for removal.
//
// Returns:
//   - `[]*op.Graph`: one assembled graph per scope, in removal order.
//   - `error`: non-nil when any scope's planning or assembly fails.
func buildScopeGraphs(ctx context.Context, cfg *Config, selected []readback.Entry) ([]*op.Graph, error) {

	byScope := make(map[string][]readback.Entry)
	for i := range selected {
		entry := &selected[i]
		byScope[entry.Scope] = append(byScope[entry.Scope], *entry)
	}

	var graphs []*op.Graph
	for _, scope := range scopesInOrder(byScope) {
		graph, err := buildScopeGraph(ctx, cfg, scope, byScope[scope])
		if err != nil {
			return nil, err
		}
		graphs = append(graphs, graph)
	}

	return graphs, nil
}

// runAll executes every graph, collecting per-scope failures.
//
// Parameters:
//   - `ctx`: the execution context.
//   - `cfg`: the decommission configuration.
//   - `graphs`: the assembled graphs, in removal order.
//
// Returns:
//   - `error`: the joined per-scope failures, or nil when every scope succeeds.
func runAll(ctx context.Context, cfg *Config, graphs []*op.Graph) error {

	var failures []error

	for _, graph := range graphs {
		if runErr := runGraph(ctx, cfg, graph); runErr != nil {
			scope := graph.Origin().Scope()
			if scope == "" {
				scope = "default"
			}
			cli.Warn("decommission scope %s failed: %v", scope, runErr)
			failures = append(failures, fmt.Errorf("scope %s: %w", scope, runErr))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d scope(s) failed: %w", len(failures), errors.Join(failures...))
	}

	return nil
}

// region HELPER FUNCTIONS

// buildScopeGraph plans one scope's removal graph: `file.unlink` per linked entry, `file.remove` per copied
// entry, in deterministic target order.
//
// Parameters:
//   - `ctx`: the planning context.
//   - `cfg`: the decommission configuration (prune flag).
//   - `scope`: the target scope ("system" / "home", or "" for unscoped runs).
//   - `entries`: the scope's deployed entries to remove.
//
// Returns:
//   - `*op.Graph`: the assembled removal graph.
//   - `error`: non-nil when an entry carries no target root, or planning or assembly fails.
func buildScopeGraph(
	ctx context.Context, cfg *Config, scope string, entries []readback.Entry,
) (*op.Graph, error) {

	targetRoot := entries[0].TargetRoot
	if targetRoot == "" {
		return nil, fmt.Errorf("scope %q: deployed entries carry no target root; cannot confine the removal", scope)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Target < entries[j].Target })

	return op.Plan(ctx, removalSpec(targetRoot, cfg.DryRun), func(environment *op.RuntimeEnvironment) (*op.Graph, error) {

		provider := plan.NewProvider(environment)
		fileMetas := make(map[string]any, len(entries))

		for i := range entries {

			entry := &entries[i]

			// One action for every kind: file.remove takes the taxonomy interface, so a deployed
			// symbolic link and a copied regular file decommission through the same node — and a link
			// is removed as the link, never followed to what it designates (2026-08-23).
			action := file.Remove

			// The target is a consumed, claimed resource in plan space; on_missing=ignore is the ruled
			// decommission posture — a target the user removed by hand decommissions as a recorded no-op
			// instead of failing the plan (§3, the claims taxonomy).
			rel, err := deploy.PlanSpacePath(environment, entry.Target)
			if err != nil {
				return nil, err
			}

			// **The claim asserts the kind writ deployed**, which is what makes tolerance safe. Ignore
			// forgives a target the user removed; it must NOT forgive one the user REPLACED, because a
			// deployed link standing where a hand-written file now sits means the goal does not hold and
			// something unexpected is there. A kinded claim turns that into a mismatch, which stops under
			// every policy ([op.ErrClaimKindMismatch]); an unasserted claim would admit the replacement
			// and delete it.
			target, err := claimDeployed(environment, entry.Action, rel)
			if err != nil {
				return nil, fmt.Errorf("claim %s: %w", entry.Target, err)
			}

			invocation, err := provider.Plan(action, nil, map[string]any{
				"target":     target,
				"on_missing": "ignore",
				"prune":      cfg.Prune,
				"boundary":   targetRoot,
			})
			if err != nil {
				return nil, fmt.Errorf("plan %s %s: %w", action, entry.Target, err)
			}

			fileMetas[invocation.Target.ID()] = map[string]any{
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
			"run_root":    targetRoot,
			"projects":    cfg.Projects,
			"files":       fileMetas,
		}))

		// The catalog travels with the graph (§5.4): the planning session interned this graph's claims
		// into the environment's catalog — a fresh one here would strand them (the empty-section defect
		// scoped verification exposed, 2026-08-22).
		graph, err := op.NewGraph(op.NewGraphSpec().WithOrigin(origin).WithUnits(units...).
			WithResourceCatalog(environment.ResourceCatalog).
			WithTimestamp(environment.ConceivedAt))
		if err != nil {
			return nil, fmt.Errorf("assemble scope %q: %w", scope, err)
		}
		return graph, nil
	})
}

// runGraph executes one scope's removal graph: persist the plan, run, persist the trace, report the summary.
//
// Parameters:
//   - `ctx`: the cancellation context for the run.
//   - `cfg`: the decommission configuration (dry-run flag, verbosity).
//   - `graph`: the scope graph to execute.
//
// Returns:
//   - `error`: non-nil when the spec cannot be configured, the plan cannot persist, or the run fails.
func runGraph(ctx context.Context, cfg *Config, graph *op.Graph) error {

	runRoot := ""
	if value, ok := graph.Origin().Annotations().Get("run_root"); ok {
		runRoot = assert.Type[string]("run_root annotation", value)
	}

	spec := removalSpec(runRoot, cfg.DryRun)

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
			summary := trace.Summarize(graph)
			byAction := summary.ByAction()
			removed := byAction[string(file.Remove)].Completed()
			if scope := graph.Origin().Scope(); scope != "" {
				cli.Success("Decommissioned %d file(s) [%s]", removed, scope)
			} else {
				cli.Success("Decommissioned %d file(s)", removed)
			}
		}
	}

	return runErr
}

// removalSpec constructs a fresh [op.RuntimeEnvironmentSpec] confined at `root` for one phase of a removal.
//
// Parameters:
//   - `root`: the scope's target root — removal touches targets only, so it is also the confinement root and
//     the prune boundary.
//   - `dryRun`: forwarded to the application flag map.
//
// Returns:
//   - `*op.RuntimeEnvironmentSpec`: the constructed spec.
func removalSpec(root string, dryRun bool) *op.RuntimeEnvironmentSpec {

	return op.NewRuntimeEnvironmentSpec("writ").
		WithStatus(cli.UI()).
		WithRoot(root).
		WithApplication(&application.Application{
			Name:  "writ",
			Flags: map[string]any{"dry-run": dryRun},
		})
}

// selectEntries filters the folded inventory to the requested projects.
//
// Parameters:
//   - `inventory`: the readback fold.
//   - `projects`: the projects to select; empty selects nothing.
//
// Returns:
//   - `[]readback.Entry`: the selected entries, unordered.
func selectEntries(inventory *readback.Inventory, projects []string) []readback.Entry {

	wanted := make(map[string]bool, len(projects))
	for _, p := range projects {
		wanted[p] = true
	}

	var selected []readback.Entry
	//nolint:gocritic // rangeValCopy: map values are unaddressable; the per-iteration copy is the read.
	for _, entry := range inventory.Entries {
		if wanted[entry.Project] {
			selected = append(selected, entry)
		}
	}
	return selected
}

// scopeOrder defines removal priority: System first, then Home, then unscoped.
var scopeOrder = map[string]int{
	"system": 0,
	"home":   1,
}

// scopesInOrder returns the populated scopes in deterministic removal order.
//
// Parameters:
//   - `byScope`: the entries grouped by scope.
//
// Returns:
//   - `[]string`: the scopes — system, home, then the rest lexically.
func scopesInOrder(byScope map[string][]readback.Entry) []string {

	scopes := make([]string, 0, len(byScope))
	for scope := range byScope {
		scopes = append(scopes, scope)
	}
	sort.Slice(scopes, func(i, j int) bool {
		oi, ok := scopeOrder[scopes[i]]
		if !ok {
			oi = len(scopeOrder)
		}
		oj, ok := scopeOrder[scopes[j]]
		if !ok {
			oj = len(scopeOrder)
		}
		if oi != oj {
			return oi < oj
		}
		return scopes[i] < scopes[j]
	})
	return scopes
}

// endregion

// claimDeployed mints the claim for a deployed entry, asserting the kind writ put there.
//
// A linked entry claims a symbolic link and a copied entry claims a regular file, so that pre-flight
// judges what the run actually expects to find. Without the assertion the claim would be unasserted and
// any entry at the path would satisfy it — including one the user substituted.
//
// Parameters:
//   - `environment`: the planning runtime environment.
//   - `deployedAction`: the action that put the entry there, as recorded in the inventory.
//   - `rel`: the target's plan-space path.
//
// Returns:
//   - `file.Resource`: the kinded claim.
//   - `error`: a construction failure.
func claimDeployed(environment *op.RuntimeEnvironment, deployedAction, rel string) (file.Resource, error) {

	if deployedAction == string(file.Link) {
		return file.DiscoverSymbolicLink(environment, rel)
	}

	return file.DiscoverRegular(environment, rel)
}
