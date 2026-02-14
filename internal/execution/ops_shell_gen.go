// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Code generated from gen-receiver templates; DO NOT EDIT.

package execution

import "fmt"

// ShellOp executes a POSIX shell command from node's "command" slot.
type ShellOp struct{ impl *ShellService }

func (o *ShellOp) Name() string { return "shell" }

func (o *ShellOp) Execute(ctx *Context, node *Node) error {
	command, _ := node.GetSlot("command").(string)
	if command == "" {
		return fmt.Errorf("shell: no command specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] shell %v\n", command)
		return nil
	}
	_, _ = fmt.Fprintf(ctx.Logger, "[shell] %s\n", command)
	return o.impl.Shell(command, ctx.Logger)
}

// PowerShellOp executes a PowerShell command from node's "command" slot (Windows).
type PowerShellOp struct{ impl *ShellService }

func (o *PowerShellOp) Name() string { return "powershell" }

func (o *PowerShellOp) Execute(ctx *Context, node *Node) error {
	command, _ := node.GetSlot("command").(string)
	if command == "" {
		return fmt.Errorf("powershell: no command specified")
	}

	if ctx.DryRun {
		_, _ = fmt.Fprintf(ctx.Logger, "[dry-run] powershell %v\n", command)
		return nil
	}
	_, _ = fmt.Fprintf(ctx.Logger, "[powershell] %s\n", command)
	return o.impl.PowerShell(command, ctx.Logger)
}

// ShellOps returns all shell operations backed by the given ShellService.
func ShellOps(impl *ShellService) []Operation {
	return []Operation{
		&ShellOp{impl: impl},
		&PowerShellOp{impl: impl},
	}
}
