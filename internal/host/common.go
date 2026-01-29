// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package host

import (
	"os/exec"
	"runtime"
	"strings"
)

// detectArch returns the architecture string (GOARCH).
func detectArch() string {
	return runtime.GOARCH
}

// runShellCommand executes a shell command (Unix).
// Used by darwin.go and linux.go.
func runShellCommand(command string, sudo bool) Result {
	var cmd *exec.Cmd
	if sudo {
		cmd = exec.Command("sudo", "bash", "-c", command)
	} else {
		cmd = exec.Command("bash", "-c", command)
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}

	return Result{
		OK:     code == 0,
		Stdout: strings.TrimSuffix(stdout.String(), "\n"),
		Stderr: strings.TrimSuffix(stderr.String(), "\n"),
		Code:   code,
	}
}
