// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package upgrade

import (
	"slices"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// --- helpers ---

// copiedTargets returns the targets of entries, sorted.
//
// Parameters:
//   - `t`: the test harness.
//   - `entries`: the entries.
//
// Returns:
//   - `[]string`: their targets, in order.
func copiedTargets(t *testing.T, entries []readback.Entry) []string {

	t.Helper()

	targets := make([]string, 0, len(entries))
	for i := range entries {
		targets = append(targets, entries[i].Target)
	}
	slices.Sort(targets)
	return targets
}

// --- selectCopied ---

// TestSelectCopied_NarrowsToTheNamedScopes pins #926 for upgrade: named scopes narrow the copied inventory, none
// named reads every copied entry, and a link is never upgrade's.
func TestSelectCopied_NarrowsToTheNamedScopes(t *testing.T) {

	inventory := &readback.Inventory{Entries: map[string]readback.Entry{
		"/home/a":   {Target: "/home/a", Action: string(file.Copy), Scope: "home", Project: "common"},
		"/system/b": {Target: "/system/b", Action: string(file.Copy), Scope: "system", Project: "common"},
		"/home/c":   {Target: "/home/c", Action: string(file.Link), Scope: "home", Project: "common"},
	}}

	got := copiedTargets(t, selectCopied(inventory, nil, []string{"home"}))
	if !slices.Equal(got, []string{"/home/a"}) {
		t.Errorf("--scope home selected %q, want home's copy alone", got)
	}
	got = copiedTargets(t, selectCopied(inventory, nil, nil))
	if !slices.Equal(got, []string{"/home/a", "/system/b"}) {
		t.Errorf("no --scope selected %q, want every copy", got)
	}
}
