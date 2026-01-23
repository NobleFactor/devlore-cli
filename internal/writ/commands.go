// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package writ

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	osexec "os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/writ/exec"
	"github.com/NobleFactor/devlore-cli/internal/writ/receipt"
	"github.com/NobleFactor/devlore-cli/internal/writ/segment"
	"github.com/NobleFactor/devlore-cli/internal/writ/state"
	"github.com/NobleFactor/devlore-cli/internal/writ/status"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [flags] <project>...",
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
		Example: `  writ add noblefactor
  writ add all noblefactor thenobles
  writ add --conflict=backup noblefactor
  writ add --conflict=overwrite noblefactor
  writ add -s ROLE=desktop noblefactor`,
		Args: cobra.MinimumNArgs(1),
		RunE: runAdd,
	}

	cmd.Flags().StringP("conflict", "c", "stop", "Conflict resolution: stop, backup, overwrite, skip")
	cmd.Flags().StringArrayP("segment", "s", nil, "Set custom segment value (KEY=value, repeatable)")

	return cmd
}

// runAdd implements the add command.
func runAdd(cmd *cobra.Command, args []string) error {
	// Get config values
	dryRun := viper.GetBool("writ.dry-run")
	verbose := viper.GetBool("writ.verbose")
	conflictFlag, _ := cmd.Flags().GetString("conflict")
	segmentFlags, _ := cmd.Flags().GetStringArray("segment")

	// Determine conflict resolution strategy
	var resolution exec.ConflictResolution
	switch conflictFlag {
	case "stop", "":
		resolution = exec.ResolutionStop
	case "backup":
		resolution = exec.ResolutionBackup
	case "overwrite":
		resolution = exec.ResolutionOverwrite
	case "skip":
		resolution = exec.ResolutionSkip
	default:
		return fmt.Errorf("invalid --conflict value %q: must be stop, backup, overwrite, or skip", conflictFlag)
	}

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
		fmt.Fprintf(os.Stderr, "Source: %s\n", sourceRoot)
		fmt.Fprintf(os.Stderr, "Target: %s\n", targetRoot)
		fmt.Fprintf(os.Stderr, "Projects: %v\n", args)
		fmt.Fprintf(os.Stderr, "Segments: %s\n", segs.String())
	}

	// Build deployment tree
	deployTree, err := tree.Build(tree.BuildConfig{
		SourceRoot: sourceRoot,
		TargetRoot: targetRoot,
		Projects:   args,
		Segments:   segs,
	})
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}

	// Warn about source collisions (same target from different source dirs)
	if deployTree.HasCollisions() {
		fmt.Fprintf(os.Stderr, "Warning: %d source collision(s) resolved by specificity:\n", len(deployTree.Collisions))
		for _, c := range deployTree.Collisions {
			fmt.Fprintf(os.Stderr, "  %s: using %s (specificity %d) over %s (specificity %d)\n",
				c.Target, c.Winner, c.WinnerSpecificity, c.Loser, c.LoserSpecificity)
		}
	}

	// Load age identities for decryption and signing
	identities, identityErr := exec.LoadIdentities()
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

	// Create executor
	executor := &exec.Executor{
		DryRun:             dryRun,
		ConflictResolution: resolution,
		Identities:         identities,
		TemplateData:       templateData,
		Segments:           segMap,
		Output:             os.Stdout,
	}

	// Pre-flight conflict detection
	preflight := executor.Preflight(deployTree)

	// Report conflicts upfront
	if len(preflight.Conflicts) > 0 {
		fmt.Fprintf(os.Stderr, "\nConflicts detected (%d):\n", len(preflight.Conflicts))
		for _, c := range preflight.Conflicts {
			fmt.Fprintf(os.Stderr, "  %s: %s (%s)\n", c.Node.RelTarget, c.Message, c.Type)
		}

		if resolution == exec.ResolutionStop {
			fmt.Fprintf(os.Stderr, "\nUse --backup, --overwrite, or --skip to resolve conflicts.\n")
			return fmt.Errorf("%d conflict(s) detected", len(preflight.Conflicts))
		}

		// Show what will happen
		switch resolution {
		case exec.ResolutionBackup:
			fmt.Fprintf(os.Stderr, "\nBacking up conflicting files...\n")
		case exec.ResolutionOverwrite:
			fmt.Fprintf(os.Stderr, "\nOverwriting conflicting files...\n")
		case exec.ResolutionSkip:
			fmt.Fprintf(os.Stderr, "\nSkipping conflicting files...\n")
		}
	}

	// Report already deployed (verbose only)
	if verbose && len(preflight.AlreadyDone) > 0 {
		fmt.Fprintf(os.Stderr, "\nAlready deployed (%d):\n", len(preflight.AlreadyDone))
		for _, c := range preflight.AlreadyDone {
			fmt.Fprintf(os.Stderr, "  %s\n", c.Node.RelTarget)
		}
	}

	// Execute deployment
	results, err := executor.Execute(deployTree)
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	// Build receipt (skip for dry-run)
	var rcpt *receipt.Receipt
	if !dryRun {
		rcpt = receipt.New(sourceRoot, targetRoot, args, segMap)

		// Populate receipt from results
		for _, r := range results {
			if r.Node == nil {
				continue
			}

			if r.Node.IsDelegate() {
				rcpt.AddDelegated(r.Node.Source)
				continue
			}

			if r.Skipped {
				rcpt.AddSkipped(r.Node.RelTarget)
				continue
			}

			// Determine if already deployed (symlink already correct)
			alreadyDeployed := r.Message == "already deployed"

			// Use checksums for copied files (templates, secrets)
			if r.SourceChecksum != "" || r.TargetChecksum != "" {
				rcpt.AddEntryWithChecksums(r.Node, alreadyDeployed, r.SourceChecksum, r.TargetChecksum)
			} else {
				rcpt.AddEntry(r.Node, alreadyDeployed)
			}
		}

		// Record backups (from preflight conflicts when using backup resolution)
		if resolution == exec.ResolutionBackup {
			timestamp := time.Now().Format("20060102-150405")
			for _, c := range preflight.Conflicts {
				backupPath := c.Node.Target + ".writ-backup." + timestamp
				rcpt.AddBackup(c.Node.Target, backupPath)
			}
		}
	}

	// Count results for summary
	var deployed, skipped, backed int
	for _, r := range results {
		if r.Skipped {
			skipped++
		} else if r.Success {
			deployed++
		}
	}
	backed = len(preflight.Conflicts)
	if resolution != exec.ResolutionBackup {
		backed = 0
	}

	// Handle delegated nodes (packages.manifest files)
	delegated := exec.DelegatedNodes(results)
	if len(delegated) > 0 && !dryRun {
		fmt.Fprintf(os.Stderr, "\nDelegated to lore (%d manifests):\n", len(delegated))
		for _, node := range delegated {
			fmt.Fprintf(os.Stderr, "  %s\n", node.Source)
		}
		fmt.Fprintf(os.Stderr, "Run 'lore apply' to install packages.\n")
	}

	// Sign and write receipt
	var receiptFilename string
	if !dryRun && rcpt != nil {
		// Sign receipt if we have an age identity
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
	}

	// Update state file
	if !dryRun && rcpt != nil && receiptFilename != "" {
		deployState, err := state.LoadOrCreate(sourceRoot, targetRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load state: %v\n", err)
		} else {
			// Merge receipt entries into state
			deployState.UpdateFromReceipt(rcpt, receiptFilename)

			// Sign state if we have an identity
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

	// Summary (unless dry-run, which outputs JSON)
	if !dryRun {
		summary := deployTree.CompactString()
		if skipped > 0 {
			summary += fmt.Sprintf(", %d skipped", skipped)
		}
		if backed > 0 {
			summary += fmt.Sprintf(", %d backed up", backed)
		}
		fmt.Printf("\nDeployed %s\n", summary)
	}

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

func newRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [flags] <project>...",
		Short: "Remove deployed files for the specified projects",
		Long: `Remove deployed files for the specified projects.

Symlinks are removed directly. Copied files (templates, secrets) are removed
only after drift detection confirms they haven't been locally modified.

Safety behavior depends on state file:
  Signed state    → Safe: full drift detection before removal
  Unsigned state  → Warning, requires --force to proceed
  No state        → Error: cannot safely remove without state

With --decommission, delegates to lore to remove orphaned software packages
that were installed via packages.manifest files in the removed projects.`,
		Example: `  writ remove noblefactor              # Remove project files
  writ remove all noblefactor          # Remove multiple projects
  writ remove --force noblefactor      # Skip confirmation prompts
  writ remove --decommission noblefactor  # Also remove installed software`,
		Args: cobra.MinimumNArgs(1),
		RunE: runRemove,
	}

	cmd.Flags().Bool("decommission", false, "Delegate to lore to decommission orphaned software")
	cmd.Flags().Bool("force", false, "Skip confirmation and proceed with unsigned state")

	return cmd
}

