// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"filippo.io/age"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/lore"
	"github.com/NobleFactor/devlore-cli/internal/output"
	"github.com/NobleFactor/devlore-cli/internal/writ/identity"
	"github.com/NobleFactor/devlore-cli/internal/writ/reconcile"
	"github.com/NobleFactor/devlore-cli/internal/writ/segment"
	"github.com/NobleFactor/devlore-cli/internal/writ/snapshot"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/sops"
)

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [flags] <project>...",
		Short: "Deploy projects by creating symlinks in the target location",
		Long: `Deploy projects by creating symlinks in the target location.

Files inside each project directory are symlinked to the target (default: ~).
Platform-specific variants (e.g., project.Darwin) are selected automatically.
If a project contains packages-manifest.yaml, the manifest is resolved through
the lore Planner, adding package installation nodes to the execution graph.

Conflict handling (--conflict):
  stop      Stop on first conflict (default)
  backup    Move conflicting files to timestamped backups
  overwrite Remove conflicting files without backup
  skip      Skip conflicting files and continue`,
		Example: `  writ deploy noblefactor
  writ deploy all noblefactor thenobles
  writ deploy --conflict=backup noblefactor
  writ deploy --conflict=overwrite noblefactor
  writ deploy -s ROLE=desktop noblefactor`,
		Args: cobra.MinimumNArgs(1),
		RunE: runDeployV2,
	}

	cmd.Flags().StringP("conflict", "c", "stop", "Conflict resolution: stop, backup, overwrite, skip")
	cmd.Flags().StringArrayP("segment", "s", nil, "Set custom segment value (KEY=value, repeatable)")
	cmd.Flags().Bool("allow-dirty", false, "Allow planning against layers with uncommitted changes")

	return cmd
}

// scopeOrder defines the execution priority for target scopes.
// System executes first (elevated, unconfined), then Home (confined to $HOME).
var scopeOrder = map[string]int{
	"system": 0,
	"home":   1,
}

// sortGraphsByScope sorts graphs in deterministic execution order:
// system first, then home. Unscoped graphs (single-source mode) sort last.
//
// Parameters:
//   - graphs: graphs to sort in place
func sortGraphsByScope(graphs []*op.Graph) {

	sort.SliceStable(graphs, func(i, j int) bool {
		oi, ok := scopeOrder[graphs[i].Context.Scope]
		if !ok {
			oi = len(scopeOrder)
		}
		oj, ok := scopeOrder[graphs[j].Context.Scope]
		if !ok {
			oj = len(scopeOrder)
		}
		return oi < oj
	})
}

// runDeployV2 implements the deploy command using the graph design.
func runDeployV2(cmd *cobra.Command, args []string) error {
	// 1. Parse config (rolls up entire settings hierarchy)
	cfg, err := parseDeployConfig(cmd, args)
	if err != nil {
		return err
	}

	// 2. Check dirty state and pin layer sources (multi-source mode only)
	var commitHashes map[string]string
	var dirtyLayers []string
	if len(cfg.LayerSources) > 0 {
		// Check for uncommitted changes
		dirty, err := snapshot.CheckClean(cfg.LayerSources)
		if err != nil {
			return fmt.Errorf("check dirty: %w", err)
		}
		if len(dirty) > 0 {
			if !cfg.AllowDirty {
				return fmt.Errorf("layers have uncommitted changes: %v\nCommit your changes or use --allow-dirty to plan against HEAD", dirty)
			}
			cli.Warn("Planning against dirty layers (uncommitted changes): %v", dirty)
			dirtyLayers = dirty
		}

		// Pin to git worktree snapshots
		snapshots, cleanup, err := snapshot.PinAll(cfg.LayerSources)
		if err != nil {
			return fmt.Errorf("pin layers: %w", err)
		}
		defer cleanup()

		cfg.LayerSources = snapshot.RewriteSources(cfg.LayerSources, snapshots)
		commitHashes = snapshot.Hashes(snapshots)

		if cfg.Verbose {
			for _, s := range snapshots {
				cli.Note("Pinned %s → %s (%s)", s.Layer, s.CommitHash[:12], s.WorktreePath)
			}
		}
	}

	// 3. Build execution graphs (one per target scope)
	reg := op.NewActionRegistry()
	op.InitAll(reg, op.Context{})
	builder := NewDeployGraphBuilder(cfg, reg)
	builder.Planner = &lore.Planner{
		ActionRegistry: reg,
	}
	graphs, err := builder.Build()
	if err != nil {
		return err
	}

	if len(graphs) == 0 {
		cli.Note("No files to deploy")
		return nil
	}

	// Record commit hashes and dirty state on each graph
	for _, g := range graphs {
		g.Context.CommitHashes = commitHashes
		g.Context.DirtyLayers = dirtyLayers
	}

	// Sort: system first, then home
	sortGraphsByScope(graphs)

	// Verbose output (command-layer concern)
	if cfg.Verbose {
		reportGraphContext(cfg)
	}

	// Report collisions (recorded on all graphs from unified tree; use first)
	if len(graphs[0].Collisions) > 0 {
		reportCollisions(cfg, graphs[0].Collisions)
	}

	// 4a. Dry-run: serialize all graphs to stdout
	if cfg.DryRun {
		enc := yaml.NewEncoder(os.Stdout)
		enc.SetIndent(2)
		defer func() { _ = enc.Close() }() //nolint:errcheck
		for _, g := range graphs {
			if err := g.Serialize(enc); err != nil {
				return err
			}
		}
		return nil
	}

	// 4b. Execute each graph (fail-forward: independent scopes continue on failure)
	var errs []error
	for _, g := range graphs {
		engine, err := ConfigureEngine(&cfg.Config, g.Context.TargetRoot)
		if err != nil {
			errs = append(errs, fmt.Errorf("configure engine [%s]: %w", g.Context.Scope, err))
			continue
		}

		if err := engine.Run(context.Background(), g); err != nil {
			scope := g.Context.Scope
			if scope == "" {
				scope = "default"
			}
			cli.Warn("scope %s failed: %v", scope, err)
			errs = append(errs, fmt.Errorf("scope %s: %w", scope, err))
			continue
		}

		// Write receipt per graph
		path, err := cli.WriteReceipt(g, "writ")
		if err != nil {
			cli.Warn("failed to write receipt: %v", err)
		} else if cfg.Verbose {
			cli.Note("Receipt: %s", path)
		}

		// Summary per graph
		if g.Context.Scope != "" {
			cli.Success("Deployed %s [%s]", g.Summary.String(), g.Context.Scope)
		} else {
			cli.Success("Deployed %s", g.Summary.String())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d scope(s) failed", len(errs))
	}
	return nil
}

// reportGraphContext outputs verbose context information.
func reportGraphContext(cfg *DeployConfig) {
	if len(cfg.LayerSources) > 0 {
		cli.Note("Layers: %d sources", len(cfg.LayerSources))
		for _, src := range cfg.LayerSources {
			cli.Note("  %s/%s: %s → %s", src.Layer, src.TargetName, src.SourceRoot, src.TargetRoot)
		}
	} else {
		cli.Note("Source: %s", cfg.SourceRoot)
		cli.Note("Target: %s", cfg.TargetRoot)
	}
	cli.Note("Projects: %v", cfg.Projects)
	cli.Note("Segments: %s", cfg.Segments.String())
}

// reportCollisions outputs collision warnings.
func reportCollisions(cfg *DeployConfig, collisions []op.Collision) {
	if len(cfg.LayerSources) > 0 {
		cli.Warn("%d source collision(s) resolved by layer/specificity:", len(collisions))
		for _, c := range collisions {
			cli.Warn("  %s: using %s [%s] over %s [%s]",
				c.Target, c.Winner, c.WinnerLayer, c.Loser, c.LoserLayer)
		}
	} else {
		cli.Warn("%d source collision(s) resolved by specificity:", len(collisions))
		for _, c := range collisions {
			cli.Warn("  %s: using %s (specificity %d) over %s (specificity %d)",
				c.Target, c.Winner, c.WinnerSpecificity, c.Loser, c.LoserSpecificity)
		}
	}
}

// expandPath expands ~ to $HOME in paths.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		return os.Getenv("HOME") + path[1:]
	}
	if path == "~" {
		return os.Getenv("HOME")
	}
	return path
}

func newDecommissionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decommission [flags] <project>...",
		Short: "Remove deployed files and clean up resources for specified projects",
		Long: `Remove deployed files and clean up resources for specified projects.

Symlinks are removed directly. Copied files (templates, secrets) are removed
only after drift detection confirms they haven't been locally modified.

Safety behavior depends on state file:
  Signed state    → Safe: full drift detection before removal
  Unsigned state  → Warning, requires --force to proceed
  No state        → Error: cannot safely remove without state

`,
		Example: `  writ decommission noblefactor              # Remove project files
  writ decommission all noblefactor          # Remove multiple projects
  writ decommission --prune noblefactor      # Also remove empty parent directories
  writ decommission --force noblefactor      # Skip confirmation prompts`,
		Args: cobra.MinimumNArgs(1),
		RunE: runDecommission,
	}

	cmd.Flags().Bool("force", false, "Skip confirmation and proceed with unsigned state")
	cmd.Flags().Bool("prune", false, "Remove empty parent directories after file removal")

	return cmd
}

// runDecommission implements the decommission command.
// Builds per-scope state views and executes per-scope decommission graphs.
func runDecommission(cmd *cobra.Command, args []string) error {
	// 1. Parse config
	cfg, err := parseDecommissionConfig(cmd, args)
	if err != nil {
		return err
	}

	// 2. Discover scopes from receipts
	scopes, err := discoverScopes(cfg.Verbose)
	if err != nil {
		return err
	}
	if len(scopes) == 0 {
		return fmt.Errorf("no deployment receipts found; cannot decommission without deployment history\nRun 'writ deploy' first")
	}

	// 3. Build and execute per-scope decommission graphs
	reg := op.NewActionRegistry()
	op.InitAll(reg, op.Context{})

	var graphs []*op.Graph
	for _, scope := range scopes {
		view, err := loadStateView(cfg.Verbose, scope)
		if err != nil {
			return err
		}
		if len(view.Files.Entries) == 0 {
			continue
		}

		g, err := NewDecommissionGraphBuilder(cfg, view, reg).Build()
		if err != nil {
			return err
		}
		if len(g.Nodes) == 0 {
			continue
		}
		graphs = append(graphs, g)
	}

	if len(graphs) == 0 {
		cli.Note("No files found for specified projects.")
		return nil
	}

	// Sort: system first, then home (reverse of deploy is safe; order doesn't
	// matter for independent scopes, but determinism is good)
	sortGraphsByScope(graphs)

	if cfg.Verbose {
		total := 0
		for _, g := range graphs {
			total += len(g.Nodes)
		}
		cli.Note("Files to decommission: %d across %d scope(s)", total, len(graphs))
	}

	// 4. Dry-run: serialize all graphs to stdout
	if cfg.DryRun {
		enc := yaml.NewEncoder(os.Stdout)
		enc.SetIndent(2)
		defer func() { _ = enc.Close() }() //nolint:errcheck
		for _, g := range graphs {
			if err := g.Serialize(enc); err != nil {
				return err
			}
		}
		return nil
	}

	// 5. Execute each graph (fail-forward: independent scopes continue on failure)
	var errs []error
	for _, g := range graphs {
		// If --prune is set, enable directory cleanup per scope.
		if cfg.Prune {
			cfg.TemplateData["prune"] = true
			cfg.TemplateData["boundary"] = g.Context.TargetRoot
		}

		engine, err := ConfigureEngine(&cfg.Config, g.Context.TargetRoot)
		if err != nil {
			errs = append(errs, fmt.Errorf("configure engine [%s]: %w", g.Context.Scope, err))
			continue
		}

		if err := engine.Run(context.Background(), g); err != nil {
			scope := g.Context.Scope
			if scope == "" {
				scope = "default"
			}
			cli.Warn("decommission scope %s failed: %v", scope, err)
			errs = append(errs, fmt.Errorf("scope %s: %w", scope, err))
			continue
		}

		// Write receipt per graph
		path, err := cli.WriteReceipt(g, "writ")
		if err != nil {
			cli.Warn("failed to write receipt: %v", err)
		} else if cfg.Verbose {
			cli.Note("Receipt: %s", path)
		}

		// Summary per graph
		if g.Context.Scope != "" {
			cli.Success("Decommissioned %s [%s]", g.Summary.String(), g.Context.Scope)
		} else {
			cli.Success("Decommissioned %s", g.Summary.String())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d scope(s) failed", len(errs))
	}
	return nil
}

// discoverScopes returns the distinct target scopes from writ receipts.
//
// Parameters:
//   - verbose: enable verbose logging
//
// Returns:
//   - []string: sorted unique scope values (empty string for unscoped)
//   - error: receipt loading error
func discoverScopes(verbose bool) ([]string, error) {
	receiptsDir := cli.ReceiptsDir()
	builder := execution.NewStateViewBuilder(execution.ViewOptions{
		Tools: []string{"writ"},
	})
	scopes, err := builder.DistinctScopes(receiptsDir)
	if err != nil {
		return nil, fmt.Errorf("discover scopes: %w", err)
	}
	if verbose {
		cli.Note("Discovered scopes: %v", scopes)
	}
	return scopes, nil
}

// loadStateView builds a StateView from writ receipts, optionally filtered by scope.
// An empty scope loads all writ receipts (all scopes merged).
//
// Parameters:
//   - verbose: enable verbose logging
//   - scope: target scope filter ("system", "home", or "" for all)
//
// Returns:
//   - *execution.StateView: built state view
//   - error: receipt loading error
func loadStateView(verbose bool, scope string) (*execution.StateView, error) {
	receiptsDir := cli.ReceiptsDir()
	builder := execution.NewStateViewBuilder(execution.ViewOptions{
		Tools: []string{"writ"},
		Scope: scope,
	})

	view, err := builder.Build(receiptsDir)
	if err != nil {
		return nil, fmt.Errorf("build state view: %w", err)
	}

	if verbose {
		if scope != "" {
			cli.Note("Loaded %d receipts [%s]", view.ReceiptCount, scope)
		} else {
			cli.Note("Loaded %d receipts", view.ReceiptCount)
		}
	}

	return view, nil
}

// projectSet converts a slice of projects to a map for quick lookup.
func projectSet(projects []string) map[string]bool {
	m := make(map[string]bool)
	for _, p := range projects {
		m[p] = true
	}
	return m
}

func newUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade [<project>...]",
		Short: "Regenerate copied files (templates, secrets) from current sources",
		Long: `Regenerate copied files (templates, secrets) from current sources.

Symlinks are not affected. Only files that were copied during deployment
(templates expanded, secrets decrypted) are regenerated.

Reads from the state file to identify copied files. Source templates/secrets
are re-processed and written to their target locations.

Drift handling:
  Source changed only   → Regenerate
  Target modified only  → Skip, warn (use --force to overwrite)
  Both changed          → Skip, warn (use --force to overwrite)`,
		Example: `  writ upgrade                     # Regenerate all copied files
  writ upgrade noblefactor         # Regenerate for specific project
  writ upgrade --force             # Overwrite locally modified files`,
		RunE: runUpgrade,
	}

	cmd.Flags().Bool("force", false, "Overwrite locally modified files without prompting")

	return cmd
}

