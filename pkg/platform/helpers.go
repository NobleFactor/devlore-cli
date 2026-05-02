// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// runShellCommand executes a shell command via bash, optionally with sudo, and captures stdout/stderr/exit
// code into a [PlatformResult].
//
// Used by every Linux/Darwin [PackageManager] and [ServiceManager] mutator. The command string is passed to
// `bash -c` directly; callers are responsible for safe quoting.
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
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
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
