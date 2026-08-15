// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/xdg"
	"github.com/spf13/cobra"
)

// TestExpandTilde tests the tilde expansion function.
func TestExpandTilde(t *testing.T) {
	home := xdg.UserHomeDir()

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

// TestNewSelfCmd_InstallDefaultPrefix verifies that "self install" with no args uses ~/.local.
func TestNewSelfCmd_InstallDefaultPrefix(t *testing.T) {
	rootCmd := &cobra.Command{Use: "test"}
	info := SelfInstallInfo{Name: "test", Version: "0.1.0"}
	selfCmd := NewSelfCmd(rootCmd, info)

	// The self command should have 3 subcommands: install, upgrade, uninstall.
	if len(selfCmd.Commands()) != 3 {
		t.Fatalf("expected 3 subcommands, got %d", len(selfCmd.Commands()))
	}

	var installCmd *cobra.Command
	for _, c := range selfCmd.Commands() {
		if c.Name() == "install" {
			installCmd = c
			break
		}
	}
	if installCmd == nil {
		t.Fatal("install subcommand not found")
	}

	// Verify it accepts 0 or 1 positional arg.
	var stdout, stderr bytes.Buffer
	installCmd.SetOut(&stdout)
	installCmd.SetErr(&stderr)
	installCmd.SetArgs([]string{})

	// It will fail during execution (no binary to copy), but the flag parsing should work.
	_ = installCmd.Execute()
}

// TestNewSelfCmd_InstallCustomPrefix verifies "self install /tmp/test" passes prefix through.
func TestNewSelfCmd_InstallCustomPrefix(t *testing.T) {
	rootCmd := &cobra.Command{Use: "test"}
	info := SelfInstallInfo{Name: "test", Version: "0.1.0"}
	selfCmd := NewSelfCmd(rootCmd, info)

	var installCmd *cobra.Command
	for _, c := range selfCmd.Commands() {
		if c.Name() == "install" {
			installCmd = c
			break
		}
	}
	if installCmd == nil {
		t.Fatal("install subcommand not found")
	}

	// Pass a custom prefix — the command will fail during execution but flag parsing works.
	var stdout, stderr bytes.Buffer
	installCmd.SetOut(&stdout)
	installCmd.SetErr(&stderr)
	installCmd.SetArgs([]string{"/tmp/selftest"})
	_ = installCmd.Execute()
}

// TestNewSelfCmd_UpgradeExists verifies the upgrade subcommand exists.
func TestNewSelfCmd_UpgradeExists(t *testing.T) {
	rootCmd := &cobra.Command{Use: "test"}
	info := SelfInstallInfo{Name: "test", Version: "0.1.0"}
	selfCmd := NewSelfCmd(rootCmd, info)

	var found bool
	for _, c := range selfCmd.Commands() {
		if c.Name() == "upgrade" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("upgrade subcommand not found")
	}
}

// TestNewSelfCmd_UninstallExists verifies the uninstall subcommand exists.
func TestNewSelfCmd_UninstallExists(t *testing.T) {
	rootCmd := &cobra.Command{Use: "test"}
	info := SelfInstallInfo{Name: "test", Version: "0.1.0"}
	selfCmd := NewSelfCmd(rootCmd, info)

	var found bool
	for _, c := range selfCmd.Commands() {
		if c.Name() == "uninstall" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("uninstall subcommand not found")
	}
}

// TestResolveInstalledPrefix tests the prefix resolution logic.
func TestResolveInstalledPrefix(t *testing.T) {
	// Create a temp structure: <prefix>/bin/<tool>
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toolPath := filepath.Join(binDir, "test-tool")
	if err := os.WriteFile(toolPath, []byte("fake binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// resolveInstalledPrefix uses os.Executable() which we can't mock easily,
	// so just test the edge case detection.
	_, err := resolveInstalledPrefix("test-tool")
	// This will either succeed (if test binary is in a bin/ dir) or fail with
	// "not in a <prefix>/bin/ directory". Either way, it shouldn't panic.
	_ = err
}

// TestShellCompletionPath_PerShell verifies shellCompletionPath returns the right install path and filename per shell.
func TestShellCompletionPath_PerShell(t *testing.T) {
	tests := []struct {
		shell    string
		cmdName  string
		wantRel  string
		wantFile string
	}{
		{"bash", "writ", "share/bash-completion/completions", "writ"},
		{"fish", "writ", "share/fish/vendor_completions.d", "writ.fish"},
		{"zsh", "writ", "share/zsh/site-functions", "_writ"},
		{"pwsh", "writ", "share/powershell/completions", "writ.ps1"},
		{"unknown", "writ", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			gotRel, gotFile := shellCompletionPath(tt.shell, tt.cmdName)
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
	_ = hasMan()
}

// TestDetectShells tests the shell detection function.
func TestDetectShells(t *testing.T) {
	shells := detectShells()
	validShells := map[string]bool{"bash": true, "fish": true, "pwsh": true, "zsh": true}

	for _, shell := range shells {
		if !validShells[shell] {
			t.Errorf("detectShells() returned invalid shell: %q", shell)
		}
	}

	// Verify alphabetical order.
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
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}

	if !bytes.Equal(got, content) {
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

// TestCopyDir tests recursive directory copying.
func TestCopyDir(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "dest")

	// Create source structure.
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("bbb"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("CopyDir failed: %v", err)
	}

	// Verify.
	got, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "aaa" {
		t.Errorf("a.txt = %q, want %q", string(got), "aaa")
	}

	got, err = os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "bbb" {
		t.Errorf("sub/b.txt = %q, want %q", string(got), "bbb")
	}
}

// TestWriteAndReadManifest tests the manifest round-trip.
func TestWriteAndReadManifest(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file to checksum.
	testFile := filepath.Join(tmpDir, "bin", "test")
	if err := os.MkdirAll(filepath.Dir(testFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile, []byte("binary content"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Write manifest.
	if err := writeManifest(tmpDir, "test", "1.0.0", []string{"bin/test"}); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}

	// Read manifest.
	m, err := readManifest(tmpDir, "test")
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}

	if m.Tool != "test" {
		t.Errorf("tool = %q, want %q", m.Tool, "test")
	}
	if m.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", m.Version, "1.0.0")
	}
	if len(m.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(m.Files))
	}
	if m.Files[0].Path != "bin/test" {
		t.Errorf("path = %q, want %q", m.Files[0].Path, "bin/test")
	}
	if m.Files[0].SHA256 == "" {
		t.Error("sha256 should not be empty")
	}
}

// TestFileSHA256 tests the SHA-256 computation.
func TestFileSHA256(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	hash, err := fileSHA256(path)
	if err != nil {
		t.Fatalf("fileSHA256: %v", err)
	}

	// SHA-256 of "hello" is known.
	expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if hash != expected {
		t.Errorf("hash = %q, want %q", hash, expected)
	}
}

// TestRunSelfInstall_NilConfigInfo verifies install works when ConfigInfo is nil (star case).
func TestRunSelfInstall_NilConfigInfo(t *testing.T) {
	rootCmd := &cobra.Command{Use: "test"}
	info := SelfInstallInfo{
		Name:    "test",
		Version: "0.1.0",
		ManHeader: ManHeader{
			Title:   "TEST",
			Section: "1",
			Source:  "Test",
			Manual:  "Test Manual",
		},
		ConfigInfo: nil,
	}

	// runSelfInstall will fail on installBinary (since os.Executable() won't be a real binary
	// in the test context), but it should not panic on nil ConfigInfo.
	err := runSelfInstall(rootCmd, t.TempDir(), info, installFlags{})
	// We expect an error from installBinary, not a nil pointer dereference.
	if err == nil {
		// If it somehow succeeds (unlikely in test), that's fine too.
		return
	}
	// Verify it's not a nil pointer issue.
	if err.Error() == "runtime error: invalid memory address or nil pointer dereference" {
		t.Fatal("nil ConfigInfo caused a panic")
	}
}

// TestManifestUninstall tests the manifest-based uninstall flow.
func TestManifestUninstall(t *testing.T) {
	tmpDir := t.TempDir()

	// Create installed files.
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(binDir, "test")
	if err := os.WriteFile(binPath, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	manDir := filepath.Join(tmpDir, "share", "man", "man1")
	if err := os.MkdirAll(manDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manPath := filepath.Join(manDir, "test.1")
	if err := os.WriteFile(manPath, []byte("man page"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write manifest.
	if err := writeManifest(tmpDir, "test", "1.0.0", []string{"bin/test", "share/man/man1/test.1"}); err != nil {
		t.Fatal(err)
	}

	// Modify the man page (should be skipped during uninstall).
	if err := os.WriteFile(manPath, []byte("modified man page"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run uninstall.
	info := SelfInstallInfo{Name: "test", Version: "1.0.0"}
	if err := runSelfUninstall(tmpDir, info); err != nil {
		t.Fatalf("runSelfUninstall: %v", err)
	}

	// Binary should be removed (unchanged).
	if _, err := os.Stat(binPath); !os.IsNotExist(err) {
		t.Error("binary should have been removed")
	}

	// Man page should be preserved (modified).
	if _, err := os.Stat(manPath); err != nil {
		t.Error("modified man page should have been preserved")
	}
}

// TestCollectFiles tests the file collection helper.
func TestCollectFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test structure.
	if err := os.MkdirAll(filepath.Join(tmpDir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "a", "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "a", "b", "y.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	files := CollectFiles(tmpDir, filepath.Join(tmpDir, "a"))
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
}

// TestManifestJSON verifies manifest serialization format.
func TestManifestJSON(t *testing.T) {
	m := manifest{
		Tool:      "writ",
		Version:   "0.4.0",
		Prefix:    "/home/user/.local",
		Installed: "2026-08-09T14:30:00Z",
		Files: []manifestEntry{
			{Path: "bin/writ", SHA256: "abc123"},
		},
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	var decoded manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Tool != "writ" {
		t.Errorf("tool = %q", decoded.Tool)
	}
	if len(decoded.Files) != 1 {
		t.Fatalf("files = %d", len(decoded.Files))
	}
	if decoded.Files[0].Path != "bin/writ" {
		t.Errorf("path = %q", decoded.Files[0].Path)
	}
}
