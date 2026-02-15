// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import "fmt"

// Provider provides platform-independent package management.
// The package manager is resolved at runtime via host.PackageManager().
type Provider struct{}

// Install installs packages using the platform's package manager.
func (p *Provider) Install(packages []string, manager string, cask bool) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
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

// Upgrade upgrades packages using the platform's package manager.
func (p *Provider) Upgrade(packages []string, manager string, cask bool) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
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

// Remove removes packages using the platform's package manager.
func (p *Provider) Remove(packages []string, manager string, cask bool) error {
	if len(packages) == 0 {
		return fmt.Errorf("no packages specified")
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

// Update refreshes the package manager index.
func (p *Provider) Update(manager string) error {
	pm := resolvePMForInstall(manager)
	if pm == nil {
		return fmt.Errorf("no package manager available")
	}

	result := pm.Update()
	if !result.OK {
		return fmt.Errorf("%s update failed: %s", pm.Name(), result.Stderr)
	}
	return nil
}
