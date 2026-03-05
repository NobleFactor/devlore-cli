// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides platform-independent package management.
// Platform-specific behavior is delegated to p.Context().Platform.
//
// +devlore:access=both
type Provider struct {
	op.ProviderBase
}

func (p *Provider) platform() (*op.Platform, error) {
	plat := p.Context().Platform
	if plat == nil {
		return nil, fmt.Errorf("no platform available")
	}
	return plat, nil
}

// packageNames extracts the Name field from each Resource.
func packageNames(resources []Resource) []string {
	names := make([]string, len(resources))
	for i, r := range resources {
		names[i] = r.Name
	}
	return names
}

// ── Compensable Pairs ────────────────────────────────────────────────

// Install installs packages using the platform's package manager.
// Returns compensation state with pre-install status per package.
//
// Parameters:
//   - packages: package resources to install
//   - manager: Package manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
func (p *Provider) Install(packages []Resource, manager string, cask bool) (result []Resource, state Tombstone, err error) {
	if len(packages) == 0 {
		return nil, Tombstone{}, fmt.Errorf("no packages specified")
	}

	plat, err := p.platform()
	if err != nil {
		return nil, Tombstone{}, err
	}

	names := packageNames(packages)

	packageManager := resolvePlatformManagerForInstall(plat, manager)
	if packageManager == nil {
		return nil, Tombstone{}, fmt.Errorf("no package manager available")
	}

	// Query which packages are already installed before acting.
	var alreadyInstalled []string
	for _, packageName := range names {
		if packageManager.Installed(packageName) {
			alreadyInstalled = append(alreadyInstalled, packageName)
		}
	}

	if cask {
		if err := runBrewCask("install", names...); err != nil {
			return nil, Tombstone{}, err
		}
	} else {
		r := packageManager.Install(names...)
		if !r.OK {
			return nil, Tombstone{}, fmt.Errorf("%s install failed: %s", packageManager.Name(), r.Stderr)
		}
	}

	return packages, Tombstone{
		Packages:         names,
		Manager:          manager,
		Cask:             cask,
		AlreadyInstalled: alreadyInstalled,
	}, nil
}

// CompensateInstall undoes an Install by removing packages that weren't
// already installed before the action.
func (p *Provider) CompensateInstall(state Tombstone) error {
	if len(state.Packages) == 0 {
		return nil
	}

	installed := make(map[string]bool)
	for _, packageName := range state.AlreadyInstalled {
		installed[packageName] = true
	}

	var toRemove []string
	for _, packageName := range state.Packages {
		if !installed[packageName] {
			toRemove = append(toRemove, packageName)
		}
	}

	if len(toRemove) == 0 {
		return nil
	}

	if state.Cask {
		for _, packageName := range toRemove {
			if err := runBrewCask("uninstall", packageName); err != nil {
				return err
			}
		}
		return nil
	}

	plat, err := p.platform()
	if err != nil {
		return err
	}
	packageManager := resolvePlatformManagerForInstall(plat, state.Manager)
	if packageManager == nil {
		return fmt.Errorf("no package manager available for compensation")
	}
	for _, packageName := range toRemove {
		r := packageManager.Remove(packageName)
		if !r.OK {
			return fmt.Errorf("%s remove %s failed: %s", packageManager.Name(), packageName, r.Stderr)
		}
	}
	return nil
}

// Remove removes packages using the platform's package manager.
// Returns compensation state for reinstallation.
//
// Parameters:
//   - packages: package resources to remove
//   - manager: Package manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
func (p *Provider) Remove(packages []Resource, manager string, cask bool) (result []Resource, state Tombstone, err error) {
	if len(packages) == 0 {
		return nil, Tombstone{}, fmt.Errorf("no packages specified")
	}

	plat, err := p.platform()
	if err != nil {
		return nil, Tombstone{}, err
	}

	names := packageNames(packages)

	for _, packageName := range names {
		if cask {
			if err := runBrewCask("uninstall", packageName); err != nil {
				return nil, Tombstone{}, err
			}
		} else {
			packageManager := resolvePlatformManagerForRemove(plat, manager, packageName)
			if packageManager == nil {
				return nil, Tombstone{}, fmt.Errorf("no package manager available")
			}
			r := packageManager.Remove(packageName)
			if !r.OK {
				return nil, Tombstone{}, fmt.Errorf("%s remove %s failed: %s", packageManager.Name(), packageName, r.Stderr)
			}
		}
	}

	return packages, Tombstone{
		Packages: names,
		Manager:  manager,
		Cask:     cask,
	}, nil
}

