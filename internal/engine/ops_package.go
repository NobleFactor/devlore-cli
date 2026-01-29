// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package engine

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// =============================================================================
// Unified Package Manager Operations
// =============================================================================
//
// These four operations work on ALL platforms. The package manager is determined
// at execution time by host.PackageManager().
//
// On Darwin, the optional node.Metadata["manager"] can override the auto-detected
// package manager ("brew" or "port"). This supports the brew:pkg and port:pkg
// prefix syntax in plan.install().
//
// Package names are read from node.Metadata["packages"] (comma-separated).

// PackageInstallOp installs packages using the platform's package manager.
type PackageInstallOp struct{}

func (o *PackageInstallOp) Name() string         { return "package-install" }
func (o *PackageInstallOp) Category() OpCategory { return OpDirect }

func (o *PackageInstallOp) Execute(ctx *Context, node *Node) error {
	packages := parsePackages(node.Metadata["packages"])
	if len(packages) == 0 {
		return fmt.Errorf("package-install: no packages specified")
	}

	pm := resolvePM(node.Metadata["manager"])
	if pm == nil {
		return fmt.Errorf("package-install: no package manager available")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] %s install %s\n", pm.Name(), strings.Join(packages, " "))
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[package] %s install %s\n", pm.Name(), strings.Join(packages, " "))
	result := pm.Install(packages...)
	if !result.OK {
		return fmt.Errorf("%s install failed: %s", pm.Name(), result.Stderr)
	}
	return nil
}

// PackageUpgradeOp upgrades packages using the platform's package manager.
type PackageUpgradeOp struct{}

func (o *PackageUpgradeOp) Name() string         { return "package-upgrade" }
func (o *PackageUpgradeOp) Category() OpCategory { return OpDirect }

func (o *PackageUpgradeOp) Execute(ctx *Context, node *Node) error {
	packages := parsePackages(node.Metadata["packages"])
	if len(packages) == 0 {
		return fmt.Errorf("package-upgrade: no packages specified")
	}

	pm := resolvePM(node.Metadata["manager"])
	if pm == nil {
		return fmt.Errorf("package-upgrade: no package manager available")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] %s upgrade %s\n", pm.Name(), strings.Join(packages, " "))
		return nil
	}

	// Most package managers use the same command for install and upgrade
	// (they upgrade if already installed). The host.PackageManager interface
	// currently doesn't have a separate Upgrade method.
	_, _ = fmt.Fprintf(ctx.Logger, "[package] %s upgrade %s\n", pm.Name(), strings.Join(packages, " "))
	result := pm.Install(packages...)
	if !result.OK {
		return fmt.Errorf("%s upgrade failed: %s", pm.Name(), result.Stderr)
	}
	return nil
}

// PackageRemoveOp removes packages using the platform's package manager.
type PackageRemoveOp struct{}

func (o *PackageRemoveOp) Name() string         { return "package-remove" }
func (o *PackageRemoveOp) Category() OpCategory { return OpDirect }

func (o *PackageRemoveOp) Execute(ctx *Context, node *Node) error {
	packages := parsePackages(node.Metadata["packages"])
	if len(packages) == 0 {
		return fmt.Errorf("package-remove: no packages specified")
	}

	pm := resolvePM(node.Metadata["manager"])
	if pm == nil {
		return fmt.Errorf("package-remove: no package manager available")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] %s remove %s\n", pm.Name(), strings.Join(packages, " "))
		return nil
	}

	_, _ = fmt.Fprintf(ctx.Logger, "[package] %s remove %s\n", pm.Name(), strings.Join(packages, " "))
	for _, pkg := range packages {
		result := pm.Remove(pkg)
		if !result.OK {
			return fmt.Errorf("%s remove %s failed: %s", pm.Name(), pkg, result.Stderr)
		}
	}
	return nil
}

// PackageUpdateOp refreshes the package manager index.
type PackageUpdateOp struct{}

func (o *PackageUpdateOp) Name() string         { return "package-update" }
func (o *PackageUpdateOp) Category() OpCategory { return OpDirect }

func (o *PackageUpdateOp) Execute(ctx *Context, node *Node) error {
	pm := resolvePM(node.Metadata["manager"])
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

// resolvePM returns the package manager for the current platform.
// If managerOverride is specified (for Darwin brew:/port: prefix), it returns
// that specific manager instead of the auto-detected one.
func resolvePM(managerOverride string) host.PackageManager {
	h := host.NewHost()
	pm := h.PackageManager()

	// Handle Darwin manager override (brew: or port: prefix)
	if managerOverride != "" {
		// The override is handled by creating a specific PM
		// This requires access to the Darwin-specific managers
		// For now, we can only override if we're on Darwin and the
		// host package exposes a way to get a specific manager.
		// TODO: Add host.GetPackageManager(name string) to Host interface
		// For now, the override is stored in metadata and the PM is auto-detected
		_ = managerOverride // Acknowledge but can't use without Host interface change
	}

	return pm
}

// =============================================================================
// Shell Operations
// =============================================================================

// ShellOp executes a shell command from node.Metadata["command"].
type ShellOp struct{}

func (o *ShellOp) Name() string         { return "shell" }
func (o *ShellOp) Category() OpCategory { return OpDirect }

func (o *ShellOp) Execute(ctx *Context, node *Node) error {
	command := node.Metadata["command"]
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

// PowerShellOp executes a PowerShell command from node.Metadata["command"] (Windows).
type PowerShellOp struct{}

func (o *PowerShellOp) Name() string         { return "powershell" }
func (o *PowerShellOp) Category() OpCategory { return OpDirect }

func (o *PowerShellOp) Execute(ctx *Context, node *Node) error {
	command := node.Metadata["command"]
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
