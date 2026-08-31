// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package devloretest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/internal/cli"
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

// runCmd executes the run subcommand and returns the result it wrote to stdout.
//
// Documents go to a store rooted in the test's own directory, so a run leaves nothing in the shared state
// home and one test cannot see another's definitions.
func runCmd(t *testing.T, script string, extraArgs ...string) (Result, error) {
	t.Helper()

	opts := cli.SinkOptions{Format: "json", Store: t.TempDir()}

	args := append(append([]string{}, extraArgs...), script)

	var stdout bytes.Buffer
	cmd := newRunCmd(&opts)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	cmd.SetOut(&stdout)
	err := cmd.Execute()

	var result Result
	if stdout.Len() > 0 {
		_ = json.Unmarshal(stdout.Bytes(), &result)
	}
	return result, err
}

func TestRunCmd_BasicExecution(t *testing.T) {
	script := writeScript(t, `graph = plan.assemble_definition([])
t.expect_unit_count(0)`)
	result, err := runCmd(t, script)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass, got failures: %v", result.Failures)
	}
	if result.UnitCount != 0 {
		t.Errorf("unit_count = %d, want 0", result.UnitCount)
	}
}

func TestRunCmd_DryRun(t *testing.T) {
	script := writeScript(t, `
graph = plan.assemble_definition([
    plan.shell.exec(command="echo hello"),
])
t.expect_unit_count(1)
`)
	result, err := runCmd(t, script, "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected pass, got failures: %v", result.Failures)
	}
	if result.UnitCount != 1 {
		t.Errorf("unit_count = %d, want 1", result.UnitCount)
	}
}

func TestRunCmd_Trace(t *testing.T) {
	script := writeScript(t, `graph = plan.assemble_definition([])
t.expect_unit_count(0)`)
	result, err := runCmd(t, script, "--trace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Trace) == 0 {
		t.Error("expected trace entries, got none")
	}
}

// TestRunCmd_ResultGoesToStdout is the law: the result is machine-readable data on stdout.
func TestRunCmd_ResultGoesToStdout(t *testing.T) {

	script := writeScript(t, `
graph = plan.assemble_definition([plan.shell.exec(command='echo hi')])
t.expect_unit_count(1)
t.run(graph)
`)

	opts := cli.SinkOptions{Format: "json", Store: t.TempDir()}

	var stdout bytes.Buffer
	cmd := newRunCmd(&opts)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{script})
	cmd.SetOut(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var result Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not the JSON result: %v\n%s", err, stdout.String())
	}
	if !result.Passed {
		t.Errorf("result.Passed = false, want true: %+v", result)
	}
}

// TestRunCmd_DocumentsGoToTheStore is row 10 of the #740 test plan.
//
// The definition and its trace are documents. They belong in the execution store, keyed by checksum, and
// nowhere else -- #738 wrote the definition to a file named for the script and called it a receipt.
func TestRunCmd_DocumentsGoToTheStore(t *testing.T) {

	script := writeScript(t, `
graph = plan.assemble_definition([plan.shell.exec(command='echo hi')])
t.run(graph)
`)

	store := t.TempDir()
	opts := cli.SinkOptions{Format: "json", Store: store}

	cmd := newRunCmd(&opts)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{script})
	cmd.SetOut(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	definitions, err := filepath.Glob(filepath.Join(store, "graphs", "*.yaml"))
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions in the store = %v (err %v), want exactly one", definitions, err)
	}

	traces, err := filepath.Glob(filepath.Join(store, "traces", "*", "2*.yaml"))
	if err != nil || len(traces) != 1 {
		t.Fatalf("traces in the store = %v (err %v), want exactly one", traces, err)
	}

	// The trace lives under its definition's checksum: that tie is what makes a trace resumable.
	checksum := strings.TrimSuffix(filepath.Base(definitions[0]), ".yaml")
	if got := filepath.Base(filepath.Dir(traces[0])); got != checksum {
		t.Errorf("trace filed under %q, want its definition's checksum %q", got, checksum)
	}
}

// TestRunCmd_NoneRendersNothing covers `-o none`: the result is not produced, not merely discarded.
func TestRunCmd_NoneRendersNothing(t *testing.T) {

	script := writeScript(t, `
graph = plan.assemble_definition([plan.shell.exec(command='echo hi')])
t.run(graph)
`)

	opts := cli.SinkOptions{Format: "none", Store: t.TempDir()}

	var stdout bytes.Buffer
	cmd := newRunCmd(&opts)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{script})
	cmd.SetOut(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout wrote %d bytes under -o none: %q", stdout.Len(), stdout.String())
	}
}

func TestRunCmd_MissingScript(t *testing.T) {
	_, err := runCmd(t, "/nonexistent/test.star")
	if err == nil {
		t.Fatal("expected error for missing script")
	}
}

// TestRunCmd_DefaultsToScriptNamedArtifacts pins the default routing (ruled 2026-08-20): each stream lands in
// an artifact file named for the script, in the working directory — results are files, narration is stderr,
// and stdout stays clean. An explicit --output overrides per stream, which every other test here exercises.
func TestRunCmd_LeavesNoArtifactsInTheWorkingDirectory(t *testing.T) {

	script := writeScript(t, `
graph = plan.assemble_definition([plan.shell.exec(command='echo hi')])
t.run(graph)
`)

	work := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	opts := cli.SinkOptions{Format: "json", Store: t.TempDir()}
	cmd := newRunCmd(&opts)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{script})
	cmd.SetOut(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	entries, err := os.ReadDir(work)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("the run littered the working directory: %v", entries)
	}
}
