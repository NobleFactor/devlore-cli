// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package bind

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

// region TEST TYPES

// testParagraph is a struct with fields and methods for testing method exposure.
type testParagraph struct {
	Words []string `starlark:"words"`
}

func (p *testParagraph) Text() string {
	return strings.Join(p.Words, " ")
}

func (p *testParagraph) WordCount() int {
	return len(p.Words)
}

// testWithError has a method that returns (T, error).
type testWithError struct {
	ShouldFail bool `starlark:"should_fail"`
}

func (t *testWithError) Compute() (string, error) {
	if t.ShouldFail {
		return "", errors.New("computation failed")
	}
	return "ok", nil
}

// testUnsupportedSig has methods with unsupported signatures (should be ignored).
type testUnsupportedSig struct {
	Name string `starlark:"name"`
}

func (t *testUnsupportedSig) Transform(input string) string {
	return strings.ToUpper(input)
}

func (t *testUnsupportedSig) MultiReturn() (string, int) {
	return "hello", 42
}

func (t *testUnsupportedSig) NoReturn() {
}

func (t *testUnsupportedSig) ErrorOnly() error {
	return nil
}

// testStringer implements fmt.Stringer — String() should drive representation, not be an attr.
type testStringer struct {
	Value string `starlark:"value"`
}

func (t *testStringer) String() string {
	return "custom:" + t.Value
}

// testStarlarkValue implements starlark.Value with no exported fields.
type testStarlarkValue struct {
	data string
}

func (v *testStarlarkValue) String() string        { return v.data }
func (v *testStarlarkValue) Type() string          { return "test_starlark_value" }
func (v *testStarlarkValue) Freeze()               {}
func (v *testStarlarkValue) Truth() starlark.Bool  { return starlark.True }
func (v *testStarlarkValue) Hash() (uint32, error) { return 0, nil }

// testParamMethod has a method with one string parameter, registered via typeParamsRegistry.
type testParamMethod struct {
	Prefix string `starlark:"prefix"`
}

func (t *testParamMethod) Greet(name string) string {
	return t.Prefix + " " + name
}

func (t *testParamMethod) GreetErr(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("name required")
	}
	return t.Prefix + " " + name, nil
}

// ZeroArg is a zero-arg method that should remain auto-invoked.
func (t *testParamMethod) ZeroArg() string {
	return "zero"
}


// endregion

// region TYPEINFO TESTS

func TestGetTypeInfo_AttrListIncludesFields(t *testing.T) {
	info := getTypeInfo(reflect.TypeOf(testParagraph{}))

	want := []string{"words"}
	if !reflect.DeepEqual(info.attrList, want) {
		t.Errorf("attrList = %v, want %v", info.attrList, want)
	}
}

func TestGetTypeInfo_CachesTypeName(t *testing.T) {
	info := getTypeInfo(reflect.TypeOf(testParagraph{}))

	if info.typeName != "test_paragraph" {
		t.Errorf("typeName = %q, want %q", info.typeName, "test_paragraph")
	}
}

// endregion

// region STRUCTVALUE ATTR TESTS

func TestStructValue_FieldAttr(t *testing.T) {
	p := &testParagraph{Words: []string{"hello", "world"}}
	sv, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	ha, ok := sv.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("expected HasAttrs, got %T", sv)
	}

	wordsVal, err := ha.Attr("words")
	if err != nil {
		t.Fatalf("Attr(words) error: %v", err)
	}
	list, ok := wordsVal.(*starlark.List)
	if !ok {
		t.Fatalf("expected *starlark.List, got %T", wordsVal)
	}
	if list.Len() != 2 {
		t.Errorf("words len = %d, want 2", list.Len())
	}
}

func TestStructValue_NoSuchAttr(t *testing.T) {
	p := &testParagraph{Words: []string{"a"}}
	sv, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	ha := sv.(starlark.HasAttrs)

	_, err = ha.Attr("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent attr")
	}
}

// endregion

// region STRING REPRESENTATION TESTS

func TestStructValue_StringWithStringer(t *testing.T) {
	v := &testStringer{Value: "hello"}
	sv, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if sv.String() != "custom:hello" {
		t.Errorf("String() = %q, want %q", sv.String(), "custom:hello")
	}
}

// endregion

// region STARLARK.VALUE PASSTHROUGH TESTS

func TestMarshal_StarlarkValuePassthrough(t *testing.T) {
	v := &testStarlarkValue{data: "pass-through"}
	sv, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	// Should return the value directly, not wrap in StructValue.
	if _, ok := sv.(*StructValue); ok {
		t.Error("expected starlark.Value passthrough, got *StructValue")
	}
	if sv.String() != "pass-through" {
		t.Errorf("String() = %q, want %q", sv.String(), "pass-through")
	}
}

// endregion

// region UNMARSHAL ROUND-TRIP TESTS

func TestStructValue_UnmarshalRoundTrip(t *testing.T) {
	original := testPoint{X: 42, Y: 99}
	sv, err := Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var result testPoint
	if err := unmarshal(sv, &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if result != original {
		t.Errorf("round-trip: got %+v, want %+v", result, original)
	}
}

// endregion

// region PARAMETERIZED METHOD TESTS

func TestStructValue_ParameterizedMethodReturnsBuiltin(t *testing.T) {
	v := &testParamMethod{Prefix: "Hello"}
	sv, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	ha := sv.(starlark.HasAttrs)

	attr, err := ha.Attr("greet")
	if err != nil {
		t.Fatalf("Attr(greet) error: %v", err)
	}
	if _, ok := attr.(*starlark.Builtin); !ok {
		t.Fatalf("expected *starlark.Builtin, got %T", attr)
	}
}

func TestStructValue_ParameterizedMethodPositionalArg(t *testing.T) {
	v := &testParamMethod{Prefix: "Hello"}
	sv, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	ha := sv.(starlark.HasAttrs)

	attr, err := ha.Attr("greet")
	if err != nil {
		t.Fatalf("Attr(greet) error: %v", err)
	}
	builtin := attr.(*starlark.Builtin)
	result, err := starlark.Call(&starlark.Thread{}, builtin, starlark.Tuple{starlark.String("World")}, nil)
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	if s, ok := result.(starlark.String); !ok || string(s) != "Hello World" {
		t.Errorf("result = %v, want %q", result, "Hello World")
	}
}

func TestStructValue_ParameterizedMethodErrorPropagation(t *testing.T) {
	v := &testParamMethod{Prefix: "Hi"}
	sv, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	ha := sv.(starlark.HasAttrs)

	attr, err := ha.Attr("greet_err")
	if err != nil {
		t.Fatalf("Attr(greet_err) error: %v", err)
	}
	builtin := attr.(*starlark.Builtin)
	_, err = starlark.Call(&starlark.Thread{}, builtin, starlark.Tuple{starlark.String("")}, nil)
	if err == nil {
		t.Fatal("expected error from greet_err with empty name")
	}
	if !strings.Contains(err.Error(), "name required") {
		t.Errorf("error = %q, want to contain %q", err, "name required")
	}
}

func TestStructValue_ParameterizedMethodMissingArg(t *testing.T) {
	v := &testParamMethod{Prefix: "Hi"}
	sv, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	ha := sv.(starlark.HasAttrs)

	attr, err := ha.Attr("greet")
	if err != nil {
		t.Fatalf("Attr(greet) error: %v", err)
	}
	builtin := attr.(*starlark.Builtin)
	_, err = starlark.Call(&starlark.Thread{}, builtin, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing required arg")
	}
}

// endregion
