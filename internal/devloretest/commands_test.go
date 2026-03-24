// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devloretest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/starlarktest"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// writeScript creates a temporary Starlark test script and returns its path.
func writeScript(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.star")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runCmd executes the run subcommand with outputs routed to temp files.
// Returns parsed summary and any error from execution.
func runCmd(t *testing.T, script string, extraArgs ...string) (starlarktest.Result, error) {
	t.Helper()
	dir := t.TempDir()
	summaryFile := filepath.Join(dir, "summary.json")
	receiptFile := filepath.Join(dir, "receipt.yaml")
	graphFile := filepath.Join(dir, "graph.txt")

	args := []string{
		"--output", "summary=" + summaryFile,
		"--output", "receipt=" + receiptFile,
		"--output", "graph=" + graphFile,
	}
	args = append(args, extraArgs...)
	args = append(args, script)

	cmd := newRunCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	err := cmd.Execute()

	var result starlarktest.Result
	if data, readErr := os.ReadFile(summaryFile); readErr == nil {
		_ = json.Unmarshal(data, &result)
	}
	return result, err
}

// --- outputFlags ---

func TestOutputFlags_Set_ValidKeys(t *testing.T) {
	of := &outputFlags{entries: make(map[string]string)}
	for _, key := range []string{"summary", "receipt", "graph"} {
		val := key + "=/tmp/out"
		if err := of.Set(val); err != nil {
			t.Errorf("Set(%q): %v", val, err)
		}
		if of.entries[key] != "/tmp/out" {
			t.Errorf("entries[%q] = %q, want /tmp/out", key, of.entries[key])
		}
	}
}

func TestOutputFlags_Set_InvalidKey(t *testing.T) {
	of := &outputFlags{entries: make(map[string]string)}
	if err := of.Set("bogus=/tmp/out"); err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestOutputFlags_Set_MissingEquals(t *testing.T) {
	of := &outputFlags{entries: make(map[string]string)}
	if err := of.Set("summary"); err == nil {
		t.Fatal("expected error for missing =")
	}
}

func TestOutputFlags_Type(t *testing.T) {
	of := &outputFlags{}
	if got := of.Type(); got != "stream=dest" {
		t.Errorf("ProviderType() = %q, want \"stream=dest\"", got)
	}
}

// --- runner wiring ---

func TestRunCmd_BasicExecution(t *testing.T) {
	script := writeScript(t, `t.expect_node_count(0)`)
	result, err := runCmd(t, script)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass, got failures: %v", result.Failures)
	}
	if result.NodeCount != 0 {
		t.Errorf("node_count = %d, want 0", result.NodeCount)
	}
}

func TestRunCmd_DryRun(t *testing.T) {
	script := writeScript(t, `
plan.shell.exec(command="echo hello")
t.expect_node_count(1)
`)
	result, err := runCmd(t, script, "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass, got failures: %v", result.Failures)
	}
	if result.NodeCount != 1 {
		t.Errorf("node_count = %d, want 1", result.NodeCount)
	}
}

func TestRunCmd_Trace(t *testing.T) {
	script := writeScript(t, `t.expect_node_count(0)`)
	result, err := runCmd(t, script, "--trace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Trace) == 0 {
		t.Error("expected trace entries, got none")
	}
}

func TestRunCmd_InvalidReceiptFormat(t *testing.T) {
	script := writeScript(t, `t.expect_node_count(0)`)
	_, err := runCmd(t, script, "--receipt-format", "xml")
	if err == nil {
		t.Fatal("expected error for invalid receipt format")
	}
}

func TestRunCmd_ReceiptJSON(t *testing.T) {
	dir := t.TempDir()
	receiptFile := filepath.Join(dir, "receipt.json")
	script := writeScript(t, `
plan.shell.exec(command="echo hello")
t.expect_node_count(1)
`)
	cmd := newRunCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{
		"--dry-run",
		"--receipt-format", "json",
		"--output", "summary=/dev/null",
		"--output", "receipt=" + receiptFile,
		"--output", "graph=/dev/null",
		script,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(receiptFile)
	if err != nil {
		t.Fatalf("reading receipt: %v", err)
	}
	if !json.Valid(data) {
		t.Errorf("receipt is not valid JSON: %s", data)
	}
}

func TestRunCmd_OutputRouting(t *testing.T) {
	dir := t.TempDir()
	summaryFile := filepath.Join(dir, "summary.json")
	receiptFile := filepath.Join(dir, "receipt.yaml")

	script := writeScript(t, `
plan.shell.exec(command="echo routed")
t.expect_node_count(1)
`)
	cmd := newRunCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{
		"--dry-run",
		"--output", "summary=" + summaryFile,
		"--output", "receipt=" + receiptFile,
		"--output", "graph=/dev/null",
		script,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sData, err := os.ReadFile(summaryFile)
	if err != nil {
		t.Fatalf("summary not written: %v", err)
	}
	if !json.Valid(sData) {
		t.Errorf("summary is not valid JSON: %s", sData)
	}

	rData, err := os.ReadFile(receiptFile)
	if err != nil {
		t.Fatalf("receipt not written: %v", err)
	}
	if len(rData) == 0 {
		t.Error("receipt file is empty")
	}
}

func TestRunCmd_MissingScript(t *testing.T) {
	_, err := runCmd(t, "/nonexistent/test.star")
	if err == nil {
		t.Fatal("expected error for missing script")
	}
}
