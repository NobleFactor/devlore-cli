// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"slices"
	"testing"

	"go.starlark.net/starlark"
)

// --- Test provider types ---

// testProvider mimics a real provider with various method signatures.
type testProvider struct {
	Root string
}

// Simple return patterns.
func (p *testProvider) Greet(name string) string { return "hello " + name }
func (p *testProvider) Exists(path string) bool  { return path == "/exists" }
func (p *testProvider) Count(items []string) int { return len(items) }
func (p *testProvider) Divide(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("division by zero")
	}
	return a / b, nil
}
func (p *testProvider) ListFiles(dir string) ([]string, error) {
	return []string{dir + "/a", dir + "/b"}, nil
}

// Void return.
func (p *testProvider) Noop() {}

// Error-only return.
func (p *testProvider) Validate(s string) error {
	if s == "" {
		return errors.New("empty string")
	}
	return nil
}

// Compensable pattern: (T, map[string]any, error).
func (p *testProvider) Write(path, content string) (string, map[string]any, error) {
	return path, map[string]any{"path": path, "content": content}, nil
}

// Compensate method — should be excluded.
func (p *testProvider) CompensateWrite(_ map[string]any) error { return nil }

// NoResult pattern: (NoResult, map[string]any, error).
func (p *testProvider) Remove(path string) (NoResult, map[string]any, error) {
	return NoResult{}, map[string]any{"path": path}, nil
}

// Compensate method for Remove.
func (p *testProvider) CompensateRemove(_ map[string]any) error { return nil }

// Struct return.
func (p *testProvider) GetPoint() testPoint { return testPoint{X: 10, Y: 20} }

// Method not in params — should not be exposed.
func (p *testProvider) InternalHelper() string { return "hidden" }

// Optional params.
func (p *testProvider) Search(query string, limit int) ([]string, error) {
	results := make([]string, 0, limit)
	for i := range limit {
		results = append(results, query+string(rune('0'+i)))
	}
	return results, nil
}

// Multi-return with bool.
func (p *testProvider) TryParse(s string) (int, bool) {
	if s == "42" {
		return 42, true
	}
	return 0, false
}

// Variadic method.
func (p *testProvider) Join(parts ...string) string {
	result := ""
	for i, part := range parts {
		if i > 0 {
			result += "/"
		}
		result += part
	}
	return result
}

// Variadic with a named param before the variadic.
func (p *testProvider) Prefix(pfx string, parts ...string) string {
	result := pfx
	for _, part := range parts {
		result += "/" + part
	}
	return result
}

var testParams = MethodParams{
	"Greet":     {"name"},
	"Exists":    {"path"},
	"Count":     {"items"},
	"Divide":    {"a", "b"},
	"ListFiles": {"dir"},
	"Noop":      {},
	"Validate":  {"s"},
	"Write":     {"path", "content"},
	"Remove":    {"path"},
	"GetPoint":  {},
	"Search":    {"query", "limit?"},
	"TryParse":  {"s"},
	"Join":      {"*parts"},
	"Prefix":    {"pfx", "*parts"},
}

// wrapTestReceiver registers params and wraps a provider in one step.
// Test-only helper — production code relies on RegisterReflectedActions.
func wrapTestReceiver(name string, provider any, params MethodParams) *ReflectedReceiver {
	registerReceiverParamsReflect(name, provider, params)
	return WrapReceiver(name, provider)
}

// --- WrapReceiver tests ---

func TestWrapReceiver_MethodDiscovery(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)

	names := r.AttrNames()

	// Verify expected methods are present.
	expected := []string{
		"count", "divide", "exists", "get_point", "greet",
		"join", "list_files", "noop", "prefix", "remove", "search", "try_parse", "validate", "write",
	}
	if len(names) != len(expected) {
		t.Fatalf("AttrNames() = %v (len %d), want %v (len %d)", names, len(names), expected, len(expected))
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("AttrNames()[%d] = %q, want %q", i, names[i], name)
		}
	}
}

