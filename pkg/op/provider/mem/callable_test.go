// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestCallableImplementsResource(t *testing.T) {
	var _ op.Resource = (*Callable)(nil)
}

func TestCallableImplementsCallableResource(t *testing.T) {
	var _ bind.CallableResource = (*Callable)(nil)
}

func TestNewCallable(t *testing.T) {
	c := NewCallable("file.Reducer", "count_python_files")
	if c.FuncType != "file.Reducer" {
		t.Errorf("FuncType = %q, want %q", c.FuncType, "file.Reducer")
	}
	if c.Name != "count_python_files" {
		t.Errorf("ReceiverName = %q, want %q", c.Name, "count_python_files")
	}
	if c.FuncName != "_callable" {
		t.Errorf("FuncName = %q, want %q", c.FuncName, "_callable")
	}
	if c.ContentType != "callable" {
		t.Errorf("ContentType = %q, want %q", c.ContentType, "callable")
	}
}

func TestCallableURI(t *testing.T) {
	c := NewCallable("file.Reducer", "count_python_files")
	want := "mem:callable/file.Reducer/count_python_files"
	if got := c.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestCallableURI_Lambda(t *testing.T) {
	c := NewCallable("file.Reducer", "file.walk_tree.fn")
	want := "mem:callable/file.Reducer/file.walk_tree.fn"
	if got := c.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestCallableURI_OpaqueScheme(t *testing.T) {
	c := NewCallable("Predicate", "is_large")
	if c.Scheme() != "mem" {
		t.Errorf("Scheme() = %q, want %q", c.Scheme(), "mem")
	}
	if c.Opaque() != "callable/Predicate/is_large" {
		t.Errorf("Opaque() = %q, want %q", c.Opaque(), "callable/Predicate/is_large")
	}
}

func TestCallableSetSource(t *testing.T) {
	c := NewCallable("file.Reducer", "myfn")
	source := []byte(`def _callable(initial, resource, path):
    return initial + [resource]
`)
	c.SetSource(source)
	if string(c.Data) != string(source) {
		t.Errorf("Data = %q, want %q", c.Data, source)
	}
	if c.Hash == "" {
		t.Error("Hash is empty after SetSource")
	}
}

func TestCallableSetSource_HashChanges(t *testing.T) {
	c := NewCallable("file.Reducer", "myfn")
	c.SetSource([]byte("version 1"))
	hash1 := c.Hash
	c.SetSource([]byte("version 2"))
	hash2 := c.Hash
	if hash1 == hash2 {
		t.Error("hash did not change after SetSource with different data")
	}
}

func TestCallableFn_PanicsBeforeInit(t *testing.T) {
	c := NewCallable("file.Reducer", "myfn")
	defer func() {
		if r := recover(); r == nil {
			t.Error("Fn() did not panic before Init")
		}
	}()
	c.Fn()
}

// -------------------------------------------------------------------
// Phase 3: Compilation and Initialization
// -------------------------------------------------------------------

// makeCallable creates a Callable with valid source for testing.
func makeCallable(t *testing.T, funcName string, source string) *Callable {
	t.Helper()
	c := NewCallable("TestType", "testfn")
	c.FuncName = funcName
	c.SetSource([]byte(source))
	return c
}

func TestCompile_ProducesNonEmptyBytecode(t *testing.T) {
	c := makeCallable(t, "double", "def double(x):\n    return x * 2\n")
	if err := c.Compile(); err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(c.Compiled) == 0 {
		t.Error("Compiled is empty after Compile")
	}
	if c.CompilerVersion == 0 {
		t.Error("CompilerVersion is 0 after Compile")
	}
	if c.CompilerVersion != starlark.CompilerVersion {
		t.Errorf("CompilerVersion = %d, want %d", c.CompilerVersion, starlark.CompilerVersion)
	}
}

func TestCompile_Idempotent(t *testing.T) {
	c := makeCallable(t, "double", "def double(x):\n    return x * 2\n")
	if err := c.Compile(); err != nil {
		t.Fatalf("Compile 1: %v", err)
	}
	first := make([]byte, len(c.Compiled))
	copy(first, c.Compiled)

	if err := c.Compile(); err != nil {
		t.Fatalf("Compile 2: %v", err)
	}
	if string(c.Compiled) != string(first) {
		t.Error("Compile produced different bytecode on second call")
	}
}

func TestCompile_NoSource(t *testing.T) {
	c := NewCallable("TestType", "testfn")
	if err := c.Compile(); err == nil {
		t.Fatal("expected error for empty source")
	}
}

func TestCompile_InvalidSource(t *testing.T) {
	c := makeCallable(t, "_callable", "def !!invalid syntax")
	if err := c.Compile(); err == nil {
		t.Fatal("expected error for invalid source")
	}
}

func TestInit_WithCompiledBytecode(t *testing.T) {
	c := makeCallable(t, "double", "def double(x):\n    return x * 2\n")
	if err := c.Compile(); err != nil {
		t.Fatalf("Compile: %v", err)
	}

	thread := &starlark.Thread{Name: "test"}
	if err := c.Init(thread); err != nil {
		t.Fatalf("Init: %v", err)
	}

	fn := c.Fn()
	result, err := starlark.Call(thread, fn, starlark.Tuple{starlark.MakeInt(7)}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got, _ := result.(starlark.Int).Int64()
	if got != 14 {
		t.Errorf("double(7) = %d, want 14", got)
	}
}

func TestInit_FallbackToSourceOnVersionMismatch(t *testing.T) {
	c := makeCallable(t, "double", "def double(x):\n    return x * 2\n")
	if err := c.Compile(); err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// Simulate a version mismatch by setting a bogus compiler version.
	c.CompilerVersion = 999999

	thread := &starlark.Thread{Name: "test"}
	if err := c.Init(thread); err != nil {
		t.Fatalf("Init should fall back to source recompilation: %v", err)
	}

	fn := c.Fn()
	result, err := starlark.Call(thread, fn, starlark.Tuple{starlark.MakeInt(5)}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got, _ := result.(starlark.Int).Int64()
	if got != 10 {
		t.Errorf("double(5) = %d, want 10", got)
	}
}

func TestInit_SourceOnlyNoBytecode(t *testing.T) {
	c := makeCallable(t, "add_one", "def add_one(x):\n    return x + 1\n")
	// No Compile() call — Init should compile from source.

	thread := &starlark.Thread{Name: "test"}
	if err := c.Init(thread); err != nil {
		t.Fatalf("Init: %v", err)
	}

	result, err := starlark.Call(thread, c.Fn(), starlark.Tuple{starlark.MakeInt(41)}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got, _ := result.(starlark.Int).Int64()
	if got != 42 {
		t.Errorf("add_one(41) = %d, want 42", got)
	}
}

func TestInit_ExtractsNamedFunction(t *testing.T) {
	source := "def alpha():\n    return 1\n\ndef beta():\n    return 2\n"
	c := makeCallable(t, "beta", source)

	thread := &starlark.Thread{Name: "test"}
	if err := c.Init(thread); err != nil {
		t.Fatalf("Init: %v", err)
	}

	result, err := starlark.Call(thread, c.Fn(), nil, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got, _ := result.(starlark.Int).Int64()
	if got != 2 {
		t.Errorf("beta() = %d, want 2", got)
	}
}

func TestInit_RejectsMissingFuncName(t *testing.T) {
	c := makeCallable(t, "nonexistent", "def actual(x):\n    return x\n")

	thread := &starlark.Thread{Name: "test"}
	err := c.Init(thread)
	if err == nil {
		t.Fatal("expected error for missing function name")
	}
	if !testing.Verbose() {
		return
	}
	t.Logf("error: %v", err)
}

func TestInit_RejectsNonCallable(t *testing.T) {
	// FuncName points to a global that isn't a function.
	c := makeCallable(t, "x", "x = 42\n")

	thread := &starlark.Thread{Name: "test"}
	err := c.Init(thread)
	if err == nil {
		t.Fatal("expected error for non-callable global")
	}
}

func TestBytecodeRoundTrip(t *testing.T) {
	// Full lifecycle: source → Compile → Init → call → verify result.
	source := "offset = 10\ndef transform(x):\n    return x + offset\n"
	c := makeCallable(t, "transform", source)

	if err := c.Compile(); err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// Simulate serialization round-trip: create a new Callable with only
	// the bytecode and metadata (no source recompilation path).
	c2 := NewCallable("TestType", "testfn")
	c2.FuncName = "transform"
	c2.Compiled = make([]byte, len(c.Compiled))
	copy(c2.Compiled, c.Compiled)
	c2.CompilerVersion = c.CompilerVersion
	// Source is set but won't be used since bytecode + version match.
	c2.SetSource([]byte(source))

	thread := &starlark.Thread{Name: "test"}
	if err := c2.Init(thread); err != nil {
		t.Fatalf("Init: %v", err)
	}

	result, err := starlark.Call(thread, c2.Fn(), starlark.Tuple{starlark.MakeInt(5)}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got, _ := result.(starlark.Int).Int64()
	if got != 15 {
		t.Errorf("transform(5) = %d, want 15", got)
	}
}

func TestExtractCompileInitRoundTrip(t *testing.T) {
	// End-to-end: Extract from live function → Compile → Init → call.
	globals := execScript(t, "e2e.star", `
def make():
    multiplier = 3
    return lambda x: x * multiplier

fn = make()
`)
	starFn := globals["fn"].(*starlark.Function)

	c, err := Extract(starFn, "Transform")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if err := c.Compile(); err != nil {
		t.Fatalf("Compile: %v", err)
	}

	thread := &starlark.Thread{Name: "test"}
	if err := c.Init(thread); err != nil {
		t.Fatalf("Init: %v", err)
	}

	result, err := starlark.Call(thread, c.Fn(), starlark.Tuple{starlark.MakeInt(7)}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got, _ := result.(starlark.Int).Int64()
	if got != 21 {
		t.Errorf("fn(7) = %d, want 21", got)
	}
}
