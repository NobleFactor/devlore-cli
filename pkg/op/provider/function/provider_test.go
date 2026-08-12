// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package function

import (
	"strings"
	"testing"
)

// --- Provider.Call ---

// TestCall_PassesArgsAndKwargs verifies that Call routes positional and keyword arguments to the callable and returns
// its result.
//
// combine's suffix parameter defaults to "!"; the call overrides it with a "suffix" kwarg, so the "ab?" result proves
// both the positional args ("a", "b") and the keyword arg reached the function body and the return value came back.
func TestCall_PassesArgsAndKwargs(t *testing.T) {

	runtimeEnvironment := newTestRuntimeEnvironment(t)

	starFn := compileFixture(t, `
def combine(prefix, value, suffix="!"):
    return prefix + value + suffix
`, "combine")

	callable, err := NewResource(runtimeEnvironment, "", starFn)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	provider := NewProvider(runtimeEnvironment)

	got, err := provider.Call(callable, []any{"a", "b"}, map[string]any{"suffix": "?"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got != "ab?" {
		t.Errorf("Call result = %v, want %q", got, "ab?")
	}
}

// TestCall_PropagatesCallableError verifies that an error raised inside the callable surfaces from Call rather than
// being swallowed.
//
// The fail builtin raises with "fail: boom: <reason>"; the returned error must mention that message.
func TestCall_PropagatesCallableError(t *testing.T) {

	runtimeEnvironment := newTestRuntimeEnvironment(t)

	starFn := compileFixture(t, `
def explode(reason):
    fail("boom: " + reason)
`, "explode")

	callable, err := NewResource(runtimeEnvironment, "", starFn)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	provider := NewProvider(runtimeEnvironment)

	if _, err := provider.Call(callable, []any{"detonated"}, nil); err == nil {
		t.Fatal("Call succeeded, want error from fail()")
	} else if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want it to mention %q", err.Error(), "boom")
	}
}

// TestCall_NonCallableResource verifies that Call returns a clean error — not a panic — when the resource's named
// global resolves to a non-callable value.
//
// newFromSource builds a resource whose FuncName points at an integer binding, so Init reports "not callable" and Call
// surfaces that error.
func TestCall_NonCallableResource(t *testing.T) {

	runtimeEnvironment := newTestRuntimeEnvironment(t)

	notCallable, err := newFromSource(runtimeEnvironment, "not_a_func", nil, "", []byte("not_a_func = 42\n"))
	if err != nil {
		t.Fatalf("newFromSource: %v", err)
	}

	provider := NewProvider(runtimeEnvironment)

	if _, err := provider.Call(notCallable, nil, nil); err == nil {
		t.Error("Call(non-callable resource) succeeded, want error")
	}
}
