// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package secret implements the writ secret family: SOPS lifecycle operations scoped to registered layers.
//
// Encrypt writes the `<file>.sops` sibling of each argument through the standard pipeline — one
// `encryption.encrypt_file` unit per file, one graph per containing layer, graphs and traces persisted to the
// execution store. Every argument must lie inside a registered layer's working tree (ruled 2026-08-10); the
// containing layer's root is the runtime environment's confinement root, which bounds `.sops.yaml` discovery
// to the layer and mechanically enforces the root-config shape. Plan: docs/plans/writ-secret-encrypt.md;
// architecture: docs/architecture/3.5.13-encryption-provider.md.
package secret

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/deploy"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/encryption"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
)

// EncryptConfig carries the resolved settings for one encrypt operation.
type EncryptConfig struct {

	// Files are the plaintext paths to encrypt; each must lie inside a registered layer's working tree.
	Files []string

	// DryRun serializes the planned graphs to stdout instead of executing them.
	DryRun bool

	// Verbose narrates per-run receipt paths via [cli.Note].
	Verbose bool
}

// ExecuteEncrypt runs the full encrypt operation.
//
// Contain the arguments in registered layers, plan one graph per layer, and execute them.
//
// Each argument's `<file>.sops` sibling is the destination — an existing sibling refuses before any planning,
// and the plaintext source is never deleted. Recipients and document format come from the `.sops.yaml`
// resolved within the containing layer (the confinement root); a file no creation rule governs fails the run
// with the resolver's error verbatim. Each graph persists via [cli.WriteGraph] before its run and each run's
// trace persists via [cli.WriteTrace] win or lose.
//
// Parameters:
//   - `ctx`: the cancellation context for planning and execution.
//   - `cfg`: the resolved encrypt configuration.
//
// Returns:
//   - `error`: non-nil when containment fails, a destination exists, planning fails, or a run fails.
func ExecuteEncrypt(ctx context.Context, cfg *EncryptConfig) error {

	groups, err := assignToLayers(cfg.Files)
	if err != nil {
		return err
	}

	if err := refuseExistingDestinations(groups); err != nil {
		return err
	}

	var graphs []*op.Graph
	for i := range groups {
		graph, err := buildLayerGraph(ctx, cfg, &groups[i])
		if err != nil {
			return err
		}
		graphs = append(graphs, graph)
	}

	if cfg.DryRun {
		return op.SerializeGraphs(os.Stdout, graphs)
	}

	return runAll(ctx, cfg, graphs)
}

// region HELPER FUNCTIONS

// assignToLayers canonicalizes the files and groups them by the registered layer containing each.
//
// The longest matching layer root wins when registrations nest. A file outside every registered layer is an
// error naming `writ repo add` — the layer-scoping ruling (2026-08-10).
//
// Parameters:
//   - `files`: the argument paths, absolute or relative.
//
// Returns:
//   - `[]layerGroup`: one group per containing layer, layers and files in deterministic order.
//   - `error`: non-nil when a file does not exist or lies outside every registered layer.
func assignToLayers(files []string) ([]layerGroup, error) {

	layers, err := registeredLayers()
	if err != nil {
		return nil, err
	}

	byRoot := make(map[string]*layerGroup)
	for _, file := range files {

		canonical, err := canonicalPath(file)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", file, err)
		}

		name, root := containingLayer(layers, canonical)
		if root == "" {
			return nil, fmt.Errorf(
				"%s is not inside a registered layer; register its repository with 'writ repo add'", file)
		}

		group, ok := byRoot[root]
		if !ok {
			group = &layerGroup{name: name, root: root}
			byRoot[root] = group
		}
		group.files = append(group.files, canonical)
	}

	groups := make([]layerGroup, 0, len(byRoot))
	for _, group := range byRoot {
		sort.Strings(group.files)
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].name < groups[j].name })

	return groups, nil
}

