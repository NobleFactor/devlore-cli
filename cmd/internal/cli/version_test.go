// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// --- AddVersionFlag ---

// TestAddVersionFlag_AnswersInOneLine pins the `--version` surface: docker's shape, one line, and nothing
// from the detail block.
//
// The one-line property is the whole point of the flag's existence beside the `version` command, so it is
// asserted directly rather than inferred from the text matching.
func TestAddVersionFlag_AnswersInOneLine(t *testing.T) {

	rootCmd := &cobra.Command{Use: "writ"}
	AddVersionFlag(rootCmd, VersionInfo{Version: "1.2.3", Commit: "abc1234", BuildDate: "2026-03-17T00:00:00Z"})

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"--version"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if want := "writ version 1.2.3, build abc1234\n"; out.String() != want {
		t.Errorf("--version = %q, want %q", out.String(), want)
	}

	if lines := strings.Count(strings.TrimSuffix(out.String(), "\n"), "\n"); lines != 0 {
		t.Errorf("--version printed %d extra lines; the flag answers in one", lines)
	}

	// The build date belongs to `version`, not to `--version` — docker's split is the reason both exist.
	if strings.Contains(out.String(), "2026-03-17") {
		t.Error("--version reported the build date; that detail belongs to the version command")
	}
}

// TestAddVersionFlag_IsNotBoundToShorthandV pins the deliberate omission: `-v` is verbose output across this
// repository's commands, and cobra would happily take it for --version.
func TestAddVersionFlag_IsNotBoundToShorthandV(t *testing.T) {

	rootCmd := &cobra.Command{Use: "writ"}
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	AddVersionFlag(rootCmd, VersionInfo{Version: "1.2.3", Commit: "abc1234"})

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetArgs([]string{"-v"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if strings.Contains(out.String(), "version") {
		t.Errorf("-v printed the version %q; it is the verbose flag", out.String())
	}
}

// captureStdout runs fn and returns what it wrote to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	orig := os.Stdout
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = orig

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	r.Close()

	return string(buf[:n])
}

func TestNewVersionCmd_FullOutput(t *testing.T) {
	info := VersionInfo{
		Version:   "1.2.3",
		Commit:    "abc1234",
		BuildDate: "2026-03-17T00:00:00Z",
	}

	cmd := NewVersionCmd(info)
	cmd.SetArgs(nil)

	output := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	for _, want := range []string{"1.2.3", "abc1234", "2026-03-17T00:00:00Z", "Go version:", "OS/Arch:"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q, got:\n%s", want, output)
		}
	}
}

func TestNewVersionCmd_ShortFlag(t *testing.T) {
	info := VersionInfo{
		Version:   "1.2.3",
		Commit:    "abc1234",
		BuildDate: "2026-03-17T00:00:00Z",
	}

	cmd := NewVersionCmd(info)
	cmd.SetArgs([]string{"--short"})

	output := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	trimmed := strings.TrimSpace(output)
	if trimmed != "1.2.3" {
		t.Errorf("short output = %q, want '1.2.3'", trimmed)
	}
}

func TestNewVersionCmd_DefaultsAreVisible(t *testing.T) {
	info := VersionInfo{
		Version:   "dev",
		Commit:    "none",
		BuildDate: "unknown",
	}

	cmd := NewVersionCmd(info)
	cmd.SetArgs(nil)

	output := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	for _, want := range []string{"dev", "none", "unknown"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing default %q, got:\n%s", want, output)
		}
	}
}
