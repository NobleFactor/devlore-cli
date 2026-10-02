// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package decommission

import (
	"slices"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// --- helpers ---

// selectedTargets returns the targets of entries, sorted.
//
// Parameters:
//   - `t`: the test harness.
//   - `entries`: the entries.
//
// Returns:
//   - `[]string`: their targets, in order.
func selectedTargets(t *testing.T, entries []readback.Entry) []string {

	t.Helper()

	targets := make([]string, 0, len(entries))
	for i := range entries {
		targets = append(targets, entries[i].Target)
	}
	slices.Sort(targets)
	return targets
}

// --- selectEntries ---

// TestSelectEntries_NarrowsToTheNamedScopes pins #926 for decommission: the named projects' entries, narrowed to the
// named scopes when there are any, and every scope's when there are none.
func TestSelectEntries_NarrowsToTheNamedScopes(t *testing.T) {

	inventory := &readback.Inventory{Entries: map[string]readback.Entry{
		"/home/a":   {Target: "/home/a", Action: string(file.Link), Scope: "home", Project: "common"},
		"/system/b": {Target: "/system/b", Action: string(file.Link), Scope: "system", Project: "common"},
		"/home/c":   {Target: "/home/c", Action: string(file.Link), Scope: "home", Project: "other"},
	}}
	common := []string{"common"}

	got := selectedTargets(t, selectEntries(inventory, common, []string{"home"}))
	if !slices.Equal(got, []string{"/home/a"}) {
		t.Errorf("common in --scope home selected %q, want its home entry alone", got)
	}
	got = selectedTargets(t, selectEntries(inventory, common, nil))
	if !slices.Equal(got, []string{"/home/a", "/system/b"}) {
		t.Errorf("common with no --scope selected %q, want every scope's entry", got)
	}
}

// --- scopesInOrder ---

// TestScopesInOrder_TheModelsOrder pins removal order (#926): the scopes the platform defines, in scope order; then
// any other the record holds, by name; then the unscoped.
func TestScopesInOrder_TheModelsOrder(t *testing.T) {

	byScope := map[string][]readback.Entry{"": nil, "home": nil, "staging": nil, "system": nil}

	got := scopesInOrder(byScope, []string{"system", "home"})
	if want := []string{"system", "home", "staging", ""}; !slices.Equal(got, want) {
		t.Errorf("scopesInOrder = %q, want %q", got, want)
	}
}
