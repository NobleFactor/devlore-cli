// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package lorepackage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/platform"
	"github.com/NobleFactor/devlore-cli/pkg/selector"
)

// PackageSource indicates where a package was resolved from.
type PackageSource string

// Package source constants for resolution.
const (
	SourceLore   PackageSource = "lore"   // Lore registry (full lifecycle)
	SourceApt    PackageSource = "apt"    // Debian/Ubuntu apt
	SourceDnf    PackageSource = "dnf"    // Fedora/RHEL dnf
	SourcePacman PackageSource = "pacman" // Arch/Manjaro pacman
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
//
// Parameters:
//   - `host`: the host the package is planned for.
//   - `action`: the lifecycle action.
//   - `phase`: the phase.
//
// Returns:
//   - `[]string`: the scripts, general to specific.
//   - `error`: a [*GrammarRefusal] when a platform directory's name breaks the selector grammar.
func (rel *Release) DiscoverPhaseScripts(host selector.Host, action Action, phase string) ([]string, error) {
	if rel.Source != SourceLore || rel.Dir == "" {
		return nil, nil // Native PM packages don't have scripts
	}
	return rel.Lifecycle().DiscoverPhaseScripts(rel.Dir, host, action, phase)
}

// PhaseActions returns the executable actions for a phase.
// This provides a uniform interface for both lore and native PM packages.
//
// For lore packages: returns ScriptAction items for each discovered script.
// For native PM packages: returns a NativePMAction for install/uninstall phases.
//
// Parameters:
//   - `host`: the host the package is planned for.
//   - `action`: the lifecycle action.
//   - `phase`: the phase.
//
// Returns:
//   - `[]PhaseAction`: the phase's actions.
//   - `error`: a [*GrammarRefusal] when a platform directory's name breaks the selector grammar.
func (rel *Release) PhaseActions(host selector.Host, action Action, phase string) ([]PhaseAction, error) {
	if rel.Source == SourceLore && rel.Dir != "" {
		// Lore package: return script actions
		scripts, err := rel.DiscoverPhaseScripts(host, action, phase)
		if err != nil {
			return nil, err
		}
		actions := make([]PhaseAction, 0, len(scripts))
		for _, script := range scripts {
			actions = append(actions, &ScriptAction{Path: script, PhaseName: phase})
		}
		return actions, nil
	}

	// Native PM package: return native PM action for relevant phases
	pmCmd, ok := phaseToNativePMCmd(action, phase)
	if !ok {
		return nil, nil // Phase not applicable for native PM
	}

	pkgName := rel.NativeName
	if pkgName == "" {
		pkgName = rel.Name
	}

	return []PhaseAction{
		&NativePMAction{
			Manager:   rel.Source,
			Command:   pmCmd,
			Packages:  []string{pkgName},
			PhaseName: phase,
		},
	}, nil
}

// phaseToNativePMCmd maps action+phase to native PM command.
// Native PM packages only implement the required phase for each action.
// Returns false if the phase is not the required phase for the action,
// or if the action has no native PM equivalent (e.g., Reconcile).
func phaseToNativePMCmd(action Action, phase string) (PMCommand, bool) {
	// Only the required phase maps to a native PM command
	if phase != RequiredPhase(action) {
		return 0, false
	}

	switch action {
	case Deploy:
		return PMInstall, true
	case Upgrade:
		return PMUpgrade, true
	case Decommission:
		return PMRemove, true
	default:
		return 0, false
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
// It checks the lore registry first, then falls back to this host's native package manager.
func (r *Registry) Resolve(name string) (*Release, error) {
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

	// Fall back to this host's native package manager
	return r.resolveNative(name)
}

// resolveNative creates a synthetic Release for a native PM package.
// It uses the synthetic cache to avoid repeated lookups and store verification results.
//
// The native source is this host's default package manager, as pkg/platform detects it (#944): a distribution it
// doesn't list takes its closest listed ancestor's, so Pop!_OS uses apt; Arch and Manjaro use pacman.
//
// Parameters:
//   - `name`: the package's name.
//
// Returns:
//   - `*Release`: the synthetic release.
//   - `error`: non-nil when this host's package manager can't be detected.
func (r *Registry) resolveNative(name string) (*Release, error) {
	source, err := nativeSource()
	if err != nil {
		return nil, err
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
	_ = cache.Put(info) //nolint:errcheck // cache errors are non-fatal

	return pkg, nil
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
func ParsePackagePrefix(name string) (packageName, prefix string) {
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

// nativeSource names this host's default native package manager.
//
// Returns:
//   - `PackageSource`: the source for the host's default manager's purl type: deb is apt, rpm dnf, alpm pacman.
//   - `error`: non-nil when pkg/platform can't detect this host, or its default manager has no lore source.
func nativeSource() (PackageSource, error) {

	spec, err := platform.Detect()
	if err != nil {
		return "", fmt.Errorf("native package manager: %w", err)
	}
	host, err := platform.New(spec)
	if err != nil {
		return "", fmt.Errorf("native package manager: %w", err)
	}

	purlType := host.DefaultPurlType()
	source, ok := sourceForPurlType(purlType)
	if !ok {
		return "", fmt.Errorf("native package manager: lore has no source for %q packages", purlType)
	}
	return source, nil
}

// sourceForPurlType names the source a native package manager's packages come from, by the purl type the manager
// reports.
//
// Parameters:
//   - `purlType`: the manager's purl type: deb, rpm, alpm, brew, port, winget.
//
// Returns:
//   - `PackageSource`: deb is apt, rpm dnf, alpm pacman; brew, port and winget are themselves.
//   - `bool`: false when lore has no source for the type, as for flatpak and snap.
func sourceForPurlType(purlType string) (PackageSource, bool) {

	switch purlType {
	case "deb":
		return SourceApt, true
	case "rpm":
		return SourceDnf, true
	case "alpm":
		return SourcePacman, true
	case "brew":
		return SourceBrew, true
	case "port":
		return SourcePort, true
	case "winget":
		return SourceWinget, true
	default:
		return "", false
	}
}
