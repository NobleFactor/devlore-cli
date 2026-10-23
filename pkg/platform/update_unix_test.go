// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build unix

package platform

import (
	"context"
	"testing"
)

// captureRefresh records the argv and the account a leaf refresh asks for.
//
// It installs a recording [CommandRunner] on the leaf and invokes `refresh`, so it asserts a leaf's refresh wiring
// (the argv and its account) without shelling out or needing root.
//
// It carries the unix constraint because its callers are the darwin- and linux-tagged refresh tests.
//
// Parameters:
//   - `t`: the test.
//   - `leaf`: the leaf under test, which receives the recording runner.
//   - `refresh`: the leaf refresh method value to invoke.
//
// Returns:
//   - `[]string`: the argv the refresh issued.
//   - `string`: the account it asked for.
func captureRefresh(t *testing.T, leaf runnerSetter, refresh func() Result) (argv []string, account string) {

	t.Helper()

	leaf.setRunner(func(_ context.Context, request CommandRequest) Result {
		argv, account = request.Argv, request.Account
		return Result{OK: true}
	})

	refresh()

	return argv, account
}
