// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package shell

import (
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func newTestProvider() *Provider {
	return &Provider{
		ProviderBase: op.NewProviderBase(&op.RuntimeEnvironment{}),
	}
}

func TestExecSuccess(t *testing.T) {
	p := newTestProvider()

	result, err := p.Exec("echo hello")
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result == nil {
		t.Fatal("Exec() returned nil result")
	}
	if result.Command != "echo hello" {
		t.Errorf("result.Command = %q, want %q", result.Command, "echo hello")
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("result.Stdout = %q, want it to contain %q", result.Stdout, "hello")
	}
	if result.ExitCode != 0 {
		t.Errorf("result.ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestExecEmptyCommand(t *testing.T) {
	p := newTestProvider()

	result, err := p.Exec("")
	if err == nil {
		t.Fatal("Exec() with empty command should return error")
	}
	if !strings.Contains(err.Error(), "no command specified") {
		t.Errorf("error = %q, want message containing %q", err, "no command specified")
	}
	if result != nil {
		t.Errorf("result = %v, want nil on empty-command error", result)
	}
}

func TestExecFailure(t *testing.T) {
	p := newTestProvider()

	result, err := p.Exec("exit 1")
	if err == nil {
		t.Fatal("Exec() with 'exit 1' should return non-nil error")
	}
	if result == nil {
		t.Fatal("Exec() should still return a populated result on subprocess failure")
	}
	if result.ExitCode == 0 {
		t.Errorf("result.ExitCode = %d, want non-zero on subprocess failure", result.ExitCode)
	}
}
