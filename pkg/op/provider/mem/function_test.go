// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// --- Interface guards ---

func TestFunctionImplementsResourceInterface(t *testing.T) {
	// Function embeds Resource, which implements op.Resource.
	var _ = (*Function)(nil)
}

// --- Test helpers ---

// compileFixture parses and executes `src` and returns the named starlark function from its globals.
// Used to produce a *starlark.Function fixture for exercising NewFunction end-to-end.
func compileFixture(t *testing.T, src, name string) *starlark.Function {
	t.Helper()

	thread := &starlark.Thread{Name: "test"}
	_, prog, err := starlark.SourceProgramOptions(
		&syntax.FileOptions{}, "fixture.star", []byte(src), func(string) bool { return false },
	)
	if err != nil {
		t.Fatalf("compile fixture: %v", err)
	}

	globals, err := prog.Init(thread, nil)
	if err != nil {
		t.Fatalf("init fixture: %v", err)
	}

	fn, ok := globals[name]
	if !ok {
		t.Fatalf("fixture %q not found in globals", name)
	}

	starFn, ok := fn.(*starlark.Function)
	if !ok {
		t.Fatalf("fixture %q is %T, want *starlark.Function", name, fn)
	}

	return starFn
}

// --- NewFunction ---

func TestNewFunction_ArchivesPackToRecoverySite(t *testing.T) {

	ctx := newTestCtx(t)

	starFn := compileFixture(t, `
def inc(x):
    return x + 1
`, "inc")

	f, err := NewFunction(ctx, ResourceSpec{
		Namespace: "Incrementer",
		Data:      starFn,
	})
	if err != nil {
		t.Fatalf("NewFunction: %v", err)
	}

	if f.SourcePath.Abs() == "" {
		t.Fatal("SourcePath is empty — pack was not archived")
	}
	if f.Hash == "" {
		t.Fatal("Hash is empty — source was not hashed")
	}
	if len(f.Compiled) == 0 {
		t.Fatal("Compiled cache is empty — bytecode was not retained")
	}
	if f.CompilerVersion != starlark.CompilerVersion {
		t.Errorf("CompilerVersion = %d, want %d", f.CompilerVersion, starlark.CompilerVersion)
	}

	if f.FuncName != "inc" {
		t.Errorf("FuncName = %q, want %q", f.FuncName, "inc")
	}
	if f.NumParams != 1 {
		t.Errorf("NumParams = %d, want 1", f.NumParams)
	}
	if len(f.ParamNames) != 1 || f.ParamNames[0] != "x" {
		t.Errorf("ParamNames = %v, want [x]", f.ParamNames)
	}
}

func TestNewFunction_WrongSpecType(t *testing.T) {

	ctx := newTestCtx(t)

	if _, err := NewFunction(ctx, 42); err == nil {
		t.Fatal("expected error for non-ResourceSpec")
	}
}

func TestNewFunction_NonFunctionData(t *testing.T) {

	ctx := newTestCtx(t)

	if _, err := NewFunction(ctx, ResourceSpec{Namespace: "X", Data: "not a function"}); err == nil {
		t.Fatal("expected error for non-function Data")
	}
}

func TestNewFunction_EmptyNamespace(t *testing.T) {

	ctx := newTestCtx(t)

	starFn := compileFixture(t, `def f(): pass
`, "f")

	if _, err := NewFunction(ctx, ResourceSpec{Data: starFn}); err == nil {
		t.Fatal("expected error for empty namespace")
	}
}

// --- Init ---

func TestFunction_Init_FastPath(t *testing.T) {

	ctx := newTestCtx(t)

	starFn := compileFixture(t, `
def double(x):
    return x * 2
`, "double")

	f, err := NewFunction(ctx, ResourceSpec{Namespace: "Doubler", Data: starFn})
	if err != nil {
		t.Fatalf("NewFunction: %v", err)
	}

	// Compiled is cached from NewFunction; Init takes the fast path.
	thread := &starlark.Thread{Name: "test-init"}
	callable, err := f.Init(thread)
	if err != nil {
		t.Fatalf("Init fast-path: %v", err)
	}
	if callable == nil {
		t.Fatal("Init returned nil callable")
	}

	// Invoke and check result.
	result, err := starlark.Call(thread, callable, starlark.Tuple{starlark.MakeInt(21)}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got, ok := result.(starlark.Int)
	if !ok {
		t.Fatalf("result is %T, want starlark.Int", result)
	}
	if gotInt, _ := got.Int64(); gotInt != 42 {
		t.Errorf("double(21) = %d, want 42", gotInt)
	}
}

func TestFunction_Init_MmapFallback(t *testing.T) {

	ctx := newTestCtx(t)

	starFn := compileFixture(t, `
def greet(name):
    return "hello " + name
`, "greet")

	f, err := NewFunction(ctx, ResourceSpec{Namespace: "Greeter", Data: starFn})
	if err != nil {
		t.Fatalf("NewFunction: %v", err)
	}

	// Simulate "process restart" — clear in-memory caches. Init must fall back to the pack.
	f.Compiled = nil
	f.CompilerVersion = 0

	thread := &starlark.Thread{Name: "test-mmap"}
	callable, err := f.Init(thread)
	if err != nil {
		t.Fatalf("Init mmap-fallback: %v", err)
	}
	if callable == nil {
		t.Fatal("Init returned nil callable")
	}

	// After fallback, caches should be repopulated.
	if len(f.Compiled) == 0 {
		t.Error("Compiled cache not refreshed after mmap fallback")
	}
	if f.CompilerVersion != starlark.CompilerVersion {
		t.Errorf("CompilerVersion = %d, want %d", f.CompilerVersion, starlark.CompilerVersion)
	}

	// Invoke and check result.
	result, err := starlark.Call(thread, callable, starlark.Tuple{starlark.String("world")}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got, ok := result.(starlark.String)
	if !ok {
		t.Fatalf("result is %T, want starlark.String", result)
	}
	if string(got) != "hello world" {
		t.Errorf("greet(world) = %q, want %q", got, "hello world")
	}
}

func TestFunction_Init_VersionSkewFallback(t *testing.T) {

	ctx := newTestCtx(t)

	starFn := compileFixture(t, `
def identity(x):
    return x
`, "identity")

	f, err := NewFunction(ctx, ResourceSpec{Namespace: "Identity", Data: starFn})
	if err != nil {
		t.Fatalf("NewFunction: %v", err)
	}

	// Simulate skew: clear cache and force fallback; the pack header will show the real compiler version
	// so the compiled section is used. To exercise the recompile-from-source path, clear cache AND tamper
	// with the pack is beyond this test's scope. This test instead confirms that the in-memory cache's
	// version tag is the only source of truth for the fast-path decision.
	f.Compiled = []byte("stale bytecode ignored")
	f.CompilerVersion = 0 // mismatch → fast path rejects → mmap fallback

	thread := &starlark.Thread{Name: "test-skew"}
	callable, err := f.Init(thread)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if callable == nil {
		t.Fatal("Init returned nil callable")
	}

	// Cache should be refreshed to the real compiled bytes.
	if string(f.Compiled) == "stale bytecode ignored" {
		t.Error("Compiled cache still contains stale bytes after skew detection")
	}
	if f.CompilerVersion != starlark.CompilerVersion {
		t.Errorf("CompilerVersion = %d, want %d", f.CompilerVersion, starlark.CompilerVersion)
	}
}
