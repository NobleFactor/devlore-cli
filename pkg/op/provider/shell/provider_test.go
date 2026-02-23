// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package shell

import (
	"bytes"
	"strings"
	"testing"
)

func TestExecSuccess(t *testing.T) {
	p := &Provider{}
	buf := &bytes.Buffer{}

	summary, err := p.Exec("echo hello", buf)
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if summary != "echo hello" {
		t.Errorf("summary = %q, want %q", summary, "echo hello")
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("output = %q, want it to contain %q", buf.String(), "hello")
	}
}

func TestExecEmptyCommand(t *testing.T) {
	p := &Provider{}
	buf := &bytes.Buffer{}

	summary, err := p.Exec("", buf)
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
	p := &Provider{}
	buf := &bytes.Buffer{}

	_, err := p.Exec("exit 1", buf)
	if err == nil {
		t.Fatal("Exec() with 'exit 1' should return non-nil error")
	}
}
