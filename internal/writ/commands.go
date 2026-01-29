// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

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
	"time"

	"filippo.io/age"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/engine"
	"github.com/NobleFactor/devlore-cli/internal/writ/identity"
	"github.com/NobleFactor/devlore-cli/internal/writ/receipt"
	"github.com/NobleFactor/devlore-cli/internal/writ/secrets"
	"github.com/NobleFactor/devlore-cli/internal/writ/segment"
	"github.com/NobleFactor/devlore-cli/internal/writ/state"
	"github.com/NobleFactor/devlore-cli/internal/writ/reconcile"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [flags] <project>...",
		Short: "Deploy projects by creating symlinks in the target location",
		Long: `Deploy projects by creating symlinks in the target location.

Files inside each project directory are symlinked to the target (default: ~).
Platform-specific variants (e.g., project.Darwin) are selected automatically.
If a project contains packages.manifest, writ delegates to lore for software installation.

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
		RunE: runDeploy,
	}

	cmd.Flags().StringP("conflict", "c", "stop", "Conflict resolution: stop, backup, overwrite, skip")
	cmd.Flags().StringArrayP("segment", "s", nil, "Set custom segment value (KEY=value, repeatable)")

	return cmd
}

// runDeploy implements the deploy command.
func runDeploy(cmd *cobra.Command, args []string) error {
	// Get config values
	dryRun := viper.GetBool("writ.dry-run")
	verbose := viper.GetBool("writ.verbose")
	conflictFlag, _ := cmd.Flags().GetString("conflict")
	segmentFlags, _ := cmd.Flags().GetStringArray("segment")

	// Determine conflict resolution strategy
	var resolution engine.ConflictResolution
	switch conflictFlag {
	case "stop", "":
		resolution = engine.ResolutionStop
	case "backup":
		resolution = engine.ResolutionBackup
	case "overwrite":
		resolution = engine.ResolutionOverwrite
	case "skip":
		resolution = engine.ResolutionSkip
	default:
		return fmt.Errorf("invalid --conflict value %q: must be stop, backup, overwrite, or skip", conflictFlag)
	}

	// Collect layer sources (base → team → personal, each with System → Home)
	layerSources, err := CollectLayerSources()
	if err != nil {
		return fmt.Errorf("collect layer sources: %w", err)
	}

	// Fall back to legacy single-repo mode if no layers configured
	var sourceRoot string
	if len(layerSources) == 0 {
		sourceRoot = cli.GetString("writ", "repo", true)
		if sourceRoot == "" {
			return fmt.Errorf("no layer configured; use 'writ migrate <source>' to migrate your environment to a writ layer")
		}
		sourceRoot = expandPath(sourceRoot)
	}

	// Resolve target root (default: $HOME) - used for single-repo mode
	targetRoot := os.Getenv("HOME")
	if targetRoot == "" {
		return fmt.Errorf("HOME environment variable not set")
	}

	// Build segments from platform detection + env overrides + flag overrides
	segs := segment.DetectSegments()
	segs = segs.LoadFromEnv()

	// Apply segment flags (-s KEY=value)
	for _, sf := range segmentFlags {
		parts := strings.SplitN(sf, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid segment flag %q: expected KEY=value", sf)
		}
		segs = segs.Set(parts[0], parts[1])
	}

	// Load template variables from config
	templateData := make(map[string]any)
	if varsMap := viper.GetStringMapString("writ.vars"); varsMap != nil {
		for k, v := range varsMap {
			templateData[k] = v
		}
	}

	if verbose {
		if len(layerSources) > 0 {
			fmt.Fprintf(os.Stderr, "Layers: %d sources\n", len(layerSources))
			for _, src := range layerSources {
				fmt.Fprintf(os.Stderr, "  %s/%s: %s → %s\n", src.Layer, src.TargetName, src.SourceRoot, src.TargetRoot)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Source: %s\n", sourceRoot)
			fmt.Fprintf(os.Stderr, "Target: %s\n", targetRoot)
		}
		fmt.Fprintf(os.Stderr, "Projects: %v\n", args)
		fmt.Fprintf(os.Stderr, "Segments: %s\n", segs.String())
	}

	// Build deployment tree
	var deployTree *tree.BuildResult
	if len(layerSources) > 0 {
		// Multi-layer mode
		deployTree, err = tree.Build(tree.BuildConfig{
			Sources:    layerSources,
			TargetRoot: targetRoot,
			Projects:   args,
			Segments:   segs,
		})
	} else {
		// Single-repo mode (backwards compatible)
		deployTree, err = tree.Build(tree.BuildConfig{
			SourceRoot: sourceRoot,
			TargetRoot: targetRoot,
			Projects:   args,
			Segments:   segs,
		})
	}
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}

	// Warn about source collisions (same target from different source dirs or layers)
	if deployTree.HasCollisions() {
		if len(layerSources) > 0 {
			fmt.Fprintf(os.Stderr, "Warning: %d source collision(s) resolved by layer/specificity:\n", len(deployTree.Collisions))
			for _, c := range deployTree.Collisions {
				fmt.Fprintf(os.Stderr, "  %s: using %s [%s] over %s [%s]\n",
					c.Target, c.Winner, c.WinnerLayer, c.Loser, c.LoserLayer)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Warning: %d source collision(s) resolved by specificity:\n", len(deployTree.Collisions))
			for _, c := range deployTree.Collisions {
				fmt.Fprintf(os.Stderr, "  %s: using %s (specificity %d) over %s (specificity %d)\n",
					c.Target, c.Winner, c.WinnerSpecificity, c.Loser, c.LoserSpecificity)
			}
		}
	}

	// Load age identities for decryption and signing
	identities, identityErr := identity.LoadIdentities()
	if identityErr != nil && deployTree.SecretCount() > 0 {
		return fmt.Errorf("load identities: %w (required for %d encrypted files)", identityErr, deployTree.SecretCount())
	}

	// Get X25519 identity for receipt signing (prefer first native age identity)
	var signingIdentity *age.X25519Identity
	if identityErr == nil {
		for _, id := range identities {
			if x, ok := id.(*age.X25519Identity); ok {
				signingIdentity = x
				break
			}
		}
	}

	// Build segments map for template data
	segMap := make(map[string]string)
	for _, seg := range segs {
		if seg.Value != "" {
			segMap[seg.Name] = seg.Value
		}
	}

	// Build engine data (template vars + builtins + decryptor)
	engineData := builtinTemplateData(segMap)
	for k, v := range templateData {
		engineData[k] = v
	}

	// Set up SOPS decryptor for encrypted files (.age, .sops)
	// SOPS handles key resolution via .sops.yaml + environment variables
	secretsMgr, _ := secrets.NewManager(sourceRoot)
	engineData["decryptor"] = secretsMgr.Decryptor()

	// Dry-run: output plan and return
	if dryRun {
		return outputDryRun(deployTree)
	}

	// Create engine
	registry := engine.NewRegistry()
	for _, op := range engine.FileOps() {
		registry.Register(op)
	}
	eng := engine.New(registry, engine.Options{
		DryRun:             false,
		Data:               engineData,
		ConflictResolution: resolution,
	})

	// Pre-flight conflict detection
	preflight := eng.Preflight(deployTree.Graph)

	// Report conflicts upfront
	if preflight.HasConflicts() {
		fmt.Fprintf(os.Stderr, "\nConflicts detected (%d):\n", len(preflight.Conflicts))
		for _, c := range preflight.Conflicts {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", c.Node.ID, c.Message)
		}

		if resolution == engine.ResolutionStop {
			fmt.Fprintf(os.Stderr, "\nUse --conflict=backup, --conflict=overwrite, or --conflict=skip to resolve.\n")
			return fmt.Errorf("%d conflict(s) detected", len(preflight.Conflicts))
		}

		// Show what will happen
		switch resolution {
		case engine.ResolutionBackup:
			fmt.Fprintf(os.Stderr, "\nBacking up conflicting files...\n")
		case engine.ResolutionOverwrite:
			fmt.Fprintf(os.Stderr, "\nOverwriting conflicting files...\n")
		case engine.ResolutionSkip:
			fmt.Fprintf(os.Stderr, "\nSkipping conflicting files...\n")
		}
	}

	// Report already deployed (verbose only)
	if verbose && len(preflight.AlreadyDone) > 0 {
		fmt.Fprintf(os.Stderr, "\nAlready deployed (%d):\n", len(preflight.AlreadyDone))
		for _, c := range preflight.AlreadyDone {
			fmt.Fprintf(os.Stderr, "  %s\n", c.Node.ID)
		}
	}

	// Handle conflicts before execution
	var backupPaths map[string]string // node.Target → backup path
	skippedSet := make(map[string]bool)

	switch resolution {
	case engine.ResolutionBackup:
		backupPaths = make(map[string]string)
		timestamp := time.Now().Format("20060102-150405")
		for _, c := range preflight.Conflicts {
			backupPath := c.Node.Target + ".writ-backup." + timestamp
			if err := os.Rename(c.Node.Target, backupPath); err != nil {
				return fmt.Errorf("backup %s: %w", c.Node.Target, err)
			}
			backupPaths[c.Node.Target] = backupPath
		}
	case engine.ResolutionSkip:
		for _, c := range preflight.Conflicts {
			skippedSet[c.Node.ID] = true
		}
	}

	// Build node lookup map
	nodeByID := make(map[string]*engine.Node)
	for _, n := range deployTree.Graph.Nodes {
		nodeByID[n.ID] = n
	}

	// Collect already-deployed node IDs
	alreadyDeployedSet := make(map[string]bool)
	for _, c := range preflight.AlreadyDone {
		alreadyDeployedSet[c.Node.ID] = true
	}

	// Execute deployment
	results, err := eng.Run(context.Background(), deployTree.Graph)
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	// Build receipt
	rcpt := receipt.New(sourceRoot, targetRoot, args, segMap)

	var deployed, skipped int
	for _, r := range results {
		node := nodeByID[r.NodeID]
		if node == nil {
			continue
		}

		// Handle skipped nodes
		if skippedSet[r.NodeID] {
			rcpt.AddSkipped(r.NodeID)
			skipped++
			continue
		}

		// Handle delegate nodes
		if isDelegate(node) {
			rcpt.AddDelegated(node)
			continue
		}

		// Determine if already deployed
		alreadyDeployed := alreadyDeployedSet[r.NodeID]

		// Use checksums for copied files (templates, secrets)
		if r.SourceChecksum != "" || r.TargetChecksum != "" {
			rcpt.AddNodeWithChecksums(node, alreadyDeployed, r.SourceChecksum, r.TargetChecksum)
		} else {
			rcpt.AddNode(node, alreadyDeployed)
		}

		if r.Status == engine.StatusCompleted {
			deployed++
		}
	}

	// Record backups in receipt
	for target, backupPath := range backupPaths {
		rcpt.AddBackup(target, backupPath)
	}

	// Handle delegated nodes (packages.manifest files)
	var delegatedNodes []*engine.Node
	for _, n := range deployTree.Graph.Nodes {
		if isDelegate(n) {
			delegatedNodes = append(delegatedNodes, n)
		}
	}
	if len(delegatedNodes) > 0 {
		fmt.Fprintf(os.Stderr, "\nDelegated to lore (%d manifests):\n", len(delegatedNodes))
		for _, node := range delegatedNodes {
			fmt.Fprintf(os.Stderr, "  %s\n", node.Source)
		}
		fmt.Fprintf(os.Stderr, "Run 'lore apply' to install packages.\n")
	}

	// Sign and write receipt
	var receiptFilename string
	if signingIdentity != nil {
		if err := rcpt.Sign(signingIdentity); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to sign receipt: %v\n", err)
		} else if verbose {
			fmt.Fprintf(os.Stderr, "Receipt signed with: %s\n", signingIdentity.Recipient().String())
		}
	} else if verbose {
		fmt.Fprintf(os.Stderr, "Warning: no age identity found, receipt will be unsigned\n")
	}

	receiptPath, err := rcpt.Write()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write receipt: %v\n", err)
	} else {
		receiptFilename = filepath.Base(receiptPath)
		if verbose {
			fmt.Fprintf(os.Stderr, "Receipt: %s\n", receiptPath)
		}
	}

	// Update state file
	if receiptFilename != "" {
		deployState, err := state.LoadOrCreate(sourceRoot, targetRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load state: %v\n", err)
		} else {
			deployState.UpdateFromReceipt(rcpt, receiptFilename)

			if signingIdentity != nil {
				if err := deployState.Sign(signingIdentity); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to sign state: %v\n", err)
				}
			}

			if err := deployState.Write(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to write state: %v\n", err)
			} else if verbose {
				fmt.Fprintf(os.Stderr, "State: %s\n", state.StatePath())
			}
		}
	}

	// Summary
	backed := len(backupPaths)
	summaryStr := deployTree.CompactString()
	if skipped > 0 {
		summaryStr += fmt.Sprintf(", %d skipped", skipped)
	}
	if backed > 0 {
		summaryStr += fmt.Sprintf(", %d backed up", backed)
	}
	fmt.Printf("\nDeployed %s\n", summaryStr)

	return nil
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
  writ decommission --force noblefactor      # Skip confirmation prompts`,
		Args: cobra.MinimumNArgs(1),
		RunE: runDecommission,
	}

	cmd.Flags().Bool("force", false, "Skip confirmation and proceed with unsigned state")

	return cmd
}