func TestWrapReceiver_CompensateExcluded(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)

	_, err := r.Attr("compensate_write")
	if err == nil {
		t.Error("Compensate method should not be exposed")
	}
}

func TestWrapReceiver_UnlistedMethodExcluded(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)

	_, err := r.Attr("internal_helper")
	if err == nil {
		t.Error("Method not in MethodParams should not be exposed")
	}
}

func TestWrapReceiver_AttrReturnsBuiltin(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)

	val, err := r.Attr("greet")
	if err != nil {
		t.Fatalf("Attr(greet) error: %v", err)
	}
	if val == nil {
		t.Fatal("Attr(greet) returned nil")
	}
	if val.Type() != "builtin_function_or_method" {
		t.Errorf("type = %q, want builtin_function_or_method", val.Type())
	}
}

// --- Method call tests ---

func callMethod(t *testing.T, r *ReflectedReceiver, name string, args ...starlark.Value) starlark.Value {
	t.Helper()
	attr, err := r.Attr(name)
	if err != nil {
		t.Fatalf("Attr(%s) error: %v", name, err)
	}
	builtin, ok := attr.(*starlark.Builtin)
	if !ok {
		t.Fatalf("Attr(%s) = %T, want *starlark.Builtin", name, attr)
	}
	result, err := builtin.CallInternal(nil, starlark.Tuple(args), nil)
	if err != nil {
		t.Fatalf("%s() error: %v", name, err)
	}
	return result
}

func callMethodKw(t *testing.T, r *ReflectedReceiver, name string, kwargs []starlark.Tuple) starlark.Value {
	t.Helper()
	attr, err := r.Attr(name)
	if err != nil {
		t.Fatalf("Attr(%s) error: %v", name, err)
	}
	builtin := attr.(*starlark.Builtin)
	result, err := builtin.CallInternal(nil, nil, kwargs)
	if err != nil {
		t.Fatalf("%s() error: %v", name, err)
	}
	return result
}

func callMethodArgsKw(t *testing.T, r *ReflectedReceiver, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	t.Helper()
	attr, err := r.Attr(name)
	if err != nil {
		t.Fatalf("Attr(%s) error: %v", name, err)
	}
	builtin := attr.(*starlark.Builtin)
	return builtin.CallInternal(nil, args, kwargs)
}

func callMethodErr(t *testing.T, r *ReflectedReceiver, name string, args ...starlark.Value) error {
	t.Helper()
	attr, err := r.Attr(name)
	if err != nil {
		t.Fatalf("Attr(%s) error: %v", name, err)
	}
	builtin := attr.(*starlark.Builtin)
	_, err = builtin.CallInternal(nil, starlark.Tuple(args), nil)
	return err
}

func TestCall_StringReturn(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethod(t, r, "greet", starlark.String("world"))

	s, ok := starlark.AsString(result)
	if !ok || s != "hello world" {
		t.Errorf("greet(world) = %v, want 'hello world'", result)
	}
}

func TestCall_BoolReturn(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)

	result := callMethod(t, r, "exists", starlark.String("/exists"))
	if result != starlark.True {
		t.Errorf("exists(/exists) = %v, want True", result)
	}

	result = callMethod(t, r, "exists", starlark.String("/nope"))
	if result != starlark.False {
		t.Errorf("exists(/nope) = %v, want False", result)
	}
}

func TestCall_IntReturn(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	list := starlark.NewList([]starlark.Value{starlark.String("a"), starlark.String("b")})
	result := callMethod(t, r, "count", list)

	si, ok := result.(starlark.Int)
	if !ok {
		t.Fatalf("count() = %T, want Int", result)
	}
	i, _ := si.Int64()
	if i != 2 {
		t.Errorf("count([a,b]) = %d, want 2", i)
	}
}

func TestCall_TupleReturn(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethod(t, r, "divide", starlark.MakeInt(10), starlark.MakeInt(3))

	si, ok := result.(starlark.Int)
	if !ok {
		t.Fatalf("divide() = %T, want Int", result)
	}
	i, _ := si.Int64()
	if i != 3 {
		t.Errorf("divide(10,3) = %d, want 3", i)
	}
}

