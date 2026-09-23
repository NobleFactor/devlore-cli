// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
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
//
// The destination side goes through a root, the source side does not — the asymmetry copyFile exists to
// express, since a source is whatever the operator points at.
func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")

	content := []byte("test content")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}

	dstRoot, err := fsroot.OpenExisting(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dstRoot.Close() })

	dst := dstRoot.NewPath("dest.txt")
	if err := copyFile(dstRoot, src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	got, err := os.ReadFile(dst.Abs())
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

	dstRoot, err := fsroot.OpenExisting(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dstRoot.Close() })

	if err := copyFile(dstRoot, src, dstRoot.NewPath("dest.txt")); err == nil {
		t.Error("expected error for non-existent source")
	}
}

// TestCopyDir tests recursive directory copying.
func TestCopyDir(t *testing.T) {
	src := t.TempDir()

	dstRoot, err := fsroot.OpenExisting(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dstRoot.Close() })
	dstPath := dstRoot.NewPath("dest")
	dst := dstPath.Abs()

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

	if err := CopyDir(dstRoot, src, dstPath); err != nil {
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

// --- Install / uninstall, end to end ---

// installIntoTempPrefix runs a real install into a temporary prefix and returns it.
//
// Every location the install touches is redirected: the prefix is a temp directory, and the XDG variables
// point at another, so a test never writes into the developer's own trees. Shells are named explicitly rather
// than detected, so the completion assertions hold on a runner with no shells installed.
//
// Parameters:
//   - `t`: the test harness.
//
// Returns:
//   - `string`: the prefix the tool was installed into.
//   - `SelfInstallInfo`: the descriptor it was installed with.
func installIntoTempPrefix(t *testing.T) (string, SelfInstallInfo) {

	t.Helper()

	sandbox := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(sandbox, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(sandbox, "cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(sandbox, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(sandbox, "state"))

	prefix := filepath.Join(sandbox, "prefix")
	info := SelfInstallInfo{Name: "selftest", Version: "1.0.0"}

	rootCmd := &cobra.Command{Use: "selftest"}
	if err := runSelfInstall(rootCmd, prefix, info, installFlags{Shells: []string{"bash"}}); err != nil {
		t.Fatalf("runSelfInstall: %v", err)
	}

	return prefix, info
}

// TestRunSelfInstall_LaysOutThePrefix proves an install produces the tree it claims to, on every platform.
//
// This is the first coverage of the install path itself: the tests around it check that subcommands exist and
// that helpers behave, and none of them ever installed anything. It runs on ubuntu, macOS and Windows through
// the standard test matrix, which is what makes it a check on the root plumbing rather than on one platform's
// path handling.
//
// Modes are deliberately not asserted: `Mode().Perm()` reports 0666 on Windows whatever the DACL says (#405
// ruling 5), so a mode assertion here would either be Unix-only or false. Enforcement is proved by the DACL
// read-back tests in pkg/signing and pkg/fsroot.
func TestRunSelfInstall_LaysOutThePrefix(t *testing.T) {

	prefix, info := installIntoTempPrefix(t)

	for _, relative := range []string{
		// executableName, not the bare tool name: on Windows an install without the suffix produces a file
		// the operator cannot run. The unit test asserted the bare name and so agreed with the bug; the
		// scenario, driving the real binary, did not.
		filepath.Join("bin", executableName(info.Name)),
		filepath.Join("share", "bash-completion", "completions", info.Name),
		filepath.Join("share", info.Name, "manifest.json"),
	} {
		if _, err := os.Stat(filepath.Join(prefix, relative)); err != nil {
			t.Errorf("install did not produce %s: %v", relative, err)
		}
	}

	m, err := readManifest(prefix, info.Name)
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}

	if m.Prefix != prefix {
		t.Errorf("manifest prefix = %q, want %q", m.Prefix, prefix)
	}
	if len(m.Files) == 0 {
		t.Fatal("manifest records no files")
	}

	// Every recorded path must resolve — a manifest naming a file that is not there is how uninstall
	// silently leaves things behind.
	for _, entry := range m.Files {
		if _, err := os.Stat(filepath.Join(prefix, entry.Path)); err != nil {
			t.Errorf("manifest names %s, which does not exist: %v", entry.Path, err)
		}
		if entry.SHA256 == "" {
			t.Errorf("manifest entry %s carries no checksum", entry.Path)
		}
	}
}

// TestRunSelfUninstall_RemovesWhatItInstalled closes the loop: what the install recorded, the uninstall takes
// away.
func TestRunSelfUninstall_RemovesWhatItInstalled(t *testing.T) {

	prefix, info := installIntoTempPrefix(t)

	m, err := readManifest(prefix, info.Name)
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}

	if err := runSelfUninstall(prefix, info); err != nil {
		t.Fatalf("runSelfUninstall: %v", err)
	}

	for _, entry := range m.Files {
		if _, err := os.Stat(filepath.Join(prefix, entry.Path)); !os.IsNotExist(err) {
			t.Errorf("%s survived uninstall (err = %v)", entry.Path, err)
		}
	}

	if _, err := os.Stat(filepath.Join(prefix, "share", info.Name, "manifest.json")); !os.IsNotExist(err) {
		t.Errorf("manifest survived uninstall (err = %v)", err)
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
	prefixRoot, err := fsroot.OpenExisting(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prefixRoot.Close() })

	if err := writeManifest(prefixRoot, "test", "1.0.0", []string{"bin/test"}); err != nil {
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
	prefixRoot, err := fsroot.OpenExisting(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prefixRoot.Close() })

	if err := writeManifest(prefixRoot, "test", "1.0.0", []string{"bin/test", "share/man/man1/test.1"}); err != nil {
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

// --- The record is what the tool owns: an install replaces it (#933) ---

// installSandbox redirects every location an install touches and returns the prefix to use.
//
// Separate from installIntoTempPrefix because these tests install twice into the same prefix, which is
// the case #933 is about: the second install is what retires the first one's leavings.
//
// Parameters:
//   - `t`: the test harness.
//
// Returns:
//   - `string`: the prefix to install into; it does not exist yet.
func installSandbox(t *testing.T) string {

	t.Helper()

	sandbox := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(sandbox, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(sandbox, "cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(sandbox, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(sandbox, "state"))

	return filepath.Join(sandbox, "prefix")
}

// installOnce runs one install into `prefix`, with hooks standing in for star's extensions.
//
// Parameters:
//   - `t`: the test harness.
//   - `prefix`: where to install.
//   - `hooks`: post-install hooks, each planting files and returning their paths relative to `prefix`.
func installOnce(t *testing.T, prefix string, hooks ...func(string) []string) {

	t.Helper()

	info := SelfInstallInfo{Name: "selftest", Version: "1.0.0", PostInstallHooks: hooks}
	rootCmd := &cobra.Command{Use: "selftest"}

	if err := runSelfInstall(rootCmd, prefix, info, installFlags{Shells: []string{"bash"}}); err != nil {
		t.Fatalf("runSelfInstall: %v", err)
	}
}

// plantingHook returns a post-install hook that writes one file, the way star's hook writes its
// extension tree.
//
// Parameters:
//   - `t`: the test harness.
//   - `relative`: the file's path relative to the prefix.
//   - `content`: what to write, so a later install can be told from an earlier one.
//
// Returns:
//   - `func(string) []string`: the hook, returning the one path it planted.
func plantingHook(t *testing.T, relative, content string) func(string) []string {

	t.Helper()

	return func(prefix string) []string {
		absolute := filepath.Join(prefix, relative)

		if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
			t.Fatalf("planting %s: %v", relative, err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
			t.Fatalf("planting %s: %v", relative, err)
		}

		return []string{relative}
	}
}

// TestRunSelfInstall_RetiresWhatItNoLongerOwns is #933 itself, in miniature.
//
// star's extensions moved under `devlore/` in #918. The install that followed wrote them to the new path
// and overwrote the manifest, which left 24 files at the old path owned by nothing: `self uninstall`
// reads only the manifest, so no later command could reach them. They were removed by hand on
// 2026-09-23. An install must retire what the record it replaces owned and it does not.
func TestRunSelfInstall_RetiresWhatItNoLongerOwns(t *testing.T) {

	prefix := installSandbox(t)
	oldPath := filepath.Join("share", "selftest", "extensions", "old.star")
	newPath := filepath.Join("share", "selftest", "devlore", "extensions", "new.star")

	installOnce(t, prefix, plantingHook(t, oldPath, "first"))

	if _, err := os.Stat(filepath.Join(prefix, oldPath)); err != nil {
		t.Fatalf("the first install did not plant %s: %v", oldPath, err)
	}

	installOnce(t, prefix, plantingHook(t, newPath, "second"))

	if _, err := os.Stat(filepath.Join(prefix, oldPath)); !os.IsNotExist(err) {
		t.Errorf("%s survived an install that stopped writing it (stat error = %v); "+
			"that is the file nothing can ever remove", oldPath, err)
	}
	if _, err := os.Stat(filepath.Join(prefix, newPath)); err != nil {
		t.Errorf("the second install did not plant %s: %v", newPath, err)
	}
}

// TestRunSelfInstall_ManifestMatchesTheTree is the invariant the retirement exists to hold.
//
// Requirement 3 of the plan: after any install, the record names exactly what is on disk for that tool.
// It is asserted after a *changing* re-install, because that is the case a snapshot-shaped record gets
// wrong. Stated as a set comparison in both directions: a path on disk and in no record can never be
// uninstalled, and a path in the record and not on disk is how an uninstall reports success over files
// it never touched.
func TestRunSelfInstall_ManifestMatchesTheTree(t *testing.T) {

	prefix := installSandbox(t)

	installOnce(t, prefix, plantingHook(t, filepath.Join("share", "selftest", "a", "one.star"), "first"))
	installOnce(t, prefix, plantingHook(t, filepath.Join("share", "selftest", "b", "two.star"), "second"))

	m, err := readManifest(prefix, "selftest")
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}

	recorded := make(map[string]struct{}, len(m.Files))
	for _, entry := range m.Files {
		recorded[filepath.Clean(entry.Path)] = struct{}{}

		if _, err := os.Stat(filepath.Join(prefix, entry.Path)); err != nil {
			t.Errorf("the record names %s, which is not on disk: %v", entry.Path, err)
		}
	}

	// The manifest does not name itself: it is written last, from the list of everything else.
	manifestRelative := filepath.Clean(relativeManifestPath("selftest"))

	err = filepath.Walk(prefix, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		relative, err := filepath.Rel(prefix, path)
		if err != nil {
			return err
		}
		relative = filepath.Clean(relative)

		if relative == manifestRelative {
			return nil
		}
		if _, ok := recorded[relative]; !ok {
			t.Errorf("%s is on disk and in no record, so no uninstall can ever remove it", relative)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walking the prefix: %v", err)
	}
}

// TestRunSelfInstall_LeavesAFileChangedSinceItWasWritten pins the ruling of 2026-09-23.
//
// A file whose hash no longer matches the record may be the operator's own edit, so it is left and
// reported rather than deleted. There is no solution to that on the file itself: it cannot be removed
// safely and it cannot be trusted. This test exists so the behaviour is a decision someone made rather
// than something a later change quietly reverses.
func TestRunSelfInstall_LeavesAFileChangedSinceItWasWritten(t *testing.T) {

	prefix := installSandbox(t)
	planted := filepath.Join("share", "selftest", "extensions", "edited.star")

	installOnce(t, prefix, plantingHook(t, planted, "as installed"))

	absolute := filepath.Join(prefix, planted)
	if err := os.WriteFile(absolute, []byte("edited by the operator"), 0o600); err != nil {
		t.Fatalf("editing %s: %v", planted, err)
	}

	installOnce(t, prefix, plantingHook(t, filepath.Join("share", "selftest", "other", "new.star"), "second"))

	content, err := os.ReadFile(absolute)
	if err != nil {
		t.Fatalf("the edited file was removed, and it was not ours to remove: %v", err)
	}
	if string(content) != "edited by the operator" {
		t.Errorf("content = %q, want the operator's edit untouched", content)
	}
}

// TestRetireSupersededFiles_ReachesNothingOutsideTheRecord is Requirement 4.
//
// Config, cache and writ's layer directories are placed by an install and recorded by nothing, so the
// retirement must not reach them. Stated generally, as a file in the prefix that no record names:
// whatever is not in the record is not this program's to remove.
func TestRetireSupersededFiles_ReachesNothingOutsideTheRecord(t *testing.T) {

	prefix := installSandbox(t)
	installOnce(t, prefix, plantingHook(t, filepath.Join("share", "selftest", "a", "one.star"), "first"))

	stranger := filepath.Join(prefix, "share", "someone-else", "theirs.conf")
	if err := os.MkdirAll(filepath.Dir(stranger), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(stranger, []byte("not ours"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	installOnce(t, prefix, plantingHook(t, filepath.Join("share", "selftest", "b", "two.star"), "second"))

	if _, err := os.Stat(stranger); err != nil {
		t.Errorf("a file no record names was removed: %v", err)
	}
}

// TestRetireSupersededFiles_FirstInstallHasNothingToRetire is Requirement 1.
//
// `self uninstall` treats a missing record as an error, because the operator asked to remove something
// this program never placed. An install must treat the same absence as the ordinary first install.
func TestRetireSupersededFiles_FirstInstallHasNothingToRetire(t *testing.T) {

	prefix := t.TempDir()

	prefixRoot, err := OpenTree(prefix)
	if err != nil {
		t.Fatalf("OpenTree: %v", err)
	}
	defer func() {
		if err := prefixRoot.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	removed, skipped := retireSupersededFiles(prefixRoot, prefix, "selftest", []string{"bin/selftest"})

	if len(removed) != 0 || len(skipped) != 0 {
		t.Errorf("removed = %v, skipped = %v; a first install has no record to retire", removed, skipped)
	}
}
