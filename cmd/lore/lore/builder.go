// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lore

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/internal/manifest"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// lifecycleVerbs are the plan.* orchestration attributes denied to phase-script runtimes. Scripts only contribute
// invocations into the shared registry; lore alone assembles, runs, and persists.
var lifecycleVerbs = []string{"assemble", "clear", "load", "run", "save"}

// BuildResult contains the built execution graph and metadata for packages.
type BuildResult struct {
	// Graph is the execution graph ready for execution.
	Graph *op.Graph

	// Packages lists the resolved package names.
	Packages []string

	// Platform is the detected or specified platform.
	Platform string
}

// BuildConfig holds configuration for building a package graph.
type BuildConfig struct {
	// ManifestPath is the path to a packages-manifest.yaml file. Mutually exclusive with Packages.
	ManifestPath string

	// Packages is a list of package names to install. Mutually exclusive with ManifestPath.
	Packages []string

	// Platform is the target platform (e.g., "Darwin", "Linux.Debian"). If empty, auto-detected.
	Platform string

	// Features are optional feature flags to enable.
	Features []string

	// Settings are key-value configuration settings.
	Settings map[string]string

	// DryRun prevents actual installation when true.
	DryRun bool

	// RegistryClient provides access to the package registry. If nil, a default client is created.
	RegistryClient *lorepackage.Registry

	// ActionRegistry provides access to execution actions. Must be set before calling Build.
	ActionRegistry *op.ReceiverRegistry
}

// Planner resolves packages and plans their lifecycle phases against a shared [plan.Provider].
//
// One Planner drives one build: every package and every phase registers its invocations into the same provider's
// session ledger, and the phases are grouped into subgraphs by [Planner.buildPackage].
type Planner struct {
	Platform       string
	ActionRegistry *op.ReceiverRegistry
	RegistryClient *lorepackage.Registry
	Features       []string
	Settings       map[string]string
	DryRun         bool
}

// Build creates an execution graph from the given configuration.
//
// One shared [op.RuntimeEnvironment] backs the whole build: lore's Go-side native-software invocations and the
// `.star` phase scripts both register into the env's cached [plan.Provider], so they pool in one invocation ledger.
// Each lifecycle phase becomes a subgraph of that phase's contributions; the phase subgraphs are the roots of the
// returned graph, stamped with a lore [op.Origin].
//
// Parameters:
//   - `cfg`: the build configuration (manifest path or package list, platform, options).
//
// Returns:
//   - `*BuildResult`: the execution graph and metadata.
//   - `error`: non-nil if the configuration is invalid or graph building fails.
func Build(cfg BuildConfig) (*BuildResult, error) {

	if cfg.ManifestPath != "" && len(cfg.Packages) > 0 {
		return nil, fmt.Errorf("lore.Build: cannot specify both ManifestPath and Packages")
	}
	if cfg.ManifestPath == "" && len(cfg.Packages) == 0 {
		return nil, fmt.Errorf("lore.Build: must specify either ManifestPath or Packages")
	}

	targetPlatform := cfg.Platform
	if targetPlatform == "" {
		targetPlatform = detectPlatform()
	}

	reg := cfg.ActionRegistry
	if reg == nil {
		reg = op.NewReceiverRegistry()
	}

	sharedEnv := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("lore", reg).
		WithModules(reg.Modules()...).
		WithApplication(&application.Application{Name: "lore"}))

	provider, err := sharedProvider(sharedEnv)
	if err != nil {
		return nil, err
	}

	planner := &Planner{
		Platform:       targetPlatform,
		ActionRegistry: reg,
		RegistryClient: cfg.RegistryClient,
		Features:       cfg.Features,
		Settings:       cfg.Settings,
		DryRun:         cfg.DryRun,
	}

	var packages []string
	var phases []op.ExecutableUnit

	if cfg.ManifestPath != "" {
		packages, phases, err = planner.PlanPackages(provider, sharedEnv, cfg.ManifestPath)
	} else {
		packages, phases, err = planner.PlanByName(provider, sharedEnv, cfg.Packages)
	}
	if err != nil {
		return nil, err
	}

	origin := op.NewOriginBase("lore", strings.Join(packages, "+"), op.NewAnnotationMap(map[string]any{
		"packages": packages,
		"platform": targetPlatform,
		"features": cfg.Features,
		"settings": cfg.Settings,
	}))

	graph, err := op.NewGraph(op.NewGraphSpec().WithOrigin(origin).WithUnits(phases...))
	if err != nil {
		return nil, fmt.Errorf("lore.Build: %w", err)
	}

	return &BuildResult{Graph: graph, Packages: packages, Platform: targetPlatform}, nil
}

