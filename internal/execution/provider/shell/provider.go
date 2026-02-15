// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package shell

import (
	"io"
	"os/exec"
)

// Provider provides shell command execution.
type Provider struct{}

// Shell executes a POSIX shell command.
func (p *Provider) Shell(command string, output io.Writer) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

// PowerShell executes a PowerShell command (Windows).
func (p *Provider) PowerShell(command string, output io.Writer) error {
	cmd := exec.Command("powershell", "-Command", command)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}
