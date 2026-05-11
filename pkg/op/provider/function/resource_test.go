// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package function

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// --- Interface guards ---

func TestResourceImplementsResourceInterface(t *testing.T) {
	// function.Resource embeds mem.Resource, which implements op.Resource.
	var _ op.Resource = (*Resource)(nil)
}

// --- Test helpers ---

// newTestCtx returns a RuntimeEnvironment with a Root anchored at a fresh temp dir and a populated
// RecoverySite — the shape Resource construction requires.
func newTestCtx(t *testing.T) *op.RuntimeEnvironment {
	t.Helper()
	root := op.NewRootReaderWriter(t.TempDir())
	ctx := &op.RuntimeEnvironment{Root: root}
	ctx.RecoverySite = op.NewRecoverySite(ctx)
	return ctx
}

// testActivation wraps ctx in an [op.ActivationRecord] with a test-derived SiteID. Sufficient for
// production-claim test calls (non-nil + non-empty SiteID).
func testActivation(t *testing.T, ctx *op.RuntimeEnvironment) *op.ActivationRecord {
	t.Helper()
	return &op.ActivationRecord{Runtime: ctx, SiteID: "test:" + t.Name()}
}

// compileFixture parses and executes `src` and returns the named starlark function from its globals.
// Used to produce a *starlark.Function fixture for exercising NewResource end-to-end.
func compileFixture(t *testing.T, src, name string) *starlark.Function {
	t.Helper()

	// synthesize (extractDefSource) needs the source on disk at the position indicated by the function.
	tmp := t.TempDir()
	path := filepath.Join(tmp, "functions.in.star")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	thread := &starlark.Thread{Name: "test"}
	_, prog, err := starlark.SourceProgramOptions(
		&syntax.FileOptions{}, path, []byte(src), func(string) bool { return false },
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

	f, err := NewResource(testActivation(t, ctx), ResourceSpec{
		Namespace: "Incrementer",
		Data:      starFn,
	})
	if err != nil {
		t.Fatalf("NewFunction: %v", err)
	}

	if f.SourcePath().Abs() == "" {
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

	if _, err := NewResource(testActivation(t, ctx), 42); err == nil {
		t.Error("NewFunction(int) succeeded, want error")
	}
}

// --- Function.Init (Bridge) ---

func TestFunction_Init_FastPath(t *testing.T) {

	ctx := newTestCtx(t)

	starFn := compileFixture(t, `
def double(x):
    return x * 2
`, "double")

	f, _ := NewResource(testActivation(t, ctx), ResourceSpec{Namespace: "math", Data: starFn})

	// Bridge it: func(int) int
	target := reflect.TypeFor[func(int) int]()
	bridged, err := op.Convert(ctx, f, target)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	double := bridged.(func(int) int)
	if res := double(21); res != 42 {
		t.Errorf("double(21) = %d, want 42", res)
	}
}

func TestFunction_Init_MmapFallback(t *testing.T) {

	ctx := newTestCtx(t)

	starFn := compileFixture(t, `
def greet(name):
    return "hello " + name
`, "greet")

	f, _ := NewResource(testActivation(t, ctx), ResourceSpec{Namespace: "ui", Data: starFn})

	// Wipe memory cache to force mmap fallback.
	f.Compiled = nil

	target := reflect.TypeFor[func(string) string]()
	bridged, err := op.Convert(ctx, f, target)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	greet := bridged.(func(string) string)
	if res := greet("world"); res != "hello world" {
		t.Errorf("greet(world) = %q, want \"hello world\"", res)
	}
}

func TestFunction_Init_VersionSkewFallback(t *testing.T) {

	ctx := newTestCtx(t)

	starFn := compileFixture(t, `
def identity(x):
    return x
`, "identity")

	f, _ := NewResource(testActivation(t, ctx), ResourceSpec{Namespace: "core", Data: starFn})

	// Force version skew and wipe cache.
	f.Compiled = nil
	f.CompilerVersion = 0

	target := reflect.TypeFor[func(int) int]()
	bridged, err := op.Convert(ctx, f, target)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	identity := bridged.(func(int) int)
	if res := identity(42); res != 42 {
		t.Errorf("identity(42) = %d, want 42", res)
	}
}

func TestFunction_Init_NotAPointer(t *testing.T) {

	ctx := newTestCtx(t)
	f := &Resource{}
	target := reflect.TypeFor[int]() // not a func
	_, err := op.Convert(ctx, f, target)
	if err == nil {
		t.Error("Convert(non-func) succeeded, want error")
	}
}
