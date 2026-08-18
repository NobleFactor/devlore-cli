// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/encryption"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/template"
)

// PinInfo carries the layer-pinning results that ride the graphs' origin annotations.
type PinInfo struct {

	// CommitHashes maps each pinned layer to the commit hash its snapshot was taken at.
	CommitHashes map[string]string

	// DirtyLayers names the layers that had uncommitted changes at pin time (planning proceeded under
	// --allow-dirty).
	DirtyLayers []string
}

// BuildResult is the outcome of planning.
//
// One graph per populated target scope, plus the tree's collision report for command-layer presentation.
type BuildResult struct {

	// Graphs holds one assembled, immutable graph per populated target scope.
	Graphs []*op.Graph

	// Collisions are the cross-layer/specificity conflicts the tree build resolved.
	Collisions []tree.Collision
}

// BuildGraphs walks the source tree and plans one immutable graph per populated target scope.
//
// Single-source mode (no layer sources) yields one unscoped graph against `cfg.TargetRoot`. Multi-source mode
// partitions the tree's winning entries by target scope ("System" / "Home") and yields one graph per populated
// scope, each against its own target root. Each graph's origin annotations carry the writ metadata bag — source
// and target roots, projects, segments, layers, the pin's commit hashes and dirty layers, the run root, and the
// per-unit file inventory (`files`) the readback package folds.
//
// Parameters:
//   - `ctx`: the planning context.
//   - `cfg`: the resolved deploy configuration.
//   - `pin`: the layer-pinning results; zero-valued in single-source mode.
//
// Returns:
//   - `*BuildResult`: the per-scope graphs and the collision report.
//   - `error`: non-nil when the tree build or any scope's planning fails.
func BuildGraphs(ctx context.Context, cfg *Config, pin *PinInfo) (*BuildResult, error) {

	result, err := tree.Build(tree.BuildConfig{
		SourceRoot: cfg.SourceRoot,
		TargetRoot: cfg.TargetRoot,
		Sources:    cfg.LayerSources,
		Projects:   cfg.Projects,
		Segments:   cfg.Segments,
	})
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}

	build := &BuildResult{Collisions: result.Collisions}

	if len(cfg.LayerSources) == 0 {
		if len(result.Files) == 0 {
			return build, nil
		}
		graph, err := buildScopeGraph(ctx, cfg, pin, "", cfg.TargetRoot, nil, result.Files)
		if err != nil {
			return nil, err
		}
		build.Graphs = append(build.Graphs, graph)
		return build, nil
	}

	filesByScope := make(map[string][]*tree.FileEntry)
	for _, f := range result.Files {
		filesByScope[f.TargetName] = append(filesByScope[f.TargetName], f)
	}

	scopeTargetRoots := make(map[string]string)
	for _, src := range cfg.LayerSources {
		scopeTargetRoots[src.TargetName] = src.TargetRoot
	}

	scopes := make([]string, 0, len(filesByScope))
	for scope := range filesByScope {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)

	for _, scope := range scopes {
		graph, err := buildScopeGraph(
			ctx, cfg, pin,
			strings.ToLower(scope), scopeTargetRoots[scope],
			scopeLayers(cfg.LayerSources, scope), filesByScope[scope],
		)
		if err != nil {
			return nil, err
		}
		build.Graphs = append(build.Graphs, graph)
	}

	return build, nil
}

