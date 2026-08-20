// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package main_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/pkg/xdg"
)

// binary is the path to the compiled devlore-test binary, set by TestMain.
var binary string

// scriptPath is the absolute path to test_hello.star, set by TestMain.
var scriptPath string

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

// testMain builds the binary and runs the suite, returning the process exit code.
//
// The extraction keeps TestMain's single os.Exit above every cleanup defer (gocritic
// exitAfterDefer).
func testMain(m *testing.M) int {
	// Build the binary to a temp directory.
	tmp, err := os.MkdirTemp("", "devlore-test-cli-*")
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "creating temp dir: %v\n", err)
		return 1
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	// `go build -o` appends .exe on Windows, so the path used to exec must carry it too —
	// otherwise every invocation fails to start and reports exit code -1 with no output.
	binary = filepath.Join(tmp, "devlore-test")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	// Find repo root (walk up from this file's directory until we find go.mod).
	root, err := findRepoRoot()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "finding repo root: %v\n", err)
		return 1
	}

	scriptPath = filepath.Join(root, "cmd", "devlore-test", "devloretest", "data", "test_hello.star")

	// TestMain runs outside any test; Background is the honest lifetime here.
	build := exec.CommandContext(context.Background(), "go", "build", "-o", binary, "./cmd/devlore-test")
	build.Dir = root
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "building devlore-test: %v\n", err)
		return 1
	}

	return m.Run()
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
	return runIn("", args...)
}

// runIn executes devlore-test with the given working directory (empty means inherit) — the ruled defaults
// write artifact files into the working directory, so tests exercising them must own one.
func runIn(dir string, args ...string) (stdout, stderr string, exitCode int) {
	// The helper has no *testing.T; the subprocess is bounded by cmd.Run below.
	cmd := exec.CommandContext(context.Background(), binary, args...)
	cmd.Dir = dir
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
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
	work := t.TempDir()
	_, stderr, code := runIn(work, "run", "nonexistent.star")
	assertExit(t, 1, code)

	// A run that cannot start must not litter the working directory with artifacts named for the missing
	// script — this exact litter shipped once (nonexistent.graph.yaml in the package directory).
	entries, err := os.ReadDir(work)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("missing-script run created artifacts: %v", entries)
	}

	// The wrapped syscall text belongs to the OS -- "no such file or directory" on Unix, "The system cannot
	// find the file specified." on Windows -- so asserting on it tested the platform, not the CLI. These two
	// substrings are ours: the message the runner writes, and the path the caller passed.
	assertContains(t, stderr, "reading script")
	assertContains(t, stderr, "nonexistent.star")
}

func TestCLI_ScriptFirst(t *testing.T) {
	stdout, _, code := run("run", scriptPath, "--output", "summary=/dev/stdout", "--output", "receipt="+os.DevNull, "--output", "graph="+os.DevNull)
	assertExit(t, 0, code)
	assertValidSummary(t, stdout)
}

func TestCLI_ScriptMiddle(t *testing.T) {
	stdout, _, code := run("run", "--output", "summary=/dev/stdout", "--output", "receipt="+os.DevNull, scriptPath, "--output", "graph="+os.DevNull)
	assertExit(t, 0, code)
	assertValidSummary(t, stdout)
}

func TestCLI_ScriptLast(t *testing.T) {
	stdout, _, code := run("run", "--output", "summary=/dev/stdout", "--output", "receipt="+os.DevNull, "--output", "graph="+os.DevNull, scriptPath)
	assertExit(t, 0, code)
	assertValidSummary(t, stdout)
}

// --- Output routing ---

// TestCLI_DefaultsToArtifactFiles pins the ruled default routing (2026-08-20): results are files named for
// the script in the working directory; stdout carries none of the three payloads.
func TestCLI_DefaultsToArtifactFiles(t *testing.T) {
	work := t.TempDir()
	stdout, _, code := runIn(work, "run", scriptPath)
	assertExit(t, 0, code)
	assertNotContains(t, stdout, `"passed"`)
	assertNotContains(t, stdout, "Hello World!")
	assertNotContains(t, stdout, "version:")
	for _, artifact := range []string{"test_hello.summary.json", "test_hello.graph.yaml", "test_hello.receipt.yaml"} {
		if _, err := os.Stat(filepath.Join(work, artifact)); err != nil {
			t.Errorf("default artifact %s not written: %v", artifact, err)
		}
	}
}

