// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides platform-independent package management.
// All platform-specific behavior is delegated to the Platform
// injected at construction time from op.Context.Platform.
//
// Compensable Forward methods return ([]string, map[string]any, error):
// the packages acted upon, the compensation receipt, and an error.
// The map is opaque to the executor, meaningful only to the corresponding
// Compensate* Backward method.
//
// +devlore:access=both
type Provider struct {
	Platform *op.Platform
}

// ── Compensable Pairs ────────────────────────────────────────────────

// Install installs packages using the platform's package manager.
// Returns compensation state with pre-install status per package.
//
// Parameters:
//   - packages: List of package names to install
//   - manager: Package manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
func (p *Provider) Install(packages []string, manager string, cask bool) (result []string, state map[string]any, err error) {
	if len(packages) == 0 {
		return nil, nil, fmt.Errorf("no packages specified")
	}

	packageManager := resolvePlatformManagerForInstall(p.Platform, manager)
	if packageManager == nil {
		return nil, nil, fmt.Errorf("no package manager available")
	}

	// Query which packages are already installed before acting.
	var alreadyInstalled []string
	for _, packageName := range packages {
		if packageManager.Installed(packageName) {
			alreadyInstalled = append(alreadyInstalled, packageName)
		}
	}

	if cask {
		if err := runBrewCask("install", packages...); err != nil {
			return nil, nil, err
		}
	} else {
		r := packageManager.Install(packages...)
		if !r.OK {
			return nil, nil, fmt.Errorf("%s install failed: %s", packageManager.Name(), r.Stderr)
		}
	}

	return packages, map[string]any{
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
	for _, packageName := range alreadyInstalled {
		installed[packageName] = true
	}

	var toRemove []string
	for _, packageName := range packages {
		if !installed[packageName] {
			toRemove = append(toRemove, packageName)
		}
	}

	if len(toRemove) == 0 {
		return nil
	}

	if cask {
		for _, packageName := range toRemove {
			if err := runBrewCask("uninstall", packageName); err != nil {
				return err
			}
		}
		return nil
	}

	packageManager := resolvePlatformManagerForInstall(p.Platform, manager)
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
//   - packages: List of package names to remove
//   - manager: Package manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
func (p *Provider) Remove(packages []string, manager string, cask bool) (result []string, state map[string]any, err error) {
	if len(packages) == 0 {
		return nil, nil, fmt.Errorf("no packages specified")
	}

	for _, packageName := range packages {
		if cask {
			if err := runBrewCask("uninstall", packageName); err != nil {
				return nil, nil, err
			}
		} else {
			packageManager := resolvePlatformManagerForRemove(p.Platform, manager, packageName)
			if packageManager == nil {
				return nil, nil, fmt.Errorf("no package manager available")
			}
			r := packageManager.Remove(packageName)
			if !r.OK {
				return nil, nil, fmt.Errorf("%s remove %s failed: %s", packageManager.Name(), packageName, r.Stderr)
			}
		}
	}

	return packages, map[string]any{
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

	if cask {
		return runBrewCask("install", packages...)
	}

	packageManager := resolvePlatformManagerForInstall(p.Platform, manager)
	if packageManager == nil {
		return fmt.Errorf("no package manager available for compensation")
	}
	r := packageManager.Install(packages...)
	if !r.OK {
		return fmt.Errorf("%s install failed: %s", packageManager.Name(), r.Stderr)
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
func (p *Provider) Upgrade(packages []string, manager string, cask bool) (result []string, state map[string]any, err error) {
	if len(packages) == 0 {
		return nil, nil, fmt.Errorf("no packages specified")
	}

	packageManager := resolvePlatformManagerForUpgrade(p.Platform, manager, packages)
	if packageManager == nil {
		return nil, nil, fmt.Errorf("no package manager available")
	}

	// Capture current versions before upgrading.
	previousVersions := make(map[string]string)
	for _, packageName := range packages {
		if v := packageManager.Version(packageName); v != "" {
			previousVersions[packageName] = v
		}
	}

	if cask {
		if err := runBrewCask("upgrade", packages...); err != nil {
			return nil, nil, err
		}
	} else {
		r := packageManager.Install(packages...)
		if !r.OK {
			return nil, nil, fmt.Errorf("%s upgrade failed: %s", packageManager.Name(), r.Stderr)
		}
	}

	return packages, map[string]any{
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

// ── Standalone Methods ───────────────────────────────────────────────

// Update refreshes the package manager index.
//
// Parameters:
//   - manager: Package manager override (empty for auto-detect)
func (p *Provider) Update(manager string) (string, error) {
	packageManager := resolvePlatformManagerForInstall(p.Platform, manager)
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
//   - name: Package name to check
func (p *Provider) Installed(name string) (bool, error) {
	packageManager := p.Platform.PackageManager
	if packageManager == nil {
		return false, fmt.Errorf("no package manager available")
	}
	return packageManager.Installed(name), nil
}

// NotInstalled returns true if the named package is not installed.
//
// Parameters:
//   - name: Package name to check
func (p *Provider) NotInstalled(name string) (bool, error) {
	packageManager := p.Platform.PackageManager
	if packageManager == nil {
		return false, fmt.Errorf("no package manager available")
	}
	return !packageManager.Installed(name), nil
}

// VersionGTE returns true if the installed version of name is >= version.
//
// Parameters:
//   - name: Package name to check
//   - version: Minimum version string to compare against
func (p *Provider) VersionGTE(name, version string) (bool, error) {
	packageManager := p.Platform.PackageManager
	if packageManager == nil {
		return false, fmt.Errorf("no package manager available")
	}
	current := packageManager.Version(name)
	if current == "" {
		return false, nil
	}
	return current >= version, nil
}
