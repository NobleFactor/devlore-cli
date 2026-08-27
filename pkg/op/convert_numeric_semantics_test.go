// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"math"
	"reflect"
	"testing"
)

// region Numeric semantics (#709)

// TestNumericSemantics_MatchPython enumerates every numeric conversion class and the rule that decides it.
//
// Starlark is python-shaped, so conversions answer to python's rule rather than Go's: a cross-category
// conversion is an error whatever the value, and within a category the value must survive. This table is the
// whole of that contract in one place — the accepting cases and the refusing ones together, because a rule
// stated only by its refusals cannot be checked for overreach.
//
// Each row names the python behavior it mirrors. Where devlore deliberately DIFFERS from python, the row
// lives in [TestNumericSemantics_DeliberateDeviationsFromPython] instead, so a deviation can never hide here
// as an untested assumption.
func TestNumericSemantics_MatchPython(t *testing.T) {

	for _, testCase := range []struct {
		name    string
		value   any
		target  reflect.Type
		refused bool
		python  string
	}{
		// --- cross-category: refused whatever the value ---
		{
			name: "integer to string", value: int64(65), target: reflect.TypeFor[string](),
			refused: true,
			python:  `"x" + 65 raises TypeError; str(65) is how an author says it`,
		},
		{
			name: "integer zero to string", value: int64(0), target: reflect.TypeFor[string](),
			refused: true,
			python:  "same rule; 0 is here because it round-trips through a string and back, so only a kind check refuses it",
		},
		{
			name: "rune to string", value: 'A', target: reflect.TypeFor[string](),
			refused: true,
			python:  "a rune is an int32; chr(65) is explicit in python for the same reason",
		},
		{
			name: "float to integer, fractional", value: 3.9, target: reflect.TypeFor[int](),
			refused: true,
			python:  "'float' object cannot be interpreted as an integer",
		},
		{
			name: "float to integer, integral", value: 3.0, target: reflect.TypeFor[int](),
			refused: true,
			python:  "[1,2,3][1.0] raises TypeError though 1.0 is integral: the rule is category, not value",
		},
		{
			name: "string to integer", value: "42", target: reflect.TypeFor[int](),
			refused: true,
			python:  "int(\"42\") is required; python never coerces a string to a number",
		},
		{
			name: "float to string", value: 1.5, target: reflect.TypeFor[string](),
			refused: true,
			python:  "str(1.5) is required; Go refuses this one too, so no guard is involved",
		},

		// --- within the integer category: the value decides ---
		{
			name: "file mode, in range", value: int64(0o644), target: reflect.TypeFor[uint32](),
			refused: false,
			python:  "struct.pack('I', 0o644) succeeds; this is what file.mkdir(mode=...) depends on",
		},
		{
			name: "widening", value: int64(300), target: reflect.TypeFor[int64](),
			refused: false,
			python:  "an int is an int; python has one integer type and no width to overflow",
		},
		{
			name: "too wide for the target", value: int64(300), target: reflect.TypeFor[int8](),
			refused: true,
			python:  "struct.error: byte format requires -128 <= number <= 127",
		},
		{
			name: "negative to unsigned", value: int64(-1), target: reflect.TypeFor[uint8](),
			refused: true,
			python:  "struct.error: argument out of range",
		},
		{
			name: "unsigned above the signed range, same width", value: uint64(math.MaxUint64), target: reflect.TypeFor[int64](),
			refused: true,
			python: "struct.error: argument out of range. Same width means signed and unsigned share a bit " +
				"pattern, so a round trip returns the original and hides the sign flip; sign is asked about directly",
		},

		// --- widening into the float category: python does this implicitly ---
		{
			name: "integer to float", value: int64(7), target: reflect.TypeFor[float64](),
			refused: false,
			python:  "1 + 2.0 == 3.0; python widens an int to a float without being asked",
		},
		{
			name: "integer beyond float64's exact range", value: int64(9007199254740993), target: reflect.TypeFor[float64](),
			refused: false,
			python: "float(9007199254740993) silently returns 9007199254740992.0. Python loses this precision " +
				"too, so refusing it would be stricter than python rather than truer to it",
		},
		{
			name: "float widening", value: float32(1.5), target: reflect.TypeFor[float64](),
			refused: false,
			python:  "a widening loses nothing",
		},

		// --- within the float category ---
		{
			name: "float narrowing that loses precision", value: 0.1, target: reflect.TypeFor[float32](),
			refused: false,
			python: "struct.pack('f', 0.1) succeeds and quietly stores 0.100000001490116. Precision loss is " +
				"not an error in python, and is not one here",
		},
		{
			name: "float narrowing that overflows", value: 1e40, target: reflect.TypeFor[float32](),
			refused: true,
			python: "struct.error: argument out of range. The distinction from the row above is exactly " +
				"python's: precision may be lost, magnitude may not",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			got, err := Convert(nil, testCase.value, testCase.target)

			switch {
			case testCase.refused && err == nil:
				t.Errorf("Convert(%#v -> %s) = %#v, want a refusal.\npython: %s",
					testCase.value, testCase.target, got, testCase.python)

			case !testCase.refused && err != nil:
				t.Errorf("Convert(%#v -> %s) was refused (%v), want it to convert.\npython: %s",
					testCase.value, testCase.target, err, testCase.python)
			}
		})
	}
}