func TestCLI_SummaryOnly(t *testing.T) {
	stdout, _, code := run("run", "--output", "summary=/dev/stdout", "--output", "graph="+os.DevNull, "--output", "receipt="+os.DevNull, scriptPath)
	assertExit(t, 0, code)
	assertValidSummary(t, stdout)
	assertNotContains(t, stdout, "Hello World!")
	assertNotContains(t, stdout, "version:")
}

func TestCLI_GraphOnly(t *testing.T) {
	stdout, _, code := run("run", "--output", "graph=/dev/stdout", "--output", "summary="+os.DevNull, "--output", "receipt="+os.DevNull, scriptPath)
	assertExit(t, 0, code)
	assertContains(t, stdout, "Hello World!")
	assertNotContains(t, stdout, `"passed"`)
	assertNotContains(t, stdout, "version:")
}

func TestCLI_ReceiptOnlyYAML(t *testing.T) {
	stdout, _, code := run("run", "--output", "receipt=/dev/stdout", "--output", "graph="+os.DevNull, "--output", "summary="+os.DevNull, scriptPath)
	assertExit(t, 0, code)
	assertNotContains(t, stdout, `"passed"`)
	assertValidYAML(t, stdout)
	assertContains(t, stdout, "shell.exec")
	assertContains(t, stdout, "version:")
}

func TestCLI_ReceiptOnlyJSON(t *testing.T) {
	stdout, _, code := run("run", "--output", "receipt=/dev/stdout", "--output", "graph="+os.DevNull, "--output", "summary="+os.DevNull, "--receipt-format=json", scriptPath)
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
	assertValidSummary(t, string(summary))

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
		"--output", "graph="+os.DevNull,
		"--output", "summary="+os.DevNull,
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
	stdout, _, code := run("run", "--dry-run", "--output", "summary=/dev/stdout", "--output", "graph="+os.DevNull, "--output", "receipt="+os.DevNull, scriptPath)
	assertExit(t, 0, code)
	assertValidSummary(t, stdout)
}

func TestCLI_Trace(t *testing.T) {
	stdout, _, code := run("run", "--trace", "--output", "summary=/dev/stdout", "--output", "graph="+os.DevNull, "--output", "receipt="+os.DevNull, scriptPath)
	assertExit(t, 0, code)
	assertContains(t, stdout, `"trace"`)
}

func TestCLI_Silent(t *testing.T) {
	_, stderr, code := run("--silent", "run", "--output", "summary="+os.DevNull, "--output", "graph="+os.DevNull, "--output", "receipt="+os.DevNull, scriptPath)
	assertExit(t, 0, code)
	if stderr != "" {
		t.Errorf("--silent should suppress stderr, got: %q", stderr)
	}
}

// --- Error cases ---

func TestCLI_InvalidOutputStream(t *testing.T) {
	_, _, code := run("run", "--output", "bogus="+os.DevNull, scriptPath)
	assertExit(t, 1, code)
}

func TestCLI_MalformedOutput(t *testing.T) {
	_, _, code := run("run", "--output", "nodestination", scriptPath)
	assertExit(t, 1, code)
}

func TestCLI_InvalidReceiptFormat(t *testing.T) {
	_, _, code := run("run", "--receipt-format=xml", "--output", "receipt="+os.DevNull, "--output", "graph="+os.DevNull, scriptPath)
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

	// xdg.ConfigPath, not a literal: the printed path is OS-native, so "devlore/config.yaml" only ever matched
	// on Unix. Asking xdg rather than cli.SharedConfigPath keeps the assertion independent of the accessor the
	// command itself calls -- a wrong helper should fail here, not agree with itself.
	assertContains(t, stdout, xdg.ConfigPath("devlore", "config.yaml"))
}

func TestCLI_SelfInstallHelp(t *testing.T) {
	stdout, _, code := run("self", "install", "--help")
	assertExit(t, 0, code)
	assertContains(t, stdout, "prefix")
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

func assertValidSummary(t *testing.T, s string) {
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
		if !result.Passed {
			t.Errorf("summary.passed = %v, want true", result.Passed)
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
