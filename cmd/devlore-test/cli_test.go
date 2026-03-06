// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package main_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// binary is the path to the compiled devlore-test binary, set by TestMain.
var binary string

// scriptPath is the absolute path to test_hello.star, set by TestMain.
var scriptPath string

func TestMain(m *testing.M) {
	// Build the binary to a temp directory.
	tmp, err := os.MkdirTemp("", "devlore-test-cli-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	binary = filepath.Join(tmp, "devlore-test")

	// Find repo root (walk up from this file's directory until we find go.mod).
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "finding repo root: %v\n", err)
		os.Exit(1)
	}

	scriptPath = filepath.Join(root, "internal", "e2e", "testrunner", "data", "test_hello.star")

	build := exec.Command("go", "build", "-o", binary, "./cmd/devlore-test")
	build.Dir = root
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "building devlore-test: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

// run executes devlore-test with the given args and returns stdout, stderr, and exit code.
func run(args ...string) (stdout, stderr string, exitCode int) {
	cmd := exec.Command(binary, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// --- Argument handling ---

func TestCLI_NoArgs(t *testing.T) {
	stdout, _, code := run()
	assertExit(t, 0, code)
	assertContains(t, stdout, "Usage:")
	assertContains(t, stdout, "devlore-test [command]")
}

func TestCLI_RunNoScript(t *testing.T) {
	_, _, code := run("run")
	assertExit(t, 1, code)
}

func TestCLI_RunTooManyArgs(t *testing.T) {
	_, _, code := run("run", "a.star", "b.star")
	assertExit(t, 1, code)
}

func TestCLI_RunMissingFile(t *testing.T) {
	_, stderr, code := run("run", "nonexistent.star")
	assertExit(t, 1, code)
	assertContains(t, stderr, "no such file")
}

func TestCLI_ScriptFirst(t *testing.T) {
	stdout, _, code := run("run", scriptPath, "--output", "receipt=/dev/null", "--output", "graph=/dev/null")
	assertExit(t, 0, code)
	assertValidSummary(t, stdout, true)
}

func TestCLI_ScriptMiddle(t *testing.T) {
	stdout, _, code := run("run", "--output", "receipt=/dev/null", scriptPath, "--output", "graph=/dev/null")
	assertExit(t, 0, code)
	assertValidSummary(t, stdout, true)
}

func TestCLI_ScriptLast(t *testing.T) {
	stdout, _, code := run("run", "--output", "receipt=/dev/null", "--output", "graph=/dev/null", scriptPath)
	assertExit(t, 0, code)
	assertValidSummary(t, stdout, true)
}

// --- Output routing ---

func TestCLI_DefaultAllToStdout(t *testing.T) {
	stdout, _, code := run("run", scriptPath)
	assertExit(t, 0, code)
	assertContains(t, stdout, "Hello World!")
	assertContains(t, stdout, `"passed":true`)
	assertContains(t, stdout, "version:")
}

func TestCLI_SummaryOnly(t *testing.T) {
	stdout, _, code := run("run", "--output", "graph=/dev/null", "--output", "receipt=/dev/null", scriptPath)
	assertExit(t, 0, code)
	assertValidSummary(t, stdout, true)
	assertNotContains(t, stdout, "Hello World!")
	assertNotContains(t, stdout, "version:")
}

func TestCLI_GraphOnly(t *testing.T) {
	stdout, _, code := run("run", "--output", "summary=/dev/null", "--output", "receipt=/dev/null", scriptPath)
	assertExit(t, 0, code)
	assertContains(t, stdout, "Hello World!")
	assertNotContains(t, stdout, `"passed"`)
	assertNotContains(t, stdout, "version:")
}

func TestCLI_ReceiptOnlyYAML(t *testing.T) {
	stdout, _, code := run("run", "--output", "graph=/dev/null", "--output", "summary=/dev/null", scriptPath)
	assertExit(t, 0, code)
	assertNotContains(t, stdout, `"passed"`)
	assertValidYAML(t, stdout)
	assertContains(t, stdout, "shell.exec")
	assertContains(t, stdout, "version:")
}

func TestCLI_ReceiptOnlyJSON(t *testing.T) {
	stdout, _, code := run("run", "--output", "graph=/dev/null", "--output", "summary=/dev/null", "--receipt-format=json", scriptPath)
	assertExit(t, 0, code)
	assertValidJSON(t, stdout)
	assertContains(t, stdout, "shell.exec")
}

func TestCLI_RoutToFiles(t *testing.T) {
	tmp := t.TempDir()
	summaryPath := filepath.Join(tmp, "summary.json")
	receiptPath := filepath.Join(tmp, "receipt.yaml")
	graphPath := filepath.Join(tmp, "graph.txt")

	_, _, code := run("run",
		"--output", "summary="+summaryPath,
		"--output", "receipt="+receiptPath,
		"--output", "graph="+graphPath,
		scriptPath)
	assertExit(t, 0, code)

	summary, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("reading summary: %v", err)
	}
	assertValidSummary(t, string(summary), true)

	receipt, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatalf("reading receipt: %v", err)
	}
	assertValidYAML(t, string(receipt))
	assertContains(t, string(receipt), "shell.exec")

	graph, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatalf("reading graph: %v", err)
	}
	assertContains(t, string(graph), "Hello World!")
}

