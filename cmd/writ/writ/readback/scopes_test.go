// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package readback_test

import (
	"slices"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/readback"
)

// --- Entry.InScopes ---

// TestInScopes_NoneNamedReadsEveryEntry pins open question 4 of #926: a run that names no scope reads the whole
// record, the unscoped entries of single-source mode among them.
func TestInScopes_NoneNamedReadsEveryEntry(t *testing.T) {

	for _, scope := range []string{"home", "system", "staging", ""} {
		if entry := (readback.Entry{Scope: scope}); !entry.InScopes(nil) {
			t.Errorf("an entry of scope %q is outside a run that names no scope", scope)
		}
	}
}

// TestInScopes_NamedScopesNarrow pins the narrowing: a run that names scopes reads only their entries.
func TestInScopes_NamedScopesNarrow(t *testing.T) {

	named := []string{"home"}
	if entry := (readback.Entry{Scope: "home"}); !entry.InScopes(named) {
		t.Error("home's entry is outside a run that names home")
	}
	for _, scope := range []string{"system", ""} {
		if entry := (readback.Entry{Scope: scope}); entry.InScopes(named) {
			t.Errorf("an entry of scope %q is inside a run that names home alone", scope)
		}
	}
}

// --- CompareScopes ---

// TestCompareScopes_TheModelsOrder pins the order deploy, upgrade and decommission take the record's scopes in: the
// scopes the platform defines, in scope order; then any other, by name; then the unscoped.
func TestCompareScopes_TheModelsOrder(t *testing.T) {

	scopes := []string{"", "staging", "home", "alpha", "system"}
	slices.SortFunc(scopes, readback.CompareScopes([]string{"system", "home"}))

	if want := []string{"system", "home", "alpha", "staging", ""}; !slices.Equal(scopes, want) {
		t.Errorf("sorted = %q, want %q", scopes, want)
	}
}
