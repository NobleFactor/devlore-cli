// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lore

import (
	"fmt"
	"os"
	"strings"

	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/ui"
	"github.com/NobleFactor/devlore-cli/internal/host"
	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/internal/manifest"
	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
)

// BuildResult contains the built execution graph and metadata for packages.
type BuildResult struct {
	// Graph is the execution graph ready for the execution.
	Graph *execution.Graph

	// Packages lists the resolved package names.
	Packages []string

	// Platform is the detected or specified platform.
	Platform string
}

// BuildConfig holds configuration for building a package graph.
type BuildConfig struct {
	// ManifestPath is the path to a packages-manifest.yaml file.
	// Mutually exclusive with Packages.
	ManifestPath string

	// Packages is a list of package names to install.
	// Mutually exclusive with ManifestPath.
	Packages []string

	// Platform is the target platform (e.g., "Darwin", "Linux.Debian").
	// If empty, auto-detected.
	Platform string

	// Features are optional feature flags to enable.
	Features []string

	// Settings are key-value configuration settings.
	Settings map[string]string

	// DryRun prevents actual installation when true.
	DryRun bool

	// RegistryClient provides access to the package lorepackage.
	// If nil, a default client is created.
	RegistryClient *lorepackage.Registry

	// ActionRegistry provides access to execution actions.
	// Must be set before calling Build.
	ActionRegistry *execution.ActionRegistry
}

// Planner encapsulates package resolution for adding installation nodes
// and phases to an execution graph. Used by both lore.Build() and writ deploy.
type Planner struct {
	Platform       string
	ActionRegistry *execution.ActionRegistry
	RegistryClient *lorepackage.Registry
	Features       []string
	Settings       map[string]string
	DryRun         bool
}

// PlanPackages parses a packages-manifest file and adds installation nodes
// to the graph. Returns the resolved package names.
func (p *Planner) PlanPackages(graph *execution.Graph, manifestPath string) ([]string, error) {
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	plat, reg, regClient, err := p.resolve()
	if err != nil {
		return nil, err
	}

	h := host.NewHost()

	var names []string
	for _, entry := range m.Packages {
		features := mergeFeatures(entry.With, p.Features)

		pkg, err := regClient.Resolve(entry.Name, plat)
		if err != nil {
			return nil, fmt.Errorf("resolving package %q: %w", entry.Name, err)
		}

		cfg := BuildConfig{
			Features: features,
			Settings: p.Settings,
			DryRun:   p.DryRun,
		}
		if err := buildPackageNodes(graph, pkg, h, plat, cfg, reg); err != nil {
			return nil, fmt.Errorf("building nodes for %q: %w", entry.Name, err)
		}

		names = append(names, entry.Name)
	}

	return names, nil
}

// PlanByName resolves explicit package names and adds installation nodes
// to the graph. Returns the resolved package names.
func (p *Planner) PlanByName(graph *execution.Graph, packages []string) ([]string, error) {
	plat, reg, regClient, err := p.resolve()
	if err != nil {
		return nil, err
	}

	h := host.NewHost()

	cfg := BuildConfig{
		Features: p.Features,
		Settings: p.Settings,
		DryRun:   p.DryRun,
	}

	var names []string
	for _, pkgName := range packages {
		pkg, err := regClient.Resolve(pkgName, plat)
		if err != nil {
			return nil, fmt.Errorf("resolving package %q: %w", pkgName, err)
		}

		if err := buildPackageNodes(graph, pkg, h, plat, cfg, reg); err != nil {
			return nil, fmt.Errorf("building nodes for %q: %w", pkgName, err)
		}

		names = append(names, pkgName)
	}

	return names, nil
}

// resolve returns the resolved platform, action registry, and registry client,
// auto-creating any that are nil on the Planner.
func (p *Planner) resolve() (string, *execution.ActionRegistry, *lorepackage.Registry, error) {
	plat := p.Platform
	if plat == "" {
		plat = detectPlatform()
	}

	reg := p.ActionRegistry
	if reg == nil {
		reg = execution.NewActionRegistry()
		provider.RegisterAll(reg)
	}

	regClient := p.RegistryClient
	if regClient == nil {
		var err error
		regClient, err = lorepackage.NewRegistry()
		if err != nil {
			return "", nil, nil, fmt.Errorf("creating registry client: %w", err)
		}
	}

	return plat, reg, regClient, nil
}

