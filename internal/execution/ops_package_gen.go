// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Code generated from gen-receiver templates; DO NOT EDIT.

package execution

import (
	"fmt"
	"strings"
)

// PackageInstallOp installs packages using the platform's package manager.
type PackageInstallOp struct{ impl *PackageService }

func (o *PackageInstallOp) Name() string { return "package-install" }

func (o *PackageInstallOp) Execute(ctx *Context, node *Node) error {
	packages, _ := node.GetSlot("packages").([]string)
	manager, _ := node.GetSlot("manager").(string)
	cask, _ := node.GetSlot("cask").(bool)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] package-install %v\n", strings.Join(packages, " "))
		return nil
	}
	return o.impl.Install(packages, manager, cask)
}

// PackageUpgradeOp upgrades packages using the platform's package manager.
type PackageUpgradeOp struct{ impl *PackageService }

func (o *PackageUpgradeOp) Name() string { return "package-upgrade" }

func (o *PackageUpgradeOp) Execute(ctx *Context, node *Node) error {
	packages, _ := node.GetSlot("packages").([]string)
	manager, _ := node.GetSlot("manager").(string)
	cask, _ := node.GetSlot("cask").(bool)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] package-upgrade %v\n", strings.Join(packages, " "))
		return nil
	}
	return o.impl.Upgrade(packages, manager, cask)
}

// PackageRemoveOp removes packages using the platform's package manager.
type PackageRemoveOp struct{ impl *PackageService }

func (o *PackageRemoveOp) Name() string { return "package-remove" }

func (o *PackageRemoveOp) Execute(ctx *Context, node *Node) error {
	packages, _ := node.GetSlot("packages").([]string)
	manager, _ := node.GetSlot("manager").(string)
	cask, _ := node.GetSlot("cask").(bool)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] package-remove %v\n", strings.Join(packages, " "))
		return nil
	}
	return o.impl.Remove(packages, manager, cask)
}

// PackageUpdateOp refreshes the package manager index.
type PackageUpdateOp struct{ impl *PackageService }

func (o *PackageUpdateOp) Name() string { return "package-update" }

func (o *PackageUpdateOp) Execute(ctx *Context, node *Node) error {
	manager, _ := node.GetSlot("manager").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] package-update\n")
		return nil
	}
	return o.impl.Update(manager)
}

// PackageOps returns all package operations backed by the given PackageService.
func PackageOps(impl *PackageService) []Operation {
	return []Operation{
		&PackageInstallOp{impl: impl},
		&PackageUpgradeOp{impl: impl},
		&PackageRemoveOp{impl: impl},
		&PackageUpdateOp{impl: impl},
	}
}
