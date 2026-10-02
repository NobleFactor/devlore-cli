// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package adopt

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/segment"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
)

// Config carries the adopt run's inputs from the cobra layer.
type Config struct {

	// Files are the items to adopt, as the user supplied them (files or directories; `~` expands).
	Files []string

	// TargetRoot is the Home scope's root (the user's home directory), which a relative item resolves against.
	TargetRoot string

	// Scopes are the scopes this platform defines, in scope order: each item is adopted into the one whose root is the
	// deepest that holds it (#926).
	Scopes []Scope

	// Layer is the layer's name (personal, team, base); the record names it.
	Layer string

	// LayerPath is the resolved path to the layer directory.
	LayerPath string

	// Project is the origin name within the layer.
	Project string

	// Platform is the segment suffix the adopted files carry (#931): the project directory becomes
	// `<project>.<Platform>`; "" is the platform-neutral directory.
	Platform string

	// Verbose narrates per-item progress.
	Verbose bool

	// DryRun narrates the would-do steps during enumeration; nothing is built or run.
	DryRun bool
}

// region EXPORTED METHODS

// region Behaviors

// ProjectDirectory returns the project directory's name within a scope: the project, suffixed by the platform when
// one is named (#931): `noblefactor-ops`, `noblefactor-ops.Debian`.
//
// Returns:
//   - `string`: the directory name the layer tree matches.
func (c *Config) ProjectDirectory() string {

	if c.Platform == "" {
		return c.Project
	}
	return c.Project + "." + c.Platform
}

// endregion

// endregion

// region EXPORTED FUNCTIONS

// Collect enumerates the configured files into per-scope adoption batches.
//
// Enumeration is intent, not framework work: paths expand and absolutize, missing items report per-item errors,
// existing symlinks warn and skip, directories walk recursively, and each surviving file derives its destinations
// ([Item]) from its inferred scope. An item under no scope's root reports an error as a missing one does, and so does
// one whose scope's root shares no directory with the layer: on Windows, a layer on another drive. Under dry-run the
// would-do steps narrate here; nothing touches the filesystem in either mode.
//
// Parameters:
//   - `cfg`: the adopt configuration.
//
// Returns:
//   - `map[string][]Item`: the batches keyed by scope name, in lower case.
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

// RunBatches executes one adopt graph per scope group and persists each run's graph and trace as the record.
//
// Groups run in scope order, `cfg.Scopes`' order, as deploy's graphs do (#926). An occupied destination anywhere
// refuses the whole batch before a file moves. Each group plans once ([BuildGraph]) and runs once ([runBatch]); the
// trace persists into the current lifetime success or failure (a failed run's journal survives — the step-21 R4
// stance). Per-file "Adopted" lines report post-run (the settled reporting ruling). A failed run stops the remaining
// groups.
//
// Parameters:
//   - `ctx`: the cancellation context for the runs.
//   - `cfg`: the adopt configuration.
//   - `groups`: the per-scope batches from [Collect].
//
// Returns:
//   - `int`: the number of files adopted by the groups that completed.
//   - `error`: non-nil when there is no current deployment (66), a destination is occupied, or planning, persisting
//     or a run fails.
func RunBatches(ctx context.Context, cfg *Config, groups map[string][]Item) (int, error) {

	// An adoption is a write into the current deployment (#922, #931): its trace joins the current lifetime, and
	// with none there is no deployment to adopt into.
	lifetime, err := cli.RequireCurrentLifetime(cli.RunOperationAdopt)
	if err != nil {
		return 0, err
	}

	if err := refuseOccupiedDestinations(groups, cfg.Scopes); err != nil {
		return 0, err
	}

	adopted := 0
	for _, scope := range cfg.Scopes {
		items := groups[scope.Name]
		if len(items) == 0 {
			continue
		}
		if err := runBatch(ctx, cfg, lifetime, scope, items); err != nil {
			return adopted, err
		}
		adopted += len(items)
	}

	return adopted, nil
}

