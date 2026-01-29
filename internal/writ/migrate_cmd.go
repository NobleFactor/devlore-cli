// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package writ

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/model"
	"github.com/NobleFactor/devlore-cli/internal/registry"
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
	cmd.Flags().String("format", "text", "Output format: text, yaml, json (for --dry-run)")
	cmd.Flags().String("system", "", "Override auto-detection with a specific source system")
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
	useMove, _ := cmd.Flags().GetBool("move")
	layer, _ := cmd.Flags().GetString("layer")
	format, _ := cmd.Flags().GetString("format")
	verbose, _ := cmd.Root().Flags().GetBool("verbose")

	// Validate layer
	if layer != "personal" && layer != "team" && layer != "base" {
		return fmt.Errorf("invalid --layer %q: must be personal, team, or base", layer)
	}

	// All structures go through AI for analysis, validation, and secret detection
	regClient, err := registry.NewDefault()
	if err != nil {
		return fmt.Errorf("initializing registry: %w", err)
	}

	if !regClient.Exists() {
		fmt.Fprintln(os.Stderr, "Syncing registry...")
		if _, err := regClient.Sync(ctx, registry.SyncOptions{}); err != nil {
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

	provider, err := model.EnsureProvider(ctx, false, modelFlags)
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

	plan, err := migrate.BuildPlan(ctx, opts)
	if err != nil {
		return err
	}

	if dryRun {
		return migrate.FormatPlan(os.Stdout, plan, format)
	}

	// Restructure content to writ conventions
	if err := migrate.Execute(os.Stderr, plan); err != nil {
		return err
	}

	// Register layer via link or move
	layerDir := filepath.Join(cli.WritLayersDir(), layer)

	if useMove {
		// Move: relocate content to layer directory
		if err := moveToLayer(sourceRoot, layerDir, verbose); err != nil {
			return fmt.Errorf("move to layer: %w", err)
		}
		cli.Success("Moved %s to %s layer", sourceRoot, layer)
	} else {
		// Link: create symlink from layer directory to source
		if err := linkToLayer(sourceRoot, layerDir, verbose); err != nil {
			return fmt.Errorf("link to layer: %w", err)
		}
		cli.Success("Linked %s layer to %s", layer, sourceRoot)
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