func TestCall_ErrorPropagation(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	err := callMethodErr(t, r, "divide", starlark.MakeInt(1), starlark.MakeInt(0))
	if err == nil {
		t.Fatal("divide(1,0) should return error")
	}
	if err.Error() != "division by zero" {
		t.Errorf("error = %q, want 'division by zero'", err.Error())
	}
}

func TestCall_SliceReturn(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethod(t, r, "list_files", starlark.String("/tmp"))

	list, ok := result.(*starlark.List)
	if !ok {
		t.Fatalf("list_files() = %T, want List", result)
	}
	if list.Len() != 2 {
		t.Fatalf("len = %d, want 2", list.Len())
	}
	s, _ := starlark.AsString(list.Index(0))
	if s != "/tmp/a" {
		t.Errorf("[0] = %q, want '/tmp/a'", s)
	}
}

func TestCall_VoidReturn(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethod(t, r, "noop")
	if result != starlark.None {
		t.Errorf("noop() = %v, want None", result)
	}
}

func TestCall_ErrorOnlyReturn(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)

	// Success case.
	result := callMethod(t, r, "validate", starlark.String("ok"))
	if result != starlark.None {
		t.Errorf("validate('ok') = %v, want None", result)
	}

	// Error case.
	err := callMethodErr(t, r, "validate", starlark.String(""))
	if err == nil {
		t.Fatal("validate('') should return error")
	}
}

func TestCall_CompensableReturn(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethod(t, r, "write", starlark.String("/tmp/f"), starlark.String("data"))

	// Compensable methods return (T, map, error). Bridge returns marshal(T),
	// discarding the compensation state.
	s, ok := starlark.AsString(result)
	if !ok || s != "/tmp/f" {
		t.Errorf("write() = %v, want '/tmp/f'", result)
	}
}

func TestCall_StructReturn(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethod(t, r, "get_point")

	// testPoint has X and Y fields.
	if result.Type() != "struct" {
		t.Fatalf("type = %q, want struct", result.Type())
	}

	ha, ok := result.(starlark.HasAttrs)
	if !ok {
		t.Fatal("result does not implement HasAttrs")
	}
	xv, _ := ha.Attr("x")
	xi, _ := xv.(starlark.Int)
	x, _ := xi.Int64()
	if x != 10 {
		t.Errorf("x = %d, want 10", x)
	}
}

func TestCall_OptionalParam(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)

	// With optional param provided.
	result := callMethodKw(t, r, "search", []starlark.Tuple{
		{starlark.String("query"), starlark.String("test")},
		{starlark.String("limit"), starlark.MakeInt(3)},
	})
	list, ok := result.(*starlark.List)
	if !ok {
		t.Fatalf("search() = %T, want List", result)
	}
	if list.Len() != 3 {
		t.Errorf("len = %d, want 3", list.Len())
	}

	// Without optional param (limit defaults to 0).
	result = callMethodKw(t, r, "search", []starlark.Tuple{
		{starlark.String("query"), starlark.String("test")},
	})
	list, ok = result.(*starlark.List)
	if !ok {
		t.Fatalf("search() = %T, want List", result)
	}
	if list.Len() != 0 {
		t.Errorf("len = %d, want 0 (default)", list.Len())
	}
}

func TestCall_KwargsSupported(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethodKw(t, r, "greet", []starlark.Tuple{
		{starlark.String("name"), starlark.String("kwargs")},
	})
	s, _ := starlark.AsString(result)
	if s != "hello kwargs" {
		t.Errorf("greet(name='kwargs') = %q, want 'hello kwargs'", s)
	}
}

func TestCall_WrongArgType(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	err := callMethodErr(t, r, "greet", starlark.MakeInt(42))
	if err == nil {
		t.Fatal("greet(42) should fail with type error")
	}
}

func TestCall_MissingRequiredArg(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	err := callMethodErr(t, r, "greet")
	if err == nil {
		t.Fatal("greet() should fail with missing arg")
	}
}