// ValidatePlatform checks a `--platform` value the way deploy will read the directory it names (#931, #944): the
// project suffixed by the platform must pass the selector grammar -- the OS part one word of this machine's chain,
// then the architecture, then each declared segment in configured order -- and must name this machine, so adopt
// never creates a directory the next deploy refuses or skips.
//
// Parameters:
//   - `project`: the project adopted into.
//   - `platform`: the flag's value; "" is valid and means the platform-neutral directory.
//   - `segs`: this machine's segments, the declared extras included.
//
// Returns:
//   - `error`: non-nil when the name breaks the grammar, or names another machine.
func ValidatePlatform(project, platform string, segs segment.Segments) error {

	if platform == "" {
		return nil
	}

	name := project + "." + platform
	selected, grammarErrors := segs.Selector([]string{project}).Select([]string{name})
	if len(grammarErrors) > 0 {
		return fmt.Errorf("invalid --platform %q: %w", platform, grammarErrors[0])
	}
	if len(selected) == 0 {
		host := segs.Host()
		return fmt.Errorf("invalid --platform %q: %s doesn't name this machine, whose OS part is one of %s and whose "+
			"architecture is %s", platform, name, strings.Join(host.Chain, ", "), host.Arch)
	}
	return nil
}

// endregion

// region SUPPORTING TYPES

// Scope is one scope an item can be adopted into, as the cobra layer resolves it from the scope model (#926).
type Scope struct {

	// Name is the scope's name in lower case, as the record and a graph's origin name it: `home`, `system`, `staging`.
	Name string

	// Directory is the scope's directory in the layer: a builtin's own name, or a custom scope's directory as the layer
	// spells it.
	Directory string

	// Root is the path the scope's files deploy beneath; an item's path within the scope is relative to it.
	Root string
}

// endregion

// region HELPER FUNCTIONS

// appendItem derives one file's destinations and appends the [Item] to its scope's batch.
//
// Parameters:
//   - `cfg`: the adopt configuration.
//   - `groups`: the accumulating per-scope batches (mutated in place).
//   - `filePath`: the absolute path of the file to adopt.
//   - `scope`: the file's scope, whose root its path is taken relative to.
//   - `projectDir`: the destination project directory under `<layer>/<scope>/<project>/`.
func appendItem(cfg *Config, groups map[string][]Item, filePath string, scope Scope, projectDir string) {

	relPath, err := filepath.Rel(scope.Root, filePath)
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

	groups[scope.Name] = append(groups[scope.Name], Item{
		Source:   filePath,
		RelPath:  relPath,
		DestDir:  filepath.Dir(destPath),
		DestPath: destPath,
		Scope:    scope.Name,
	})
}

// buildSpec constructs a fresh [op.RuntimeEnvironmentSpec] anchored at `root` for the adopt flow.
//
// The spec carries only the anchor path and mode (issue #393): each phase's environment — planning and
// execution alike — mints its own [fsroot.Dir] from it and closes it, so one spec safely serves both phases.
// The bare [application.Application] satisfies the runtime environment's non-nil requirement; no flag plumbing
// rides it — all slots are immediates or item projections.
//
// Parameters:
//   - `root`: the absolute path the confined Root is anchored at: the deepest directory holding the scope's root and
//     the layer.
//
// Returns:
//   - `*op.RuntimeEnvironmentSpec`: the constructed spec.
func buildSpec(root string) *op.RuntimeEnvironmentSpec {

	return op.NewRuntimeEnvironmentSpec("writ").
		WithStatus(cli.UI()).
		WithRoot(root).
		WithApplication(&application.Application{Name: "writ"})
}

// collectDirectory recursively enumerates a directory's files into their scope's batch.
//
// Parameters:
//   - `cfg`: the adopt configuration.
//   - `groups`: the accumulating per-scope batches (mutated in place).
//   - `dirPath`: the directory to walk.
//   - `scope`: the directory's scope, which every file beneath it shares.
//   - `projectDir`: the destination project directory under `<layer>/<scope>/<project>/`.
func collectDirectory(cfg *Config, groups map[string][]Item, dirPath string, scope Scope, projectDir string) {

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

		appendItem(cfg, groups, path, scope, projectDir)
		return nil
	})
	if err != nil {
		cli.Error("walking directory %s: %v", dirPath, err)
	}
}

// collectItem enumerates a single file or directory into its scope's batch.
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

	scope, held := inferScope(filePath, cfg.Scopes)
	if !held {
		cli.Error("%s: not under any scope's root", item)
		return
	}
	if _, shared := fsroot.CommonAncestor(scope.Root, cfg.LayerPath); !shared {
		cli.Error("%s: %s's root %s and the layer %s share no directory", item, scope.Name, scope.Root, cfg.LayerPath)
		return
	}
	projectDir := filepath.Join(cfg.LayerPath, scope.Directory, cfg.ProjectDirectory())

	if cfg.Verbose {
		cli.Note("File: %s -> scope: %s", filePath, scope.Name)
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

	if info.IsDir() {
		collectDirectory(cfg, groups, filePath, scope, projectDir)
		return
	}

	appendItem(cfg, groups, filePath, scope, projectDir)
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
		return xdg.UserHomeDir() + path[1:]
	}
	if path == "~" {
		return xdg.UserHomeDir()
	}

	return path
}

