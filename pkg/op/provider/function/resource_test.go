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

// newTestRuntimeEnvironment returns a RuntimeEnvironment with a Root anchored at a fresh temp dir and a populated
// RecoverySite — the shape Resource construction requires.
func newTestRuntimeEnvironment(t *testing.T) *op.RuntimeEnvironment {
	t.Helper()
	root := op.NewRootReaderWriter(t.TempDir())
	runtimeEnvironment := &op.RuntimeEnvironment{Root: root}
	runtimeEnvironment.RecoverySite = op.NewRecoverySite(runtimeEnvironment)
	runtimeEnvironment.Catalog = op.NewResourceCatalog()
	return runtimeEnvironment
}

// testActivation wraps runtimeEnvironment in an [op.ActivationRecord] for non-graph dispatch. Graph and Unit are
// nil — production-claim test calls produce Resources with empty producer stamps.
func testActivation(t *testing.T, runtimeEnvironment *op.RuntimeEnvironment) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, nil, runtimeEnvironment)
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

// TestProducerStamp_NewResource verifies the m.5(iii) contract.
//
// A producer-style NewResource call flows through [op.ResourceCatalog.GetOrCreate], which stamps
// `Unit.ID()` as the catalog entry's producerID. function.Resources are produced directly via
// NewResource(activation.RuntimeEnvironment, activation.Unit, *starlark.Function). Under the test fixture's non-graph dispatch (nil `Unit`)
// the produced Resource carries an empty producer stamp.
func TestProducerStamp_NewResource(t *testing.T) {
	runtimeEnvironment := newTestRuntimeEnvironment(t)
	activation := testActivation(t, runtimeEnvironment)

	starFn := compileFixture(t, `
def stamp(x):
    return x
`, "stamp")

	r, err := NewResource(activation.RuntimeEnvironment, activation.Unit, starFn)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	if got := r.ProducerID(); got != "" {
		t.Errorf("producerID = %q, want empty (nil Unit)", got)
	}
}

func TestNewFunction_ArchivesPackToRecoverySite(t *testing.T) {

	runtimeEnvironment := newTestRuntimeEnvironment(t)

	starFn := compileFixture(t, `
def inc(x):
    return x + 1
`, "inc")

	f, err := NewResource(runtimeEnvironment, nil, starFn)
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

	runtimeEnvironment := newTestRuntimeEnvironment(t)

	if _, err := NewResource(runtimeEnvironment, nil, 42); err == nil {
		t.Error("NewFunction(int) succeeded, want error")
	}
}

// --- Function.Init (Bridge) ---

func TestFunction_Init_FastPath(t *testing.T) {

	runtimeEnvironment := newTestRuntimeEnvironment(t)

	starFn := compileFixture(t, `
def double(x):
    return x * 2
`, "double")

	f, _ := NewResource(runtimeEnvironment, nil, starFn)

	// Bridge it: func(int) int
	target := reflect.TypeFor[func(int) int]()
	bridged, err := op.Convert(runtimeEnvironment, f, target)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	double := bridged.(func(int) int)
	if res := double(21); res != 42 {
		t.Errorf("double(21) = %d, want 42", res)
	}
}

func TestFunction_Init_MmapFallback(t *testing.T) {

	runtimeEnvironment := newTestRuntimeEnvironment(t)

	starFn := compileFixture(t, `
def greet(name):
    return "hello " + name
`, "greet")

	f, _ := NewResource(runtimeEnvironment, nil, starFn)

	// Wipe memory cache to force mmap fallback.
	f.Compiled = nil

	target := reflect.TypeFor[func(string) string]()
	bridged, err := op.Convert(runtimeEnvironment, f, target)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	greet := bridged.(func(string) string)
	if res := greet("world"); res != "hello world" {
		t.Errorf("greet(world) = %q, want \"hello world\"", res)
	}
}

func TestFunction_Init_VersionSkewFallback(t *testing.T) {

	runtimeEnvironment := newTestRuntimeEnvironment(t)

	starFn := compileFixture(t, `
def identity(x):
    return x
`, "identity")

	f, _ := NewResource(runtimeEnvironment, nil, starFn)

	// Force version skew and wipe cache.
	f.Compiled = nil
	f.CompilerVersion = 0

	target := reflect.TypeFor[func(int) int]()
	bridged, err := op.Convert(runtimeEnvironment, f, target)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	identity := bridged.(func(int) int)
	if res := identity(42); res != 42 {
		t.Errorf("identity(42) = %d, want 42", res)
	}
}

func TestFunction_Init_NotAPointer(t *testing.T) {

	runtimeEnvironment := newTestRuntimeEnvironment(t)
	f := &Resource{}
	target := reflect.TypeFor[int]() // not a func
	_, err := op.Convert(runtimeEnvironment, f, target)
	if err == nil {
		t.Error("Convert(non-func) succeeded, want error")
	}
}
