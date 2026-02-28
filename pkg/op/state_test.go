// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op_test

import (
	"os"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestAsStateMap(t *testing.T) {
	tests := []struct {
		name  string
		state any
		want  bool
	}{
		{"nil", nil, false},
		{"wrong type", "not a map", false},
		{"empty map", map[string]any{}, true},
		{"populated map", map[string]any{"key": "val"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := op.AsStateMap(tt.state)
			if (got != nil) != tt.want {
				t.Errorf("AsStateMap() returned nil=%v, want nil=%v", got == nil, !tt.want)
			}
		})
	}
}

func TestStateString(t *testing.T) {
	m := map[string]any{
		"name":  "sshd",
		"count": 42,
		"empty": "",
	}
	tests := []struct {
		key  string
		want string
	}{
		{"name", "sshd"},
		{"missing", ""},
		{"count", ""}, // wrong type
		{"empty", ""}, // empty string
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := op.StateString(m, tt.key); got != tt.want {
				t.Errorf("StateString(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestStateBool(t *testing.T) {
	m := map[string]any{
		"running": true,
		"stopped": false,
		"name":    "sshd",
	}
	tests := []struct {
		key  string
		want bool
	}{
		{"running", true},
		{"stopped", false},
		{"missing", false},
		{"name", false}, // wrong type
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := op.StateBool(m, tt.key); got != tt.want {
				t.Errorf("StateBool(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestStateBytes(t *testing.T) {
	content := []byte("hello world")
	m := map[string]any{
		"content": content,
		"name":    "file.txt",
	}

	if got := op.StateBytes(m, "content"); string(got) != "hello world" {
		t.Errorf("StateBytes(content) = %q, want %q", got, content)
	}
	if got := op.StateBytes(m, "missing"); got != nil {
		t.Errorf("StateBytes(missing) = %v, want nil", got)
	}
	if got := op.StateBytes(m, "name"); got != nil {
		t.Errorf("StateBytes(name) = %v, want nil (wrong type)", got)
	}
}

func TestStateFileMode(t *testing.T) {
	m := map[string]any{
		"mode":  os.FileMode(0o644),
		"zero":  os.FileMode(0),
		"wrong": "0644",
	}

	if got := op.StateFileMode(m, "mode"); got != 0o644 {
		t.Errorf("StateFileMode(mode) = %v, want 0644", got)
	}
	if got := op.StateFileMode(m, "zero"); got != 0 {
		t.Errorf("StateFileMode(zero) = %v, want 0", got)
	}
	if got := op.StateFileMode(m, "missing"); got != 0 {
		t.Errorf("StateFileMode(missing) = %v, want 0", got)
	}
	if got := op.StateFileMode(m, "wrong"); got != 0 {
		t.Errorf("StateFileMode(wrong) = %v, want 0 (wrong type)", got)
	}
}

func TestExtractUndo_NilMap_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("ExtractUndo(nil, ...) did not panic")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is %T, want string", r)
		}
		if !strings.Contains(msg, "BUG") || !strings.Contains(msg, "nil undo state") {
			t.Errorf("panic message = %q, want to contain 'BUG' and 'nil undo state'", msg)
		}
	}()
	op.ExtractUndo[string](nil, "tombstone")
}

func TestExtractUndo_ValidKey(t *testing.T) {
	m := map[string]any{"name": "test"}
	got, err := op.ExtractUndo[string](m, "name")
	if err != nil {
		t.Fatalf("ExtractUndo() error = %v", err)
	}
	if got != "test" {
		t.Errorf("ExtractUndo() = %q, want %q", got, "test")
	}
}

func TestExtractUndo_WrongType(t *testing.T) {
	m := map[string]any{"name": 42}
	_, err := op.ExtractUndo[string](m, "name")
	if err == nil {
		t.Fatal("ExtractUndo() error = nil, want error for wrong type")
	}
}

func TestExtractUndo_MissingKey(t *testing.T) {
	m := map[string]any{"other": "val"}
	_, err := op.ExtractUndo[string](m, "name")
	if err == nil {
		t.Fatal("ExtractUndo() error = nil, want error for missing key")
	}
}

func TestStateStringSlice(t *testing.T) {
	m := map[string]any{
		"packages": []string{"vim", "git"},
		"name":     "test",
		"empty":    []string{},
	}

	got := op.StateStringSlice(m, "packages")
	if len(got) != 2 || got[0] != "vim" || got[1] != "git" {
		t.Errorf("StateStringSlice(packages) = %v, want [vim git]", got)
	}
	if got := op.StateStringSlice(m, "missing"); got != nil {
		t.Errorf("StateStringSlice(missing) = %v, want nil", got)
	}
	if got := op.StateStringSlice(m, "name"); got != nil {
		t.Errorf("StateStringSlice(name) = %v, want nil (wrong type)", got)
	}
	if got := op.StateStringSlice(m, "empty"); len(got) != 0 {
		t.Errorf("StateStringSlice(empty) = %v, want []", got)
	}
}
