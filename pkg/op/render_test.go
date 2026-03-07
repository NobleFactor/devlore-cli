// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"strings"
	"testing"
)

func TestRenderErrorPlainString(t *testing.T) {
	err := RenderError("disk space low", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "disk space low" {
		t.Errorf("got %q, want %q", err.Error(), "disk space low")
	}
}

func TestRenderErrorWithKwargs(t *testing.T) {
	err := RenderError("{{ .service }} is down", nil, map[string]any{"service": "redis"})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "redis is down" {
		t.Errorf("got %q, want %q", err.Error(), "redis is down")
	}
}

func TestRenderErrorWithArgs(t *testing.T) {
	err := RenderError("{{ index .Args 0 }} failed", []any{"db"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "db failed" {
		t.Errorf("got %q, want %q", err.Error(), "db failed")
	}
}

func TestRenderErrorWithArgsAndKwargs(t *testing.T) {
	err := RenderError(
		"{{ index .Args 0 }} on {{ .host }}",
		[]any{"timeout"},
		map[string]any{"host": "node-3"},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "timeout on node-3" {
		t.Errorf("got %q, want %q", err.Error(), "timeout on node-3")
	}
}

func TestRenderErrorInvalidTemplate(t *testing.T) {
	err := RenderError("{{ .unclosed", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.HasPrefix(err.Error(), "render: ") {
		t.Errorf("expected render prefix, got %q", err.Error())
	}
}

func TestRenderErrorNilArgsKwargs(t *testing.T) {
	err := RenderError("all clear", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "all clear" {
		t.Errorf("got %q, want %q", err.Error(), "all clear")
	}
}