// BuildFromManifest creates an execution graph from a packages-manifest.yaml file.
//
// Parameters:
//   - `manifestPath`: the path to the packages-manifest file.
//   - `targetPlatform`: the platform string (empty for auto-detect).
//
// Returns:
//   - `*BuildResult`: the execution graph and metadata.
//   - `error`: non-nil if graph building fails.
func BuildFromManifest(manifestPath, targetPlatform string) (*BuildResult, error) {
	return Build(BuildConfig{ManifestPath: manifestPath, Platform: targetPlatform})
}

// BuildFromPackages creates an execution graph from a list of package names.
//
// Parameters:
//   - `packages`: the package names to resolve and install.
//   - `targetPlatform`: the platform string (empty for auto-detect).
//
// Returns:
//   - `*BuildResult`: the execution graph and metadata.
//   - `error`: non-nil if graph building fails.
func BuildFromPackages(packages []string, targetPlatform string) (*BuildResult, error) {
	return Build(BuildConfig{Packages: packages, Platform: targetPlatform})
}

// region EXPORTED METHODS

// region Behaviors

// PlanPackages parses a packages-manifest file and plans every package's phases into `provider`.
//
// Parameters:
//   - `provider`: the shared plan provider all invocations register into.
//   - `sharedEnv`: the shared runtime environment the phase scripts run against.
//   - `manifestPath`: the path to the packages-manifest file.
//
// Returns:
//   - `[]string`: the resolved package names.
//   - `[]op.ExecutableUnit`: the phase subgraphs, in build order.
//   - `error`: non-nil if manifest parsing, package resolution, or phase building fails.
func (p *Planner) PlanPackages(provider *plan.Provider, sharedEnv *op.RuntimeEnvironment, manifestPath string) ([]string, []op.ExecutableUnit, error) {

	loaded, err := manifest.Load(manifestPath)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing manifest: %w", err)
	}

	targetPlatform, reg, registryClient, err := p.resolve()
	if err != nil {
		return nil, nil, err
	}

	var names []string
	var phases []op.ExecutableUnit

	for _, entry := range loaded.Packages {
		release, err := registryClient.Resolve(entry.Name, targetPlatform)
		if err != nil {
			return nil, nil, fmt.Errorf("resolving package %q: %w", entry.Name, err)
		}

		cfg := BuildConfig{Features: mergeFeatures(entry.With, p.Features), Settings: p.Settings, DryRun: p.DryRun}

		built, err := p.buildPackage(provider, sharedEnv, release, targetPlatform, cfg, reg)
		if err != nil {
			return nil, nil, fmt.Errorf("building %q: %w", entry.Name, err)
		}

		names = append(names, entry.Name)
		phases = append(phases, built...)
	}

	return names, phases, nil
}

