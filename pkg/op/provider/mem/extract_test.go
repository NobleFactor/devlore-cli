// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// execScript writes a Starlark script to a temp file, executes it, and
// returns the resulting globals. The caller can then extract functions
// from the globals dict.
func execScript(t *testing.T, name, source string) starlark.StringDict {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	thread := &starlark.Thread{Name: "test"}
	opts := &syntax.FileOptions{
		TopLevelControl: true,
		GlobalReassign:  true,
	}
	globals, err := starlark.ExecFileOptions(opts, thread, path, nil, nil)
	if err != nil {
		t.Fatalf("exec script: %v", err)
	}
	return globals
}

// -------------------------------------------------------------------
// Extract simple lambda (no closure)
// -------------------------------------------------------------------

func TestExtract_SimpleLambda(t *testing.T) {
	globals := execScript(t, "simple.star", `
fn = lambda x, y: x + y
`)
	fn := globals["fn"].(*starlark.Function)

	c, err := Extract(fn, "TestType")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if c.FuncType != "TestType" {
		t.Errorf("FuncType = %q, want %q", c.FuncType, "TestType")
	}
	if c.Name != "_lambda" {
		t.Errorf("Name = %q, want %q", c.Name, "_lambda")
	}
	if c.FuncName != "_callable" {
		t.Errorf("FuncName = %q, want %q", c.FuncName, "_callable")
	}
	if c.NumParams != 2 {
		t.Errorf("NumParams = %d, want 2", c.NumParams)
	}
	if len(c.ParamNames) != 2 || c.ParamNames[0] != "x" || c.ParamNames[1] != "y" {
		t.Errorf("ParamNames = %v, want [x y]", c.ParamNames)
	}

	source := string(c.Data)
	if !strings.Contains(source, "def _callable(x, y):") {
		t.Errorf("source missing def _callable:\n%s", source)
	}
	if !strings.Contains(source, "return x + y") {
		t.Errorf("source missing return body:\n%s", source)
	}
	if c.Hash == "" {
		t.Error("Hash is empty")
	}
	if c.OriginalPos == "" {
		t.Error("OriginalPos is empty")
	}
}

// -------------------------------------------------------------------
// Extract lambda with closure bindings
// -------------------------------------------------------------------

func TestExtract_LambdaWithClosure(t *testing.T) {
	// Variables must be in an enclosing function scope to become free vars.
	// Top-level variables are module globals and do NOT appear as free vars.
	globals := execScript(t, "closure.star", `
def make():
    ext = ".py"
    return lambda path: path.endswith(ext)

fn = make()
`)
	fn := globals["fn"].(*starlark.Function)

	c, err := Extract(fn, "Predicate")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	source := string(c.Data)

	// Should have closure bindings inlined.
	if !strings.Contains(source, `ext = ".py"`) {
		t.Errorf("source missing ext binding:\n%s", source)
	}
	// The lambda should be wrapped in a def.
	if !strings.Contains(source, "def _callable(path):") {
		t.Errorf("source missing def _callable:\n%s", source)
	}
}

// -------------------------------------------------------------------
// Extract named def function
// -------------------------------------------------------------------

func TestExtract_NamedDef(t *testing.T) {
	globals := execScript(t, "named.star", `
def count_files(initial, resource, path):
    if path.endswith(".py"):
        return initial + 1
    return initial
`)
	fn := globals["count_files"].(*starlark.Function)

	c, err := Extract(fn, "file.Reducer")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if c.Name != "count_files" {
		t.Errorf("Name = %q, want %q", c.Name, "count_files")
	}
	if c.FuncName != "count_files" {
		t.Errorf("FuncName = %q, want %q", c.FuncName, "count_files")
	}
	if c.NumParams != 3 {
		t.Errorf("NumParams = %d, want 3", c.NumParams)
	}

	source := string(c.Data)
	if !strings.Contains(source, "def count_files(initial, resource, path):") {
		t.Errorf("source missing def:\n%s", source)
	}
	if !strings.Contains(source, `path.endswith(".py")`) {
		t.Errorf("source missing body:\n%s", source)
	}

	// URI should reflect the function identity.
	want := "mem:callable/file.Reducer/count_files"
	if c.URI() != want {
		t.Errorf("URI() = %q, want %q", c.URI(), want)
	}
}