// runUpgrade implements the upgrade command.
func runUpgrade(cmd *cobra.Command, args []string) error {
	// 1. Parse config
	cfg, err := parseUpgradeConfig(cmd, args)
	if err != nil {
		return err
	}

	// 2. Load state view and get copied files
	view, copied, err := loadViewAndCopiedFiles(cfg)
	if err != nil {
		return err
	}

	if len(copied) == 0 {
		cli.Note("No copied files to upgrade.")
		return nil
	}

	if cfg.Verbose {
		cli.Note("Upgrading %d copied file(s)...", len(copied))
	}

	// 3. Prepare engine data
	engineData, sopsClient, identities := prepareUpgradeEngine(cfg)

	// 4. Execute upgrades
	return executeUpgrades(cfg, view, copied, engineData, sopsClient, identities)
}

// loadViewAndCopiedFiles loads state view and filters copied files by project.
func loadViewAndCopiedFiles(cfg *UpgradeConfig) (*execution.StateView, map[string]*execution.FileEntry, error) {
	view, err := loadStateView(cfg.Verbose, "")
	if err != nil {
		return nil, nil, fmt.Errorf("load state: %w (run 'writ deploy' first)", err)
	}

	if view.ReceiptCount == 0 {
		return nil, nil, fmt.Errorf("no deployment receipts found; run 'writ deploy' first")
	}

	if viper.GetString("writ.repo") == "" {
		return nil, nil, fmt.Errorf("no repo configured; set writ.repo in config or use WRIT_REPO env var")
	}

	copied := view.Files.CopiedFiles()

	if len(cfg.Projects) > 0 {
		projects := projectSet(cfg.Projects)
		filtered := make(map[string]*execution.FileEntry)
		for relTarget, entry := range copied {
			if projects[entry.Project] {
				filtered[relTarget] = entry
			}
		}
		copied = filtered
	}

	return view, copied, nil
}

// prepareUpgradeEngine prepares the engine data, SOPS client, and loads identities.
func prepareUpgradeEngine(cfg *UpgradeConfig) (map[string]any, *sops.Client, []age.Identity) {
	segs := segment.DetectSegments().LoadFromEnv()

	segMap := make(map[string]string)
	for _, seg := range segs {
		if seg.Value != "" {
			segMap[seg.Name] = seg.Value
		}
	}

	templateData := make(map[string]any)
	if varsMap := viper.GetStringMapString("writ.vars"); varsMap != nil {
		for k, v := range varsMap {
			templateData[k] = v
		}
	}

	engineData := builtinTemplateData(segMap)
	for k, v := range templateData {
		engineData[k] = v
	}

	// Set up SOPS client
	sopsClient, _ := sops.NewClient(cfg.SourceRoot) //nolint:errcheck // nil when no .sops.yaml found

	identities, _ := identity.LoadIdentities() //nolint:errcheck // fallback: continue without identities
	return engineData, sopsClient, identities
}

