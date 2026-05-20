// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package assert_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// recoverError runs fn and returns the recovered [*assert.AssertionError], or nil if fn did not panic with one.
func recoverError(t *testing.T, fn func()) (got *assert.AssertionError) {

	t.Helper()

	defer func() {
		r := recover()
		if r == nil {
			return
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("panic value is %T, want error", r)
		}
		if !errors.As(err, &got) {
			t.Fatalf("panic value %v does not match *AssertionError", err)
		}
	}()

	fn()

	return got
}

// expectNoPanic asserts that fn does not panic. The test fails with a descriptive message if it does.
func expectNoPanic(t *testing.T, label string, fn func()) {

	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s: unexpected panic: %v", label, r)
		}
	}()

	fn()
}

// region Nil

func TestNil_PassesOnNilPointer(t *testing.T) {

	var p *int

	expectNoPanic(t, "Nil(p)", func() { assert.Nil("p", p) })
}

func TestNil_FailsOnNonNilPointer(t *testing.T) {

	v := 42
	p := &v

	got := recoverError(t, func() { assert.Nil("p", p) })

	if got == nil {
		t.Fatal("expected panic on non-nil pointer")
	}
	if !strings.Contains(got.Message, "expected nil") {
		t.Errorf("Message = %q, want contains \"expected nil\"", got.Message)
	}
	if !strings.Contains(got.Message, "p:") {
		t.Errorf("Message = %q, want contains \"p:\"", got.Message)
	}
}

// endregion

// region NonZero

func TestNonZero_PassesOnNonNilPointer(t *testing.T) {

	v := struct{}{}

	expectNoPanic(t, "NonZero(&v)", func() { assert.NonZero("v", &v) })
}

func TestNonZero_FailsOnTypedNilPointer(t *testing.T) {

	var p *int

	got := recoverError(t, func() { assert.NonZero("p", p) })

	if got == nil {
		t.Fatal("expected panic on typed-nil pointer")
	}
	if !strings.Contains(got.Message, "non-zero value for p is required") {
		t.Errorf("Message = %q, want contains \"non-zero value for p is required\"", got.Message)
	}
	if !strings.HasSuffix(got.Function, "TestNonZero_FailsOnTypedNilPointer.func1") {
		t.Errorf("Function = %q, want suffix TestNonZero_FailsOnTypedNilPointer.func1", got.Function)
	}
}

func TestNonZero_PassesOnNonZeroPrimitives(t *testing.T) {

	expectNoPanic(t, "NonZero(int)", func() { assert.NonZero("i", 1) })
	expectNoPanic(t, "NonZero(string)", func() { assert.NonZero("s", "x") })
	expectNoPanic(t, "NonZero(bool)", func() { assert.NonZero("b", true) })
}

func TestNonZero_FailsOnZeroInt(t *testing.T) {

	got := recoverError(t, func() { assert.NonZero("i", 0) })

	if got == nil {
		t.Fatal("expected panic on zero int")
	}
	if !strings.Contains(got.Message, "non-zero value for i is required") {
		t.Errorf("Message = %q, want contains \"non-zero value for i is required\"", got.Message)
	}
}

func TestNonZero_FailsOnEmptyString(t *testing.T) {

	got := recoverError(t, func() { assert.NonZero("s", "") })

	if got == nil {
		t.Fatal("expected panic on empty string")
	}
	if !strings.Contains(got.Message, "non-zero value for s is required") {
		t.Errorf("Message = %q, want contains \"non-zero value for s is required\"", got.Message)
	}
}

func TestNonZero_FailsOnFalseBool(t *testing.T) {

	got := recoverError(t, func() { assert.NonZero("b", false) })

	if got == nil {
		t.Fatal("expected panic on false bool")
	}
	if !strings.Contains(got.Message, "non-zero value for b is required") {
		t.Errorf("Message = %q, want contains \"non-zero value for b is required\"", got.Message)
	}
}

func TestNonZero_FailsOnZeroStruct(t *testing.T) {

	type point struct {
		X int
		Y int
	}

	got := recoverError(t, func() { assert.NonZero("pt", point{}) })

	if got == nil {
		t.Fatal("expected panic on zero struct")
	}
	if !strings.Contains(got.Message, "non-zero value for pt is required") {
		t.Errorf("Message = %q, want contains \"non-zero value for pt is required\"", got.Message)
	}
}

