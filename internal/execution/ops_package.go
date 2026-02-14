// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"fmt"
	"os/exec"
	"strings"
)

// =============================================================================
// Unified Package Manager Operations
// =============================================================================
//
// These four operations work on ALL platforms. The package manager is determined
// at execution time by host.PackageManager().
//
// On Darwin, the optional node "manager" slot can override the auto-detected
// package manager ("brew" or "port"). This supports the brew:pkg and port:pkg
// prefix syntax in plan.install().
//
// Package names are read from node's "packages" slot (comma-separated).

// PackageInstallOp installs packages using the platform's package manager.
type PackageInstallOp struct{}

func (o *PackageInstallOp) Name() string { return "package-install" }

func (o *PackageInstallOp) Execute(ctx *Context, node *Node) error {
	pkgList, _ := node.GetSlot("packages").(string)
	packages := parsePackages(pkgList)
	if len(packages) == 0 {
		return fmt.Errorf("package-install: no packages specified")
	}

	manager, _ := node.GetSlot("manager").(string)
	pm := resolvePMForInstall(manager)
	if pm == nil {
		return fmt.Errorf("package-install: no package manager available")
	}

	// Check if cask mode is enabled (for Homebrew Cask)
	cask, _ := node.GetSlot("cask").(string)
	isCask := cask == "true"

	if ctx.DryRun {
		if isCask {
			_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] %s install --cask %s\n", pm.Name(), strings.Join(packages, " "))
		} else {
			_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] %s install %s\n", pm.Name(), strings.Join(packages, " "))
		}
		return nil
	}

	if isCask {
		_, _ = fmt.Fprintf(ctx.Logger, "[package] %s install --cask %s\n", pm.Name(), strings.Join(packages, " "))
		// Homebrew Cask requires --cask flag
		result := runBrewCaskInstall(packages)
		if !result.OK {
			return fmt.Errorf("brew install --cask failed: %s", result.Stderr)
		}
	} else {
		_, _ = fmt.Fprintf(ctx.Logger, "[package] %s install %s\n", pm.Name(), strings.Join(packages, " "))
		result := pm.Install(packages...)
		if !result.OK {
			return fmt.Errorf("%s install failed: %s", pm.Name(), result.Stderr)
		}
	}
	return nil
}

// PackageUpgradeOp upgrades packages using the platform's package manager.
type PackageUpgradeOp struct{}

func (o *PackageUpgradeOp) Name() string { return "package-upgrade" }

func (o *PackageUpgradeOp) Execute(ctx *Context, node *Node) error {
	pkgList, _ := node.GetSlot("packages").(string)
	packages := parsePackages(pkgList)
	if len(packages) == 0 {
		return fmt.Errorf("package-upgrade: no packages specified")
	}

	// Use InstalledBy to determine which PM to upgrade with
	manager, _ := node.GetSlot("manager").(string)
	pm := resolvePMForUpgrade(manager, packages)
	if pm == nil {
		return fmt.Errorf("package-upgrade: no package manager available")
	}

	// Check if cask mode is enabled (for Homebrew Cask)
	cask, _ := node.GetSlot("cask").(string)
	isCask := cask == "true"

	if ctx.DryRun {
		if isCask {
			_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] %s upgrade --cask %s\n", pm.Name(), strings.Join(packages, " "))
		} else {
			_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] %s upgrade %s\n", pm.Name(), strings.Join(packages, " "))
		}
		return nil
	}

	// Most package managers use the same command for install and upgrade
	// (they upgrade if already installed). The host.PackageManager interface
	// currently doesn't have a separate Upgrade method.
	if isCask {
		_, _ = fmt.Fprintf(ctx.Logger, "[package] %s upgrade --cask %s\n", pm.Name(), strings.Join(packages, " "))
		result := runBrewCaskUpgrade(packages)
		if !result.OK {
			return fmt.Errorf("brew upgrade --cask failed: %s", result.Stderr)
		}
	} else {
		_, _ = fmt.Fprintf(ctx.Logger, "[package] %s upgrade %s\n", pm.Name(), strings.Join(packages, " "))
		result := pm.Install(packages...)
		if !result.OK {
			return fmt.Errorf("%s upgrade failed: %s", pm.Name(), result.Stderr)
		}
	}
	return nil
}

// PackageRemoveOp removes packages using the platform's package manager.
type PackageRemoveOp struct{}

func (o *PackageRemoveOp) Name() string { return "package-remove" }

