// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/adopt"
	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// newAdoptCmd constructs the cobra command for `writ adopt`.
//
// Moves files from their target location into the project directory and creates symlinks back. Scope (Home or
// System) is inferred per-item from the path relative to `$HOME`. Directories are walked recursively; existing
// symlinks within directories are skipped.
//
// Extracted from `commands.go` under Phase 6.B (literal relocation, no behavior change). Phase 6.C rewires
// `adoptFile`'s nil-activation `file.Mkdir`/`file.Move`/`file.Link` call sites onto the
// `graph + VariableResolver + GraphExecutor.Run` path via the new `cmd/writ/writ/adopt` subpackage.
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

// adoptFiles processes each file for adoption.
func adoptFiles(cfg *AdoptConfig) int {
	if cfg.Verbose {
		cli.Note("Layer: %s", cfg.Layer)
		cli.Note("Layer path: %s", cfg.LayerPath)
		cli.Note("Origin: %s", cfg.Project)
	}

	var adopted int
	for _, item := range cfg.Files {
		count := adoptItem(cfg, item)
		adopted += count
	}
	return adopted
}

// adoptItem processes a single file or directory for adoption.
func adoptItem(cfg *AdoptConfig, item string) int {
	filePath := expandPath(item)
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
			cli.Error("%s: file does not exist", item)
		} else {
			cli.Error("%s: %v", item, err)
		}
		return 0
	}

	if info.Mode()&os.ModeSymlink != 0 {
		cli.Warn("%s: already a symlink (skip)", item)
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
		cli.Error("%s: %v", item, err)
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
	return fmt.Errorf("adopt --from-receipt: not yet implemented")
}

// adoptFile moves a single file to the project directory and creates a symlink back.
//
// Phase 6.C migration target: constructs the mkdir → move → link graph via [adopt.BuildGraph], populates the
// application's flag map with the per-file resolved paths so the variable resolver picks them up at preflight, and
// dispatches via [op.GraphExecutor.Run] wrapped by [adopt.Run]. Cross-filesystem move recovery (the previous
// EXDEV→copy+remove fallback) was intentionally dropped under Phase 6 Q2 — `writ adopt` runs within a single
// `os.Root`, and EXDEV is now surfaced as the user-visible error rather than silently hidden. Users who hit the
// edge case can wire a per-call `error_action=` subgraph; a CLI flag is a future enhancement.
//
// Parameters:
//   - `filePath`: the absolute path of the file being adopted (the source location).
//   - `targetRoot`: the cobra-resolved target root the file lives under (`$HOME` for Home scope, `/` for System).
//   - `projectDir`: the destination project directory under `<layer>/<scope>/<project>/`.
//   - `verbose`: when true, narrates per-file progress via [cli.Note].
//   - `dryRun`: when true, skips graph construction entirely and narrates the would-do steps.
//
// Returns:
//   - `int`: 1 on successful adoption (or in dry-run mode); 0 on failure.
//   - `error`: non-nil when path computation, graph construction, preflight, or dispatch fails. Errors are mapped
//     by [adopt.Run] / [adopt.mapAdoptError] to preserve the legacy `creating directory %s: %w` / `moving file: %w`
//     / `creating symlink: %w` prefix style.
func adoptFile(filePath, targetRoot, projectDir string, verbose, dryRun bool) (int, error) {

	relPath, err := filepath.Rel(targetRoot, filePath)
	if err != nil {
		return 0, fmt.Errorf("cannot compute relative path: %w", err)
	}

	destPath := filepath.Join(projectDir, relPath)
	destDir := filepath.Dir(destPath)

	if verbose {
		cli.Note("%s -> %s", filePath, destPath)
	}

	if dryRun {
		cli.Note("Would adopt %s -> %s", relPath, destPath)
		cli.Note("Would symlink %s -> %s", filePath, destPath)
		return 1, nil
	}

	if _, err := os.Stat(destPath); err == nil {
		return 0, fmt.Errorf("destination already exists: %s", destPath)
	}

	flags := map[string]any{
		"dest_dir":    destDir,
		"source_path": filePath,
		"dest_path":   destPath,
		"dry-run":     dryRun,
	}

	ctx := context.Background()

	planningSpec, err := buildAdoptSpec(targetRoot, flags)
	if err != nil {
		return 0, err
	}

	graph, err := op.Plan(ctx, planningSpec, func(env *op.RuntimeEnvironment) (*op.Graph, error) {
		return adopt.BuildGraph(env)
	})
	if err != nil {
		return 0, err
	}

	executeSpec, err := buildAdoptSpec(targetRoot, flags)
	if err != nil {
		return 0, err
	}

	if err := adopt.Run(ctx, op.NewGraphExecutor(graph, executeSpec)); err != nil {
		return 0, err
	}

	cli.Success("Adopted %s", relPath)
	return 1, nil
}

// buildAdoptSpec constructs a fresh [op.RuntimeEnvironmentSpec] for one phase of the adopt flow (planning or
// execution). Each call mints a fresh [op.Root] handle so the planning env's [op.RuntimeEnvironment.Close]
// (which closes the Root) doesn't invalidate the spec for the subsequent execution Run. The Root, registry, and
// Application are all per-spec — planning and execution share nothing but the resolved graph.
//
// Parameters:
//   - `targetRoot`: the absolute path the confined Root is anchored at.
//   - `flags`: the Application's flag map; passed to both specs so the variable resolver picks up the same
//     per-file values during preflight in each phase.
//
// Returns:
//   - *op.RuntimeEnvironmentSpec: the constructed spec.
//   - `error`: non-nil when [op.NewConfinedRoot] fails (the target root does not exist or is not accessible).
func buildAdoptSpec(targetRoot string, flags map[string]any) (*op.RuntimeEnvironmentSpec, error) {

	root, err := op.NewConfinedRoot(targetRoot)
	if err != nil {
		return nil, fmt.Errorf("open root %s: %w", targetRoot, err)
	}

	return op.NewRuntimeEnvironmentSpec("writ", op.NewReceiverRegistry()).
		WithRoot(root).
		WithApplication(&application.Application{
			Name:  "writ",
			Flags: flags,
		}), nil
}
