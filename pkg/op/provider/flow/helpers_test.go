// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import "testing"

// --- isTruthy ---

func TestIsTruthy_Nil(t *testing.T) {

	if isTruthy(nil) {
		t.Error("isTruthy(nil) = true, want false")
	}
}

func TestIsTruthy_Bool(t *testing.T) {

	if !isTruthy(true) {
		t.Error("isTruthy(true) = false, want true")
	}
	if isTruthy(false) {
		t.Error("isTruthy(false) = true, want false")
	}
}

func TestIsTruthy_Integers(t *testing.T) {

	cases := []struct {
		value any
		want  bool
	}{
		{int(0), false}, {int(1), true}, {int(-1), true},
		{int32(0), false}, {int32(2), true},
		{int64(0), false}, {int64(2), true},
		{uint(0), false}, {uint(2), true},
		{uint64(0), false}, {uint64(2), true},
	}

	for _, tc := range cases {
		if got := isTruthy(tc.value); got != tc.want {
			t.Errorf("isTruthy(%v of %T) = %v, want %v", tc.value, tc.value, got, tc.want)
		}
	}
}

func TestIsTruthy_Floats(t *testing.T) {

	if isTruthy(0.0) {
		t.Error("isTruthy(0.0) = true, want false")
	}
	if !isTruthy(0.1) {
		t.Error("isTruthy(0.1) = false, want true")
	}
	if !isTruthy(-1.5) {
		t.Error("isTruthy(-1.5) = false, want true")
	}
}

func TestIsTruthy_String(t *testing.T) {

	if isTruthy("") {
		t.Error(`isTruthy("") = true, want false`)
	}
	if !isTruthy("anything") {
		t.Error(`isTruthy("anything") = false, want true`)
	}
}

func TestIsTruthy_OtherTypes(t *testing.T) {

	type marker struct{ X int }

	if !isTruthy(&marker{}) {
		t.Error("isTruthy(&marker{}) = false, want true (non-nil pointer)")
	}
	if !isTruthy([]int{}) {
		t.Error("isTruthy([]int{}) = false, want true (non-nil slice)")
	}
	if !isTruthy(map[string]int{}) {
		t.Error("isTruthy(map{}) = false, want true (non-nil map)")
	}
}

// --- Choose sequential + short-circuit contract ---

func TestChoose_FirstTruthyWins(t *testing.T) {

	p := &Provider{}

	got, _, err := p.Choose("default",
		Case{When: false, Then: "first"},
		Case{When: true, Then: "second"},
		Case{When: true, Then: "third-should-not-fire"},
	)
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if got != "second" {
		t.Errorf("got %q, want %q (first truthy When wins; later truthy cases short-circuited)", got, "second")
	}
}

func TestChoose_NoMatchReturnsDefault(t *testing.T) {

	p := &Provider{}

	got, _, err := p.Choose("default",
		Case{When: false, Then: "a"},
		Case{When: 0, Then: "b"},
		Case{When: "", Then: "c"},
		Case{When: nil, Then: "d"},
	)
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if got != "default" {
		t.Errorf("got %q, want %q", got, "default")
	}
}

func TestChoose_NoCases_ReturnsDefault(t *testing.T) {

	p := &Provider{}

	got, _, err := p.Choose("only")
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if got != "only" {
		t.Errorf("got %q, want %q", got, "only")
	}
}
