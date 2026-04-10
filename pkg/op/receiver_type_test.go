// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

// region TEST FIXTURES

// testProvider is a minimal provider with methods covering all MethodKind classifications.
type testProvider struct{}

// Action — returns nothing.
func (p *testProvider) Ping() {}

// FallibleAction — returns error.
func (p *testProvider) Validate(value string) error {
	if value == "" {
		return errors.New("empty")
	}
	return nil
}

// Function — returns a result.
func (p *testProvider) Echo(msg string) string { return msg }

// FallibleFunction — returns (T, error).
func (p *testProvider) Parse(input string) (int, error) {
	if input == "bad" {
		return 0, errors.New("parse error")
	}
	return 42, nil
}

// CompensableFunction — returns (T, U, error).
func (p *testProvider) Create(name string) (string, string, error) {
	return "created:" + name, "undo:" + name, nil
}

// CompensateCreate is the required companion for Create.
func (p *testProvider) CompensateCreate(complement string) error { return nil }

// MultiParam — multiple parameters.
func (p *testProvider) Multi(a string, b int, c bool) string {
	if c {
		return a
	}
	return ""
}

var testProviderType = reflect.TypeFor[*testProvider]()

// endregion

// region newReceiverType

func TestNewReceiverType_WithNamedParameters(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Echo":  {"msg"},
		"Parse": {"input"},
	})

	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	if rt.name != "testProvider" {
		t.Errorf("name = %q, want %q", rt.name, "testProvider")
	}

	if len(rt.methods) != 2 {
		t.Fatalf("methods count = %d, want 2", len(rt.methods))
	}

	// Methods are sorted by name.
	if rt.methods[0].Name() != "Echo" {
		t.Errorf("methods[0] = %q, want Echo", rt.methods[0].Name())
	}
	if rt.methods[1].Name() != "Parse" {
		t.Errorf("methods[1] = %q, want Parse", rt.methods[1].Name())
	}
}

func TestNewReceiverType_WithNilParameters_RegistersAllMethods(t *testing.T) {

	rt, err := newReceiverType(testProviderType, nil)

	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	// Should register all exported methods with positional numeric names.
	if len(rt.methods) == 0 {
		t.Fatal("nil parameters should register all methods, got 0")
	}

	// Check that Echo got a numeric parameter name.
	m, ok := rt.methodMap["Echo"]
	if !ok {
		t.Fatal("Echo not in methodMap")
	}

	params := m.Parameters()
	if len(params) != 1 {
		t.Fatalf("Echo params count = %d, want 1", len(params))
	}
	if params[0].Name != "0" {
		t.Errorf("Echo param name = %q, want %q", params[0].Name, "0")
	}
}

func TestNewReceiverType_WithNilParameters_MultiParam(t *testing.T) {

	rt, err := newReceiverType(testProviderType, nil)

	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	m, ok := rt.methodMap["Multi"]
	if !ok {
		t.Fatal("Multi not in methodMap")
	}

	params := m.Parameters()
	if len(params) != 3 {
		t.Fatalf("Multi params count = %d, want 3", len(params))
	}

	for i, p := range params {
		want := fmt.Sprintf("%d", i)
		if p.Name != want {
			t.Errorf("Multi param[%d] name = %q, want %q", i, p.Name, want)
		}
	}
}

func TestNewReceiverType_WithEmptyParameters_RegistersNoMethods(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{})

	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	if len(rt.methods) != 0 {
		t.Errorf("empty parameters should register 0 methods, got %d", len(rt.methods))
	}
}

func TestNewReceiverType_MethodsSorted(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Parse": {"input"},
		"Echo":  {"msg"},
		"Multi": {"a", "b", "c"},
	})

	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	for i := 1; i < len(rt.methods); i++ {
		if rt.methods[i].Name() < rt.methods[i-1].Name() {
			t.Errorf("methods not sorted: %q before %q", rt.methods[i-1].Name(), rt.methods[i].Name())
		}
	}
}

func TestNewReceiverType_WrongParamCount_ReturnsError(t *testing.T) {

	_, err := newReceiverType(testProviderType, map[string][]string{
		"Echo": {"a", "b"}, // Echo takes 1 param, not 2
	})

	if err == nil {
		t.Fatal("expected error for wrong param count")
	}
}

// endregion

// region ReceiverType interface

