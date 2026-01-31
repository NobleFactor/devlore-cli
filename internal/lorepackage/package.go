// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lorepackage

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/host"
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
)

// Release provides a uniform view over any package release, whether from
// the lore registry or a native package manager. Use Lifecycle() to
// get phase and feature metadata.
type Release struct {
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
func (rel *Release) Lifecycle() *Lifecycle {
	if rel.lifecycle != nil {
		return rel.lifecycle
	}

	if rel.Source == SourceLore && rel.Dir != "" {
		// Load from lifecycle.yaml
		lc, err := LoadLifecycle(rel.Dir)
		if err == nil {
			rel.lifecycle = lc
			return rel.lifecycle
		}
	}

	// Synthetic lifecycle for native PM packages
	rel.lifecycle = &Lifecycle{
		Name:        rel.Name,
		Version:     rel.Version,
		Description: rel.Description,
		Platforms:   []string{"Darwin", "Linux", "Windows"},
		synthetic:   true,
	}
	return rel.lifecycle
}

// DiscoverPhaseScripts returns all phase scripts for a phase, ordered from
// most general to most specific for chained execution.
//
// For native PM packages, returns empty (the engine handles install directly).
func (rel *Release) DiscoverPhaseScripts(platform string, op Operation, phase string) []string {
	if rel.Source != SourceLore || rel.Dir == "" {
		return nil // Native PM packages don't have scripts
	}
	return rel.Lifecycle().DiscoverPhaseScripts(rel.Dir, platform, op, phase)
}

// HasPhase returns true if at least one phase script exists for this phase.
func (rel *Release) HasPhase(platform string, op Operation, phase string) bool {
	return len(rel.DiscoverPhaseScripts(platform, op, phase)) > 0
}

// PhaseActions returns the executable actions for a phase.
// This provides a uniform interface for both lore and native PM packages.
//
// For lore packages: returns ScriptAction items for each discovered script.
// For native PM packages: returns a NativePMAction for install/uninstall phases.
func (rel *Release) PhaseActions(platform string, op Operation, phase string) []PhaseAction {
	if rel.Source == SourceLore && rel.Dir != "" {
		// Lore package: return script actions
		scripts := rel.DiscoverPhaseScripts(platform, op, phase)
		actions := make([]PhaseAction, 0, len(scripts))
		for _, script := range scripts {
			actions = append(actions, &ScriptAction{
				Path:      script,
				PhaseName: phase,
				Platform:  platformFromPath(script, rel.Dir),
			})
		}
		return actions
	}

	// Native PM package: return native PM action for relevant phases
	pmOp, ok := phaseToNativePMOp(op, phase)
	if !ok {
		return nil // Phase not applicable for native PM
	}

	pkgName := rel.NativeName
	if pkgName == "" {
		pkgName = rel.Name
	}

	return []PhaseAction{
		&NativePMAction{
			Manager:   rel.Source,
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
func (rel *Release) IsNative() bool {
	return rel.Source != SourceLore
}

// IsSynthetic returns true if the lifecycle is synthetic (not from lifecycle.yaml).
func (rel *Release) IsSynthetic() bool {
	return rel.Lifecycle().synthetic
}

// Resolve looks up a package by name in the registry.
// It checks the lore registry first, then falls back to native package managers.
func (r *Registry) Resolve(name string, platform string) (*Release, error) {
	// First, check lore registry
	pkgDir := filepath.Join(r.cacheDir, "packages", name)
	if dirExists(pkgDir) {
		lc, err := LoadLifecycle(pkgDir)
		if err != nil {
			return nil, err
		}
		return &Release{
			Name:        lc.Name,
			Version:     lc.Version,
			Description: lc.Description,
			Source:      SourceLore,
			Dir:         pkgDir,
			lifecycle:   lc,
		}, nil
	}

	// Fall back to native package manager based on platform
	return r.resolveNative(name, platform)
}

// resolveNative creates a synthetic Release for a native PM package.
// It uses the synthetic cache to avoid repeated lookups and store verification results.
func (r *Registry) resolveNative(name string, platform string) (*Release, error) {
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

	// Check synthetic cache first
	cache := NewSyntheticCache(r.cacheDir)
	if cached := cache.Get(source, name); cached != nil {
		return &Release{
			Name:        cached.Name,
			Version:     cached.Version,
			Description: cached.Description,
			Source:      cached.Source,
			NativeName:  cached.NativeName,
		}, nil
	}

	// Create new synthetic package
	pkg := &Release{
		Name:       name,
		Version:    "latest",
		Source:     source,
		NativeName: name, // Same name; could be mapped differently
	}

	// Cache the synthetic package (unverified initially)
	info := &SyntheticPackageInfo{
		Name:       name,
		Source:     source,
		NativeName: name,
		Version:    "latest",
		Verified:   false,
	}
	_ = cache.Put(info) // Ignore cache errors; they're non-fatal

	return pkg, nil
}

// VerifySyntheticPackage checks if a synthetic package is available and updates the cache.
func (r *Registry) VerifySyntheticPackage(pkg *Release) bool {
	if pkg.Source == SourceLore {
		return true // Lore packages are always "verified"
	}

	cache := NewSyntheticCache(r.cacheDir)

	// Check if already verified in cache
	if cached := cache.Get(pkg.Source, pkg.Name); cached != nil && cached.Verified {
		return true
	}

	// Verify with the package manager
	h := host.NewHost()
	pm := h.PackageManager()
	if pm == nil {
		return false
	}

	available := pm.Available(pkg.Name)

	// Update cache with verification result
	info := &SyntheticPackageInfo{
		Name:       pkg.Name,
		Source:     pkg.Source,
		NativeName: pkg.NativeName,
		Version:    pkg.Version,
		Verified:   available,
	}
	_ = cache.Put(info)

	return available
}

// SyntheticCache returns the synthetic package cache for this registry.
func (r *Registry) SyntheticCache() *SyntheticCache {
	return NewSyntheticCache(r.cacheDir)
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ParsePackagePrefix extracts the package manager prefix from a package name.
// On Darwin, packages can be prefixed to explicitly select the package manager:
//   - brew: — Homebrew formula (CLI tools)
//   - cask: — Homebrew Cask (GUI applications)
//   - port: — MacPorts
//
// Without a prefix, auto-detection is used (port if installed, else brew).
//
// Examples:
//
//	"brew:wget"        → ("wget", "brew")
//	"cask:iterm2"      → ("iterm2", "cask")
//	"port:wget"        → ("wget", "port")
//	"wget"             → ("wget", "")
//
// Returns (packageName, prefix) where prefix is "brew", "cask", "port", or "" for auto-detect.
func ParsePackagePrefix(name string) (string, string) {
	if strings.HasPrefix(name, "brew:") {
		return strings.TrimPrefix(name, "brew:"), "brew"
	}
	if strings.HasPrefix(name, "cask:") {
		return strings.TrimPrefix(name, "cask:"), "cask"
	}
	if strings.HasPrefix(name, "port:") {
		return strings.TrimPrefix(name, "port:"), "port"
	}
	return name, ""
}