// inferScope finds the scope a path belongs to: of the scopes given, the one whose root is the deepest that holds it
// (#926, #761).
//
// A root holds a path that is the root or lies beneath it. On Unix, System's root `/` holds every path, so an item
// outside every other scope's root is System's; on Windows, System's root is the system drive's, and an item on
// another drive belongs to no scope. When scopes share the deepest root, the first of them wins.
//
// Parameters:
//   - `filePath`: the absolute path to classify.
//   - `scopes`: the scopes this platform defines, in scope order.
//
// Returns:
//   - `Scope`: the scope that holds `filePath`; the zero value when none does.
//   - `bool`: whether a scope holds `filePath`.
func inferScope(filePath string, scopes []Scope) (Scope, bool) {

	var held Scope
	depth := -1

	for _, scope := range scopes {
		root := filepath.Clean(scope.Root)
		if _, within := fsroot.RelWithin(root, filePath); !within && filepath.Clean(filePath) != root {
			continue
		}
		if len(root) > depth {
			held, depth = scope, len(root)
		}
	}

	return held, depth >= 0
}

// refuseOccupiedDestinations is the existing-destination guard, ahead of any run: an occupied destination refuses
// the whole batch before a file moves, so nothing needs compensating. It lived in the graph as a flow.choose until
// devlore-cli#939 found that such a graph does not load back as a record.
//
// Parameters:
//   - `groups`: the per-scope batches.
//   - `scopes`: the scopes, in the order their batches run.
//
// Returns:
//   - `error`: non-nil, naming the destination, when one already exists.
func refuseOccupiedDestinations(groups map[string][]Item, scopes []Scope) error {

	for _, scope := range scopes {
		for _, item := range groups[scope.Name] {
			if _, err := os.Lstat(item.DestPath); err == nil {
				return fmt.Errorf("adopt: destination already exists: %s", item.DestPath)
			}
		}
	}
	return nil
}

// runBatch plans, persists and runs one scope group's adoptions, and records the run on the lifetime.
//
// The run removes items beneath the scope's root and writes them into the layer, so it is confined to the deepest
// directory that holds both (#926, open question 7); the record's `target_root` stays the scope's root.
//
// Parameters:
//   - `ctx`: the cancellation context for the run.
//   - `cfg`: the adopt configuration.
//   - `lifetime`: the current lifetime the run joins.
//   - `scope`: the group's scope.
//   - `items`: the group's adoptions.
//
// Returns:
//   - `error`: non-nil when the scope's root and the layer share no directory, or planning, persisting the graph, or
//     the run fails.
func runBatch(ctx context.Context, cfg *Config, lifetime *cli.Lifetime, scope Scope, items []Item) error {

	runRoot, shared := fsroot.CommonAncestor(scope.Root, cfg.LayerPath)
	if !shared {
		return fmt.Errorf("adopt run (%s): the scope's root %s and the layer %s share no directory", scope.Name,
			scope.Root, cfg.LayerPath)
	}
	spec := buildSpec(runRoot)

	graph, err := op.Plan(ctx, spec, func(environment *op.RuntimeEnvironment) (*op.Graph, error) {
		return BuildGraph(environment, cfg, scope.Root, items)
	})
	if err != nil {
		return err
	}

	// The record joins a trace to its graph by checksum: without the graph document the fold cannot read the
	// adoption's `files` annotation, and the trace is a finding instead of a record (#931).
	if _, err := cli.WriteGraph(graph); err != nil {
		return fmt.Errorf("persist graph: %w", err)
	}

	executor := op.NewGraphExecutor(graph, spec)
	_, runErr := executor.Run(ctx, nil)

	if trace := executor.Trace(); trace != nil {
		receiptPath, writeErr := cli.WriteLifetimeTrace(lifetime, cli.RunOperationAdopt, graph.Origin().Scope(), trace)
		if writeErr != nil {
			cli.Note("Failed to save receipt: %v", writeErr)
		} else if cfg.Verbose {
			cli.Note("Receipt: %s", receiptPath)
		}
	}

	if runErr != nil {
		return fmt.Errorf("adopt run (%s): %w", scope.Name, runErr)
	}

	for _, item := range items {
		cli.Success("Adopted %s", item.RelPath)
	}
	return nil
}

// endregion
