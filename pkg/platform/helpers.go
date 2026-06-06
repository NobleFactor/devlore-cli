// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// commandTimeout bounds every shell-out so a wedged command — a stuck mirror, a tool that prompts on /dev/tty — can
// never hang the run indefinitely. It is a generous backstop, not a per-operation deadline (large installs are
// legitimate). TODO: make it configurable via the run environment.
const commandTimeout = 30 * time.Minute

// confirmReplies is how many "y" answers are piped to a command's stdin — a backstop that auto-confirms any prompt a
// tool raises despite its non-interactive flags, so an op the plan requested proceeds instead of aborting on EOF.
// Far more than any command asks; the stream ends in EOF afterward. apt's dangerous conffile prompt is handled
// separately and safely by DEBIAN_FRONTEND (keep-old default), not by this stream.
const confirmReplies = 4096

// refreshTTL is the staleness threshold for the automatic index refresh: a leaf whose local index is older than this
// refreshes before an index-consuming operation. A single default knob for now; promote to a per-leaf value if
// managers need to diverge.
const refreshTTL = 24 * time.Hour

// unknownIndexAge is the age reported for an index that cannot be stat'd (never built, or an unreadable path): well
// past [refreshTTL], so the gate treats it as stale and refreshes.
const unknownIndexAge = 365 * 24 * time.Hour

// indexAgeOf returns how long ago `path` was last modified, or [unknownIndexAge] when it cannot be stat'd.
//
// Leaves whose index is a file or directory touched by their refresh command (apt's lists, pacman's sync db) report
// staleness through this; the automatic gate compares the result against [refreshTTL].
//
// Parameters:
//   - `path`: the index file or directory whose mtime marks the last refresh.
//
// Returns:
//   - `time.Duration`: the age since last modification, or [unknownIndexAge] when the path is unreadable.
func indexAgeOf(path string) time.Duration {

	info, err := os.Stat(path)
	if err != nil {
		return unknownIndexAge
	}

	return time.Since(info.ModTime())
}

// runShellCommand executes a shell command via bash, optionally with sudo, capturing the result.
//
// It captures stdout, stderr, and the exit code into a [PlatformResult]. Used by every Linux/Darwin
// [PackageManager] and [ServiceManager] mutator. The command string is passed to `bash -c` directly; callers are
// responsible for safe quoting.
//
// Hang safety, in layers: the call is bounded by [commandTimeout]; an endless `y\n` stream on stdin auto-confirms
// any prompt a tool raises despite its non-interactive flags — a backstop for the ones port's `-N` misses — so the
// op proceeds rather than blocking or aborting on EOF; sudo runs with `-n`, so it fails fast instead of prompting
// for a password on the tty (credential handling is the elevation model's job — see TODO); and
// `DEBIAN_FRONTEND=noninteractive` keeps apt quiet (and safely keeps modified config files). Callers still pass
// per-tool non-interactive flags (apt `-y`, pacman `--noconfirm`, port `-N`).
//
// It is a package var, not a plain func, so tests can substitute a recording fake.
var runShellCommand = func(command string, sudo bool) PlatformResult {

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if sudo {
		// TODO(elevation): centralize this. Route privileged execution through the ElevationOffer/Elevator — one
		// credential-cached, policy-governed, audited sudo session — instead of every command inlining `sudo -n`.
		cmd = exec.CommandContext(ctx, "sudo", "-n", "bash", "-c", command) //nolint:gosec // G204: shell command from internal caller
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-c", command) //nolint:gosec // G204: shell command from internal caller
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
		stderr.WriteString(fmt.Sprintf("\ncommand timed out after %s", commandTimeout))
	}

	return PlatformResult{
		OK:     code == 0,
		Stdout: strings.TrimSuffix(stdout.String(), "\n"),
		Stderr: strings.TrimSuffix(stderr.String(), "\n"),
		Code:   code,
	}
}

// bracket runs a best-effort batch package operation and returns one [Receipt] per package.
//
// It is the shared mechanism behind every leaf's Install / Remove / Upgrade: pre-query each package's installed
// version, run the (idempotent) command once over the whole slice, then re-query — so success and resulting state
// are derived from the observed post-state, never from the command's exit code. A package's [Receipt.Err] is set
// when `satisfied` rejects the post-state (e.g. still absent after an install). The call is best-effort: every
// package gets a receipt regardless of failures, and the aggregate error is the first failing receipt's error.
//
// Parameters:
//   - `packages`: the packages to act on; each contributes one receipt in input order.
//   - `token`: derives a package's native install token from its [PURL] (usually the name; winget adds its publisher).
//   - `version`: queries a package's installed version by its native token ("" when absent).
//   - `run`: runs the operation over the native tokens and returns its raw [PlatformResult].
//   - `satisfied`: reports whether an observed post-version satisfies the verb's intent (present / absent).
//
// Returns:
//   - `[]Receipt`: one receipt per package, carrying the pre/post versions and any error.
//   - `error`: the first failing receipt's error, or nil when all packages reached the requested state.
func bracket(packages []PURL, token func(p PURL) string, version func(name string) string, run func(names []string) PlatformResult, satisfied func(post string) bool) ([]Receipt, error) {

	names := make([]string, len(packages))
	prior := make([]string, len(packages))

	for i, p := range packages {
		names[i] = token(p)
		prior[i] = version(names[i])
	}

	result := run(names)

	receipts := make([]Receipt, len(packages))

	var firstErr error

	for i, p := range packages {
		post := version(names[i])

		var err error
		if !satisfied(post) {
			err = fmt.Errorf("platform: %s/%s did not reach the requested state (post=%q): %s", p.Type, p.Name, post, result.Stderr)
		}

		receipts[i] = Receipt{Purl: p, PriorVersion: prior[i], Version: post, Err: err}

		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return receipts, firstErr
}

// present reports whether a post-operation version indicates the package is installed (the Install / Upgrade goal).
func present(post string) bool { return post != "" }

// absent reports whether a post-operation version indicates the package is gone (the Remove goal).
func absent(post string) bool { return post == "" }

// tagManager stamps `manager` onto every [SearchResult] so a federated search self-identifies each hit's source.
//
// Parameters:
//   - `results`: the raw search hits from a leaf's index query.
//   - `manager`: the leaf's purl type (e.g. "deb", "brew").
//
// Returns:
//   - `[]SearchResult`: `results` with `Manager` set on each element.
func tagManager(results []SearchResult, manager string) []SearchResult {

	for i := range results {
		results[i].Manager = manager
	}

	return results
}
