// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// region Tests

// TestTypeWrapper_RoundTripsEveryDocumentType is rows 1, 2, 3, 15, and 19 of the #712 test plan.
//
// Each value is wrapped, encoded, decoded by a real codec, and unwrapped -- so the assertion covers what a
// document actually carries rather than what the wrapper returns in memory.
func TestTypeWrapper_RoundTripsEveryDocumentType(t *testing.T) {

	for _, testCase := range []struct {
		name  string
		value any
	}{
		{"nil", nil},
		{"bool", true},
		{"string", "hi"},
		{"string that looks like a number", "42"},
		{"bytes", []byte("hi")},
		{"integer", int64(42)},
		{"integer beyond a float64", int64(9007199254740993)},
		{"negative integer", int64(-7)},
		{"float", 1.5},
		{"integral float", float64(42)},
		{"float needing every digit", 0.1},
		{"positive infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
		{"list", []any{int64(1), "two", 3.5}},
		{"map", map[string]any{"a": int64(1), "b": "two"}},
		{"nested", map[string]any{"outer": []any{map[string]any{"inner": float64(42)}}}},
		{"empty list", []any{}},
		{"empty map", map[string]any{}},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			for _, codec := range documentCodecs() {
				t.Run(codec.name, func(t *testing.T) {

					wrapped, err := encodeTypeWrapper(testCase.value)
					if err != nil {
						t.Fatalf("encodeTypeWrapper(%#v): %v", testCase.value, err)
					}

					document, err := codec.encode(wrapped)
					if err != nil {
						t.Fatalf("encode: %v", err)
					}

					var decoded any
					if err := codec.decode(document, &decoded); err != nil {
						t.Fatalf("decode(%s): %v", document, err)
					}

					got, err := decodeTypeWrapper(decoded, nil)
					if err != nil {
						t.Fatalf("decodeTypeWrapper(%s): %v", document, err)
					}

					if !reflect.DeepEqual(got, testCase.value) {
						t.Errorf("round trip of %#v gave %#v (%s)", testCase.value, got, document)
					}
				})
			}
		})
	}
}

// TestTypeWrapper_NaNRoundTripsAsNaN covers the one value reflect.DeepEqual cannot assert.
//
// NaN is not equal to itself, so it needs its own test rather than a row in the table above.
func TestTypeWrapper_NaNRoundTripsAsNaN(t *testing.T) {

	wrapped, err := encodeTypeWrapper(math.NaN())
	if err != nil {
		t.Fatalf("encodeTypeWrapper(NaN): %v", err)
	}

	document, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded any
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	got, err := decodeTypeWrapper(decoded, nil)
	if err != nil {
		t.Fatalf("decodeTypeWrapper(%s): %v", document, err)
	}

	number, isFloat := got.(float64)
	if !isFloat || !math.IsNaN(number) {
		t.Errorf("NaN round-tripped to %#v, want a NaN float64", got)
	}
}

// TestTypeWrapper_ABareValueIsRefused is row 6 of the #712 test plan.
//
// We serialize with full knowledge of the field type, so we deserialize with full knowledge of it. A reader
// reduced to inspecting a literal's shape is reading a document the writer got wrong, and saying so is the
// only thing that keeps a missing wrapper detectable.
func TestTypeWrapper_ABareValueIsRefused(t *testing.T) {

	for _, bare := range []any{
		float64(42),
		"42",
		true,
		nil,
		map[string]any{"one": 1, "two": 2},
		[]any{1, 2},
	} {
		if got, err := decodeTypeWrapper(bare, nil); err == nil {
			t.Errorf("decodeTypeWrapper(%#v) = %#v, want an error: a bare value must not be guessed at", bare, got)
		}
	}
}

// TestTypeWrapper_AnUnknownTypeNameIsRefused covers the other half of refusing to guess.
//
// A wrapper naming a type this reader does not know is a document from a future it cannot read. Falling back
// to the payload's apparent type would be the same guess by another route.
func TestTypeWrapper_AnUnknownTypeNameIsRefused(t *testing.T) {

	if got, err := decodeTypeWrapper(map[string]any{"$decimal128": "1.0"}, nil); err == nil {
		t.Errorf("decodeTypeWrapper($decimal128) = %#v, want an error", got)
	}
}

// TestTypeWrapper_AMapKeyedLikeAWrapperSurvives is row 7 of the #712 test plan.
//
// An author's own map is always the PAYLOAD of a wrapper, never a wrapper itself, so a key that looks like a
// type name cannot be mistaken for one. This is what the uniform rule buys: under a partial rule this map
// would need an escape hatch.
func TestTypeWrapper_AMapKeyedLikeAWrapperSurvives(t *testing.T) {

	want := map[string]any{typeNameInt64: "not a number at all"}

	wrapped, err := encodeTypeWrapper(want)
	if err != nil {
		t.Fatalf("encodeTypeWrapper: %v", err)
	}

	document, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded any
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	got, err := decodeTypeWrapper(decoded, nil)
	if err != nil {
		t.Fatalf("decodeTypeWrapper(%s): %v", document, err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("a map keyed %q round-tripped to %#v, want %#v", typeNameInt64, got, want)
	}
}

// TestTypeWrapper_ANonFiniteFloatSurvivesJSON is row 15 of the #712 test plan, at the codec.
//
// encoding/json refuses a non-finite float as a bare number (encode.go:572), which is why the payload is a
// string. This proves the wrapper is what makes the value expressible, not merely tidier.
func TestTypeWrapper_ANonFiniteFloatSurvivesJSON(t *testing.T) {

	wrapped, err := encodeTypeWrapper(math.Inf(1))
	if err != nil {
		t.Fatalf("encodeTypeWrapper(+Inf): %v", err)
	}

	if _, err := json.Marshal(wrapped); err != nil {
		t.Errorf("Marshal(+Inf wrapper): %v; a string payload carries what a bare number cannot", err)
	}
}

// endregion

// region Helpers

// documentCodecs returns the two document codecs a wrapper has to agree across.
//
// Returns:
//   - the codec name, its encoder, and its decoder.
func documentCodecs() []struct {
	name   string
	encode func(any) ([]byte, error)
	decode func([]byte, any) error
} {
	return []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"json", json.Marshal, func(data []byte, target any) error {
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.UseNumber()
			return decoder.Decode(target)
		}},
		{"yaml", yaml.Marshal, yaml.Unmarshal},
	}
}

// endregion
