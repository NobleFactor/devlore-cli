// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package process

import (
	"errors"
	"fmt"
	"os/exec"
)

// wrapExitError formats a non-zero subprocess exit into a clean graph-control signal carrying only the command path
// and exit code.
//
// Diagnostic context (full stderr/stdout) lives elsewhere — in the streamed status narration during Run/Capture, and
// in the structured Result that capture-and-parse callers return. The error itself stays minimal so it can be matched
// and routed by graph-runtime control flow without parsing prose.
//
// Parameters:
//   - cmd: the exec.Cmd that was launched (used for its Path).
//   - err: the error returned by cmd.Run.
//
// Returns:
//   - error: a formatted wrapping error.
func wrapExitError(cmd *exec.Cmd, err error) error {

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("process: %s exited with code %d", cmd.Path, exitErr.ExitCode())
	}
	return fmt.Errorf("process: %s: %w", cmd.Path, err)
}
