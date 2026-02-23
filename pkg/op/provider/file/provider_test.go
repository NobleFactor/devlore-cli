// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Link ---

func TestLink_CreatesNewSymlink(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "target")
	if err := os.WriteFile(source, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmp, "link")

	p := Provider{}
	result, state, err := p.Link(source, linkPath)
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if result != linkPath {
		t.Errorf("result = %q, want %q", result, linkPath)
	}

	s := op.AsStateMap(state)
	if s == nil {
		t.Fatal("state is nil, want non-nil")
	}
	if op.StateBool(s, "existed_before") {
		t.Error("existed_before = true, want false")
	}

	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if got != source {
		t.Errorf("symlink target = %q, want %q", got, source)
	}
}

func TestLink_OverwritesExistingSymlink(t *testing.T) {
	tmp := t.TempDir()
	oldTarget := filepath.Join(tmp, "old-target")
	newTarget := filepath.Join(tmp, "new-target")
	if err := os.WriteFile(oldTarget, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newTarget, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmp, "link")
	if err := os.Symlink(oldTarget, linkPath); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	result, state, err := p.Link(newTarget, linkPath)
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if result != linkPath {
		t.Errorf("result = %q, want %q", result, linkPath)
	}

	s := op.AsStateMap(state)
	if s == nil {
		t.Fatal("state is nil, want non-nil")
	}
	if !op.StateBool(s, "existed_before") {
		t.Error("existed_before = false, want true")
	}
	if prev := op.StateString(s, "previous_target"); prev != oldTarget {
		t.Errorf("previous_target = %q, want %q", prev, oldTarget)
	}

	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if got != newTarget {
		t.Errorf("symlink target = %q, want %q", got, newTarget)
	}
}

func TestLink_IdempotentWhenCorrect(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "target")
	if err := os.WriteFile(source, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmp, "link")
	if err := os.Symlink(source, linkPath); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	result, state, err := p.Link(source, linkPath)
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if result != linkPath {
		t.Errorf("result = %q, want %q", result, linkPath)
	}
	if state != nil {
		t.Errorf("state = %v, want nil (no-op)", state)
	}
}

func TestLink_CreatesParentDirectories(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "target")
	if err := os.WriteFile(source, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmp, "deep", "nested", "link")

	p := Provider{}
	_, _, err := p.Link(source, linkPath)
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}

	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if got != source {
		t.Errorf("symlink target = %q, want %q", got, source)
	}
}

// --- CompensateLink ---

func TestCompensateLink_NilState(t *testing.T) {
	p := Provider{}
	if err := p.CompensateLink(nil); err != nil {
		t.Errorf("CompensateLink(nil) = %v, want nil", err)
	}
}

func TestCompensateLink_ExistedBeforeFalse_RemovesSymlink(t *testing.T) {
	tmp := t.TempDir()
	linkPath := filepath.Join(tmp, "link")
	if err := os.Symlink("/some/target", linkPath); err != nil {
		t.Fatal(err)
	}

	state := map[string]any{
		"path":           linkPath,
		"existed_before": false,
	}

	p := Provider{}
	if err := p.CompensateLink(state); err != nil {
		t.Fatalf("CompensateLink() error = %v", err)
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("symlink still exists after compensation")
	}
}

func TestCompensateLink_ExistedBeforeTrue_RestoresOldTarget(t *testing.T) {
	tmp := t.TempDir()
	linkPath := filepath.Join(tmp, "link")
	oldTarget := filepath.Join(tmp, "old-target")
	// Create the new symlink that we want to compensate.
	if err := os.Symlink("/some/new-target", linkPath); err != nil {
		t.Fatal(err)
	}

	state := map[string]any{
		"path":            linkPath,
		"existed_before":  true,
		"previous_target": oldTarget,
	}

	p := Provider{}
	if err := p.CompensateLink(state); err != nil {
		t.Fatalf("CompensateLink() error = %v", err)
	}

	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if got != oldTarget {
		t.Errorf("restored target = %q, want %q", got, oldTarget)
	}
}

