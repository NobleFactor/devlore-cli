// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package shell provides POSIX shell command execution actions for the operation graph.
//
// Commands are run via `sh -c <command>` — POSIX shell semantics, available on Linux, macOS, and any platform with
// `sh` on PATH. For PowerShell 7+ execution see [pkg/op/provider/powershell].
//
// Returns a [Result] with the original command, both captured streams, and the subprocess exit code so the value
// can be Emitted to a [result.Sink] (JSON, YAML, or CSV) and returned to the caller in one shape.
package shell

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides shell command execution.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

// Result is the structured outcome of a shell command execution.
//
// Carries the original command, both captured streams, and the subprocess exit code so the value can be Emitted to a
// [result.Sink] (JSON, YAML, or CSV) and returned to the caller in one shape.
type Result struct {
	Command  string `json:"command"  yaml:"command"  csv:"command"` // command string passed to the shell
	ExitCode int    `json:"exit"     yaml:"exit"     csv:"exit"`    // subprocess exit code; 0 on success
	Stderr   string `json:"stderr"   yaml:"stderr"   csv:"stderr"`  // captured stderr bytes as a string
	Stdout   string `json:"stdout"   yaml:"stdout"   csv:"stdout"`  // captured stdout bytes as a string
}

// NewProvider constructs a POSIX shell Provider bound to the given runtime environment.
//
// Parameters:
//   - ctx: the runtime environment that supplies the subprocess context, status sink, and result sink.
//
// Returns:
//   - *Provider: the initialized provider.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// Exec executes a POSIX shell command and returns the structured execution result.
//
// Parameters:
//   - command: Shell command string to execute via sh -c.
//
// Returns:
//   - *Result: command, both captured streams, and the exit code; nil only when command is empty.
//   - error:   any error from cmd.Run (the result is still returned with whatever was captured).
func (p *Provider) Exec(command string) (*Result, error) {
	if command == "" {
		return nil, fmt.Errorf("no command specified")
	}
	cmd := exec.CommandContext(p.RuntimeEnvironment().Context, "sh", "-c", command) //nolint:gosec // G204: command built from provider inputs
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