// runRemove implements the remove command.
func runRemove(cmd *cobra.Command, args []string) error {
	dryRun := viper.GetBool("writ.dry-run")
	verbose := viper.GetBool("writ.verbose")
	force, _ := cmd.Flags().GetBool("force")
	decommission, _ := cmd.Flags().GetBool("decommission")

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
	identities, identityErr := exec.LoadIdentities()

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
	var manifestPaths []string // For decommission delegation

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

			// Track manifest paths for decommission
			if decommission && strings.HasSuffix(entry.Source, "packages.manifest") {
				manifestPaths = append(manifestPaths, entry.Source)
			}
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
		if decommission && len(manifestPaths) > 0 {
			fmt.Println("\nWould delegate to lore decommission:")
			for _, p := range manifestPaths {
				fmt.Printf("  %s\n", p)
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

	// Handle decommission delegation
	if decommission && len(manifestPaths) > 0 {
		fmt.Fprintf(os.Stderr, "\nDelegating to lore for decommission:\n")
		for _, p := range manifestPaths {
			fmt.Fprintf(os.Stderr, "  %s\n", p)
		}
		fmt.Fprintf(os.Stderr, "Run: lore decommission --manifests %s\n", strings.Join(manifestPaths, ","))
		// TODO: Actually invoke lore when available
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
		return fmt.Errorf("load state: %w (run 'writ add' first to create state)", err)
	}

	// Resolve source root from config
	sourceRoot := cli.GetString("writ", "repo", true)
	if sourceRoot == "" {
		return fmt.Errorf("no repo configured; set writ.repo in config or use WRIT_REPO env var")
	}
	sourceRoot = expandPath(sourceRoot)

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
	identities, identityErr := exec.LoadIdentities()

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

	// Create executor for regeneration
	executor := &exec.Executor{
		DryRun:             dryRun,
		ConflictResolution: exec.ResolutionOverwrite, // We handle drift ourselves
		Identities:         identities,
		TemplateData:       templateData,
		Segments:           segMap,
		Output:             os.Stdout,
	}

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

		// Build a tree node for this file
		node := &tree.Node{
			Source:    entry.Source,
			Target:    filepath.Join(deployState.TargetRoot, relTarget),
			RelTarget: relTarget,
			Project:   entry.Project,
		}

		// Determine operations from source file
		targetName, ops := tree.ProcessingPipeline(filepath.Base(entry.Source))
		_ = targetName // We already have the target
		node.Operations = ops

		// Check if we need identities for decryption
		if node.IsSecret() && identityErr != nil {
			return fmt.Errorf("load identities: %w (required for encrypted files)", identityErr)
		}

		// Execute the regeneration
		result := executor.ExecuteNode(node)
		if !result.Success {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %s\n", relTarget, result.Message)
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
			newTargetChecksum := receipt.ChecksumFile(node.Target)
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

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [<project>...]",
		Short: "Show symlink status for projects",
		Long: `Show symlink status for projects.

Without arguments, scans target directory for writ-managed symlinks and uses
the receipt to check copied files (templates, secrets).

With project arguments, builds a fresh tree and checks status against expected state.

Status indicators:
  ✓ Linked   — Symlink exists and points to project
  ✓ Copied   — File was copied (template/secret) and exists
  ⚠ Conflict — File exists but isn't our symlink
  ✗ Missing  — Project file has no corresponding symlink
  ? Orphan   — Symlink points to nonexistent file

With --drift (requires receipt):
  ↑ Stale    — Source changed since deployment, redeploy needed
  M Modified — Target file was edited locally
  ! Conflict — Both source and target changed`,
		Example: `  writ status                    # Scan for deployed files
  writ status noblefactor        # Check specific project
  writ status --drift            # Check for drift in copied files`,
		RunE: runStatus,
	}

	cmd.Flags().Bool("drift", false, "Check for drift in copied files using receipt checksums")
	cmd.Flags().Bool("json", false, "Output as JSON")

	return cmd
}

// runStatus implements the status command.
func runStatus(cmd *cobra.Command, args []string) error {
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

	var report *status.Report

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

		report = status.FromTree(deployTree)
	} else {
		// No projects: prefer state file, fall back to scanning + receipt
		deployState, stateErr := state.Load()
		if stateErr == nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Using state file: %s\n", state.StatePath())
			}

			// Verify signature if drift checking is enabled
			if checkDrift {
				identities, identityErr := exec.LoadIdentities()
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
			report = statusFromState(deployState, checkDrift)
		} else {
			// Fall back to scanning + receipt
			report = status.ScanTarget(targetRoot, sourceRoot)

			// Load receipt to check copied files (templates, secrets)
			rcpt, err := receipt.LoadLatest()
			if err == nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "Using receipt for copied files: %s\n", receipt.LatestReceiptPath())
				}

				// Verify signature if drift checking is enabled
				if checkDrift {
					// Load identities for verification
					identities, identityErr := exec.LoadIdentities()
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
		return outputStatusJSON(report)
	}

	// Human-readable output
	return outputStatusText(report)
}