func TestCompensateLink_ExistedBeforeTrue_NoPreviousTarget_RemovesSymlink(t *testing.T) {
	tmp := t.TempDir()
	linkPath := filepath.Join(tmp, "link")
	if err := os.Symlink("/some/target", linkPath); err != nil {
		t.Fatal(err)
	}

	state := map[string]any{
		"path":           linkPath,
		"existed_before": true,
		// No previous_target — was a non-symlink before.
	}

	p := Provider{}
	if err := p.CompensateLink(state); err != nil {
		t.Fatalf("CompensateLink() error = %v", err)
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("symlink still exists after compensation")
	}
}

// --- Copy ---

func TestCopy_WritesNewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")
	content := []byte("hello world")

	p := Provider{}
	checksum, state, err := p.Copy(path, 0o600, content)
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
	}

	h := sha256.Sum256(content)
	wantChecksum := "sha256:" + hex.EncodeToString(h[:])
	if checksum != wantChecksum {
		t.Errorf("checksum = %q, want %q", checksum, wantChecksum)
	}

	s := op.AsStateMap(state)
	if op.StateBool(s, "existed_before") {
		t.Error("existed_before = true, want false")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("file content = %q, want %q", got, "hello world")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %o, want %o", info.Mode().Perm(), 0o600)
	}
}

func TestCopy_OverwritesExistingFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")
	if err := os.WriteFile(path, []byte("original"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	_, state, err := p.Copy(path, 0o644, []byte("replaced"))
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
	}

	s := op.AsStateMap(state)
	if !op.StateBool(s, "existed_before") {
		t.Error("existed_before = false, want true")
	}
	if prev := op.StateBytes(s, "previous_content"); string(prev) != "original" {
		t.Errorf("previous_content = %q, want %q", prev, "original")
	}
	if prevMode := op.StateFileMode(s, "previous_mode"); prevMode != 0o755 {
		t.Errorf("previous_mode = %o, want %o", prevMode, 0o755)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "replaced" {
		t.Errorf("file content = %q, want %q", got, "replaced")
	}
}

func TestCopy_DefaultModeWhenZero(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")

	p := Provider{}
	_, _, err := p.Copy(path, 0, []byte("content"))
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("file mode = %o, want %o (default)", info.Mode().Perm(), 0o644)
	}
}

// --- CompensateCopy ---

func TestCompensateCopy_NilState(t *testing.T) {
	p := Provider{}
	if err := p.CompensateCopy(nil); err != nil {
		t.Errorf("CompensateCopy(nil) = %v, want nil", err)
	}
}

func TestCompensateCopy_ExistedBeforeFalse_RemovesFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")
	if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	state := map[string]any{
		"path":           path,
		"existed_before": false,
	}

	p := Provider{}
	if err := p.CompensateCopy(state); err != nil {
		t.Fatalf("CompensateCopy() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after compensation")
	}
}

func TestCompensateCopy_ExistedBeforeTrue_RestoresContent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")

	// File does not exist on disk — simulates that the forward Copy
	// created it and we are now compensating (the forward Copy already
	// removed the old file and wrote the new one; compensation restores
	// the original content by writing a fresh file).

	state := map[string]any{
		"path":             path,
		"existed_before":   true,
		"previous_content": []byte("original content"),
		"previous_mode":    os.FileMode(0o755),
	}

	p := Provider{}
	if err := p.CompensateCopy(state); err != nil {
		t.Fatalf("CompensateCopy() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "original content" {
		t.Errorf("file content = %q, want %q", got, "original content")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("file mode = %o, want %o", info.Mode().Perm(), 0o755)
	}
}

// --- Backup ---

func TestBackup_MovesFileToTimestampedBackup(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "myfile.txt")
	if err := os.WriteFile(path, []byte("backup me"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	result, state, err := p.Backup(path, ".bak")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	if !strings.HasPrefix(result, path+".bak.") {
		t.Errorf("backup path = %q, want prefix %q", result, path+".bak.")
	}

	// Original should be gone.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("original file still exists after backup")
	}

	// Backup should exist with correct content.
	got, err := os.ReadFile(result)
	if err != nil {
		t.Fatalf("ReadFile(backup) error = %v", err)
	}
	if string(got) != "backup me" {
		t.Errorf("backup content = %q, want %q", got, "backup me")
	}

	s := op.AsStateMap(state)
	if op.StateString(s, "original_path") != path {
		t.Errorf("state original_path = %q, want %q", op.StateString(s, "original_path"), path)
	}
	if op.StateString(s, "backup_path") != result {
		t.Errorf("state backup_path = %q, want %q", op.StateString(s, "backup_path"), result)
	}
}