// runDecommission implements the decommission command.
func runDecommission(cmd *cobra.Command, args []string) error {
	dryRun := viper.GetBool("writ.dry-run")
	verbose := viper.GetBool("writ.verbose")
	force, _ := cmd.Flags().GetBool("force")

	// Load state file
	deployState, stateErr := state.Load()
	if stateErr != nil {
		if os.IsNotExist(stateErr) {
			if force {
				fmt.Fprintf(os.Stderr, "Warning: no state file found, proceeding with --force\n")
				fmt.Fprintf(os.Stderr, "Cannot track what was deployed; only scanning for symlinks.\n")
			} else {
				return fmt.Errorf("no state file found; cannot safely remove without deployment state\nUse --force to proceed without state tracking (may leave orphaned files)")
			}
		} else {
			return fmt.Errorf("load state: %w", stateErr)
		}
	}

	// Load identities for signature verification
	identities, identityErr := identity.LoadIdentities()

	// Verify state signature if state exists
	if deployState != nil {
		if deployState.IsSigned() {
			if identityErr != nil {
				return fmt.Errorf("load identities for signature verification: %w", identityErr)
			}
			if err := deployState.Verify(identities); err != nil {
				return fmt.Errorf("state signature invalid, redeploy to regenerate: %v", err)
			}
			if verbose {
				fmt.Fprintf(os.Stderr, "State signature valid\n")
			}
		} else {
			// Unsigned state - warn and require --force
			if !force {
				fmt.Fprintf(os.Stderr, "Warning: state file is unsigned (legacy or tampered)\n")
				return fmt.Errorf("unsigned state file; use --force to proceed or redeploy to regenerate signed state")
			}
			fmt.Fprintf(os.Stderr, "Warning: proceeding with unsigned state (--force)\n")
		}
	}

	// Build project set
	projectSet := make(map[string]bool)
	for _, p := range args {
		projectSet[p] = true
	}

	// Determine prune boundary (for removing empty parent directories)
	var pruneRoot string
	if deployState != nil {
		pruneRoot = deployState.TargetRoot
	} else {
		pruneRoot = os.Getenv("HOME")
	}

	// Collect files to remove
	type removeEntry struct {
		RelTarget      string
		Target         string
		Source         string
		IsSymlink      bool
		IsCopied       bool
		SourceChanged  bool
		TargetModified bool
		Project        string
	}

	var toRemove []removeEntry

	if deployState != nil {
		// Use state file for accurate removal
		for relTarget, entry := range deployState.Files {
			if !projectSet[entry.Project] {
				continue
			}

			target := filepath.Join(deployState.TargetRoot, relTarget)
			re := removeEntry{
				RelTarget: relTarget,
				Target:    target,
				Source:    entry.Source,
				IsSymlink: entry.IsLinked(),
				IsCopied:  entry.IsCopied(),
				Project:   entry.Project,
			}

			// Check drift for copied files
			if entry.IsCopied() && entry.SourceChecksum != "" {
				currentSourceChecksum := receipt.ChecksumFile(entry.Source)
				currentTargetChecksum := receipt.ChecksumFile(target)

				re.SourceChanged = currentSourceChecksum != "" && currentSourceChecksum != entry.SourceChecksum
				re.TargetModified = currentTargetChecksum != "" && currentTargetChecksum != entry.TargetChecksum
			}

			toRemove = append(toRemove, re)
		}
	} else {
		// No state - scan target for symlinks only (--force mode)
		sourceRoot := cli.GetString("writ", "repo", true)
		if sourceRoot == "" {
			return fmt.Errorf("no repo configured; set writ.repo in config or use WRIT_REPO env var")
		}
		sourceRoot = expandPath(sourceRoot)

		targetRoot := os.Getenv("HOME")
		if targetRoot == "" {
			return fmt.Errorf("HOME environment variable not set")
		}

		// Walk target looking for symlinks that point to our source
		err := filepath.Walk(targetRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip inaccessible paths
			}
			if info.Mode()&os.ModeSymlink == 0 {
				return nil // Not a symlink
			}

			linkTarget, err := os.Readlink(path)
			if err != nil {
				return nil
			}

			// Resolve relative symlinks
			if !filepath.IsAbs(linkTarget) {
				linkTarget = filepath.Join(filepath.Dir(path), linkTarget)
			}
			linkTarget = filepath.Clean(linkTarget)

			// Check if it points to our source root
			if strings.HasPrefix(linkTarget, sourceRoot) {
				// Try to determine project from path
				relSource := strings.TrimPrefix(linkTarget, sourceRoot+"/")
				parts := strings.SplitN(relSource, "/", 3) // Home/<project>/...
				if len(parts) >= 2 {
					project := parts[1]
					// Extract base project name (strip segments like .Darwin)
					if idx := strings.Index(project, "."); idx > 0 {
						project = project[:idx]
					}

					if projectSet[project] {
						relTarget, _ := filepath.Rel(targetRoot, path)
						toRemove = append(toRemove, removeEntry{
							RelTarget: relTarget,
							Target:    path,
							Source:    linkTarget,
							IsSymlink: true,
							Project:   project,
						})
					}
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("scan target: %w", err)
		}
	}

	if len(toRemove) == 0 {
		fmt.Println("No files found for specified projects.")
		return nil
	}

	// Sort entries for consistent output
	sort.Slice(toRemove, func(i, j int) bool {
		return toRemove[i].RelTarget < toRemove[j].RelTarget
	})

	// Show drift detection results
	var symlinks, copied, modified int
	for _, re := range toRemove {
		if re.IsSymlink {
			symlinks++
		} else if re.IsCopied {
			copied++
			if re.TargetModified {
				modified++
			}
		}
	}

	fmt.Printf("Files to remove for %v:\n", args)
	fmt.Printf("  %d symlinks\n", symlinks)
	if copied > 0 {
		fmt.Printf("  %d copied files", copied)
		if modified > 0 {
			fmt.Printf(" (%d locally modified)", modified)
		}
		fmt.Println()
	}

	// Show modified files that need attention
	if modified > 0 && !force {
		fmt.Fprintf(os.Stderr, "\nLocally modified files (will be preserved unless --force):\n")
		for _, re := range toRemove {
			if re.TargetModified {
				fmt.Fprintf(os.Stderr, "  M %s\n", re.RelTarget)
			}
		}
	}

	// Dry-run: show what would happen
	if dryRun {
		fmt.Println("\nDry-run: would remove:")
		for _, re := range toRemove {
			if re.TargetModified && !force {
				fmt.Printf("  skip %s (locally modified)\n", re.RelTarget)
			} else if re.IsSymlink {
				fmt.Printf("  unlink %s\n", re.RelTarget)
			} else {
				fmt.Printf("  remove %s\n", re.RelTarget)
			}
		}
		return nil
	}

	// Perform removal
	var removed, skipped int
	var signingIdentity *age.X25519Identity
	if identityErr == nil {
		for _, id := range identities {
			if x, ok := id.(*age.X25519Identity); ok {
				signingIdentity = x
				break
			}
		}
	}

	for _, re := range toRemove {
		// Skip modified files unless --force
		if re.TargetModified && !force {
			skipped++
			if verbose {
				fmt.Fprintf(os.Stderr, "  skip %s (locally modified)\n", re.RelTarget)
			}
			continue
		}

		// Check if target exists
		if _, err := os.Lstat(re.Target); os.IsNotExist(err) {
			if verbose {
				fmt.Fprintf(os.Stderr, "  skip %s (already removed)\n", re.RelTarget)
			}
			continue
		}

		// Remove the file/symlink
		if err := os.Remove(re.Target); err != nil {
			fmt.Fprintf(os.Stderr, "  error removing %s: %v\n", re.RelTarget, err)
			continue
		}

		// Prune empty parent directories up to the target root
		pruneEmptyParentDirs(re.Target, pruneRoot)

		removed++
		if verbose {
			if re.IsSymlink {
				fmt.Fprintf(os.Stderr, "  unlinked %s\n", re.RelTarget)
			} else {
				fmt.Fprintf(os.Stderr, "  removed %s\n", re.RelTarget)
			}
		}

		// Update state file
		if deployState != nil {
			deployState.RemoveEntry(re.RelTarget)
		}
	}

	// Write updated state
	if deployState != nil && removed > 0 {
		if signingIdentity != nil {
			if err := deployState.Sign(signingIdentity); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to sign state: %v\n", err)
			}
		}
		if err := deployState.Write(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write state: %v\n", err)
		} else if verbose {
			fmt.Fprintf(os.Stderr, "State updated: %s\n", state.StatePath())
		}
	}

	// Summary
	fmt.Printf("\nRemoved %d files", removed)
	if skipped > 0 {
		fmt.Printf(", %d skipped (locally modified)", skipped)
	}
	fmt.Println()

	if skipped > 0 && !force {
		fmt.Println("Use --force to remove locally modified files.")
	}

	return nil
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
	dryRun := viper.GetBool("writ.dry-run")
	verbose := viper.GetBool("writ.verbose")
	force, _ := cmd.Flags().GetBool("force")

	// Load state file
	deployState, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w (run 'writ deploy' first to create state)", err)
	}

	// Resolve source root from config
	if cli.GetString("writ", "repo", true) == "" {
		return fmt.Errorf("no repo configured; set writ.repo in config or use WRIT_REPO env var")
	}

	// Get segments for template expansion
	segs := segment.DetectSegments()
	segs = segs.LoadFromEnv()

	// Build segments map for template data
	segMap := make(map[string]string)
	for _, seg := range segs {
		if seg.Value != "" {
			segMap[seg.Name] = seg.Value
		}
	}

	// Load template variables from config
	templateData := make(map[string]any)
	if varsMap := viper.GetStringMapString("writ.vars"); varsMap != nil {
		for k, v := range varsMap {
			templateData[k] = v
		}
	}

	// Load age identities for decryption and signing
	identities, identityErr := identity.LoadIdentities()

	// Get X25519 identity for signing
	var signingIdentity *age.X25519Identity
	if identityErr == nil {
		for _, id := range identities {
			if x, ok := id.(*age.X25519Identity); ok {
				signingIdentity = x
				break
			}
		}
	}

	// Get copied files to regenerate
	copied := deployState.CopiedFiles()

	// Filter by project if specified
	if len(args) > 0 {
		projectSet := make(map[string]bool)
		for _, p := range args {
			projectSet[p] = true
		}
		filtered := make(map[string]*state.FileEntry)
		for relTarget, entry := range copied {
			if projectSet[entry.Project] {
				filtered[relTarget] = entry
			}
		}
		copied = filtered
	}

	if len(copied) == 0 {
		fmt.Println("No copied files to upgrade.")
		return nil
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Upgrading %d copied file(s)...\n", len(copied))
	}

	// Build engine data (template vars + builtins + decryptor)
	engineData := builtinTemplateData(segMap)
	for k, v := range templateData {
		engineData[k] = v
	}
	// Set up SOPS decryptor for encrypted files (.age, .sops)
	// SOPS handles key resolution via .sops.yaml + environment variables
	upgradeSecretsMgr, _ := secrets.NewManager(deployState.SourceRoot)
	engineData["decryptor"] = upgradeSecretsMgr.Decryptor()

	// Track results
	var regenerated, skipped int
	var skippedFiles []string

	for relTarget, entry := range copied {
		// Check for drift
		currentSourceChecksum := receipt.ChecksumFile(entry.Source)
		currentTargetChecksum := receipt.ChecksumFile(filepath.Join(deployState.TargetRoot, relTarget))

		sourceChanged := currentSourceChecksum != "" && entry.SourceChecksum != "" && currentSourceChecksum != entry.SourceChecksum
		targetChanged := currentTargetChecksum != "" && entry.TargetChecksum != "" && currentTargetChecksum != entry.TargetChecksum

		// Handle drift
		if targetChanged && !force {
			skipped++
			skippedFiles = append(skippedFiles, relTarget)
			if verbose {
				if sourceChanged {
					fmt.Fprintf(os.Stderr, "  ⚠ %s (skipped: both source and target changed)\n", relTarget)
				} else {
					fmt.Fprintf(os.Stderr, "  ⚠ %s (skipped: locally modified)\n", relTarget)
				}
			}
			continue
		}

		// Determine operations from source file
		_, ops := tree.ProcessingPipeline(filepath.Base(entry.Source))
		opStrings := ops.Strings()

		// Check if we need identities for decryption
		if hasDecryptOp(opStrings) && identityErr != nil {
			return fmt.Errorf("load identities: %w (required for encrypted files)", identityErr)
		}

		// Build an engine node for this file
		target := filepath.Join(deployState.TargetRoot, relTarget)
		node := &engine.Node{
			ID:         relTarget,
			Operations: opStrings,
			Source:     entry.Source,
			Target:     target,
			Project:    entry.Project,
		}

		// Set restricted permissions for secrets
		if hasDecryptOp(opStrings) {
			node.Mode = 0600
		}

		// Execute via single-node graph
		registry := engine.NewRegistry()
		for _, op := range engine.FileOps() {
			registry.Register(op)
		}
		eng := engine.New(registry, engine.Options{
			DryRun:             dryRun,
			Data:               engineData,
			ConflictResolution: engine.ResolutionOverwrite,
		})

		graph := &engine.Graph{Nodes: []*engine.Node{node}}
		results, runErr := eng.Run(context.Background(), graph)
		if runErr != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", relTarget, runErr)
			continue
		}
		if len(results) > 0 && results[0].Status == engine.StatusFailed {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", relTarget, results[0].Error)
			continue
		}

		regenerated++
		if verbose {
			if targetChanged && force {
				fmt.Fprintf(os.Stderr, "  ✓ %s (regenerated, local changes overwritten)\n", relTarget)
			} else {
				fmt.Fprintf(os.Stderr, "  ✓ %s (regenerated)\n", relTarget)
			}
		}

		// Update checksums in state
		if !dryRun {
			newSourceChecksum := receipt.ChecksumFile(entry.Source)
			newTargetChecksum := receipt.ChecksumFile(target)
			deployState.UpdateChecksum(relTarget, newSourceChecksum, newTargetChecksum)
		}
	}

	// Write updated state
	if !dryRun && regenerated > 0 {
		if signingIdentity != nil {
			if err := deployState.Sign(signingIdentity); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to sign state: %v\n", err)
			}
		}
		if err := deployState.Write(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write state: %v\n", err)
		}
	}

	// Summary
	if skipped > 0 {
		fmt.Printf("\n%d file(s) regenerated, %d skipped\n", regenerated, skipped)
		if !verbose {
			fmt.Println("Skipped files (locally modified):")
			for _, f := range skippedFiles {
				fmt.Printf("  %s\n", f)
			}
		}
		fmt.Println("\nUse --force to overwrite locally modified files.")
	} else {
		fmt.Printf("\n%d file(s) regenerated\n", regenerated)
	}

	return nil
}

func newReconcileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconcile [<project>...]",
		Short: "Full-stack drift detection and repair",
		Long: `Full-stack drift detection and repair.

Checks symlinks, copied files (templates/secrets), and optionally installed
packages against the state file. Can automatically repair detected issues.

Without arguments, scans target directory for writ-managed files.
With project arguments, builds a fresh tree and checks against expected state.

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

// runReconcile implements the reconcile command.
func runReconcile(cmd *cobra.Command, args []string) error {
	verbose := viper.GetBool("writ.verbose")
	checkDrift, _ := cmd.Flags().GetBool("drift")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	// Resolve source root from config
	sourceRoot := cli.GetString("writ", "repo", true)
	if sourceRoot == "" {
		return fmt.Errorf("no repo configured; set writ.repo in config or use WRIT_REPO env var")
	}
	sourceRoot = expandPath(sourceRoot)

	// Resolve target root (default: $HOME)
	targetRoot := os.Getenv("HOME")
	if targetRoot == "" {
		return fmt.Errorf("HOME environment variable not set")
	}

	var report *reconcile.Report

	if len(args) > 0 {
		// Projects specified: build tree and check status
		segs := segment.DetectSegments()
		segs = segs.LoadFromEnv()

		deployTree, err := tree.Build(tree.BuildConfig{
			SourceRoot: sourceRoot,
			TargetRoot: targetRoot,
			Projects:   args,
			Segments:   segs,
		})
		if err != nil {
			return fmt.Errorf("build tree: %w", err)
		}

		report = reconcile.FromBuildResult(deployTree)
	} else {
		// No projects: prefer state file, fall back to scanning + receipt
		deployState, stateErr := state.Load()
		if stateErr == nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Using state file: %s\n", state.StatePath())
			}

			// Verify signature if drift checking is enabled
			if checkDrift {
				identities, identityErr := identity.LoadIdentities()
				if identityErr != nil {
					return fmt.Errorf("load identities for signature verification: %w", identityErr)
				}

				if err := deployState.Verify(identities); err != nil {
					return fmt.Errorf("state signature invalid, redeploy to regenerate: %v", err)
				}
				if verbose && deployState.IsSigned() {
					fmt.Fprintf(os.Stderr, "State signature valid\n")
				}
			}

			// Build report from state
			report = reconcileFromState(deployState, checkDrift)
		} else {
			// Fall back to scanning + receipt
			report = reconcile.ScanTarget(targetRoot, sourceRoot)

			// Load receipt to check copied files (templates, secrets)
			rcpt, err := receipt.LoadLatest()
			if err == nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Using receipt for copied files: %s\n", receipt.LatestReceiptPath())
				}

				// Verify signature if drift checking is enabled
				if checkDrift {
					// Load identities for verification
					identities, identityErr := identity.LoadIdentities()
					if identityErr != nil {
						return fmt.Errorf("load identities for signature verification: %w", identityErr)
					}

					verifyResult, verifyErr := rcpt.VerifyWithResult(identities)
					switch verifyResult {
					case receipt.VerifyOK:
						if verbose {
							fmt.Fprintf(os.Stderr, "Receipt signature valid\n")
						}
					case receipt.VerifyLegacy:
						if verbose {
							fmt.Fprintf(os.Stderr, "Receipt unsigned (legacy v%s), skipping verification\n", rcpt.Version)
						}
					case receipt.VerifyInvalid, receipt.VerifyMissing:
						return fmt.Errorf("receipt signature invalid, redeploy to regenerate: %v", verifyErr)
					}
				}

				// Add copied files from receipt to the report
				addCopiedFilesFromReceipt(report, rcpt, checkDrift)
			} else if checkDrift {
				return fmt.Errorf("--drift requires state file or receipt; none found")
			} else if verbose {
				fmt.Fprintf(os.Stderr, "No state file or receipt found, showing symlinks only\n")
			}
		}
	}

	// JSON output
	if jsonOutput {
		return outputReconcileJSON(report)
	}

	// Human-readable output
	return outputReconcileText(report)
}

// addCopiedFilesFromReceipt adds copied file nodes from a receipt to the report.
func addCopiedFilesFromReceipt(report *reconcile.Report, rcpt *receipt.Receipt, checkDrift bool) {
	report.FromReceipt = true
	report.ReceiptPath = receipt.LatestReceiptPath()

	for _, n := range rcpt.Nodes {
		// Skip non-file nodes and symlinks
		if n.Status == "skipped" || n.Operation == "delegate" || n.Operation == "backup" {
			continue
		}
		if n.Operation == "link" {
			continue // Symlinks are found by scanning
		}

		var entry reconcile.Entry
		if checkDrift && n.SourceChecksum != "" {
			entry = reconcile.Entry{
				RelTarget:      n.ID,
				Source:         n.Source,
				Target:         n.Target,
				Project:        n.Project,
				Operations:     []string{n.Operation},
				SourceChecksum: n.SourceChecksum,
				TargetChecksum: n.TargetChecksum,
			}
			// Check drift
			currentSourceChecksum := receipt.ChecksumFile(n.Source)
			currentTargetChecksum := receipt.ChecksumFile(n.Target)

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
				Source:     n.Source,
				Target:     n.Target,
				Project:    n.Project,
				Operations: []string{n.Operation},
			}
			if _, err := os.Stat(n.Target); os.IsNotExist(err) {
				entry.State = reconcile.StateMissing
				entry.Message = "file not deployed"
			} else {
				entry.State = reconcile.StateCopied
			}
		}

		report.Entries = append(report.Entries, entry)
	}
}

// statusFromState builds a status report from the state file.
func reconcileFromState(s *state.State, checkDrift bool) *reconcile.Report {
	report := &reconcile.Report{
		TargetRoot:  s.TargetRoot,
		SourceRoot:  s.SourceRoot,
		Projects:    s.Projects(),
		FromReceipt: true, // State file is the source
		ReceiptPath: state.StatePath(),
	}

	for relTarget, entry := range s.Files {
		target := filepath.Join(s.TargetRoot, relTarget)

		statusEntry := reconcile.Entry{
			RelTarget:      relTarget,
			Source:         entry.Source,
			Target:         target,
			Project:        entry.Project,
			Operations:     entry.Operations,
			SourceChecksum: entry.SourceChecksum,
			TargetChecksum: entry.TargetChecksum,
		}

		if entry.IsCopied() {
			// Copied file - check existence and optionally drift
			if _, err := os.Stat(target); os.IsNotExist(err) {
				statusEntry.State = reconcile.StateMissing
				statusEntry.Message = "file not deployed"
			} else if checkDrift && entry.SourceChecksum != "" {
				// Check drift
				currentSourceChecksum := receipt.ChecksumFile(entry.Source)
				currentTargetChecksum := receipt.ChecksumFile(target)

				sourceChanged := currentSourceChecksum != "" && currentSourceChecksum != entry.SourceChecksum
				targetChanged := currentTargetChecksum != "" && currentTargetChecksum != entry.TargetChecksum

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

With --from-receipt, reads a lore receipt and adopts packages.manifest and
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
	cmd.Flags().Bool("from-receipt", false, "Adopt packages.manifest and config from lore receipt")

	return cmd
}

// runAdopt implements the adopt command.
func runAdopt(cmd *cobra.Command, args []string) error {
	verbose := viper.GetBool("writ.verbose")
	dryRun := viper.GetBool("writ.dry-run")
	layer, _ := cmd.Flags().GetString("layer")
	project, _ := cmd.Flags().GetString("project")
	fromReceipt, _ := cmd.Flags().GetBool("from-receipt")

	// Handle --from-receipt mode
	if fromReceipt {
		receiptPath := ""
		if len(args) > 0 {
			receiptPath = args[0]
		}
		return runAdoptFromReceipt(receiptPath, layer, project, verbose, dryRun)
	}

	// Normal mode: adopt --project=<project> <item>...
	if project == "" {
		return fmt.Errorf("--project is required")
	}
	if len(args) < 1 {
		return fmt.Errorf("requires at least 1 item to adopt")
	}

	files := args

	// Validate layer
	if layer != "personal" && layer != "team" && layer != "base" {
		return fmt.Errorf("invalid --layer %q: must be personal, team, or base", layer)
	}

	// Get layer path
	layerPath := filepath.Join(cli.WritLayersDir(), layer)
	if _, err := os.Stat(layerPath); os.IsNotExist(err) {
		return fmt.Errorf("layer %q does not exist at %s\nRun 'writ self-install' to create layers", layer, layerPath)
	}

	// Determine HOME for scoping
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		return fmt.Errorf("HOME environment variable not set")
	}

	if verbose {
		cli.Note("Layer: %s", layer)
		cli.Note("Layer path: %s", layerPath)
		cli.Note("Project: %s", project)
	}

	// Process each file
	var adopted int
	for _, file := range files {
		// Expand ~ in file path
		filePath := expandPath(file)

		// Make absolute if relative
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(homeDir, filePath)
		}

		// Determine scope based on file location
		scope := inferScope(filePath, homeDir)
		projectDir := filepath.Join(layerPath, scope, project)

		if verbose {
			cli.Note("File: %s -> scope: %s", filePath, scope)
		}

		// Verify file exists and is not a symlink
		info, err := os.Lstat(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				cli.Error("%s: file does not exist", file)
				continue
			}
			cli.Error("%s: %v", file, err)
			continue
		}

		if info.Mode()&os.ModeSymlink != 0 {
			cli.Warn("%s: already a symlink (skip)", file)
			continue
		}

		// Determine target root for this scope
		targetRoot := homeDir
		if scope == "System" {
			targetRoot = "/"
		}

		// Handle directories recursively
		if info.IsDir() {
			err := filepath.WalkDir(filePath, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					cli.Error("%s: %v", path, walkErr)
					return nil // Continue walking
				}

				// Skip directories themselves (we only adopt files)
				if d.IsDir() {
					return nil
				}

				// Skip symlinks
				fileInfo, err := d.Info()
				if err != nil {
					cli.Error("%s: %v", path, err)
					return nil
				}
				if fileInfo.Mode()&os.ModeSymlink != 0 {
					cli.Warn("%s: already a symlink (skip)", path)
					return nil
				}

				// Adopt this file
				count, err := adoptFile(path, targetRoot, projectDir, verbose, dryRun)
				if err != nil {
					cli.Error("%s: %v", path, err)
					return nil
				}
				adopted += count
				return nil
			})
			if err != nil {
				cli.Error("walking directory %s: %v", file, err)
			}
			continue
		}

		// Single file adoption
		count, err := adoptFile(filePath, targetRoot, projectDir, verbose, dryRun)
		if err != nil {
			cli.Error("%s: %v", file, err)
			continue
		}
		adopted += count
	}

	if dryRun {
		cli.Note("Dry-run: would adopt %d file(s)", adopted)
	} else {
		cli.Success("Adopted %d file(s) into %s/%s", adopted, layer, project)
		if adopted > 0 {
			cli.Note("Remember to commit: cd %s && git add -A && git commit", layerPath)
		}
	}

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
	// TODO: Implement reading lore receipt and adopting packages.manifest + config
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
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

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

For a project: shows source location, deployed files, segments, and state.
For a file path: shows source, target, operations, checksums, and drift status.

Output is JSON by default for scripting. Use --format for alternatives.`,
		Example: `  writ inspect noblefactor
  writ inspect ~/.zshrc
  writ inspect noblefactor --format yaml
  writ inspect noblefactor --format '{{.Name}}\t{{.Source}}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("inspect: not yet implemented")
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
			fmt.Println("list: not yet implemented")
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
of your environment state.`,
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
				fmt.Printf("receipt show %s --unified: not yet implemented\n", name)
			} else {
				fmt.Printf("receipt show %s: not yet implemented\n", name)
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
			fmt.Println("receipt list: not yet implemented")
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

// isDelegate returns true if the node is a delegate operation.
func isDelegate(node *engine.Node) bool {
	return len(node.Operations) == 1 && node.Operations[0] == "delegate"
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

// pruneEmptyParentDirs removes empty parent directories up to (but not including) boundary.
// Stops at non-empty directories or on any error.
func pruneEmptyParentDirs(target, boundary string) {
	if boundary == "" {
		return
	}

	boundary = filepath.Clean(boundary)
	dir := filepath.Dir(target)

	for {
		// Stop at or above boundary
		if dir == boundary || dir == "/" || dir == "." {
			return
		}

		// Check if dir is under boundary
		rel, err := filepath.Rel(boundary, dir)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return
		}

		// Try to remove (fails if not empty)
		if err := os.Remove(dir); err != nil {
			return // Not empty or permission error
		}

		// Move up
		dir = filepath.Dir(dir)
	}
}

// DryRunOutput represents the dry-run output format.
type DryRunOutput struct {
	SourceRoot string       `json:"source_root"`
	TargetRoot string       `json:"target_root"`
	Projects   []string     `json:"projects"`
	Nodes      []DryRunNode `json:"nodes"`
}

// DryRunNode represents a node in the dry-run output.
type DryRunNode struct {
	ID         string   `json:"id"`
	Operations []string `json:"operations"`
	Source     string   `json:"source"`
	Target     string   `json:"target"`
	Project    string   `json:"project"`
}

// outputDryRun outputs the deployment plan as JSON.
func outputDryRun(br *tree.BuildResult) error {
	output := DryRunOutput{
		SourceRoot: br.SourceRoot,
		TargetRoot: br.TargetRoot,
		Projects:   br.Projects,
	}

	for _, n := range br.Graph.Nodes {
		output.Nodes = append(output.Nodes, DryRunNode{
			ID:         n.ID,
			Operations: n.Operations,
			Source:     n.Source,
			Target:     n.Target,
			Project:    n.Project,
		})
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