func TestCall_MultiReturn_NoError(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	// TryParse returns (int, bool) — bridge marshals first return.
	result := callMethod(t, r, "try_parse", starlark.String("42"))

	si, ok := result.(starlark.Int)
	if !ok {
		t.Fatalf("try_parse('42') = %T, want Int", result)
	}
	i, _ := si.Int64()
	if i != 42 {
		t.Errorf("try_parse('42') = %d, want 42", i)
	}
}

// --- Override tests ---

func TestOverride_ReplacesMethod(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)

	r.Override("greet", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("overridden"), nil
	})

	result := callMethod(t, r, "greet", starlark.String("ignored"))
	s, _ := starlark.AsString(result)
	if s != "overridden" {
		t.Errorf("overridden greet = %q, want 'overridden'", s)
	}
}

func TestOverride_AddsNewMethod(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)

	r.Override("custom_method", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("custom"), nil
	})

	// Verify it's in attr list.
	if !slices.Contains(r.AttrNames(), "custom_method") {
		t.Error("custom_method not in AttrNames()")
	}

	result := callMethod(t, r, "custom_method")
	s, _ := starlark.AsString(result)
	if s != "custom" {
		t.Errorf("custom_method = %q, want 'custom'", s)
	}
}

// --- Starlark interface tests ---

func TestReflectedReceiver_StarlarkValue(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)

	if r.String() != "test" {
		t.Errorf("String() = %q, want 'test'", r.String())
	}
	if r.Type() != "test" {
		t.Errorf("Type() = %q, want 'test'", r.Type())
	}
	if r.Truth() != starlark.True {
		t.Error("Truth() should be True")
	}
}

// --- NoResult tests ---

func TestCall_NoResultReturn(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethod(t, r, "remove", starlark.String("/tmp/file"))

	if result != starlark.None {
		t.Errorf("remove() = %v, want None", result)
	}
}

// --- Catalog integration tests ---

func TestWrapReceiver_CatalogShadow(t *testing.T) {
	catalog := NewResourceCatalog()
	r := wrapTestReceiver("test", &actionProvider{}, MethodParams{
		"Create": {"path"},
	})
	r.SetCatalog(catalog)

	callMethod(t, r, "create", starlark.String("/tmp/new"))

	if catalog.Len() != 1 {
		t.Errorf("catalog len = %d, want 1 (Resource result should be shadowed)", catalog.Len())
	}

	// Verify the originID is the qualified method name, not empty.
	id := catalog.Current("file:///tmp/new")
	if id == "" {
		t.Fatal("catalog has no entry for file:///tmp/new")
	}
	entry, ok := catalog.Lookup(id)
	if !ok {
		t.Fatalf("catalog.Lookup(%q) failed", id)
	}
	base := entry.resourceBase()
	if base.originID != "test.create" {
		t.Errorf("originID = %q, want %q", base.originID, "test.create")
	}
}

func TestWrapReceiver_NoCatalog_NoShadow(t *testing.T) {
	r := wrapTestReceiver("test", &actionProvider{}, MethodParams{
		"Create": {"path"},
	})
	// No catalog set — should not panic.
	callMethod(t, r, "create", starlark.String("/tmp/new"))
}

func TestWrapReceiver_CatalogError_NoShadow(t *testing.T) {
	catalog := NewResourceCatalog()
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	r.SetCatalog(catalog)

	// Validate("") returns error — should not shadow.
	err := callMethodErr(t, r, "validate", starlark.String(""))
	if err == nil {
		t.Fatal("validate('') should return error")
	}
	if catalog.Len() != 0 {
		t.Errorf("catalog len = %d, want 0 (error result should not be shadowed)", catalog.Len())
	}
}

func TestWrapReceiver_CatalogNonResource_NoShadow(t *testing.T) {
	catalog := NewResourceCatalog()
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	r.SetCatalog(catalog)

	// Greet returns string — not a Resource, should not be shadowed.
	callMethod(t, r, "greet", starlark.String("world"))

	if catalog.Len() != 0 {
		t.Errorf("catalog len = %d, want 0 (non-Resource result should not be shadowed)", catalog.Len())
	}
}

