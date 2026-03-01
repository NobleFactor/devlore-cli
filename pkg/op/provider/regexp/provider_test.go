// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package regexp

import (
	"testing"
)

func TestMatch(t *testing.T) {
	p := &Provider{}
	tests := []struct {
		pattern string
		input   string
		want    bool
	}{
		{`\d+`, "abc123", true},
		{`\d+`, "abcdef", false},
		{`^hello`, "hello world", true},
		{`^hello`, "say hello", false},
		{`(?i)test`, "Testing", true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.input, func(t *testing.T) {
			got, err := p.Match(tt.pattern, tt.input)
			if err != nil {
				t.Fatalf("Match() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.pattern, tt.input, got, tt.want)
			}
		})
	}
}

func TestMatch_InvalidPattern(t *testing.T) {
	p := &Provider{}
	_, err := p.Match("[invalid", "test")
	if err == nil {
		t.Fatal("Match(invalid pattern) should fail")
	}
}

func TestFind(t *testing.T) {
	p := &Provider{}
	tests := []struct {
		pattern string
		input   string
		want    string
	}{
		{`\d+`, "abc123def456", "123"},
		{`\d+`, "abcdef", ""},
		{`\w+@\w+`, "user@host other", "user@host"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.input, func(t *testing.T) {
			got, err := p.Find(tt.pattern, tt.input)
			if err != nil {
				t.Fatalf("Find() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Find(%q, %q) = %q, want %q", tt.pattern, tt.input, got, tt.want)
			}
		})
	}
}

func TestFindAll(t *testing.T) {
	p := &Provider{}

	got, err := p.FindAll(`\d+`, "a1b2c3", -1)
	if err != nil {
		t.Fatalf("FindAll() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("FindAll() = %v (len %d), want 3 matches", got, len(got))
	}
	if got[0] != "1" || got[1] != "2" || got[2] != "3" {
		t.Errorf("FindAll() = %v, want [1 2 3]", got)
	}
}

func TestFindAll_WithCount(t *testing.T) {
	p := &Provider{}

	got, err := p.FindAll(`\d+`, "a1b2c3", 2)
	if err != nil {
		t.Fatalf("FindAll() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("FindAll(count=2) = %v (len %d), want 2 matches", got, len(got))
	}
}

func TestFindAll_NoMatch(t *testing.T) {
	p := &Provider{}

	got, err := p.FindAll(`\d+`, "abcdef", -1)
	if err != nil {
		t.Fatalf("FindAll() error: %v", err)
	}
	if got != nil {
		t.Errorf("FindAll() = %v, want nil", got)
	}
}

func TestFindSubmatch(t *testing.T) {
	p := &Provider{}

	got, err := p.FindSubmatch(`(\w+)@(\w+)`, "user@host")
	if err != nil {
		t.Fatalf("FindSubmatch() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("FindSubmatch() = %v (len %d), want 3", got, len(got))
	}
	if got[0] != "user@host" {
		t.Errorf("[0] = %q, want 'user@host'", got[0])
	}
	if got[1] != "user" {
		t.Errorf("[1] = %q, want 'user'", got[1])
	}
	if got[2] != "host" {
		t.Errorf("[2] = %q, want 'host'", got[2])
	}
}

func TestFindSubmatch_NoMatch(t *testing.T) {
	p := &Provider{}

	got, err := p.FindSubmatch(`(\d+)`, "abcdef")
	if err != nil {
		t.Fatalf("FindSubmatch() error: %v", err)
	}
	if got != nil {
		t.Errorf("FindSubmatch() = %v, want nil", got)
	}
}

func TestFindAllSubmatch(t *testing.T) {
	p := &Provider{}

	got, err := p.FindAllSubmatch(`(\w+)=(\w+)`, "a=1 b=2", -1)
	if err != nil {
		t.Fatalf("FindAllSubmatch() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("FindAllSubmatch() = %v (len %d), want 2", got, len(got))
	}
	if got[0][1] != "a" || got[0][2] != "1" {
		t.Errorf("[0] = %v, want [a=1 a 1]", got[0])
	}
	if got[1][1] != "b" || got[1][2] != "2" {
		t.Errorf("[1] = %v, want [b=2 b 2]", got[1])
	}
}

func TestReplace(t *testing.T) {
	p := &Provider{}
	tests := []struct {
		name        string
		pattern     string
		input       string
		replacement string
		want        string
	}{
		{"simple", `\d+`, "abc123def", "NUM", "abcNUMdef"},
		{"submatch", `(\w+)@(\w+)`, "user@host", "${2}/${1}", "host/user"},
		{"no_match", `\d+`, "abcdef", "NUM", "abcdef"},
		{"multiple", `\s+`, "a b  c", "-", "a-b-c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.Replace(tt.pattern, tt.input, tt.replacement)
			if err != nil {
				t.Fatalf("Replace() error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Replace() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReplaceLiteral(t *testing.T) {
	p := &Provider{}

	// ReplaceLiteral should NOT expand $1.
	got, err := p.ReplaceLiteral(`\d+`, "abc123def", "$1")
	if err != nil {
		t.Fatalf("ReplaceLiteral() error: %v", err)
	}
	if got != "abc$1def" {
		t.Errorf("ReplaceLiteral() = %q, want 'abc$1def'", got)
	}
}

func TestSplit(t *testing.T) {
	p := &Provider{}

	got, err := p.Split(`\s+`, "a b  c", -1)
	if err != nil {
		t.Fatalf("Split() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Split() = %v (len %d), want 3", got, len(got))
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("Split() = %v, want [a b c]", got)
	}
}

func TestSplit_WithCount(t *testing.T) {
	p := &Provider{}

	got, err := p.Split(`\s+`, "a b c d", 2)
	if err != nil {
		t.Fatalf("Split() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Split(count=2) = %v (len %d), want 2", got, len(got))
	}
	if got[0] != "a" || got[1] != "b c d" {
		t.Errorf("Split(count=2) = %v, want [a 'b c d']", got)
	}
}

func TestCache_ReuseCompiledPattern(t *testing.T) {
	p := &Provider{}

	// Call twice with same pattern — second should use cache.
	_, err := p.Match(`\d+`, "abc123")
	if err != nil {
		t.Fatalf("first Match() error: %v", err)
	}
	_, err = p.Match(`\d+`, "def456")
	if err != nil {
		t.Fatalf("second Match() error: %v", err)
	}

	// Verify pattern was cached.
	if _, ok := p.cache.Load(`\d+`); !ok {
		t.Error("pattern not found in cache")
	}
}
