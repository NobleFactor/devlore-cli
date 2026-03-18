// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package template //nolint:revive // package name is domain-specific

import (
	"strings"
	"testing"
)

func TestRenderText_Simple(t *testing.T) {
	p := &Provider{}
	data := map[string]any{
		"Source":  "/src/file.txt",
		"Target":  "/dst/file.txt",
		"Project": "myproject",
	}

	got, err := p.RenderText("src={{ .Source }} dst={{ .Target }} proj={{ .Project }}", data)
	if err != nil {
		t.Fatalf("RenderText() error = %v", err)
	}

	want := "src=/src/file.txt dst=/dst/file.txt proj=myproject"
	if got != want {
		t.Errorf("RenderText() = %q, want %q", got, want)
	}
}

func TestRenderText_WithVars(t *testing.T) {
	p := &Provider{}
	data := map[string]any{
		"user":    "alice",
		"count":   42,
		"Project": "proj",
	}

	got, err := p.RenderText("user={{ .user }} count={{ .count }} project={{ .Project }}", data)
	if err != nil {
		t.Fatalf("RenderText() error = %v", err)
	}

	want := "user=alice count=42 project=proj"
	if got != want {
		t.Errorf("RenderText() = %q, want %q", got, want)
	}
}

func TestRenderBytes_Simple(t *testing.T) {
	p := &Provider{}
	data := map[string]any{"Name": "world"}

	got, err := p.RenderBytes([]byte("hello {{ .Name }}"), data)
	if err != nil {
		t.Fatalf("RenderBytes() error = %v", err)
	}

	want := []byte("hello world")
	if string(got) != string(want) {
		t.Errorf("RenderBytes() = %q, want %q", got, want)
	}
}

func TestRenderText_NilData(t *testing.T) {
	p := &Provider{}

	got, err := p.RenderText("static content", nil)
	if err != nil {
		t.Fatalf("RenderText() error = %v", err)
	}
	if got != "static content" {
		t.Errorf("RenderText() = %q, want 'static content'", got)
	}
}

func TestRenderText_ParseError(t *testing.T) {
	p := &Provider{}

	_, err := p.RenderText("{{ broken", nil)
	if err == nil {
		t.Fatal("RenderText() expected error for invalid template syntax")
	}
	if !strings.Contains(err.Error(), "parse template") {
		t.Errorf("error = %q, want message containing %q", err, "parse template")
	}
}

func TestRenderText_ExecuteError(t *testing.T) {
	p := &Provider{}

	_, err := p.RenderText("{{ .Source.NonExistent }}", map[string]any{"Source": "value"})
	if err == nil {
		t.Fatal("RenderText() expected error for invalid field access on string")
	}
	if !strings.Contains(err.Error(), "execute template") {
		t.Errorf("error = %q, want message containing %q", err, "execute template")
	}
}
