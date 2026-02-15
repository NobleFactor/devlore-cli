// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"fmt"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Install installs packages using the platform's package manager.
type Install struct{ Impl *Provider }

func (o *Install) Name() string { return "package-install" }

func (o *Install) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	packages, _ := node.GetSlot("packages").([]string)
	manager, _ := node.GetSlot("manager").(string)
	cask, _ := node.GetSlot("cask").(bool)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] package-install %v\n", strings.Join(packages, " "))
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Install(packages, manager, cask)
}

func (o *Install) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Upgrade upgrades packages using the platform's package manager.
type Upgrade struct{ Impl *Provider }

func (o *Upgrade) Name() string { return "package-upgrade" }

func (o *Upgrade) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	packages, _ := node.GetSlot("packages").([]string)
	manager, _ := node.GetSlot("manager").(string)
	cask, _ := node.GetSlot("cask").(bool)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] package-upgrade %v\n", strings.Join(packages, " "))
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Upgrade(packages, manager, cask)
}

func (o *Upgrade) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Remove removes packages using the platform's package manager.
type Remove struct{ Impl *Provider }

func (o *Remove) Name() string { return "package-remove" }

func (o *Remove) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	packages, _ := node.GetSlot("packages").([]string)
	manager, _ := node.GetSlot("manager").(string)
	cask, _ := node.GetSlot("cask").(bool)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] package-remove %v\n", strings.Join(packages, " "))
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Remove(packages, manager, cask)
}

func (o *Remove) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Update refreshes the package manager index.
type Update struct{ Impl *Provider }

func (o *Update) Name() string { return "package-update" }

func (o *Update) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	manager, _ := node.GetSlot("manager").(string)

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] package-update\n")
		return nil, nil, nil
	}
	return nil, nil, o.Impl.Update(manager)
}

func (o *Update) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Register registers all package actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Install{Impl: p})
	reg.Register(&Upgrade{Impl: p})
	reg.Register(&Remove{Impl: p})
	reg.Register(&Update{Impl: p})
}
