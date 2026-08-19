// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

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

// runCommand executes a command as an argv vector, optionally under sudo, capturing the result.
//
// The vector is handed to the kernel as a vector: no shell parses it, so nothing needs quoting and no argument
// can inject. A package name containing a semicolon is a package name containing a semicolon, not a second
// command (issue #561). Everything this package runs is expressible this way -- a format string quoted only to
// hide it from a shell is a plain argument, and a pipe whose right-hand side is a text utility is a Go
// expression.
//
// Hang safety, in layers: the call is bounded by [commandTimeout]; an endless `y\n` stream on stdin
// auto-confirms any prompt a tool raises despite its non-interactive flags, so the op proceeds rather than
// blocking or aborting on EOF; sudo runs with `-n`, so it fails fast instead of prompting for a password on the
// tty (credential handling is the elevation model's job -- see TODO); and `DEBIAN_FRONTEND=noninteractive`
// keeps apt quiet, and safely keeps modified config files. Callers still pass per-tool non-interactive flags
// (apt `-y`, pacman `--noconfirm`, port `-N`).
//
// It is a package var, not a plain func, so tests can substitute a recording fake. It carries the unix
// constraint with its two knobs above because every consumer does: the Linux and Darwin manager leaves run
// through it, while the Windows leaves drive winget through their own path -- under a windows analysis all
// three were dead code (#373 phase 1b).
//
// Parameters:
//   - `argv`: the command and its arguments; `argv[0]` is the executable.
//   - `sudo`: whether to run under `sudo -n`.
//
// Returns:
//   - `Result`: the captured stdout, stderr, and exit code.
var runCommand = func(argv []string, sudo bool) Result {

	if len(argv) == 0 {
		return Result{Code: -1, Stderr: "platform: no command given"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	name, args := argv[0], argv[1:]

	if sudo {
		// TODO(elevation): centralize this. Route privileged execution through the ElevationOffer/Elevator -- one
		// credential-cached, policy-governed, audited sudo session -- instead of every command inlining `sudo -n`.
		name, args = "sudo", append([]string{"-n"}, argv...)
	}

	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // G204: argv from internal callers, never parsed

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
