// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"testing"

	"go.starlark.net/starlark"
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

	_, err := ExtractCallable(nil, "TestType")
	if err == nil {
		t.Fatal("expected error when no extractor registered")
	}
}

func TestExtractCallable_WithExtractor(t *testing.T) {
	saved := callableExtractorFn
	defer func() { callableExtractorFn = saved }()

	called := false
	RegisterCallableExtractor(func(fn *starlark.Function, funcType string) (CallableResource, error) {
		called = true
		if funcType != "file.Reducer" {
			t.Errorf("funcType = %q, want %q", funcType, "file.Reducer")
		}
		m := &mockCallableResource{funcType: funcType}
		m.SetURI("mem:callable/file.Reducer/test")
		return m, nil
	})

	cr, err := ExtractCallable(nil, "file.Reducer")
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
