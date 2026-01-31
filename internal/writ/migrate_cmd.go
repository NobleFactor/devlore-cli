// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/console"
	"github.com/NobleFactor/devlore-cli/internal/model"
	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/internal/writ/migrate"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate [flags] <source-directory>",
		Short: "Migrate an existing environment to a writ layer",
		Long: `Migrate an existing environment repository to a writ layer.

Writ auto-detects the source system (Tuckr, Stow, chezmoi, yadm, bare git,
or script-based setups) and uses AI to analyze and restructure content to
writ conventions (Home/, System/, projects).

AI-assisted migration provides:
  - Intelligent file classification (configs, scripts, secrets)
  - Content-aware secret detection (unencrypted credentials, API keys)
  - Package manifest generation from setup scripts
  - Recommendations (e.g., migrating from git-crypt to SOPS)
  - Structure validation for already writ-compatible layouts

After analysis and any restructuring, the source is registered as a layer:
  --link (default): Layer directory becomes a symlink to source location
  --move: Content is moved into the layer directory, source is deleted

Use --dry-run to preview without making changes.`,
		Example: `  # Migrate and link to source location (default)
  writ migrate ~/my-environment

  # Preview without making changes
  writ migrate --dry-run ~/my-environment

  # Move content to layer directory instead of linking
  writ migrate --move ~/my-environment

  # Migrate as team layer instead of personal
  writ migrate --layer team ~/team-environment`,
		Args: cobra.ExactArgs(1),
		RunE: runMigrate,
	}

	cmd.Flags().Bool("link", true, "Create symlink from layer to source (default)")
	cmd.Flags().Bool("move", false, "Move content to layer directory, delete source")
	cmd.Flags().String("layer", "personal", "Target layer: personal, team, or base")
	cmd.Flags().String("format", "json", "Output format: json (default), yaml, text (for --dry-run)")
	cmd.Flags().String("system", "", "Override auto-detection with a specific source system")
	cmd.Flags().Bool("non-interactive", false, "Migrate without interactive prompts")
	cmd.MarkFlagsMutuallyExclusive("link", "move")

	return cmd
}

func runMigrate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	sourceRoot, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	info, err := os.Stat(sourceRoot)
	if err != nil {
		return fmt.Errorf("source root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source root %s is not a directory", sourceRoot)
	}

	dryRun, _ := cmd.Root().Flags().GetBool("dry-run")
	nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
	useMove, _ := cmd.Flags().GetBool("move")
	layer, _ := cmd.Flags().GetString("layer")
	format, _ := cmd.Flags().GetString("format")
	verbose, _ := cmd.Root().Flags().GetBool("verbose")

	// Validate layer
	if layer != "personal" && layer != "team" && layer != "base" {
		return fmt.Errorf("invalid --layer %q: must be personal, team, or base", layer)
	}

	// All structures go through AI for analysis, validation, and secret detection
	regClient, err := lorepackage.NewDefault()
	if err != nil {
		return fmt.Errorf("initializing registry: %w", err)
	}

	if !regClient.Exists() {
		cli.Note("Syncing lorepackage...")
		if _, err := regClient.Sync(ctx, lorepackage.SyncOptions{}); err != nil {
			return fmt.Errorf("registry sync failed: %w", err)
		}
	}

	// Read model flags from root command
	modelFlags := model.CLIFlags{
		Model:    mustGetString(cmd.Root(), "model"),
		APIKey:   mustGetString(cmd.Root(), "model-api-key"),
		Endpoint: mustGetString(cmd.Root(), "model-endpoint"),
		Provider: mustGetString(cmd.Root(), "model-provider"),
	}

	interactive := isInteractive(nonInteractive)
	provider, err := model.EnsureProvider(ctx, interactive, modelFlags)
	if err != nil {
		return fmt.Errorf("model provider required: %w", err)
	}
	if provider == nil {
		return fmt.Errorf("model provider required for migration analysis; configure with 'lore config model'")
	}

	opts := migrate.Options{
		SourceRoot: sourceRoot,
		Execute:    !dryRun,
		Verbose:    verbose,
		Format:     format,
		Provider:   provider,
		RegClient:  regClient,
	}

	if interactive {
		return runMigrateInteractive(opts, layer, useMove, verbose)
	}

	return runMigrateBatch(ctx, opts, layer, useMove, verbose, dryRun, format)
}

