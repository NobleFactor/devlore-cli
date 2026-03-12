// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package shell

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func newTestProvider() *Provider {
	return &Provider{
		ProviderBase: op.NewProviderBase(op.Context{
			ContextBase: op.ContextBase{
				Writer: &bytes.Buffer{},
			},
		}),
	}
}

func TestExecSuccess(t *testing.T) {
	p := newTestProvider()

	summary, err := p.Exec("echo hello")
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if summary != "echo hello" {
		t.Errorf("summary = %q, want %q", summary, "echo hello")
	}
	buf := p.Context().Writer.(*bytes.Buffer)
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("output = %q, want it to contain %q", buf.String(), "hello")
	}
}

func TestExecEmptyCommand(t *testing.T) {
	p := newTestProvider()

	summary, err := p.Exec("")
	if err == nil {
		t.Fatal("Exec() with empty command should return error")
	}
	if !strings.Contains(err.Error(), "no command specified") {
		t.Errorf("error = %q, want message containing %q", err, "no command specified")
	}
	if summary != "" {
		t.Errorf("summary = %q, want empty", summary)
	}
}

func TestExecFailure(t *testing.T) {
	p := newTestProvider()

	_, err := p.Exec("exit 1")
	if err == nil {
		t.Fatal("Exec() with 'exit 1' should return non-nil error")
	}
}
