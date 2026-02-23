// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"fmt"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides platform-independent package management.
// All platform-specific behavior is delegated to the HostProvider
// injected via op.Context.Host.
//
// Compensable Forward methods return (string, map[string]any, error):
// a summary of affected packages, the compensation receipt, and an error.
// The map is opaque to the executor, meaningful only to the corresponding
// Compensate* Backward method.
type Provider struct{}

// Install installs packages using the platform's package manager.
// Returns compensation state with pre-install status per package.
//
// Parameters:
//   - packages: List of package names to install
//   - manager: Package manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
//
// +devlore:access=planned
func (p *Provider) Install(host op.HostProvider, packages []string, manager string, cask bool) (summary string, state map[string]any, retErr error) {
	if len(packages) == 0 {
		return "", nil, fmt.Errorf("no packages specified")
	}

	pm := resolvePMForInstall(host, manager)
	if pm == nil {
		return "", nil, fmt.Errorf("no package manager available")
	}

	// Query which packages are already installed before acting.
	var alreadyInstalled []string
	for _, pkg := range packages {
		if pm.Installed(pkg) {
			alreadyInstalled = append(alreadyInstalled, pkg)
		}
	}

	if cask {
		if err := runBrewCask("install", packages...); err != nil {
			return "", nil, err
		}
	} else {
		if err := pm.Install(packages...); err != nil {
			return "", nil, fmt.Errorf("%s install failed: %w", pm.Name(), err)
		}
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
func (p *Provider) CompensateInstall(host op.HostProvider, state any) error {
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	packages := op.StateStringSlice(s, "packages")
	alreadyInstalled := op.StateStringSlice(s, "already_installed")
	manager := op.StateString(s, "manager")
	cask := op.StateBool(s, "cask")

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

	if cask {
		for _, pkg := range toRemove {
			if err := runBrewCask("uninstall", pkg); err != nil {
				return err
			}
		}
		return nil
	}

	pm := resolvePMForInstall(host, manager)
	if pm == nil {
		return fmt.Errorf("no package manager available for compensation")
	}
	for _, pkg := range toRemove {
		if err := pm.Remove(pkg); err != nil {
			return fmt.Errorf("%s remove %s failed: %w", pm.Name(), pkg, err)
		}
	}
	return nil
}

// Upgrade upgrades packages using the platform's package manager.
// Returns compensation state with pre-upgrade versions per package.
//
// Parameters:
//   - packages: List of package names to upgrade
//   - manager: Package manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
//
// +devlore:access=planned
func (p *Provider) Upgrade(host op.HostProvider, packages []string, manager string, cask bool) (summary string, state map[string]any, retErr error) {
	if len(packages) == 0 {
		return "", nil, fmt.Errorf("no packages specified")
	}

	pm := resolvePMForUpgrade(host, manager, packages)
	if pm == nil {
		return "", nil, fmt.Errorf("no package manager available")
	}

	// Capture current versions before upgrading.
	previousVersions := make(map[string]string)
	for _, pkg := range packages {
		if v := pm.Version(pkg); v != "" {
			previousVersions[pkg] = v
		}
	}

	if cask {
		if err := runBrewCask("upgrade", packages...); err != nil {
			return "", nil, err
		}
	} else {
		if err := pm.Install(packages...); err != nil {
			return "", nil, fmt.Errorf("%s upgrade failed: %w", pm.Name(), err)
		}
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
//
// +devlore:access=planned
func (p *Provider) Remove(host op.HostProvider, packages []string, manager string, cask bool) (summary string, state map[string]any, retErr error) {
	if len(packages) == 0 {
		return "", nil, fmt.Errorf("no packages specified")
	}

	for _, pkg := range packages {
		if cask {
			if err := runBrewCask("uninstall", pkg); err != nil {
				return "", nil, err
			}
		} else {
			pm := resolvePMForRemove(host, manager, pkg)
			if pm == nil {
				return "", nil, fmt.Errorf("no package manager available")
			}
			if err := pm.Remove(pkg); err != nil {
				return "", nil, fmt.Errorf("%s remove %s failed: %w", pm.Name(), pkg, err)
			}
		}
	}

	return strings.Join(packages, ", "), map[string]any{
		"packages": packages,
		"manager":  manager,
		"cask":     cask,
	}, nil
}

// CompensateRemove undoes a Remove by reinstalling the removed packages.
func (p *Provider) CompensateRemove(host op.HostProvider, state any) error {
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	packages := op.StateStringSlice(s, "packages")
	manager := op.StateString(s, "manager")
	cask := op.StateBool(s, "cask")

	if len(packages) == 0 {
		return nil
	}

	if cask {
		return runBrewCask("install", packages...)
	}

	pm := resolvePMForInstall(host, manager)
	if pm == nil {
		return fmt.Errorf("no package manager available for compensation")
	}
	return pm.Install(packages...)
}

// Update refreshes the package manager index.
//
// Parameters:
//   - manager: Package manager override (empty for auto-detect)
//
// +devlore:access=planned
func (p *Provider) Update(host op.HostProvider, manager string) (string, error) {
	pm := resolvePMForInstall(host, manager)
	if pm == nil {
		return "", fmt.Errorf("no package manager available")
	}

	if err := pm.Update(); err != nil {
		return "", fmt.Errorf("%s update failed: %w", pm.Name(), err)
	}
	return pm.Name(), nil
}

// --- Predicates ---

// Installed returns true if the named package is installed.
//
// Parameters:
//   - name: Package name to check
//
// +devlore:access=both
func (p *Provider) Installed(host op.HostProvider, name string) (bool, error) {
	pm := host.PackageManager()
	if pm == nil {
		return false, fmt.Errorf("no package manager available")
	}
	return pm.Installed(name), nil
}

// NotInstalled returns true if the named package is not installed.
//
// Parameters:
//   - name: Package name to check
//
// +devlore:access=both
func (p *Provider) NotInstalled(host op.HostProvider, name string) (bool, error) {
	pm := host.PackageManager()
	if pm == nil {
		return false, fmt.Errorf("no package manager available")
	}
	return !pm.Installed(name), nil
}

// VersionGTE returns true if the installed version of name is >= version.
//
// Parameters:
//   - name: Package name to check
//   - version: Minimum version string to compare against
//
// +devlore:access=both
func (p *Provider) VersionGTE(host op.HostProvider, name, version string) (bool, error) {
	pm := host.PackageManager()
	if pm == nil {
		return false, fmt.Errorf("no package manager available")
	}
	current := pm.Version(name)
	if current == "" {
		return false, nil
	}
	return current >= version, nil
}
