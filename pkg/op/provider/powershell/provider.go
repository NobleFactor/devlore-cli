// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package powershell provides PowerShell 7+ command execution actions for the operation graph.
//
// Commands are run via `pwsh -NoLogo -NoProfile -Command <command>`. PowerShell 7+ is cross-platform — `pwsh`
// runs on Windows, macOS, and Linux — so this provider is platform-agnostic and requires only that `pwsh` is on
// PATH. The legacy Windows-only `powershell.exe` (PowerShell 5.x) is not supported.
//
// Returns a [Result] with the original command, both captured streams, and the subprocess exit code so the value
// can be Emitted to a [result.Sink] (JSON, YAML, or CSV) and returned to the caller in one shape.
package powershell

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides PowerShell 7+ command execution.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

// Result is the structured outcome of a PowerShell command execution.
//
// PowerShell has six streams (Output 1, Error 2, Warning 3, Verbose 4, Debug 5, Information 6). At the OS process
// level these collapse to two: stream 1 → Stdout; streams 2-6 → Stderr (by default). Callers who need finer
// control redirect inside the command itself using PowerShell operators (`*>&1`, `4>&1`, etc.).
//
// pwsh defaults to UTF-8 across all platforms, so Stdout and Stderr are valid UTF-8 strings without transcoding.
type Result struct {
	Command  string `json:"command"  yaml:"command"  csv:"command"` // command string passed to pwsh -Command
	ExitCode int    `json:"exit"     yaml:"exit"     csv:"exit"`    // subprocess exit code; 0 on success
	Stderr   string `json:"stderr"   yaml:"stderr"   csv:"stderr"`  // streams 2-6 (Error, Warning, Verbose, Debug, Information) by default
	Stdout   string `json:"stdout"   yaml:"stdout"   csv:"stdout"`  // stream 1 (Output) by default
}

func NewProvider(ctx *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// Exec executes a PowerShell command via `pwsh` (PowerShell 7+) and returns the structured execution result.
//
// The command is invoked as:
//
//	pwsh -NoLogo -NoProfile -Command <command>
//
//   - `-NoLogo`     suppresses the startup banner.
//   - `-NoProfile`  prevents sourcing `$PROFILE`, so user-specific profile output never contaminates the captured
//     streams.
//
// Parameters:
//   - command: PowerShell command string passed to `pwsh -Command`.
//
// Returns:
//   - *Result: command, both captured streams, and the exit code; nil only when command is empty.
//   - error:   any error from cmd.Run (the result is still returned with whatever was captured).
func (p *Provider) Exec(command string) (*Result, error) {
	if command == "" {
		return nil, fmt.Errorf("no command specified")
	}
	cmd := exec.Command("pwsh", "-NoLogo", "-NoProfile", "-Command", command) //nolint:gosec // G204: command built from provider inputs
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	return &Result{
		Command:  command,
		ExitCode: exitCode,
		Stderr:   stderr.String(),
		Stdout:   stdout.String(),
	}, err
}