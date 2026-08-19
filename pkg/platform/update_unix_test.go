// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build unix

package platform

import "testing"

// captureRefresh records the argv and elevation flag a leaf refresh issues.
//
// It swaps in a recording [runCommand], invokes `refresh`, and restores the real command runner on return — so it
// asserts a leaf's refresh wiring (the argv and its elevation flag) without shelling out or needing root.
//
// It carries the unix constraint because [runCommand] does, and because its callers are the darwin- and
// linux-tagged refresh tests.
//
// Parameters:
//   - `t`: the test.
//   - `refresh`: the leaf refresh method value to invoke.
//
// Returns:
//   - `[]string`: the argv the refresh issued.
//   - `bool`: the sudo (elevation) flag it requested.
func captureRefresh(t *testing.T, refresh func() Result) ([]string, bool) {

	t.Helper()

	var (
		gotArgv []string
		gotSudo bool
	)

	original := runCommand
	runCommand = func(argv []string, sudo bool) Result {
		gotArgv, gotSudo = argv, sudo
		return Result{OK: true}
	}
	defer func() { runCommand = original }()

	refresh()

	return gotArgv, gotSudo
}
