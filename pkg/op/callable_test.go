// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// mockCallableResource implements CallableResource for testing.
type mockCallableResource struct {
	ResourceBase
	funcType string
	initErr  error
	fn       starlark.Callable
}

func (m *mockCallableResource) String() string                { return "mock" }
func (m *mockCallableResource) Init(_ *starlark.Thread) error { return m.initErr }
func (m *mockCallableResource) Fn() starlark.Callable         { return m.fn }
func (m *mockCallableResource) FuncTypeName() string          { return m.funcType }

func TestCallableResourceInterface(t *testing.T) {
	var _ CallableResource = (*mockCallableResource)(nil)
}

func TestExtractCallable_NoExtractor(t *testing.T) {
	// Save and restore the extractor.
	saved := callableExtractorFn
	callableExtractorFn = nil
	defer func() { callableExtractorFn = saved }()

	_, err := ExtractCallable(nil, "TestType", nil)
	if err == nil {
		t.Fatal("expected error when no extractor registered")
	}
}

func TestExtractCallable_WithExtractor(t *testing.T) {
	saved := callableExtractorFn
	defer func() { callableExtractorFn = saved }()

	called := false
	RegisterCallableExtractor(func(fn *starlark.Function, funcType string, _ Root) (CallableResource, error) {
		called = true
		if funcType != "file.Reducer" {
			t.Errorf("funcType = %q, want %q", funcType, "file.Reducer")
		}
		m := &mockCallableResource{funcType: funcType}
		m.SetURI("mem:callable/file.Reducer/test")
		return m, nil
	})

	cr, err := ExtractCallable(nil, "file.Reducer", nil)
	if err != nil {
		t.Fatalf("ExtractCallable: %v", err)
	}
	if !called {
		t.Error("extractor was not called")
	}
	if cr.FuncTypeName() != "file.Reducer" {
		t.Errorf("FuncTypeName = %q, want %q", cr.FuncTypeName(), "file.Reducer")
	}
}

func TestIsCallableResource(t *testing.T) {
	m := &mockCallableResource{}
	if !isCallableResource(m) {
		t.Error("mockCallableResource should satisfy isCallableResource")
	}
	if isCallableResource("not a callable") {
		t.Error("string should not satisfy isCallableResource")
	}
}

func TestIsFuncType(t *testing.T) {
	type Reducer func(int) int
	if !isFuncType(reflect.TypeOf(Reducer(nil))) {
		t.Error("Reducer should be a func type")
	}
	if isFuncType(reflect.TypeOf("string")) {
		t.Error("string should not be a func type")
	}
}

func TestValidateSlotType_CallableToFunc(t *testing.T) {
	type Reducer func(int) int
	m := &mockCallableResource{funcType: "file.Reducer"}
	err := validateSlotType(m, reflect.TypeOf(Reducer(nil)))
	if err != nil {
		t.Errorf("validateSlotType should accept CallableResource for func type: %v", err)
	}
}

// ── Generic callable adapter ─────────────────────────────────────────────────