// addCopiedFilesFromReceipt adds copied file entries from a receipt to the report.
func addCopiedFilesFromReceipt(report *status.Report, rcpt *receipt.Receipt, checkDrift bool) {
	report.FromReceipt = true
	report.ReceiptPath = receipt.LatestReceiptPath()

	for _, e := range rcpt.Entries {
		if !e.IsCopied() {
			continue // Skip symlinks, they're found by scanning
		}

		var entry status.Entry
		if checkDrift && e.SourceChecksum != "" {
			entry = status.Entry{
				RelTarget:      e.RelTarget,
				Source:         e.Source,
				Target:         e.Target,
				Project:        e.Project,
				Operations:     e.Operations,
				SourceChecksum: e.SourceChecksum,
				TargetChecksum: e.TargetChecksum,
			}
			// Check drift
			currentSourceChecksum := receipt.ChecksumFile(e.Source)
			currentTargetChecksum := receipt.ChecksumFile(e.Target)

			sourceChanged := currentSourceChecksum != "" && currentSourceChecksum != e.SourceChecksum
			targetChanged := currentTargetChecksum != "" && currentTargetChecksum != e.TargetChecksum

			switch {
			case sourceChanged && targetChanged:
				entry.State = status.StateDriftConflict
				entry.Message = "both source and target changed"
			case sourceChanged:
				entry.State = status.StateStale
				entry.Message = "source changed, redeploy needed"
			case targetChanged:
				entry.State = status.StateModified
				entry.Message = "target modified locally"
			default:
				entry.State = status.StateCopied
			}
		} else {
			// Just check if file exists
			entry = status.Entry{
				RelTarget:  e.RelTarget,
				Source:     e.Source,
				Target:     e.Target,
				Project:    e.Project,
				Operations: e.Operations,
			}
			if _, err := os.Stat(e.Target); os.IsNotExist(err) {
				entry.State = status.StateMissing
				entry.Message = "file not deployed"
			} else {
				entry.State = status.StateCopied
			}
		}

		report.Entries = append(report.Entries, entry)
	}
}