// -------------------------------------------------------------------
// Extract named def with closure bindings
// -------------------------------------------------------------------

func TestExtract_NamedDefWithClosure(t *testing.T) {
	globals := execScript(t, "defclosure.star", `
def make():
    threshold = 42
    def check(x):
        return x > threshold
    return check

fn = make()
`)
	check := globals["fn"].(*starlark.Function)

	c, err := Extract(check, "Predicate")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	source := string(c.Data)
	if !strings.Contains(source, "threshold = 42") {
		t.Errorf("source missing closure binding:\n%s", source)
	}
	if !strings.Contains(source, "def check(x):") {
		t.Errorf("source missing def:\n%s", source)
	}
	if c.Name != "check" {
		t.Errorf("Name = %q, want %q", c.Name, "check")
	}
}

// -------------------------------------------------------------------
// ExtractWithName — custom name for lambda
// -------------------------------------------------------------------

func TestExtractWithName(t *testing.T) {
	globals := execScript(t, "withname.star", `
fn = lambda x: x * 2
`)
	fn := globals["fn"].(*starlark.Function)

	c, err := ExtractWithName(fn, "Transform", "file.walk_tree.fn")
	if err != nil {
		t.Fatalf("ExtractWithName: %v", err)
	}

	if c.Name != "file.walk_tree.fn" {
		t.Errorf("Name = %q, want %q", c.Name, "file.walk_tree.fn")
	}
	want := "mem:callable/Transform/file.walk_tree.fn"
	if c.URI() != want {
		t.Errorf("URI() = %q, want %q", c.URI(), want)
	}
}

// -------------------------------------------------------------------
// Round-trip: extract → synthetic source → parse → execute → same result
// -------------------------------------------------------------------