func TestNonZero_PassesOnNonZeroStruct(t *testing.T) {

	type point struct {
		X int
		Y int
	}

	expectNoPanic(t, "NonZero(point{1, 2})", func() { assert.NonZero("pt", point{X: 1, Y: 2}) })
}

func TestNonZero_ReturnsValue(t *testing.T) {

	got := assert.NonZero("i", 42)
	if got != 42 {
		t.Errorf("NonZero returned %d, want 42", got)
	}

	str := assert.NonZero("s", "hello")
	if str != "hello" {
		t.Errorf("NonZero returned %q, want %q", str, "hello")
	}

	v := 7
	p := assert.NonZero("p", &v)
	if p != &v {
		t.Errorf("NonZero returned pointer %p, want %p", p, &v)
	}
}

// endregion

// NonEmpty is unexercised here: its current signature
//
//	func NonEmpty[T ~[]E | ~map[K]V | ~string, E any, K comparable, V any](name string, value T) T
//
// can't be called with Go's normal type inference — every call needs all four type parameters
// supplied explicitly (e.g., assert.NonEmpty[[]int, int, string, any]("s", []int{1})). The function
// has zero real callers in the codebase. Adding tests would either (a) lock in that hostile call
// shape or (b) misrepresent how the function is meant to be used. Flagged for redesign before
// any caller adopts it.

// region True / Truef

func TestTruePasses(t *testing.T) {

	expectNoPanic(t, "True(always)", func() { assert.True("always", true) })
}

func TestTrueFails(t *testing.T) {

	got := recoverError(t, func() { assert.True("must hold", false) })

	if got == nil {
		t.Fatal("expected panic")
	}
	if got.Message != "must hold" {
		t.Errorf("Message = %q, want %q", got.Message, "must hold")
	}
}

func TestTruefPasses(t *testing.T) {

	expectNoPanic(t, "Truef(true)", func() { assert.Truef(true, "got %d, want %d", 3, 5) })
}

func TestTruefFails(t *testing.T) {

	got := recoverError(t, func() { assert.Truef(false, "got %d, want %d", 3, 5) })

	if got == nil {
		t.Fatal("expected panic")
	}
	if got.Message != "got 3, want 5" {
		t.Errorf("Message = %q, want %q", got.Message, "got 3, want 5")
	}
}

// endregion

// region Unreachable

func TestUnreachable(t *testing.T) {

	got := recoverError(t, func() { assert.Unreachable("default branch") })

	if got == nil {
		t.Fatal("expected panic")
	}
	if got.Message != "unreachable: default branch" {
		t.Errorf("Message = %q, want %q", got.Message, "unreachable: default branch")
	}
}

// endregion

// region Failf

func TestFailf(t *testing.T) {

	got := recoverError(t, func() { assert.Failf("got %d, want %d", 3, 5) })

	if got == nil {
		t.Fatal("expected panic")
	}
	if got.Message != "got 3, want 5" {
		t.Errorf("Message = %q, want %q", got.Message, "got 3, want 5")
	}
}

// endregion

// region NoError

func TestNoErrorPasses(t *testing.T) {

	expectNoPanic(t, "NoError(nil)", func() { assert.NoError("op.Defer", nil) })
}

func TestNoErrorFails(t *testing.T) {

	got := recoverError(t, func() { assert.NoError("op.Defer", errors.New("bad path")) })

	if got == nil {
		t.Fatal("expected panic")
	}
	if got.Message != "op.Defer: bad path" {
		t.Errorf("Message = %q, want %q", got.Message, "op.Defer: bad path")
	}
}

// endregion

// region AssertionError

func TestErrorErrorFormat(t *testing.T) {

	got := recoverError(t, func() { assert.True("claim", false) })

	if got == nil {
		t.Fatal("expected panic")
	}
	if !strings.HasSuffix(got.Error(), ": claim") {
		t.Errorf("Error() = %q, want suffix \": claim\"", got.Error())
	}
}

func TestFrameCaptured(t *testing.T) {

	got := recoverError(t, func() { assert.True("c", false) })

	if got == nil {
		t.Fatal("expected panic")
	}
	if got.Line == 0 {
		t.Error("Line = 0, want non-zero")
	}
	if !strings.HasSuffix(got.File, "assert_test.go") {
		t.Errorf("File = %q, want suffix assert_test.go", got.File)
	}
}

// endregion