// executeUpgrades regenerates copied files.
func executeUpgrades(cfg *UpgradeConfig, view *execution.StateView, copied map[string]*execution.FileEntry, engineData map[string]any, sopsClient *sops.Client, identities []age.Identity) error {
	var regenerated, skipped int
	var skippedFiles []string

	var errored int
	for relTarget, entry := range copied {
		result := upgradeFile(cfg, view, relTarget, entry, engineData, sopsClient, identities)
		switch result {
		case upgradeResultRegenerated:
			regenerated++
		case upgradeResultSkipped:
			skipped++
			skippedFiles = append(skippedFiles, relTarget)
		case upgradeResultError:
			errored++
		}
	}

	// Note: receipts are written per-deployment, not for individual upgrades
	// The upgrade operation doesn't currently write a receipt, but could in the future

	if skipped > 0 {
		cli.Success("%d file(s) regenerated, %d skipped", regenerated, skipped)
		if !cfg.Verbose {
			cli.Note("Skipped files (locally modified):")
			for _, f := range skippedFiles {
				cli.Note("  %s", f)
			}
		}
		cli.Note("Use --force to overwrite locally modified files.")
	} else {
		cli.Success("%d file(s) regenerated", regenerated)
	}

	if errored > 0 {
		return fmt.Errorf("%d file(s) failed to upgrade", errored)
	}
	return nil
}

type upgradeResult int

const (
	upgradeResultRegenerated upgradeResult = iota
	upgradeResultSkipped
	upgradeResultError
)

// upgradeFile regenerates a single copied file.
func upgradeFile(cfg *UpgradeConfig, view *execution.StateView, relTarget string, entry *execution.FileEntry, engineData map[string]any, sopsClient *sops.Client, identities []age.Identity) upgradeResult {
	targetRoot := view.Files.Root

	_, actions := tree.ProcessingPipeline(filepath.Base(entry.Source))

	reg := op.NewActionRegistry()
	op.InitAll(reg, op.Context{})

	if hasDecryptAction(actions) && len(identities) == 0 {
		cli.Error("%s: identities required for encrypted files", relTarget)
		return upgradeResultError
	}

	target := filepath.Join(targetRoot, relTarget)
	nodes, edges := buildUpgradeChain(reg, actions, relTarget, entry, target)

	eng := execution.NewGraphExecutor(execution.ExecutorOptions{
		DryRun:             cfg.DryRun,
		SopsClient:         sopsClient,
		Data:               engineData,
		ConflictResolution: execution.ResolutionOverwrite,
	})

	results, runErr := eng.RunNodes(context.Background(), nodes, edges)
	if runErr != nil {
		cli.Error("%s: %v", relTarget, runErr)
		return upgradeResultError
	}
	if len(results) > 0 && results[len(results)-1].Status == execution.ResultFailed {
		cli.Error("%s: %v", relTarget, results[len(results)-1].Error)
		return upgradeResultError
	}

	if cfg.Verbose {
		cli.Success("%s (regenerated)", relTarget)
	}

	return upgradeResultRegenerated
}

// buildUpgradeChain builds the node chain for a multi-action upgrade pipeline.
func buildUpgradeChain(reg *op.ActionRegistry, actions []string, relTarget string, entry *execution.FileEntry, target string) ([]*op.Node, []op.Edge) {
	hasDecrypt := hasDecryptAction(actions)
	var nodes []*op.Node
	var edges []op.Edge
	var prevNodeID string

	for i, opName := range actions {
		isLast := i == len(actions)-1
		nodeID := relTarget
		if !isLast {
			nodeID = relTarget + ":" + opName
		}

		node := &op.Node{
			ID:      nodeID,
			Action:  reg.MustGet(opName),
			Project: entry.Project,
		}
		if i == 0 {
			node.SetSlotImmediate("source", entry.Source)
		}
		if isLast {
			node.SetSlotImmediate("path", target)
			if hasDecrypt {
				node.SetSlotImmediate("mode", os.FileMode(0o600))
			}
		}
		nodes = append(nodes, node)
		if prevNodeID != "" {
			edges = append(edges, op.Edge{From: prevNodeID, To: nodeID})
		}
		prevNodeID = nodeID
	}
	return nodes, edges
}

func newReconcileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconcile [<project>...]",
		Short: "Full-stack drift detection and repair",
		Long: `Full-stack drift detection and repair.

Checks symlinks, copied files (templates/secrets), and optionally installed
packages against the state file. Can automatically repair detected issues.

Without arguments, scans target directory for writ-managed files.
With project arguments, builds a fresh tree and checks against expected deploystate.

Status indicators:
  ✓ Linked   — Symlink exists and points to project
  ✓ Copied   — File was copied (template/secret) and exists
  ⚠ Conflict — File exists but isn't our symlink
  ✗ Missing  — Project file has no corresponding symlink
  ? Orphan   — Symlink points to nonexistent file
  ↑ Stale    — Source changed since deployment, redeploy needed
  M Modified — Target file was edited locally
  ! Conflict — Both source and target changed`,
		Example: `  writ reconcile                    # Scan for deployed files
  writ reconcile noblefactor        # Check specific project
  writ reconcile --fix              # Automatically repair issues`,
		RunE: runReconcile,
	}

	cmd.Flags().Bool("drift", false, "Check for drift in copied files (default: true)")
	cmd.Flags().Bool("fix", false, "Automatically repair detected issues")
	cmd.Flags().Bool("json", false, "Promise as JSON")

	return cmd
}

// buildReconcileReport builds the reconcile report from available data sources.
func buildReconcileReport(cfg *ReconcileConfig) (*reconcile.Report, error) {
	if len(cfg.Projects) > 0 {
		return buildReportFromTree(cfg)
	}
	return buildReportFromStateOrScan(cfg)
}