func (o *PackageRemoveOp) Execute(ctx *Context, node *Node) error {
	pkgList, _ := node.GetSlot("packages").(string)
	packages := parsePackages(pkgList)
	if len(packages) == 0 {
		return fmt.Errorf("package-remove: no packages specified")
	}

	// Check if cask mode is enabled (for Homebrew Cask)
	cask, _ := node.GetSlot("cask").(string)
	isCask := cask == "true"
	manager, _ := node.GetSlot("manager").(string)

	for _, pkg := range packages {
		// Use InstalledBy to determine which PM to remove with
		pm, otherPMs := resolvePMForRemove(manager, pkg)
		if pm == nil {
			return fmt.Errorf("package-remove: no package manager available")
		}

		// Warn if package is installed by multiple PMs
		if len(otherPMs) > 0 {
			otherNames := make([]string, len(otherPMs))
			for i, other := range otherPMs {
				otherNames[i] = other.Name()
			}
			_, _ = fmt.Fprintf(ctx.Logger, "[warning] %s is also installed via %s; use '%s:%s' to remove that copy\n",
				pkg, strings.Join(otherNames, ", "), otherNames[0], pkg)
		}

		if ctx.DryRun {
			if isCask {
				_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] %s uninstall --cask %s\n", pm.Name(), pkg)
			} else {
				_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] %s remove %s\n", pm.Name(), pkg)
			}
			continue
		}

		if isCask {
			_, _ = fmt.Fprintf(ctx.Logger, "[package] %s uninstall --cask %s\n", pm.Name(), pkg)
			result := runBrewCaskRemove(pkg)
			if !result.OK {
				return fmt.Errorf("brew uninstall --cask %s failed: %s", pkg, result.Stderr)
			}
		} else {
			_, _ = fmt.Fprintf(ctx.Logger, "[package] %s remove %s\n", pm.Name(), pkg)
			result := pm.Remove(pkg)
			if !result.OK {
				return fmt.Errorf("%s remove %s failed: %s", pm.Name(), pkg, result.Stderr)
			}
		}
	}
	return nil
}

// PackageUpdateOp refreshes the package manager index.
type PackageUpdateOp struct{}

func (o *PackageUpdateOp) Name() string { return "package-update" }

func (o *PackageUpdateOp) Execute(ctx *Context, node *Node) error {
	// Update uses preferred PM (not InstalledBy - we're updating the index, not a package)
	manager, _ := node.GetSlot("manager").(string)
	pm := resolvePMForInstall(manager)
	if pm == nil {
		return fmt.Errorf("package-update: no package manager available")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] %s update\n", pm.Name())
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[package] %s update\n", pm.Name())
	result := pm.Update()
	if !result.OK {
		return fmt.Errorf("%s update failed: %s", pm.Name(), result.Stderr)
	}
	return nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// parsePackages splits comma-separated package list, handling empty input.
func parsePackages(pkgList string) []string {
	if pkgList == "" {
		return nil
	}
	packages := strings.Split(pkgList, ",")
	if len(packages) == 1 && packages[0] == "" {
		return nil
	}
	return packages
}

// =============================================================================
// Shell Operations
// =============================================================================

// ShellOp executes a shell command from node's "command" slot.
type ShellOp struct{}

func (o *ShellOp) Name() string { return "shell" }

func (o *ShellOp) Execute(ctx *Context, node *Node) error {
	command, _ := node.GetSlot("command").(string)
	if command == "" {
		return fmt.Errorf("shell: no command specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] %s\n", command)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[shell] %s\n", command)
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// PowerShellOp executes a PowerShell command from node's "command" slot (Windows).
type PowerShellOp struct{}

func (o *PowerShellOp) Name() string { return "powershell" }

func (o *PowerShellOp) Execute(ctx *Context, node *Node) error {
	command, _ := node.GetSlot("command").(string)
	if command == "" {
		return fmt.Errorf("powershell: no command specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] powershell -Command %s\n", command)
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[powershell] %s\n", command)
	cmd := exec.Command("powershell", "-Command", command)
	cmd.Stdout = ctx.Logger
	cmd.Stderr = ctx.Logger
	return cmd.Run()
}

// PackageOps returns all package manager operations for registration.
func PackageOps() []Operation {
	return []Operation{
		// Unified package operations (work on all platforms)
		// Namespaced with "package-" prefix to distinguish from file operations
		&PackageInstallOp{},
		&PackageUpgradeOp{},
		&PackageRemoveOp{},
		&PackageUpdateOp{},
		// Shell operations
		&ShellOp{},
		&PowerShellOp{},
	}
}