func TestBackup_DefaultSuffix(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "myfile.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	result, _, err := p.Backup(path, "")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	if !strings.HasPrefix(result, path+".writ-backup.") {
		t.Errorf("backup path = %q, want prefix %q (default suffix)", result, path+".writ-backup.")
	}
}

// --- CompensateBackup ---

func TestCompensateBackup_RestoresOriginal(t *testing.T) {
	tmp := t.TempDir()
	originalPath := filepath.Join(tmp, "myfile.txt")
	backupPath := filepath.Join(tmp, "myfile.txt.bak.20250101-120000")
	if err := os.WriteFile(backupPath, []byte("restore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	state := map[string]any{
		"original_path": originalPath,
		"backup_path":   backupPath,
	}

	p := Provider{}
	if err := p.CompensateBackup(state); err != nil {
		t.Fatalf("CompensateBackup() error = %v", err)
	}

	got, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "restore me" {
		t.Errorf("restored content = %q, want %q", got, "restore me")
	}

	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("backup file still exists after compensation")
	}
}

// --- Unlink ---

func TestUnlink_RemovesSymlink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmp, "link")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	result, state, err := p.Unlink(linkPath, false, "")
	if err != nil {
		t.Fatalf("Unlink() error = %v", err)
	}
	if result != linkPath {
		t.Errorf("result = %q, want %q", result, linkPath)
	}

	s := op.AsStateMap(state)
	if op.StateString(s, "target") != target {
		t.Errorf("state target = %q, want %q", op.StateString(s, "target"), target)
	}

	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("symlink still exists after unlink")
	}
}

func TestUnlink_AlreadyGone(t *testing.T) {
	tmp := t.TempDir()
	linkPath := filepath.Join(tmp, "nonexistent")

	p := Provider{}
	result, state, err := p.Unlink(linkPath, false, "")
	if err != nil {
		t.Fatalf("Unlink() error = %v", err)
	}
	if result != linkPath {
		t.Errorf("result = %q, want %q", result, linkPath)
	}
	if state != nil {
		t.Errorf("state = %v, want nil (no-op)", state)
	}
}

func TestUnlink_NotASymlink_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "regular-file")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	_, _, err := p.Unlink(path, false, "")
	if err == nil {
		t.Fatal("Unlink() on regular file should return error")
	}
	if !strings.Contains(err.Error(), "not a symlink") {
		t.Errorf("error = %q, want message containing 'not a symlink'", err)
	}
}

// --- Remove ---

func TestRemove_RemovesFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	result, state, err := p.Remove(path, false, "")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if result != path {
		t.Errorf("result = %q, want %q", result, path)
	}

	s := op.AsStateMap(state)
	if op.StateString(s, "path") != path {
		t.Errorf("state path = %q, want %q", op.StateString(s, "path"), path)
	}
	if string(op.StateBytes(s, "content")) != "content" {
		t.Errorf("state content = %q, want %q", op.StateBytes(s, "content"), "content")
	}
	if op.StateFileMode(s, "mode") != 0o600 {
		t.Errorf("state mode = %o, want %o", op.StateFileMode(s, "mode"), 0o600)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after remove")
	}
}

func TestRemove_AlreadyGone(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nonexistent")

	p := Provider{}
	result, state, err := p.Remove(path, false, "")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if result != path {
		t.Errorf("result = %q, want %q", result, path)
	}
	if state != nil {
		t.Errorf("state = %v, want nil (no-op)", state)
	}
}

// --- Write ---

