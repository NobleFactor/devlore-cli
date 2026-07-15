// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package adopt

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Config carries the adopt run's inputs from the cobra layer.
type Config struct {

	// Files are the items to adopt, as the user supplied them (files or directories; `~` expands).
	Files []string

	// TargetRoot is the Home scope's root (the user's home directory).
	TargetRoot string

	// LayerPath is the resolved path to the layer directory.
	LayerPath string

	// Project is the origin name within the layer.
	Project string

	// Verbose narrates per-item progress.
	Verbose bool

	// DryRun narrates the would-do steps during enumeration; nothing is built or run.
	DryRun bool
}

// Collect enumerates the configured files into per-scope adoption batches.
//
// Enumeration is intent, not framework work: paths expand and absolutize, missing items report per-item errors,
// existing symlinks warn and skip, directories walk recursively, and each surviving file derives its destinations
// ([Item]) from its inferred scope. Under dry-run the would-do steps narrate here; nothing touches the filesystem in
// either mode.
//
// Parameters:
//   - `cfg`: the adopt configuration.
//
// Returns:
//   - map[string][]Item: the batches keyed by scope root (`cfg.TargetRoot` for Home, "/" for System).
func Collect(cfg *Config) map[string][]Item {

	if cfg.Verbose {
		cli.Note("Layer path: %s", cfg.LayerPath)
		cli.Note("Origin: %s", cfg.Project)
	}

	groups := make(map[string][]Item)
	for _, item := range cfg.Files {
		collectItem(cfg, groups, item)
	}
	return groups
}

// RunBatches executes one adopt graph per scope group and persists each run's trace as the receipt.
//
// Groups run in deterministic (sorted-root) order. Each group plans once ([BuildGraph]) and runs once; the trace
// persists via [cli.WriteTrace] success or failure (a failed run's journal survives — the step-21 R4 stance).
// Per-file "Adopted" lines report post-run (the settled reporting ruling). A failed run stops the remaining groups.
//
// Parameters:
//   - `ctx`: the cancellation context for the runs.
//   - `cfg`: the adopt configuration.
//   - `groups`: the per-scope batches from [Collect].
//
// Returns:
//   - `int`: the number of files adopted by the groups that completed.
//   - `error`: non-nil when planning, preflight, or a run fails.
func RunBatches(ctx context.Context, cfg *Config, groups map[string][]Item) (int, error) {

	roots := make([]string, 0, len(groups))
	for root := range groups {
		roots = append(roots, root)
	}
	sort.Strings(roots)

	adopted := 0
	for _, root := range roots {

		items := groups[root]

		planningSpec, err := buildSpec(root)
		if err != nil {
			return adopted, err
		}

		graph, err := op.Plan(ctx, planningSpec, func(env *op.RuntimeEnvironment) (*op.Graph, error) {
			return BuildGraph(env, items)
		})
		if err != nil {
			return adopted, err
		}

		executeSpec, err := buildSpec(root)
		if err != nil {
			return adopted, err
		}

		executor := op.NewGraphExecutor(graph, executeSpec)
		_, runErr := executor.Run(ctx, nil)

		if trace := executor.Trace(); trace != nil {
			if receiptPath, writeErr := cli.WriteTrace(trace); writeErr != nil {
				cli.Note("Failed to save receipt: %v", writeErr)
			} else if cfg.Verbose {
				cli.Note("Receipt: %s", receiptPath)
			}
		}

		if runErr != nil {
			return adopted, fmt.Errorf("adopt run (%s): %w", root, runErr)
		}

		for _, item := range items {
			cli.Success("Adopted %s", item.RelPath)
		}
		adopted += len(items)
	}

	return adopted, nil
}

// region HELPER FUNCTIONS

// collectItem enumerates a single file or directory into its scope batch.
//
// Parameters:
//   - `cfg`: the adopt configuration.
//   - `groups`: the accumulating per-scope batches (mutated in place).
//   - `item`: the file or directory argument as the user supplied it.
func collectItem(cfg *Config, groups map[string][]Item, item string) {

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
		return
	}

	if info.Mode()&os.ModeSymlink != 0 {
		cli.Warn("%s: already a symlink (skip)", item)
		return
	}

	targetRoot := cfg.TargetRoot
	if scope == "System" {
		targetRoot = "/"
	}

	if info.IsDir() {
		collectDirectory(cfg, groups, filePath, targetRoot, projectDir)
		return
	}

	appendItem(cfg, groups, filePath, targetRoot, projectDir)
}

