// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func init() { op.RegisterConstructor(ResourceFromValue) }

func TestResourceURI_LocalClone(t *testing.T) {
	r, err := op.Construct[Resource]("/tmp/repo")
	if err != nil {
		t.Fatalf("Construct: %v", err)
	}
	want := "git:/tmp/repo"
	if got := r.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestResourceURI_RemoteURL(t *testing.T) {
	r := Resource{URL: "https://github.com/org/repo", ClonePath: "/tmp/repo"}
	r.SetURI(r.buildURI())
	want := "git:https://github.com/org/repo"
	if got := r.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestResourceURI_WithRef(t *testing.T) {
	r := Resource{URL: "https://github.com/org/repo", Ref: "abc123"}
	r.SetURI(r.buildURI())
	want := "git:https://github.com/org/repo#abc123"
	if got := r.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestResourceURI_EscapesFragment(t *testing.T) {
	r := Resource{URL: "https://github.com/org/repo#readme", Ref: "main"}
	r.SetURI(r.buildURI())
	want := "git:https://github.com/org/repo%23readme#main"
	if got := r.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestResourceURI_EscapesQuery(t *testing.T) {
	r := Resource{URL: "https://github.com/org/repo?token=abc"}
	r.SetURI(r.buildURI())
	want := "git:https://github.com/org/repo%3Ftoken=abc"
	if got := r.URI(); got != want {
		t.Errorf("URI() = %q, want %q", got, want)
	}
}

func TestResourceURI_LocalCloneScheme(t *testing.T) {
	r, err := op.Construct[Resource]("/tmp/repo")
	if err != nil {
		t.Fatalf("Construct: %v", err)
	}
	if r.Scheme() != "git" {
		t.Errorf("Scheme() = %q, want %q", r.Scheme(), "git")
	}
	// Local absolute paths produce hierarchical URIs (path starts with /)
	if r.Path() != "/tmp/repo" {
		t.Errorf("Path() = %q, want %q", r.Path(), "/tmp/repo")
	}
}

func TestResourceURI_RemoteOpaqueScheme(t *testing.T) {
	r := Resource{URL: "https://github.com/org/repo"}
	r.SetURI(r.buildURI())
	if r.Scheme() != "git" {
		t.Errorf("Scheme() = %q, want %q", r.Scheme(), "git")
	}
	if r.Opaque() != "https://github.com/org/repo" {
		t.Errorf("Opaque() = %q, want %q", r.Opaque(), "https://github.com/org/repo")
	}
}

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

func TestTombstoneImplementsInterface(t *testing.T) {
	var _ op.Tombstone = (*Tombstone)(nil)
}

func TestConstructorRoundTrip(t *testing.T) {
	r, err := op.Construct[Resource]("/tmp/myrepo")
	if err != nil {
		t.Fatalf("Construct: %v", err)
	}
	if r.ClonePath != "/tmp/myrepo" {
		t.Errorf("ClonePath = %q, want %q", r.ClonePath, "/tmp/myrepo")
	}
}

func TestEscapeInnerURI(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/path", "https://example.com/path"},
		{"https://example.com/path?q=1", "https://example.com/path%3Fq=1"},
		{"https://example.com/repo#readme", "https://example.com/repo%23readme"},
		{"https://example.com/r?q=1#frag", "https://example.com/r%3Fq=1%23frag"},
	}
	for _, tt := range tests {
		got := escapeInnerURI(tt.input)
		if got != tt.want {
			t.Errorf("escapeInnerURI(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestUnescapeInnerURI(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/path", "https://example.com/path"},
		{"https://example.com/path%3Fq=1", "https://example.com/path?q=1"},
		{"https://example.com/repo%23readme", "https://example.com/repo#readme"},
	}
	for _, tt := range tests {
		got := unescapeInnerURI(tt.input)
		if got != tt.want {
			t.Errorf("unescapeInnerURI(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
