// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

// newProviderWithCapture returns a *Provider whose runtime environment carries a [status.Narrator]
// wrapping a buffer-backed [sink.Sink]. The buffer is returned alongside so tests can assert on the
// bytes the narrator emitted.
//
// The provider holds no rendering state of its own — every method forwards directly to the
// Narrator on the runtime environment. These tests verify the forwarding wiring by checking that
// the expected category symbol + message appear in the captured bytes; the rendering behavior
// (program-name prefix, color codes) is exercised by pkg/status tests.
func newProviderWithCapture(t *testing.T) (*Provider, *bytes.Buffer) {
	t.Helper()
	s, buf := sink.Capture()
	env := &op.RuntimeEnvironment{Status: status.NewNarrator("test", s)}
	return NewProvider(env), buf
}

// --- Note ---

func TestNote_Forwards(t *testing.T) {
	p, buf := newProviderWithCapture(t)

	p.Note("hello")

	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("Note forward: output = %q, want substring %q", buf.String(), "hello")
	}
}

// --- Warn ---

func TestWarn_Forwards(t *testing.T) {
	p, buf := newProviderWithCapture(t)

	p.Warn("alert")

	if !strings.Contains(buf.String(), "alert") {
		t.Errorf("Warn forward: output = %q, want substring %q", buf.String(), "alert")
	}
}

// --- Error ---

func TestError_Forwards(t *testing.T) {
	p, buf := newProviderWithCapture(t)

	p.Error("oops")

	if !strings.Contains(buf.String(), "oops") {
		t.Errorf("Error forward: output = %q, want substring %q", buf.String(), "oops")
	}
}

// --- Succeed ---

func TestSucceed_Forwards(t *testing.T) {
	p, buf := newProviderWithCapture(t)

	p.Succeed("done")

	if !strings.Contains(buf.String(), "done") {
		t.Errorf("Success forward: output = %q, want substring %q", buf.String(), "done")
	}
}

// --- Fail ---

func TestFail_Forwards(t *testing.T) {
	p, buf := newProviderWithCapture(t)

	err := p.Fail("broken")

	if err == nil {
		t.Fatal("Fail() returned nil error, want non-nil")
	}
	if err.Error() != "broken" {
		t.Errorf("error text = %q, want %q", err.Error(), "broken")
	}
	if !strings.Contains(buf.String(), "broken") {
		t.Errorf("Fail forward: output = %q, want substring %q", buf.String(), "broken")
	}
}

// --- Print ---

func TestPrint_Forwards(t *testing.T) {
	p, buf := newProviderWithCapture(t)

	p.Print("raw text")

	if !strings.Contains(buf.String(), "raw text") {
		t.Errorf("Print forward: output = %q, want substring %q", buf.String(), "raw text")
	}
}
