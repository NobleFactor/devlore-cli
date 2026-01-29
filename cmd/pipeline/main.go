// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Command pipeline runs lore package phase scripts sequentially.
//
// Usage:
//
//	pipeline [flags] <package>
//
// Example:
//
//	pipeline -registry ./registry astro
//	pipeline -registry ./registry -features global-cli,completions astro
//	pipeline -registry ./registry -dry-run astro
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
	"gopkg.in/yaml.v3"

	loreStar "github.com/NobleFactor/devlore-cli/internal/starlark"
)

// Phase represents a pipeline phase.
type Phase struct {
	Name   string
	Script string
}

// Feature represents a package feature definition.
type Feature struct {
	Description string `yaml:"description"`
	Default     bool   `yaml:"default"`
}

// Setting represents a package setting definition.
type Setting struct {
	Description string   `yaml:"description"`
	Type        string   `yaml:"type"`
	Default     string   `yaml:"default"`
	Values      []string `yaml:"values"`
}

// Lifecycle represents a package's lifecycle manifest.
type Lifecycle struct {
	Name         string             `yaml:"name"`
	Version      string             `yaml:"version"`
	Description  string             `yaml:"description"`
	Platforms    []string           `yaml:"platforms"`
	Features     map[string]Feature `yaml:"features"`
	Settings     map[string]Setting `yaml:"settings"`
	Phases       map[string]string  `yaml:"phases"`
	Verification struct {
		Command string `yaml:"command"`
		Pattern string `yaml:"pattern"`
	} `yaml:"verification"`
}

var (
	registryDir  = flag.String("registry", "./registry", "Registry directory containing package manifests")
	featuresFlag = flag.String("features", "", "Comma-separated list of features to enable")
	settingsFlag = flag.String("settings", "", "Comma-separated key=value settings")
	dryRun       = flag.Bool("dry-run", false, "Show what would happen without executing")
	verbose      = flag.Bool("verbose", false, "Enable verbose output")
	phaseFlag    = flag.String("phase", "", "Run only this phase (prepare, install, provision, verify)")
)

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Usage: pipeline [flags] <package>")
		fmt.Fprintln(os.Stderr, "\nFlags:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nExample:")
		fmt.Fprintln(os.Stderr, "  pipeline -registry ./registry astro")
		fmt.Fprintln(os.Stderr, "  pipeline -registry ./registry -features global-cli,completions astro")
		os.Exit(1)
	}

	packageName := flag.Arg(0)

	// Parse features
	features := []string{}
	if *featuresFlag != "" {
		features = strings.Split(*featuresFlag, ",")
		for i := range features {
			features[i] = strings.TrimSpace(features[i])
		}
	}

	// Parse settings
	settings := make(map[string]string)
	if *settingsFlag != "" {
		for _, pair := range strings.Split(*settingsFlag, ",") {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) == 2 {
				settings[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}

	// Load lifecycle manifest
	lifecycle, err := loadLifecycle(*registryDir, packageName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading package %q: %v\n", packageName, err)
		os.Exit(1)
	}

	// Add default features
	for name, feat := range lifecycle.Features {
		if feat.Default {
			// Check if not explicitly disabled
			found := false
			for _, f := range features {
				if f == name || f == "-"+name {
					found = true
					break
				}
			}
			if !found {
				features = append(features, name)
			}
		}
	}

	// Add default settings
	for name, setting := range lifecycle.Settings {
		if _, ok := settings[name]; !ok {
			if setting.Default != "" {
				settings[name] = setting.Default
			}
		}
	}

	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  Lore Pipeline: %s\n", lifecycle.Name)
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("Package:  %s v%s\n", lifecycle.Name, lifecycle.Version)
	fmt.Printf("Features: %v\n", features)
	fmt.Printf("Settings: %v\n", settings)
	fmt.Println()

	if *dryRun {
		fmt.Println("[DRY RUN] Would execute the following phases:")
		for _, phaseName := range []string{"prepare", "install", "provision", "verify"} {
			if script, ok := lifecycle.Phases[phaseName]; ok {
				fmt.Printf("  - %s: %s\n", phaseName, script)
			}
		}
		return
	}

	// Determine which phases to run
	phasesToRun := []string{"prepare", "install", "provision", "verify"}
	if *phaseFlag != "" {
		phasesToRun = []string{*phaseFlag}
	}

	// Run phases
	bindings := loreStar.NewBindings(features, settings, os.Stdout)

	for _, phaseName := range phasesToRun {
		scriptFile, ok := lifecycle.Phases[phaseName]
		if !ok {
			if *verbose {
				fmt.Printf("Skipping phase %q (not defined)\n", phaseName)
			}
			continue
		}

		fmt.Println("────────────────────────────────────────────────────────────────")
		fmt.Printf("Phase: %s\n", phaseName)
		fmt.Println("────────────────────────────────────────────────────────────────")

		scriptPath := filepath.Join(*registryDir, packageName, scriptFile)
		err := runPhase(scriptPath, phaseName, bindings)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n✗ Phase %q failed: %v\n", phaseName, err)
			os.Exit(1)
		}

		fmt.Printf("✓ Phase %q completed\n\n", phaseName)
	}

	fmt.Println("════════════════════════════════════════════════════════════════")
	fmt.Printf("✓ Pipeline completed successfully for %s\n", lifecycle.Name)
	fmt.Println("════════════════════════════════════════════════════════════════")
}

func loadLifecycle(registryDir, packageName string) (*Lifecycle, error) {
	path := filepath.Join(registryDir, packageName, "lifecycle.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading lifecycle.yaml: %w", err)
	}

	var lifecycle Lifecycle
	if err := yaml.Unmarshal(data, &lifecycle); err != nil {
		return nil, fmt.Errorf("parsing lifecycle.yaml: %w", err)
	}

	return &lifecycle, nil
}

func runPhase(scriptPath, phaseName string, bindings *loreStar.Bindings) error {
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("reading script: %w", err)
	}

	thread := &starlark.Thread{
		Name: phaseName,
		Print: func(_ *starlark.Thread, msg string) {
			fmt.Printf("  [print] %s\n", msg)
		},
	}

	// Execute the script
	globals, err := starlark.ExecFile(thread, scriptPath, data, bindings.Globals())
	if err != nil {
		return fmt.Errorf("executing script: %w", err)
	}

	// Call the phase function (prepare, install, provision, or verify)
	fn, ok := globals[phaseName]
	if !ok {
		return fmt.Errorf("function %q not found in script", phaseName)
	}

	callable, ok := fn.(starlark.Callable)
	if !ok {
		return fmt.Errorf("%q is not callable", phaseName)
	}

	_, err = starlark.Call(thread, callable, nil, nil)
	if err != nil {
		return fmt.Errorf("calling %s(): %w", phaseName, err)
	}

	return nil
}