func TestCLI_JSONReceiptToFile(t *testing.T) {
	tmp := t.TempDir()
	receiptPath := filepath.Join(tmp, "receipt.json")

	_, _, code := run("run",
		"--output", "receipt="+receiptPath,
		"--output", "graph=/dev/null",
		"--output", "summary=/dev/null",
		"--receipt-format=json",
		scriptPath)
	assertExit(t, 0, code)

	data, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatalf("reading receipt: %v", err)
	}
	assertValidJSON(t, string(data))
}

// --- Flags ---

func TestCLI_DryRun(t *testing.T) {
	stdout, _, code := run("run", "--dry-run", "--output", "graph=/dev/null", "--output", "receipt=/dev/null", scriptPath)
	assertExit(t, 0, code)
	assertValidSummary(t, stdout, true)
}

func TestCLI_Trace(t *testing.T) {
	stdout, _, code := run("run", "--trace", "--output", "graph=/dev/null", "--output", "receipt=/dev/null", scriptPath)
	assertExit(t, 0, code)
	assertContains(t, stdout, `"trace"`)
}

func TestCLI_Silent(t *testing.T) {
	_, stderr, code := run("--silent", "run", "--output", "graph=/dev/null", "--output", "receipt=/dev/null", scriptPath)
	assertExit(t, 0, code)
	if stderr != "" {
		t.Errorf("--silent should suppress stderr, got: %q", stderr)
	}
}

// --- Error cases ---

func TestCLI_InvalidOutputStream(t *testing.T) {
	_, _, code := run("run", "--output", "bogus=/dev/null", scriptPath)
	assertExit(t, 1, code)
}

func TestCLI_MalformedOutput(t *testing.T) {
	_, _, code := run("run", "--output", "nodestination", scriptPath)
	assertExit(t, 1, code)
}

func TestCLI_InvalidReceiptFormat(t *testing.T) {
	_, _, code := run("run", "--receipt-format=xml", "--output", "receipt=/dev/null", "--output", "graph=/dev/null", scriptPath)
	assertExit(t, 1, code)
}

func TestCLI_BadOutputPath(t *testing.T) {
	_, _, code := run("run", "--output", "graph=/no/such/dir/out.txt", scriptPath)
	assertExit(t, 1, code)
}

func TestCLI_UnknownFlag(t *testing.T) {
	_, _, code := run("run", "--foobar", scriptPath)
	assertExit(t, 1, code)
}

func TestCLI_UnknownCommand(t *testing.T) {
	_, _, code := run("foobar")
	assertExit(t, 1, code)
}

// --- Shared commands ---

func TestCLI_Version(t *testing.T) {
	stdout, _, code := run("version")
	assertExit(t, 0, code)
	assertContains(t, stdout, "Version:")
}

func TestCLI_Help(t *testing.T) {
	stdout, _, code := run("help")
	assertExit(t, 0, code)
	assertContains(t, stdout, "devlore-test")
}

func TestCLI_HelpRun(t *testing.T) {
	stdout, _, code := run("help", "run")
	assertExit(t, 0, code)
	assertContains(t, stdout, "<script.star>")
}

func TestCLI_ConfigPath(t *testing.T) {
	stdout, _, code := run("config", "path")
	assertExit(t, 0, code)
	assertContains(t, stdout, "devlore/config.yaml")
}

func TestCLI_SelfInstallHelp(t *testing.T) {
	stdout, _, code := run("self-install", "--help")
	assertExit(t, 0, code)
	assertContains(t, stdout, "--prefix")
}

func TestCLI_CompletionBash(t *testing.T) {
	stdout, _, code := run("completion", "bash")
	assertExit(t, 0, code)
	assertContains(t, stdout, "bash completion")
}

func TestCLI_CompletionZsh(t *testing.T) {
	stdout, _, code := run("completion", "zsh")
	assertExit(t, 0, code)
	assertContains(t, stdout, "compdef")
}

// --- Assertions ---

func assertExit(t *testing.T, want, got int) {
	t.Helper()
	if got != want {
		t.Errorf("exit code = %d, want %d", got, want)
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("output missing %q (len=%d)", substr, len(s))
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("output should not contain %q", substr)
	}
}

func assertValidSummary(t *testing.T, s string, wantPassed bool) {
	t.Helper()
	// Summary line is the first JSON line in the output.
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var result struct {
			Passed bool `json:"passed"`
		}
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			t.Errorf("summary is not valid JSON: %v", err)
			return
		}
		if result.Passed != wantPassed {
			t.Errorf("summary.passed = %v, want %v", result.Passed, wantPassed)
		}
		return
	}
	t.Error("no JSON summary found in output")
}

func assertValidJSON(t *testing.T, s string) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &v); err != nil {
		t.Errorf("invalid JSON: %v", err)
	}
}

func assertValidYAML(t *testing.T, s string) {
	t.Helper()
	var v any
	if err := yaml.Unmarshal([]byte(s), &v); err != nil {
		t.Errorf("invalid YAML: %v", err)
	}
}
