// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package shell

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	sh := New()
	result := sh.Run("echo hello")

	if result.Failed() {
		t.Errorf("expected success, got: %s", result.String())
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Errorf("expected 'hello', got: %q", result.Stdout)
	}
}

func TestPipes(t *testing.T) {
	sh := New()
	result := sh.Run("echo hello | tr a-z A-Z")

	if result.Failed() {
		t.Errorf("expected success, got: %s", result.String())
	}
	if strings.TrimSpace(result.Stdout) != "HELLO" {
		t.Errorf("expected 'HELLO', got: %q", result.Stdout)
	}
}

func TestLoops(t *testing.T) {
	sh := New()
	result := sh.Run("for i in a b c; do echo $i; done")

	if result.Failed() {
		t.Errorf("expected success, got: %s", result.String())
	}
	if strings.TrimSpace(result.Stdout) != "a\nb\nc" {
		t.Errorf("expected 'a\\nb\\nc', got: %q", result.Stdout)
	}
}

func TestSet(t *testing.T) {
	sh := New()
	sh.Set("MY_VAR", "my_value")
	result := sh.Run("echo $MY_VAR")

	if result.Failed() {
		t.Errorf("expected success, got: %s", result.String())
	}
	if strings.TrimSpace(result.Stdout) != "my_value" {
		t.Errorf("expected 'my_value', got: %q", result.Stdout)
	}
}

func TestWith(t *testing.T) {
	sh := New()
	sh.Set("BASE", "base_value")

	// With creates a copy with additional env
	result := sh.With("EXTRA", "extra_value").Run("echo $BASE $EXTRA")

	if result.Failed() {
		t.Errorf("expected success, got: %s", result.String())
	}
	if strings.TrimSpace(result.Stdout) != "base_value extra_value" {
		t.Errorf("expected 'base_value extra_value', got: %q", result.Stdout)
	}

	// Original session doesn't have EXTRA
	result = sh.Run("echo ${EXTRA:-unset}")
	if strings.TrimSpace(result.Stdout) != "unset" {
		t.Errorf("expected 'unset', got: %q", result.Stdout)
	}
}

func TestAudit(t *testing.T) {
	var buf bytes.Buffer
	sh := New().Audit(&buf)

	sh.Run("echo test")

	if !strings.Contains(buf.String(), `"command": "echo test"`) {
		t.Errorf("audit log missing command, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"exit_code": 0`) {
		t.Errorf("audit log missing exit_code, got: %s", buf.String())
	}
}

func TestHistory(t *testing.T) {
	sh := New()
	sh.Run("echo one")
	sh.Run("echo two")
	sh.Run("echo three")

	if len(sh.History()) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(sh.History()))
	}
}

func TestFailingCommand(t *testing.T) {
	sh := New()
	result := sh.Run("exit 42")

	if result.OK() {
		t.Error("expected failure")
	}
	if result.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", result.ExitCode)
	}
}

func TestScript(t *testing.T) {
	sh := New()
	result := sh.Script(
		"echo one",
		"echo two",
		"echo three",
	)

	if result.Failed() {
		t.Errorf("expected success, got: %s", result.String())
	}
	if len(sh.History()) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(sh.History()))
	}
}

func TestScriptStopsOnFailure(t *testing.T) {
	sh := New()
	result := sh.Script(
		"echo one",
		"exit 1",
		"echo three",
	)

	if result.OK() {
		t.Error("expected failure")
	}
	if len(sh.History()) != 2 {
		t.Errorf("expected 2 history entries (stopped at failure), got %d", len(sh.History()))
	}
}

func TestChaining(t *testing.T) {
	var buf bytes.Buffer

	// Fluent API
	sh := New().
		Dir("/tmp").
		Set("FOO", "bar").
		Audit(&buf)

	result := sh.Run("pwd && echo $FOO")

	if result.Failed() {
		t.Errorf("expected success, got: %s", result.String())
	}
	if !strings.Contains(result.Stdout, "/tmp") {
		t.Errorf("expected /tmp in output, got: %q", result.Stdout)
	}
	if !strings.Contains(result.Stdout, "bar") {
		t.Errorf("expected 'bar' in output, got: %q", result.Stdout)
	}
}