// statusFromState builds a status report from the state file.
func statusFromState(s *state.State, checkDrift bool) *status.Report {
	report := &status.Report{
		TargetRoot:  s.TargetRoot,
		SourceRoot:  s.SourceRoot,
		Projects:    s.Projects(),
		FromReceipt: true, // State file is the source
		ReceiptPath: state.StatePath(),
	}

	for relTarget, entry := range s.Files {
		target := filepath.Join(s.TargetRoot, relTarget)

		statusEntry := status.Entry{
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
				statusEntry.State = status.StateMissing
				statusEntry.Message = "file not deployed"
			} else if checkDrift && entry.SourceChecksum != "" {
				// Check drift
				currentSourceChecksum := receipt.ChecksumFile(entry.Source)
				currentTargetChecksum := receipt.ChecksumFile(target)

				sourceChanged := currentSourceChecksum != "" && currentSourceChecksum != entry.SourceChecksum
				targetChanged := currentTargetChecksum != "" && currentTargetChecksum != entry.TargetChecksum

				switch {
				case sourceChanged && targetChanged:
					statusEntry.State = status.StateDriftConflict
					statusEntry.Message = "both source and target changed"
				case sourceChanged:
					statusEntry.State = status.StateStale
					statusEntry.Message = "source changed, redeploy needed"
				case targetChanged:
					statusEntry.State = status.StateModified
					statusEntry.Message = "target modified locally"
				default:
					statusEntry.State = status.StateCopied
				}
			} else {
				statusEntry.State = status.StateCopied
			}
		} else {
			// Symlink - check if it exists and points correctly
			info, err := os.Lstat(target)
			if os.IsNotExist(err) {
				statusEntry.State = status.StateMissing
				statusEntry.Message = "symlink not created"
			} else if err != nil {
				statusEntry.State = status.StateConflict
				statusEntry.Message = err.Error()
			} else if info.Mode()&os.ModeSymlink == 0 {
				statusEntry.State = status.StateConflict
				statusEntry.Message = "file exists, not a symlink"
			} else {
				// Check symlink target
				linkTarget, err := os.Readlink(target)
				if err != nil {
					statusEntry.State = status.StateConflict
					statusEntry.Message = "cannot read symlink"
				} else {
					// Resolve relative symlinks
					if !filepath.IsAbs(linkTarget) {
						linkTarget = filepath.Join(filepath.Dir(target), linkTarget)
					}
					linkTarget = filepath.Clean(linkTarget)

					if linkTarget == entry.Source {
						if _, err := os.Stat(entry.Source); os.IsNotExist(err) {
							statusEntry.State = status.StateOrphan
							statusEntry.Message = "source file deleted"
						} else {
							statusEntry.State = status.StateLinked
						}
					} else {
						statusEntry.State = status.StateConflict
						statusEntry.Message = "symlink points to " + linkTarget
					}
				}
			}
		}

		report.Entries = append(report.Entries, statusEntry)
	}

	return report
}

