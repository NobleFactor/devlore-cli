// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package yaml

import (
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Parse ---

// TestParse_ProducerStamp verifies the empty-producer-stamp behavior for non-graph dispatch.
//
// Under graph dispatch the producerID would be activation.Unit.ID(); under non-graph dispatch (this test
// fixture) Unit is nil and the catalog records an empty producer stamp.
func TestParse_ProducerStamp(t *testing.T) {
	runtimeEnvironment := &op.RuntimeEnvironment{ResourceCatalog: op.NewResourceCatalog()}
	p := &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
	activation := op.NewActivationRecord(nil, nil, runtimeEnvironment)

	r, err := p.Parse(activation, "hello: world\n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := r.ProducerID(); got != "" {
		t.Errorf("producerID = %q, want empty (nil Unit)", got)
	}
}

// --- Encode ---

func TestEncode_MarshalsValues(t *testing.T) {
	p := &Provider{}
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"string", "hello", "hello\n"},
		{"number", 42, "42\n"},
		{"bool", true, "true\n"},
		{"null", nil, "null\n"},
		{"map", map[string]any{"a": 1}, "a: 1\n"},
		{"slice", []any{1, "two"}, "- 1\n- two\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Encode(tt.input)
			if err != nil {
				t.Fatalf("Encode() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Encode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEncode_Error(t *testing.T) {
	p := &Provider{}
	_, err := p.Encode(make(chan int))
	if err == nil {
		t.Fatal("Encode(chan) should fail")
	}
}

func TestEncode_NestedMap(t *testing.T) {
	p := &Provider{}
	input := map[string]any{
		"outer": map[string]any{
			"inner": "value",
		},
	}
	got, err := p.Encode(input)
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
	if !strings.Contains(got, "inner: value") {
		t.Errorf("Encode() = %q, want nested map with 'inner: value'", got)
	}
}

// --- Decode ---

func TestDecode_ParsesValues(t *testing.T) {
	p := &Provider{}
	tests := []struct {
		name  string
		input string
		check func(any) bool
	}{
		{"string", "hello", func(v any) bool { return v == "hello" }},
		{"number", "42", func(v any) bool { return v == 42 }},
		{"bool", "true", func(v any) bool { return v == true }},
		{"null", "null", func(v any) bool { return v == nil }},
		{"map", "a: 1", func(v any) bool {
			m, ok := v.(map[string]any)
			return ok && m["a"] == 1
		}},
		{"sequence", "- 1\n- two", func(v any) bool {
			s, ok := v.([]any)
			return ok && len(s) == 2
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Decode(tt.input)
			if err != nil {
				t.Fatalf("Decode() error: %v", err)
			}
			if !tt.check(got) {
				t.Errorf("Decode(%q) = %v (%T), unexpected", tt.input, got, got)
			}
		})
	}
}

func TestDecode_Error(t *testing.T) {
	p := &Provider{}
	_, err := p.Decode(":\n  :\n    - :\n      bad: [")
	if err == nil {
		t.Fatal("Decode(invalid) should fail")
	}
}

// --- RoundTrip ---

func TestRoundTrip_EncodeDecode(t *testing.T) {
	p := &Provider{}
	input := map[string]any{
		"name":    "test",
		"version": 1,
		"tags":    []any{"a", "b"},
	}
	encoded, err := p.Encode(input)
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
	decoded, err := p.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error: %v", err)
	}
	m, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("decoded = %T, want map[string]any", decoded)
	}
	if m["name"] != "test" {
		t.Errorf("name = %v, want 'test'", m["name"])
	}
	if m["version"] != 1 {
		t.Errorf("version = %v, want 1", m["version"])
	}
	tags, ok := m["tags"].([]any)
	if !ok || len(tags) != 2 {
		t.Errorf("tags = %v, want [a b]", m["tags"])
	}
}
