// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package process

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

// narratingChildEnv, set in a test binary's environment, turns [TestRunner_NarratingChild] into a child that narrates
// the way a program on the shared root does: its story on stderr, the last line without its newline. Its value is the
// status the child exits with.
const narratingChildEnv = "DEVLORE_TEST_NARRATING_CHILD"

// narratingChild returns a command that runs this test binary as a child that narrates on stderr.
//
// Parameters:
//   - `t`: the test harness.
//   - `exitStatus`: the status the child exits with.
//
// Returns:
//   - `*exec.Cmd`: the command, not yet started.
func narratingChild(t *testing.T, exitStatus int) *exec.Cmd {

	t.Helper()

	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	cmd := exec.CommandContext(context.Background(), self, "-test.run=^TestRunner_NarratingChild$")
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%d", narratingChildEnv, exitStatus))

	return cmd
}

// capturingRunner returns a runner whose narration is captured, and the buffer that captures it.
//
// Returns:
//   - `*Runner`: the runner, not in dry-run, with no context to cancel it.
//   - `*bytes.Buffer`: what the runner narrates, one decorated line per message.
func capturingRunner() (*Runner, *bytes.Buffer) {

	captured, buffer := sink.Capture()

	return NewRunner(context.Background(), false, nil, status.NewNarrator("probe", captured)), buffer
}

// --- the narrating child ---

// TestRunner_NarratingChild is the child [narratingChild] runs, not a test of its own.
//
// It returns at once unless [narratingChildEnv] is set. When it is set, it writes one line of result to stdout and
// two lines of story to stderr, the second without its newline, and exits with the status the variable names, before
// the test framework can add its own output.
func TestRunner_NarratingChild(t *testing.T) {

	exitStatus, err := strconv.Atoi(os.Getenv(narratingChildEnv))
	if err != nil {
		return
	}

	fmt.Fprintln(os.Stdout, "the result")
	fmt.Fprint(os.Stderr, "[child] [+] Installed child\n[child] [+] the last line, unterminated")
	os.Exit(exitStatus)
}

// --- Run ---

// TestRunner_RunRelaysStderrAsWarnings pins what Run has always done: a child's stderr reads as warnings.
func TestRunner_RunRelaysStderrAsWarnings(t *testing.T) {

	runner, narration := capturingRunner()

	if err := runner.Run(narratingChild(t, 0)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(narration.String(), "[probe] [△] [child] [+] Installed child") {
		t.Errorf("the child's stderr was not relayed as a warning:\n%s", narration)
	}
}

// TestRunner_RunRelaysTheLastLineOfEachStream pins the flush: a last line without its newline still arrives.
//
// Run flushed stdout and not stderr, so a child whose last word on stderr had no newline lost it.
func TestRunner_RunRelaysTheLastLineOfEachStream(t *testing.T) {

	runner, narration := capturingRunner()

	if err := runner.Run(narratingChild(t, 0)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, line := range []string{"[probe] [+] the result", "[child] [+] the last line, unterminated"} {
		if !strings.Contains(narration.String(), line) {
			t.Errorf("the narration lacks %q:\n%s", line, narration)
		}
	}
}

// --- RunNarrating ---

// TestRunner_RunNarratingRelaysStderrAsNotes is #947's Requirement 7: a child's narration is relayed as notes.
//
// Every program on the shared root narrates on stderr. `self upgrade` runs each program's own `self install` as a
// child, and relayed as warnings, a successful install would read as a page of them.
func TestRunner_RunNarratingRelaysStderrAsNotes(t *testing.T) {

	runner, narration := capturingRunner()

	if err := runner.RunNarrating(narratingChild(t, 0)); err != nil {
		t.Fatalf("RunNarrating: %v", err)
	}

	for _, line := range []string{
		"[probe] [+] the result",
		"[probe] [+] [child] [+] Installed child",
		"[probe] [+] [child] [+] the last line, unterminated",
	} {
		if !strings.Contains(narration.String(), line) {
			t.Errorf("the narration lacks %q:\n%s", line, narration)
		}
	}
	if strings.Contains(narration.String(), "[△]") {
		t.Errorf("the child's narration was relayed as a warning:\n%s", narration)
	}
}

// TestRunner_RunNarratingReportsTheExitStatus pins that the exit status, not stderr, says whether a child failed.
func TestRunner_RunNarratingReportsTheExitStatus(t *testing.T) {

	runner, narration := capturingRunner()

	err := runner.RunNarrating(narratingChild(t, 3))
	if err == nil {
		t.Fatal("RunNarrating reported success for a child that exited 3")
	}
	if !strings.Contains(err.Error(), "exited with code 3") {
		t.Errorf("error = %v; want the exit code", err)
	}
	if strings.Contains(narration.String(), "[△]") {
		t.Errorf("a failing child's narration was relayed as a warning:\n%s", narration)
	}
}

// TestRunner_RunNarratingInDryRunStartsNothing pins that dry-run narrates the command and starts no child.
func TestRunner_RunNarratingInDryRunStartsNothing(t *testing.T) {

	captured, narration := sink.Capture()
	runner := NewRunner(context.Background(), true, nil, status.NewNarrator("probe", captured))

	if err := runner.RunNarrating(narratingChild(t, 0)); err != nil {
		t.Fatalf("RunNarrating: %v", err)
	}

	if !strings.Contains(narration.String(), "[dry-run] $ ") {
		t.Errorf("dry-run did not narrate the command:\n%s", narration)
	}
	if strings.Contains(narration.String(), "Installed child") {
		t.Errorf("dry-run started the child:\n%s", narration)
	}
}
