// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"fmt"
	"os"
	"time"
)

// refreshTTL is the staleness threshold for the automatic index refresh.
//
// A leaf whose local index is older than this refreshes before an index-consuming operation. A single default knob
// for now; promote to a per-leaf value if managers need to diverge.
const refreshTTL = 24 * time.Hour

// unknownIndexAge is the age reported for an index that cannot be stat'd (never built, or an unreadable path).
//
// Well past [refreshTTL], so the gate treats it as stale and refreshes.
//
//nolint:unused // read by the Linux-tagged leaves in linux_managers_linux.go; invisible to non-Linux analyses.
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
//
//nolint:unused // called by the Linux-tagged leaves in linux_managers_linux.go; invisible to non-Linux analyses.
func indexAgeOf(path string) time.Duration {

	info, err := os.Stat(path)
	if err != nil {
		return unknownIndexAge
	}

	return time.Since(info.ModTime())
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
//   - `run`: runs the operation over the native tokens and returns its raw [Result].
//   - `satisfied`: reports whether an observed post-version satisfies the verb's intent (present / absent).
//
// Returns:
//   - `[]Receipt`: one receipt per package, carrying the pre/post versions and any error.
//   - `error`: the first failing receipt's error, or nil when all packages reached the requested state.
func bracket(packages []PURL, token func(p PURL) string, version func(name string) string, run func(names []string) Result, satisfied func(post string) bool) ([]Receipt, error) {

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
			err = fmt.Errorf("platform: %s/%s did not reach the requested state (post=%q): %s",
				p.Type, p.Name, post, result.Stderr)
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