// buildReportFromTree builds a report from the deploy tree for specific projects.
func buildReportFromTree(cfg *ReconcileConfig) (*reconcile.Report, error) {
	segs := segment.DetectSegments().LoadFromEnv()

	deployTree, err := tree.Build(tree.BuildConfig{
		SourceRoot: cfg.SourceRoot,
		TargetRoot: cfg.TargetRoot,
		Projects:   cfg.Projects,
		Segments:   segs,
	})
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}

	return reconcile.FromBuildResult(deployTree), nil
}

// buildReportFromStateOrScan builds a report from receipts or scan.
func buildReportFromStateOrScan(cfg *ReconcileConfig) (*reconcile.Report, error) {
	view, err := loadStateView(cfg.Verbose, "")
	if err == nil && view.ReceiptCount > 0 {
		return buildReportFromView(cfg, view)
	}
	return buildReportFromScan(cfg)
}

// buildReportFromView builds a report from the StateView (derived from receipts).
func buildReportFromView(cfg *ReconcileConfig, view *execution.StateView) (*reconcile.Report, error) {
	if cfg.Verbose {
		cli.Note("Using %d receipts from: %s", view.ReceiptCount, cli.ReceiptsDir())
	}

	return reconcileFromView(view, cfg.CheckDrift), nil
}

// buildReportFromScan builds a report by scanning the target directory.
func buildReportFromScan(cfg *ReconcileConfig) (*reconcile.Report, error) {
	report := reconcile.ScanTarget(cfg.TargetRoot, cfg.SourceRoot)

	rcpt, err := cli.LoadLatestReceipt("writ", "")
	if err != nil {
		if cfg.CheckDrift {
			return nil, fmt.Errorf("--drift requires state file or receipt; none found")
		}
		if cfg.Verbose {
			cli.Note("No state file or receipt found, showing symlinks only")
		}
		return report, nil
	}

	if cfg.Verbose {
		cli.Note("Using receipt for copied files: %s", cli.LatestReceiptPath("writ", ""))
	}

	if cfg.CheckDrift {
		if err := verifyGraphSignatureForReconcile(cfg, rcpt); err != nil {
			return nil, err
		}
	}

	addCopiedFilesFromGraph(report, rcpt, cfg.CheckDrift)
	return report, nil
}

// verifyGraphSignatureForReconcile verifies the graph signature for reconcile.
func verifyGraphSignatureForReconcile(cfg *ReconcileConfig, g *op.Graph) error {
	identities, err := identity.LoadIdentities()
	if err != nil {
		return fmt.Errorf("load identities for signature verification: %w", err)
	}

	verifyResult, verifyErr := VerifyGraphSignature(g, identities)
	switch verifyResult {
	case VerifyOK:
		if cfg.Verbose {
			cli.Success("Receipt signature valid")
		}
	case VerifyUnsigned:
		if cfg.Verbose {
			cli.Note("Receipt unsigned, skipping verification")
		}
	case VerifyInvalid, VerifyMissing:
		return fmt.Errorf("receipt signature invalid, redeploy to regenerate: %w", verifyErr)
	}
	return nil
}

func runReconcile(cmd *cobra.Command, args []string) error {
	cfg, err := parseReconcileConfig(cmd, args)
	if err != nil {
		return err
	}

	report, err := buildReconcileReport(cfg)
	if err != nil {
		return err
	}

	if cfg.JSONOutput {
		return outputReconcileJSON(report)
	}
	return outputReconcileText(report)
}

// addCopiedFilesFromGraph adds copied file nodes from a graph to the report.
func addCopiedFilesFromGraph(report *reconcile.Report, g *op.Graph, checkDrift bool) {
	report.FromReceipt = true
	report.ReceiptPath = cli.LatestReceiptPath("writ", "")

	for _, n := range g.Nodes {
		if isSkippableNode(n) {
			continue
		}
		source, _ := n.GetSlot("source").(string) //nolint:errcheck // zero value (empty) is acceptable
		target, _ := n.GetSlot("path").(string)   //nolint:errcheck // zero value (empty) is acceptable
		report.Entries = append(report.Entries, buildNodeEntry(n, source, target, checkDrift))
	}
}

// isSkippableNode returns true for nodes that should not appear in the reconcile report.
func isSkippableNode(n *op.Node) bool {
	action := n.ActionName()
	return n.Status == op.StatusSkipped ||
		action == "file.backup" ||
		action == "file.link" ||
		action == "template.render_text" || action == "template.render_bytes" ||
		action == "encryption.decrypt"
}

// buildNodeEntry creates a reconcile entry from a graph node.
func buildNodeEntry(n *op.Node, source, target string, _ bool) reconcile.Entry {
	entry := reconcile.Entry{
		RelTarget: n.ID,
		Source:    source,
		Target:    target,
		Project:   n.Project,
		Action:    n.ActionName(),
	}

	if _, err := os.Stat(target); os.IsNotExist(err) {
		entry.State = reconcile.StateMissing
		entry.Message = "file not deployed"
	} else {
		entry.State = reconcile.StateCopied
	}
	return entry
}

// reconcileFromView builds a status report from the StateView.
func reconcileFromView(view *execution.StateView, checkDrift bool) *reconcile.Report {
	report := &reconcile.Report{
		TargetRoot:  view.Files.Root,
		Projects:    view.Files.Projects(),
		FromReceipt: true,
		ReceiptPath: cli.ReceiptsDir(),
	}

	for relTarget, entry := range view.Files.Entries {
		target := filepath.Join(view.Files.Root, relTarget)
		statusEntry := reconcile.Entry{
			RelTarget: relTarget,
			Source:    entry.Source,
			Target:    target,
			Project:   entry.Project,
			Action:    entry.LastActionName(),
		}

		if entry.IsCopied() {
			classifyCopiedEntry(&statusEntry, checkDrift)
		} else {
			classifySymlinkEntry(&statusEntry, entry.Source)
		}

		report.Entries = append(report.Entries, statusEntry)
	}

	return report
}

// classifyCopiedEntry determines the state of a copied file entry.
func classifyCopiedEntry(entry *reconcile.Entry, _ bool) {
	if _, err := os.Stat(entry.Target); os.IsNotExist(err) {
		entry.State = reconcile.StateMissing
		entry.Message = "file not deployed"
		return
	}

	entry.State = reconcile.StateCopied
}

