// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package extension

import (
	"encoding/json"
	"testing"

	"go.starlark.net/starlark"
)

// marshalResult converts a script's return value as a command's result is converted, and encodes it as JSON.
//
// Parameters:
//   - `t`: the test.
//   - `value`: the value a script returned.
//
// Returns:
//   - `string`: the JSON the output pipeline renders.
func marshalResult(t *testing.T, value starlark.Value) string {

	t.Helper()
	converted, err := goValue(value)
	if err != nil {
		t.Fatalf("goValue(%s): %v", value, err)
	}
	encoded, err := json.Marshal(converted)
	if err != nil {
		t.Fatalf("json.Marshal(%v): %v", converted, err)
	}
	return string(encoded)
}

// --- goValue ---

// A list is always a list, [] when it is empty, at any depth; a value that holds none is null (#1215, ruled
// 2026-11-08).
func TestGoValue_AnEmptyListIsAnEmptyList(t *testing.T) {

	record := starlark.NewDict(1)
	if err := record.SetKey(starlark.String("home"), starlark.NewList(nil)); err != nil {
		t.Fatalf("SetKey: %v", err)
	}

	tests := []struct {
		name  string
		value starlark.Value
		want  string
	}{
		{name: "an empty list", value: starlark.NewList(nil), want: `[]`},
		{name: "an empty tuple", value: starlark.Tuple{}, want: `[]`},
		{name: "an empty list in a dict", value: record, want: `{"home":[]}`},
		{name: "an empty list in a list", value: starlark.NewList([]starlark.Value{starlark.NewList(nil)}), want: `[[]]`},
		{name: "a list with an element", value: starlark.NewList([]starlark.Value{starlark.String("a")}), want: `["a"]`},
		{name: "None", value: starlark.None, want: `null`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := marshalResult(t, tt.value); got != tt.want {
				t.Errorf("goValue(%s) encodes as %s, want %s", tt.value, got, tt.want)
			}
		})
	}
}
