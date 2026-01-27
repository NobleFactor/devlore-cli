// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package pipeline provides the lore four-phase execution pipeline.
// Phases: prepare → install → provision → verify
//
// Phase scripts are discovered from the directory structure:
//
//	<package>/<platform>/<operation>/<phase>.star
//
// Platform resolution order (most specific first):
//   - Linux.Debian, Linux.Fedora (distro-specific)
//   - Linux, Darwin, Windows (OS-level)
//   - Unix (Darwin + Linux + BSD)
//   - Common (all platforms)
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Operation represents a pipeline operation type.
type Operation string

const (
	OpDeploy       Operation = "Deploy"
	OpUpgrade      Operation = "Upgrade"
	OpDecommission Operation = "Decommission"
)

// Lifecycle represents a lore package's lifecycle manifest.
// This is loaded from lifecycle.yaml in the package directory.
// Phase scripts are discovered from the directory structure, not from YAML.
type Lifecycle struct {
	Name        string             `yaml:"name"`
	Version     string             `yaml:"version"`
	Description string             `yaml:"description"`
	Homepage    string             `yaml:"homepage,omitempty"`
	Repository  string             `yaml:"repository,omitempty"`
	License     string             `yaml:"license,omitempty"`
	Maintainer  string             `yaml:"maintainer,omitempty"`
	Aliases     []string           `yaml:"aliases,omitempty"`
	Platforms   []string           `yaml:"platforms"`
	Provides    []string           `yaml:"provides,omitempty"`
	Conflicts   []string           `yaml:"conflicts,omitempty"`
	Features    map[string]Feature `yaml:"features,omitempty"`
	Settings    map[string]Setting `yaml:"settings,omitempty"`
	Tags        []string           `yaml:"tags,omitempty"`
	Notes       string             `yaml:"notes,omitempty"`

	// Verification defines how to verify the installation.
	Verification struct {
		Command string `yaml:"command,omitempty"`
		Pattern string `yaml:"pattern,omitempty"`
	} `yaml:"verification,omitempty"`

	// HardwareProvisions defines hardware-specific configuration requirements.
	HardwareProvisions map[string]HardwareProvision `yaml:"hardware_provisions,omitempty"`

	// PackageDir is the directory containing this lifecycle (set by loader).
	PackageDir string `yaml:"-"`
}

// HardwareProvision defines hardware-specific configuration.
type HardwareProvision struct {
	Description string `yaml:"description"`
	Reference   string `yaml:"reference,omitempty"`
	BootArg     string `yaml:"boot_arg,omitempty"`
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
	Values      []string `yaml:"values,omitempty"`
}

// DeployPhaseOrder is the standard order of deploy pipeline phases.
var DeployPhaseOrder = []string{"prepare", "install", "provision", "verify"}

// UpgradePhaseOrder is the order for upgrade operations.
var UpgradePhaseOrder = []string{"prepare", "install", "verify"}

// DecommissionPhaseOrder is the order for decommission operations.
var DecommissionPhaseOrder = []string{"unprovision", "uninstall", "cleanup"}

// PhaseOrder returns the phase order for an operation.
func PhaseOrder(op Operation) []string {
	switch op {
	case OpDeploy:
		return DeployPhaseOrder
	case OpUpgrade:
		return UpgradePhaseOrder
	case OpDecommission:
		return DecommissionPhaseOrder
	default:
		return DeployPhaseOrder
	}
}

// PlatformResolutionOrder returns the order of platform directories for
// phase script chaining, from most general to most specific.
//
// Scripts are executed in this order, allowing specific platforms to
// build upon general setup. Each script that exists is executed.
//
// Examples:
//   - "Linux.Debian" → ["Common", "Unix", "Linux", "Linux.Debian"]
//   - "Darwin" → ["Common", "Unix", "Darwin"]
//   - "Windows" → ["Common", "Windows"]
func PlatformResolutionOrder(platform string) []string {
	var order []string

	// Common is always first (base setup for all platforms)
	order = append(order, "Common")

	// Unix covers Darwin, Linux, and BSD
	if platform == "Darwin" || strings.HasPrefix(platform, "Linux") {
		order = append(order, "Unix")
	}

	// OS-level platform
	if strings.HasPrefix(platform, "Linux.") {
		// For distro-qualified Linux, add base Linux before the distro
		order = append(order, "Linux")
	}

	// The specific platform last (most specific)
	order = append(order, platform)

	return order
}

