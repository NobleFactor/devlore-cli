// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
)

// RegisterLayer registers `sourceRoot` as the layer at `layerDir` — the migrate flow's phase 5.
//
// Its own graph and run per the settled two-run design (a failed registration must not unwind the completed
// restructure).
//
// Link mode (`useMove` false, the default) symlinks `layerDir` → `sourceRoot`; move mode moves the content into
// `layerDir`. The [clearExistingLayer] guard runs Go-side first (remove a symlink or empty directory at the layer
// path; refuse a non-empty directory). The graph — `file.mkdir(<layers-parent>)` then `file.link` or `file.move`,
// immediate-bound — runs under a root confined at the deepest common ancestor of `sourceRoot` and `layerDir` (the
// settled confinement ruling: computed over the actual paths, typically `$HOME`). The run's trace persists via
// [cli.WriteTrace], success or failure.
//
// Parameters:
//   - `ctx`: the cancellation context for the run.
//   - `sourceRoot`: the absolute path of the migrated environment repository.
//   - `layerDir`: the absolute layer directory (`<layers>/<layer>`).
//   - `useMove`: move the content into the layer directory instead of symlinking.
//   - `verbose`: narrate progress via [cli.Note].
//
// Returns:
//   - `error`: non-nil when the guard refuses, planning fails, or the run fails.
func RegisterLayer(ctx context.Context, sourceRoot, layerDir string, useMove, verbose bool) error {

	if err := clearExistingLayer(layerDir, verbose); err != nil {
		return err
	}

	root := commonAncestor(sourceRoot, layerDir)

	if verbose {
		if useMove {
			cli.Note("Moving: %s -> %s (root %s)", sourceRoot, layerDir, root)
		} else {
			cli.Note("Creating symlink: %s -> %s (root %s)", layerDir, sourceRoot, root)
		}
	}

	spec := migrateSpec(root)

	graph, err := op.Plan(ctx, spec, func(environment *op.RuntimeEnvironment) (*op.Graph, error) {
		return buildRegistrationGraph(environment, sourceRoot, layerDir, useMove)
	})
	if err != nil {
		return err
	}

	executor := op.NewGraphExecutor(graph, spec)
	_, runErr := executor.Run(ctx, nil)

	if trace := executor.Trace(); trace != nil {
		if receiptPath, writeErr := cli.WriteTrace(trace); writeErr != nil {
			cli.Note("Failed to save registration receipt: %v", writeErr)
		} else if verbose {
			cli.Note("Registration receipt: %s", receiptPath)
		}
	}

	if runErr != nil {
		return fmt.Errorf("register layer %s: %w", layerDir, runErr)
	}

	return nil
}

// region HELPER FUNCTIONS

// buildRegistrationGraph constructs the two-node registration graph: create the layers parent, then link or move.
//
// Parameters:
//   - `environment`: the planning runtime environment.
//   - `sourceRoot`: the migrated repository (the link target / move source).
//   - `layerDir`: the layer directory to register.
//   - `useMove`: move instead of link.
//
// Returns:
//   - *op.Graph: the assembled registration graph.
//   - `error`: non-nil when planning or assembly fails.
func buildRegistrationGraph(
	environment *op.RuntimeEnvironment, sourceRoot, layerDir string, useMove bool,
) (*op.Graph, error) {

	planProvider := plan.NewProvider(environment)

	mkdirInvocation, err := planProvider.Plan(file.Mkdir, nil, map[string]any{
		"path":  filepath.Dir(layerDir),
		"chmod": os.FileMode(0o755),
		"chown": "",
	})
	if err != nil {
		return nil, fmt.Errorf("migrate.RegisterLayer: plan file.mkdir: %w", err)
	}

	var registerInvocation *op.Invocation
	if useMove {
		registerInvocation, err = planProvider.Plan(file.Move, nil, map[string]any{
			"source_path":      sourceRoot,
			"destination_path": layerDir,
		})
	} else {
		registerInvocation, err = planProvider.Plan(file.Link, nil, map[string]any{
			"source_path": sourceRoot,
			"target_path": layerDir,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("migrate.RegisterLayer: plan the registration node: %w", err)
	}

	graph, err := planProvider.AssembleDefinition(
		[]*op.Invocation{mkdirInvocation, registerInvocation},
		nil, nil, nil, nil, nil,
		planProvider.Origin("migrate"),
	)
	if err != nil {
		return nil, fmt.Errorf("migrate.RegisterLayer: assemble: %w", err)
	}

	return graph, nil
}

// clearExistingLayer removes an existing symlink or empty directory at `layerDir`, refusing anything else.
//
// The Go-side precondition guard ahead of the registration run: nil when `layerDir` does not exist; a symlink or an
// empty directory is removed; a non-empty directory or a non-directory file is refused with an error.
//
// Parameters:
//   - `layerDir`: the layer path to clear.
//   - `verbose`: narrate removals via [cli.Note].
//
// Returns:
//   - `error`: non-nil when `layerDir` holds a non-empty directory or an unexpected file, or a removal fails.
func clearExistingLayer(layerDir string, verbose bool) error {

	info, err := os.Lstat(layerDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		if verbose {
			cli.Note("Removing existing symlink: %s", layerDir)
		}
		if err := os.Remove(layerDir); err != nil {
			return fmt.Errorf("remove existing symlink: %w", err)
		}
		return nil
	}

	if info.IsDir() {
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
		return nil
	}

	return fmt.Errorf("layer path %s exists and is not a directory or symlink", layerDir)
}

// commonAncestor returns the deepest directory containing both `a` and `b`.
//
// The settled confinement ruling for the registration graph: the run's Root anchors here, so the graph reaches both
// the source repository and the layers tree with the tightest confinement the operation allows (typically `$HOME`;
// `/` only when the trees genuinely span that far).
//
// Parameters:
//   - `a`: the first absolute path.
//   - `b`: the second absolute path.
//
// Returns:
//   - `string`: the deepest common ancestor directory.
func commonAncestor(a, b string) string {

	segmentsA := strings.Split(filepath.Clean(a), string(filepath.Separator))
	segmentsB := strings.Split(filepath.Clean(b), string(filepath.Separator))

	var common []string
	for i := 0; i < len(segmentsA) && i < len(segmentsB) && segmentsA[i] == segmentsB[i]; i++ {
		common = append(common, segmentsA[i])
	}

	ancestor := strings.Join(common, string(filepath.Separator))
	if ancestor == "" {
		return string(filepath.Separator)
	}
	return ancestor
}

// endregion
