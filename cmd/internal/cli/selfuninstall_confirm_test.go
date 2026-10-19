// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// region HELPER FUNCTIONS

// stdinHolding replaces the process's standard input with a pipe holding `answer`, already closed, so a read takes the
// answer and then meets end of input. The original is restored on cleanup.
//
// Parameters:
//   - `t`: the test harness.
//   - `answer`: what stdin holds; "" is end of input at once.
func stdinHolding(t *testing.T, answer string) {

	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := writer.WriteString(answer); err != nil {
		t.Fatalf("write the answer: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close the pipe's writer: %v", err)
	}

	previous := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() {
		os.Stdin = previous
		_ = reader.Close()
	})
}

// uninstallThroughRoot runs `selftest self uninstall <prefix>` with the flags given, on the shared root, and returns
// its error and everything it narrated.
//
// Parameters:
//   - `t`: the test harness.
//   - `prefix`: the prefix the tool was installed into.
//   - `flags`: the flags after the prefix.
//
// Returns:
//   - `string`: what it wrote to stdout and stderr.
//   - `error`: the command's error.
func uninstallThroughRoot(t *testing.T, prefix string, flags ...string) (string, error) {

	t.Helper()

	root := NewRootCmd(RootConfig{Name: "selftest", Short: "a probe"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"self", "uninstall", prefix}, flags...))

	previous := UI()
	t.Cleanup(func() { SetUI(previous) })

	err := root.Execute()

	return out.String(), err
}

// binaryRemains reports whether the installed binary is still in the prefix.
//
// Parameters:
//   - `prefix`: the installation prefix.
//
// Returns:
//   - `bool`: true when `bin/selftest` is on disk.
func binaryRemains(prefix string) bool {

	_, err := os.Stat(filepath.Join(prefix, "bin", executableName("selftest")))

	return err == nil
}

// endregion

// TestSelfUninstall_UnattendedTakesTheDefault is #1005 (ruled 2026-10-01, "--unattended takes the default and
// aborts"): the default is No, so nothing is asked, nothing is read, and nothing is removed, and it exits 0 as a No
// does. stdin holds "y": a command that read it would remove the install.
func TestSelfUninstall_UnattendedTakesTheDefault(t *testing.T) {

	prefix, _ := installIntoTempPrefix(t)
	stdinHolding(t, "y\n")

	said, err := uninstallThroughRoot(t, prefix, "--unattended")

	if err != nil {
		t.Fatalf("self uninstall --unattended: %v, want exit 0\n%s", err, said)
	}
	if !binaryRemains(prefix) {
		t.Errorf("self uninstall --unattended removed the install; it should take the default, No\n%s", said)
	}
	if _, err := os.Stat(filepath.Join(prefix, "share", "selftest", "manifest.json")); err != nil {
		t.Errorf("self uninstall --unattended removed the manifest: %v", err)
	}
}

// TestSelfUninstall_NoAnswerIsNo is #1005 as docker does it (ruled 2026-10-19): end of input is No, not an error;
// nothing is removed and the command exits 0.
func TestSelfUninstall_NoAnswerIsNo(t *testing.T) {

	prefix, _ := installIntoTempPrefix(t)
	stdinHolding(t, "")

	said, err := uninstallThroughRoot(t, prefix)

	if err != nil {
		t.Fatalf("self uninstall with no answer: %v, want exit 0\n%s", err, said)
	}
	if !binaryRemains(prefix) {
		t.Errorf("self uninstall with no answer removed the install\n%s", said)
	}
}

// TestSelfUninstall_AnswerOnAPipe holds the behavior docker's prompts keep: the answer is read from stdin whether or
// not it is a terminal, so `echo y | <tool> self uninstall` removes the install.
func TestSelfUninstall_AnswerOnAPipe(t *testing.T) {

	prefix, _ := installIntoTempPrefix(t)
	stdinHolding(t, "y\n")

	if said, err := uninstallThroughRoot(t, prefix); err != nil {
		t.Fatalf("self uninstall answered y: %v\n%s", err, said)
	}
	if binaryRemains(prefix) {
		t.Error("self uninstall answered y on a pipe left the install in place")
	}
}

// TestSelfUninstall_NoOnAPipe holds today's No: nothing is removed, and the command exits 0.
func TestSelfUninstall_NoOnAPipe(t *testing.T) {

	prefix, _ := installIntoTempPrefix(t)
	stdinHolding(t, "n\n")

	if said, err := uninstallThroughRoot(t, prefix); err != nil {
		t.Fatalf("self uninstall answered n: %v, want exit 0\n%s", err, said)
	}
	if !binaryRemains(prefix) {
		t.Error("self uninstall answered n removed the install")
	}
}

// TestSelfUninstall_ForceWinsOverUnattended is #1005's "--force still removes without asking": --force is the
// answer, and --unattended supplies one only when none is given. stdin is at end of input, so a prompt would say No.
func TestSelfUninstall_ForceWinsOverUnattended(t *testing.T) {

	prefix, _ := installIntoTempPrefix(t)
	stdinHolding(t, "")

	if said, err := uninstallThroughRoot(t, prefix, "--force", "--unattended"); err != nil {
		t.Fatalf("self uninstall --force --unattended: %v\n%s", err, said)
	}
	if binaryRemains(prefix) {
		t.Error("self uninstall --force --unattended left the install in place")
	}
}

// TestRoot_InteractiveIsGone is #1005's Requirement 7 (ruled 2026-10-19, "Delete it in lane 28's pull request."): nothing read
// `--interactive`, so the shared root no longer declares it, and giving it is a usage error, exit 64.
func TestRoot_InteractiveIsGone(t *testing.T) {

	root := NewRootCmd(RootConfig{Name: "probe", Short: "a probe"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version", "--interactive"})

	previous := UI()
	t.Cleanup(func() { SetUI(previous) })

	err := root.Execute()

	if err == nil || !strings.Contains(err.Error(), "unknown flag: --interactive") {
		t.Fatalf("--interactive: got %v, want an unknown-flag error", err)
	}
	if code := ExitCode(err); code != ExitUsage {
		t.Errorf("--interactive exits %d, want %d", code, ExitUsage)
	}
}