// classifySymlinkEntry determines the state of a symlink entry.
func classifySymlinkEntry(entry *reconcile.Entry, expectedSource string) {
	info, err := os.Lstat(entry.Target)
	if os.IsNotExist(err) {
		entry.State = reconcile.StateMissing
		entry.Message = "symlink not created"
		return
	}
	if err != nil {
		entry.State = reconcile.StateConflict
		entry.Message = err.Error()
		return
	}
	if info.Mode()&os.ModeSymlink == 0 {
		entry.State = reconcile.StateConflict
		entry.Message = "file exists, not a symlink"
		return
	}

	linkTarget, err := os.Readlink(entry.Target)
	if err != nil {
		entry.State = reconcile.StateConflict
		entry.Message = "cannot read symlink"
		return
	}

	// Resolve relative symlinks
	if !filepath.IsAbs(linkTarget) {
		linkTarget = filepath.Join(filepath.Dir(entry.Target), linkTarget)
	}
	linkTarget = filepath.Clean(linkTarget)

	if linkTarget != expectedSource {
		entry.State = reconcile.StateConflict
		entry.Message = "symlink points to " + linkTarget
		return
	}

	if _, err := os.Stat(expectedSource); os.IsNotExist(err) {
		entry.State = reconcile.StateOrphan
		entry.Message = "source file deleted"
	} else {
		entry.State = reconcile.StateLinked
	}
}

// outputReconcileJSON outputs the reconcile report as JSON.
func outputReconcileJSON(report *reconcile.Report) error {
	type jsonEntry struct {
		RelTarget string `json:"rel_target"`
		Source    string `json:"source"`
		Target    string `json:"target"`
		State     string `json:"state"`
		Project   string `json:"project"`
		Action    string `json:"action"`
		Message   string `json:"message,omitempty"`
	}

	type jsonReport struct {
		TargetRoot  string      `json:"target_root"`
		SourceRoot  string      `json:"source_root"`
		Projects    []string    `json:"projects"`
		FromReceipt bool        `json:"from_receipt"`
		ReceiptPath string      `json:"receipt_path,omitempty"`
		Entries     []jsonEntry `json:"entries"`
		Summary     struct {
			Linked        int `json:"linked"`
			Copied        int `json:"copied"`
			Conflict      int `json:"conflict"`
			Missing       int `json:"missing"`
			Orphan        int `json:"orphan"`
			Stale         int `json:"stale"`
			Modified      int `json:"modified"`
			DriftConflict int `json:"drift_conflict"`
		} `json:"summary"`
	}

	jr := jsonReport{
		TargetRoot:  report.TargetRoot,
		SourceRoot:  report.SourceRoot,
		Projects:    report.Projects,
		FromReceipt: report.FromReceipt,
		ReceiptPath: report.ReceiptPath,
	}

	for _, e := range report.Entries {
		jr.Entries = append(jr.Entries, jsonEntry{
			RelTarget: e.RelTarget,
			Source:    e.Source,
			Target:    e.Target,
			State:     e.State.Label(),
			Project:   e.Project,
			Action:    e.Action,
			Message:   e.Message,
		})
	}

	summary := report.Summary()
	jr.Summary.Linked = summary[reconcile.StateLinked]
	jr.Summary.Copied = summary[reconcile.StateCopied]
	jr.Summary.Conflict = summary[reconcile.StateConflict]
	jr.Summary.Missing = summary[reconcile.StateMissing]
	jr.Summary.Orphan = summary[reconcile.StateOrphan]
	jr.Summary.Stale = summary[reconcile.StateStale]
	jr.Summary.Modified = summary[reconcile.StateModified]
	jr.Summary.DriftConflict = summary[reconcile.StateDriftConflict]

	data, err := json.MarshalIndent(jr, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// outputReconcileText outputs the reconcile report as human-readable text.
func outputReconcileText(report *reconcile.Report) error {
	if len(report.Entries) == 0 {
		fmt.Println("No deployed files found.")
		if report.FromReceipt {
			fmt.Printf("(checked receipt: %s)\n", report.ReceiptPath)
		}
		return nil
	}

	// Group entries by project
	byProject := make(map[string][]reconcile.Entry)
	for _, e := range report.Entries {
		project := e.Project
		if project == "" {
			project = "(unknown)"
		}
		byProject[project] = append(byProject[project], e)
	}

	// Sort projects for consistent output
	projects := make([]string, 0, len(byProject))
	for p := range byProject {
		projects = append(projects, p)
	}
	sort.Strings(projects)

	// Promise each project
	for _, project := range projects {
		entries := byProject[project]
		fmt.Printf("%s:\n", project)

		// Sort entries by path
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].RelTarget < entries[j].RelTarget
		})

		for _, e := range entries {
			indicator := e.State.String()
			path := e.RelTarget
			msg := ""
			if e.Message != "" {
				msg = " (" + e.Message + ")"
			}
			fmt.Printf("  %s %s%s\n", indicator, path, msg)
		}
		fmt.Println()
	}

	// Summary
	printReconcileSummary(report)

	if report.FromReceipt {
		fmt.Printf("(from receipt: %s)\n", report.ReceiptPath)
	}

	return nil
}

// printReconcileSummary prints a one-line summary of reconcile results.
func printReconcileSummary(report *reconcile.Report) {
	summary := report.Summary()
	total := len(report.Entries)
	linked := summary[reconcile.StateLinked] + summary[reconcile.StateCopied]
	issues := total - linked

	if issues == 0 {
		fmt.Printf("%d files, all deployed correctly\n", total)
		return
	}

	fmt.Printf("%d files: %d ok", total, linked)
	for _, pair := range []struct {
		state reconcile.State
		label string
	}{
		{reconcile.StateConflict, "conflict"},
		{reconcile.StateMissing, "missing"},
		{reconcile.StateOrphan, "orphan"},
		{reconcile.StateStale, "stale"},
		{reconcile.StateModified, "modified"},
		{reconcile.StateDriftConflict, "drift-conflict"},
	} {
		if n := summary[pair.state]; n > 0 {
			fmt.Printf(", %d %s", n, pair.label)
		}
	}
	fmt.Println()
}

func newAdoptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adopt [flags] <item>...",
		Short: "Move files from target location into a project and create symlinks",
		Long: `Move files from target location into a project and create symlinks.

Use this to bring existing configuration files under version control.
Files are moved to <layer>/<scope>/<project>/ preserving their relative path,
then symlinked back to the original location.

Scope (Home or System) is inferred from the item's location:
  - Items under $HOME are adopted into Home/
  - Items under / (Unix) or %SystemRoot% (Windows) are adopted into System/

Directories are adopted recursively—all files within are moved and symlinked.
Existing symlinks within directories are skipped.

With --from-receipt, reads a lore receipt and adopts packages-manifest.yaml and
config files into the environment repository.`,
		Example: `  # Adopt a single file into personal layer
  writ adopt --project noblefactor ~/.zshrc

  # Adopt multiple files
  writ adopt --project noblefactor ~/.zshrc ~/.bashrc ~/.config/nvim/init.lua

  # Adopt an entire directory recursively
  writ adopt --project noblefactor ~/.config/nvim

  # Adopt into team layer
  writ adopt --layer team --project shared ~/.editorconfig

  # Adopt system file (inferred as System scope)
  writ adopt --project noblefactor /etc/myapp/config.yaml

  # Adopt from lore receipt
  writ adopt --from-receipt
  writ adopt --from-receipt ~/.local/state/lore/receipts/2026-01-19T14:32:07.yaml`,
		Args: cobra.MinimumNArgs(0),
		RunE: runAdopt,
	}

	cmd.Flags().String("layer", "personal", "Layer to adopt into: personal, team, or base")
	cmd.Flags().String("project", "", "Project name within the layer (required)")
	cmd.Flags().Bool("from-receipt", false, "Adopt packages-manifest.yaml and config from lore receipt")

	return cmd
}

// adoptFiles processes each file for adoption.
func adoptFiles(cfg *AdoptConfig) int {
	if cfg.Verbose {
		cli.Note("Layer: %s", cfg.Layer)
		cli.Note("Layer path: %s", cfg.LayerPath)
		cli.Note("Project: %s", cfg.Project)
	}

	var adopted int
	for _, item := range cfg.Files {
		count := adoptItem(cfg, item)
		adopted += count
	}
	return adopted
}

// adoptItem processes a single file or directory for adoption.
func adoptItem(cfg *AdoptConfig, item string) int {
	filePath := expandPath(item)
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(cfg.TargetRoot, filePath)
	}

	scope := inferScope(filePath, cfg.TargetRoot)
	projectDir := filepath.Join(cfg.LayerPath, scope, cfg.Project)

	if cfg.Verbose {
		cli.Note("File: %s -> scope: %s", filePath, scope)
	}

	info, err := os.Lstat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			cli.Error("%s: file does not exist", item)
		} else {
			cli.Error("%s: %v", item, err)
		}
		return 0
	}

	if info.Mode()&os.ModeSymlink != 0 {
		cli.Warn("%s: already a symlink (skip)", item)
		return 0
	}

	targetRoot := cfg.TargetRoot
	if scope == "System" {
		targetRoot = "/"
	}

	if info.IsDir() {
		return adoptDirectory(cfg, filePath, targetRoot, projectDir)
	}

	count, err := adoptFile(filePath, targetRoot, projectDir, cfg.Verbose, cfg.DryRun)
	if err != nil {
		cli.Error("%s: %v", item, err)
		return 0
	}
	return count
}

// adoptDirectory recursively adopts files from a directory.
func adoptDirectory(cfg *AdoptConfig, dirPath, targetRoot, projectDir string) int {
	var adopted int
	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			cli.Error("%s: %v", path, walkErr)
			return nil
		}

		if d.IsDir() {
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil {
			cli.Error("%s: %v", path, err)
			return nil
		}

		if fileInfo.Mode()&os.ModeSymlink != 0 {
			cli.Warn("%s: already a symlink (skip)", path)
			return nil
		}

		count, err := adoptFile(path, targetRoot, projectDir, cfg.Verbose, cfg.DryRun)
		if err != nil {
			cli.Error("%s: %v", path, err)
			return nil
		}
		adopted += count
		return nil
	})
	if err != nil {
		cli.Error("walking directory %s: %v", dirPath, err)
	}
	return adopted
}

// reportAdoptResult outputs the adoption summary.
func reportAdoptResult(cfg *AdoptConfig, adopted int) {
	if cfg.DryRun {
		cli.Note("Dry-run: would adopt %d file(s)", adopted)
	} else {
		cli.Success("Adopted %d file(s) into %s/%s", adopted, cfg.Layer, cfg.Project)
		if adopted > 0 {
			cli.Note("Remember to commit: cd %s && git add -A && git commit", cfg.LayerPath)
		}
	}
}

func runAdopt(cmd *cobra.Command, args []string) error {
	cfg, err := parseAdoptConfig(cmd, args)
	if err != nil {
		return err
	}

	if cfg.FromReceipt {
		receiptPath := ""
		if len(cfg.Files) > 0 {
			receiptPath = cfg.Files[0]
		}
		return runAdoptFromReceipt(receiptPath, cfg.Layer, cfg.Project, cfg.Verbose, cfg.DryRun)
	}

	adopted := adoptFiles(cfg)
	reportAdoptResult(cfg, adopted)
	return nil
}

// inferScope determines whether a file path belongs to Home or System scope.
// Unix: paths under $HOME are Home, paths under / are System
// Windows: paths under %USERPROFILE% are Home, paths under %SystemRoot% are System
func inferScope(filePath, homeDir string) string {
	// Normalize paths for comparison
	filePath = filepath.Clean(filePath)
	homeDir = filepath.Clean(homeDir)

	// If path is under home directory, it's Home scope
	if strings.HasPrefix(filePath, homeDir+string(filepath.Separator)) || filePath == homeDir {
		return "Home"
	}

	// Otherwise it's System scope
	return "System"
}

// runAdoptFromReceipt adopts files from a lore receipt.
func runAdoptFromReceipt(receiptPath, layer, project string, verbose, dryRun bool) error {
	// TODO: Implement reading lore receipt and adopting packages-manifest.yaml + config
	return fmt.Errorf("adopt --from-receipt: not yet implemented")
}

