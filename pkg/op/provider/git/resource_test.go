// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- gitURI ---

func TestGitURI_LocalClone(t *testing.T) {
	want := "git:/tmp/repo"
	if got := gitURI("", "/tmp/repo", ""); got != want {
		t.Errorf("gitURI() = %q, want %q", got, want)
	}
}

func TestGitURI_RemoteURL(t *testing.T) {
	want := "git:https://github.com/org/repo"
	if got := gitURI("https://github.com/org/repo", "/tmp/repo", ""); got != want {
		t.Errorf("gitURI() = %q, want %q", got, want)
	}
}

func TestGitURI_WithRef(t *testing.T) {
	want := "git:https://github.com/org/repo#abc123"
	if got := gitURI("https://github.com/org/repo", "", "abc123"); got != want {
		t.Errorf("gitURI() = %q, want %q", got, want)
	}
}

func TestGitURI_EscapesFragment(t *testing.T) {
	want := "git:https://github.com/org/repo%23readme#main"
	if got := gitURI("https://github.com/org/repo#readme", "", "main"); got != want {
		t.Errorf("gitURI() = %q, want %q", got, want)
	}
}

func TestGitURI_EscapesQuery(t *testing.T) {
	want := "git:https://github.com/org/repo%3Ftoken=abc"
	if got := gitURI("https://github.com/org/repo?token=abc", "", ""); got != want {
		t.Errorf("gitURI() = %q, want %q", got, want)
	}
}

func TestGitURI_LocalCloneScheme(t *testing.T) {
	r := Resource{
		ResourceBase: op.NewResourceBase(nil, gitURI("", "/tmp/repo", "")),
		ClonePath:    "/tmp/repo",
	}
	if r.Scheme() != "git" {
		t.Errorf("Scheme() = %q, want %q", r.Scheme(), "git")
	}
	if r.Path() != "/tmp/repo" {
		t.Errorf("Path() = %q, want %q", r.Path(), "/tmp/repo")
	}
}

func TestGitURI_RemoteOpaqueScheme(t *testing.T) {
	r := Resource{
		ResourceBase: op.NewResourceBase(nil, gitURI("https://github.com/org/repo", "", "")),
		URL:          "https://github.com/org/repo",
	}
	if r.Scheme() != "git" {
		t.Errorf("Scheme() = %q, want %q", r.Scheme(), "git")
	}
	if r.Opaque() != "https://github.com/org/repo" {
		t.Errorf("Opaque() = %q, want %q", r.Opaque(), "https://github.com/org/repo")
	}
}

// --- Interface guards ---

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

func TestTombstoneImplementsInterface(t *testing.T) {
	var _ op.Tombstone = (*Tombstone)(nil)
}

// --- escapeInnerURI / unescapeInnerURI ---

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