// Build creates an execution graph from the given configuration.
func Build(cfg BuildConfig) (*BuildResult, error) {
	// Validate configuration
	if cfg.ManifestPath != "" && len(cfg.Packages) > 0 {
		return nil, fmt.Errorf("cannot specify both ManifestPath and Packages")
	}
	if cfg.ManifestPath == "" && len(cfg.Packages) == 0 {
		return nil, fmt.Errorf("must specify either ManifestPath or Packages")
	}

	plat := cfg.Platform
	if plat == "" {
		plat = detectPlatform()
	}

	p := &Planner{
		Platform:       plat,
		ActionRegistry: cfg.ActionRegistry,
		RegistryClient: cfg.RegistryClient,
		Features:       cfg.Features,
		Settings:       cfg.Settings,
		DryRun:         cfg.DryRun,
	}

	graph := &execution.Graph{}

	var packages []string
	var err error
	if cfg.ManifestPath != "" {
		packages, err = p.PlanPackages(graph, cfg.ManifestPath)
	} else {
		packages, err = p.PlanByName(graph, cfg.Packages)
	}
	if err != nil {
		return nil, err
	}

	return &BuildResult{
		Graph:    graph,
		Packages: packages,
		Platform: plat,
	}, nil
}

// BuildFromManifest creates an execution graph from a packages-manifest.yaml file.
func BuildFromManifest(manifestPath, plat string) (*BuildResult, error) {
	return Build(BuildConfig{
		ManifestPath: manifestPath,
		Platform:     plat,
	})
}

// BuildFromPackages creates an execution graph from a list of package names.
func BuildFromPackages(packages []string, plat string) (*BuildResult, error) {
	return Build(BuildConfig{
		Packages: packages,
		Platform: plat,
	})
}

// buildPackageNodes adds execution nodes and phases for a package to the graph.
// Each lifecycle phase becomes a Phase entry in the graph. Compensation is
// handled by Action Do/Undo on the recovery stack — no Starlark-level compensation.
func buildPackageNodes(graph *execution.Graph, pkg *lorepackage.Release, h host.Host, plat string, cfg BuildConfig, reg *execution.ActionRegistry) error {
	action := lorepackage.Deploy
	phases := lorepackage.PhaseOrder(action)

	for _, phaseName := range phases {
		actions := pkg.PhaseActions(plat, action, phaseName)
		if len(actions) == 0 {
			continue
		}

		phaseID := fmt.Sprintf("phase.%s.%s", pkg.Name, phaseName)
		phase := &execution.Phase{
			ID:     phaseID,
			Name:   phaseName,
			Status: execution.PhasePending,
		}

		// Snapshot current node count to track which nodes this phase adds.
		nodesBefore := len(graph.Nodes)

		for _, action := range actions {
			switch a := action.(type) {
			case *lorepackage.ScriptAction:
				retryPolicy, err := executeScriptAction(graph, pkg, h, plat, a, cfg, reg)
				if err != nil {
					return fmt.Errorf("phase %q: %w", phaseName, err)
				}
				if retryPolicy != nil && phase.Retry == nil {
					phase.Retry = retryPolicy
				}
			case *lorepackage.NativePMAction:
				if err := addNativePMNodes(graph, pkg, a, reg); err != nil {
					return fmt.Errorf("phase %q: %w", phaseName, err)
				}
			default:
				return fmt.Errorf("unknown action type: %T", action)
			}
		}

		// Collect forward node IDs.
		for i := nodesBefore; i < len(graph.Nodes); i++ {
			phase.NodeIDs = append(phase.NodeIDs, graph.Nodes[i].ID)
		}

		graph.Phases = append(graph.Phases, phase)
	}

	return nil
}