func TestReceiverType_MethodByName(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Echo": {"msg"},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	m, ok := rt.MethodByName("Echo")
	if !ok {
		t.Fatal("MethodByName(Echo) returned false")
	}
	if m.Name() != "Echo" {
		t.Errorf("method name = %q, want Echo", m.Name())
	}

	_, ok = rt.MethodByName("Nonexistent")
	if ok {
		t.Error("MethodByName(Nonexistent) returned true")
	}
}

func TestReceiverType_Methods_Iterator(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Echo":  {"msg"},
		"Parse": {"input"},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	var names []string
	for m := range rt.Methods() {
		names = append(names, m.Name())
	}

	if len(names) != 2 {
		t.Fatalf("iterator yielded %d methods, want 2", len(names))
	}
}

func TestReceiverType_ProviderType(t *testing.T) {

	rt, err := newReceiverType(testProviderType, nil)
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	if rt.ProviderType() != testProviderType {
		t.Errorf("ProviderType = %v, want %v", rt.ProviderType(), testProviderType)
	}
}

// endregion

// region Do — MethodAction

func TestDo_Action(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Ping": {},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	result, complement, doErr := rt.Do("Ping", &testProvider{}, []any{})

	if doErr != nil {
		t.Fatalf("Do(Ping): %v", doErr)
	}
	if result.IsValid() {
		t.Error("expected invalid result for Action")
	}
	if complement.IsValid() {
		t.Error("expected invalid complement for Action")
	}
}

// endregion

// region Do — MethodFallibleAction

func TestDo_FallibleAction_Success(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Validate": {"value"},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	_, _, doErr := rt.Do("Validate", &testProvider{}, []any{"ok"})

	if doErr != nil {
		t.Fatalf("Do(Validate, ok): %v", doErr)
	}
}

func TestDo_FallibleAction_Error(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Validate": {"value"},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	_, _, doErr := rt.Do("Validate", &testProvider{}, []any{""})

	if doErr == nil {
		t.Fatal("expected error from Validate with empty string")
	}
	if doErr.Error() != "empty" {
		t.Errorf("error = %q, want %q", doErr.Error(), "empty")
	}
}

// endregion

// region Do — MethodFunction

func TestDo_Function(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Echo": {"msg"},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	result, complement, doErr := rt.Do("Echo", &testProvider{}, []any{"hello"})

	if doErr != nil {
		t.Fatalf("Do(Echo): %v", doErr)
	}
	if !result.IsValid() {
		t.Fatal("expected valid result")
	}
	if result.String() != "hello" {
		t.Errorf("result = %q, want %q", result.String(), "hello")
	}
	if complement.IsValid() {
		t.Error("expected invalid complement for Function")
	}
}

// endregion

// region Do — MethodFallibleFunction

func TestDo_FallibleFunction_Success(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Parse": {"input"},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	result, _, doErr := rt.Do("Parse", &testProvider{}, []any{"good"})

	if doErr != nil {
		t.Fatalf("Do(Parse): %v", doErr)
	}
	if result.Int() != 42 {
		t.Errorf("result = %d, want 42", result.Int())
	}
}

func TestDo_FallibleFunction_Error(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Parse": {"input"},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	_, _, doErr := rt.Do("Parse", &testProvider{}, []any{"bad"})

	if doErr == nil {
		t.Fatal("expected error from Parse(bad)")
	}
}

// endregion

// region Do — MethodCompensableFunction

func TestDo_CompensableFunction(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Create":           {"name"},
		"CompensateCreate": {"complement"},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	result, complement, doErr := rt.Do("Create", &testProvider{}, []any{"foo"})

	if doErr != nil {
		t.Fatalf("Do(Create): %v", doErr)
	}
	if result.String() != "created:foo" {
		t.Errorf("result = %q, want %q", result.String(), "created:foo")
	}
	if !complement.IsValid() {
		t.Fatal("expected valid complement for CompensableFunction")
	}
	if complement.String() != "undo:foo" {
		t.Errorf("complement = %q, want %q", complement.String(), "undo:foo")
	}
}

// endregion

// region Do — nil args

func TestDo_NilArg_BecomesZeroValue(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Echo": {"msg"},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	result, _, doErr := rt.Do("Echo", &testProvider{}, []any{nil})

	if doErr != nil {
		t.Fatalf("Do(Echo, nil): %v", doErr)
	}
	if result.String() != "" {
		t.Errorf("result = %q, want empty string (zero value)", result.String())
	}
}

// endregion

// region Do — unknown method

func TestDo_UnknownMethod_ReturnsError(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Echo": {"msg"},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	_, _, doErr := rt.Do("Nonexistent", &testProvider{}, nil)

	if doErr == nil {
		t.Fatal("expected error for unknown method")
	}
}