// buildLayerGraph plans one layer's encrypt graph: one `encryption.encrypt_file` unit per file, source order.
//
// Parameters:
//   - `ctx`: the planning context.
//   - `cfg`: the encrypt configuration.
//   - `group`: the layer and its files.
//
// Returns:
//   - `*op.Graph`: the assembled graph.
//   - `error`: non-nil when the spec cannot be configured, or planning or assembly fails.
func buildLayerGraph(ctx context.Context, cfg *EncryptConfig, group *layerGroup) (*op.Graph, error) {

	return op.Plan(ctx, encryptSpec(group.root, cfg.DryRun), func(environment *op.RuntimeEnvironment) (*op.Graph, error) {

		provider := plan.NewProvider(environment)
		fileMetas := make(map[string]any, len(group.files))

		for _, source := range group.files {

			rel, err := deploy.PlanSpacePath(environment, source)
			if err != nil {
				return nil, err
			}

			destination := rel + sopsSuffix
			invocation, err := provider.Plan(encryption.EncryptFile, nil, map[string]any{
				"source":           rel,
				"destination_path": destination,
			})
			if err != nil {
				return nil, fmt.Errorf("plan encrypt %s: %w", source, err)
			}

			fileMetas[invocation.Target.ID()] = map[string]any{
				"source":      source,
				"destination": source + sopsSuffix,
				"layer":       group.name,
			}
		}

		var units []op.ExecutableUnit
		for _, invocation := range provider.InvocationRegistry().All() {
			if invocation.Target.ParentID() == "" {
				units = append(units, invocation.Target)
			}
		}

		origin := op.NewOriginBase("writ", "", op.NewAnnotationMap(map[string]any{
			"layer":    group.name,
			"run_root": group.root,
			"files":    fileMetas,
		}))

		graph, err := op.NewGraph(op.NewGraphSpec().WithOrigin(origin).WithUnits(units...))
		if err != nil {
			return nil, fmt.Errorf("assemble layer %q: %w", group.name, err)
		}
		return graph, nil
	})
}

// canonicalPath resolves a path to its absolute, symlink-free form.
//
// Parameters:
//   - `path`: the path to resolve; must exist.
//
// Returns:
//   - `string`: the canonical path.
//   - `error`: non-nil when the path cannot be made absolute or does not exist.
func canonicalPath(path string) (string, error) {

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

// containingLayer returns the registered layer containing the canonical path; the longest root wins.
//
// Parameters:
//   - `layers`: the registered layers, name to canonical root.
//   - `canonical`: the canonical file path.
//
// Returns:
//   - `name`: the containing layer's name, or empty.
//   - `root`: the containing layer's root, or empty.
func containingLayer(layers map[string]string, canonical string) (name, root string) {

	for candidate, candidateRoot := range layers {
		if canonical != candidateRoot && !strings.HasPrefix(canonical, candidateRoot+string(filepath.Separator)) {
			continue
		}
		if len(candidateRoot) > len(root) {
			name, root = candidate, candidateRoot
		}
	}
	return name, root
}

// encryptSpec constructs a fresh [op.RuntimeEnvironmentSpec] confined at the layer root.
//
// The confinement root doubles as the `.sops.yaml` discovery boundary, so resolution can only ever find the
// layer's root configuration or the XDG fallback.
//
// Parameters:
//   - `root`: the containing layer's canonical root.
//   - `dryRun`: forwarded to the application flag map.
//
// Returns:
//   - `*op.RuntimeEnvironmentSpec`: the constructed spec.
func encryptSpec(root string, dryRun bool) *op.RuntimeEnvironmentSpec {

	return op.NewRuntimeEnvironmentSpec("writ").
		WithStatus(cli.UI()).
		WithRoot(root).
		WithApplication(&application.Application{
			Name:  "writ",
			Flags: map[string]any{"dry-run": dryRun},
		})
}

// refuseExistingDestinations fails when any file's `.sops` sibling already exists — encrypt never overwrites.
//
// Parameters:
//   - `groups`: the layer groups whose destinations are checked.
//
// Returns:
//   - `error`: non-nil listing every existing destination.
func refuseExistingDestinations(groups []layerGroup) error {

	var existing []string
	for i := range groups {
		for _, source := range groups[i].files {
			if _, err := os.Lstat(source + sopsSuffix); err == nil {
				existing = append(existing, source+sopsSuffix)
			}
		}
	}

	if len(existing) > 0 {
		return fmt.Errorf(
			"refusing to overwrite existing destination(s): %s — writ secret encrypt never overwrites",
			strings.Join(existing, ", "))
	}
	return nil
}

// registeredLayers enumerates the registered layers as name to canonical working-tree root.
//
// A missing layers directory yields an empty map (every file then fails containment); broken registrations
// are skipped.
//
// Returns:
//   - `map[string]string`: the registered layers.
//   - `error`: non-nil when the layers directory exists but cannot be read.
func registeredLayers() (map[string]string, error) {

	layers := make(map[string]string)

	entries, err := os.ReadDir(devlore.WritLayersDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return layers, nil
		}
		return nil, fmt.Errorf("read layers directory: %w", err)
	}

	for _, entry := range entries {
		root, err := filepath.EvalSymlinks(filepath.Join(devlore.WritLayersDir(), entry.Name()))
		if err != nil {
			continue
		}
		layers[entry.Name()] = root
	}
	return layers, nil
}

