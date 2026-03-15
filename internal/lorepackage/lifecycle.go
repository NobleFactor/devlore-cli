// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lorepackage

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/document"
)

// Action represents a lifecycle action type.
type Action string

// Lifecycle action constants.
const (
	Deploy       Action = "Deploy"
	Upgrade      Action = "Upgrade"
	Decommission Action = "Decommission"
	Reconcile    Action = "Reconcile"
)

// Signatures maps package managers to the names this package is known by.
// Keys are package manager names (brew, apt, dnf, pacman, winget, choco,
// cargo, pip, npm, go). Special key "urls" contains regex patterns for
// detecting URL-based installations (curl|bash, wget, etc.).
//
// Example:
//
//	signatures:
//	  brew: [ripgrep, rg]
//	  apt: [ripgrep]
//	  cargo: [ripgrep]
//	  urls: ['github\.com/BurntSushi/ripgrep']
type Signatures map[string][]string

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
	Signatures  Signatures         `yaml:"signatures,omitempty"`
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

	// synthetic is true for packages from native PMs (not lifecycle.yaml)
	synthetic bool
}

// Feature represents a package feature definition.
type Feature struct {
	Description string   `yaml:"description"`
	Default     bool     `yaml:"default"`
	Platforms   []string `yaml:"platforms,omitempty"`
}

// Setting represents a package setting definition.
type Setting struct {
	Description string   `yaml:"description"`
	Type        string   `yaml:"type"`
	Default     string   `yaml:"default"`
	Values      []string `yaml:"values,omitempty"`
	Platforms   []string `yaml:"platforms,omitempty"`
}

// HardwareProvision defines hardware-specific configuration.
type HardwareProvision struct {
	Description string `yaml:"description"`
	Reference   string `yaml:"reference,omitempty"`
	BootArg     string `yaml:"boot_arg,omitempty"`
}

// DeployPhaseOrder is the standard order of deploy pipeline phases.
var DeployPhaseOrder = []string{"prepare", "install", "provision", "verify"}

// UpgradePhaseOrder is the order for upgrade actions.
// The "migrate" phase handles version-specific migrations (config format changes,
// data migrations, etc.) that may be needed between versions.
var UpgradePhaseOrder = []string{"prepare", "upgrade", "migrate", "verify"}

// DecommissionPhaseOrder is the order for decommission actions.
var DecommissionPhaseOrder = []string{"unprovision", "uninstall", "cleanup"}

// ReconcilePhaseOrder is the order for reconcile actions.
// Scan discovers drift, repair corrects it, verify confirms the system is good.
var ReconcilePhaseOrder = []string{"scan", "repair", "verify"}

// RequiredPhase returns the required phase for an action.
// Each action has exactly one required phase that must be implemented.
// Native PM packages implement only this phase; lore packages may add others.
//
//   - Deploy requires "install"
//   - Upgrade requires "upgrade"
//   - Decommission requires "uninstall"
//   - Reconcile requires "repair"
func RequiredPhase(action Action) string {
	switch action {
	case Deploy:
		return "install"
	case Upgrade:
		return "upgrade"
	case Decommission:
		return "uninstall"
	case Reconcile:
		return "repair"
	default:
		return "install"
	}
}

// PhaseOrder returns the phase order for an action.
func PhaseOrder(action Action) []string {
	switch action {
	case Deploy:
		return DeployPhaseOrder
	case Upgrade:
		return UpgradePhaseOrder
	case Decommission:
		return DecommissionPhaseOrder
	case Reconcile:
		return ReconcilePhaseOrder
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

	var lifecycle Lifecycle
	if err := document.Read(path, &lifecycle); err != nil {
		return nil, err
	}

	return &lifecycle, nil
}

// DiscoverPhaseScripts returns all phase scripts for a phase, ordered from
// most general to most specific for chained execution.
//
// Example for platform="Linux.Debian", action=Deploy, phase="install":
//
//	["Common/Deploy/install.star", "Unix/Deploy/install.star",
//	 "Linux/Deploy/install.star", "Linux.Debian/Deploy/install.star"]
//
// Only scripts that exist are included.
func (l *Lifecycle) DiscoverPhaseScripts(packageDir, platform string, action Action, phase string) []string {
	if l.synthetic {
		return nil // Synthetic lifecycles have no scripts
	}

	var scripts []string
	for _, p := range PlatformResolutionOrder(platform) {
		path := filepath.Join(packageDir, p, string(action), phase+".star")
		if _, err := os.Stat(path); err == nil {
			scripts = append(scripts, path)
		}
	}
	return scripts
}

// GetPhaseScript returns the path to a single phase script for the given
// platform and action. This finds the MOST SPECIFIC script only.
// For chained execution, use DiscoverPhaseScripts instead.
//
// Returns empty string if no script exists for this phase.
func (l *Lifecycle) GetPhaseScript(packageDir, platform string, action Action, phase string) string {
	if l.synthetic {
		return ""
	}

	// Check platforms from most specific to least specific (reverse order)
	platforms := PlatformResolutionOrder(platform)
	for i := len(platforms) - 1; i >= 0; i-- {
		p := platforms[i]
		path := filepath.Join(packageDir, p, string(action), phase+".star")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// HasPhase returns true if at least one phase script exists for this phase
// on the given platform and action.
func (l *Lifecycle) HasPhase(packageDir, platform string, action Action, phase string) bool {
	return len(l.DiscoverPhaseScripts(packageDir, platform, action, phase)) > 0
}

// DiscoverAllPhases returns a map of phase name to script paths for all
// phases in an action on the given platform.
func (l *Lifecycle) DiscoverAllPhases(packageDir, platform string, action Action) map[string][]string {
	phases := make(map[string][]string)
	for _, phase := range PhaseOrder(action) {
		scripts := l.DiscoverPhaseScripts(packageDir, platform, action, phase)
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
		if f != "" && f[0] == '-' {
			// Negative feature: -completions means disable
			explicitSet[f[1:]] = false
		} else {
			explicitSet[f] = true
		}
	}

	// Start with explicit enables
	var result []string
	for _, f := range explicit {
		if f != "" && f[0] != '-' {
			result = append(result, f)
		}
	}

	// Add defaults that weren't explicitly disabled
	for name, feat := range l.Features {
		if feat.Default {
			if _, mentioned := explicitSet[name]; !mentioned {
				// Default is on and not mentioned: enable
				result = append(result, name)
			}
			// If explicitly mentioned (enabled or disabled), the loop above already handled it
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
		// Check for distro match (e.g., "Linux" matches "Linux.Debian")
		if strings.HasPrefix(platform, p+".") {
			return true
		}
	}
	return false
}

// IsSynthetic returns true if this lifecycle was synthesized for a native PM package.
func (l *Lifecycle) IsSynthetic() bool {
	return l.synthetic
}
