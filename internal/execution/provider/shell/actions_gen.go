// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package shell

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// Exec executes a POSIX shell command from node's "command" slot.
type Exec struct{ Impl *Provider }

func (o *Exec) Name() string { return "shell" }

func (o *Exec) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	command, _ := node.GetSlot("command").(string)
	if command == "" {
		return nil, nil, fmt.Errorf("shell: no command specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] shell %v\n", command)
		return nil, nil, nil
	}
	_, _ = fmt.Fprintf(ctx.Logger, "[shell] %s\n", command)
	return nil, nil, o.Impl.Shell(command, ctx.Logger)
}

func (o *Exec) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// PowerShell executes a PowerShell command from node's "command" slot (Windows).
type PowerShell struct{ Impl *Provider }

func (o *PowerShell) Name() string { return "powershell" }

func (o *PowerShell) Do(ctx *execution.Context, node *execution.Node) (execution.Result, execution.UndoState, error) {
	command, _ := node.GetSlot("command").(string)
	if command == "" {
		return nil, nil, fmt.Errorf("powershell: no command specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] powershell %v\n", command)
		return nil, nil, nil
	}
	_, _ = fmt.Fprintf(ctx.Logger, "[powershell] %s\n", command)
	return nil, nil, o.Impl.PowerShell(command, ctx.Logger)
}

func (o *PowerShell) Undo(_ *execution.Context, _ *execution.Node, _ execution.UndoState) error {
	return nil
}

// Register registers all shell actions with the given registry.
func Register(reg *execution.ActionRegistry) {
	p := &Provider{}
	reg.Register(&Exec{Impl: p})
	reg.Register(&PowerShell{Impl: p})
}