// adoptFile moves a single file to the project directory and creates a symlink back.
// Returns the count of adopted files (0 or 1) and any error.
func adoptFile(filePath, targetRoot, projectDir string, verbose, dryRun bool) (int, error) {
	// Compute relative path from target root
	relPath, err := filepath.Rel(targetRoot, filePath)
	if err != nil {
		return 0, fmt.Errorf("cannot compute relative path: %w", err)
	}

	// Destination in repo
	destPath := filepath.Join(projectDir, relPath)

	if verbose {
		cli.Note("%s -> %s", filePath, destPath)
	}

	if dryRun {
		cli.Note("Would adopt %s -> %s", relPath, destPath)
		cli.Note("Would symlink %s -> %s", filePath, destPath)
		return 1, nil
	}

	fp := &file.Provider{}

	// Create destination directory
	destDir := filepath.Dir(destPath)
	if _, err := fp.Mkdir(file.Resource{SourcePath: op.NewPath("", destDir)}, 0o755); err != nil {
		return 0, fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	// Check if destination already exists
	if _, err := os.Stat(destPath); err == nil {
		return 0, fmt.Errorf("destination already exists: %s", destPath)
	}

	// Move file to repo
	if _, _, err := fp.Move(file.Resource{SourcePath: op.NewPath("", filePath)}, file.Resource{SourcePath: op.NewPath("", destPath)}); err != nil {
		// Move may fail across filesystems, try copy+remove
		if err := copyFile(filePath, destPath); err != nil {
			return 0, fmt.Errorf("moving file: %w", err)
		}
		if err := os.Remove(filePath); err != nil {
			cli.Warn("could not remove original %s: %v", filePath, err)
			// Continue anyway, file is copied
		}
	}

	// Create symlink back
	if _, _, err := fp.Link(file.Resource{SourcePath: op.NewPath("", destPath)}, file.Resource{SourcePath: op.NewPath("", filePath)}); err != nil {
		return 0, fmt.Errorf("creating symlink (file remains at %s): %w", destPath, err)
	}

	cli.Success("Adopted %s", relPath)
	return 1, nil
}

// copyFile copies a file from src to dst preserving permissions.
func copyFile(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer func() { _ = dstFile.Close() }()

	buf := make([]byte, 32*1024)
	for {
		n, err := srcFile.Read(buf)
		if n > 0 {
			if _, writeErr := dstFile.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	return nil
}

func newInspectCmd() *cobra.Command {
	var opts output.Options

	cmd := &cobra.Command{
		Use:   "inspect <project|file>",
		Short: "Show detailed information about a project or deployed file",
		Long: `Show detailed information about a project or deployed file.

For a project: shows source location, deployed files, segments, and deploystate.
For a file path: shows source, target, operations, checksums, and drift status.

Promise is JSON by default for scripting. Use --format for alternatives.`,
		Example: `  writ inspect noblefactor
  writ inspect ~/.zshrc
  writ inspect noblefactor --format yaml
  writ inspect noblefactor --format '{{.ReceiverName}}\t{{.Source}}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("inspect: not yet implemented")
		},
	}

	cli.AddOutputFlags(cmd, &opts)

	return cmd
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available projects for the current target",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("list: not yet implemented")
		},
	}
	return cmd
}

// getConfiguredRepo returns the path for a layer, or empty string if it doesn't exist.
// Layers are directories (or symlinks) at ~/.local/share/devlore/writ/layers/{layer}/
func getConfiguredRepo(layer string) string {
	layerPath := filepath.Join(cli.WritLayersDir(), layer)

	// Check if layer exists (directory or symlink)
	info, err := os.Lstat(layerPath)
	if err != nil {
		return ""
	}

	// If it's a symlink, resolve it to get the actual path
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(layerPath)
		if err != nil {
			return "" // Broken symlink
		}
		return target
	}

	// It's a directory
	if info.IsDir() {
		return layerPath
	}

	return ""
}

func newReceiptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "receipt <subcommand>",
		Short: "View and manage deployment receipts",
	}

	showCmd := &cobra.Command{
		Use:   "show [name]",
		Short: "Display deployment receipt",
		Long: `Display deployment receipt for a writ deployment.

Shows what was deployed: packages, symlinks, templates processed.

Use --unified to include lore receipts (software installations) alongside
writ receipts (configuration deployments). This provides a complete view
of your environment deploystate.`,
		Example: `  writ receipt show                     # Show default receipt
  writ receipt show workstation          # Show named receipt
  writ receipt show workstation --unified # Include lore software receipts`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("receipt show: not yet implemented")
		},
	}
	showCmd.Flags().Bool("unified", false, "Include lore receipts (software + configuration)")
	cmd.AddCommand(showCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List available receipts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("receipt list: not yet implemented")
		},
	})

	return cmd
}

// builtinTemplateData returns the default template data available to all templates.
func builtinTemplateData(segMap map[string]string) map[string]any {
	data := make(map[string]any)

	// Platform info
	data["OS"] = runtime.GOOS
	data["ARCH"] = runtime.GOARCH
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	data["Hostname"] = hostname

	// User info
	data["Home"] = os.Getenv("HOME")
	if u, err := user.Current(); err == nil {
		data["Username"] = u.Username
	} else {
		data["Username"] = os.Getenv("USER")
	}

	// Segments
	data["Segments"] = segMap

	// XDG directories
	home := os.Getenv("HOME")
	data["ConfigHome"] = xdgPath("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	data["DataHome"] = xdgPath("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	data["StateHome"] = xdgPath("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	data["CacheHome"] = xdgPath("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	// Environment lookup function (usable in templates as {{ Env "KEY" }})
	data["Env"] = os.Getenv

	return data
}

// xdgPath returns the XDG directory from env or the default path.
func xdgPath(envVar, defaultPath string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return defaultPath
}

// hasDecryptAction returns true if the actions include decrypt.
func hasDecryptAction(actions []string) bool {
	for _, action := range actions {
		if action == "encryption.decrypt" {
			return true
		}
	}
	return false
}