// compileStarlarkFn compiles a Starlark source and returns the named function.
func compileStarlarkFn(t *testing.T, source, funcName string) *starlark.Function {
	t.Helper()
	thread := &starlark.Thread{Name: "test"}
	_, prog, err := starlark.SourceProgramOptions(
		&syntax.FileOptions{}, "<test>", source, func(string) bool { return false },
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	globals, err := prog.Init(thread, nil)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	fn, ok := globals[funcName]
	if !ok {
		t.Fatalf("function %q not found", funcName)
	}
	return fn.(*starlark.Function)
}

// liveCallableResource creates a mockCallableResource with a live function.
func liveCallableResource(t *testing.T, source, funcName string) *mockCallableResource {
	t.Helper()
	fn := compileStarlarkFn(t, source, funcName)
	return &mockCallableResource{
		funcType: "TestType",
		fn:       fn,
	}
}

func TestBuildCallableFunc_SimpleReturn(t *testing.T) {
	// Starlark function: def add(a, b): return a + b
	fn := compileStarlarkFn(t, "def add(a, b):\n    return a + b\n", "add")
	thread := &starlark.Thread{Name: "test"}

	type addFunc func(any, any) (any, error)
	targetType := reflect.TypeOf(addFunc(nil))

	adapted, err := buildCallableFunc(fn, thread, targetType)
	if err != nil {
		t.Fatalf("buildCallableFunc: %v", err)
	}

	f := adapted.(addFunc)
	result, err := f(3, 4)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if result != 7 {
		t.Errorf("result = %v, want 7", result)
	}
}

func TestBuildCallableFunc_FullSignature(t *testing.T) {
	// Starlark function must accept all 4 params — full Go signature.
	fn := compileStarlarkFn(t, `
def reducer(initial, resource, path, stack):
    if initial == None:
        return [path]
    return initial + [path]
`, "reducer")
	thread := &starlark.Thread{Name: "test"}

	// Go target: func(any, any, string, *int) (any, error)
	// All 4 params are passed to the Starlark function.
	type reducerFunc func(any, any, string, *int) (any, error)
	targetType := reflect.TypeOf(reducerFunc(nil))

	adapted, err := buildCallableFunc(fn, thread, targetType)
	if err != nil {
		t.Fatalf("buildCallableFunc: %v", err)
	}

	f := adapted.(reducerFunc)
	dummy := 42

	result, err := f(nil, "res1", "a.txt", &dummy)
	if err != nil {
		t.Fatalf("call 1: %v", err)
	}

	result, err = f(result, "res2", "b.txt", &dummy)
	if err != nil {
		t.Fatalf("call 2: %v", err)
	}

	paths, ok := result.([]string)
	if !ok {
		t.Fatalf("result type = %T, want []string", result)
	}
	if len(paths) != 2 || paths[0] != "a.txt" || paths[1] != "b.txt" {
		t.Errorf("result = %v, want [a.txt b.txt]", paths)
	}
}

func TestBuildCallableFunc_StarlarkError(t *testing.T) {
	fn := compileStarlarkFn(t, "def bad(x):\n    fail(\"boom\")\n", "bad")
	thread := &starlark.Thread{Name: "test"}

	type f func(any) (any, error)
	adapted, err := buildCallableFunc(fn, thread, reflect.TypeOf(f(nil)))
	if err != nil {
		t.Fatalf("buildCallableFunc: %v", err)
	}

	_, err = adapted.(f)(42)
	if err == nil {
		t.Fatal("expected error from failing Starlark function")
	}
}

func TestInitCallableSlots_ReplacesCallable(t *testing.T) {
	m := liveCallableResource(t, "def double(x):\n    return x * 2\n", "double")
	thread := &starlark.Thread{Name: "test"}

	// Simulate a method type: func(receiver, fn func(any)(any,error))
	type targetFunc func(any) (any, error)
	type fakeProvider struct{}
	type fakeMethod func(fakeProvider, targetFunc)
	methodType := reflect.TypeOf(fakeMethod(nil))

	ctx := &Context{
		Context: context.Background(),
		Thread:  thread,
		Writer:  &bytes.Buffer{},
	}

	slots := map[string]any{"fn": m}
	paramNames := []string{"fn"}

	err := initCallableSlots(ctx, slots, methodType, paramNames)
	if err != nil {
		t.Fatalf("initCallableSlots: %v", err)
	}

	// The slot should now contain a Go func, not a CallableResource.
	if _, ok := slots["fn"].(CallableResource); ok {
		t.Fatal("slot still contains CallableResource after initCallableSlots")
	}

	// Call the adapted function.
	adapted, ok := slots["fn"].(targetFunc)
	if !ok {
		t.Fatalf("slot type = %T, want targetFunc", slots["fn"])
	}
	result, err := adapted(5)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if result != 10 {
		t.Errorf("result = %v, want 10", result)
	}
}

func TestInitCallableSlots_SkipsNonCallable(t *testing.T) {
	ctx := &Context{
		Context: context.Background(),
		Thread:  &starlark.Thread{Name: "test"},
		Writer:  &bytes.Buffer{},
	}

	// String slot targeting a string param — should be untouched.
	type fakeMethod func(struct{}, string)
	methodType := reflect.TypeOf(fakeMethod(nil))

	slots := map[string]any{"path": "/tmp/foo"}
	err := initCallableSlots(ctx, slots, methodType, []string{"path"})
	if err != nil {
		t.Fatalf("initCallableSlots: %v", err)
	}
	if slots["path"] != "/tmp/foo" {
		t.Errorf("non-callable slot was modified: %v", slots["path"])
	}
}
