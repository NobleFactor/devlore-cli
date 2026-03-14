// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// detectArch returns the architecture string (GOARCH).
func detectArch() string {
	return runtime.GOARCH
}

// runShellCommand executes a shell command (Unix).
func runShellCommand(command string, sudo bool) PlatformResult {
	var cmd *exec.Cmd
	if sudo {
		cmd = exec.CommandContext(context.Background(), "sudo", "bash", "-c", command) //nolint:gosec // G204: shell command from internal caller
	} else {
		cmd = exec.CommandContext(context.Background(), "bash", "-c", command) //nolint:gosec // G204: shell command from internal caller
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}

	return PlatformResult{
		OK:     code == 0,
		Stdout: strings.TrimSuffix(stdout.String(), "\n"),
		Stderr: strings.TrimSuffix(stderr.String(), "\n"),
		Code:   code,
	}
}