// CompensateRemove undoes a Remove by reinstalling the removed packages.
func (p *Provider) CompensateRemove(state Tombstone) error {
	if len(state.Packages) == 0 {
		return nil
	}

	if state.Cask {
		return runBrewCask("install", state.Packages...)
	}

	plat, err := p.platform()
	if err != nil {
		return err
	}
	packageManager := resolvePlatformManagerForInstall(plat, state.Manager)
	if packageManager == nil {
		return fmt.Errorf("no package manager available for compensation")
	}
	r := packageManager.Install(state.Packages...)
	if !r.OK {
		return fmt.Errorf("%s install failed: %s", packageManager.Name(), r.Stderr)
	}
	return nil
}

// Upgrade upgrades packages using the platform's package manager.
// Returns compensation state with pre-upgrade versions per package.
//
// Parameters:
//   - packages: package resources to upgrade
//   - manager: Package manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
func (p *Provider) Upgrade(packages []Resource, manager string, cask bool) (result []Resource, state Tombstone, err error) {
	if len(packages) == 0 {
		return nil, Tombstone{}, fmt.Errorf("no packages specified")
	}

	plat, err := p.platform()
	if err != nil {
		return nil, Tombstone{}, err
	}

	names := packageNames(packages)

	packageManager := resolvePlatformManagerForUpgrade(plat, manager, names)
	if packageManager == nil {
		return nil, Tombstone{}, fmt.Errorf("no package manager available")
	}

	// Capture current versions before upgrading.
	previousVersions := make(map[string]string)
	for _, packageName := range names {
		if v := packageManager.Version(packageName); v != "" {
			previousVersions[packageName] = v
		}
	}

	if cask {
		if err := runBrewCask("upgrade", names...); err != nil {
			return nil, Tombstone{}, err
		}
	} else {
		r := packageManager.Install(names...)
		if !r.OK {
			return nil, Tombstone{}, fmt.Errorf("%s upgrade failed: %s", packageManager.Name(), r.Stderr)
		}
	}

	return packages, Tombstone{
		Packages:         names,
		Manager:          manager,
		Cask:             cask,
		PreviousVersions: previousVersions,
	}, nil
}

// CompensateUpgrade is a diagnostic no-op. Previous versions are captured
// in state for manual recovery, but automatic downgrade is not reliable
// across package managers.
func (p *Provider) CompensateUpgrade(_ Tombstone) error {
	return nil
}

// ── Standalone Methods ───────────────────────────────────────────────

// Update refreshes the package manager index.
//
// Parameters:
//   - manager: Package manager override (empty for auto-detect)
func (p *Provider) Update(manager string) (string, error) {
	plat, err := p.platform()
	if err != nil {
		return "", err
	}

	packageManager := resolvePlatformManagerForInstall(plat, manager)
	if packageManager == nil {
		return "", fmt.Errorf("no package manager available")
	}

	r := packageManager.Update()
	if !r.OK {
		return "", fmt.Errorf("%s update failed: %s", packageManager.Name(), r.Stderr)
	}
	return packageManager.Name(), nil
}

// ── Predicates ───────────────────────────────────────────────────────

// Installed returns true if the named package is installed.
//
// Parameters:
//   - name: package resource to check
func (p *Provider) Installed(name Resource) (bool, error) {
	plat, err := p.platform()
	if err != nil {
		return false, err
	}
	if plat.PackageManager == nil {
		return false, fmt.Errorf("no package manager available")
	}
	return plat.PackageManager.Installed(name.Name), nil
}

// NotInstalled returns true if the named package is not installed.
//
// Parameters:
//   - name: package resource to check
func (p *Provider) NotInstalled(name Resource) (bool, error) {
	plat, err := p.platform()
	if err != nil {
		return false, err
	}
	if plat.PackageManager == nil {
		return false, fmt.Errorf("no package manager available")
	}
	return !plat.PackageManager.Installed(name.Name), nil
}

// VersionGTE returns true if the installed version of name is >= version.
//
// Parameters:
//   - name: package resource to check
//   - version: Minimum version string to compare against
func (p *Provider) VersionGTE(name Resource, version string) (bool, error) {
	plat, err := p.platform()
	if err != nil {
		return false, err
	}
	if plat.PackageManager == nil {
		return false, fmt.Errorf("no package manager available")
	}
	current := plat.PackageManager.Version(name.Name)
	if current == "" {
		return false, nil
	}
	return current >= version, nil
}
