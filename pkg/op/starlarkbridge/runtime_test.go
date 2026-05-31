// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlarkbridge

import (
	"sort"
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

// region Test fixtures

// stubHasAttrs is a minimal [starlark.HasAttrs] whose attribute set is fixed at construction, used to verify
// filteredReceiver narrowing without standing up a full runtime environment.
type stubHasAttrs struct {
	attrs map[string]starlark.Value
}

// Attr returns the fixed attribute, or (nil, nil) — starlark's "no such attribute" — when absent.
func (s *stubHasAttrs) Attr(name string) (starlark.Value, error) { return s.attrs[name], nil }

// AttrNames returns the fixed attribute names in sorted order.
func (s *stubHasAttrs) AttrNames() []string {

	names := make([]string, 0, len(s.attrs))
	for name := range s.attrs {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

func (s *stubHasAttrs) String() string        { return "stub" }
func (s *stubHasAttrs) Type() string          { return "stub" }
func (s *stubHasAttrs) Freeze()               {}
func (s *stubHasAttrs) Truth() starlark.Bool  { return starlark.True }
func (s *stubHasAttrs) Hash() (uint32, error) { return 0, nil }

// endregion

// region Tests

// TestFilteredReceiver_Attr verifies a denied name errors, a retained name delegates, and an absent name passes
// through the "no such attribute" signal unchanged.
func TestFilteredReceiver_Attr(t *testing.T) {

	inner := &stubHasAttrs{attrs: map[string]starlark.Value{
		"keep":   starlark.String("kept"),
		"deny":   starlark.String("denied"),
		"deny_2": starlark.String("also-denied"),
	}}

	receiver := &filteredReceiver{
		HasAttrs: inner,
		global:   "plan",
		denied:   map[string]bool{"deny": true, "deny_2": true},
	}

	t.Run("retained name delegates", func(t *testing.T) {

		value, err := receiver.Attr("keep")
		if err != nil {
			t.Fatalf("Attr(keep): unexpected error: %v", err)
		}

		if value != starlark.String("kept") {
			t.Fatalf("Attr(keep) = %v, want %q", value, "kept")
		}
	})

	t.Run("denied name errors with global-qualified message", func(t *testing.T) {

		_, err := receiver.Attr("deny")
		if err == nil {
			t.Fatal("Attr(deny): expected error, got nil")
		}

		if want := "plan.deny is not available in this runtime"; !strings.Contains(err.Error(), want) {
			t.Fatalf("Attr(deny) error = %q, want it to contain %q", err.Error(), want)
		}
	})

	t.Run("absent name delegates the no-such-attribute signal", func(t *testing.T) {

		value, err := receiver.Attr("missing")
		if err != nil || value != nil {
			t.Fatalf("Attr(missing) = (%v, %v), want (nil, nil)", value, err)
		}
	})
}

// TestFilteredReceiver_AttrNames verifies the denied names are dropped and the rest retained.
func TestFilteredReceiver_AttrNames(t *testing.T) {

	inner := &stubHasAttrs{attrs: map[string]starlark.Value{
		"keep":   starlark.String("kept"),
		"deny":   starlark.String("denied"),
		"deny_2": starlark.String("also-denied"),
	}}

	receiver := &filteredReceiver{
		HasAttrs: inner,
		global:   "plan",
		denied:   map[string]bool{"deny": true, "deny_2": true},
	}

	got := receiver.AttrNames()
	want := []string{"keep"}

	if !equalStrings(got, want) {
		t.Fatalf("AttrNames() = %v, want %v", got, want)
	}
}

// TestDenyAttributes_RecordsDenials verifies the option unions names across calls and keeps globals separate.
func TestDenyAttributes_RecordsDenials(t *testing.T) {

	rt := &Runtime{}

	DenyAttributes("plan", "assemble", "run")(rt)
	DenyAttributes("plan", "save")(rt)
	DenyAttributes("ui", "prompt")(rt)

	plan := sortedKeys(rt.denied["plan"])
	if want := []string{"assemble", "run", "save"}; !equalStrings(plan, want) {
		t.Fatalf("denied[plan] = %v, want %v", plan, want)
	}

	ui := sortedKeys(rt.denied["ui"])
	if want := []string{"prompt"}; !equalStrings(ui, want) {
		t.Fatalf("denied[ui] = %v, want %v", ui, want)
	}
}

// TestRuntime_applyDenials verifies a present global is wrapped, and an absent denied global is skipped.
func TestRuntime_applyDenials(t *testing.T) {

	t.Run("present global is wrapped", func(t *testing.T) {

		inner := &stubHasAttrs{attrs: map[string]starlark.Value{"deny": starlark.String("x")}}
		predeclared := starlark.StringDict{"plan": inner}

		rt := &Runtime{denied: map[string]map[string]bool{"plan": {"deny": true}}}
		rt.applyDenials(predeclared)

		wrapped, ok := predeclared["plan"].(*filteredReceiver)
		if !ok {
			t.Fatalf("predeclared[plan] is %T, want *filteredReceiver", predeclared["plan"])
		}

		if _, err := wrapped.Attr("deny"); err == nil {
			t.Fatal("wrapped Attr(deny): expected error, got nil")
		}
	})

	t.Run("absent denied global is skipped", func(t *testing.T) {

		predeclared := starlark.StringDict{"plan": &stubHasAttrs{}}

		rt := &Runtime{denied: map[string]map[string]bool{"absent": {"x": true}}}
		rt.applyDenials(predeclared)

		if _, present := predeclared["absent"]; present {
			t.Fatal("applyDenials introduced an entry for an absent global")
		}
	})
}

// endregion

// region Test helpers

// equalStrings reports whether two string slices are element-wise equal.
func equalStrings(a, b []string) bool {

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// sortedKeys returns the keys of a set in sorted order.
func sortedKeys(set map[string]bool) []string {

	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}

	sort.Strings(keys)
	return keys
}

// endregion
