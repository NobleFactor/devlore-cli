// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build unix

package platform

import "testing"

// captureRefresh records the command and elevation flag a leaf refresh issues.
//
// It swaps in a recording [runShellCommand], invokes `refresh`, and restores the real command runner on return — so
// it asserts a leaf's refresh wiring (the command and its elevation flag) without shelling out or needing root.
//
// It carries the unix constraint because [runShellCommand] does, and because its callers are the darwin- and
// linux-tagged refresh tests.
//
// Parameters:
//   - `t`: the test.
//   - `refresh`: the leaf refresh method value to invoke.
//
// Returns:
//   - `string`: the command the refresh issued.
//   - `bool`: the sudo (elevation) flag it requested.
func captureRefresh(t *testing.T, refresh func() Result) (string, bool) {

	t.Helper()

	var (
		gotCmd  string
		gotSudo bool
	)

	original := runShellCommand
	runShellCommand = func(command string, sudo bool) Result {
		gotCmd, gotSudo = command, sudo
		return Result{OK: true}
	}
	defer func() { runShellCommand = original }()

	refresh()

	return gotCmd, gotSudo
}
