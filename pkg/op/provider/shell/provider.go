// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

// Package shell provides POSIX shell command execution actions for the operation graph.
//
// Commands are run via `sh -c <command>` — POSIX shell semantics, available on Linux, macOS, and any platform with
// `sh` on PATH. For PowerShell 7+ execution see [pkg/op/provider/powershell].
//
// Returns a [Result] with the original command, both captured streams, and the subprocess exit code so the value
// can be Emitted to a [result.Sink] (JSON, YAML, or CSV) and returned to the caller in one shape.
package shell

import (
	"fmt"
	"io"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides shell command execution.
type Provider struct {
	op.ProviderBase
}

// NewProvider constructs a POSIX shell Provider bound to the given runtime environment.
//
// Parameters:
//   - `runtimeEnvironment`: the runtime environment that supplies the subprocess context, status sink, and result sink.
//
// Returns:
//   - `*Provider`: the initialized provider.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// region EXPORTED METHODS

// region Behaviors

// Exec runs a POSIX shell command through the environment's broker and returns the structured execution result.
//
// The command is a tier-2 script of 6.3, dialect `sh`, run as the caller: the broker writes the body to a file and
// runs `sh file`, so nothing here hands a string to `-c`. The command's world is the run's root (#584 phase 2) when
// the environment has one.
//
// Parameters:
//   - `command`: the shell script to run.
//
// Returns:
//   - `*Result`: command, both captured streams, and the exit code; nil only when command is empty or the broker
//     refused the exec.
//   - `error`: non-nil on a non-zero exit (the result is still returned with whatever was captured), on a refusal,
//     or when the exec could not be launched.
func (p *Provider) Exec(command string) (*Result, error) {

	if command == "" {
		return nil, fmt.Errorf("no command specified")
	}

	request := op.ExecRequest{
		Run:     shellRun,
		Unit:    string(Exec),
		Account: op.AccountCaller,
		Script:  &op.Script{Dialect: "sh", Body: command},
	}
	if p.RuntimeEnvironment().HasRoot() {
		request.Cwd = p.RuntimeEnvironment().Root().Name()
	}

	var narrator io.Writer
	if p.RuntimeEnvironment().Status != nil {
		narrator = p.RuntimeEnvironment().Status
	}

	outcome, err := op.Run(p.RuntimeEnvironment().Context, p.RuntimeEnvironment().Broker, request, op.WithNarrator(narrator))
	if err != nil {
		return nil, err
	}

	result := &Result{
		Command:  command,
		ExitCode: outcome.ExitStatus,
		Stderr:   outcome.Stderr(),
		Stdout:   string(outcome.Output),
	}
	switch {
	case outcome.Error != "":
		return result, fmt.Errorf("shell: %s", outcome.Error)
	case outcome.ExitStatus != 0:
		return result, fmt.Errorf("shell: exit status %d", outcome.ExitStatus)
	default:
		return result, nil
	}
}

// endregion

// endregion

// shellRun is the run id the shell provider's execs carry until the executor gives runs ids of their own.
const shellRun op.RunID = "shell"

// region SUPPORTING TYPES

// Result is the structured outcome of a shell command execution.
//
// Carries the original command, both captured streams, and the subprocess exit code so the value can be Emitted to a
// [result.Sink] (JSON, YAML, or CSV) and returned to the caller in one shape.
type Result struct {

	// Command is the command string passed to the shell.
	Command string `json:"command" yaml:"command" csv:"command"`

	// ExitCode is the subprocess exit code; 0 on success.
	ExitCode int `json:"exit" yaml:"exit" csv:"exit"`

	// Stderr holds the captured stderr bytes as a string.
	Stderr string `json:"stderr" yaml:"stderr" csv:"stderr"`

	// Stdout holds the captured stdout bytes as a string.
	Stdout string `json:"stdout" yaml:"stdout" csv:"stdout"`
}

// endregion