// endregion

// region Do — dispatch caching

func TestDo_CachesDispatcher(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Echo": {"msg"},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	// First call — compiles and caches.
	result1, _, err1 := rt.Do("Echo", &testProvider{}, []any{"first"})
	if err1 != nil {
		t.Fatalf("first Do: %v", err1)
	}

	// Second call — uses cache.
	result2, _, err2 := rt.Do("Echo", &testProvider{}, []any{"second"})
	if err2 != nil {
		t.Fatalf("second Do: %v", err2)
	}

	if result1.String() != "first" {
		t.Errorf("first result = %q, want first", result1.String())
	}
	if result2.String() != "second" {
		t.Errorf("second result = %q, want second", result2.String())
	}

	// Verify the dispatcher is in the cache.
	_, ok := rt.dispatchTable.Load("Echo")
	if !ok {
		t.Error("dispatcher not found in cache after two calls")
	}
}

func TestDo_ConcurrentFirstCall(t *testing.T) {

	rt, err := newReceiverType(testProviderType, map[string][]string{
		"Echo": {"msg"},
	})
	if err != nil {
		t.Fatalf("newReceiverType: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 100)

	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _, doErr := rt.Do("Echo", &testProvider{}, []any{"concurrent"})
			if doErr != nil {
				errs <- doErr
				return
			}
			if result.String() != "concurrent" {
				errs <- fmt.Errorf("result = %q, want concurrent", result.String())
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// endregion

// region NewProviderReceiverType

func TestNewProviderReceiverType(t *testing.T) {

	construct := func(ctx *ExecutionContext) (any, error) { return &testProvider{}, nil }

	prt, err := NewProviderReceiverType(testProviderType, construct, RoleModule|RoleAction, map[string][]string{
		"Echo": {"msg"},
	})

	if err != nil {
		t.Fatalf("NewProviderReceiverType: %v", err)
	}

	if prt.Roles() != RoleModule|RoleAction {
		t.Errorf("Roles = %d, want %d", prt.Roles(), RoleModule|RoleAction)
	}

	if prt.Construct() == nil {
		t.Error("Construct() returned nil")
	}

	// Do still works through the embedded receiverType.
	result, _, doErr := prt.Do("Echo", &testProvider{}, []any{"via provider"})
	if doErr != nil {
		t.Fatalf("Do: %v", doErr)
	}
	if result.String() != "via provider" {
		t.Errorf("result = %q, want %q", result.String(), "via provider")
	}
}

// endregion

// region NewResourceReceiverType

func TestNewResourceReceiverType(t *testing.T) {

	construct := func(ctx *ExecutionContext, v any) (any, error) { return nil, nil }

	rrt, err := NewResourceReceiverType(testProviderType, construct, map[string][]string{
		"Echo": {"msg"},
	})

	if err != nil {
		t.Fatalf("NewResourceReceiverType: %v", err)
	}

	if rrt.Construct() == nil {
		t.Error("Construct() returned nil")
	}

	result, _, doErr := rrt.Do("Echo", &testProvider{}, []any{"via resource"})
	if doErr != nil {
		t.Fatalf("Do: %v", doErr)
	}
	if result.String() != "via resource" {
		t.Errorf("result = %q, want %q", result.String(), "via resource")
	}
}

// endregion

// region NewReceiverType (public)

func TestNewReceiverType_Public(t *testing.T) {

	rt, err := NewReceiverType(testProviderType, nil)
	if err != nil {
		t.Fatalf("NewReceiverType: %v", err)
	}

	if rt.ReceiverName() != "testProvider" {
		t.Errorf("ReceiverName = %q, want testProvider", rt.ReceiverName())
	}

	// Should have methods with positional names since nil was passed.
	var count int
	for range rt.Methods() {
		count++
	}
	if count == 0 {
		t.Error("expected methods from nil parameter path")
	}

	// Do should work.
	result, _, doErr := rt.Do("Echo", &testProvider{}, []any{"public"})
	if doErr != nil {
		t.Fatalf("Do: %v", doErr)
	}
	if result.String() != "public" {
		t.Errorf("result = %q, want public", result.String())
	}
}

// endregion

// region receiverName

func TestReceiverName_NonProvider(t *testing.T) {

	name := receiverName(testProviderType)

	if name != "testProvider" {
		t.Errorf("receiverName = %q, want testProvider", name)
	}
}

// endregion