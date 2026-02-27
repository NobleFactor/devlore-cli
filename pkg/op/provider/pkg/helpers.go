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

// resolvePlatformManagerForInstall returns the package manager for install actions.
func resolvePlatformManagerForInstall(platform *op.Platform, managerOverride string) op.PackageManager { //nolint:ireturn // returns concrete behind interface
	if managerOverride != "" {
		packageManager := platform.GetPackageManager(managerOverride)
		if packageManager != nil {
			return packageManager
		}
	}
	return platform.PackageManager
}

// resolvePlatformManagerForUpgrade returns the package manager for upgrade actions.
func resolvePlatformManagerForUpgrade(platform *op.Platform, managerOverride string, packages []string) op.PackageManager { //nolint:ireturn // returns concrete behind interface
	if managerOverride != "" {
		packageManager := platform.GetPackageManager(managerOverride)
		if packageManager != nil {
			return packageManager
		}
	}

	if len(packages) > 0 {
		packageManager := platform.InstalledBy(packages[0])
		if packageManager != nil {
			return packageManager
		}
	}

	return platform.PackageManager
}

// resolvePlatformManagerForRemove returns the package manager for remove actions.
func resolvePlatformManagerForRemove(platform *op.Platform, managerOverride, name string) op.PackageManager { //nolint:ireturn // returns concrete behind interface
	if managerOverride != "" {
		packageManager := platform.GetPackageManager(managerOverride)
		if packageManager != nil {
			return packageManager
		}
	}

	packageManager := platform.InstalledBy(name)
	if packageManager != nil {
		return packageManager
	}

	return platform.PackageManager
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
