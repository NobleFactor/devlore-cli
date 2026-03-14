// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pwsh

import (
	"bytes"
	"strings"
	"testing"
)

func TestAvailable(t *testing.T) {
	if !Available() {
		t.Fatalf("requires PowerShell: install pwsh (brew install powershell or https://github.com/PowerShell/PowerShell)")
	}
}

func TestVersion(t *testing.T) {
	if !Available() {
		t.Fatalf("requires PowerShell: install pwsh (brew install powershell or https://github.com/PowerShell/PowerShell)")
	}

	ver, err := Version()
	if err != nil {
		t.Fatalf("failed to get version: %v", err)
	}
	if ver == "" {
		t.Error("version is empty")
	}
	t.Logf("PowerShell version: %s", ver)
}

func TestRun(t *testing.T) {
	if !Available() {
		t.Fatalf("requires PowerShell: install pwsh (brew install powershell or https://github.com/PowerShell/PowerShell)")
	}

	s, err := New()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer func() { _ = s.Close() }()

	result := s.Run("Write-Output 'hello'")

	if result.Failed() {
		t.Errorf("expected success, got: %s", result.String())
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Errorf("expected 'hello', got: %q", result.Stdout)
	}
}

func TestVariablePersistence(t *testing.T) {
	if !Available() {
		t.Fatalf("requires PowerShell: install pwsh (brew install powershell or https://github.com/PowerShell/PowerShell)")
	}

	s, err := New()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Set a variable
	s.Run("$greeting = 'Hello, World'")

	// Read it back
	result := s.Run("Write-Output $greeting")

	if result.Failed() {
		t.Errorf("expected success, got: %s", result.String())
	}
	if strings.TrimSpace(result.Stdout) != "Hello, World" {
		t.Errorf("expected 'Hello, World', got: %q", result.Stdout)
	}
}

func TestSet(t *testing.T) {
	if !Available() {
		t.Fatalf("requires PowerShell: install pwsh (brew install powershell or https://github.com/PowerShell/PowerShell)")
	}

	s, err := New()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer func() { _ = s.Close() }()

	s.Set("MY_VAR", "my_value")
	result := s.Run("Write-Output $MY_VAR")

	if result.Failed() {
		t.Errorf("expected success, got: %s", result.String())
	}
	if strings.TrimSpace(result.Stdout) != "my_value" {
		t.Errorf("expected 'my_value', got: %q", result.Stdout)
	}
}

func TestAudit(t *testing.T) {
	if !Available() {
		t.Fatalf("requires PowerShell: install pwsh (brew install powershell or https://github.com/PowerShell/PowerShell)")
	}

	var buf bytes.Buffer
	s, err := New()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer func() { _ = s.Close() }()

	s.Audit(&buf)
	s.Run("Write-Output 'test'")

	if !strings.Contains(buf.String(), `"command": "Write-Output 'test'"`) {
		t.Errorf("audit log missing command, got: %s", buf.String())
	}
	if !strings.Contains(buf.String(), `"exit_code": 0`) {
		t.Errorf("audit log missing exit_code, got: %s", buf.String())
	}
}

func TestHistory(t *testing.T) {
	if !Available() {
		t.Fatalf("requires PowerShell: install pwsh (brew install powershell or https://github.com/PowerShell/PowerShell)")
	}

	s, err := New()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer func() { _ = s.Close() }()

	s.Run("Write-Output 'one'")
	s.Run("Write-Output 'two'")
	s.Run("Write-Output 'three'")

	if len(s.History()) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(s.History()))
	}
}

func TestFailingCommand(t *testing.T) {
	if !Available() {
		t.Fatalf("requires PowerShell: install pwsh (brew install powershell or https://github.com/PowerShell/PowerShell)")
	}

	s, err := New()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer func() { _ = s.Close() }()

	result := s.Run("exit 42")

	if result.OK() {
		t.Error("expected failure")
	}
	if result.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", result.ExitCode)
	}
}

func TestScript(t *testing.T) {
	if !Available() {
		t.Fatalf("requires PowerShell: install pwsh (brew install powershell or https://github.com/PowerShell/PowerShell)")
	}

	s, err := New()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer func() { _ = s.Close() }()

	result := s.Script(
		"Write-Output 'one'",
		"Write-Output 'two'",
		"Write-Output 'three'",
	)

	if result.Failed() {
		t.Errorf("expected success, got: %s", result.String())
	}
	if len(s.History()) != 3 {
		t.Errorf("expected 3 history entries, got %d", len(s.History()))
	}
}

func TestScriptStopsOnFailure(t *testing.T) {
	if !Available() {
		t.Fatalf("requires PowerShell: install pwsh (brew install powershell or https://github.com/PowerShell/PowerShell)")
	}

	s, err := New()
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	defer func() { _ = s.Close() }()

	result := s.Script(
		"Write-Output 'one'",
		"exit 1",
		"Write-Output 'three'",
	)

	if result.OK() {
		t.Error("expected failure")
	}
	if len(s.History()) != 2 {
		t.Errorf("expected 2 history entries (stopped at failure), got %d", len(s.History()))
	}
}

func TestQuotePowerShell(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "'hello'"},
		{"it's a test", "'it''s a test'"},
		{"", "''"},
		{"quote's 'inside'", "'quote''s ''inside'''"},
	}

	for _, tt := range tests {
		got := quotePowerShell(tt.input)
		if got != tt.expected {
			t.Errorf("quotePowerShell(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
