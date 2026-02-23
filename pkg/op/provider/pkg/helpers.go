// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package pkg provides package management actions for the operation graph.
package pkg

import (
	"context"
	"os/exec"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// resolvePMForInstall returns the package manager for install actions.
func resolvePMForInstall(host op.HostProvider, managerOverride string) op.PackageManagerProvider {
	if managerOverride != "" {
		pm := host.GetPackageManager(managerOverride)
		if pm != nil {
			return pm
		}
	}
	return host.PackageManager()
}

// resolvePMForUpgrade returns the package manager for upgrade actions.
func resolvePMForUpgrade(host op.HostProvider, managerOverride string, packages []string) op.PackageManagerProvider {
	if managerOverride != "" {
		pm := host.GetPackageManager(managerOverride)
		if pm != nil {
			return pm
		}
	}

	if len(packages) > 0 {
		pm := host.InstalledBy(packages[0])
		if pm != nil {
			return pm
		}
	}

	return host.PackageManager()
}

// resolvePMForRemove returns the package manager for remove actions.
func resolvePMForRemove(host op.HostProvider, managerOverride, p string) op.PackageManagerProvider {
	if managerOverride != "" {
		pm := host.GetPackageManager(managerOverride)
		if pm != nil {
			return pm
		}
	}

	pm := host.InstalledBy(p)
	if pm != nil {
		return pm
	}

	return host.PackageManager()
}

// runBrewCask executes a brew cask command (install, upgrade, or uninstall).
func runBrewCask(action string, packages ...string) error {
	args := append([]string{action, "--cask"}, packages...)
	cmd := exec.CommandContext(context.Background(), "brew", args...) //nolint:gosec // G204: command built from provider inputs
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return &brewCaskError{action: action, stderr: stderr.String()}
	}
	return nil
}

// brewCaskError wraps a failed brew cask command.
type brewCaskError struct {
	action string
	stderr string
}

func (e *brewCaskError) Error() string {
	return "brew " + e.action + " --cask failed: " + e.stderr
}