func TestExtract_RoundTrip_Lambda(t *testing.T) {
	globals := execScript(t, "roundtrip_lambda.star", `
def make():
    offset = 10
    return lambda x: x + offset

fn = make()
`)
	fn := globals["fn"].(*starlark.Function)

	c, err := Extract(fn, "Transform")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Execute the synthetic source and call the function.
	thread := &starlark.Thread{Name: "roundtrip"}
	opts := &syntax.FileOptions{
		TopLevelControl: true,
		GlobalReassign:  true,
	}
	synGlobals, err := starlark.ExecFileOptions(opts, thread, "<synthetic>", c.Data, nil)
	if err != nil {
		t.Fatalf("exec synthetic source:\n%s\nerror: %v", c.Data, err)
	}

	callable := synGlobals[c.FuncName]
	if callable == nil {
		t.Fatalf("function %q not found in synthetic globals: %v", c.FuncName, synGlobals.Keys())
	}

	result, err := starlark.Call(thread, callable, starlark.Tuple{starlark.MakeInt(5)}, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	got, ok := result.(starlark.Int)
	if !ok {
		t.Fatalf("result is %s, want Int", result.Type())
	}
	if v, _ := got.Int64(); v != 15 {
		t.Errorf("result = %d, want 15", v)
	}
}

func TestExtract_RoundTrip_NamedDef(t *testing.T) {
	globals := execScript(t, "roundtrip_def.star", `
def double(x):
    return x * 2
`)
	fn := globals["double"].(*starlark.Function)

	c, err := Extract(fn, "Transform")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	thread := &starlark.Thread{Name: "roundtrip"}
	opts := &syntax.FileOptions{
		TopLevelControl: true,
		GlobalReassign:  true,
	}
	synGlobals, err := starlark.ExecFileOptions(opts, thread, "<synthetic>", c.Data, nil)
	if err != nil {
		t.Fatalf("exec synthetic source:\n%s\nerror: %v", c.Data, err)
	}

	callable := synGlobals[c.FuncName]
	if callable == nil {
		t.Fatalf("function %q not found in synthetic globals: %v", c.FuncName, synGlobals.Keys())
	}

	result, err := starlark.Call(thread, callable, starlark.Tuple{starlark.MakeInt(7)}, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	got, ok := result.(starlark.Int)
	if !ok {
		t.Fatalf("result is %s, want Int", result.Type())
	}
	if v, _ := got.Int64(); v != 14 {
		t.Errorf("result = %d, want 14", v)
	}
}

func TestExtract_RoundTrip_ClosureDict(t *testing.T) {
	globals := execScript(t, "roundtrip_dict.star", `
def make():
    config = {"multiplier": 3, "offset": 10}
    def transform(x):
        return x * config["multiplier"] + config["offset"]
    return transform

fn = make()
`)
	fn := globals["fn"].(*starlark.Function)

	c, err := Extract(fn, "Transform")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	thread := &starlark.Thread{Name: "roundtrip"}
	opts := &syntax.FileOptions{
		TopLevelControl: true,
		GlobalReassign:  true,
	}
	synGlobals, err := starlark.ExecFileOptions(opts, thread, "<synthetic>", c.Data, nil)
	if err != nil {
		t.Fatalf("exec synthetic source:\n%s\nerror: %v", c.Data, err)
	}

	callable := synGlobals[c.FuncName]
	result, err := starlark.Call(thread, callable, starlark.Tuple{starlark.MakeInt(5)}, nil)
	if err != nil {
		t.Fatalf("call: %v", err)
	}

	got, ok := result.(starlark.Int)
	if !ok {
		t.Fatalf("result is %s, want Int", result.Type())
	}
	// 5 * 3 + 10 = 25
	if v, _ := got.Int64(); v != 25 {
		t.Errorf("result = %d, want 25", v)
	}
}

// -------------------------------------------------------------------
// ValidateArity
// -------------------------------------------------------------------

func TestValidateArity_OK(t *testing.T) {
	globals := execScript(t, "arity_ok.star", `
def f(a, b, c):
    pass
`)
	fn := globals["f"].(*starlark.Function)
	if err := ValidateArity(fn, 2, 3); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateArity_TooManyRequired(t *testing.T) {
	globals := execScript(t, "arity_many.star", `
def f(a, b, c):
    pass
`)
	fn := globals["f"].(*starlark.Function)
	err := ValidateArity(fn, 1, 2)
	if err == nil {
		t.Fatal("expected error for too many required params")
	}
	if !strings.Contains(err.Error(), "requires 3 args") {
		t.Errorf("error = %q, expected mention of required args", err)
	}
}

func TestValidateArity_TooFewParams(t *testing.T) {
	globals := execScript(t, "arity_few.star", `
def f(a):
    pass
`)
	fn := globals["f"].(*starlark.Function)
	err := ValidateArity(fn, 3, 5)
	if err == nil {
		t.Fatal("expected error for too few params")
	}
	if !strings.Contains(err.Error(), "accepts 1 args") {
		t.Errorf("error = %q, expected mention of accepted args", err)
	}
}

func TestValidateArity_WithDefaults(t *testing.T) {
	globals := execScript(t, "arity_defaults.star", `
def f(a, b = 10, c = 20):
    pass
`)
	fn := globals["f"].(*starlark.Function)
	// 1 required (a), 3 total. Target accepts 1-3 → should pass.
	if err := ValidateArity(fn, 1, 3); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Target accepts at most 0 → required (1) > max (0).
	if err := ValidateArity(fn, 0, 0); err == nil {
		t.Fatal("expected error for maxParams=0")
	}
}

func TestValidateArity_Varargs(t *testing.T) {
	globals := execScript(t, "arity_varargs.star", `
def f(a, *args):
    pass
`)
	fn := globals["f"].(*starlark.Function)
	// With varargs, the function can accept any number of args beyond its params.
	// minParams=5 would fail without varargs (only 1 param), but passes with varargs.
	if err := ValidateArity(fn, 5, 10); err != nil {
		t.Errorf("unexpected error with varargs: %v", err)
	}
}
