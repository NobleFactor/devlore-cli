// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Code generated from gen-receiver templates; DO NOT EDIT.

package execution

import "fmt"

// ShellOp executes a POSIX shell command from node's "command" slot.
type ShellOp struct{ impl *ShellService }

func (o *ShellOp) Name() string { return "shell" }

func (o *ShellOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	command, _ := node.GetSlot("command").(string)
	if command == "" {
		return nil, nil, fmt.Errorf("shell: no command specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] shell %v\n", command)
		return nil, nil, nil
	}
	_, _ = fmt.Fprintf(ctx.Logger, "[shell] %s\n", command)
	return nil, nil, o.impl.Shell(command, ctx.Logger)
}

func (o *ShellOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// PowerShellOp executes a PowerShell command from node's "command" slot (Windows).
type PowerShellOp struct{ impl *ShellService }

func (o *PowerShellOp) Name() string { return "powershell" }

func (o *PowerShellOp) Do(ctx *Context, node *Node) (Result, UndoState, error) {
	command, _ := node.GetSlot("command").(string)
	if command == "" {
		return nil, nil, fmt.Errorf("powershell: no command specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] powershell %v\n", command)
		return nil, nil, nil
	}
	_, _ = fmt.Fprintf(ctx.Logger, "[powershell] %s\n", command)
	return nil, nil, o.impl.PowerShell(command, ctx.Logger)
}

func (o *PowerShellOp) Undo(_ *Context, _ *Node, _ UndoState) error { return nil }

// ShellOps returns all shell actions backed by the given ShellService.
func ShellOps(impl *ShellService) []Action {
	return []Action{
		&ShellOp{impl: impl},
		&PowerShellOp{impl: impl},
	}
}
