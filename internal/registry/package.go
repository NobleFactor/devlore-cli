// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package registry

import (
	"os"
	"path/filepath"
	"strings"
)

// PackageSource indicates where a package was resolved from.
type PackageSource string

const (
	SourceLore   PackageSource = "lore"   // Lore registry (full lifecycle)
	SourceApt    PackageSource = "apt"    // Debian/Ubuntu apt
	SourceDnf    PackageSource = "dnf"    // Fedora/RHEL dnf
	SourceBrew   PackageSource = "brew"   // macOS Homebrew
	SourcePort   PackageSource = "port"   // macOS MacPorts
	SourceWinget PackageSource = "winget" // Windows winget
	SourceChoco  PackageSource = "choco"  // Windows Chocolatey
)

// LorePackage provides a uniform view over any package, whether from
// the lore registry or a native package manager. Use Lifecycle() to
// get phase and feature metadata.
type LorePackage struct {
	Name        string        // Package name
	Version     string        // Version (may be "latest" for native PMs)
	Description string        // One-line description
	Source      PackageSource // Where this package was resolved from
	Dir         string        // Package directory (lore packages only)

	// Native package manager name (for non-lore packages)
	// e.g., "docker.io" for apt, "docker" for brew
	NativeName string

	// Cached lifecycle (loaded lazily)
	lifecycle *Lifecycle
}

// Lifecycle returns the package's lifecycle metadata.
// For lore packages, this loads from lifecycle.yaml.
// For native PM packages, this returns a synthetic lifecycle.
func (p *LorePackage) Lifecycle() *Lifecycle {
	if p.lifecycle != nil {
		return p.lifecycle
	}

	if p.Source == SourceLore && p.Dir != "" {
		// Load from lifecycle.yaml
		lc, err := LoadLifecycle(p.Dir)
		if err == nil {
			p.lifecycle = lc
			return p.lifecycle
		}
	}

	// Synthetic lifecycle for native PM packages
	p.lifecycle = &Lifecycle{
		Name:        p.Name,
		Version:     p.Version,
		Description: p.Description,
		Platforms:   []string{"Darwin", "Linux", "Windows"},
		synthetic:   true,
	}
	return p.lifecycle
}

// DiscoverPhaseScripts returns all phase scripts for a phase, ordered from
// most general to most specific for chained execution.
//
// For native PM packages, returns empty (the engine handles install directly).
func (p *LorePackage) DiscoverPhaseScripts(platform string, op Operation, phase string) []string {
	if p.Source != SourceLore || p.Dir == "" {
		return nil // Native PM packages don't have scripts
	}
	return p.Lifecycle().DiscoverPhaseScripts(p.Dir, platform, op, phase)
}

// HasPhase returns true if at least one phase script exists for this phase.
func (p *LorePackage) HasPhase(platform string, op Operation, phase string) bool {
	return len(p.DiscoverPhaseScripts(platform, op, phase)) > 0
}

// PhaseActions returns the executable actions for a phase.
// This provides a uniform interface for both lore and native PM packages.
//
// For lore packages: returns ScriptAction items for each discovered script.
// For native PM packages: returns a NativePMAction for install/uninstall phases.
func (p *LorePackage) PhaseActions(platform string, op Operation, phase string) []PhaseAction {
	if p.Source == SourceLore && p.Dir != "" {
		// Lore package: return script actions
		scripts := p.DiscoverPhaseScripts(platform, op, phase)
		actions := make([]PhaseAction, 0, len(scripts))
		for _, script := range scripts {
			actions = append(actions, &ScriptAction{
				Path:      script,
				PhaseName: phase,
				Platform:  platformFromPath(script, p.Dir),
			})
		}
		return actions
	}

	// Native PM package: return native PM action for relevant phases
	pmOp, ok := phaseToNativePMOp(op, phase)
	if !ok {
		return nil // Phase not applicable for native PM
	}

	pkgName := p.NativeName
	if pkgName == "" {
		pkgName = p.Name
	}

	return []PhaseAction{
		&NativePMAction{
			Manager:   p.Source,
			Operation: pmOp,
			Packages:  []string{pkgName},
			PhaseName: phase,
		},
	}
}

// platformFromPath extracts the platform directory name from a script path.
func platformFromPath(scriptPath, packageDir string) string {
	rel, err := filepath.Rel(packageDir, scriptPath)
	if err != nil {
		return ""
	}
	// Path is like "Darwin/Deploy/install.star" - extract first component
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// phaseToNativePMOp maps operation+phase to native PM operation.
// Native PM packages only implement the required phase for each operation.
// Returns false if the phase is not the required phase for the operation.
func phaseToNativePMOp(op Operation, phase string) (PMOperation, bool) {
	// Only the required phase maps to a native PM operation
	if phase != RequiredPhase(op) {
		return 0, false
	}

	switch op {
	case OpDeploy:
		return PMInstall, true
	case OpUpgrade:
		return PMUpgrade, true
	case OpDecommission:
		return PMRemove, true
	default:
		return PMInstall, true
	}
}

// IsNative returns true if this package comes from a native package manager.
func (p *LorePackage) IsNative() bool {
	return p.Source != SourceLore
}

// IsSynthetic returns true if the lifecycle is synthetic (not from lifecycle.yaml).
func (p *LorePackage) IsSynthetic() bool {
	return p.Lifecycle().synthetic
}

// Resolve looks up a package by name in the registry.
// It checks the lore registry first, then falls back to native package managers.
func (c *Client) Resolve(name string, platform string) (*LorePackage, error) {
	// First, check lore registry
	pkgDir := filepath.Join(c.cacheDir, "packages", name)
	if dirExists(pkgDir) {
		lc, err := LoadLifecycle(pkgDir)
		if err != nil {
			return nil, err
		}
		return &LorePackage{
			Name:        lc.Name,
			Version:     lc.Version,
			Description: lc.Description,
			Source:      SourceLore,
			Dir:         pkgDir,
			lifecycle:   lc,
		}, nil
	}

	// Fall back to native package manager based on platform
	return c.resolveNative(name, platform)
}

// resolveNative creates a synthetic LorePackage for a native PM package.
func (c *Client) resolveNative(name string, platform string) (*LorePackage, error) {
	var source PackageSource
	switch {
	case strings.HasPrefix(platform, "Linux.Debian") || platform == "Linux":
		source = SourceApt
	case strings.HasPrefix(platform, "Linux.Fedora"):
		source = SourceDnf
	case platform == "Darwin":
		source = SourceBrew
	case platform == "Windows":
		source = SourceWinget
	default:
		source = SourceApt // Default fallback
	}

	return &LorePackage{
		Name:       name,
		Version:    "latest",
		Source:     source,
		NativeName: name, // Same name; could be mapped differently
	}, nil
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
