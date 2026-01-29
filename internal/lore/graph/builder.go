// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package graph provides the Package Graph Builder for lore.
//
// # Architecture
//
// The Package Graph Builder produces execution graph nodes for software package
// operations. It shares the execution engine with writ's File Tree Builder:
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                    Execution Engine                         │
//	│                  (internal/engine)                          │
//	├─────────────────────────────────────────────────────────────┤
//	│                                                             │
//	│  ┌─────────────────┐          ┌─────────────────┐          │
//	│  │  File Tree      │          │  Package Graph  │          │
//	│  │  Builder        │          │  Builder        │          │
//	│  │  (writ/tree)    │          │  (lore/graph)   │          │
//	│  └────────┬────────┘          └────────┬────────┘          │
//	│           │                            │                    │
//	│           │    ┌──────────────────┐    │                    │
//	│           └───►│ Execution Graph  │◄───┘                    │
//	│                │ (engine.Graph)   │                         │
//	│                └────────┬─────────┘                         │
//	│                         │                                   │
//	│                         ▼                                   │
//	│                ┌──────────────────┐                         │
//	│                │   Engine.Run()   │                         │
//	│                └──────────────────┘                         │
//	│                                                             │
//	└─────────────────────────────────────────────────────────────┘
//
// # Usage
//
// When writ encounters a packages-manifest.yaml file, it calls BuildFromManifest
// to produce package nodes that are merged into the execution graph:
//
//	// In writ deploy:
//	fileResult, _ := tree.Build(fileCfg)
//
//	// For each packages-manifest.yaml found:
//	pkgResult, _ := graph.BuildFromManifest(manifestPath, platform)
//
//	// Merge into single graph:
//	combinedGraph := engine.MergeGraphs(fileResult.Graph, pkgResult.Graph)
//
//	// Run unified graph:
//	results, _ := eng.Run(ctx, combinedGraph)
//
// When lore is invoked directly, it uses the same builder:
//
//	// In lore deploy:
//	pkgResult, _ := graph.BuildFromPackages(packageNames, platform)
//	results, _ := eng.Run(ctx, pkgResult.Graph)
package graph

import (
	"fmt"
	"os"
	"strings"

	"go.starlark.net/starlark"
	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/engine"
	"github.com/NobleFactor/devlore-cli/internal/host"
	"github.com/NobleFactor/devlore-cli/internal/registry"
	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
	"github.com/NobleFactor/devlore-cli/internal/starlark/platform"
)

// BuildResult contains the built execution graph and metadata for packages.
type BuildResult struct {
	// Graph is the execution graph ready for the engine.
	Graph *engine.Graph

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

	// RegistryClient provides access to the package registry.
	// If nil, a default client is created.
	RegistryClient *registry.Client
}

// packagesManifest represents the structure of packages-manifest.yaml.
type packagesManifest struct {
	Packages []packageEntry `yaml:"packages"`
}