// PlanFileChain plans one file entry's pipeline and returns the target-producing invocation.
//
// Exported as the family's shared chain seam: upgrade re-plans copied entries through the same pipelines.
// The tree's pipelines map onto the sealed actions as: `[file.link]` → one `file.link`;
// `[encryption.decrypt, file.copy]` → one `encryption.decrypt_sops_file` (the decrypt is compound — it reads,
// decrypts, and writes 0600); `[template.render_bytes, file.copy]` → `file.read_text` → `template.render_text`
// → `file.write_text` (promise-chained); the decrypt+render pipeline decrypts to the target, then reads the
// decrypt's product, renders, and rewrites it 0600.
//
// Parameters:
//   - `provider`: the plan provider to register invocations into.
//   - `f`: the tree entry to plan.
//   - `data`: the template data map for render chains.
//
// Returns:
//   - `*op.Invocation`: the final, target-producing invocation (the readback correlates on its unit ID).
//   - `string`: the final invocation's action name.
//   - `error`: non-nil when the pipeline is unknown or a planning call fails.
func PlanFileChain(provider *plan.Provider, f *tree.FileEntry, data map[string]any) (*op.Invocation, string, error) {

	pipeline := strings.Join(f.Operations, "+")

	switch pipeline {

	case "file.link":
		// The link targets the ORIGIN path: the snapshot this entry was read from is removed when the
		// run ends, so a durable link must point at the layer repo itself (ruled 2026-08-08).
		invocation, err := provider.Plan(file.Link, nil, map[string]any{
			"source_path": f.Origin,
			"target_path": f.Target,
		})
		return invocation, string(file.Link), err

	case "encryption.decrypt+file.copy":
		invocation, err := provider.Plan(encryption.DecryptSopsFile, nil, map[string]any{
			"source":           f.Source,
			"destination_path": f.Target,
		})
		return invocation, string(encryption.DecryptSopsFile), err

	case "template.render_bytes+file.copy":
		content, err := provider.Plan(file.ReadText, nil, map[string]any{"resource": f.Source})
		if err != nil {
			return nil, "", err
		}
		rendered, err := provider.Plan(template.RenderText, nil, map[string]any{
			"content": content,
			"data":    data,
		})
		if err != nil {
			return nil, "", err
		}
		mode := f.Mode
		if mode == 0 {
			mode = 0o644
		}
		invocation, err := provider.Plan(file.WriteText, nil, map[string]any{
			"destination_path": f.Target,
			"content":          rendered,
			"mode":             mode,
			"user":             "", "group": "",
		})
		return invocation, string(file.WriteText), err

	case "encryption.decrypt+template.render_bytes+file.copy":
		decrypted, err := provider.Plan(encryption.DecryptSopsFile, nil, map[string]any{
			"source":           f.Source,
			"destination_path": f.Target,
		})
		if err != nil {
			return nil, "", err
		}
		content, err := provider.Plan(file.ReadText, nil, map[string]any{"resource": decrypted})
		if err != nil {
			return nil, "", err
		}
		rendered, err := provider.Plan(template.RenderText, nil, map[string]any{
			"content": content,
			"data":    data,
		})
		if err != nil {
			return nil, "", err
		}
		invocation, err := provider.Plan(file.WriteText, nil, map[string]any{
			"destination_path": f.Target,
			"content":          rendered,
			"mode":             os.FileMode(0o600),
			"user":             "", "group": "",
		})
		return invocation, string(file.WriteText), err

	default:
		return nil, "", fmt.Errorf("unknown pipeline %q", pipeline)
	}
}

