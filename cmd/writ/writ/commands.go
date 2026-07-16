// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/decommission"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/identity"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/reconcile"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/upgrade"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/pkg/op"
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

// runDeployV2 implements the deploy command on the deploy package (phase-8 step 47 slice 1).
//
// Parsing stays here (cobra/viper are command-layer); planning and execution live in
// [deploy.Execute] — tree walk, layer pinning, per-scope graphs, store persistence, and reporting.
func runDeployV2(cmd *cobra.Command, args []string) error {

	cfg, err := parseDeployConfig(cmd, args)
	if err != nil {
		return err
	}

	// TODO(step 47 slice 4): wire the manifest planner — lore.Planner needs the platform token and the
	// detection helper is lore-internal today. Until wired, manifests are reported and skipped.
	return deploy.Execute(cmd.Context(), &deploy.Config{
		SourceRoot:   cfg.SourceRoot,
		TargetRoot:   cfg.TargetRoot,
		LayerSources: cfg.LayerSources,
		Projects:     cfg.Projects,
		Segments:     cfg.Segments,
		Vars:         cfg.TemplateData,
		AllowDirty:   cfg.AllowDirty,
		DryRun:       cfg.DryRun,
		Verbose:      cfg.Verbose,
	})
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

The deployed inventory comes from the store readback (never a directory scan), so
decommission removes exactly what writ's runs put into effect. Symlinked entries are
unlinked — a target the user replaced with a real file is refused, not deleted.
Copied files are removed with recovery-site archival (restorable on unwind).

Signature-gated safety (refusing unsigned state) arrives with graph signing (step 46).

`,
		Example: `  writ decommission noblefactor              # Remove project files
  writ decommission all noblefactor          # Remove multiple projects
  writ decommission --prune noblefactor      # Also remove empty parent directories
  writ decommission --force noblefactor      # Skip confirmation prompts`,
		Args: cobra.MinimumNArgs(1),
		RunE: runDecommission,
	}

	cmd.Flags().Bool("prune", false, "Remove empty parent directories after file removal")

	return cmd
}

// runDecommission implements the decommission command on the decommission package (phase-8 step 47 slice 2).
func runDecommission(cmd *cobra.Command, args []string) error {

	cfg, err := parseDecommissionConfig(cmd, args)
	if err != nil {
		return err
	}

	return decommission.Execute(cmd.Context(), &decommission.Config{
		Projects: cfg.Projects,
		Prune:    cfg.Prune,
		DryRun:   cfg.DryRun,
		Verbose:  cfg.Verbose,
	})
}

func newUpgradeCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "upgrade [<project>...]",
		Short: "Regenerate copied files (templates, secrets) from current sources",
		Long: `Regenerate copied files (templates, secrets) from current sources.

Symlinks are not affected. Only files that were copied during deployment
(templates expanded, secrets decrypted) are regenerated.

The copied inventory comes from the store readback; source templates/secrets are
re-processed through the same planned chains deploy uses.

Each copied file is classified first: a missing target regenerates freely; an
up-to-date target is left alone; a differing target skips with a warning and
regenerates only under --force — until recorded content identity lands (step 48),
source-changed cannot be distinguished from target-modified. Encrypted (sops)
entries cannot be compared without decrypting and follow the same --force rule.`,
		Example: `  writ upgrade                     # Regenerate all copied files
  writ upgrade noblefactor         # Regenerate for specific project
  writ upgrade --force             # Overwrite locally modified files`,
		RunE: runUpgrade,
	}

	cmd.Flags().Bool("force", false, "Overwrite locally modified files without prompting")

	return cmd
}

// runUpgrade implements the upgrade command on the upgrade package (phase-8 step 47 slice 2).
func runUpgrade(cmd *cobra.Command, args []string) error {

	cfg, err := parseUpgradeConfig(cmd, args)
	if err != nil {
		return err
	}

	return upgrade.Execute(cmd.Context(), &upgrade.Config{
		Projects: cfg.Projects,
		Force:    cfg.Force,
		Segments: cfg.Segments,
		Vars:     cfg.TemplateData,
		DryRun:   cfg.DryRun,
		Verbose:  cfg.Verbose,
	})
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
  ✗ Missing  — Origin file has no corresponding symlink
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

	for _, n := range g.Nodes() {
		if isSkippableNode(n) {
			continue
		}
		source, _ := op.ImmediateOf(n.Slots()["source"]).(string) //nolint:errcheck // zero value (empty) is acceptable
		target, _ := op.ImmediateOf(n.Slots()["path"]).(string)   //nolint:errcheck // zero value (empty) is acceptable
		report.Entries = append(report.Entries, buildNodeEntry(n, source, target, checkDrift))
	}
}

// isSkippableNode returns true for nodes that should not appear in the reconcile report.
func isSkippableNode(n *op.Node) bool {
	name := nodeActionName(n)
	// TODO(step 15): skipped-status filter was removed when Node.Status was dropped; per-node status
	// now lives on the recovery-stack receipts. Rewire from the audit trail when StateView sources
	// from receipts.
	return name == "file.backup" ||
		name == "file.link" ||
		name == "template.render_text" || name == "template.render_bytes" ||
		name == "encryption.decrypt"
}

// buildNodeEntry creates a reconcile entry from a graph node.
func buildNodeEntry(n *op.Node, source, target string, _ bool) reconcile.Entry {
	entry := reconcile.Entry{
		RelTarget: n.ID(),
		Source:    source,
		Target:    target,
		Project:   n.Origin,
		Action:    nodeActionName(n),
	}

	if _, err := os.Stat(target); os.IsNotExist(err) {
		entry.State = reconcile.StateMissing
		entry.Message = "file not deployed"
	} else {
		entry.State = reconcile.StateCopied
	}
	return entry
}

// nodeActionName returns the bound action's name, or empty string when no action is bound.
//
// Parameters:
//   - node: the node to read the action name from.
//
// Returns:
//   - string: the action name, or empty string.
func nodeActionName(node *op.Node) string {

	action := node.Action()

	if action == nil {
		return ""
	}

	return action.Name()
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

func newInspectCmd() *cobra.Command {
	var opts cli.SinkOptions

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
