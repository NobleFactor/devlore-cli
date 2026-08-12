// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build unix

package platform

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// commandTimeout bounds every shell-out.
//
// A wedged command — a stuck mirror, a tool that prompts on /dev/tty — can never hang the run indefinitely. It is a
// generous backstop, not a per-operation deadline (large installs are legitimate). TODO: make it configurable via the
// run environment.
const commandTimeout = 30 * time.Minute

// confirmReplies is how many "y" answers are piped to a command's stdin.
//
// A backstop that auto-confirms any prompt a tool raises despite its non-interactive flags, so an op the plan
// requested proceeds instead of aborting on EOF. Far more than any command asks; the stream ends in EOF afterward.
// apt's dangerous conffile prompt is handled separately and safely by DEBIAN_FRONTEND (keep-old default), not by
// this stream.
const confirmReplies = 4096

// runShellCommand executes a shell command via bash, optionally with sudo, capturing the result.
//
// It captures stdout, stderr, and the exit code into a [Result]. Used by every Linux/Darwin [PackageManager] and
// [ServiceManager] mutator. The command string is passed to `bash -c` directly; callers are responsible for safe
// quoting.
//
// Hang safety, in layers: the call is bounded by [commandTimeout]; an endless `y\n` stream on stdin auto-confirms
// any prompt a tool raises despite its non-interactive flags — a backstop for the ones port's `-N` misses — so the
// op proceeds rather than blocking or aborting on EOF; sudo runs with `-n`, so it fails fast instead of prompting
// for a password on the tty (credential handling is the elevation model's job — see TODO); and
// `DEBIAN_FRONTEND=noninteractive` keeps apt quiet (and safely keeps modified config files). Callers still pass
// per-tool non-interactive flags (apt `-y`, pacman `--noconfirm`, port `-N`).
//
// It is a package var, not a plain func, so tests can substitute a recording fake. It carries the unix constraint
// with its two knobs above because every consumer does: the Linux and Darwin manager leaves shell out through bash,
// while the Windows leaves drive winget through their own path — under a windows analysis all three were dead code
// (#373 phase 1b).
//
// Parameters:
//   - `command`: the shell command, passed to `bash -c` verbatim.
//   - `sudo`: whether to run under `sudo -n`.
//
// Returns:
//   - `Result`: the captured stdout, stderr, and exit code.
var runShellCommand = func(command string, sudo bool) Result {

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if sudo {
		// TODO(elevation): centralize this. Route privileged execution through the ElevationOffer/Elevator — one
		// credential-cached, policy-governed, audited sudo session — instead of every command inlining `sudo -n`.
		cmd = exec.CommandContext(ctx, "sudo", "-n", "bash", "-c", command) //nolint:gosec // G204: internal caller
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-c", command) //nolint:gosec // G204: internal caller
	}

	cmd.Env = append(cmd.Environ(), "DEBIAN_FRONTEND=noninteractive")
	cmd.Stdin = strings.NewReader(strings.Repeat("y\n", confirmReplies))

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

	if ctx.Err() == context.DeadlineExceeded {
		fmt.Fprintf(&stderr, "\ncommand timed out after %s", commandTimeout)
	}

	return Result{
		OK:     code == 0,
		Stdout: strings.TrimSuffix(stdout.String(), "\n"),
		Stderr: strings.TrimSuffix(stderr.String(), "\n"),
		Code:   code,
	}
}