// collectDirectory recursively enumerates a directory's files into their scope batch.
//
// Parameters:
//   - `cfg`: the adopt configuration.
//   - `groups`: the accumulating per-scope batches (mutated in place).
//   - `dirPath`: the directory to walk.
//   - `targetRoot`: the scope's root (`cfg.TargetRoot` for Home, "/" for System).
//   - `projectDir`: the destination project directory under `<layer>/<scope>/<project>/`.
func collectDirectory(cfg *Config, groups map[string][]Item, dirPath, targetRoot, projectDir string) {

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

		appendItem(cfg, groups, path, targetRoot, projectDir)
		return nil
	})
	if err != nil {
		cli.Error("walking directory %s: %v", dirPath, err)
	}
}

// appendItem derives one file's destinations and appends the [Item] to its scope batch.
//
// Parameters:
//   - `cfg`: the adopt configuration.
//   - `groups`: the accumulating per-scope batches (mutated in place).
//   - `filePath`: the absolute path of the file to adopt.
//   - `targetRoot`: the scope's root (`cfg.TargetRoot` for Home, "/" for System).
//   - `projectDir`: the destination project directory under `<layer>/<scope>/<project>/`.
func appendItem(cfg *Config, groups map[string][]Item, filePath, targetRoot, projectDir string) {

	relPath, err := filepath.Rel(targetRoot, filePath)
	if err != nil {
		cli.Error("%s: cannot compute relative path: %v", filePath, err)
		return
	}

	destPath := filepath.Join(projectDir, relPath)

	if cfg.Verbose {
		cli.Note("%s -> %s", filePath, destPath)
	}
	if cfg.DryRun {
		cli.Note("Would adopt %s -> %s", relPath, destPath)
		cli.Note("Would symlink %s -> %s", filePath, destPath)
	}

	groups[targetRoot] = append(groups[targetRoot], Item{
		Source:   filePath,
		RelPath:  relPath,
		DestDir:  filepath.Dir(destPath),
		DestPath: destPath,
	})
}

// buildSpec constructs a fresh [op.RuntimeEnvironmentSpec] confined at `root` for one phase of the adopt flow.
//
// Each call mints a fresh [fsroot.Root] handle so the planning environment's Close (which closes the spec's Root)
// does not invalidate the execution phase's spec. The bare [application.Application] satisfies the runtime
// environment's non-nil requirement; no flag plumbing rides it — all slots are immediates or item projections.
//
// Parameters:
//   - `root`: the absolute path the confined Root is anchored at (the scope root).
//
// Returns:
//   - *op.RuntimeEnvironmentSpec: the constructed spec.
//   - `error`: non-nil when [fsroot.OpenConfined] fails.
func buildSpec(root string) (*op.RuntimeEnvironmentSpec, error) {

	confined, err := fsroot.OpenConfined(root)
	if err != nil {
		return nil, fmt.Errorf("open root %s: %w", root, err)
	}

	return op.NewRuntimeEnvironmentSpec("writ").
		WithRoot(confined).
		WithApplication(&application.Application{Name: "writ"}), nil
}

// inferScope determines whether a file path belongs to Home or System scope.
//
// Unix: paths under `$HOME` are Home, paths under `/` are System. Windows: paths under `%USERPROFILE%` are Home,
// paths under `%SystemRoot%` are System.
//
// Parameters:
//   - `filePath`: the absolute path to classify.
//   - `homeDir`: the Home scope's root.
//
// Returns:
//   - `string`: "Home" or "System".
func inferScope(filePath, homeDir string) string {

	filePath = filepath.Clean(filePath)
	homeDir = filepath.Clean(homeDir)

	if strings.HasPrefix(filePath, homeDir+string(filepath.Separator)) || filePath == homeDir {
		return "Home"
	}

	return "System"
}

// expandPath expands a leading `~` to `$HOME`.
//
// Parameters:
//   - `path`: the path as the user supplied it.
//
// Returns:
//   - `string`: the expanded path.
func expandPath(path string) string {

	if strings.HasPrefix(path, "~/") {
		return os.Getenv("HOME") + path[1:]
	}
	if path == "~" {
		return os.Getenv("HOME")
	}

	return path
}

// endregion
