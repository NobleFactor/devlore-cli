// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package json

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// TestProducerStamp_Parse verifies the m.5(iii) contract: a forward producer-method call results in a
// catalog entry whose producerID matches the dispatch's activation SiteID. Parse delegates to NewResource;
// the catalog stamp comes from activation.SiteID via Catalog.GetOrCreate.
func TestProducerStamp_Parse(t *testing.T) {
	ctx := &op.RuntimeEnvironment{Catalog: op.NewResourceCatalog()}
	p := &Provider{ProviderBase: op.NewProviderBase(ctx)}
	activation := &op.ActivationRecord{Runtime: ctx, SiteID: "test:" + t.Name()}

	r, err := p.Parse(activation, `{"hello":"world"}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := r.ProducerID(); got != activation.SiteID {
		t.Errorf("producerID = %q, want %q", got, activation.SiteID)
	}
}

func TestEncode(t *testing.T) {
	p := &Provider{}
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"string", "hello", `"hello"`},
		{"number", 42.0, "42"},
		{"bool", true, "true"},
		{"null", nil, "null"},
		{"map", map[string]any{"a": 1.0}, `{"a":1}`},
		{"slice", []any{1.0, "two"}, `[1,"two"]`},
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

func TestEncodeIndent(t *testing.T) {
	p := &Provider{}
	got, err := p.EncodeIndent(map[string]any{"a": 1.0}, "  ")
	if err != nil {
		t.Fatalf("EncodeIndent() error: %v", err)
	}
	want := "{\n  \"a\": 1\n}"
	if got != want {
		t.Errorf("EncodeIndent() = %q, want %q", got, want)
	}
}

func TestDecode(t *testing.T) {
	p := &Provider{}
	tests := []struct {
		name  string
		input string
		check func(any) bool
	}{
		{"string", `"hello"`, func(v any) bool { return v == "hello" }},
		{"number", "42", func(v any) bool { return v == 42.0 }},
		{"bool", "true", func(v any) bool { return v == true }},
		{"null", "null", func(v any) bool { return v == nil }},
		{"object", `{"a":1}`, func(v any) bool {
			m, ok := v.(map[string]any)
			return ok && m["a"] == 1.0
		}},
		{"array", `[1,"two"]`, func(v any) bool {
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
	_, err := p.Decode("not json")
	if err == nil {
		t.Fatal("Decode(invalid) should fail")
	}
}