// TestNumericSemantics_DeliberateDeviationsFromPython pins the places devlore does NOT follow python.
//
// A deviation recorded only in prose is an assumption. These rows assert the behavior we actually chose, so
// changing one is a decision someone has to make on purpose rather than a test quietly going green.
func TestNumericSemantics_DeliberateDeviationsFromPython(t *testing.T) {

	for _, testCase := range []struct {
		name    string
		value   any
		target  reflect.Type
		refused bool
		python  string
		why     string
	}{
		{
			name: "bool where an integer is wanted", value: true, target: reflect.TypeFor[int](),
			refused: true,
			python:  "[1,2,3][True] == 2; python's bool IS an int subclass",
			why: "STRICTER than python, deliberately. Go permits no bool-to-int conversion, so this is " +
				"refused by the language rather than by a guard -- and a bool arriving where a number is " +
				"expected is far likelier to be a mistake than an intent",
		},
		{
			name: "integer where a bool is wanted", value: 1, target: reflect.TypeFor[bool](),
			refused: true,
			python:  "if 1: is truthy; python accepts any object where a bool is wanted",
			why:     "STRICTER than python. Same reasoning: an accidental 1 should not silently mean true",
		},
		{
			name: "string to bytes", value: "hi", target: reflect.TypeFor[[]byte](),
			refused: false,
			python:  `"hi".encode() is required; python will not implicitly encode a str to bytes`,
			why: "LOOSER than python. Starlark has a distinct bytes type, so making this explicit is a real " +
				"question -- and one that touches every provider method taking bytes, so it is deferred " +
				"rather than decided here",
		},
		{
			name: "bytes to string", value: []byte("hi"), target: reflect.TypeFor[string](),
			refused: false,
			python:  `b"hi".decode() is required`,
			why:     "LOOSER than python, for the same reason as the row above",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			got, err := Convert(nil, testCase.value, testCase.target)

			switch {
			case testCase.refused && err == nil:
				t.Errorf("Convert(%#v -> %s) = %#v, want a refusal.\npython: %s\nours: %s",
					testCase.value, testCase.target, got, testCase.python, testCase.why)

			case !testCase.refused && err != nil:
				t.Errorf("Convert(%#v -> %s) was refused (%v), want it to convert.\npython: %s\nours: %s",
					testCase.value, testCase.target, err, testCase.python, testCase.why)
			}
		})
	}
}

// endregion
