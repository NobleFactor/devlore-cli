// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"fmt"
	"strings"
)

// Provider provides platform-independent package management.
// The package manager is resolved at runtime via host.PackageManager().
//
// Compensable Forward methods return (string, map[string]any, error):
// a summary of affected packages, the compensation receipt, and an error.
// The map is opaque to the executor, meaningful only to the corresponding
// Compensate* Backward method.
//
//devlore:plannable
type Provider struct {
	// Test hooks. Nil means use real host implementation.
	isInstalledFn func(pkg, manager string) bool
	getVersionFn  func(pkg, manager string) string
	installFn     func(packages []string, manager string, cask bool) error
	upgradeFn     func(packages []string, manager string, cask bool) error
	removeFn      func(packages []string, manager string, cask bool) error
}

// Install installs packages using the platform's package manager.
// Returns compensation state with pre-install status per package.
//
// Parameters:
//   - packages: List of package names to install
//   - manager: Package manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
func (p *Provider) Install(packages []string, manager string, cask bool) (string, map[string]any, error) {
	if len(packages) == 0 {
		return "", nil, fmt.Errorf("no packages specified")
	}

	// Query which packages are already installed before acting
	var alreadyInstalled []string
	for _, pkg := range packages {
		if p.isInstalled(pkg, manager) {
			alreadyInstalled = append(alreadyInstalled, pkg)
		}
	}

	if err := p.doInstall(packages, manager, cask); err != nil {
		return "", nil, err
	}

	return strings.Join(packages, ", "), map[string]any{
		"packages":          packages,
		"manager":           manager,
		"cask":              cask,
		"already_installed": alreadyInstalled,
	}, nil
}

// CompensateInstall undoes an Install by removing packages that weren't
// already installed before the action.
func (p *Provider) CompensateInstall(state any) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	packages, _ := s["packages"].([]string)
	alreadyInstalled, _ := s["already_installed"].([]string)
	manager, _ := s["manager"].(string)
	cask, _ := s["cask"].(bool)

	installed := make(map[string]bool)
	for _, pkg := range alreadyInstalled {
		installed[pkg] = true
	}

	var toRemove []string
	for _, pkg := range packages {
		if !installed[pkg] {
			toRemove = append(toRemove, pkg)
		}
	}

	if len(toRemove) == 0 {
		return nil
	}
	return p.doRemove(toRemove, manager, cask)
}

// Upgrade upgrades packages using the platform's package manager.
// Returns compensation state with pre-upgrade versions per package.
//
// Parameters:
//   - packages: List of package names to upgrade
//   - manager: Package manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
func (p *Provider) Upgrade(packages []string, manager string, cask bool) (string, map[string]any, error) {
	if len(packages) == 0 {
		return "", nil, fmt.Errorf("no packages specified")
	}

	// Capture current versions before upgrading
	previousVersions := make(map[string]string)
	for _, pkg := range packages {
		if v := p.getVersion(pkg, manager); v != "" {
			previousVersions[pkg] = v
		}
	}

	if err := p.doUpgrade(packages, manager, cask); err != nil {
		return "", nil, err
	}

	return strings.Join(packages, ", "), map[string]any{
		"packages":          packages,
		"manager":           manager,
		"cask":              cask,
		"previous_versions": previousVersions,
	}, nil
}

// CompensateUpgrade is a diagnostic no-op. Previous versions are captured
// in state for manual recovery, but automatic downgrade is not reliable
// across package managers.
func (p *Provider) CompensateUpgrade(_ any) error {
	return nil
}

// Remove removes packages using the platform's package manager.
// Returns compensation state for reinstallation.
//
// Parameters:
//   - packages: List of package names to remove
//   - manager: Package manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
func (p *Provider) Remove(packages []string, manager string, cask bool) (string, map[string]any, error) {
	if len(packages) == 0 {
		return "", nil, fmt.Errorf("no packages specified")
	}

	if err := p.doRemove(packages, manager, cask); err != nil {
		return "", nil, err
	}

	return strings.Join(packages, ", "), map[string]any{
		"packages": packages,
		"manager":  manager,
		"cask":     cask,
	}, nil
}

