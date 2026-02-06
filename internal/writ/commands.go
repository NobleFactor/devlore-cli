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
	"github.com/NobleFactor/devlore-cli/internal/writ/identity"
	"github.com/NobleFactor/devlore-cli/internal/writ/reconcile"
	"github.com/NobleFactor/devlore-cli/internal/writ/secrets"
	"github.com/NobleFactor/devlore-cli/internal/writ/segment"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [flags] <project>...",
		Short: "Deploy projects by creating symlinks in the target location",
		Long: `Deploy projects by creating symlinks in the target location.

Files inside each project directory are symlinked to the target (default: ~).
Platform-specific variants (e.g., project.Darwin) are selected automatically.
If a project contains packages-manifest.yaml, the Package Graph Builder adds
package installation nodes to the execution graph (NOT YET IMPLEMENTED).

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

	return cmd
}

// runDeployV2 implements the deploy command using the graph design.
func runDeployV2(cmd *cobra.Command, args []string) error {
	// 1. Parse config (rolls up entire settings hierarchy)
	cfg, err := parseDeployConfig(cmd, args)
	if err != nil {
		return err
	}

	// 2. Build execution graph
	g, err := NewDeployGraphBuilder(cfg).Build()
	if err != nil {
		return err
	}

	// Verbose output (command-layer concern)
	if cfg.Verbose {
		reportGraphContext(cfg, g)
	}

	// Report collisions
	if len(g.Collisions) > 0 {
		reportCollisions(cfg, g.Collisions)
	}

	// 3a. Dry-run: serialize plan to stdout
	if cfg.DryRun {
		enc := yaml.NewEncoder(os.Stdout)
		enc.SetIndent(2)
		defer func() { _ = enc.Close() }()
		return g.Serialize(enc)
	}

	// 3b. Configure engine and execute
	engine, err := ConfigureEngine(&cfg.Config)
	if err != nil {
		return fmt.Errorf("configure engine: %w", err)
	}

	if err := engine.Run(context.Background(), g); err != nil {
		return err
	}

	// 3c. Write receipt to file
	path, err := cli.WriteReceipt(g, "writ")
	if err != nil {
		cli.Warn("failed to write receipt: %v", err)
	} else if cfg.Verbose {
		cli.Note("Receipt: %s", path)
	}

	// Summary
	cli.Success("Deployed %s", g.Summary.String())

	return nil
}

// reportGraphContext outputs verbose context information.
func reportGraphContext(cfg *DeployConfig, g *execution.Graph) {
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
func reportCollisions(cfg *DeployConfig, collisions []execution.Collision) {
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
func runDecommission(cmd *cobra.Command, args []string) error {
	// 1. Parse config
	cfg, err := parseDecommissionConfig(cmd, args)
	if err != nil {
		return err
	}

	// 2. Load state view from receipts
	view, err := loadStateView(cfg.Verbose)
	if err != nil {
		return err
	}

	if len(view.Files.Entries) == 0 {
		return fmt.Errorf("no deployment receipts found; cannot decommission without deployment history\nRun 'writ deploy' first")
	}

	// 3. Build execution graph
	g, err := NewDecommissionGraphBuilder(cfg, view).Build()
	if err != nil {
		return err
	}

	if len(g.Nodes) == 0 {
		cli.Note("No files found for specified projects.")
		return nil
	}

	if cfg.Verbose {
		cli.Note("Files to decommission: %d", len(g.Nodes))
	}

	// 4. Dry-run: serialize plan to stdout
	if cfg.DryRun {
		enc := yaml.NewEncoder(os.Stdout)
		enc.SetIndent(2)
		defer func() { _ = enc.Close() }()
		return g.Serialize(enc)
	}

	// 5. Configure engine and execute
	// If --prune is set, enable directory cleanup: after each file removal,
	// empty parent directories are removed up to the target root boundary.
	if cfg.Prune {
		cfg.TemplateData["prune_empty_dirs"] = true
		cfg.TemplateData["prune_boundary"] = view.Files.Root
	}

	engine, err := ConfigureEngine(&cfg.Config)
	if err != nil {
		return fmt.Errorf("configure engine: %w", err)
	}

	if err := engine.Run(context.Background(), g); err != nil {
		return err
	}

	// 6. Write receipt (the receipt IS the state record)
	path, err := cli.WriteReceipt(g, "writ")
	if err != nil {
		cli.Warn("failed to write receipt: %v", err)
	} else if cfg.Verbose {
		cli.Note("Receipt: %s", path)
	}

	// 7. Summary
	cli.Success("Decommissioned %s", g.Summary.String())
	return nil
}

// loadStateView builds a StateView from writ receipts.
func loadStateView(verbose bool) (*execution.StateView, error) {
	receiptsDir := cli.ReceiptsDir()
	builder := execution.NewStateViewBuilder(execution.ViewOptions{
		Tools: []string{"writ"},
	})

	view, err := builder.Build(receiptsDir)
	if err != nil {
		return nil, fmt.Errorf("build state view: %w", err)
	}

	if verbose {
		cli.Note("Loaded %d receipts", view.ReceiptCount)
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
	engineData, identities, err := prepareUpgradeEngine(cfg)
	if err != nil {
		return err
	}

	// 4. Execute upgrades
	return executeUpgrades(cfg, view, copied, engineData, identities)
}

// loadViewAndCopiedFiles loads state view and filters copied files by project.
func loadViewAndCopiedFiles(cfg *UpgradeConfig) (*execution.StateView, map[string]*execution.FileEntry, error) {
	view, err := loadStateView(cfg.Verbose)
	if err != nil {
		return nil, nil, fmt.Errorf("load state: %w (run 'writ deploy' first)", err)
	}

	if view.ReceiptCount == 0 {
		return nil, nil, fmt.Errorf("no deployment receipts found; run 'writ deploy' first")
	}

	if cli.GetString("writ", "repo", true) == "" {
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

// prepareUpgradeEngine prepares the engine data and loads identities.
func prepareUpgradeEngine(cfg *UpgradeConfig) (map[string]any, []age.Identity, error) {
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

	// Use the configured source root for secrets
	upgradeSecretsMgr, _ := secrets.NewManager(cfg.SourceRoot)
	engineData["decryptor"] = upgradeSecretsMgr.Decryptor()

	identities, _ := identity.LoadIdentities()
	return engineData, identities, nil
}

// executeUpgrades regenerates copied files.
func executeUpgrades(cfg *UpgradeConfig, view *execution.StateView, copied map[string]*execution.FileEntry, engineData map[string]any, identities []age.Identity) error {
	var regenerated, skipped int
	var skippedFiles []string

	for relTarget, entry := range copied {
		result := upgradeFile(cfg, view, relTarget, entry, engineData, identities)
		switch result {
		case upgradeResultRegenerated:
			regenerated++
		case upgradeResultSkipped:
			skipped++
			skippedFiles = append(skippedFiles, relTarget)
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

	return nil
}

type upgradeResult int

const (
	upgradeResultRegenerated upgradeResult = iota
	upgradeResultSkipped
	upgradeResultError
)

// upgradeFile regenerates a single copied file.
func upgradeFile(cfg *UpgradeConfig, view *execution.StateView, relTarget string, entry *execution.FileEntry, engineData map[string]any, identities []age.Identity) upgradeResult {
	targetRoot := view.Files.Root
	entrySourceChecksum := entry.SourceChecksum()
	entryTargetChecksum := entry.TargetChecksum()

	currentSourceChecksum := execution.ChecksumFile(entry.Source)
	currentTargetChecksum := execution.ChecksumFile(filepath.Join(targetRoot, relTarget))

	sourceChanged := currentSourceChecksum != "" && entrySourceChecksum != "" && currentSourceChecksum != entrySourceChecksum
	targetChanged := currentTargetChecksum != "" && entryTargetChecksum != "" && currentTargetChecksum != entryTargetChecksum

	if targetChanged && !cfg.Force {
		if cfg.Verbose {
			if sourceChanged {
				cli.Warn("%s (skipped: both source and target changed)", relTarget)
			} else {
				cli.Warn("%s (skipped: locally modified)", relTarget)
			}
		}
		return upgradeResultSkipped
	}

	_, ops := tree.ProcessingPipeline(filepath.Base(entry.Source))
	opStrings := ops.Strings()

	if hasDecryptOp(opStrings) && len(identities) == 0 {
		cli.Error("%s: identities required for encrypted files", relTarget)
		return upgradeResultError
	}

	target := filepath.Join(targetRoot, relTarget)
	node := &execution.Node{
		ID:         relTarget,
		Operations: opStrings,
		Project:    entry.Project,
	}
	node.SetSlotImmediate("source", entry.Source)
	node.SetSlotImmediate("path", target)

	if hasDecryptOp(opStrings) {
		node.Mode = 0600
	}

	reg := execution.NewOperationRegistry()
	for _, op := range execution.AllOps() {
		reg.Register(op)
	}
	eng := execution.NewGraphExecutor(reg, execution.ExecutorOptions{
		DryRun:             cfg.DryRun,
		Data:               engineData,
		ConflictResolution: execution.ResolutionOverwrite,
	})

	results, runErr := eng.RunNodes(context.Background(), []execution.Executable{node}, nil)
	if runErr != nil {
		cli.Error("%s: %v", relTarget, runErr)
		return upgradeResultError
	}
	if len(results) > 0 && results[0].Status == execution.ResultFailed {
		cli.Error("%s: %v", relTarget, results[0].Error)
		return upgradeResultError
	}

	if cfg.Verbose {
		if targetChanged && cfg.Force {
			cli.Success("%s (regenerated, local changes overwritten)", relTarget)
		} else {
			cli.Success("%s (regenerated)", relTarget)
		}
	}

	return upgradeResultRegenerated
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
	cmd.Flags().Bool("json", false, "Output as JSON")

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
	view, err := loadStateView(cfg.Verbose)
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

	rcpt, err := cli.LoadLatestReceipt("writ")
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
		cli.Note("Using receipt for copied files: %s", cli.LatestReceiptPath("writ"))
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
func verifyGraphSignatureForReconcile(cfg *ReconcileConfig, g *execution.Graph) error {
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
		return fmt.Errorf("receipt signature invalid, redeploy to regenerate: %v", verifyErr)
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
func addCopiedFilesFromGraph(report *reconcile.Report, g *execution.Graph, checkDrift bool) {
	report.FromReceipt = true
	report.ReceiptPath = cli.LatestReceiptPath("writ")

	for _, n := range g.Nodes {
		// Get primary operation
		primaryOp := ""
		if len(n.Operations) > 0 {
			primaryOp = n.Operations[0]
		}

		// Skip non-file nodes and symlinks
		if n.Status == execution.StatusSkipped || primaryOp == "delegate" || primaryOp == "backup" {
			continue
		}
		if primaryOp == "link" {
			continue // Symlinks are found by scanning
		}

		var entry reconcile.Entry
		source := n.GetSlot("source")
		target := n.GetSlot("path")
		if checkDrift && n.SourceChecksum != "" {
			entry = reconcile.Entry{
				RelTarget:      n.ID,
				Source:         source,
				Target:         target,
				Project:        n.Project,
				Operations:     n.Operations,
				SourceChecksum: n.SourceChecksum,
				TargetChecksum: n.TargetChecksum,
			}
			// Check drift
			currentSourceChecksum := execution.ChecksumFile(source)
			currentTargetChecksum := execution.ChecksumFile(target)

			sourceChanged := currentSourceChecksum != "" && currentSourceChecksum != n.SourceChecksum
			targetChanged := currentTargetChecksum != "" && currentTargetChecksum != n.TargetChecksum

			switch {
			case sourceChanged && targetChanged:
				entry.State = reconcile.StateDriftConflict
				entry.Message = "both source and target changed"
			case sourceChanged:
				entry.State = reconcile.StateStale
				entry.Message = "source changed, redeploy needed"
			case targetChanged:
				entry.State = reconcile.StateModified
				entry.Message = "target modified locally"
			default:
				entry.State = reconcile.StateCopied
			}
		} else {
			// Just check if file exists
			entry = reconcile.Entry{
				RelTarget:  n.ID,
				Source:     source,
				Target:     target,
				Project:    n.Project,
				Operations: n.Operations,
			}
			if _, err := os.Stat(target); os.IsNotExist(err) {
				entry.State = reconcile.StateMissing
				entry.Message = "file not deployed"
			} else {
				entry.State = reconcile.StateCopied
			}
		}

		report.Entries = append(report.Entries, entry)
	}
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
		entrySourceChecksum := entry.SourceChecksum()
		entryTargetChecksum := entry.TargetChecksum()

		statusEntry := reconcile.Entry{
			RelTarget:      relTarget,
			Source:         entry.Source,
			Target:         target,
			Project:        entry.Project,
			Operations:     entry.Operations(),
			SourceChecksum: entrySourceChecksum,
			TargetChecksum: entryTargetChecksum,
		}

		if entry.IsCopied() {
			// Copied file - check existence and optionally drift
			if _, err := os.Stat(target); os.IsNotExist(err) {
				statusEntry.State = reconcile.StateMissing
				statusEntry.Message = "file not deployed"
			} else if checkDrift && entrySourceChecksum != "" {
				// Check drift
				currentSourceChecksum := execution.ChecksumFile(entry.Source)
				currentTargetChecksum := execution.ChecksumFile(target)

				sourceChanged := currentSourceChecksum != "" && currentSourceChecksum != entrySourceChecksum
				targetChanged := currentTargetChecksum != "" && currentTargetChecksum != entryTargetChecksum

				switch {
				case sourceChanged && targetChanged:
					statusEntry.State = reconcile.StateDriftConflict
					statusEntry.Message = "both source and target changed"
				case sourceChanged:
					statusEntry.State = reconcile.StateStale
					statusEntry.Message = "source changed, redeploy needed"
				case targetChanged:
					statusEntry.State = reconcile.StateModified
					statusEntry.Message = "target modified locally"
				default:
					statusEntry.State = reconcile.StateCopied
				}
			} else {
				statusEntry.State = reconcile.StateCopied
			}
		} else {
			// Symlink - check if it exists and points correctly
			info, err := os.Lstat(target)
			if os.IsNotExist(err) {
				statusEntry.State = reconcile.StateMissing
				statusEntry.Message = "symlink not created"
			} else if err != nil {
				statusEntry.State = reconcile.StateConflict
				statusEntry.Message = err.Error()
			} else if info.Mode()&os.ModeSymlink == 0 {
				statusEntry.State = reconcile.StateConflict
				statusEntry.Message = "file exists, not a symlink"
			} else {
				// Check symlink target
				linkTarget, err := os.Readlink(target)
				if err != nil {
					statusEntry.State = reconcile.StateConflict
					statusEntry.Message = "cannot read symlink"
				} else {
					// Resolve relative symlinks
					if !filepath.IsAbs(linkTarget) {
						linkTarget = filepath.Join(filepath.Dir(target), linkTarget)
					}
					linkTarget = filepath.Clean(linkTarget)

					if linkTarget == entry.Source {
						if _, err := os.Stat(entry.Source); os.IsNotExist(err) {
							statusEntry.State = reconcile.StateOrphan
							statusEntry.Message = "source file deleted"
						} else {
							statusEntry.State = reconcile.StateLinked
						}
					} else {
						statusEntry.State = reconcile.StateConflict
						statusEntry.Message = "symlink points to " + linkTarget
					}
				}
			}
		}

		report.Entries = append(report.Entries, statusEntry)
	}

	return report
}

// outputReconcileJSON outputs the reconcile report as JSON.
func outputReconcileJSON(report *reconcile.Report) error {
	type jsonEntry struct {
		RelTarget  string   `json:"rel_target"`
		Source     string   `json:"source"`
		Target     string   `json:"target"`
		State      string   `json:"state"`
		Project    string   `json:"project"`
		Operations []string `json:"operations"`
		Message    string   `json:"message,omitempty"`
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
			RelTarget:  e.RelTarget,
			Source:     e.Source,
			Target:     e.Target,
			State:      e.State.Label(),
			Project:    e.Project,
			Operations: e.Operations,
			Message:    e.Message,
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

	// Output each project
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
	summary := report.Summary()
	total := len(report.Entries)
	linked := summary[reconcile.StateLinked] + summary[reconcile.StateCopied]
	issues := total - linked

	if issues == 0 {
		fmt.Printf("%d files, all deployed correctly\n", total)
	} else {
		fmt.Printf("%d files: %d ok", total, linked)
		if n := summary[reconcile.StateConflict]; n > 0 {
			fmt.Printf(", %d conflict", n)
		}
		if n := summary[reconcile.StateMissing]; n > 0 {
			fmt.Printf(", %d missing", n)
		}
		if n := summary[reconcile.StateOrphan]; n > 0 {
			fmt.Printf(", %d orphan", n)
		}
		if n := summary[reconcile.StateStale]; n > 0 {
			fmt.Printf(", %d stale", n)
		}
		if n := summary[reconcile.StateModified]; n > 0 {
			fmt.Printf(", %d modified", n)
		}
		if n := summary[reconcile.StateDriftConflict]; n > 0 {
			fmt.Printf(", %d drift-conflict", n)
		}
		fmt.Println()
	}

	if report.FromReceipt {
		fmt.Printf("(from receipt: %s)\n", report.ReceiptPath)
	}

	return nil
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
	for _, file := range cfg.Files {
		count := adoptItem(cfg, file)
		adopted += count
	}
	return adopted
}

// adoptItem processes a single file or directory for adoption.
func adoptItem(cfg *AdoptConfig, file string) int {
	filePath := expandPath(file)
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
			cli.Error("%s: file does not exist", file)
		} else {
			cli.Error("%s: %v", file, err)
		}
		return 0
	}

	if info.Mode()&os.ModeSymlink != 0 {
		cli.Warn("%s: already a symlink (skip)", file)
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
		cli.Error("%s: %v", file, err)
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
	if receiptPath == "" {
		cli.Warn("adopt --from-receipt: not yet implemented (would use most recent receipt)")
	} else {
		cli.Warn("adopt --from-receipt %s: not yet implemented", receiptPath)
	}
	return nil
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

	// Create destination directory
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, fmt.Errorf("creating directory %s: %w", destDir, err)
	}

	// Check if destination already exists
	if _, err := os.Stat(destPath); err == nil {
		return 0, fmt.Errorf("destination already exists: %s", destPath)
	}

	// Move file to repo
	if err := os.Rename(filePath, destPath); err != nil {
		// Rename may fail across filesystems, try copy+remove
		if err := copyFile(filePath, destPath); err != nil {
			return 0, fmt.Errorf("moving file: %w", err)
		}
		if err := os.Remove(filePath); err != nil {
			cli.Warn("could not remove original %s: %v", filePath, err)
			// Continue anyway, file is copied
		}
	}

	// Create symlink back
	if err := os.Symlink(destPath, filePath); err != nil {
		// Try to restore the file
		if mvErr := os.Rename(destPath, filePath); mvErr != nil {
			return 0, fmt.Errorf("creating symlink (file remains at %s): %w", destPath, err)
		}
		return 0, fmt.Errorf("creating symlink: %w", err)
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
	var output cli.OutputFlags

	cmd := &cobra.Command{
		Use:   "inspect <project|file>",
		Short: "Show detailed information about a project or deployed file",
		Long: `Show detailed information about a project or deployed file.

