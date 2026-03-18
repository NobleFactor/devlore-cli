// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
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

// endregion

// region METHOD DISCOVERY TESTS

func TestGetTypeInfo_DiscoversMethods(t *testing.T) {
	info := getTypeInfo(reflect.TypeOf(testParagraph{}))

	if len(info.methods) != 2 {
		t.Fatalf("expected 2 methods, got %d", len(info.methods))
	}

	if _, ok := info.byMethod["text"]; !ok {
		t.Error("expected method 'text' in byMethod")
	}
	if _, ok := info.byMethod["word_count"]; !ok {
		t.Error("expected method 'word_count' in byMethod")
	}
}

func TestGetTypeInfo_ExcludesUnsupportedSignatures(t *testing.T) {
	info := getTypeInfo(reflect.TypeOf(testUnsupportedSig{}))

	if len(info.methods) != 0 {
		names := make([]string, 0, len(info.methods))
		for _, m := range info.methods {
			names = append(names, m.starName)
		}
		t.Errorf("expected 0 methods, got %d: %v", len(info.methods), names)
	}
}

func TestGetTypeInfo_ExcludesStringerFromMethods(t *testing.T) {
	info := getTypeInfo(reflect.TypeOf(testStringer{}))

	if _, ok := info.byMethod["string"]; ok {
		t.Error("String() should not appear as a method attr")
	}
}

func TestGetTypeInfo_MethodWithError(t *testing.T) {
	info := getTypeInfo(reflect.TypeOf(testWithError{}))

	mi, ok := info.byMethod["compute"]
	if !ok {
		t.Fatal("expected method 'compute' in byMethod")
	}
	if !mi.hasError {
		t.Error("expected hasError=true for Compute() (string, error)")
	}
}

func TestGetTypeInfo_AttrListIncludesMethodsAndFields(t *testing.T) {
	info := getTypeInfo(reflect.TypeOf(testParagraph{}))

	want := []string{"text", "word_count", "words"}
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

func TestStructValue_MethodAttr(t *testing.T) {
	p := &testParagraph{Words: []string{"hello", "world"}}
	sv, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	ha := sv.(starlark.HasAttrs)

	textVal, err := ha.Attr("text")
	if err != nil {
		t.Fatalf("Attr(text) error: %v", err)
	}
	s, ok := textVal.(starlark.String)
	if !ok {
		t.Fatalf("expected starlark.String, got %T", textVal)
	}
	if string(s) != "hello world" {
		t.Errorf("text = %q, want %q", string(s), "hello world")
	}
}

func TestStructValue_MethodWithErrorSuccess(t *testing.T) {
	v := &testWithError{ShouldFail: false}
	sv, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	ha := sv.(starlark.HasAttrs)

	result, err := ha.Attr("compute")
	if err != nil {
		t.Fatalf("Attr(compute) error: %v", err)
	}
	s, ok := result.(starlark.String)
	if !ok {
		t.Fatalf("expected starlark.String, got %T", result)
	}
	if string(s) != "ok" {
		t.Errorf("compute = %q, want %q", string(s), "ok")
	}
}

func TestStructValue_MethodWithErrorPropagates(t *testing.T) {
	v := &testWithError{ShouldFail: true}
	sv, err := Marshal(v)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	ha := sv.(starlark.HasAttrs)

	_, err = ha.Attr("compute")
	if err == nil {
		t.Fatal("expected error from Attr(compute)")
	}
	if !strings.Contains(err.Error(), "computation failed") {
		t.Errorf("error = %q, want to contain %q", err, "computation failed")
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

func TestStructValue_AttrNames(t *testing.T) {
	p := &testParagraph{Words: []string{"a"}}
	sv, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	ha := sv.(starlark.HasAttrs)
	names := ha.AttrNames()

	want := []string{"text", "word_count", "words"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("AttrNames = %v, want %v", names, want)
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

func TestStructValue_StringWithoutStringer(t *testing.T) {
	p := &testPoint{X: 10, Y: 20}
	sv, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	got := sv.String()
	if !strings.Contains(got, "test_point(") {
		t.Errorf("String() = %q, want to contain %q", got, "test_point(")
	}
}

func TestStructValue_Type(t *testing.T) {
	p := &testParagraph{Words: []string{"a"}}
	sv, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if sv.Type() != "test_paragraph" {
		t.Errorf("Type() = %q, want %q", sv.Type(), "test_paragraph")
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
