// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// region TEST FIXTURES

// convertResource is a registered Resource used to test construction from string.
type convertResource struct {
	ResourceBase
	Path string
}

func (r *convertResource) URI() string { return "test:" + r.Path }

// newConvertResource matches the ResourceConstructor signature.
func newConvertResource(runtimeEnvironment *RuntimeEnvironment, identity any) (Resource, error) {

	s, ok := identity.(string)
	if !ok {
		return nil, fmt.Errorf("expected string, got %T", identity)
	}

	base, err := NewResourceBase(runtimeEnvironment, "test:"+s, reflect.TypeFor[*convertResource]())
	if err != nil {
		return nil, err
	}

	return &convertResource{
		ResourceBase: base,
		Path:         s,
	}, nil
}

// sourceConverter implements SourceConverter.
type sourceConverter struct{}

func (s sourceConverter) CanConvertTo(target reflect.Type) bool {
	return target == reflect.TypeFor[int]()
}

func (s sourceConverter) ConvertTo(target reflect.Type) (any, error) {
	if target == reflect.TypeFor[int]() {
		return 42, nil
	}
	return nil, fmt.Errorf("cannot convert to %s", target)
}

// targetConverter implements TargetConverter.
type targetConverter struct {
	Value string
}

func (t *targetConverter) CanConvertFrom(source reflect.Type) bool {
	return source == reflect.TypeFor[int]()
}

func (t *targetConverter) ConvertFrom(value any) (any, error) {
	if i, ok := value.(int); ok {
		return &targetConverter{Value: fmt.Sprintf("int:%d", i)}, nil
	}
	return nil, errors.New("not an int")
}

// init registers convertResource for construction tests.
func init() {
	AnnounceResource(reflect.TypeFor[*convertResource](), newConvertResource, nil)
}

// endregion

func TestConvert_Identity(t *testing.T) {

	val := 42
	got, err := Convert(nil, val, reflect.TypeFor[int]())

	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if got.(int) != val {
		t.Errorf("got %v, want %v", got, val)
	}
}

func TestConvert_Assignability(t *testing.T) {

	val := "hello"
	got, err := Convert(nil, val, reflect.TypeFor[any]())

	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if got.(string) != val {
		t.Errorf("got %v, want %v", got, val)
	}
}

func TestConvert_Slice(t *testing.T) {

	val := []int{1, 2, 3}
	target := reflect.TypeFor[[]any]()
	got, err := Convert(nil, val, target)

	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	res := got.([]any)
	if len(res) != 3 {
		t.Fatalf("len = %d, want 3", len(res))
	}
	if res[0].(int) != 1 {
		t.Errorf("res[0] = %v, want 1", res[0])
	}
}

func TestConvert_Map(t *testing.T) {

	val := map[string]int{"a": 1}
	target := reflect.TypeFor[map[any]any]()
	got, err := Convert(nil, val, target)

	if err != nil {
		t.Fatalf("Convert: %v", err)
	}

	res := got.(map[any]any)
	if res["a"].(int) != 1 {
		t.Errorf("res[a] = %v, want 1", res["a"])
	}
}

func TestConvert_SourceConverter(t *testing.T) {

	val := sourceConverter{}
	got, err := Convert(nil, val, reflect.TypeFor[int]())

	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if got.(int) != 42 {
		t.Errorf("got %v, want 42", got)
	}
}

func TestConvert_TargetConverter(t *testing.T) {

	val := 123
	target := reflect.TypeFor[*targetConverter]()
	got, err := Convert(nil, val, target)

	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	res := got.(*targetConverter)
	if res.Value != "int:123" {
		t.Errorf("Value = %q, want \"int:123\"", res.Value)
	}
}

func TestConvert_ResourceConstructor(t *testing.T) {

	reg := NewReceiverRegistry()
	runtimeEnvironment := &RuntimeEnvironment{ReceiverRegistry: reg}

	val := "/etc/passwd"
	target := reflect.TypeFor[*convertResource]()
	got, err := Convert(runtimeEnvironment, val, target)

	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	res := got.(*convertResource)
	if res.Path != val {
		t.Errorf("Path = %q, want %q", res.Path, val)
	}
}

func TestConvert_ResourceConstructor_ErrOnNilContext(t *testing.T) {

	val := "/etc/passwd"
	target := reflect.TypeFor[*convertResource]()
	_, err := Convert(nil, val, target)

	if err == nil {
		t.Fatal("expected error when converting to Resource with nil context")
	}
}
