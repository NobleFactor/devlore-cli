// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"os/exec"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

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
func resolvePMForRemove(managerOverride string, p string) (pm host.PackageManager, otherPMs []host.PackageManager) {
	h := host.NewHost()

	allPMs := h.AllInstalledBy(p)

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

	pm = h.InstalledBy(p)
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
func runBrewCaskRemove(p string) host.Result {
	cmd := exec.Command("brew", "uninstall", "--cask", p)
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
