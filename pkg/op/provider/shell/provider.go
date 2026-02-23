// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package shell provides shell command execution actions for the operation graph.
package shell

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// Provider provides shell command execution.
type Provider struct{}

// Exec executes a POSIX shell command.
//
// Parameters:
//   - command: Shell command string to execute via sh -c
//
// +devlore:access=planned
func (p *Provider) Exec(command string, output io.Writer) (string, error) {
	if command == "" {
		return "", fmt.Errorf("no command specified")
	}
	cmd := exec.CommandContext(context.Background(), "sh", "-c", command) //nolint:gosec // G204: command built from provider inputs
	cmd.Stdout = output
	cmd.Stderr = output
	return command, cmd.Run()
}

// PowerShell executes a PowerShell command (Windows).
//
// Parameters:
//   - command: PowerShell command string to execute
//
// +devlore:access=planned
func (p *Provider) PowerShell(command string, output io.Writer) (string, error) {
	cmd := exec.CommandContext(context.Background(), "powershell", "-Command", command) //nolint:gosec // G204: command built from provider inputs
	cmd.Stdout = output
	cmd.Stderr = output
	return command, cmd.Run()
}