// CompensateRemove undoes a Remove by reinstalling the removed packages.
func (p *Provider) CompensateRemove(state any) error {
	s, _ := state.(map[string]any)
	if s == nil {
		return nil
	}
	packages, _ := s["packages"].([]string)
	manager, _ := s["manager"].(string)
	cask, _ := s["cask"].(bool)

	if len(packages) == 0 {
		return nil
	}
	return p.doInstall(packages, manager, cask)
}

// Update refreshes the package manager index.
//
// Parameters:
//   - manager: Package manager override (empty for auto-detect)
func (p *Provider) Update(manager string) (string, error) {
	pm := resolvePMForInstall(manager)
	if pm == nil {
		return "", fmt.Errorf("no package manager available")
	}

	result := pm.Update()
	if !result.OK {
		return "", fmt.Errorf("%s update failed: %s", pm.Name(), result.Stderr)
	}
	return pm.Name(), nil
}

// --- Predicates ---

// Installed returns true if the named package is installed.
//
// Parameters:
//   - name: Package name to check
func (p *Provider) Installed(name string) (bool, error) {
	return p.isInstalled(name, ""), nil
}

// NotInstalled returns true if the named package is not installed.
//
// Parameters:
//   - name: Package name to check
func (p *Provider) NotInstalled(name string) (bool, error) {
	return !p.isInstalled(name, ""), nil
}

// VersionGTE returns true if the installed version of name is >= version.
//
// Parameters:
//   - name: Package name to check
//   - version: Minimum version string to compare against
func (p *Provider) VersionGTE(name, version string) (bool, error) {
	current := p.getVersion(name, "")
	if current == "" {
		return false, nil
	}
	return current >= version, nil
}

// --- Internal helpers ---

func (p *Provider) isInstalled(pkg, manager string) bool {
	if p.isInstalledFn != nil {
		return p.isInstalledFn(pkg, manager)
	}
	pm := resolvePMForInstall(manager)
	if pm == nil {
		return false
	}
	return pm.Installed(pkg)
}

func (p *Provider) getVersion(pkg, manager string) string {
	if p.getVersionFn != nil {
		return p.getVersionFn(pkg, manager)
	}
	pm := resolvePMForInstall(manager)
	if pm == nil {
		return ""
	}
	return pm.Version(pkg)
}

func (p *Provider) doInstall(packages []string, manager string, cask bool) error {
	if p.installFn != nil {
		return p.installFn(packages, manager, cask)
	}

	pm := resolvePMForInstall(manager)
	if pm == nil {
		return fmt.Errorf("no package manager available")
	}

	if cask {
		result := runBrewCaskInstall(packages)
		if !result.OK {
			return fmt.Errorf("brew install --cask failed: %s", result.Stderr)
		}
		return nil
	}

	result := pm.Install(packages...)
	if !result.OK {
		return fmt.Errorf("%s install failed: %s", pm.Name(), result.Stderr)
	}
	return nil
}

func (p *Provider) doUpgrade(packages []string, manager string, cask bool) error {
	if p.upgradeFn != nil {
		return p.upgradeFn(packages, manager, cask)
	}

	pm := resolvePMForUpgrade(manager, packages)
	if pm == nil {
		return fmt.Errorf("no package manager available")
	}

	if cask {
		result := runBrewCaskUpgrade(packages)
		if !result.OK {
			return fmt.Errorf("brew upgrade --cask failed: %s", result.Stderr)
		}
		return nil
	}

	result := pm.Install(packages...)
	if !result.OK {
		return fmt.Errorf("%s upgrade failed: %s", pm.Name(), result.Stderr)
	}
	return nil
}

func (p *Provider) doRemove(packages []string, manager string, cask bool) error {
	if p.removeFn != nil {
		return p.removeFn(packages, manager, cask)
	}

	for _, pkg := range packages {
		pm, _ := resolvePMForRemove(manager, pkg)
		if pm == nil {
			return fmt.Errorf("no package manager available")
		}

		if cask {
			result := runBrewCaskRemove(pkg)
			if !result.OK {
				return fmt.Errorf("brew uninstall --cask %s failed: %s", pkg, result.Stderr)
			}
		} else {
			result := pm.Remove(pkg)
			if !result.OK {
				return fmt.Errorf("%s remove %s failed: %s", pm.Name(), pkg, result.Stderr)
			}
		}
	}
	return nil
}
