// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"io"
	"os/exec"
)

// ShellService provides shell command execution.
type ShellService struct{}

// Shell executes a POSIX shell command.
func (s *ShellService) Shell(command string, output io.Writer) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

// PowerShell executes a PowerShell command (Windows).
func (s *ShellService) PowerShell(command string, output io.Writer) error {
	cmd := exec.Command("powershell", "-Command", command)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}