func TestWrapReceiver_CatalogNoResult_NoShadow(t *testing.T) {
	catalog := NewResourceCatalog()
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	r.SetCatalog(catalog)

	// Remove returns NoResult — nothing to shadow.
	callMethod(t, r, "remove", starlark.String("/tmp/file"))

	if catalog.Len() != 0 {
		t.Errorf("catalog len = %d, want 0 (NoResult should not be shadowed)", catalog.Len())
	}
}

// --- Variadic tests ---

func TestCall_Variadic_Positional(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethod(t, r, "join", starlark.String("a"), starlark.String("b"), starlark.String("c"))

	s, ok := starlark.AsString(result)
	if !ok || s != "a/b/c" {
		t.Errorf("join('a','b','c') = %v, want 'a/b/c'", result)
	}
}

func TestCall_Variadic_Keyword(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethodKw(t, r, "join", []starlark.Tuple{
		{starlark.String("parts"), starlark.NewList([]starlark.Value{
			starlark.String("x"), starlark.String("y"),
		})},
	})

	s, ok := starlark.AsString(result)
	if !ok || s != "x/y" {
		t.Errorf("join(parts=['x','y']) = %v, want 'x/y'", result)
	}
}

func TestCall_Variadic_Empty(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethod(t, r, "join")

	s, ok := starlark.AsString(result)
	if !ok || s != "" {
		t.Errorf("join() = %v, want ''", result)
	}
}

func TestCall_Variadic_Single(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethod(t, r, "join", starlark.String("only"))

	s, ok := starlark.AsString(result)
	if !ok || s != "only" {
		t.Errorf("join('only') = %v, want 'only'", result)
	}
}

func TestCall_Variadic_BothPositionalAndKeyword_Error(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	_, err := callMethodArgsKw(t, r, "join",
		starlark.Tuple{starlark.String("a")},
		[]starlark.Tuple{
			{starlark.String("parts"), starlark.NewList([]starlark.Value{starlark.String("b")})},
		},
	)
	if err == nil {
		t.Fatal("join with both positional and keyword variadic should fail")
	}
}

func TestCall_Variadic_WithNamedParam_Positional(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	// prefix("root", "a", "b") → pfx="root", parts=["a", "b"]
	result := callMethod(t, r, "prefix",
		starlark.String("root"), starlark.String("a"), starlark.String("b"))

	s, ok := starlark.AsString(result)
	if !ok || s != "root/a/b" {
		t.Errorf("prefix('root','a','b') = %v, want 'root/a/b'", result)
	}
}

func TestCall_Variadic_WithNamedParam_Keyword(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	result := callMethodKw(t, r, "prefix", []starlark.Tuple{
		{starlark.String("pfx"), starlark.String("root")},
		{starlark.String("parts"), starlark.NewList([]starlark.Value{
			starlark.String("c"), starlark.String("d"),
		})},
	})

	s, ok := starlark.AsString(result)
	if !ok || s != "root/c/d" {
		t.Errorf("prefix(pfx='root', parts=['c','d']) = %v, want 'root/c/d'", result)
	}
}

func TestCall_Variadic_WithNamedParam_NoVariadicArgs(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	// prefix("root") → pfx="root", parts=[]
	result := callMethod(t, r, "prefix", starlark.String("root"))

	s, ok := starlark.AsString(result)
	if !ok || s != "root" {
		t.Errorf("prefix('root') = %v, want 'root'", result)
	}
}

func TestCall_Variadic_KeywordNotList_Error(t *testing.T) {
	r := wrapTestReceiver("test", &testProvider{}, testParams)
	_, err := callMethodArgsKw(t, r, "join",
		nil,
		[]starlark.Tuple{
			{starlark.String("parts"), starlark.String("not-a-list")},
		},
	)
	if err == nil {
		t.Fatal("join(parts='not-a-list') should fail — keyword variadic must be a list")
	}
}