// PlanByName resolves explicit package names and plans their phases into `provider`.
//
// Parameters:
//   - `provider`: the shared plan provider all invocations register into.
//   - `sharedEnv`: the shared runtime environment the phase scripts run against.
//   - `packages`: the package names to resolve and plan.
//
// Returns:
//   - `[]string`: the resolved package names.
//   - `[]op.ExecutableUnit`: the phase subgraphs, in build order.
//   - `error`: non-nil if any package resolution or phase building fails.
func (p *Planner) PlanByName(provider *plan.Provider, sharedEnv *op.RuntimeEnvironment, packages []string) ([]string, []op.ExecutableUnit, error) {

	targetPlatform, reg, registryClient, err := p.resolve()
	if err != nil {
		return nil, nil, err
	}

	cfg := BuildConfig{Features: p.Features, Settings: p.Settings, DryRun: p.DryRun}

	var names []string
	var phases []op.ExecutableUnit

	for _, name := range packages {
		release, err := registryClient.Resolve(name, targetPlatform)
		if err != nil {
			return nil, nil, fmt.Errorf("resolving package %q: %w", name, err)
		}

		built, err := p.buildPackage(provider, sharedEnv, release, targetPlatform, cfg, reg)
		if err != nil {
			return nil, nil, fmt.Errorf("building %q: %w", name, err)
		}

		names = append(names, name)
		phases = append(phases, built...)
	}

	return names, phases, nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// resolve returns the resolved platform, action registry, and package registry client, auto-creating any that are nil.
//
// Returns:
//   - `string`: the target platform string.
//   - `*op.ReceiverRegistry`: the action registry (created if nil on the Planner).
//   - `*lorepackage.Registry`: the package registry client (created if nil on the Planner).
//   - `error`: non-nil if creating the registry client fails.
func (p *Planner) resolve() (string, *op.ReceiverRegistry, *lorepackage.Registry, error) {

	targetPlatform := p.Platform
	if targetPlatform == "" {
		targetPlatform = detectPlatform()
	}

	reg := p.ActionRegistry
	if reg == nil {
		reg = op.NewReceiverRegistry()
	}

	registryClient := p.RegistryClient
	if registryClient == nil {
		client, err := lorepackage.NewRegistry()
		if err != nil {
			return "", nil, nil, fmt.Errorf("creating registry client: %w", err)
		}
		registryClient = client
	}

	return targetPlatform, reg, registryClient, nil
}

// buildPackage plans every lifecycle phase of `release` into `provider` and returns one subgraph per non-empty phase.
//
// Each phase runs its actions — `.star` scripts on a deny-restricted runtime over `sharedEnv`, and native-software
// actions via [Planner.addNativeSoftwarePackages] — all of which register leaf invocations into the shared ledger.
// After a phase's actions run, every still-parentless invocation Target is that phase's contribution; they become the
// children of the phase subgraph (which stamps their parent), so the next phase's parentless set is exactly the next
// phase's nodes. Phase subgraphs are not registered in the ledger; lore returns them as the graph's roots.
//
// Parameters:
//   - `provider`: the shared plan provider.
//   - `sharedEnv`: the shared runtime environment the scripts run against.
//   - `release`: the resolved package release.
//   - `targetPlatform`: the target platform string.
//   - `cfg`: the per-package build configuration.
//   - `reg`: the action registry, for resolving the structural-subgraph action.
//
// Returns:
//   - `[]op.ExecutableUnit`: the phase subgraphs, in phase order.
//   - `error`: non-nil if script execution, native planning, or subgraph construction fails.
func (p *Planner) buildPackage(provider *plan.Provider, sharedEnv *op.RuntimeEnvironment, release *lorepackage.Release, targetPlatform string, cfg BuildConfig, reg *op.ReceiverRegistry) ([]op.ExecutableUnit, error) {

	subgraphAction, err := reg.BuildAction("flow.subgraph")
	if err != nil {
		return nil, fmt.Errorf("buildPackage: %w", err)
	}

	var phases []op.ExecutableUnit

	for _, phaseName := range lorepackage.PhaseOrder(lorepackage.Deploy) {

		actions := release.PhaseActions(targetPlatform, lorepackage.Deploy, phaseName)
		if len(actions) == 0 {
			continue
		}

		var phaseRetry *op.RetryPolicy

		for _, action := range actions {
			switch typed := action.(type) {
			case *lorepackage.ScriptAction:
				retry, err := executeScriptAction(sharedEnv, release, typed, cfg)
				if err != nil {
					return nil, fmt.Errorf("phase %q: %w", phaseName, err)
				}
				if retry != nil && phaseRetry == nil {
					phaseRetry = retry
				}
			case *lorepackage.NativePMAction:
				if err := p.addNativeSoftwarePackages(provider, typed); err != nil {
					return nil, fmt.Errorf("phase %q: %w", phaseName, err)
				}
			default:
				return nil, fmt.Errorf("phase %q: unknown action type %T", phaseName, action)
			}
		}

		children := parentlessTargets(provider)
		if len(children) == 0 {
			continue
		}

		// lore owns the package output: it names the phase subgraph and stamps its provenance annotations.
		spec := op.NewSubgraphSpec().
			WithID(fmt.Sprintf("subgraph.%s.%s", release.Name, phaseName)).
			WithName(phaseName).
			WithAction(subgraphAction).
			WithAnnotations(map[string]any{"package": release.Name, "phase": phaseName}).
			WithChildren(children...).
			WithRetryPolicy(phaseRetry)

		subgraph, err := op.NewSubgraph(spec)
		if err != nil {
			return nil, fmt.Errorf("phase %q: %w", phaseName, err)
		}

		phases = append(phases, subgraph)
	}

	return phases, nil
}

// addNativeSoftwarePackages registers a native-software-management invocation (install / remove / upgrade) into the
// shared provider via [plan.Provider.Plan]. The concrete package manager is selected at execution time from the
// runtime environment's [platform.Platform]; the planned node is platform-neutral.
//
// Parameters:
//   - `provider`: the shared plan provider the invocation registers into.
//   - `action`: the native package-manager action (command, packages, phase).
//
// Returns:
//   - `error`: non-nil if the action name is unknown or the provider rejects the call.
func (p *Planner) addNativeSoftwarePackages(provider *plan.Provider, action *lorepackage.NativePMAction) error {

	name := "pkg.install"
	switch action.Command {
	case lorepackage.PMRemove:
		name = "pkg.remove"
	case lorepackage.PMUpgrade:
		name = "pkg.upgrade"
	}

	packages := make([]any, len(action.Packages))
	for i, name := range action.Packages {
		packages[i] = name
	}

	_, err := provider.Plan(name, nil, map[string]any{
		"packages": packages,
	})
	if err != nil {
		return fmt.Errorf("addNativeSoftwarePackages: %w", err)
	}

	return nil
}

// endregion

// endregion

// region HELPER FUNCTIONS

// sharedProvider returns the runtime environment's cached [plan.Provider] — the single invocation ledger that lore's
// Go path and the phase scripts both register into.
//
// Parameters:
//   - `sharedEnv`: the shared runtime environment.
//
// Returns:
//   - `*plan.Provider`: the cached plan provider.
//   - `error`: non-nil if the "plan" module is unavailable or not a plan provider.
func sharedProvider(sharedEnv *op.RuntimeEnvironment) (*plan.Provider, error) {

	resolved, err := sharedEnv.ProviderByType(reflect.TypeFor[plan.Provider]())
	if err != nil {
		return nil, fmt.Errorf("lore.Build: resolving plan provider: %w", err)
	}

	provider, ok := resolved.(*plan.Provider)
	if !ok {
		return nil, fmt.Errorf("lore.Build: plan provider is %T, not *plan.Provider", resolved)
	}

	return provider, nil
}

// parentlessTargets returns the Targets of every registered invocation whose Target has no parent — i.e. the units
// not yet grouped into a phase subgraph.
//
// Parameters:
//   - `provider`: the shared plan provider whose ledger is swept.
//
// Returns:
//   - `[]op.ExecutableUnit`: the parentless Targets, in registration order.
func parentlessTargets(provider *plan.Provider) []op.ExecutableUnit {

	var units []op.ExecutableUnit
	for _, invocation := range provider.InvocationRegistry().All() {
		if invocation.Target.ParentID() == "" {
			units = append(units, invocation.Target)
		}
	}

	return units
}

// executeScriptAction runs a Starlark phase script's phase-named entry point (e.g. `install(package, phase)`) on a
// runtime over the shared environment, returning the retry policy the script configured via `phase.retry()`.
//
// The runtime is built with the lifecycle verbs (`plan.assemble` / `run` / `save` / `load` / `clear`) denied, so the
// script may only contribute invocations into the shared ledger — never orchestrate.
//
// Parameters:
//   - `sharedEnv`: the shared runtime environment whose cached plan provider the script registers into.
//   - `release`: the resolved package release.
//   - `action`: the script action (path, phase name).
//   - `cfg`: the per-package build configuration.
//
// Returns:
//   - `*op.RetryPolicy`: the retry policy the script configured, or nil.
//   - `error`: non-nil if reading, executing, or calling the script fails.
func executeScriptAction(sharedEnv *op.RuntimeEnvironment, release *lorepackage.Release, action *lorepackage.ScriptAction, cfg BuildConfig) (*op.RetryPolicy, error) {

	thread, globals, packageContext := prepareScriptEnv(sharedEnv, release, action, cfg)

	source, err := os.ReadFile(action.Path)
	if err != nil {
		return nil, fmt.Errorf("reading script %s: %w", action.Path, err)
	}

	options := &syntax.FileOptions{Set: true, While: true, TopLevelControl: true, GlobalReassign: true, Recursion: true}

	scriptGlobals, err := starlark.ExecFileOptions(options, thread, action.Path, source, globals)
	if err != nil {
		return nil, fmt.Errorf("executing script: %w", err)
	}

	entry, ok := scriptGlobals[action.PhaseName]
	if !ok {
		return nil, fmt.Errorf("function %q not found in script %s", action.PhaseName, action.Path)
	}

	callable, ok := entry.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("%q is not callable in script %s", action.PhaseName, action.Path)
	}

	phaseContext := &PhaseContext{PhaseName: action.PhaseName, Action: "deploy"}

	args := starlark.Tuple{packageContext.ToStarlark(), phaseContext.ToStarlark()}
	if _, err := starlark.Call(thread, callable, args, nil); err != nil {
		return nil, fmt.Errorf("calling %s(): %w", action.PhaseName, err)
	}

	return phaseContext.Retry, nil
}

