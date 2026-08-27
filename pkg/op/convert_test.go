// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
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

	runtimeEnvironment := &RuntimeEnvironment{}

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

func TestTypesAreInterconvertible(t *testing.T) {

	var nilType reflect.Type

	cases := []struct {
		name string
		a, b reflect.Type
		want bool
	}{
		{"identity", reflect.TypeFor[string](), reflect.TypeFor[string](), true},
		{"assignable a to b", reflect.TypeFor[*Node](), reflect.TypeFor[ExecutableUnit](), true},
		{"assignable b to a", reflect.TypeFor[ExecutableUnit](), reflect.TypeFor[*Node](), true},
		{"source-only a to b", reflect.TypeFor[sourceConverter](), reflect.TypeFor[int](), true},
		{"source-only b to a (symmetric acceptance)", reflect.TypeFor[int](), reflect.TypeFor[sourceConverter](), true},
		{"target-only b from a", reflect.TypeFor[int](), reflect.TypeFor[*targetConverter](), true},
		{"target-only a from b (symmetric acceptance)", reflect.TypeFor[*targetConverter](), reflect.TypeFor[int](), true},
		{"neither direction", reflect.TypeFor[chan int](), reflect.TypeFor[string](), false},
		{"nil a", nilType, reflect.TypeFor[string](), false},
		{"nil b", reflect.TypeFor[string](), nilType, false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := typesAreInterconvertible(testCase.a, testCase.b); got != testCase.want {
				t.Errorf("typesAreInterconvertible(%v, %v) = %v, want %v",
					testCase.a, testCase.b, got, testCase.want)
			}
		})
	}
}

// region Python-shaped conversion rules (#709)

// TestConvert_CrossCategoryIsAnError pins the rule starlark authors actually experience.
//
// [Convert] used to ask Go whether the source was ConvertibleTo the target and convert if so. Go answers a
// question about REPRESENTABILITY, not about meaning, and it says yes to conversions that silently produce a
// value nobody wrote: an integer becomes the rune at its code point, a wide integer wraps into a narrow one,
// a negative reinterprets as unsigned, and a float loses its fraction.
//
// Starlark is python-shaped, so the rule is python's: a cross-category conversion is an error whatever the
// value, and within the integer category the value must be in range. str(), int(), and float() exist for
// authors who mean the conversion, and their rendering is correct by construction.
func TestConvert_CrossCategoryIsAnError(t *testing.T) {

	for _, testCase := range []struct {
		name   string
		value  any
		target reflect.Type
		why    string
	}{
		{"integer to string", int64(65), reflect.TypeFor[string](),
			`yielded "A", the rune at code point 65; an author who means "65" writes str(65)`},

		{"integer to string, rejected by kind not by range", int64(0), reflect.TypeFor[string](),
			"0 round-trips through a string and back, so only a kind check can reject it"},

		{"float to integer, fractional", 3.9, reflect.TypeFor[int](),
			"yielded 3, discarding the fraction"},

		{"float to integer, integral", 3.0, reflect.TypeFor[int](),
			"python rejects [1,2,3][1.0] though 1.0 is integral: the rule is category, not value"},

		{"integer too wide for the target", int64(300), reflect.TypeFor[int8](),
			"yielded 44, wrapping around int8"},

		{"negative integer to an unsigned target", int64(-1), reflect.TypeFor[uint8](),
			"yielded 255, reinterpreting the sign bit"},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			got, err := Convert(nil, testCase.value, testCase.target)
			if err == nil {
				t.Errorf("Convert(%#v -> %s) = %#v, want an error: %s",
					testCase.value, testCase.target, got, testCase.why)
			}
		})
	}
}

// TestConvert_WideningAndCategoryPreservingStillWork guards the other half.
//
// A rule that rejects everything is not a rule. Each of these is a conversion python performs, or one the
// tree depends on, and each must survive: file.mkdir(mode=0o644) reaches an os.FileMode, which is a uint32,
// so integer-to-integer in range cannot be rejected by kind.
func TestConvert_WideningAndCategoryPreservingStillWork(t *testing.T) {

	for _, testCase := range []struct {
		name   string
		value  any
		target reflect.Type
		want   any
	}{
		{"integer widens to float, as python does implicitly", int64(7), reflect.TypeFor[float64](), 7.0},
		{"integer to a wider integer", int64(300), reflect.TypeFor[int64](), int64(300)},
		{"file mode: integer to uint32, in range", int64(0o644), reflect.TypeFor[uint32](), uint32(0o644)},
		{"string to bytes", "hi", reflect.TypeFor[[]byte](), []byte("hi")},
		{"bytes to string", []byte("hi"), reflect.TypeFor[string](), "hi"},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			got, err := Convert(nil, testCase.value, testCase.target)
			if err != nil {
				t.Fatalf("Convert(%#v -> %s): %v", testCase.value, testCase.target, err)
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("Convert(%#v -> %s) = %#v, want %#v", testCase.value, testCase.target, got, testCase.want)
			}
		})
	}
}

// TestConvert_RefusalNamesTheRemedy pins the message, not merely the refusal.
//
// The error is the only feedback a starlark author gets, and "int64 value is neither assignable nor
// convertible to string" is Go's vocabulary for their mistake -- it says what failed and not what to write
// instead. Python answers these three with a sentence that names the fix, and starlark has str(), int(), and
// float() to offer.
//
// Asserting the guidance rather than just the failure is what keeps it: a test matching only "string" passes
// whether the message helps or not.
func TestConvert_RefusalNamesTheRemedy(t *testing.T) {

	for _, testCase := range []struct {
		name   string
		value  any
		target reflect.Type
		want   string
	}{
		{"integer to string offers str", int64(65), reflect.TypeFor[string](), "write str(x)"},
		{"float to integer offers int", 3.9, reflect.TypeFor[int](), "write int(x)"},
		{"out of range says so", int64(300), reflect.TypeFor[int8](), "out of range"},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			_, err := Convert(nil, testCase.value, testCase.target)
			if err == nil {
				t.Fatalf("Convert(%#v -> %s) succeeded; want a refusal", testCase.value, testCase.target)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("Convert(%#v -> %s) said %q, want it to contain %q",
					testCase.value, testCase.target, err, testCase.want)
			}
		})
	}
}

// endregion
