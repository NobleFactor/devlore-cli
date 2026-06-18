// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// =============================================================================
// Test Helpers
// =============================================================================

func setupTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "star",
		Short: "Test command",
	}
}

func setupTestInfo() SelfInstallInfo {
	return SelfInstallInfo{
		Name: "star",
		ManHeader: ManHeader{
			Title:   "STAR",
			Section: "1",
			Source:  "Test",
			Manual:  "Test Manual",
		},
	}
}

// =============================================================================
// Unit Tests
// =============================================================================

func TestShellCompletionPath(t *testing.T) {
	tests := []struct {
		shell    string
		cmdName  string
		wantRel  string
		wantFile string
	}{
		{"bash", "star", "share/bash-completion/completions", "star"},
		{"fish", "star", "share/fish/vendor_completions.d", "star.fish"},
		{"zsh", "star", "share/zsh/site-functions", "_star"},
		{"powershell", "star", "share/powershell/completions", "star.ps1"},
		{"unknown", "star", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			gotRel, gotFile := shellCompletionPath(tt.shell, tt.cmdName)
			if gotRel != tt.wantRel {
				t.Errorf("shellCompletionPath(%q, %q) relPath = %q, want %q", tt.shell, tt.cmdName, gotRel, tt.wantRel)
			}
			if gotFile != tt.wantFile {
				t.Errorf("shellCompletionPath(%q, %q) filename = %q, want %q", tt.shell, tt.cmdName, gotFile, tt.wantFile)
			}
		})
	}
}

func TestFindExtensionsDir_NotFound(t *testing.T) {
	// Change to temp dir with no extensions
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	got := findExtensionsDir()
	if got != "" {
		t.Errorf("findExtensionsDir() = %q, want empty string", got)
	}
}

func TestFindExtensionsDir_Found(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Create star/extensions directory
	extDir := filepath.Join(tmpDir, "star", "extensions")
	if err := os.MkdirAll(extDir, 0755); err != nil {
		t.Fatal(err)
	}

	got := findExtensionsDir()
	want := filepath.Join("star", "extensions")
	if got != want {
		t.Errorf("findExtensionsDir() = %q, want %q", got, want)
	}
}

func TestCopyDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directory structure
	srcDir := filepath.Join(tmpDir, "src")
	subDir := filepath.Join(srcDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create files
	files := map[string]string{
		filepath.Join(srcDir, "file1.txt"):      "content1",
		filepath.Join(srcDir, "file2.txt"):      "content2",
		filepath.Join(subDir, "nested.txt"):     "nested content",
		filepath.Join(subDir, "extension.yaml"): "extension: test",
	}

	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Copy directory
	dstDir := filepath.Join(tmpDir, "dst")
	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir() error = %v", err)
	}

	// Verify all files were copied
	for srcPath, wantContent := range files {
		relPath, _ := filepath.Rel(srcDir, srcPath)
		dstPath := filepath.Join(dstDir, relPath)

		gotContent, err := os.ReadFile(dstPath)
		if err != nil {
			t.Errorf("failed to read copied file %s: %v", dstPath, err)
			continue
		}

		if string(gotContent) != wantContent {
			t.Errorf("copied file %s content = %q, want %q", relPath, string(gotContent), wantContent)
		}
	}
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	content := "test content"
	if err := os.WriteFile(src, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile() error = %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read dest file: %v", err)
	}

	if string(got) != content {
		t.Errorf("copyFile() content = %q, want %q", string(got), content)
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestSelfInstall_Integration(t *testing.T) {
	// Save original directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Create temp directory for installation target
	installDir := t.TempDir()

	// Create temp directory for project with extensions
	projectDir := t.TempDir()

	// Create star/extensions structure
	extDir := filepath.Join(projectDir, "star", "extensions", "com.test.Extension")
	cmdDir := filepath.Join(extDir, "commands")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create extension files
	extYaml := `extension: com.test.Extension
commands:
  - name: test.cmd
    help: "Test command"
    implementation: commands/test.star
`
	if err := os.WriteFile(filepath.Join(extDir, "extension.yaml"), []byte(extYaml), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cmdDir, "test.star"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to project directory
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	// Create test command
	rootCmd := setupTestCmd()
	info := setupTestInfo()

	// Run self-install
	err = runSelfInstall(rootCmd, installDir, info, installFlags{
		Shells: []string{}, // Skip shell detection in tests
	})
	if err != nil {
		t.Fatalf("runSelfInstall() error = %v", err)
	}

	// Verify binary directory was created
	binDir := filepath.Join(installDir, "bin")
	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		t.Error("bin directory was not created")
	}

	// Verify extensions were copied
	installedExtDir := filepath.Join(installDir, "share", "star", "extensions", "com.test.Extension")
	if _, err := os.Stat(installedExtDir); os.IsNotExist(err) {
		t.Error("extensions directory was not copied")
	}

	// Verify extension.yaml was copied
	installedExtYaml := filepath.Join(installedExtDir, "extension.yaml")
	if _, err := os.Stat(installedExtYaml); os.IsNotExist(err) {
		t.Error("extension.yaml was not copied")
	}

	// Verify commands subdirectory was copied
	installedCmdDir := filepath.Join(installedExtDir, "commands")
	if _, err := os.Stat(installedCmdDir); os.IsNotExist(err) {
		t.Error("commands directory was not copied")
	}

	// Verify star file was copied
	installedStar := filepath.Join(installedCmdDir, "test.star")
	if _, err := os.Stat(installedStar); os.IsNotExist(err) {
		t.Error("test.star was not copied")
	}
}

func TestSelfInstall_NoExtensions(t *testing.T) {
	origDir, _ := os.Getwd()

	// Create temp directories
	installDir := t.TempDir()
	projectDir := t.TempDir()

	// Change to project directory (no star/extensions)
	os.Chdir(projectDir)
	defer os.Chdir(origDir)

	rootCmd := setupTestCmd()
	info := setupTestInfo()

	// Should succeed even without extensions
	err := runSelfInstall(rootCmd, installDir, info, installFlags{
		Shells: []string{},
	})
	if err != nil {
		t.Fatalf("runSelfInstall() error = %v", err)
	}

	// Extensions directory should not exist
	extDir := filepath.Join(installDir, "share", "star", "extensions")
	if _, err := os.Stat(extDir); err == nil {
		t.Error("extensions directory should not exist when no source extensions")
	}
}

func TestSelfInstall_DefaultPath(t *testing.T) {
	cmd := setupTestCmd()
	info := setupTestInfo()

	installCmd := newInstallCmd(cmd, info)

	// Verify command accepts 0 arguments (uses default)
	if installCmd.Args == nil {
		t.Error("Args validator should be set")
	}

	// Check that Use shows optional argument
	if installCmd.Use != "install [root-directory]" {
		t.Errorf("Use = %q, want %q", installCmd.Use, "install [root-directory]")
	}
}

func TestInstallExtensionsDir(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	installDir := t.TempDir()

	// Create source extensions
	srcExtDir := filepath.Join(tmpDir, "star", "extensions", "com.test.Ext")
	if err := os.MkdirAll(srcExtDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcExtDir, "extension.yaml"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	installedPath, err := installExtensionsDir(installDir, "star")
	if err != nil {
		t.Fatalf("installExtensionsDir() error = %v", err)
	}

	wantPath := filepath.Join(installDir, "share", "star", "extensions")
	if installedPath != wantPath {
		t.Errorf("installExtensionsDir() = %q, want %q", installedPath, wantPath)
	}

	// Verify extension was copied
	copiedExt := filepath.Join(wantPath, "com.test.Ext", "extension.yaml")
	if _, err := os.Stat(copiedExt); os.IsNotExist(err) {
		t.Error("extension.yaml was not copied")
	}
}