// CommonAncestor returns the deepest directory containing both `a` and `b`.
//
// Parameters:
//   - `a`: the first absolute path.
//   - `b`: the second absolute path.
//
// Returns:
//   - `string`: the deepest common ancestor directory.
func CommonAncestor(a, b string) string {

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

// region HELPER FUNCTIONS

// buildScopeGraph plans one scope's graph: deduped parent mkdirs, one planned chain per file entry, and the
// manifest-resolved package units, assembled under an origin carrying the writ annotation bag.
//
// Parameters:
//   - `ctx`: the planning context.
//   - `cfg`: the resolved deploy configuration.
//   - `pin`: the layer-pinning results for the annotation bag.
//   - `scope`: the lowercased target scope ("system" / "home"), or "" in single-source mode.
//   - `targetRoot`: the scope's target root.
//   - `layers`: the layer names contributing to this scope, in source order.
//   - `files`: the tree entries for this scope.
//
// Returns:
//   - `*op.Graph`: the assembled scope graph.
//   - `error`: non-nil when planning or assembly fails.
func buildScopeGraph(
	ctx context.Context, cfg *Config, pin *PinInfo,
	scope, targetRoot string, layers []string, files []*tree.FileEntry,
) (*op.Graph, error) {

	runRoot := runRootFor(cfg, targetRoot, files)

	spec := deploySpec(runRoot, cfg.DryRun, cfg.Conflict)

	return op.Plan(ctx, spec, func(environment *op.RuntimeEnvironment) (*op.Graph, error) {

		provider := plan.NewProvider(environment)

		manifests, chains := splitManifests(files)

		units, err := planManifests(cfg, provider, environment, manifests)
		if err != nil {
			return nil, err
		}

		if err := planParentDirectories(provider, chains); err != nil {
			return nil, err
		}

		fileMetas, err := planChains(provider, chains, templateData(cfg))
		if err != nil {
			return nil, err
		}

		units = append(units, parentlessUnits(provider)...)

		origin := op.NewOriginBase("writ", scope, op.NewAnnotationMap(map[string]any{
			"source_root":   cfg.SourceRoot,
			"target_root":   targetRoot,
			"run_root":      runRoot,
			"projects":      cfg.Projects,
			"segments":      segmentMap(cfg.Segments),
			"layers":        layers,
			"commit_hashes": pin.CommitHashes,
			"dirty_layers":  pin.DirtyLayers,
			"files":         fileMetas,
		}))

		graph, err := op.NewGraph(op.NewGraphSpec().WithOrigin(origin).WithUnits(units...))
		if err != nil {
			return nil, fmt.Errorf("assemble scope %q: %w", scope, err)
		}
		return graph, nil
	})
}

// planParentDirectories plans one deduplicated `file.mkdir` per distinct target parent directory.
//
// The write-family actions do not create parent directories; the mkdirs plan first (registration order is
// execution order) and are idempotent over existing directories.
//
// Parameters:
//   - `provider`: the plan provider to register invocations into.
//   - `files`: the scope's tree entries.
//
// Returns:
//   - `error`: non-nil when a mkdir cannot be planned.
func planParentDirectories(provider *plan.Provider, files []*tree.FileEntry) error {

	seen := make(map[string]bool)
	var directories []string

	for _, f := range files {
		directory := filepath.Dir(f.Target)
		if !seen[directory] {
			seen[directory] = true
			directories = append(directories, directory)
		}
	}

	sort.Strings(directories)

	for _, directory := range directories {
		if _, err := provider.Plan(file.Mkdir, nil, map[string]any{
			"path": directory,
			"mode": os.FileMode(0o755),
			"user": "", "group": "",
		}); err != nil {
			return fmt.Errorf("plan mkdir %s: %w", directory, err)
		}
	}

	return nil
}

// parentlessUnits drains the provider's registered, still-parentless invocation targets as executable units.
//
// Manifest planning wraps its own invocations into phase subgraphs (parenting them); what remains parentless
// is exactly the file chains this package planned.
//
// Parameters:
//   - `provider`: the plan provider whose invocation registry to drain.
//
// Returns:
//   - `[]op.ExecutableUnit`: the parentless units in registration order.
func parentlessUnits(provider *plan.Provider) []op.ExecutableUnit {

	var units []op.ExecutableUnit
	for _, invocation := range provider.InvocationRegistry().All() {
		if invocation.Target.ParentID() == "" {
			units = append(units, invocation.Target)
		}
	}
	return units
}

// deploySpec constructs a fresh [op.RuntimeEnvironmentSpec] anchored at `root` for one phase of a deploy
// (planning or execution — each phase's environment mints its own Root from the anchor and closes it; the
// spec carries no live handle, issue #393).
//
// Parameters:
//   - `root`: the absolute path the confined Root is anchored at.
//   - `dryRun`: forwarded to the application flag map for the framework's dry-run readers.
//   - `conflict`: the write-seam conflict policy, forwarded on the interim flag channel (phase-8 step 49).
//
// Returns:
//   - `*op.RuntimeEnvironmentSpec`: the constructed spec.
func deploySpec(root string, dryRun bool, conflict op.ConflictPolicy) *op.RuntimeEnvironmentSpec {

	return op.NewRuntimeEnvironmentSpec("writ").
		WithStatus(cli.UI()).
		WithRoot(root, fsroot.ModeConfined).
		WithApplication(&application.Application{
			Name:  "writ",
			Flags: map[string]any{"dry-run": dryRun, "conflict": conflict},
		})
}

// runSpec constructs the execution spec for a planned scope graph, anchored at the run root recorded in the
// graph's origin annotations.
//
// Parameters:
//   - `graph`: the scope graph; its origin annotations carry `run_root`.
//   - `dryRun`: forwarded to the application flag map.
//   - `conflict`: the write-seam conflict policy the pre-flight resolved for this run.
//
// Returns:
//   - `*op.RuntimeEnvironmentSpec`: the constructed spec.
//   - `error`: non-nil when the run root is missing from the annotations.
func runSpec(graph *op.Graph, dryRun bool, conflict op.ConflictPolicy) (*op.RuntimeEnvironmentSpec, error) {

	value, ok := graph.Origin().Annotations().Get("run_root")
	root := ""
	if ok {
		root = assert.Type[string]("run_root annotation", value)
	}
	if !ok || root == "" {
		return nil, fmt.Errorf("graph %s carries no run_root annotation", graph.Checksum())
	}

	return deploySpec(root, dryRun, conflict), nil
}

// splitManifests partitions the file entries into packages-manifest sources and file chains.
//
// Manifest-resolved package units plan first: their planner drains its own invocations into phase
// subgraphs, so they must plan before the file chains land in the shared registry.
//
// Parameters:
//   - `files`: the scope's file entries.
//
// Returns:
//   - `manifests`: the packages-manifest source paths.
//   - `chains`: the remaining file-chain entries.
func splitManifests(files []*tree.FileEntry) (manifests []string, chains []*tree.FileEntry) {

	for _, f := range files {
		if len(f.Operations) == 1 && f.Operations[0] == "manifest.resolve" {
			manifests = append(manifests, f.Source)
			continue
		}
		chains = append(chains, f)
	}

	return manifests, chains
}

// planManifests plans the package units for every manifest through the configured manifest planner.
//
// With no planner configured the manifests are skipped with a note — file chains still deploy.
//
// Parameters:
//   - `cfg`: the deploy configuration carrying the planner.
//   - `provider`: the scope's plan provider.
//   - `environment`: the planning runtime environment.
//   - `manifests`: the packages-manifest source paths.
//
// Returns:
//   - `[]op.ExecutableUnit`: the planned package units, in manifest order.
//   - `error`: non-nil when any manifest fails to plan.
func planManifests(
	cfg *Config, provider *plan.Provider, environment *op.RuntimeEnvironment, manifests []string,
) ([]op.ExecutableUnit, error) {

	if len(manifests) == 0 {
		return nil, nil
	}

	if cfg.ManifestPlanner == nil {
		cli.Note("Skipping %d packages-manifest file(s): no manifest planner configured", len(manifests))
		return nil, nil
	}

	var units []op.ExecutableUnit
	for _, m := range manifests {
		_, packageUnits, err := cfg.ManifestPlanner.PlanPackages(provider, environment, m)
		if err != nil {
			return nil, fmt.Errorf("manifest %s: %w", m, err)
		}
		units = append(units, packageUnits...)
	}

	return units, nil
}

// planChains plans every file chain and collects the per-target file metadata for the graph origin.
//
// Parameters:
//   - `provider`: the scope's plan provider.
//   - `chains`: the file-chain entries.
//   - `data`: the template render data.
//
// Returns:
//   - `map[string]any`: per-target metadata keyed by the final invocation's target ID.
//   - `error`: non-nil when any chain fails to plan.
func planChains(provider *plan.Provider, chains []*tree.FileEntry, data map[string]any) (map[string]any, error) {

	fileMetas := make(map[string]any, len(chains))

	for _, f := range chains {
		finalInvocation, action, err := PlanFileChain(provider, f, data)
		if err != nil {
			return nil, fmt.Errorf("plan %s: %w", f.ID, err)
		}
		fileMetas[finalInvocation.Target.ID()] = map[string]any{
			"target":  f.Target,
			"source":  f.Origin,
			"project": f.Project,
			"layer":   f.Layer,
			"action":  action,
		}
	}

	return fileMetas, nil
}

// runRootFor computes the confinement root for one scope: the deepest common ancestor of the scope's target
// root and every entry's source (the run reads sources and writes targets, so the root must span both;
// typically $HOME, degrading toward "/" only when the trees genuinely span that far).
//
// Parameters:
//   - `cfg`: the deploy configuration (single-source fallback root).
//   - `targetRoot`: the scope's target root.
//   - `files`: the scope's tree entries.
//
// Returns:
//   - `string`: the deepest directory containing the target root and every source.
func runRootFor(cfg *Config, targetRoot string, files []*tree.FileEntry) string {

	root := filepath.Clean(targetRoot)

	if len(cfg.LayerSources) == 0 && cfg.SourceRoot != "" {
		root = CommonAncestor(root, filepath.Clean(cfg.SourceRoot))
	}
	for _, f := range files {
		root = CommonAncestor(root, filepath.Dir(f.Source))
		// Links stat and target the origin path, so the root must span it too (ruled 2026-08-08).
		if f.Origin != "" && f.Origin != f.Source {
			root = CommonAncestor(root, filepath.Dir(f.Origin))
		}
	}

	return root
}

// scopeLayers returns the unique layer names contributing to `scope`, in source order (base → team → personal).
//
// Parameters:
//   - `sources`: all layer sources to filter.
//   - `scope`: the target scope name to filter by (e.g. "System", "Home").
//
// Returns:
//   - `[]string`: the unique layer names in source order; empty when no source targets `scope`.
func scopeLayers(sources []tree.LayerSource, scope string) []string {

	seen := make(map[string]bool)
	var layers []string
	for _, src := range sources {
		if src.TargetName == scope && !seen[src.Layer] {
			seen[src.Layer] = true
			layers = append(layers, src.Layer)
		}
	}
	return layers
}

// endregion
