// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/adopt"
	"github.com/NobleFactor/devlore-cli/internal/cli"
)

// newAdoptCmd constructs the cobra command for `writ adopt`.
//
// Moves files from their target location into the project directory and creates symlinks back. Scope (Home or
// System) is inferred per-item from the path relative to `$HOME`. Directories are walked recursively; existing
// symlinks within directories are skipped.
//
// The step-33 slice-A rewrite (the writ-adopt design): the cobra layer parses flags and delegates; the adopt
// package enumerates the inputs into per-scope [adopt.Item] batches ([adopt.Collect]) and executes ONE graph per
// scope group ([adopt.RunBatches]) — a deduplicated mkdir pre-stage plus a `flow.gather` over the item records with
// an in-graph destination guard — persisting each run's trace as the receipt, success or failure.
//
// Returns:
//   - *cobra.Command: the configured adopt command.
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

Each scope's adoptions run as one execution graph: a failed adoption fails the
run and completed adoptions roll back (moves reversed, links removed).

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
	cmd.Flags().String("project", "", "Origin name within the layer (required)")
	cmd.Flags().Bool("from-receipt", false, "Adopt packages-manifest.yaml and config from lore receipt")

	return cmd
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

	if cfg.Verbose {
		cli.Note("Layer: %s", cfg.Layer)
	}

	batchConfig := &adopt.Config{
		Files:      cfg.Files,
		TargetRoot: cfg.TargetRoot,
		LayerPath:  cfg.LayerPath,
		Project:    cfg.Project,
		Verbose:    cfg.Verbose,
		DryRun:     cfg.DryRun,
	}

	groups := adopt.Collect(batchConfig)

	total := 0
	for _, items := range groups {
		total += len(items)
	}

	if cfg.DryRun || total == 0 {
		reportAdoptResult(cfg, total)
		return nil
	}

	adopted, err := adopt.RunBatches(context.Background(), batchConfig, groups)
	if err != nil {
		return err
	}

	reportAdoptResult(cfg, adopted)
	return nil
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

// runAdoptFromReceipt adopts files from a lore receipt.
func runAdoptFromReceipt(receiptPath, layer, project string, verbose, dryRun bool) error {
	// TODO: Implement reading lore receipt and adopting packages-manifest.yaml + config
	return fmt.Errorf("adopt --from-receipt: not yet implemented")
}