// executeScriptAction runs a Starlark phase script's entry point function
// (named for the lifecycle phase, e.g., "install", "provision") and returns
// the retry policy if one was configured via phase.retry().
func executeScriptAction(graph *execution.Graph, pkg *lorepackage.Release, h host.Host, plat string, action *lorepackage.ScriptAction, cfg BuildConfig, reg *execution.ActionRegistry) (*execution.RetryPolicy, error) {
	thread, globals, pkgContext, _, err := prepareScriptEnv(graph, pkg, h, action, cfg, reg)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(action.Path)
	if err != nil {
		return nil, fmt.Errorf("reading script %s: %w", action.Path, err)
	}

	scriptGlobals, err := starlark.ExecFile(thread, action.Path, data, globals)
	if err != nil {
		return nil, fmt.Errorf("executing script: %w", err)
	}

	// Look for a phase-named entry point (e.g., "install", "provision").
	entryName := action.PhaseName
	fn, ok := scriptGlobals[entryName]
	if !ok {
		return nil, fmt.Errorf("function %q not found in script %s", entryName, action.Path)
	}

	callable, ok := fn.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("%q is not callable in script %s", entryName, action.Path)
	}

	// Create phase context with name and action.
	phaseCtx := &loreStar.PhaseContext{
		PhaseName: action.PhaseName,
		Action:    "deploy",
	}

	// Call: install(package, phase) — plan is a global, not an argument.
	args := starlark.Tuple{
		pkgContext.ToStarlark(),
		phaseCtx.ToStarlark(),
	}
	_, err = starlark.Call(thread, callable, args, nil)
	if err != nil {
		return nil, fmt.Errorf("calling %s(): %w", entryName, err)
	}

	return phaseCtx.Retry, nil
}

// prepareScriptEnv creates the Starlark thread and globals needed to execute
// a phase script. plan and ui are injected as globals.
func prepareScriptEnv(graph *execution.Graph, pkg *lorepackage.Release, h host.Host, action *lorepackage.ScriptAction, cfg BuildConfig, reg *execution.ActionRegistry) (
	*starlark.Thread, starlark.StringDict, *loreStar.PackageContext, *loreStar.PlanRoot, error,
) {
	planBindings := loreStar.NewPlanRoot(graph, h, pkg.Name, reg)

	lifecycle := pkg.Lifecycle()
	features := lifecycle.EnabledFeatures(cfg.Features)
	settings := lifecycle.ResolvedSettings(cfg.Settings)

	pkgContext := &loreStar.PackageContext{
		Name:       pkg.Name,
		Version:    pkg.Version,
		Features:   features,
		Settings:   settings,
		DryRun:     cfg.DryRun,
		SourceRoot: pkg.Dir,
		TargetRoot: h.HomeDir(),
	}

	thread := &starlark.Thread{
		Name: action.PhaseName,
		Print: func(_ *starlark.Thread, msg string) {
			fmt.Printf("  [print] %s\n", msg)
		},
	}

	globals := starlark.StringDict{
		"plan": planBindings,
		"ui": loreStar.NewUiReceiver(&ui.Provider{
			Writer:      os.Stdout,
			ProgramName: "lore",
			Color:       true,
		}),
	}

	return thread, globals, pkgContext, planBindings, nil
}

// addNativePMNodes adds nodes for a native package manager action.
// Uses namespaced action names (pkg.install, pkg.upgrade, pkg.remove) that work on all platforms.
// The actual package manager is determined at execution time by host.PackageManager().
func addNativePMNodes(graph *execution.Graph, pkg *lorepackage.Release, action *lorepackage.NativePMAction, reg *execution.ActionRegistry) error {
	// Determine the dotted action name
	var actionName string
	switch action.Command {
	case lorepackage.PMInstall:
		actionName = "pkg.install"
	case lorepackage.PMRemove:
		actionName = "pkg.remove"
	case lorepackage.PMUpgrade:
		actionName = "pkg.upgrade"
	default:
		actionName = "pkg.install"
	}

	// Create the node with resolved action
	node := &execution.Node{
		ID:      fmt.Sprintf("%s-%s-%s", actionName, pkg.Name, action.PhaseName),
		Action:  reg.MustGet(actionName),
		Project: pkg.Name,
	}
	node.SetSlotImmediate("packages", strings.Join(action.Packages, ","))
	node.SetSlotImmediate("phase", action.PhaseName)

	graph.Nodes = append(graph.Nodes, node)
	return nil
}

// detectPlatform converts host.Platform to registry platform string.
func detectPlatform() string {
	p := host.DetectPlatform()
	switch p.OS {
	case "darwin":
		return "Darwin"
	case "windows":
		return "Windows"
	case "linux":
		switch strings.ToLower(p.Distro) {
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
