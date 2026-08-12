// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// --- GuardResult serialization ---

func TestGuardResult_JSONRoundTrip(t *testing.T) {

	edge := Edge{From: "when-1", To: "then-1", Guard: GuardTruthy}

	data, err := json.Marshal(edge)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"guard":"truthy"`) {
		t.Errorf("JSON = %s, want it to carry %q", data, `"guard":"truthy"`)
	}

	var reloaded Edge
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if reloaded != edge {
		t.Errorf("round-trip = %+v, want %+v", reloaded, edge)
	}
}

func TestGuardResult_JSONOmitsGuardNone(t *testing.T) {

	data, err := json.Marshal(Edge{From: "a", To: "b"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "guard") {
		t.Errorf("JSON = %s, want no guard key for an unguarded edge (old traces unchanged)", data)
	}
}

func TestGuardResult_YAMLRoundTrip(t *testing.T) {

	edge := Edge{From: "when-1", To: "then-1", Guard: GuardFalsy}

	data, err := yaml.Marshal(edge)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), "guard: falsy") {
		t.Errorf("YAML = %s, want it to carry %q", data, "guard: falsy")
	}

	var reloaded Edge
	if err := yaml.Unmarshal(data, &reloaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if reloaded != edge {
		t.Errorf("round-trip = %+v, want %+v", reloaded, edge)
	}
}

func TestGuardResult_YAMLOmitsGuardNone(t *testing.T) {

	data, err := yaml.Marshal(Edge{From: "a", To: "b"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "guard") {
		t.Errorf("YAML = %s, want no guard key for an unguarded edge (old traces unchanged)", data)
	}
}

func TestGuardResult_UnmarshalText_UnknownName(t *testing.T) {

	var guard GuardResult
	if err := guard.UnmarshalText([]byte("sometimes")); err == nil {
		t.Error(`UnmarshalText("sometimes") error = nil, want error`)
	}
}

func TestGuardResult_String(t *testing.T) {

	cases := []struct {
		guard GuardResult
		want  string
	}{
		{GuardNone, "none"},
		{GuardTruthy, "truthy"},
		{GuardFalsy, "falsy"},
		{GuardResult(9), "GuardResult(9)"},
	}

	for _, tc := range cases {
		if got := tc.guard.String(); got != tc.want {
			t.Errorf("String(%d) = %q, want %q", uint8(tc.guard), got, tc.want)
		}
	}
}

// --- RecoveryStack starlark projection ---

func TestRecoveryStack_DerivesAsOpaqueReceiver(t *testing.T) {

	// A *RecoveryStack crosses the starlark bridge (e.g. as a file.walk_tree argument), which derives a receiver type
	// for it fresh. Methods whose return signatures have no MethodKind classification — the (T, bool) lookups — must
	// be skipped, not fatal for the whole type.
	derived, err := NewReceiverType(reflect.TypeOf(&RecoveryStack{}), nil)
	if err != nil {
		t.Fatalf("NewReceiverType(*RecoveryStack, nil) error = %v, want nil (unclassifiable methods skipped)", err)
	}
	if _, found := derived.MethodByName("ResultByUnitID"); found {
		t.Error("derived type exposes ResultByUnitID; want (T, bool) lookups excluded from the starlark projection")
	}
	if _, found := derived.MethodByName("Len"); !found {
		t.Error("derived type is missing Len; want classifiable methods retained")
	}
}
