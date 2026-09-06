// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"strings"
	"testing"
)

// runRoot executes the writ root with the given arguments and returns its combined output.
func runRoot(t *testing.T, args ...string) (string, error) {

	t.Helper()

	cmd := NewRootCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// TestReconcile_StatusIsGone pins the rename: `writ status` is not a command, and not an alias of one.
//
// Greenfield: the old name stops existing rather than being kept as a courtesy. The positive half --
// `writ reconcile --help` resolves -- is here so the test cannot pass by the root being broken.
func TestReconcile_StatusIsGone(t *testing.T) {

	out, err := runRoot(t, "status")
	if err == nil {
		t.Fatalf("`writ status` executed; it should not exist:\n%s", out)
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("`writ status` failed for the wrong reason: %v", err)
	}

	if _, err := runRoot(t, "reconcile", "--help"); err != nil {
		t.Fatalf("`writ reconcile --help` failed, so the root itself is broken: %v", err)
	}
}
