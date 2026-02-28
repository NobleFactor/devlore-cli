// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"os"
)

// AsStateMap performs a nil-safe type assertion of compensation state
// to map[string]any. Returns nil if state is nil or not a map.
func AsStateMap(state any) map[string]any {
	s, ok := state.(map[string]any)
	if !ok {
		return nil
	}
	return s
}

// StateString extracts a string value from a compensation state map.
// Returns "" if the key is missing or not a string.
func StateString(m map[string]any, key string) string {
	v, ok := m[key].(string)
	if !ok {
		return ""
	}
	return v
}

// StateBool extracts a bool value from a compensation state map.
// Returns false if the key is missing or not a bool.
func StateBool(m map[string]any, key string) bool {
	v, ok := m[key].(bool)
	if !ok {
		return false
	}
	return v
}

// StateBytes extracts a []byte value from a compensation state map.
// Returns nil if the key is missing or not a []byte.
func StateBytes(m map[string]any, key string) []byte {
	v, ok := m[key].([]byte)
	if !ok {
		return nil
	}
	return v
}

// StateFileMode extracts an os.FileMode value from a compensation state map.
// Returns 0 if the key is missing or not an os.FileMode.
func StateFileMode(m map[string]any, key string) os.FileMode {
	v, ok := m[key].(os.FileMode)
	if !ok {
		return 0
	}
	return v
}

// StateStringSlice extracts a []string value from a compensation state map.
// Returns nil if the key is missing or not a []string.
func StateStringSlice(m map[string]any, key string) []string {
	v, ok := m[key].([]string)
	if !ok {
		return nil
	}
	return v
}

// ExtractUndo extracts a typed value from an undo state map.
//
// A nil map is a programming error — it means the action layer's nil guard was bypassed.
// This panics immediately so the bug is unmistakable in logs and crash reports.
func ExtractUndo[T any](undo map[string]any, key string) (T, error) {
	if undo == nil {
		panic(fmt.Sprintf("BUG: nil undo state passed to ExtractUndo (key %q) — the action layer must guard nil before calling Compensate*", key))
	}
	val, ok := undo[key].(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("invalid undo state: expected %T for key %q", zero, key)
	}
	return val, nil
}
