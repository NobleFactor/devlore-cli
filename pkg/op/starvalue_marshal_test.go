// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// --- camelToSnake tests ---

func Test_camelToSnake(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ProgramName", "program_name"},
		{"LOC", "loc"},
		{"SLOC", "sloc"},
		{"XMLParser", "xml_parser"},
		{"HasDocstring", "has_docstring"},
		{"FileCount", "file_count"},
		{"ID", "id"},
		{"HTTPServer", "http_server"},
		{"Simple", "simple"},
		{"A", "a"},
		{"AB", "ab"},
		{"ABc", "a_bc"},
		{"BackupSuffix", "backup_suffix"},
		{"WriteBytes", "write_bytes"},
		{"IsDir", "is_dir"},
		{"HonorGitignore", "honor_gitignore"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := camelToSnake(tt.input)
			if got != tt.want {
				t.Errorf("camelToSnake(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Marshal tests ---

func TestMarshal_Nil(t *testing.T) {
	got, err := marshal(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != starlark.None {
		t.Errorf("marshal(nil) = %v, want None", got)
	}
}

func TestMarshal_StarlarkValue(t *testing.T) {
	sv := starlark.String("pass-through")
	got, err := marshal(sv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sv {
		t.Errorf("marshal(starlark.Value) did not pass through")
	}
}

func TestMarshal_Primitives(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		wantType string
		wantStr  string
	}{
		{"string", "hello", "string", `"hello"`},
		{"bool_true", true, "bool", "True"},
		{"bool_false", false, "bool", "False"},
		{"int", 42, "int", "42"},
		{"int64", int64(999), "int", "999"},
		{"float64", 3.14, "float", "3.14"},
		{"uint32", uint32(0o644), "int", "420"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := marshal(tt.input)
			if err != nil {
				t.Fatalf("marshal(%v) error: %v", tt.input, err)
			}
			if got.Type() != tt.wantType {
				t.Errorf("type = %q, want %q", got.Type(), tt.wantType)
			}
			if got.String() != tt.wantStr {
				t.Errorf("string = %q, want %q", got.String(), tt.wantStr)
			}
		})
	}
}

func TestMarshal_FileMode(t *testing.T) {
	mode := os.FileMode(0o755)
	got, err := marshal(mode)
	if err != nil {
		t.Fatalf("marshal(FileMode) error: %v", err)
	}
	si, ok := got.(starlark.Int)
	if !ok {
		t.Fatalf("expected starlark.Int, got %T", got)
	}
	u, _ := si.Uint64()
	if u != 0o755 {
		t.Errorf("got %o, want 755", u)
	}
}

func TestMarshal_Bytes(t *testing.T) {
	got, err := marshal([]byte("raw data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, ok := got.(starlark.Bytes)
	if !ok {
		t.Fatalf("expected starlark.Bytes, got %T", got)
	}
	if string(b) != "raw data" {
		t.Errorf("got %q, want %q", string(b), "raw data")
	}
}

func TestMarshal_StringSlice(t *testing.T) {
	got, err := marshal([]string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, ok := got.(*starlark.List)
	if !ok {
		t.Fatalf("expected *starlark.List, got %T", got)
	}
	if list.Len() != 3 {
		t.Fatalf("len = %d, want 3", list.Len())
	}
	for i, want := range []string{"a", "b", "c"} {
		s, ok := starlark.AsString(list.Index(i))
		if !ok || s != want {
			t.Errorf("index %d: got %v, want %q", i, list.Index(i), want)
		}
	}
}

func TestMarshal_IntSlice(t *testing.T) {
	got, err := marshal([]int{1, 2, 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, ok := got.(*starlark.List)
	if !ok {
		t.Fatalf("expected *starlark.List, got %T", got)
	}
	if list.Len() != 3 {
		t.Errorf("len = %d, want 3", list.Len())
	}
}

func TestMarshal_NilSlice(t *testing.T) {
	var s []string
	got, err := marshal(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	list, ok := got.(*starlark.List)
	if !ok {
		t.Fatalf("expected *starlark.List, got %T", got)
	}
	if list.Len() != 0 {
		t.Errorf("len = %d, want 0", list.Len())
	}
}

func TestMarshal_Map(t *testing.T) {
	got, err := marshal(map[string]any{"name": "test", "count": 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dict, ok := got.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected *starlark.Dict, got %T", got)
	}

	val, found, err := dict.Get(starlark.String("name"))
	if err != nil || !found {
		t.Fatalf("key 'name' not found")
	}
	if s, ok := starlark.AsString(val); !ok || s != "test" {
		t.Errorf("name = %v, want test", val)
	}
}

func TestMarshal_NilMap(t *testing.T) {
	var m map[string]any
	got, err := marshal(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dict, ok := got.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected *starlark.Dict, got %T", got)
	}
	if dict.Len() != 0 {
		t.Errorf("len = %d, want 0", dict.Len())
	}
}

func TestMarshal_Pointer(t *testing.T) {
	s := "hello"
	got, err := marshal(&s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.String() != `"hello"` {
		t.Errorf("got %v, want %q", got, "hello")
	}
}

func TestMarshal_NilPointer(t *testing.T) {
	var s *string
	got, err := marshal(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != starlark.None {
		t.Errorf("got %v, want None", got)
	}
}

// Test struct for marshaling.
type testPoint struct {
	X int
	Y int
}

type testPerson struct {
	Name     string
	Age      int
	Location *testPoint
	Tags     []string
}

type testWithTag struct {
	FullName string `starlark:"name"`
	Hidden   string `starlark:"-"`
	Normal   int
}

func TestMarshal_SimpleStruct(t *testing.T) {
	p := testPoint{X: 10, Y: 20}
	got, err := marshal(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := got.(*starlarkstruct.Struct)
	if !ok {
		t.Fatalf("expected *starlarkstruct.Struct, got %T", got)
	}

	xv, err := s.Attr("x")
	if err != nil {
		t.Fatalf("Attr(x) error: %v", err)
	}
	xi, ok := xv.(starlark.Int)
	if !ok {
		t.Fatalf("x: expected Int, got %T", xv)
	}
	x, _ := xi.Int64()
	if x != 10 {
		t.Errorf("x = %d, want 10", x)
	}

	yv, err := s.Attr("y")
	if err != nil {
		t.Fatalf("Attr(y) error: %v", err)
	}
	yi, ok := yv.(starlark.Int)
	if !ok {
		t.Fatalf("y: expected Int, got %T", yv)
	}
	y, _ := yi.Int64()
	if y != 20 {
		t.Errorf("y = %d, want 20", y)
	}
}

func TestMarshal_NestedStruct(t *testing.T) {
	p := testPerson{
		Name:     "Alice",
		Age:      30,
		Location: &testPoint{X: 1, Y: 2},
		Tags:     []string{"dev", "go"},
	}
	got, err := marshal(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := got.(*starlarkstruct.Struct)
	if !ok {
		t.Fatalf("expected *starlarkstruct.Struct, got %T", got)
	}

	// Check name.
	nv, _ := s.Attr("name")
	name, _ := starlark.AsString(nv)
	if name != "Alice" {
		t.Errorf("name = %q, want Alice", name)
	}

	// Check nested struct.
	lv, _ := s.Attr("location")
	ls, ok := lv.(*starlarkstruct.Struct)
	if !ok {
		t.Fatalf("location: expected struct, got %T", lv)
	}
	xv, _ := ls.Attr("x")
	xi, _ := xv.(starlark.Int)
	x, _ := xi.Int64()
	if x != 1 {
		t.Errorf("location.x = %d, want 1", x)
	}

	// Check tags slice.
	tv, _ := s.Attr("tags")
	tl, ok := tv.(*starlark.List)
	if !ok {
		t.Fatalf("tags: expected list, got %T", tv)
	}
	if tl.Len() != 2 {
		t.Errorf("tags len = %d, want 2", tl.Len())
	}
}

func TestMarshal_NilNestedStruct(t *testing.T) {
	p := testPerson{
		Name:     "Bob",
		Age:      25,
		Location: nil,
	}
	got, err := marshal(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := got.(*starlarkstruct.Struct)
	lv, _ := s.Attr("location")
	if lv != starlark.None {
		t.Errorf("nil location = %v, want None", lv)
	}
}

func TestMarshal_StructTags(t *testing.T) {
	v := testWithTag{
		FullName: "Alice",
		Hidden:   "secret",
		Normal:   42,
	}
	got, err := marshal(v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := got.(*starlarkstruct.Struct)

	// Custom tag: "name" instead of "full_name".
	nv, err := s.Attr("name")
	if err != nil {
		t.Fatalf("Attr(name) error: %v", err)
	}
	name, _ := starlark.AsString(nv)
	if name != "Alice" {
		t.Errorf("name = %q, want Alice", name)
	}

	// Hidden field should not be present.
	_, err = s.Attr("hidden")
	if err == nil {
		t.Error("hidden field should not be accessible")
	}

	// Normal field should use snake_case.
	normalv, err := s.Attr("normal")
	if err != nil {
		t.Fatalf("Attr(normal) error: %v", err)
	}
	ni, _ := normalv.(starlark.Int)
	n, _ := ni.Int64()
	if n != 42 {
		t.Errorf("normal = %d, want 42", n)
	}
}

func TestMarshal_UnsupportedType(t *testing.T) {
	ch := make(chan int)
	_, err := marshal(ch)
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

// --- Unmarshal tests ---

func TestUnmarshal_ToString(t *testing.T) {
	var s string
	if err := unmarshal(starlark.String("hello"), &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "hello" {
		t.Errorf("got %q, want %q", s, "hello")
	}
}

func TestUnmarshal_ToInt(t *testing.T) {
	var i int
	if err := unmarshal(starlark.MakeInt(42), &i); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i != 42 {
		t.Errorf("got %d, want 42", i)
	}
}

func TestUnmarshal_ToBool(t *testing.T) {
	var b bool
	if err := unmarshal(starlark.True, &b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !b {
		t.Error("got false, want true")
	}
}

func TestUnmarshal_ToFloat(t *testing.T) {
	var f float64
	if err := unmarshal(starlark.Float(3.14), &f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != 3.14 {
		t.Errorf("got %f, want 3.14", f)
	}
}

func TestUnmarshal_IntToFloat(t *testing.T) {
	var f float64
	if err := unmarshal(starlark.MakeInt(5), &f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != 5.0 {
		t.Errorf("got %f, want 5.0", f)
	}
}

func TestUnmarshal_ToUint32(t *testing.T) {
	var u uint32
	if err := unmarshal(starlark.MakeInt(0o644), &u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u != 0o644 {
		t.Errorf("got %o, want 644", u)
	}
}

func TestUnmarshal_ToFileMode(t *testing.T) {
	var mode os.FileMode
	if err := unmarshal(starlark.MakeInt(0o755), &mode); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != 0o755 {
		t.Errorf("got %o, want 755", mode)
	}
}

func TestUnmarshal_ToBytes(t *testing.T) {
	var b []byte
	if err := unmarshal(starlark.Bytes("data"), &b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(b) != "data" {
		t.Errorf("got %q, want %q", string(b), "data")
	}
}

func TestUnmarshal_ToStringSlice(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.String("a"),
		starlark.String("b"),
	})
	var s []string
	if err := unmarshal(list, &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) != 2 || s[0] != "a" || s[1] != "b" {
		t.Errorf("got %v, want [a b]", s)
	}
}

func TestUnmarshal_ToIntSlice(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.MakeInt(1),
		starlark.MakeInt(2),
		starlark.MakeInt(3),
	})
	var s []int
	if err := unmarshal(list, &s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(s, []int{1, 2, 3}) {
		t.Errorf("got %v, want [1 2 3]", s)
	}
}

func TestUnmarshal_ToMap(t *testing.T) {
	dict := starlark.NewDict(2)
	_ = dict.SetKey(starlark.String("name"), starlark.String("test"))
	_ = dict.SetKey(starlark.String("count"), starlark.MakeInt(5))

	var m map[string]any
	if err := unmarshal(dict, &m); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["name"] != "test" {
		t.Errorf("name = %v, want test", m["name"])
	}
	if m["count"] != 5 {
		t.Errorf("count = %v, want 5", m["count"])
	}
}

func TestUnmarshal_ToAny(t *testing.T) {
	tests := []struct {
		name  string
		input starlark.Value
		want  any
	}{
		{"string", starlark.String("hello"), "hello"},
		{"int", starlark.MakeInt(42), 42},
		{"bool", starlark.True, true},
		{"float", starlark.Float(3.14), 3.14},
		{"none", starlark.None, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got any
			if err := unmarshal(tt.input, &got); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestUnmarshal_ToAnyList(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.String("a"),
		starlark.String("b"),
	})
	var got any
	if err := unmarshal(list, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ss, ok := got.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", got)
	}
	if len(ss) != 2 || ss[0] != "a" || ss[1] != "b" {
		t.Errorf("got %v, want [a b]", ss)
	}
}

func TestUnmarshal_ToAnyMixedList(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.String("a"),
		starlark.MakeInt(1),
	})
	var got any
	if err := unmarshal(list, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sl, ok := got.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", got)
	}
	if len(sl) != 2 {
		t.Fatalf("len = %d, want 2", len(sl))
	}
}

func TestUnmarshal_ToStruct(t *testing.T) {
	sv := starlarkstruct.FromStringDict(starlark.String("test_point"), starlark.StringDict{
		"x": starlark.MakeInt(10),
		"y": starlark.MakeInt(20),
	})
	var p testPoint
	if err := unmarshal(sv, &p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.X != 10 || p.Y != 20 {
		t.Errorf("got {%d, %d}, want {10, 20}", p.X, p.Y)
	}
}

func TestUnmarshal_StructFromDict(t *testing.T) {
	dict := starlark.NewDict(2)
	_ = dict.SetKey(starlark.String("x"), starlark.MakeInt(5))
	_ = dict.SetKey(starlark.String("y"), starlark.MakeInt(7))

	var p testPoint
	if err := unmarshal(dict, &p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.X != 5 || p.Y != 7 {
		t.Errorf("got {%d, %d}, want {5, 7}", p.X, p.Y)
	}
}

func TestUnmarshal_StructMissingFields(t *testing.T) {
	sv := starlarkstruct.FromStringDict(starlark.String("point"), starlark.StringDict{
		"x": starlark.MakeInt(10),
	})
	var p testPoint
	if err := unmarshal(sv, &p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.X != 10 || p.Y != 0 {
		t.Errorf("got {%d, %d}, want {10, 0}", p.X, p.Y)
	}
}

func TestUnmarshal_StructWithTag(t *testing.T) {
	sv := starlarkstruct.FromStringDict(starlark.String("tagged"), starlark.StringDict{
		"name":   starlark.String("Alice"),
		"normal": starlark.MakeInt(99),
	})
	var v testWithTag
	if err := unmarshal(sv, &v); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.FullName != "Alice" {
		t.Errorf("FullName = %q, want Alice", v.FullName)
	}
	if v.Normal != 99 {
		t.Errorf("Normal = %d, want 99", v.Normal)
	}
	if v.Hidden != "" {
		t.Errorf("Hidden = %q, want empty", v.Hidden)
	}
}

func TestUnmarshal_NestedStruct(t *testing.T) {
	inner := starlarkstruct.FromStringDict(starlark.String("test_point"), starlark.StringDict{
		"x": starlark.MakeInt(1),
		"y": starlark.MakeInt(2),
	})
	outer := starlarkstruct.FromStringDict(starlark.String("test_person"), starlark.StringDict{
		"name":     starlark.String("Alice"),
		"age":      starlark.MakeInt(30),
		"location": inner,
		"tags":     starlark.NewList([]starlark.Value{starlark.String("dev")}),
	})
	var p testPerson
	if err := unmarshal(outer, &p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "Alice" {
		t.Errorf("Name = %q, want Alice", p.Name)
	}
	if p.Age != 30 {
		t.Errorf("Age = %d, want 30", p.Age)
	}
	if p.Location == nil || p.Location.X != 1 || p.Location.Y != 2 {
		t.Errorf("Location = %+v, want {1, 2}", p.Location)
	}
	if len(p.Tags) != 1 || p.Tags[0] != "dev" {
		t.Errorf("Tags = %v, want [dev]", p.Tags)
	}
}

func TestUnmarshal_NoneToPointer(t *testing.T) {
	var p *testPoint
	if err := unmarshal(starlark.None, &p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Errorf("got %v, want nil", p)
	}
}

func TestUnmarshal_NonPointerTarget(t *testing.T) {
	var s string
	err := unmarshal(starlark.String("hello"), s)
	if err == nil {
		t.Fatal("expected error for non-pointer target")
	}
}

// --- Round-trip tests ---

func TestRoundTrip_SimpleStruct(t *testing.T) {
	original := testPoint{X: 42, Y: 99}
	sv, err := marshal(original)
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

func TestRoundTrip_NestedStruct(t *testing.T) {
	original := testPerson{
		Name:     "Bob",
		Age:      25,
		Location: &testPoint{X: 3, Y: 4},
		Tags:     []string{"go", "dev"},
	}
	sv, err := marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var result testPerson
	if err := unmarshal(sv, &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if result.Name != original.Name || result.Age != original.Age {
		t.Errorf("name/age mismatch: got %+v", result)
	}
	if result.Location == nil || *result.Location != *original.Location {
		t.Errorf("location mismatch: got %+v", result.Location)
	}
	if !reflect.DeepEqual(result.Tags, original.Tags) {
		t.Errorf("tags: got %v, want %v", result.Tags, original.Tags)
	}
}

func TestRoundTrip_Map(t *testing.T) {
	original := map[string]any{
		"name":  "test",
		"count": 5,
		"flag":  true,
	}
	sv, err := marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var result map[string]any
	if err := unmarshal(sv, &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if result["name"] != "test" {
		t.Errorf("name = %v, want test", result["name"])
	}
	if result["count"] != 5 {
		t.Errorf("count = %v, want 5", result["count"])
	}
	if result["flag"] != true {
		t.Errorf("flag = %v, want true", result["flag"])
	}
}

// --- Type cache tests ---

func TestTypeCache_ComputedOnce(t *testing.T) {
	// Clear cache for this test.
	typeCache = sync.Map{}

	info1 := getTypeInfo(reflect.TypeOf(testPoint{}))
	info2 := getTypeInfo(reflect.TypeOf(testPoint{}))
	if info1 != info2 {
		t.Error("getTypeInfo should return same pointer for same type")
	}
}

func TestTypeCache_PointerDeref(t *testing.T) {
	typeCache = sync.Map{}

	info1 := getTypeInfo(reflect.TypeOf(testPoint{}))
	info2 := getTypeInfo(reflect.TypeOf(&testPoint{}))
	if info1 != info2 {
		t.Error("getTypeInfo should deref pointer type")
	}
}

func TestGetTypeInfo_AttrList(t *testing.T) {
	info := getTypeInfo(reflect.TypeOf(testPerson{}))

	wantAttrs := []string{"age", "location", "name", "tags"}
	if !reflect.DeepEqual(info.attrList, wantAttrs) {
		t.Errorf("attrList = %v, want %v", info.attrList, wantAttrs)
	}
}

func TestGetTypeInfo_HiddenField(t *testing.T) {
	info := getTypeInfo(reflect.TypeOf(testWithTag{}))

	if _, ok := info.byName["hidden"]; ok {
		t.Error("hidden field should not be in byName")
	}
	if _, ok := info.byName["name"]; !ok {
		t.Error("'name' (tagged) should be in byName")
	}
}

// --- Constructor registry tests ---

// testConstructable is a test type for constructor registry.
type testConstructable struct {
	Value string
	Extra int
}

func TestUnmarshal_WithConstructor(t *testing.T) {
	RegisterConstructor(func(v any) (testConstructable, error) {
		s, ok := v.(string)
		if !ok {
			return testConstructable{}, fmt.Errorf("expected string, got %T", v)
		}
		return testConstructable{Value: s, Extra: len(s)}, nil
	})
	defer constructorRegistry.Delete(reflect.TypeOf(testConstructable{}))

	var got testConstructable
	if err := unmarshal(starlark.String("world"), &got); err != nil {
		t.Fatalf("unmarshal() error = %v", err)
	}
	if got.Value != "world" || got.Extra != 5 {
		t.Errorf("unmarshal() = %+v, want {world, 5}", got)
	}
}

func TestUnmarshal_Constructor_InvalidInput(t *testing.T) {
	RegisterConstructor(func(v any) (testConstructable, error) {
		s, ok := v.(string)
		if !ok {
			return testConstructable{}, fmt.Errorf("expected string, got %T", v)
		}
		return testConstructable{Value: s}, nil
	})
	defer constructorRegistry.Delete(reflect.TypeOf(testConstructable{}))

	var got testConstructable
	err := unmarshal(starlark.MakeInt(42), &got)
	if err == nil {
		t.Fatal("unmarshal() expected error for wrong starlark type")
	}
	if !strings.Contains(err.Error(), "expected string") {
		t.Errorf("error = %q, want to contain %q", err, "expected string")
	}
}

// --- Starlark struct to any ---

func TestUnmarshal_StarlarkStructToAny(t *testing.T) {
	sv := starlarkstruct.FromStringDict(starlark.String("point"), starlark.StringDict{
		"x": starlark.MakeInt(10),
		"y": starlark.MakeInt(20),
	})
	var got any
	if err := unmarshal(sv, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", got)
	}
	if m["x"] != 10 || m["y"] != 20 {
		t.Errorf("got %v, want map[x:10 y:20]", m)
	}
}
