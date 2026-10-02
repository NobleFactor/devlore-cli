// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/adopt"
)

// newAdoptCmd constructs the cobra command for `writ adopt`.
//
// Moves files from their target location into the project directory and creates symlinks back. Each item's scope is
// inferred from the scope model: of the scopes this platform defines, the one whose root is the deepest that holds
// the item (#926). Directories are walked recursively; existing symlinks within directories are skipped.
//
// The cobra layer parses flags, resolves the scopes ([adoptScopes]) and delegates: the adopt package enumerates the
// inputs into per-scope [adopt.Item] batches ([adopt.Collect]) and executes ONE graph per scope group
// ([adopt.RunBatches]) — a deduplicated mkdir pre-stage, then one move-and-link chain per item — persisting each run's
// trace as the receipt, success or failure.
//
// Returns:
//   - `*cobra.Command`: the configured adopt command.
func newAdoptCmd() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "adopt [flags] <item>...",
		Short: "Move files from target location into a project and create symlinks",
		Long: `Move files from target location into a project and create symlinks.

Use this to bring existing configuration files under version control.
Files are moved to <layer>/<scope>/<project>/ preserving their relative path,
then symlinked back to the original location.

Scope is inferred from the item's location: of the scopes this platform
defines, the one whose root is the deepest that holds the item.
  - Items under $HOME are adopted into Home/
  - Items under a custom scope's root (writ.scopes), or on Windows under
    %ProgramData%, %ProgramFiles% or %ProgramFiles(x86)%, into that scope's
    directory
  - Items elsewhere under / (Unix) or %SystemDrive%\ (Windows) into System/
writ.scopes can relocate Home and System. An item under no scope's root is
refused, and so is one whose scope's root shares no directory with the layer:
on Windows, a layer on another drive.

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

  # Adopt a Debian file: it lands under <project>.Debian and deploys on Debian and every
  # distribution descended from it, Ubuntu among them
  writ adopt --project noblefactor --platform Debian ~/.config/apt.conf

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
	cmd.Flags().String("platform", "",
		"Platform suffix the adopted files carry, as a directory name reads it: one word of this machine's chain, "+
			"then its architecture, then declared segment values (Unix, Linux, Debian, Ubuntu, Debian.arm64, ...); "+
			"it must name this machine. Absent, the platform-neutral project directory")
	cmd.Flags().Bool("from-receipt", false, "Adopt packages-manifest.yaml and config from lore receipt")
	cmd.Flags().StringArrayP("segment", "s", nil,
		"Set a segment's value, NAME=value (repeatable); an extra must be declared in writ.segments")

	return cmd
}

// runAdopt runs `writ adopt`: it resolves the configuration and the scopes, enumerates the items, and runs one graph
// per scope group, unless `--dry-run` or an enumeration that found nothing stops it at the summary.
//
// Parameters:
//   - `cmd`: the adopt command, whose flags configure the run.
//   - `args`: the items to adopt; with `--from-receipt`, the receipt's path.
//
// Returns:
//   - `error`: the configuration's refusal, `--from-receipt`'s (it is not yet implemented), or the batches' failure;
//     nil when the items are adopted or, under `--dry-run`, reported.
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
		Scopes:     adoptScopes(cfg.LayerPath),
		Layer:      cfg.Layer,
		LayerPath:  cfg.LayerPath,
		Project:    cfg.Project,
		Platform:   cfg.Platform,
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

// region HELPER FUNCTIONS

// adoptScopes resolves the scopes an item can be adopted into from the scope model: every scope this platform
// defines, in scope order, each with its directory in the layer and its root (#926, #761).
//
// A builtin's directory carries the builtin's own name. A custom scope's is the layer's own directory for it, found
// without case as deploy finds it, else the scope's name as `writ.scopes` gives it.
//
// Parameters:
//   - `layerPath`: the layer the items are adopted into.
//
// Returns:
//   - `[]adopt.Scope`: the defined scopes, each named in lower case as the record names it.
func adoptScopes(layerPath string) []adopt.Scope {

	defined := ScopeOrder()
	names := scopeNames(defined)

	scopes := make([]adopt.Scope, len(defined))
	for i, spec := range defined {
		directory := spec.SourceDir
		if path, found := scopeDirectory(layerPath, spec.SourceDir); found {
			directory = filepath.Base(path)
		}
		scopes[i] = adopt.Scope{Name: names[i], Directory: directory, Root: spec.TargetRoot}
	}
	return scopes
}

// reportAdoptResult outputs the adoption summary.
//
// Parameters:
//   - `cfg`: the adopt configuration, whose dry-run flag, layer, project and layer path the summary reads.
//   - `adopted`: the number of files adopted, or under `--dry-run` the number that would be.
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

// runAdoptFromReceipt adopts files from a lore receipt; it is not yet implemented, and refuses.
//
// Parameters:
//   - `receiptPath`: the receipt's path as given; "" when none is.
//   - `layer`: the layer to adopt into.
//   - `project`: the project to adopt into.
//   - `verbose`: whether to narrate per-item progress.
//   - `dryRun`: whether to narrate the would-do steps only.
//
// Returns:
//   - `error`: always non-nil, since `--from-receipt` is not yet implemented.
func runAdoptFromReceipt(receiptPath, layer, project string, verbose, dryRun bool) error {

	// TODO: Implement reading lore receipt and adopting packages-manifest.yaml + config
	return fmt.Errorf("adopt --from-receipt: not yet implemented")
}

// endregion