// runAll executes every graph, collecting per-layer failures.
//
// Parameters:
//   - `ctx`: the execution context.
//   - `cfg`: the encrypt configuration.
//   - `graphs`: the assembled graphs, in layer order.
//
// Returns:
//   - `error`: the joined per-layer failures, or nil when every layer succeeds.
func runAll(ctx context.Context, cfg *EncryptConfig, graphs []*op.Graph) error {

	var failures []error

	for _, graph := range graphs {
		if runErr := runGraph(ctx, cfg, graph); runErr != nil {
			layer := layerAnnotation(graph)
			cli.Warn("encrypt layer %s failed: %v", layer, runErr)
			failures = append(failures, fmt.Errorf("layer %s: %w", layer, runErr))
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("%d layer(s) failed: %w", len(failures), errors.Join(failures...))
	}
	return nil
}

// runGraph executes one layer's graph: persist the plan, run, persist the trace, report the summary.
//
// Parameters:
//   - `ctx`: the cancellation context for the run.
//   - `cfg`: the encrypt configuration (dry-run flag, verbosity).
//   - `graph`: the layer graph to execute.
//
// Returns:
//   - `error`: non-nil when the spec cannot be configured, the plan cannot persist, or the run fails.
func runGraph(ctx context.Context, cfg *EncryptConfig, graph *op.Graph) error {

	runRoot := ""
	if value, ok := graph.Origin().Annotations().Get("run_root"); ok {
		runRoot = assert.Type[string]("run_root annotation", value)
	}

	spec := encryptSpec(runRoot, cfg.DryRun)

	if _, err := cli.WriteGraph(graph); err != nil {
		return fmt.Errorf("persist graph: %w", err)
	}

	executor := op.NewGraphExecutor(graph, spec)
	_, runErr := executor.Run(ctx, nil)

	if trace := executor.Trace(); trace != nil {
		if receiptPath, writeErr := cli.WriteTrace(trace); writeErr != nil {
			cli.Warn("failed to write receipt: %v", writeErr)
		} else if cfg.Verbose {
			cli.Note("Receipt: %s", receiptPath)
		}

		if runErr == nil {
			summary := trace.Summarize(graph)
			encrypted := summary.ByAction()[string(encryption.EncryptFile)].Completed()
			cli.Success("Encrypted %d file(s) [%s]", encrypted, layerAnnotation(graph))
		}
	}

	return runErr
}

// layerAnnotation returns the graph's layer annotation, or "unknown".
//
// Parameters:
//   - `graph`: the graph whose layer is wanted.
//
// Returns:
//   - `string`: the layer name.
func layerAnnotation(graph *op.Graph) string {

	if value, ok := graph.Origin().Annotations().Get("layer"); ok {
		return assert.Type[string]("layer annotation", value)
	}
	return "unknown"
}

// layerGroup collects one registered layer's files for a single graph.
type layerGroup struct {
	name  string
	root  string
	files []string
}

// sopsSuffix is the destination naming convention: `<file>.sops` deploys as `<file>`.
const sopsSuffix = ".sops"

// endregion
