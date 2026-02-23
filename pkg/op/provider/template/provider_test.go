// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package template //nolint:revive // package name is domain-specific

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderSimple(t *testing.T) {
	p := &Provider{}
	content := []byte("src={{ .Source }} dst={{ .Target }} proj={{ .Project }}")

	got, err := p.Render(nil, "/src/file.txt", "/dst/file.txt", "myproject", content)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := []byte("src=/src/file.txt dst=/dst/file.txt proj=myproject")
	if !bytes.Equal(got, want) {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderWithVars(t *testing.T) {
	p := &Provider{}
	templateData := map[string]any{
		"user":  "alice",
		"count": 42,
	}
	content := []byte("user={{ .user }} count={{ .count }} project={{ .Project }}")

	got, err := p.Render(templateData, "src.txt", "dst.txt", "proj", content)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	want := []byte("user=alice count=42 project=proj")
	if !bytes.Equal(got, want) {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderOverrideBuiltins(t *testing.T) {
	p := &Provider{}
	templateData := map[string]any{
		"Source": "user-supplied-source",
	}
	content := []byte("{{ .Source }}")

	got, err := p.Render(templateData, "param-source", "param-target", "proj", content)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	// Built-in assignment of data["Source"] = source happens after the
	// templateData loop, so the parameter value overrides user data.
	want := []byte("param-source")
	if !bytes.Equal(got, want) {
		t.Errorf("Render() = %q, want %q", got, want)
	}
}

func TestRenderParseError(t *testing.T) {
	p := &Provider{}
	content := []byte("{{ broken")

	_, err := p.Render(nil, "", "", "", content)
	if err == nil {
		t.Fatal("Render() expected error for invalid template syntax")
	}
	if !strings.Contains(err.Error(), "parse template") {
		t.Errorf("error = %q, want message containing %q", err, "parse template")
	}
}

func TestRenderExecuteError(t *testing.T) {
	p := &Provider{}
	// .Source is a string; accessing .NonExistent on a string triggers an
	// execution error because string has no such field or method.
	content := []byte("{{ .Source.NonExistent }}")

	_, err := p.Render(nil, "value", "", "", content)
	if err == nil {
		t.Fatal("Render() expected error for invalid field access on string")
	}
	if !strings.Contains(err.Error(), "execute template") {
		t.Errorf("error = %q, want message containing %q", err, "execute template")
	}
}
