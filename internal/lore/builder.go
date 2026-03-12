// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lore

import (
	"fmt"
	"os"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
	"github.com/NobleFactor/devlore-cli/internal/manifest"
	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/platform"

	// Blank import triggers init() in all provider packages,
	// which call op.Announce() to self-register.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider"
)

// BuildResult contains the built execution graph and metadata for packages.
type BuildResult struct {
	// Graph is the execution graph ready for the execution.
	Graph *op.Graph

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
	ActionRegistry *op.ActionRegistry
}

// Planner encapsulates package resolution for adding installation nodes
// and phases to an execution graph. Used by both lore.Build() and writ deploy.
type Planner struct {
	Platform       string
	ActionRegistry *op.ActionRegistry
	RegistryClient *lorepackage.Registry
	Features       []string
	Settings       map[string]string
	DryRun         bool
}

// PlanPackages parses a packages-manifest file and adds installation nodes
// to the graph. Returns the resolved package names.
func (p *Planner) PlanPackages(graph *op.Graph, manifestPath string) ([]string, error) {
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	targetPlatform, reg, regClient, err := p.resolve()
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range m.Packages {
		features := mergeFeatures(entry.With, p.Features)

		pkg, err := regClient.Resolve(entry.Name, targetPlatform)
		if err != nil {
			return nil, fmt.Errorf("resolving package %q: %w", entry.Name, err)
		}

		cfg := BuildConfig{
			Features: features,
			Settings: p.Settings,
			DryRun:   p.DryRun,
		}
		if err := buildPackageNodes(graph, pkg, targetPlatform, cfg, reg); err != nil {
			return nil, fmt.Errorf("building nodes for %q: %w", entry.Name, err)
		}

		names = append(names, entry.Name)
	}

	return names, nil
}

// PlanByName resolves explicit package names and adds installation nodes
// to the graph. Returns the resolved package names.
func (p *Planner) PlanByName(graph *op.Graph, packages []string) ([]string, error) {
	targetPlatform, reg, regClient, err := p.resolve()
	if err != nil {
		return nil, err
	}

	cfg := BuildConfig{
		Features: p.Features,
		Settings: p.Settings,
		DryRun:   p.DryRun,
	}

	var names []string
	for _, pkgName := range packages {
		pkg, err := regClient.Resolve(pkgName, targetPlatform)
		if err != nil {
			return nil, fmt.Errorf("resolving package %q: %w", pkgName, err)
		}

		if err := buildPackageNodes(graph, pkg, targetPlatform, cfg, reg); err != nil {
			return nil, fmt.Errorf("building nodes for %q: %w", pkgName, err)
		}

		names = append(names, pkgName)
	}

	return names, nil
}

// resolve returns the resolved platform, action registry, and registry client,
// auto-creating any that are nil on the Planner.
func (p *Planner) resolve() (resolvedPlatform string, resolvedReg *op.ActionRegistry, resolvedRegistry *lorepackage.Registry, err error) {
	targetPlatform := p.Platform
	if targetPlatform == "" {
		targetPlatform = detectPlatform()
	}

	reg := p.ActionRegistry
	if reg == nil {
		reg = op.NewActionRegistry()
		op.InitAll(reg, op.Context{})
	}

	regClient := p.RegistryClient
	if regClient == nil {
		var err error
		regClient, err = lorepackage.NewRegistry()
		if err != nil {
			return "", nil, nil, fmt.Errorf("creating registry client: %w", err)
		}
	}

	return targetPlatform, reg, regClient, nil
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

	targetPlatform := cfg.Platform
	if targetPlatform == "" {
		targetPlatform = detectPlatform()
	}

	p := &Planner{
		Platform:       targetPlatform,
		ActionRegistry: cfg.ActionRegistry,
		RegistryClient: cfg.RegistryClient,
		Features:       cfg.Features,
		Settings:       cfg.Settings,
		DryRun:         cfg.DryRun,
	}

	graph := op.NewGraph("lore")

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

	graph.Context = op.GraphContext{
		Scope:          strings.Join(packages, "+"),
		Packages:       packages,
		TargetPlatform: targetPlatform,
		Features:       cfg.Features,
		Settings:       cfg.Settings,
	}

	return &BuildResult{
		Graph:    graph,
		Packages: packages,
		Platform: targetPlatform,
	}, nil
}

// BuildFromManifest creates an execution graph from a packages-manifest.yaml file.
func BuildFromManifest(manifestPath, targetPlatform string) (*BuildResult, error) {
	return Build(BuildConfig{
		ManifestPath: manifestPath,
		Platform:     targetPlatform,
	})
}

