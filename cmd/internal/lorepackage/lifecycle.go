// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package lorepackage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/document"
	"github.com/NobleFactor/devlore-cli/pkg/selector"
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

// LifecycleVerbs are the plan.* orchestration attributes denied to phase-script runtimes. Scripts only
// contribute invocations into the shared registry; lore alone assembles, runs, and persists.
//
// It lives here, rather than in lore where it was, because two things read it: lore denies these attributes
// at run time through starlarkbridge.DenyAttributes, and lint.starlark subtracts them from the valid set it
// resolves a phase script's calls against (devlore-cli#721). One slice, so a verb cannot be denied at run
// time and still accepted by the linter.
//
// Two of the five name real attributes of the plan provider: Clear and Run, snake-cased to clear and run.
// The other three do not -- the provider's methods are AssembleDefinition, LoadDefinition and
// SaveDefinition -- so assemble, load and save deny nothing. That is recorded here because this slice reads
// as five effective denials and is two. Whether the denial should instead cover the three real names is a
// separate question from either of its readers, and is not decided here.
var LifecycleVerbs = []string{"assemble", "clear", "load", "run", "save"}

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

// LoadLifecycle loads a lifecycle manifest from a package directory.
//
// Parameters:
//   - packageDir: path to the package directory containing lifecycle.yaml
//
// Returns:
//   - *Lifecycle: parsed lifecycle manifest
//   - error: read or parse error
func LoadLifecycle(packageDir string) (*Lifecycle, error) {

	path := filepath.Join(packageDir, "lifecycle.yaml")

	return document.ReadFile[Lifecycle](path)
}

// PlatformDirs selects a package's platform directories for a host, in the order to apply them (#944).
//
// A package's directories are named by selectors alone, with no project: `Common`, the name with no selector words,
// then an OS word of the host's chain, its architecture, or both (`Debian.arm64`). Every directory is judged, and a name
// that breaks the grammar refuses the package, every such name listed, before anything is planned (ruled 2026-09-30).
//
// Parameters:
//   - `packageDir`: the package's directory.
//   - `host`: the host the package is planned for.
//
// Returns:
//   - `[]string`: the directories this host includes, general to specific; a later one's scripts build on an earlier
//     one's.
//   - `error`: a [*GrammarRefusal] naming every malformed directory name, or the error reading the package.
func PlatformDirs(packageDir string, host selector.Host) ([]string, error) {

	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			names = append(names, entry.Name())
		}
	}

	selected, grammarErrors := selector.Selector{Host: host, Base: "Common"}.Select(names)
	if len(grammarErrors) > 0 {
		return nil, &GrammarRefusal{PackageDir: packageDir, Errors: grammarErrors}
	}

	dirs := make([]string, 0, len(selected))
	for _, s := range selected {
		dirs = append(dirs, s.Name)
	}
	return dirs, nil
}

// DiscoverPhaseScripts returns all phase scripts for a phase, ordered from most general to most specific for chained
// execution.
//
// Example for an Ubuntu host, action=Deploy, phase="install":
//
//	["Common/Deploy/install.star", "Unix/Deploy/install.star", "Linux/Deploy/install.star",
//	 "Debian/Deploy/install.star", "Ubuntu/Deploy/install.star"]
//
// Only scripts that exist are included.
//
// Parameters:
//   - `packageDir`: the package's directory.
//   - `host`: the host the package is planned for.
//   - `action`: the lifecycle action.
//   - `phase`: the phase.
//
// Returns:
//   - `[]string`: the scripts, general to specific.
//   - `error`: a [*GrammarRefusal], or the error reading the package.
func (l *Lifecycle) DiscoverPhaseScripts(packageDir string, host selector.Host, action Action, phase string) ([]string,
	error) {

	if l.synthetic {
		return nil, nil // Synthetic lifecycles have no scripts
	}

	dirs, err := PlatformDirs(packageDir, host)
	if err != nil {
		return nil, err
	}

	var scripts []string
	for _, dir := range dirs {
		path := filepath.Join(packageDir, dir, string(action), phase+".star")
		if _, err := os.Stat(path); err == nil {
			scripts = append(scripts, path)
		}
	}
	return scripts, nil
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

// GrammarRefusal is every platform directory of a package whose name breaks the selector grammar. lore refuses to plan
// the package with it, before anything changes (#944, ruled 2026-09-30).
type GrammarRefusal struct {

	// PackageDir is the package's directory.
	PackageDir string

	// Errors are the names and the rules they break.
	Errors []*selector.GrammarError
}

// Error lists every name and the rule it breaks.
//
// Returns:
//   - `string`: the list, one name to a line.
func (r *GrammarRefusal) Error() string {

	var b strings.Builder
	fmt.Fprintf(&b, "%s: %d platform director(ies) break the selector grammar (docs/guides/selectors.md):",
		r.PackageDir, len(r.Errors))
	for _, e := range r.Errors {
		fmt.Fprintf(&b, "\n  %v", e)
	}
	return b.String()
}
