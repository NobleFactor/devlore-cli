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

func (o *PackageInstallOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	packages, _ := node.GetSlot("packages").([]string)
	manager, _ := node.GetSlot("manager").(string)
	cask, _ := node.GetSlot("cask").(bool)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] package-install %v\n", strings.Join(packages, " "))
		return nil, nil, nil
	}
	return nil, nil, o.impl.Install(packages, manager, cask)
}

func (o *PackageInstallOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// PackageUpgradeOp upgrades packages using the platform's package manager.
type PackageUpgradeOp struct{ impl *PackageService }

func (o *PackageUpgradeOp) Name() string { return "package-upgrade" }

func (o *PackageUpgradeOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	packages, _ := node.GetSlot("packages").([]string)
	manager, _ := node.GetSlot("manager").(string)
	cask, _ := node.GetSlot("cask").(bool)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] package-upgrade %v\n", strings.Join(packages, " "))
		return nil, nil, nil
	}
	return nil, nil, o.impl.Upgrade(packages, manager, cask)
}

func (o *PackageUpgradeOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// PackageRemoveOp removes packages using the platform's package manager.
type PackageRemoveOp struct{ impl *PackageService }

func (o *PackageRemoveOp) Name() string { return "package-remove" }

func (o *PackageRemoveOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	packages, _ := node.GetSlot("packages").([]string)
	manager, _ := node.GetSlot("manager").(string)
	cask, _ := node.GetSlot("cask").(bool)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] package-remove %v\n", strings.Join(packages, " "))
		return nil, nil, nil
	}
	return nil, nil, o.impl.Remove(packages, manager, cask)
}

func (o *PackageRemoveOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// PackageUpdateOp refreshes the package manager index.
type PackageUpdateOp struct{ impl *PackageService }

func (o *PackageUpdateOp) Name() string { return "package-update" }

func (o *PackageUpdateOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	manager, _ := node.GetSlot("manager").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] package-update\n")
		return nil, nil, nil
	}
	return nil, nil, o.impl.Update(manager)
}

func (o *PackageUpdateOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// PackageOps returns all package actions backed by the given PackageService.
func PackageOps(impl *PackageService) []Action {
	return []Action{
		&PackageInstallOp{impl: impl},
		&PackageUpgradeOp{impl: impl},
		&PackageRemoveOp{impl: impl},
		&PackageUpdateOp{impl: impl},
	}
}
