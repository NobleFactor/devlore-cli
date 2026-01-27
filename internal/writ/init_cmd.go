// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package writ

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/internal/ai"
	"github.com/NobleFactor/devlore-cli/internal/registry"
	initpkg "github.com/NobleFactor/devlore-cli/internal/writ/init"
)

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new environment with AI-assisted recommendations",
		Long: `Initialize a new writ environment with platform detection and AI-assisted recommendations.

Writ detects your platform, shell, and available package managers, then provides
personalized recommendations for:
  - Repository structure and layer selection
  - Essential and recommended packages
  - Initial project organization

If an existing dotfile manager is detected (chezmoi, yadm, stow, etc.),
writ recommends using 'writ migrate' instead to preserve your configuration.

By default, AI-assisted analysis provides detailed recommendations. Use --no-ai
for basic structural suggestions only.`,
		Example: `  writ init
  writ init --layer=personal
  writ init --focus=devops
  writ init --format yaml
  writ init --no-ai`,
		RunE: runInit,
	}

	cmd.Flags().String("layer", "", "Repository layer preference: personal, team, or base")
	cmd.Flags().String("focus", "", "Development focus: web, backend, devops, data, mobile")
	cmd.Flags().Bool("no-ai", false, "Skip AI-assisted analysis (basic mode)")
	cmd.Flags().String("format", "text", "Output format: text, yaml, json")

	return cmd
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	layer, _ := cmd.Flags().GetString("layer")
	focus, _ := cmd.Flags().GetString("focus")
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

	opts := initpkg.Options{
		Layer:      layer,
		Focus:      focus,
		Verbose:    verbose,
		Format:     format,
		AIProvider: aiProvider,
		RegClient:  regClient,
	}

	plan, err := initpkg.BuildPlan(ctx, opts)
	if err != nil {
		return err
	}

	return initpkg.FormatPlan(os.Stdout, plan, format)
}