// LoadLifecycle loads a lifecycle manifest from a package directory.
func LoadLifecycle(packageDir string) (*Lifecycle, error) {
	path := filepath.Join(packageDir, "lifecycle.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading lifecycle.yaml: %w", err)
	}

	var lifecycle Lifecycle
	if err := yaml.Unmarshal(data, &lifecycle); err != nil {
		return nil, fmt.Errorf("parsing lifecycle.yaml: %w", err)
	}

	lifecycle.PackageDir = packageDir
	return &lifecycle, nil
}

// LoadLifecycleFromRegistry loads a lifecycle from a registry directory.
func LoadLifecycleFromRegistry(registryDir, packageName string) (*Lifecycle, error) {
	packageDir := filepath.Join(registryDir, packageName)
	return LoadLifecycle(packageDir)
}

// GetPhaseScript returns the path to a single phase script for the given
// platform and operation. This finds the MOST SPECIFIC script only.
// For chained execution, use DiscoverPhaseScripts instead.
//
// Returns empty string if no script exists for this phase.
func (l *Lifecycle) GetPhaseScript(platform string, op Operation, phase string) string {
	// Check platforms from most specific to least specific (reverse order)
	platforms := PlatformResolutionOrder(platform)
	for i := len(platforms) - 1; i >= 0; i-- {
		p := platforms[i]
		path := filepath.Join(l.PackageDir, p, string(op), phase+".star")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// DiscoverPhaseScripts returns all phase scripts for a phase, ordered from
// most general to most specific for chained execution.
//
// Example for platform="Linux.Debian", op=OpDeploy, phase="install":
//
//	["Common/Deploy/install.star", "Unix/Deploy/install.star",
//	 "Linux/Deploy/install.star", "Linux.Debian/Deploy/install.star"]
//
// Only scripts that exist are included.
func (l *Lifecycle) DiscoverPhaseScripts(platform string, op Operation, phase string) []string {
	var scripts []string
	for _, p := range PlatformResolutionOrder(platform) {
		path := filepath.Join(l.PackageDir, p, string(op), phase+".star")
		if _, err := os.Stat(path); err == nil {
			scripts = append(scripts, path)
		}
	}
	return scripts
}

// HasPhase returns true if at least one phase script exists for this phase
// on the given platform and operation.
func (l *Lifecycle) HasPhase(platform string, op Operation, phase string) bool {
	return len(l.DiscoverPhaseScripts(platform, op, phase)) > 0
}

// DiscoverAllPhases returns a map of phase name to script paths for all
// phases in an operation on the given platform.
func (l *Lifecycle) DiscoverAllPhases(platform string, op Operation) map[string][]string {
	phases := make(map[string][]string)
	for _, phase := range PhaseOrder(op) {
		scripts := l.DiscoverPhaseScripts(platform, op, phase)
		if len(scripts) > 0 {
			phases[phase] = scripts
		}
	}
	return phases
}

// EnabledFeatures returns the list of enabled features given explicit enables
// and the default settings.
func (l *Lifecycle) EnabledFeatures(explicit []string) []string {
	// Build a set of explicitly mentioned features (positive or negative)
	explicitSet := make(map[string]bool)
	for _, f := range explicit {
		if len(f) > 0 && f[0] == '-' {
			// Negative feature: -completions means disable
			explicitSet[f[1:]] = false
		} else {
			explicitSet[f] = true
		}
	}

	// Start with explicit enables
	var result []string
	for _, f := range explicit {
		if len(f) > 0 && f[0] != '-' {
			result = append(result, f)
		}
	}

	// Add defaults that weren't explicitly disabled
	for name, feat := range l.Features {
		if feat.Default {
			if enabled, mentioned := explicitSet[name]; !mentioned {
				// Default is on and not mentioned: enable
				result = append(result, name)
			} else if !enabled {
				// Explicitly disabled: don't add
				continue
			}
		}
	}

	return result
}

// ResolvedSettings returns settings with defaults filled in.
func (l *Lifecycle) ResolvedSettings(explicit map[string]string) map[string]string {
	result := make(map[string]string)

	// First, apply defaults
	for name, setting := range l.Settings {
		if setting.Default != "" {
			result[name] = setting.Default
		}
	}

	// Then, apply explicit overrides
	for k, v := range explicit {
		result[k] = v
	}

	return result
}

// SupportsPlatform returns true if the lifecycle supports the given platform.
func (l *Lifecycle) SupportsPlatform(platform string) bool {
	for _, p := range l.Platforms {
		if p == platform {
			return true
		}
	}
	return false
}