// BuildFromPackages creates an execution graph from a list of package names.
func BuildFromPackages(packages []string, targetPlatform string) (*BuildResult, error) {
	return Build(BuildConfig{
		Packages: packages,
		Platform: targetPlatform,
	})
}

// buildPackageNodes adds execution nodes and phases for a package to the graph.
// Each lifecycle phase becomes a Phase entry in the graph.
// Compensation is handled by Action Do/Undo on the recovery stack — no Starlark-level compensation.
func buildPackageNodes(graph *op.Graph, pkg *lorepackage.Release, targetPlatform string, cfg BuildConfig, reg *op.ActionRegistry) error { //nolint:gocognit
	action := lorepackage.Deploy
	phases := lorepackage.PhaseOrder(action)

	for _, phaseName := range phases {
		actions := pkg.PhaseActions(targetPlatform, action, phaseName)
		if len(actions) == 0 {
			continue
		}

		phaseID := fmt.Sprintf("phase.%s.%s", pkg.Name, phaseName)
		phase := &op.Phase{
			ID:     phaseID,
			Name:   phaseName,
			Status: op.PhasePending,
		}

		// Snapshot current node count to track which nodes this phase adds.
		nodesBefore := len(graph.Nodes)

		for _, action := range actions {
			switch a := action.(type) {
			case *lorepackage.ScriptAction:
				retryPolicy, err := executeScriptAction(graph, pkg, targetPlatform, a, cfg, reg)
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

// executeScriptAction runs a Starlark phase script's entry point function (named for the lifecycle phase, e.g.,
// "install", "provision") and returns the retry policy if one was configured via phase.retry().
func executeScriptAction(graph *op.Graph, pkg *lorepackage.Release, _ string, action *lorepackage.ScriptAction, cfg BuildConfig, reg *op.ActionRegistry) (*op.RetryPolicy, error) {
	thread, globals, pkgContext, err := prepareScriptEnv(graph, pkg, action, cfg, reg)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(action.Path)
	if err != nil {
		return nil, fmt.Errorf("reading script %s: %w", action.Path, err)
	}

	scriptGlobals, err := starlark.ExecFileOptions(&syntax.FileOptions{
		Set:             true,
		While:           true,
		TopLevelControl: true,
		GlobalReassign:  true,
		Recursion:       true,
	}, thread, action.Path, data, globals)
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

// prepareScriptEnv creates the Starlark thread and globals needed to execute a phase script.
// plan and ui are injected as globals via Runtime.
func prepareScriptEnv(
	graph *op.Graph,
	pkg *lorepackage.Release,
	action *lorepackage.ScriptAction,
	cfg BuildConfig,
	reg *op.ActionRegistry,
) (
	*starlark.Thread,
	starlark.StringDict,
	*loreStar.PackageContext,
	error, //nolint:unparam // error return reserved for future use
) {
	rt := loreStar.NewRuntime(op.BindingConfig{
		Writer:      os.Stdout,
		ProgramName: "lore",
		Color:       true,
		Receivers:   []string{"ui", "plan"},
	})

	globals := rt.BuildGlobals(graph, pkg.Name, reg)

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
		TargetRoot: userHomeDir(),
	}

	thread := &starlark.Thread{
		Name: action.PhaseName,
		Print: func(_ *starlark.Thread, msg string) {
			fmt.Printf("  [print] %s\n", msg)
		},
	}

	rt.ConfigureThread(thread, graph, pkg.Name, reg)
	return thread, globals, pkgContext, nil
}

// addNativePMNodes adds nodes for a native package manager action.
// Uses namespaced action names (pkg.install, pkg.upgrade, pkg.remove) that work on all platforms.
// The actual package manager is determined at execution time via op.Context.Platform.
func addNativePMNodes(graph *op.Graph, pkg *lorepackage.Release, action *lorepackage.NativePMAction, reg *op.ActionRegistry) error { //nolint:unparam // error return reserved for future use
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
	node := &op.Node{
		ID:      fmt.Sprintf("%s-%s-%s", actionName, pkg.Name, action.PhaseName),
		Action:  reg.MustGet(actionName),
		Project: pkg.Name,
	}
	node.SetSlotImmediate("packages", strings.Join(action.Packages, ","))
	node.SetSlotImmediate("phase", action.PhaseName)

	graph.Nodes = append(graph.Nodes, node)
	return nil
}

// userHomeDir returns the user's home directory.
func userHomeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return os.Getenv("HOME")
}

// detectPlatform converts platform info to registry platform string.
func detectPlatform() string {
	p := platform.New()
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
