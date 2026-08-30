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
	stdout, _, code := run("run", scriptPath, "-o", "json")
	assertExit(t, 0, code)
	assertValidSummary(t, stdout)
}

func TestCLI_ScriptLast(t *testing.T) {
	stdout, _, code := run("run", "-o", "json", scriptPath)
	assertExit(t, 0, code)
	assertValidSummary(t, stdout)
}

// TestCLI_ResultToStdoutStoreForDocuments is the law, end to end.
//
// The result is JSON on stdout, narration is on stderr, documents are in the store, and the working
// directory is left alone. This replaces the 2026-08-20 routing it used to pin, which sent all three
// payloads to files named for the script and kept stdout empty.
func TestCLI_ResultToStdoutStoreForDocuments(t *testing.T) {
	work, store := t.TempDir(), t.TempDir()

	stdout, stderr, code := runIn(work, "run", "--store", store, scriptPath)
	assertExit(t, 0, code)

	assertValidSummary(t, stdout)
	assertNotContains(t, stdout, "[devlore-test]")

	// Narration went to stderr, where it belongs.
	assertContains(t, stderr, "[devlore-test]")

	// Documents went to the store.
	definitions, err := filepath.Glob(filepath.Join(store, "graphs", "*.yaml"))
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions = %v (err %v), want one", definitions, err)
	}
	traces, err := filepath.Glob(filepath.Join(store, "traces", "*", "2*.yaml"))
	if err != nil || len(traces) != 1 {
		t.Fatalf("traces = %v (err %v), want one", traces, err)
	}

	// The working directory is untouched.
	entries, err := os.ReadDir(work)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the run littered the working directory: %v", entries)
	}
}

// TestCLI_StoredDefinitionIsTheDocument asserts artifact CONTENT, which #738 says existence-only checks
// never did -- a 0-byte file satisfied the old assertion.
func TestCLI_StoredDefinitionIsTheDocument(t *testing.T) {
	store := t.TempDir()

	_, _, code := runIn(t.TempDir(), "run", "--store", store, scriptPath)
	assertExit(t, 0, code)

	definitions, err := filepath.Glob(filepath.Join(store, "graphs", "*.yaml"))
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions = %v (err %v), want one", definitions, err)
	}

	data, err := os.ReadFile(definitions[0])
	if err != nil {
		t.Fatalf("reading the definition: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("the stored definition is empty")
	}

	body := string(data)
	assertContains(t, body, "shell.exec")
	assertContains(t, body, "checksum:")
}

// TestCLI_YAMLRendering covers -o yaml on the result stream.
func TestCLI_YAMLRendering(t *testing.T) {
	stdout, _, code := runIn(t.TempDir(), "run", "--store", t.TempDir(), "-o", "yaml", scriptPath)
	assertExit(t, 0, code)
	assertValidYAML(t, stdout)
	assertContains(t, stdout, "passed:")
}

// TestCLI_NoneRendersNothing covers -o none: no result is produced, and the exit code still reports.
func TestCLI_NoneRendersNothing(t *testing.T) {
	stdout, _, code := runIn(t.TempDir(), "run", "--store", t.TempDir(), "-o", "none", scriptPath)
	assertExit(t, 0, code)
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout = %q under -o none, want empty", stdout)
	}
}

func TestCLI_DryRun(t *testing.T) {
	stdout, _, code := runIn(t.TempDir(), "run", "--dry-run", "--store", t.TempDir(), scriptPath)
	assertExit(t, 0, code)
	assertValidSummary(t, stdout)
}

func TestCLI_Trace(t *testing.T) {
	stdout, _, code := runIn(t.TempDir(), "run", "--trace", "--store", t.TempDir(), scriptPath)
	assertExit(t, 0, code)
	assertContains(t, stdout, `"trace"`)
}

func TestCLI_Silent(t *testing.T) {
	_, stderr, code := runIn(t.TempDir(), "--silent", "run", "-o", "none", "--store", t.TempDir(), scriptPath)
	assertExit(t, 0, code)
	if stderr != "" {
		t.Errorf("--silent should suppress stderr, got: %q", stderr)
	}
}

// --- Error cases ---

func TestCLI_UnknownRendering(t *testing.T) {
	_, stderr, code := runIn(t.TempDir(), "run", "-o", "nope", scriptPath)
	if code == 0 {
		t.Error("an unknown rendering exited 0, want non-zero")
	}
	assertContains(t, stderr, "unknown formatter")
}

// TestCLI_UncreatableStore covers the flag that actually takes a path.
//
// This asserted a bad --output destination until --output became a rendering. Left as it was, it passed for
// the wrong reason: "graph=/no/such/dir/out.txt" parses as an unknown format named "graph", so it exited
// non-zero without ever reaching a filesystem.
//
// The store root goes under a regular file, which MkdirAll cannot descend through on any platform. An
// unwritable directory is not portable and an absent one is not either: Windows CI runs as administrator,
// where "/no/such/dir/store" resolves to C:\no\such\dir\store and is simply created.
func TestCLI_UncreatableStore(t *testing.T) {
	directory := t.TempDir()
	blocker := filepath.Join(directory, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatalf("writing the blocker file: %v", err)
	}

	_, stderr, code := runIn(directory, "run", "--store", filepath.Join(blocker, "store"), scriptPath)
	if code == 0 {
		t.Error("a store under a regular file exited 0, want non-zero")
	}
	assertContains(t, stderr, "store")
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

	// The result is the whole of stdout, not a line within it: the json rendering is indented, so a
	// line-oriented parse sees only "{". Narration never lands here -- it is on stderr.
	var result struct {
		Passed bool `json:"passed"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &result); err != nil {
		t.Errorf("stdout is not the JSON result: %v\nstdout was:\n%s", err, s)
		return
	}
	if !result.Passed {
		t.Errorf("result.passed = %v, want true", result.Passed)
	}
}

func assertValidYAML(t *testing.T, s string) {
	t.Helper()
	var v any
	if err := yaml.Unmarshal([]byte(s), &v); err != nil {
		t.Errorf("invalid YAML: %v", err)
	}
}