// prepareScriptEnv builds the Starlark thread, globals, and package context for a phase script.
//
// The globals come from a [starlarkbridge.Runtime] over the SHARED environment with the lifecycle verbs denied, so
// `plan.*` calls in the script register into the same ledger lore's Go path uses.
//
// Parameters:
//   - `sharedEnv`: the shared runtime environment.
//   - `release`: the resolved package release.
//   - `action`: the script action being prepared.
//   - `cfg`: the per-package build configuration.
//
// Returns:
//   - `*starlark.Thread`: the configured thread.
//   - `starlark.StringDict`: the predeclared globals (lifecycle verbs denied).
//   - `*PackageContext`: the package context passed to the script's entry point.
func prepareScriptEnv(sharedEnv *op.RuntimeEnvironment, release *lorepackage.Release, action *lorepackage.ScriptAction, cfg BuildConfig) (*starlark.Thread, starlark.StringDict, *PackageContext) {

	runtime := starlarkbridge.NewRuntime(sharedEnv, starlarkbridge.DenyAttributes("plan", lifecycleVerbs...))

	lifecycle := release.Lifecycle()

	packageContext := &PackageContext{
		Name:       release.Name,
		Version:    release.Version,
		Features:   lifecycle.EnabledFeatures(cfg.Features),
		Settings:   lifecycle.ResolvedSettings(cfg.Settings),
		DryRun:     cfg.DryRun,
		SourceRoot: release.Dir,
		TargetRoot: userHomeDir(),
	}

	thread := &starlark.Thread{
		Name:  action.PhaseName,
		Print: func(_ *starlark.Thread, msg string) { fmt.Printf("  [print] %s\n", msg) },
	}

	return thread, runtime.Predeclared(), packageContext
}

// detectPlatform classifies the host into a lore registry platform token ("Darwin", "Linux.Debian", …).
//
// Returns:
//   - `string`: the registry platform token; "Linux" when detection fails or the host is unclassified.
func detectPlatform() string {

	spec, err := platform.Detect()
	if err != nil {
		return "Linux"
	}

	host, err := platform.New(spec)
	if err != nil {
		return "Linux"
	}

	switch host.OS() {
	case "darwin":
		return "Darwin"
	case "windows":
		return "Windows"
	case "linux":
		switch strings.ToLower(host.Distro()) {
		case "debian", "ubuntu":
			return "Linux.Debian"
		case "fedora", "rhel", "centos", "rocky", "alma":
			return "Linux.Fedora"
		default:
			return "Linux"
		}
	default:
		return "Linux"
	}
}

// userHomeDir returns the user's home directory, falling back to $HOME.
//
// Returns:
//   - `string`: the home directory path.
func userHomeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return os.Getenv("HOME")
}

// endregion