// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// PackageService provides platform-independent package management.
// The package manager is resolved at runtime via host.PackageManager().
type PackageService struct{}

// Install installs packages using the platform's package manager.
func (p *PackageService) Install(packages []string, manager string, cask bool) error {
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
func (p *PackageService) Upgrade(packages []string, manager string, cask bool) error {
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
func (p *PackageService) Remove(packages []string, manager string, cask bool) error {
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
func (p *PackageService) Update(manager string) error {
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

// resolvePMForInstall returns the package manager for install operations.
func resolvePMForInstall(managerOverride string) host.PackageManager {
	h := host.NewHost()

	if managerOverride != "" {
		pm := h.GetPackageManager(managerOverride)
		if pm != nil {
			return pm
		}
	}

	return h.PackageManager()
}

// resolvePMForUpgrade returns the package manager for upgrade operations.
func resolvePMForUpgrade(managerOverride string, packages []string) host.PackageManager {
	h := host.NewHost()

	if managerOverride != "" {
		pm := h.GetPackageManager(managerOverride)
		if pm != nil {
			return pm
		}
	}

	if len(packages) > 0 {
		pm := h.InstalledBy(packages[0])
		if pm != nil {
			return pm
		}
	}

	return h.PackageManager()
}

// resolvePMForRemove returns the package manager for remove operations.
func resolvePMForRemove(managerOverride string, pkg string) (pm host.PackageManager, otherPMs []host.PackageManager) {
	h := host.NewHost()

	allPMs := h.AllInstalledBy(pkg)

	if managerOverride != "" {
		pm = h.GetPackageManager(managerOverride)
		if pm != nil {
			for _, other := range allPMs {
				if other.Name() != pm.Name() {
					otherPMs = append(otherPMs, other)
				}
			}
			return pm, otherPMs
		}
	}

	pm = h.InstalledBy(pkg)
	if pm != nil {
		for _, other := range allPMs {
			if other.Name() != pm.Name() {
				otherPMs = append(otherPMs, other)
			}
		}
		return pm, otherPMs
	}

	return h.PackageManager(), nil
}

// runBrewCaskInstall installs packages via Homebrew Cask.
func runBrewCaskInstall(packages []string) host.Result {
	cmd := exec.Command("brew", append([]string{"install", "--cask"}, packages...)...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return host.Result{
		OK:     err == nil,
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
}

// runBrewCaskUpgrade upgrades packages via Homebrew Cask.
func runBrewCaskUpgrade(packages []string) host.Result {
	cmd := exec.Command("brew", append([]string{"upgrade", "--cask"}, packages...)...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return host.Result{
		OK:     err == nil,
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
}

// runBrewCaskRemove removes a package via Homebrew Cask.
func runBrewCaskRemove(pkg string) host.Result {
	cmd := exec.Command("brew", "uninstall", "--cask", pkg)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return host.Result{
		OK:     err == nil,
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
}