// packageEntry represents a package in the manifest.
type packageEntry struct {
	Name     string   `yaml:"name"`
	Version  string   `yaml:"version,omitempty"`
	Features []string `yaml:"features,omitempty"`
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

	// Resolve platform
	plat := cfg.Platform
	if plat == "" {
		plat = detectPlatform()
	}

	// Initialize registry client if not provided
	regClient := cfg.RegistryClient
	if regClient == nil {
		var err error
		regClient, err = registry.NewDefault()
		if err != nil {
			return nil, fmt.Errorf("creating registry client: %w", err)
		}
	}

	// Create the execution graph
	graph := &engine.Graph{}

	// Resolve packages
	var packages []string
	if cfg.ManifestPath != "" {
		// Parse manifest
		manifest, err := parseManifest(cfg.ManifestPath)
		if err != nil {
			return nil, fmt.Errorf("parsing manifest: %w", err)
		}
		for _, entry := range manifest.Packages {
			packages = append(packages, entry.Name)
		}
	} else {
		packages = cfg.Packages
	}

	// Create host for bindings
	h := host.NewHost()

	// Process each package
	for _, pkgName := range packages {
		// Resolve package from registry
		pkg, err := regClient.Resolve(pkgName, plat)
		if err != nil {
			return nil, fmt.Errorf("resolving package %q: %w", pkgName, err)
		}

		// Build graph nodes for this package
		if err := buildPackageNodes(graph, pkg, h, plat, cfg); err != nil {
			return nil, fmt.Errorf("building nodes for %q: %w", pkgName, err)
		}
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

// parseManifest reads and parses a packages-manifest.yaml file.
func parseManifest(path string) (*packagesManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest packagesManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// buildPackageNodes adds execution nodes for a package to the graph.
func buildPackageNodes(graph *engine.Graph, pkg *registry.LorePackage, h host.Host, plat string, cfg BuildConfig) error {
	// Get phase actions for the install phase
	op := registry.OpDeploy
	phases := registry.PhaseOrder(op)

	for _, phase := range phases {
		actions := pkg.PhaseActions(plat, op, phase)
		for _, action := range actions {
			if err := buildActionNodes(graph, pkg, h, plat, action, cfg); err != nil {
				return fmt.Errorf("phase %q: %w", phase, err)
			}
		}
	}

	return nil
}

// buildActionNodes adds nodes for a single phase action.
func buildActionNodes(graph *engine.Graph, pkg *registry.LorePackage, h host.Host, plat string, action registry.PhaseAction, cfg BuildConfig) error {
	switch a := action.(type) {
	case *registry.ScriptAction:
		return executeScriptAction(graph, pkg, h, plat, a, cfg)
	case *registry.NativePMAction:
		return addNativePMNodes(graph, pkg, a)
	default:
		return fmt.Errorf("unknown action type: %T", action)
	}
}

// executeScriptAction runs a Starlark script to populate the graph.
func executeScriptAction(graph *engine.Graph, pkg *registry.LorePackage, h host.Host, plat string, action *registry.ScriptAction, cfg BuildConfig) error {
	// Read the script
	data, err := os.ReadFile(action.Path)
	if err != nil {
		return fmt.Errorf("reading script %s: %w", action.Path, err)
	}

	// Create bindings
	systemBindings := loreStar.NewSystemBindings(h)
	planBindings := platform.NewPlanBindings(graph, h, pkg.Name)

	// Create package context
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

	// Create Starlark thread
	thread := &starlark.Thread{
		Name: action.PhaseName,
		Print: func(_ *starlark.Thread, msg string) {
			fmt.Printf("  [print] %s\n", msg)
		},
	}

	// Build globals with the three bindings
	globals := starlark.StringDict{
		"system":  systemBindings.ToStarlark(),
		"package": pkgContext.ToStarlark(),
		"plan":    planBindings.ToStarlark(),
	}

	// Execute the script
	scriptGlobals, err := starlark.ExecFile(thread, action.Path, data, globals)
	if err != nil {
		return fmt.Errorf("executing script: %w", err)
	}

	// Call the phase function with arguments (package, system, plan)
	fn, ok := scriptGlobals[action.PhaseName]
	if !ok {
		return fmt.Errorf("function %q not found in script", action.PhaseName)
	}

	callable, ok := fn.(starlark.Callable)
	if !ok {
		return fmt.Errorf("%q is not callable", action.PhaseName)
	}

	// Call with three arguments: package, system, plan
	args := starlark.Tuple{
		pkgContext.ToStarlark(),
		systemBindings.ToStarlark(),
		planBindings.ToStarlark(),
	}
	_, err = starlark.Call(thread, callable, args, nil)
	if err != nil {
		return fmt.Errorf("calling %s(): %w", action.PhaseName, err)
	}

	return nil
}

// addNativePMNodes adds nodes for a native package manager operation.
// Uses namespaced operation names (package-install, package-upgrade, package-remove) that work on all platforms.
// The actual package manager is determined at execution time by host.PackageManager().
func addNativePMNodes(graph *engine.Graph, pkg *registry.LorePackage, action *registry.NativePMAction) error {
	// Determine the namespaced operation name
	var opName string
	switch action.Operation {
	case registry.PMInstall:
		opName = "package-install"
	case registry.PMRemove:
		opName = "package-remove"
	case registry.PMUpgrade:
		opName = "package-upgrade"
	default:
		opName = "package-install"
	}

	// Create the node with namespaced operation
	node := &engine.Node{
		ID:         fmt.Sprintf("%s-%s-%s", opName, pkg.Name, action.PhaseName),
		Operations: []string{opName},
		Project:    pkg.Name,
		Metadata: map[string]string{
			"packages": strings.Join(action.Packages, ","),
			"phase":    action.PhaseName,
		},
	}

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
