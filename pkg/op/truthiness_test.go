// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import "testing"

func TestIsTruthy_Nil(t *testing.T) {

	if IsTruthy(nil) {
		t.Error("IsTruthy(nil) = true, want false")
	}
}

func TestIsTruthy_Bool(t *testing.T) {

	if !IsTruthy(true) {
		t.Error("IsTruthy(true) = false, want true")
	}
	if IsTruthy(false) {
		t.Error("IsTruthy(false) = true, want false")
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
		if got := IsTruthy(tc.value); got != tc.want {
			t.Errorf("IsTruthy(%v of %T) = %v, want %v", tc.value, tc.value, got, tc.want)
		}
	}
}

func TestIsTruthy_Floats(t *testing.T) {

	if IsTruthy(0.0) {
		t.Error("IsTruthy(0.0) = true, want false")
	}
	if !IsTruthy(0.1) {
		t.Error("IsTruthy(0.1) = false, want true")
	}
	if !IsTruthy(-1.5) {
		t.Error("IsTruthy(-1.5) = false, want true")
	}
}

func TestIsTruthy_String(t *testing.T) {

	if IsTruthy("") {
		t.Error(`IsTruthy("") = true, want false`)
	}
	if !IsTruthy("anything") {
		t.Error(`IsTruthy("anything") = false, want true`)
	}
}

func TestIsTruthy_Containers(t *testing.T) {

	cases := []struct {
		value any
		want  bool
	}{
		{[]int{}, false}, {[]int{1}, true},
		{[]any(nil), false}, {[]any{"x"}, true},
		{map[string]int{}, false}, {map[string]int{"k": 1}, true},
		{map[string]any(nil), false},
		{[0]int{}, false}, {[2]int{0, 0}, true},
	}

	for _, tc := range cases {
		if got := IsTruthy(tc.value); got != tc.want {
			t.Errorf("IsTruthy(%v of %T) = %v, want %v", tc.value, tc.value, got, tc.want)
		}
	}
}

func TestIsTruthy_OtherTypes(t *testing.T) {

	type marker struct{ X int }

	if !IsTruthy(&marker{}) {
		t.Error("IsTruthy(&marker{}) = false, want true (non-nil pointer)")
	}
	if IsTruthy((*marker)(nil)) {
		t.Error("IsTruthy((*marker)(nil)) = true, want false (typed-nil pointer)")
	}
	if IsTruthy(marker{}) {
		t.Error("IsTruthy(marker{}) = true, want false (zero-value struct)")
	}
	if !IsTruthy(marker{X: 1}) {
		t.Error("IsTruthy(marker{X: 1}) = false, want true (non-zero struct)")
	}
}