// isInteractive returns true if the session should be interactive.
// Returns false if --non-interactive was specified or stdout is not a TTY.
func isInteractive(nonInteractive bool) bool {
	if nonInteractive {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// runMigrateInteractive runs migration with step-by-step user confirmation.
func runMigrateInteractive(opts migrate.Options, layer string, useMove, verbose bool) error {
	session := migrate.NewSessionWithProvider(opts, opts.Provider, opts.RegClient)
	con := console.New()

	result, err := con.Run(session)
	if err != nil {
		return err
	}

	sessionResult, ok := result.(*migrate.SessionResult)
	if !ok || sessionResult == nil {
		return fmt.Errorf("migration cancelled")
	}

	if !sessionResult.Executed {
		cli.Note("Migration plan exported (dry run)")
		return nil
	}

	// Register layer via link or move
	layerDir := filepath.Join(cli.WritLayersDir(), layer)

	if useMove {
		if err := moveToLayer(opts.SourceRoot, layerDir, verbose); err != nil {
			return fmt.Errorf("move to layer: %w", err)
		}
		cli.Success("Moved %s to %s layer", opts.SourceRoot, layer)
	} else {
		if err := linkToLayer(opts.SourceRoot, layerDir, verbose); err != nil {
			return fmt.Errorf("link to layer: %w", err)
		}
		cli.Success("Linked %s layer to %s", layer, opts.SourceRoot)
	}

	return nil
}

// runMigrateBatch runs migration without interactive prompts (for CI/automation).
func runMigrateBatch(ctx context.Context, opts migrate.Options, layer string, useMove, verbose, dryRun bool, format string) error {
	graph, analysis, err := migrate.BuildMigration(ctx, opts)
	if err != nil {
		return err
	}

	if dryRun {
		return migrate.FormatMigrationPlan(os.Stdout, graph, analysis, format)
	}

	// Restructure content to writ conventions
	if err := migrate.Execute(os.Stderr, graph, analysis); err != nil {
		return err
	}

	// Save receipt
	receiptPath, err := cli.WriteReceipt(graph, "writ-migrate")
	if err != nil {
		cli.Note("Failed to save receipt: %v", err)
	} else if verbose {
		cli.Note("Receipt saved to %s", receiptPath)
	}

	// Register layer via link or move
	layerDir := filepath.Join(cli.WritLayersDir(), layer)

	if useMove {
		if err := moveToLayer(opts.SourceRoot, layerDir, verbose); err != nil {
			return fmt.Errorf("move to layer: %w", err)
		}
		cli.Success("Moved %s to %s layer", opts.SourceRoot, layer)
	} else {
		if err := linkToLayer(opts.SourceRoot, layerDir, verbose); err != nil {
			return fmt.Errorf("link to layer: %w", err)
		}
		cli.Success("Linked %s layer to %s", layer, opts.SourceRoot)
	}

	return nil
}

// linkToLayer creates a symlink from layerDir to sourceRoot.
// If layerDir exists, it is removed first (must be empty or a symlink).
func linkToLayer(sourceRoot, layerDir string, verbose bool) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(layerDir), 0755); err != nil {
		return err
	}

	// Check if layer directory exists
	if info, err := os.Lstat(layerDir); err == nil {
		// If it's a symlink, remove it
		if info.Mode()&os.ModeSymlink != 0 {
			if verbose {
				cli.Note("Removing existing symlink: %s", layerDir)
			}
			if err := os.Remove(layerDir); err != nil {
				return fmt.Errorf("remove existing symlink: %w", err)
			}
		} else if info.IsDir() {
			// If it's a directory, check if empty
			entries, err := os.ReadDir(layerDir)
			if err != nil {
				return err
			}
			if len(entries) > 0 {
				return fmt.Errorf("layer directory %s is not empty; remove or move contents first", layerDir)
			}
			if verbose {
				cli.Note("Removing empty directory: %s", layerDir)
			}
			if err := os.Remove(layerDir); err != nil {
				return fmt.Errorf("remove empty directory: %w", err)
			}
		} else {
			return fmt.Errorf("layer path %s exists and is not a directory or symlink", layerDir)
		}
	}

	// Create symlink
	if verbose {
		cli.Note("Creating symlink: %s -> %s", layerDir, sourceRoot)
	}
	return os.Symlink(sourceRoot, layerDir)
}

// moveToLayer moves content from sourceRoot to layerDir.
func moveToLayer(sourceRoot, layerDir string, verbose bool) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(layerDir), 0755); err != nil {
		return err
	}

	// Check if layer directory exists
	if info, err := os.Lstat(layerDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			// Remove existing symlink
			if verbose {
				cli.Note("Removing existing symlink: %s", layerDir)
			}
			if err := os.Remove(layerDir); err != nil {
				return fmt.Errorf("remove existing symlink: %w", err)
			}
		} else if info.IsDir() {
			entries, err := os.ReadDir(layerDir)
			if err != nil {
				return err
			}
			if len(entries) > 0 {
				return fmt.Errorf("layer directory %s is not empty; remove or move contents first", layerDir)
			}
			if verbose {
				cli.Note("Removing empty directory: %s", layerDir)
			}
			if err := os.Remove(layerDir); err != nil {
				return fmt.Errorf("remove empty directory: %w", err)
			}
		} else {
			return fmt.Errorf("layer path %s exists and is not a directory or symlink", layerDir)
		}
	}

	// Move (rename) source to layer directory
	if verbose {
		cli.Note("Moving: %s -> %s", sourceRoot, layerDir)
	}
	return os.Rename(sourceRoot, layerDir)
}

// mustGetString gets a string flag value, returning empty string on error.
func mustGetString(cmd *cobra.Command, name string) string {
	val, _ := cmd.Flags().GetString(name)
	return val
}
