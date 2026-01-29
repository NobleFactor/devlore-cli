// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package shell provides audited command execution for lore.
//
// Commands are executed through bash, so pipes, redirects, loops, and
// variable expansion work exactly as expected. Go orchestrates and audits;
// bash does the work.
//
//	sh := shell.New()
//	sh.Run("brew install kubectl")
//	sh.Run("kubectl version --client")
//	sh.Run("echo $PATH | tr ':' '\\n' | head -5")
package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// Result captures everything about a command execution.
type Result struct {
	Command  string        `json:"command"`
	Stdout   string        `json:"stdout,omitempty"`
	Stderr   string        `json:"stderr,omitempty"`
	ExitCode int           `json:"exit_code"`
	Start    time.Time     `json:"start"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
}

// OK returns true if the command exited with code 0.
func (r *Result) OK() bool {
	return r.ExitCode == 0 && r.Error == ""
}

// Failed returns true if the command failed.
func (r *Result) Failed() bool {
	return !r.OK()
}

// String returns a human-readable summary.
func (r *Result) String() string {
	if r.OK() {
		return fmt.Sprintf("[ok] %s (%s)", r.Command, r.Duration.Round(time.Millisecond))
	}
	if r.Error != "" {
		return fmt.Sprintf("[error] %s: %s", r.Command, r.Error)
	}
	return fmt.Sprintf("[exit %d] %s", r.ExitCode, r.Command)
}

// JSON returns the result as indented JSON.
func (r *Result) JSON() string {
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b)
}

// Session is an audited shell session.
type Session struct {
	dir      string
	env      map[string]string
	audit    io.Writer
	history  []*Result
	shellCmd string
}

// New creates a new shell session.
func New() *Session {
	return &Session{
		env:      make(map[string]string),
		history:  make([]*Result, 0),
		shellCmd: "bash",
	}
}

// Dir sets the working directory for subsequent commands.
func (s *Session) Dir(path string) *Session {
	s.dir = path
	return s
}

// Set sets an environment variable for subsequent commands.
func (s *Session) Set(key, value string) *Session {
	s.env[key] = value
	return s
}

// Audit sets the audit log writer. Each command result is written as JSON.
func (s *Session) Audit(w io.Writer) *Session {
	s.audit = w
	return s
}

// History returns all commands run in this session.
func (s *Session) History() []*Result {
	return s.history
}

// With returns a copy of the session with additional env vars.
// Useful for one-off commands that need extra context.
func (s *Session) With(key, value string) *Session {
	clone := &Session{
		dir:      s.dir,
		env:      make(map[string]string),
		audit:    s.audit,
		history:  s.history, // shared history
		shellCmd: s.shellCmd,
	}
	for k, v := range s.env {
		clone.env[k] = v
	}
	clone.env[key] = value
	return clone
}

// Run executes a command through bash.
// Pipes, redirects, loops, and variable expansion work as expected.
func (s *Session) Run(command string) *Result {
	return s.RunContext(context.Background(), command)
}

// RunContext executes a command with a context for cancellation/timeout.
func (s *Session) RunContext(ctx context.Context, command string) *Result {
	result := &Result{
		Command: command,
		Start:   time.Now(),
	}

	cmd := exec.CommandContext(ctx, s.shellCmd, "-c", command)

	if s.dir != "" {
		cmd.Dir = s.dir
	}

	// Build environment: OS env + session env
	if len(s.env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range s.env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result.Duration = time.Since(result.Start)
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.Error = err.Error()
			result.ExitCode = -1
		}
	}

	s.history = append(s.history, result)

	if s.audit != nil {
		_, _ = fmt.Fprintln(s.audit, result.JSON())
	}

	return result
}

// Must runs a command and panics if it fails.
func (s *Session) Must(command string) *Result {
	result := s.Run(command)
	if result.Failed() {
		panic(fmt.Sprintf("command failed: %s\n%s", result.String(), result.Stderr))
	}
	return result
}

// Script runs multiple commands in sequence, stopping on first failure.
// Returns the failing result, or the last successful result.
func (s *Session) Script(commands ...string) *Result {
	var last *Result
	for _, cmd := range commands {
		last = s.Run(cmd)
		if last.Failed() {
			return last
		}
	}
	return last
}
