// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package shell

import (
	"fmt"
	"io"
	"os/exec"
)

// Provider provides shell command execution.
//
//devlore:plannable
type Provider struct{}

// Exec executes a POSIX shell command.
//
// Parameters:
//   - command: Shell command string to execute via sh -c
func (p *Provider) Exec(command string, output io.Writer) (string, error) {
	if command == "" {
		return "", fmt.Errorf("no command specified")
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = output
	cmd.Stderr = output
	return command, cmd.Run()
}

// PowerShell executes a PowerShell command (Windows).
//
// Parameters:
//   - command: PowerShell command string to execute
func (p *Provider) PowerShell(command string, output io.Writer) (string, error) {
	cmd := exec.Command("powershell", "-Command", command)
	cmd.Stdout = output
	cmd.Stderr = output
	return command, cmd.Run()
}
