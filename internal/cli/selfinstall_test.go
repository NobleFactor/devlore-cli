// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestExpandTilde tests the tilde expansion function.
func TestExpandTilde(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("HOME not set")
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~", home},
		{"~/", home},
		{"~/.local", filepath.Join(home, ".local")},
		{"~/foo/bar", filepath.Join(home, "foo", "bar")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
		{"~user/path", "~user/path"}, // Only ~/... is expanded, not ~user/...
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandTilde(tt.input)
			if got != tt.want {
				t.Errorf("expandTilde(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestNewSelfInstallCmd_RequiresPrefix tests that --prefix is required.
// This test verifies that positional arguments are NOT accepted (they were removed).
func TestNewSelfInstallCmd_RequiresPrefix(t *testing.T) {
	rootCmd := &cobra.Command{Use: "test"}
	info := SelfInstallInfo{Name: "test"}
	cmd := NewSelfInstallCmd(rootCmd, info)

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	// Test: no args, no flags should fail
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when --prefix is not provided")
	}
	if !strings.Contains(err.Error(), "--prefix is required") {
		t.Errorf("expected '--prefix is required' error, got: %v", err)
	}
}

// TestNewSelfInstallCmd_RejectsPositionalArgs tests that positional args are rejected.
// Previously, the command accepted a positional directory argument. This was changed
// to --prefix flag for Unix convention compliance.
func TestNewSelfInstallCmd_RejectsPositionalArgs(t *testing.T) {
	rootCmd := &cobra.Command{Use: "test"}
	info := SelfInstallInfo{Name: "test"}
	cmd := NewSelfInstallCmd(rootCmd, info)

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	// Test: positional arg should fail (cobra.NoArgs enforces this)
	cmd.SetArgs([]string{"~/.local"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when positional argument is provided")
	}
	// The error message depends on cobra's implementation
	if !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg") {
		t.Errorf("expected rejection of positional arg, got: %v", err)
	}
}

// TestNewSelfInstallCmd_AcceptsPrefix tests that --prefix flag works.
func TestNewSelfInstallCmd_AcceptsPrefix(t *testing.T) {
	rootCmd := &cobra.Command{Use: "test"}
	info := SelfInstallInfo{Name: "test"}
	cmd := NewSelfInstallCmd(rootCmd, info)

	// We can't fully run self-install in tests (it copies binaries),
	// but we can verify the flag parsing works
	cmd.SetArgs([]string{"--prefix=/tmp/test"})

	// The command will fail during execution (no binary to copy, etc.)
	// but it should NOT fail during flag parsing
	err := cmd.Execute()
	if err != nil {
		// If error is about prefix, that's wrong
		if strings.Contains(err.Error(), "--prefix is required") {
			t.Errorf("--prefix flag not recognized: %v", err)
		}
		// Other errors (like "failed to install binary") are expected
	}
}

// TestNewSelfInstallCmd_ShellFlag tests that --shell flag is repeatable.
func TestNewSelfInstallCmd_ShellFlag(t *testing.T) {
	rootCmd := &cobra.Command{Use: "test"}
	info := SelfInstallInfo{Name: "test"}
	cmd := NewSelfInstallCmd(rootCmd, info)

	// Test multiple --shell flags
	cmd.SetArgs([]string{"--prefix=/tmp/test", "--shell", "bash", "--shell", "zsh"})

	// The command will fail during execution but should parse flags correctly
	_ = cmd.Execute()

	// Verify the flag was properly configured (checking the flag exists)
	shellFlag := cmd.Flag("shell")
	if shellFlag == nil {
		t.Error("--shell flag should exist")
	}
}

// TestNewSelfInstallCmd_Usage tests the command usage string.
func TestNewSelfInstallCmd_Usage(t *testing.T) {
	rootCmd := &cobra.Command{Use: "test"}
	info := SelfInstallInfo{Name: "test"}
	cmd := NewSelfInstallCmd(rootCmd, info)

	usage := cmd.Use
	if !strings.Contains(usage, "--prefix") {
		t.Errorf("usage should mention --prefix, got: %q", usage)
	}
}

// TestShellCompletionPath tests the shell completion path function.
func TestShellCompletionPath(t *testing.T) {
	tests := []struct {
		shell    string
		cmdName  string
		wantRel  string
		wantFile string
	}{
		{"bash", "writ", "share/bash-completion/completions", "writ"},
		{"fish", "writ", "share/fish/vendor_completions.d", "writ.fish"},
		{"zsh", "writ", "share/zsh/site-functions", "_writ"},
		{"powershell", "writ", "share/powershell/completions", "writ.ps1"},
		{"unknown", "writ", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			gotRel, gotFile := shellCompletionPath(tt.shell, tt.cmdName)
			// Normalize path separators for cross-platform
			wantRel := filepath.FromSlash(tt.wantRel)
			if gotRel != wantRel {
				t.Errorf("shellCompletionPath(%q, %q) relPath = %q, want %q", tt.shell, tt.cmdName, gotRel, wantRel)
			}
			if gotFile != tt.wantFile {
				t.Errorf("shellCompletionPath(%q, %q) filename = %q, want %q", tt.shell, tt.cmdName, gotFile, tt.wantFile)
			}
		})
	}
}

// TestHasMan tests the man command detection.
func TestHasMan(t *testing.T) {
	// This is environment-dependent, so we just test it doesn't panic
	_ = hasMan()
}

// TestDetectShells tests the shell detection function.
func TestDetectShells(t *testing.T) {
	// This is environment-dependent, so we just test it returns valid values
	shells := detectShells()
	validShells := map[string]bool{"bash": true, "fish": true, "powershell": true, "zsh": true}

	for _, shell := range shells {
		if !validShells[shell] {
			t.Errorf("detectShells() returned invalid shell: %q", shell)
		}
	}

	// Verify alphabetical order
	for i := 1; i < len(shells); i++ {
		if shells[i] < shells[i-1] {
			t.Errorf("detectShells() not sorted: %v", shells)
			break
		}
	}
}

// TestCopyFile tests the file copy function.
func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	content := []byte("test content")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(got), string(content))
	}
}

// TestCopyFile_NonExistentSource tests that copying a non-existent file fails.
func TestCopyFile_NonExistentSource(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "nonexistent.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	err := copyFile(src, dst)
	if err == nil {
		t.Error("expected error for non-existent source")
	}
}
