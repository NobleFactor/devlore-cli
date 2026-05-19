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

func TestNotNilPasses(t *testing.T) {

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NotNil panicked on non-nil value: %v", r)
		}
	}()

	v := struct{}{}
	assert.NotNil("v", &v)
}

func TestNotNilFailsOnTypedNilPointer(t *testing.T) {

	var p *int

	got := recoverError(t, func() { assert.NotNil("p", p) })

	if got == nil {
		t.Fatal("expected panic on typed-nil pointer")
	}
	if !strings.Contains(got.Message, "p is required") {
		t.Errorf("Message = %q, want contains \"p is required\"", got.Message)
	}
	if !strings.HasSuffix(got.Function, "TestNotNilFailsOnTypedNilPointer.func1") {
		t.Errorf("Function = %q, want ends with TestNotNilFailsOnTypedNilPointer.func1", got.Function)
	}
}

func TestTruePasses(t *testing.T) {

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("True panicked on true condition: %v", r)
		}
	}()

	assert.True("always", true)
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

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Truef panicked on true condition: %v", r)
		}
	}()

	assert.Truef("got %d, want %d", true, 3, 5)
}

func TestTruefFails(t *testing.T) {

	got := recoverError(t, func() { assert.Truef("got %d, want %d", false, 3, 5) })

	if got == nil {
		t.Fatal("expected panic")
	}
	if got.Message != "got 3, want 5" {
		t.Errorf("Message = %q, want %q", got.Message, "got 3, want 5")
	}
}

func TestUnreachable(t *testing.T) {

	got := recoverError(t, func() { assert.Unreachable("default branch") })

	if got == nil {
		t.Fatal("expected panic")
	}
	if got.Message != "unreachable: default branch" {
		t.Errorf("Message = %q, want %q", got.Message, "unreachable: default branch")
	}
}

func TestFailf(t *testing.T) {

	got := recoverError(t, func() { assert.Failf("got %d, want %d", 3, 5) })

	if got == nil {
		t.Fatal("expected panic")
	}
	if got.Message != "got 3, want 5" {
		t.Errorf("Message = %q, want %q", got.Message, "got 3, want 5")
	}
}

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