// outputStatusJSON outputs the status report as JSON.
func outputStatusJSON(report *status.Report) error {
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
	jr.Summary.Linked = summary[status.StateLinked]
	jr.Summary.Copied = summary[status.StateCopied]
	jr.Summary.Conflict = summary[status.StateConflict]
	jr.Summary.Missing = summary[status.StateMissing]
	jr.Summary.Orphan = summary[status.StateOrphan]
	jr.Summary.Stale = summary[status.StateStale]
	jr.Summary.Modified = summary[status.StateModified]
	jr.Summary.DriftConflict = summary[status.StateDriftConflict]

	data, err := json.MarshalIndent(jr, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}


// outputStatusText outputs the status report as human-readable text.
func outputStatusText(report *status.Report) error {
	if len(report.Entries) == 0 {
		fmt.Println("No deployed files found.")
		if report.FromReceipt {
			fmt.Printf("(checked receipt: %s)\n", report.ReceiptPath)
		}
		return nil
	}

	// Group entries by project
	byProject := make(map[string][]status.Entry)
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
	linked := summary[status.StateLinked] + summary[status.StateCopied]
	issues := total - linked

	if issues == 0 {
		fmt.Printf("%d files, all deployed correctly\n", total)
	} else {
		fmt.Printf("%d files: %d ok", total, linked)
		if n := summary[status.StateConflict]; n > 0 {
			fmt.Printf(", %d conflict", n)
		}
		if n := summary[status.StateMissing]; n > 0 {
			fmt.Printf(", %d missing", n)
		}
		if n := summary[status.StateOrphan]; n > 0 {
			fmt.Printf(", %d orphan", n)
		}
		if n := summary[status.StateStale]; n > 0 {
			fmt.Printf(", %d stale", n)
		}
		if n := summary[status.StateModified]; n > 0 {
			fmt.Printf(", %d modified", n)
		}
		if n := summary[status.StateDriftConflict]; n > 0 {
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
		Use:   "adopt [flags] <project> <file|dir>...",
		Short: "Move files from target location into a project and create symlinks",
		Long: `Move files from target location into a project and create symlinks.

Use this to bring existing configuration files under version control.
Files are moved to <repo>/<source>/<project>/ preserving their relative path,
then symlinked back to the original location.

Directories are adopted recursively—all files within are moved and symlinked.
Existing symlinks within directories are skipped.

With --from-receipt, reads a lore receipt and adopts packages.manifest and
config files into the environment repository.`,
		Example: `  # Adopt a single file into personal layer
  writ adopt noblefactor .zshrc

  # Adopt multiple files
  writ adopt noblefactor .zshrc .bashrc .config/nvim/init.lua

  # Adopt an entire directory recursively
  writ adopt noblefactor .config/nvim

  # Adopt into team layer
  writ adopt --layer=team shared .editorconfig

  # Adopt from lore receipt
  writ adopt --from-receipt
  writ adopt --from-receipt ~/.local/state/lore/receipts/2026-01-19T14:32:07.yaml`,
		Args: cobra.MinimumNArgs(1),
		RunE: runAdopt,
	}

	cmd.Flags().String("layer", "personal", "Layer to adopt into: personal, team, or base")
	cmd.Flags().String("source", "Home", "Source directory within the repo (e.g., Home, System)")
	cmd.Flags().Bool("from-receipt", false, "Adopt packages.manifest and config from lore receipt")

	return cmd
}

// runAdopt implements the adopt command.
func runAdopt(cmd *cobra.Command, args []string) error {
	verbose := viper.GetBool("writ.verbose")
	dryRun := viper.GetBool("writ.dry-run")
	layer, _ := cmd.Flags().GetString("layer")
	source, _ := cmd.Flags().GetString("source")
	fromReceipt, _ := cmd.Flags().GetBool("from-receipt")

	// Handle --from-receipt mode
	if fromReceipt {
		receiptPath := ""
		if len(args) > 0 {
			receiptPath = args[0]
		}
		return runAdoptFromReceipt(receiptPath, layer, source, verbose, dryRun)
	}

	// Normal mode: adopt <project> <file>...
	if len(args) < 2 {
		return fmt.Errorf("requires at least 2 arguments: <project> <file>...")
	}

	project := args[0]
	files := args[1:]

	// Validate layer
	if layer != "personal" && layer != "team" && layer != "base" {
		return fmt.Errorf("invalid --layer %q: must be personal, team, or base", layer)
	}

	// Get repo path for layer
	repoPath := getConfiguredRepo(layer)
	if repoPath == "" {
		return fmt.Errorf("no repository configured for layer %q\nUse 'writ repo init --layer=%s' or 'writ repo add --layer=%s <path>'", layer, layer, layer)
	}

	// Determine target root (where files currently live)
	targetRoot := os.Getenv("HOME")
	if targetRoot == "" {
		return fmt.Errorf("HOME environment variable not set")
	}

	// Project directory in repo
	projectDir := filepath.Join(repoPath, source, project)

	if verbose {
		cli.Note("Layer: %s", layer)
		cli.Note("Repo: %s", repoPath)
		cli.Note("Source: %s", source)
		cli.Note("Project: %s", project)
		cli.Note("Project dir: %s", projectDir)
		cli.Note("Target root: %s", targetRoot)
	}

	// Process each file
	var adopted int
	for _, file := range files {
		// Expand ~ in file path
		filePath := expandPath(file)

		// Make absolute if relative
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(targetRoot, filePath)
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
		cli.Success("Adopted %d file(s) into %s/%s", adopted, source, project)
		if adopted > 0 {
			cli.Note("Remember to commit: cd %s && git add -A && git commit", repoPath)
		}
	}

	return nil
}

// runAdoptFromReceipt adopts files from a lore receipt.
func runAdoptFromReceipt(receiptPath, layer, source string, verbose, dryRun bool) error {
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

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new dotfiles repository in the current directory",
		Long: `Initialize a new dotfiles repository in the current directory.

DEPRECATED: Use 'writ repo init' instead. This command is provided for
backward compatibility and will be removed in a future release.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(os.Stderr, "Deprecated: use 'writ repo init' instead\n\n")
			return runRepoInit(cmd, args)
		},
	}

	cmd.Flags().String("layer", "personal", "Layer for the repository: personal or team")
	cmd.Flags().Bool("force", false, "Overwrite existing structure")

	return cmd
}

func newRepoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo <subcommand>",
		Short: "Manage environment repositories",
		Long: `Manage environment repositories for writ.

Writ supports layered repositories with precedence: base → team → personal.
When files conflict, the higher-precedence layer wins (personal > team > base).

Repositories can be:
  - Initialized fresh (writ repo init)
  - Adopted from existing local paths (writ repo add)
  - Cloned from remote URLs (writ repo init <url>)

Repository paths are stored in ~/.config/devlore/config.yaml under writ.repos.`,
	}

	cmd.AddCommand(newRepoInitCmd())
	cmd.AddCommand(newRepoAddCmd())
	cmd.AddCommand(newRepoListCmd())
	cmd.AddCommand(newRepoRemoveCmd())

	return cmd
}

func newRepoInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [<git-url>]",
		Short: "Initialize or clone an environment repository",
		Long: `Initialize a new environment repository or clone from a remote URL.

Without arguments:
  Creates a new repository in XDG_DATA_HOME/devlore/repos/<layer>/
  with the standard writ directory structure and initializes git.

With a git URL:
  Clones the repository to XDG_DATA_HOME/devlore/repos/<layer>/
  using a simple 'git clone'. For advanced clone options (--branch,
  --depth, authentication), clone manually then use 'writ repo add'.

The repository is registered in ~/.config/devlore/config.yaml.`,
		Example: `  # Create new personal repository
  writ repo init --layer=personal

  # Clone existing repository as personal
  writ repo init --layer=personal git@github.com:me/dotfiles.git

  # Clone team repository
  writ repo init --layer=team git@github.com:company/team-configs.git

  # For advanced git options, clone first then register:
  git clone --branch main --depth 1 git@github.com:co/repo.git ~/repo
  writ repo add --layer=team ~/repo`,
		Args: cobra.MaximumNArgs(1),
		RunE: runRepoInit,
	}

	cmd.Flags().String("layer", "personal", "Layer for the repository: personal or team")
	cmd.Flags().Bool("force", false, "Overwrite existing structure or re-clone")

	return cmd
}

func newRepoAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Register an existing local repository",
		Long: `Register an existing local repository for use with writ.

The repository must already exist at the specified path. This command
validates the path and registers it in ~/.config/devlore/config.yaml.

Use this to adopt an existing dotfiles repository into writ management.`,
		Example: `  # Add existing personal dotfiles
  writ repo add --layer=personal ~/Workspace/Personal/Home/Configs

  # Add team repository at custom location
  writ repo add --layer=team /opt/company/team-configs`,
		Args: cobra.ExactArgs(1),
		RunE: runRepoAdd,
	}

	cmd.Flags().String("layer", "personal", "Layer for the repository: personal or team")
	cmd.Flags().String("name", "", "Display name for the repository (default: directory name)")

	return cmd
}

func newRepoListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured repositories",
		Long: `List all configured environment repositories.

Shows the layer, path, and status of each repository.`,
		RunE: runRepoList,
	}

	cmd.Flags().Bool("json", false, "Output as JSON")

	return cmd
}

func newRepoRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove --layer=<layer>",
		Short: "Unregister a repository",
		Long: `Unregister a repository from writ configuration.

This removes the repository entry from ~/.config/devlore/config.yaml.
It does NOT delete the repository files from disk.

To also remove deployed files from this repository, run 'writ remove'
for the relevant projects first.`,
		Example: `  # Unregister the personal repository
  writ repo remove --layer=personal

  # Unregister team repository
  writ repo remove --layer=team`,
		RunE: runRepoRemove,
	}

	cmd.Flags().String("layer", "", "Layer of the repository to remove (required)")
	cmd.MarkFlagRequired("layer")

	return cmd
}

// runRepoInit implements 'writ repo init'.
func runRepoInit(cmd *cobra.Command, args []string) error {
	verbose := viper.GetBool("writ.verbose")
	dryRun := viper.GetBool("writ.dry-run")
	layer, _ := cmd.Flags().GetString("layer")
	force, _ := cmd.Flags().GetBool("force")

	// Validate layer
	if layer != "personal" && layer != "team" && layer != "base" {
		return fmt.Errorf("invalid --layer %q: must be personal, team, or base", layer)
	}

	// Check if layer already configured
	existingRepo := getConfiguredRepo(layer)
	if existingRepo != "" && !force {
		return fmt.Errorf("layer %q already configured at %s\nUse --force to replace or 'writ repo remove --layer=%s' first", layer, existingRepo, layer)
	}

	var repoPath string
	var gitURL string

	if len(args) > 0 {
		// Clone from URL
		gitURL = args[0]
		repoPath = filepath.Join(cli.DataHome(), "devlore", "repos", layer)
	} else {
		// Create new repository
		repoPath = filepath.Join(cli.DataHome(), "devlore", "repos", layer)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Repository path: %s\n", repoPath)
		if gitURL != "" {
			fmt.Fprintf(os.Stderr, "Git URL: %s\n", gitURL)
		}
	}

	if dryRun {
		if gitURL != "" {
			fmt.Printf("Would clone %s to %s\n", gitURL, repoPath)
		} else {
			fmt.Printf("Would create new repository at %s\n", repoPath)
		}
		fmt.Printf("Would register as %s layer in config\n", layer)
		return nil
	}

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(repoPath), 0755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	if gitURL != "" {
		// Clone repository
		if _, err := os.Stat(repoPath); err == nil {
			if force {
				if err := os.RemoveAll(repoPath); err != nil {
					return fmt.Errorf("remove existing directory: %w", err)
				}
			} else {
				return fmt.Errorf("directory already exists: %s\nUse --force to overwrite", repoPath)
			}
		}

		fmt.Printf("Cloning %s...\n", gitURL)
		if err := runGitCommand("clone", gitURL, repoPath); err != nil {
			return fmt.Errorf("git clone: %w", err)
		}
		fmt.Printf("Cloned to %s\n", repoPath)
	} else {
		// Create new repository
		if _, err := os.Stat(repoPath); err == nil {
			if force {
				// Keep existing but ensure structure
				if verbose {
					fmt.Fprintf(os.Stderr, "Directory exists, ensuring structure\n")
				}
			} else {
				return fmt.Errorf("directory already exists: %s\nUse --force to initialize anyway", repoPath)
			}
		} else {
			if err := os.MkdirAll(repoPath, 0755); err != nil {
				return fmt.Errorf("create repository directory: %w", err)
			}
		}

		// Create standard structure
		if err := createRepoStructure(repoPath, layer, verbose); err != nil {
			return fmt.Errorf("create structure: %w", err)
		}

		// Initialize git if not already a repo
		gitDir := filepath.Join(repoPath, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			fmt.Println("Initializing git repository...")
			if err := runGitCommandIn(repoPath, "init"); err != nil {
				return fmt.Errorf("git init: %w", err)
			}
		}

		fmt.Printf("Created new repository at %s\n", repoPath)
	}

	// Register in config
	if err := registerRepo(layer, repoPath, gitURL); err != nil {
		return fmt.Errorf("register repository: %w", err)
	}
	fmt.Printf("Registered as %s layer\n", layer)

	return nil
}

// runRepoAdd implements 'writ repo add'.
func runRepoAdd(cmd *cobra.Command, args []string) error {
	verbose := viper.GetBool("writ.verbose")
	dryRun := viper.GetBool("writ.dry-run")
	layer, _ := cmd.Flags().GetString("layer")
	name, _ := cmd.Flags().GetString("name")

	// Validate layer
	if layer != "personal" && layer != "team" && layer != "base" {
		return fmt.Errorf("invalid --layer %q: must be personal, team, or base", layer)
	}

	// Check if layer already configured
	existingRepo := getConfiguredRepo(layer)
	if existingRepo != "" {
		return fmt.Errorf("layer %q already configured at %s\nUse 'writ repo remove --layer=%s' first", layer, existingRepo, layer)
	}

	// Expand and validate path
	repoPath := expandPath(args[0])
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	repoPath = absPath

	// Verify directory exists
	info, err := os.Stat(repoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s\nUse 'writ repo init' to create a new repository", repoPath)
		}
		return fmt.Errorf("stat path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", repoPath)
	}

	// Default name to directory name
	if name == "" {
		name = filepath.Base(repoPath)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Repository path: %s\n", repoPath)
		fmt.Fprintf(os.Stderr, "Name: %s\n", name)
		fmt.Fprintf(os.Stderr, "Layer: %s\n", layer)
	}

	// Check if it's a git repository (informational)
	gitDir := filepath.Join(repoPath, ".git")
	isGit := false
	if _, err := os.Stat(gitDir); err == nil {
		isGit = true
		if verbose {
			fmt.Fprintf(os.Stderr, "Git repository: yes\n")
		}
	}

	if dryRun {
		fmt.Printf("Would register %s as %s layer\n", repoPath, layer)
		if !isGit {
			fmt.Printf("Note: %s is not a git repository\n", repoPath)
		}
		return nil
	}

	// Register in config
	if err := registerRepoWithName(layer, repoPath, "", name); err != nil {
		return fmt.Errorf("register repository: %w", err)
	}

	fmt.Printf("Registered %s as %s layer\n", repoPath, layer)
	if !isGit {
		fmt.Printf("Note: %s is not a git repository\n", repoPath)
	}

	return nil
}

// runRepoList implements 'writ repo list'.
func runRepoList(cmd *cobra.Command, args []string) error {
	asJSON, _ := cmd.Flags().GetBool("json")

	repos := getConfiguredRepos()

	if asJSON {
		data, err := json.MarshalIndent(repos, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	if len(repos) == 0 {
		fmt.Println("No repositories configured.")
		fmt.Println("\nUse 'writ repo init' to create a new repository")
		fmt.Println("or 'writ repo add' to register an existing one.")
		return nil
	}

	fmt.Println("Configured repositories:")
	fmt.Println()

	// Print in layer precedence order
	layerOrder := []string{"base", "team", "personal"}
	for _, layer := range layerOrder {
		for _, repo := range repos {
			if repo.Layer != layer {
				continue
			}

			status := "ok"
			if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
				status = "missing"
			}

			fmt.Printf("  %-10s %s", repo.Layer, repo.Path)
			if repo.Name != "" && repo.Name != filepath.Base(repo.Path) {
				fmt.Printf(" (%s)", repo.Name)
			}
			if status != "ok" {
				fmt.Printf(" [%s]", status)
			}
			fmt.Println()
		}
	}

	return nil
}

// runRepoRemove implements 'writ repo remove'.
func runRepoRemove(cmd *cobra.Command, args []string) error {
	verbose := viper.GetBool("writ.verbose")
	dryRun := viper.GetBool("writ.dry-run")
	layer, _ := cmd.Flags().GetString("layer")

	// Validate layer
	if layer != "personal" && layer != "team" && layer != "base" {
		return fmt.Errorf("invalid --layer %q: must be personal, team, or base", layer)
	}

	// Check if layer is configured
	existingRepo := getConfiguredRepo(layer)
	if existingRepo == "" {
		return fmt.Errorf("no repository configured for layer %q", layer)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Removing %s layer: %s\n", layer, existingRepo)
	}

	if dryRun {
		fmt.Printf("Would unregister %s layer (%s)\n", layer, existingRepo)
		fmt.Println("Note: Repository files would not be deleted")
		return nil
	}

	if err := unregisterRepo(layer); err != nil {
		return fmt.Errorf("unregister repository: %w", err)
	}

	fmt.Printf("Unregistered %s layer (%s)\n", layer, existingRepo)
	fmt.Println("Note: Repository files were not deleted")

	return nil
}

// RepoConfig represents a configured repository.
type RepoConfig struct {
	Name   string `json:"name,omitempty" yaml:"name,omitempty"`
	Path   string `json:"path" yaml:"path"`
	URL    string `json:"url,omitempty" yaml:"url,omitempty"`
	Layer  string `json:"layer" yaml:"layer"`
	Origin string `json:"origin,omitempty" yaml:"origin,omitempty"` // "local" or git URL
}

// getConfiguredRepo returns the path for a configured layer, or empty string.
func getConfiguredRepo(layer string) string {
	// Check writ.repos array
	repos := viper.Get("writ.repos")
	if repoSlice, ok := repos.([]interface{}); ok {
		for _, r := range repoSlice {
			if repoMap, ok := r.(map[string]interface{}); ok {
				if repoMap["layer"] == layer {
					if path, ok := repoMap["path"].(string); ok {
						return expandPath(path)
					}
				}
			}
		}
	}

	// Fallback: check legacy writ.repo for personal layer
	if layer == "personal" {
		if legacyRepo := viper.GetString("writ.repo"); legacyRepo != "" {
			return expandPath(legacyRepo)
		}
	}

	return ""
}

// getConfiguredRepos returns all configured repositories.
func getConfiguredRepos() []RepoConfig {
	var result []RepoConfig

	// Check writ.repos array
	repos := viper.Get("writ.repos")
	if repoSlice, ok := repos.([]interface{}); ok {
		for _, r := range repoSlice {
			if repoMap, ok := r.(map[string]interface{}); ok {
				rc := RepoConfig{}
				if name, ok := repoMap["name"].(string); ok {
					rc.Name = name
				}
				if path, ok := repoMap["path"].(string); ok {
					rc.Path = expandPath(path)
				}
				if url, ok := repoMap["url"].(string); ok {
					rc.URL = url
				}
				if layer, ok := repoMap["layer"].(string); ok {
					rc.Layer = layer
				}
				if rc.Path != "" || rc.URL != "" {
					result = append(result, rc)
				}
			}
		}
	}

	// Check legacy writ.repo
	if legacyRepo := viper.GetString("writ.repo"); legacyRepo != "" {
		// Only add if not already in repos array
		found := false
		expandedLegacy := expandPath(legacyRepo)
		for _, r := range result {
			if r.Path == expandedLegacy && r.Layer == "personal" {
				found = true
				break
			}
		}
		if !found {
			result = append(result, RepoConfig{
				Path:  expandedLegacy,
				Layer: "personal",
			})
		}
	}

	return result
}

// registerRepo registers a repository in the config file.
func registerRepo(layer, path, url string) error {
	return registerRepoWithName(layer, path, url, "")
}

// registerRepoWithName registers a repository with an optional display name.
func registerRepoWithName(layer, path, url, name string) error {
	configPath := filepath.Join(cli.ConfigHome(), "devlore", "config.yaml")

	// Ensure config directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	// Read existing config
	configData, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read config: %w", err)
	}

	// Parse existing config
	var config map[string]interface{}
	if len(configData) > 0 {
		if err := yaml.Unmarshal(configData, &config); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	}
	if config == nil {
		config = make(map[string]interface{})
	}

	// Ensure writ section exists
	writSection, ok := config["writ"].(map[string]interface{})
	if !ok {
		writSection = make(map[string]interface{})
		config["writ"] = writSection
	}

	// Get or create repos array
	var repos []interface{}
	if existingRepos, ok := writSection["repos"].([]interface{}); ok {
		// Remove any existing entry for this layer
		for _, r := range existingRepos {
			if repoMap, ok := r.(map[string]interface{}); ok {
				if repoMap["layer"] != layer {
					repos = append(repos, r)
				}
			}
		}
	}

	// Add new repo entry
	newRepo := map[string]interface{}{
		"path":  path,
		"layer": layer,
	}
	if name != "" {
		newRepo["name"] = name
	}
	if url != "" {
		newRepo["url"] = url
	}
	repos = append(repos, newRepo)

	writSection["repos"] = repos

	// Write config back
	newConfigData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, newConfigData, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Reload viper
	viper.Set("writ.repos", repos)

	return nil
}

// unregisterRepo removes a repository from the config file.
func unregisterRepo(layer string) error {
	configPath := filepath.Join(cli.ConfigHome(), "devlore", "config.yaml")

	// Read existing config
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	// Parse existing config
	var config map[string]interface{}
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Get writ section
	writSection, ok := config["writ"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no writ section in config")
	}

	// Filter repos array
	var repos []interface{}
	if existingRepos, ok := writSection["repos"].([]interface{}); ok {
		for _, r := range existingRepos {
			if repoMap, ok := r.(map[string]interface{}); ok {
				if repoMap["layer"] != layer {
					repos = append(repos, r)
				}
			}
		}
	}

	if len(repos) > 0 {
		writSection["repos"] = repos
	} else {
		delete(writSection, "repos")
	}

	// Write config back
	newConfigData, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, newConfigData, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Reload viper
	viper.Set("writ.repos", repos)

	return nil
}

// createRepoStructure creates the standard writ repository structure.
func createRepoStructure(repoPath, layer string, verbose bool) error {
	// Create Home target directory with an 'all' project
	homeDir := filepath.Join(repoPath, "Home", "all")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		return err
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "Created %s\n", homeDir)
	}

	// Create .gitignore
	gitignore := filepath.Join(repoPath, ".gitignore")
	if _, err := os.Stat(gitignore); os.IsNotExist(err) {
		content := `# Decrypted secrets (never commit)
*.decrypted
*.plaintext

# Backup files
*.writ-backup

# OS files
.DS_Store
Thumbs.db
`
		if err := os.WriteFile(gitignore, []byte(content), 0644); err != nil {
			return err
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "Created %s\n", gitignore)
		}
	}

	// Create README
	readme := filepath.Join(repoPath, "README.md")
	if _, err := os.Stat(readme); os.IsNotExist(err) {
		content := fmt.Sprintf(`# Environment Repository (%s layer)

This repository is managed by [writ](https://github.com/NobleFactor/devlore-cli).

## Structure

`+"```"+`
%s/
├── Home/              # Target: ~ ($HOME)
│   ├── all/           # Applied to all platforms
│   ├── all.Darwin/    # macOS-specific
│   ├── all.Linux/     # Linux-specific
│   └── <project>/     # Project-specific configs
└── .age-recipients    # Public keys for encryption (optional)
`+"```"+`

## Usage

Deploy all files:
`+"```bash"+`
writ add all
`+"```"+`

Check status:
`+"```bash"+`
writ status
`+"```"+`
`, layer, filepath.Base(repoPath))
		if err := os.WriteFile(readme, []byte(content), 0644); err != nil {
			return err
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "Created %s\n", readme)
		}
	}

	return nil
}

// runGitCommand runs a git command with the given arguments.
func runGitCommand(args ...string) error {
	cmd := osexec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runGitCommandIn runs a git command in a specific directory.
func runGitCommandIn(dir string, args ...string) error {
	cmd := osexec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func newConfigureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Discover system info and configure template variables",
		Long: `Discover system info and configure template variables.

Auto-detects user name, email, editor, and platform. Prompts for values
that cannot be determined automatically.

Use --unattended (global flag) to auto-detect only and fail on missing values.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("configure: not yet implemented")
			return nil
		},
	}

	return cmd
}

func newSecretsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets <subcommand>",
		Short: "Manage age-encrypted secrets",
		Long: `Manage age-encrypted secrets in your environment repository.

Secrets are encrypted with age using recipients listed in .age-recipients.
Your identity (private key) is resolved from SSH keys or config.`,
	}

	rekeyCmd := &cobra.Command{
		Use:   "rekey",
		Short: "Re-encrypt all .age files to current .age-recipients",
		Long: `Re-encrypt all .age files to the current .age-recipients list.

Use this when:
  - Adding a new machine (new recipient needs access)
  - Revoking access (removed recipient should no longer decrypt)
  - Rotating keys

The operation is idempotent and can be safely re-run.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("secrets rekey: not yet implemented")
			return nil
		},
	}
	cmd.AddCommand(rekeyCmd)

	encryptCmd := &cobra.Command{
		Use:   "encrypt <file>",
		Short: "Encrypt a file with age",
		Long: `Encrypt a file using recipients from .age-recipients.

The encrypted file is written to <file>.age and the original is removed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("secrets encrypt %s: not yet implemented\n", args[0])
			return nil
		},
	}
	encryptCmd.Flags().Bool("keep", false, "Keep the original file after encryption")
	cmd.AddCommand(encryptCmd)

	decryptCmd := &cobra.Command{
		Use:   "decrypt <file.age>",
		Short: "Decrypt an age-encrypted file",
		Long: `Decrypt an age-encrypted file using your identity.

By default outputs to stdout. Use -o to write to a file.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("secrets decrypt %s: not yet implemented\n", args[0])
			return nil
		},
	}
	decryptCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
	cmd.AddCommand(decryptCmd)

	editCmd := &cobra.Command{
		Use:   "edit <file.age>",
		Short: "Edit an encrypted file in place",
		Long: `Decrypt a file, open in $EDITOR, re-encrypt on save.

The file is decrypted to a temporary location, opened in your editor,
and re-encrypted when the editor exits.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("secrets edit %s: not yet implemented\n", args[0])
			return nil
		},
	}
	cmd.AddCommand(editCmd)

	return cmd
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