func TestWrite_WritesContentToNewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")

	p := Provider{}
	result, state, err := p.Write("hello world", path, 0o644)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if result != path {
		t.Errorf("result = %q, want %q", result, path)
	}

	s := op.AsStateMap(state)
	if op.StateBool(s, "existed_before") {
		t.Error("existed_before = true, want false")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("file content = %q, want %q", got, "hello world")
	}
}

func TestWrite_EmptyContent_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")

	p := Provider{}
	_, _, err := p.Write("", path, 0o644)
	if err == nil {
		t.Fatal("Write() with empty content should return error")
	}
	if !strings.Contains(err.Error(), "no content") {
		t.Errorf("error = %q, want message containing 'no content'", err)
	}
}

// --- Move ---

func TestMove_FallsBackToOsRename(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source.txt")
	dst := filepath.Join(tmp, "dest.txt")
	if err := os.WriteFile(src, []byte("move me"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	result, state, err := p.Move(nil, src, dst)
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if result != dst {
		t.Errorf("result = %q, want %q", result, dst)
	}

	s := op.AsStateMap(state)
	if op.StateString(s, "source") != src {
		t.Errorf("state source = %q, want %q", op.StateString(s, "source"), src)
	}
	if op.StateString(s, "path") != dst {
		t.Errorf("state path = %q, want %q", op.StateString(s, "path"), dst)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source still exists after move")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile(dst) error = %v", err)
	}
	if string(got) != "move me" {
		t.Errorf("dest content = %q, want %q", got, "move me")
	}
}

func TestMove_UsesGitMvWhenProvided(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source.txt")
	dst := filepath.Join(tmp, "dest.txt")
	if err := os.WriteFile(src, []byte("git move"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gitMvCalled bool
	gitMv := func(s, d string) error {
		gitMvCalled = true
		// Simulate git mv by doing a rename.
		return os.Rename(s, d)
	}

	p := Provider{}
	result, state, err := p.Move(gitMv, src, dst)
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if !gitMvCalled {
		t.Error("gitMv was not called")
	}
	if result != dst {
		t.Errorf("result = %q, want %q", result, dst)
	}

	s := op.AsStateMap(state)
	if op.StateString(s, "source") != src {
		t.Errorf("state source = %q, want %q", op.StateString(s, "source"), src)
	}
}

func TestMove_FallsBackWhenGitMvFails(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source.txt")
	dst := filepath.Join(tmp, "dest.txt")
	if err := os.WriteFile(src, []byte("fallback"), 0o644); err != nil {
		t.Fatal(err)
	}

	gitMv := func(_, _ string) error {
		return errors.New("git mv failed")
	}

	p := Provider{}
	result, _, err := p.Move(gitMv, src, dst)
	if err != nil {
		t.Fatalf("Move() error = %v (should fall back to os.Rename)", err)
	}
	if result != dst {
		t.Errorf("result = %q, want %q", result, dst)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile(dst) error = %v", err)
	}
	if string(got) != "fallback" {
		t.Errorf("dest content = %q, want %q", got, "fallback")
	}
}

// --- Helpers ---

func TestIsSubpath(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		parent string
		want   bool
	}{
		{"child", "/a/b/c", "/a/b", true},
		{"deeply nested", "/a/b/c/d/e", "/a/b", true},
		{"equal", "/a/b", "/a/b", false},
		{"parent of", "/a", "/a/b", false},
		{"escape with dotdot", "/a/b/../c", "/a/b", false},
		{"sibling", "/a/c", "/a/b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSubpath(tt.path, tt.parent); got != tt.want {
				t.Errorf("isSubpath(%q, %q) = %v, want %v", tt.path, tt.parent, got, tt.want)
			}
		})
	}
}

func TestChecksumBytes(t *testing.T) {
	data := []byte("hello")
	got := checksumBytes(data)

	h := sha256.Sum256(data)
	want := "sha256:" + hex.EncodeToString(h[:])
	if got != want {
		t.Errorf("checksumBytes(%q) = %q, want %q", data, got, want)
	}

	// Deterministic: same input produces same output.
	if again := checksumBytes(data); again != got {
		t.Errorf("checksumBytes not deterministic: %q != %q", again, got)
	}
}
