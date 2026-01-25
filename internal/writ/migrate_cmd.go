// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package writ

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/internal/ai"
	"github.com/NobleFactor/devlore-cli/internal/registry"
	"github.com/NobleFactor/devlore-cli/internal/writ/migrate"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate [flags] <source-root>",
		Short: "Analyze and migrate an existing environment repository to writ conventions",
		Long: `Analyze an existing environment repository and produce a migration plan.

Writ auto-detects the source system (Tuckr, Stow, chezmoi, yadm, bare git,
or script-based setups using <project>-<Platform> directories) and generates
a plan showing what would change.

By default, AI-assisted analysis provides file classification, secret detection,
and contextual recommendations. Use --no-ai for basic structural analysis only.

Use --execute to perform the migration (directory renames from dash to dot
convention). Supported output formats: text (default), yaml, json.`,
		Example: `  writ migrate ~/my-env/Configs
  writ migrate ~/my-env/Configs --format yaml
  writ migrate ~/my-env/Configs --no-ai
  writ migrate ~/my-env/Configs --execute`,
		Args: cobra.ExactArgs(1),
		RunE: runMigrate,
	}

	cmd.Flags().Bool("execute", false, "Perform the migration (rename directories)")
	cmd.Flags().Bool("no-ai", false, "Skip AI-assisted analysis (basic mode)")
	cmd.Flags().String("format", "text", "Output format: text, yaml, json")
	cmd.Flags().String("system", "", "Override auto-detection with a specific source system")

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

	execute, _ := cmd.Flags().GetBool("execute")
	noAI, _ := cmd.Flags().GetBool("no-ai")
	format, _ := cmd.Flags().GetString("format")
	verbose, _ := cmd.Root().Flags().GetBool("verbose")

	// Initialize registry client
	regClient, err := registry.NewDefault()
	if err != nil {
		return fmt.Errorf("initializing registry: %w", err)
	}

	// Sync registry if not cached (or stale)
	if !regClient.Exists() {
		fmt.Fprintln(os.Stderr, "Syncing registry...")
		if _, err := regClient.Sync(ctx, registry.SyncOptions{}); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not sync registry: %v\n", err)
			fmt.Fprintln(os.Stderr, "Continuing with basic analysis...")
			noAI = true
		}
	}

	// Get AI provider (prompts for configuration if needed)
	var aiProvider ai.Provider
	if !noAI {
		aiProvider, err = ai.EnsureProvider(ctx, noAI)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: AI not available: %v\n", err)
			fmt.Fprintln(os.Stderr, "Continuing with basic analysis...")
		}
	}

	opts := migrate.Options{
		SourceRoot: sourceRoot,
		Execute:    execute,
		Verbose:    verbose,
		Format:     format,
		AIProvider: aiProvider,
		RegClient:  regClient,
	}

	plan, err := migrate.BuildPlan(ctx, opts)
	if err != nil {
		return err
	}

	if execute {
		return migrate.Execute(os.Stderr, plan)
	}

	return migrate.FormatPlan(os.Stdout, plan, format)
}