For a project: shows source location, deployed files, segments, and deploystate.
For a file path: shows source, target, operations, checksums, and drift status.

Output is JSON by default for scripting. Use --format for alternatives.`,
		Example: `  writ inspect noblefactor
  writ inspect ~/.zshrc
  writ inspect noblefactor --format yaml
  writ inspect noblefactor --format '{{.Name}}\t{{.Source}}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("inspect: not yet implemented")
			return nil
		},
	}

	cli.AddOutputFlags(cmd, &output)

	return cmd
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available projects for the current target",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("list: not yet implemented")
			return nil
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
			name := "default"
			if len(args) > 0 {
				name = args[0]
			}
			unified, _ := cmd.Flags().GetBool("unified")
			if unified {
				cli.Note("receipt show %s --unified: not yet implemented", name)
			} else {
				cli.Note("receipt show %s: not yet implemented", name)
			}
			return nil
		},
	}
	showCmd.Flags().Bool("unified", false, "Include lore receipts (software + configuration)")
	cmd.AddCommand(showCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List available receipts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cli.Note("receipt list: not yet implemented")
			return nil
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
	hostname, _ := os.Hostname()
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
	data["Env"] = func(key string) string {
		return os.Getenv(key)
	}

	return data
}

// xdgPath returns the XDG directory from env or the default path.
func xdgPath(envVar, defaultPath string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return defaultPath
}

// hasDecryptOp returns true if the operations include decrypt.
func hasDecryptOp(ops []string) bool {
	for _, op := range ops {
		if op == "decrypt" {
			return true
		}
	}
	return false
}
