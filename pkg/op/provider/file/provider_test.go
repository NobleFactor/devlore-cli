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
	fileResource := testFileResource(t, []byte("hello world"))

	p := Provider{}
	result, _, err := p.Copy(fileResource, path, 0o600)
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
	}

	if result.SourcePath != path {
		t.Errorf("result.SourcePath = %q, want %q", result.SourcePath, path)
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

	p := Provider{Root: tmp}
	blob := testFileResource(t, []byte("replaced"))
	_, _, err := p.Copy(blob, path, 0o644)
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
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
	blob := testFileResource(t, []byte("content"))
	_, _, err := p.Copy(blob, path, 0)
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

func TestCompensateCopy_NilState_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("CompensateCopy(nil) did not panic — nil undo state is a bug")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is %T, want string", r)
		}
		if !strings.Contains(msg, "BUG") {
			t.Errorf("panic message = %q, want to contain 'BUG'", msg)
		}
	}()
	p := Provider{}
	_ = p.CompensateCopy(nil)
}

func TestCompensateCopy_NewFile_RemovesOnCompensate(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")
	if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Tombstone with no RecoveryPath — file didn't exist before Copy created it.
	state := map[string]any{
		"tombstone": Tombstone{OriginalPath: path},
	}

	p := Provider{}
	if err := p.CompensateCopy(state); err != nil {
		t.Fatalf("CompensateCopy() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after compensation")
	}
}

func TestCompensateCopy_Overwrite_RestoresFromRecovery(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")
	recoveryPath := filepath.Join(tmp, "recovery", "output.txt")

	// Create the "new" file at destination (simulates forward Copy wrote it).
	if err := os.WriteFile(path, []byte("replaced content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a recovery file with original content (simulates prepareWrite moved it).
	if err := os.MkdirAll(filepath.Dir(recoveryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recoveryPath, []byte("original content"), 0o755); err != nil {
		t.Fatal(err)
	}

	state := map[string]any{
		"tombstone": Tombstone{
			OriginalPath: path,
			RecoveryPath: recoveryPath,
		},
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
		t.Errorf("restored content = %q, want %q", got, "original content")
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

	// Checksum should match the original file content.
	h := sha256.Sum256([]byte("backup me"))
	wantChecksum := "sha256:" + hex.EncodeToString(h[:])
	if got := op.StateString(s, "written_checksum"); got != wantChecksum {
		t.Errorf("state written_checksum = %q, want %q", got, wantChecksum)
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

	if !strings.HasPrefix(result, path+".devlore-backup.") {
		t.Errorf("backup path = %q, want prefix %q (default suffix)", result, path+".devlore-backup.")
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

func TestCompensateBackup_ChecksumMismatch_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	originalPath := filepath.Join(tmp, "myfile.txt")
	backupPath := filepath.Join(tmp, "myfile.txt.bak.20250101-120000")
	if err := os.WriteFile(backupPath, []byte("tampered content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Checksum computed from different content than what's on disk.
	h := sha256.Sum256([]byte("original content"))
	wrongChecksum := "sha256:" + hex.EncodeToString(h[:])

	state := map[string]any{
		"original_path":    originalPath,
		"backup_path":      backupPath,
		"written_checksum": wrongChecksum,
	}

	p := Provider{}
	err := p.CompensateBackup(state)
	if err == nil {
		t.Fatal("CompensateBackup() should return error on checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %q, want message containing 'checksum mismatch'", err)
	}

	// Backup file should NOT have been moved.
	if _, err := os.Stat(backupPath); err != nil {
		t.Error("backup file should still exist when compensation is skipped")
	}
}

// --- Unlink ---

func TestUnlink_RemovesSymlink(t *testing.T) {
	t.Skip("blocked on issue #164: recovery site creation fails on macOS SIP")
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
	if result.OriginalPath != linkPath {
		t.Errorf("result.OriginalPath = %q, want %q", result.OriginalPath, linkPath)
	}

	if state == nil {
		t.Fatal("state is nil, want non-nil")
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
	if result != (Tombstone{}) {
		t.Errorf("result = %+v, want empty Tombstone", result)
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
	t.Skip("blocked on issue #164: recovery site creation fails on macOS SIP")
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
	if result.OriginalPath != path {
		t.Errorf("result.OriginalPath = %q, want %q", result.OriginalPath, path)
	}

	tombstone, ok := state["tombstone"].(Tombstone)
	if !ok {
		t.Fatal("state missing tombstone")
	}
	if tombstone.OriginalPath != path {
		t.Errorf("tombstone.OriginalPath = %q, want %q", tombstone.OriginalPath, path)
	}
	if tombstone.RecoveryPath == "" {
		t.Error("tombstone.RecoveryPath should not be empty")
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
	if result != (Tombstone{}) {
		t.Errorf("result = %+v, want empty Tombstone", result)
	}
	if state != nil {
		t.Errorf("state = %v, want nil (no-op)", state)
	}
}

// --- Write ---

func TestWriteText_WritesContentToNewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")

	p := Provider{}
	result, state, err := p.WriteText(path, "hello world", 0o644)
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}
	if result.SourcePath != path {
		t.Errorf("result.SourcePath = %q, want %q", result.SourcePath, path)
	}

	if state == nil {
		t.Fatal("state is nil, want non-nil")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("file content = %q, want %q", got, "hello world")
	}
}

func TestWriteBytes_WritesContentToNewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.bin")

	p := Provider{}
	result, state, err := p.WriteBytes(path, "binary data", 0o600)
	if err != nil {
		t.Fatalf("WriteBytes() error = %v", err)
	}
	if result.SourcePath != path {
		t.Errorf("result.SourcePath = %q, want %q", result.SourcePath, path)
	}

	if state == nil {
		t.Fatal("state is nil, want non-nil")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "binary data" {
		t.Errorf("file content = %q, want %q", got, "binary data")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %o, want %o", info.Mode().Perm(), 0o600)
	}
}

// --- Move ---

func TestMove(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source.txt")
	dst := filepath.Join(tmp, "dest.txt")
	if err := os.WriteFile(src, []byte("move me"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	result, state, err := p.Move(src, dst)
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
	if op.StateString(s, "destination") != dst {
		t.Errorf("state destination = %q, want %q", op.StateString(s, "destination"), dst)
	}

	// Checksum should match the original file content.
	h := sha256.Sum256([]byte("move me"))
	wantChecksum := "sha256:" + hex.EncodeToString(h[:])
	if got := op.StateString(s, "written_checksum"); got != wantChecksum {
		t.Errorf("state written_checksum = %q, want %q", got, wantChecksum)
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

// --- CompensateMove ---

func TestCompensateMove_NilState(t *testing.T) {
	p := Provider{}
	if err := p.CompensateMove(nil); err != nil {
		t.Errorf("CompensateMove(nil) = %v, want nil", err)
	}
}

func TestCompensateMove_ChecksumMismatch_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source.txt")
	dst := filepath.Join(tmp, "dest.txt")
	if err := os.WriteFile(dst, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256([]byte("original"))
	wrongChecksum := "sha256:" + hex.EncodeToString(h[:])

	state := map[string]any{
		"source":           src,
		"destination":      dst,
		"written_checksum": wrongChecksum,
	}

	p := Provider{}
	err := p.CompensateMove(state)
	if err == nil {
		t.Fatal("CompensateMove() should return error on checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %q, want message containing 'checksum mismatch'", err)
	}

	// File should NOT have been moved back.
	if _, err := os.Stat(dst); err != nil {
		t.Error("dest file should still exist when compensation is skipped")
	}
}

func TestCompensateMove_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source.txt")
	dst := filepath.Join(tmp, "dest.txt")
	if err := os.WriteFile(src, []byte("round trip"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	_, state, err := p.Move(src, dst)
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}

	// Compensate: should move back.
	if err := p.CompensateMove(state); err != nil {
		t.Fatalf("CompensateMove() error = %v", err)
	}

	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("dest should not exist after compensation")
	}
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile(src) error = %v", err)
	}
	if string(got) != "round trip" {
		t.Errorf("restored content = %q, want %q", got, "round trip")
	}
}

// --- CompensateWriteText / CompensateWriteBytes ---

// TestCompensateWriteText_NilState is blocked on issue #165: compensateWrite missing nil guard.
func TestCompensateWriteText_NilState(t *testing.T) {
	t.Skip("blocked on issue #165: compensateWrite missing nil guard on undo parameter")
	p := Provider{}
	if err := p.CompensateWriteText(nil); err != nil {
		t.Errorf("CompensateWriteText(nil) = %v, want nil", err)
	}
}

func TestCompensateWriteText_InvalidTombstone_ReturnsError(t *testing.T) {
	state := map[string]any{
		"tombstone": "not-a-tombstone",
	}

	p := Provider{}
	err := p.CompensateWriteText(state)
	if err == nil {
		t.Fatal("CompensateWriteText() with invalid tombstone should return error")
	}
}

func TestCompensateWriteBytes_NilState(t *testing.T) {
	t.Skip("blocked on issue #165: compensateWrite missing nil guard on undo parameter")
	p := Provider{}
	if err := p.CompensateWriteBytes(nil); err != nil {
		t.Errorf("CompensateWriteBytes(nil) = %v, want nil", err)
	}
}

func TestCompensateWriteBytes_InvalidTombstone_ReturnsError(t *testing.T) {
	state := map[string]any{
		"tombstone": "not-a-tombstone",
	}

	p := Provider{}
	err := p.CompensateWriteBytes(state)
	if err == nil {
		t.Fatal("CompensateWriteBytes() with invalid tombstone should return error")
	}
}

func TestWriteText_DefaultModeWhenZero(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "default-mode.txt")

	p := Provider{}
	_, _, err := p.WriteText(path, "content", 0)
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("file mode = %o, want %o (default)", info.Mode().Perm(), 0o644)
	}
}

func TestWriteBytes_DefaultModeWhenZero(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "default-mode.bin")

	p := Provider{}
	_, _, err := p.WriteBytes(path, "content", 0)
	if err != nil {
		t.Fatalf("WriteBytes() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("file mode = %o, want %o (default)", info.Mode().Perm(), 0o644)
	}
}

func TestWriteText_CreatesParentDirectories(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nested", "deep", "file.txt")

	p := Provider{}
	result, _, err := p.WriteText(path, "nested content", 0o644)
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}
	if result.SourcePath != path {
		t.Errorf("result.SourcePath = %q, want %q", result.SourcePath, path)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "nested content" {
		t.Errorf("file content = %q, want %q", got, "nested content")
	}
}

func TestWriteText_CompensateWriteText_RoundTrip_NewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "roundtrip.txt")

	p := Provider{}
	_, state, err := p.WriteText(path, "to be undone", 0o644)
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}

	// File should exist after write.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist after WriteText: %v", err)
	}

	// Compensate: new file should be removed.
	if err := p.CompensateWriteText(state); err != nil {
		t.Fatalf("CompensateWriteText() error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after compensating a new-file WriteText")
	}
}

func TestWriteBytes_CompensateWriteBytes_RoundTrip_NewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "roundtrip.bin")

	p := Provider{}
	_, state, err := p.WriteBytes(path, "to be undone", 0o600)
	if err != nil {
		t.Fatalf("WriteBytes() error = %v", err)
	}

	// File should exist after write.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist after WriteBytes: %v", err)
	}

	// Compensate: new file should be removed.
	if err := p.CompensateWriteBytes(state); err != nil {
		t.Fatalf("CompensateWriteBytes() error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after compensating a new-file WriteBytes")
	}
}

// --- Exists ---

func TestExists_FileExists(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "exists.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	if !p.Exists(Resource{SourcePath: path}) {
		t.Error("Exists() = false, want true for existing file")
	}
}

func TestExists_FileDoesNotExist(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nonexistent.txt")

	p := Provider{}
	if p.Exists(Resource{SourcePath: path}) {
		t.Error("Exists() = true, want false for non-existent file")
	}
}

func TestExists_Symlink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	if !p.Exists(Resource{SourcePath: link}) {
		t.Error("Exists() = false, want true for symlink")
	}
}

func TestExists_Directory(t *testing.T) {
	tmp := t.TempDir()

	p := Provider{}
	if !p.Exists(Resource{SourcePath: tmp}) {
		t.Error("Exists() = false, want true for existing directory")
	}
}

// --- IsDir ---

func TestIsDir_Directory(t *testing.T) {
	tmp := t.TempDir()

	p := Provider{}
	if !p.IsDir(tmp) {
		t.Error("IsDir() = false, want true for directory")
	}
}

func TestIsDir_File(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	if p.IsDir(path) {
		t.Error("IsDir() = true, want false for regular file")
	}
}

func TestIsDir_NonExistent(t *testing.T) {
	p := Provider{}
	if p.IsDir("/nonexistent/path") {
		t.Error("IsDir() = true, want false for non-existent path")
	}
}

// --- IsFile ---

func TestIsFile_RegularFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	if !p.IsFile(path) {
		t.Error("IsFile() = false, want true for regular file")
	}
}

func TestIsFile_Directory(t *testing.T) {
	tmp := t.TempDir()

	p := Provider{}
	if p.IsFile(tmp) {
		t.Error("IsFile() = true, want false for directory")
	}
}

func TestIsFile_NonExistent(t *testing.T) {
	p := Provider{}
	if p.IsFile("/nonexistent/path") {
		t.Error("IsFile() = true, want false for non-existent path")
	}
}

func TestIsFile_Symlink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	// Symlink to regular file resolves via os.Stat, so IsFile returns true.
	if !p.IsFile(link) {
		t.Error("IsFile() = false, want true for symlink to regular file")
	}
}

// --- Join ---

func TestJoin(t *testing.T) {
	p := Provider{}

	if got := p.Join("a", "b", "c"); got != filepath.Join("a", "b", "c") {
		t.Errorf("Join(a,b,c) = %q, want %q", got, filepath.Join("a", "b", "c"))
	}
}

func TestJoin_Empty(t *testing.T) {
	p := Provider{}

	if got := p.Join(); got != "" {
		t.Errorf("Join() = %q, want empty string", got)
	}
}

func TestJoin_SinglePart(t *testing.T) {
	p := Provider{}

	if got := p.Join("only"); got != "only" {
		t.Errorf("Join(only) = %q, want %q", got, "only")
	}
}

// --- Name ---

func TestName(t *testing.T) {
	p := Provider{}

	tests := []struct {
		path string
		want string
	}{
		{"/a/b/c.txt", "c.txt"},
		{"/a/b/", "b"},
		{"file.go", "file.go"},
		{"/", "/"},
	}
	for _, tt := range tests {
		if got := p.Name(tt.path); got != tt.want {
			t.Errorf("Name(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// --- Parent ---

func TestParent(t *testing.T) {
	p := Provider{}

	tests := []struct {
		path string
		want string
	}{
		{"/a/b/c.txt", "/a/b"},
		{"/a/b/", "/a/b"},
		{"file.go", "."},
		{"/a", "/"},
	}
	for _, tt := range tests {
		if got := p.Parent(tt.path); got != tt.want {
			t.Errorf("Parent(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// --- Mkdir ---

func TestMkdir_CreatesDirectory(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "newdir")

	p := Provider{}
	result, err := p.Mkdir(path, 0o755)
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if result != path {
		t.Errorf("result = %q, want %q", result, path)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("created path is not a directory")
	}
}

func TestMkdir_CreatesParents(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "a", "b", "c")

	p := Provider{}
	_, err := p.Mkdir(path, 0o755)
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("created path is not a directory")
	}
}

func TestMkdir_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "existing")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	_, err := p.Mkdir(path, 0o755)
	if err != nil {
		t.Fatalf("Mkdir() on existing directory error = %v", err)
	}
}

// --- Read ---

func TestRead_ReturnsBlob(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	blob, err := p.Read(path)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if blob.SourcePath != path {
		t.Errorf("blob.SourcePath = %q, want %q", blob.SourcePath, path)
	}
	if blob.Size != 5 {
		t.Errorf("blob.Size = %d, want 5", blob.Size)
	}
}

func TestRead_NonExistent_ReturnsBlobThatDoesNotExist(t *testing.T) {
	p := Provider{}
	blob, err := p.Read("/nonexistent/file.txt")
	if err != nil {
		t.Fatalf("Read() error = %v, want nil for non-existent file", err)
	}
	if blob.Exists() {
		t.Error("blob.Exists() = true, want false for non-existent file")
	}
	if blob.SourcePath != "/nonexistent/file.txt" {
		t.Errorf("blob.SourcePath = %q, want %q", blob.SourcePath, "/nonexistent/file.txt")
	}
}

func TestRead_Directory_ReturnsError(t *testing.T) {
	tmp := t.TempDir()

	p := Provider{}
	_, err := p.Read(tmp)
	if err == nil {
		t.Fatal("Read() on directory should return error")
	}
}

// --- Glob ---

func TestGlob_MatchesFiles(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, "a.go", "package a")
	writeTestFile(t, tmp, "b.go", "package b")
	writeTestFile(t, tmp, "c.txt", "text")

	p := Provider{Root: tmp}
	matches, err := p.Glob(filepath.Join(tmp, "*.go"), false)
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("Glob() returned %d matches, want 2: %v", len(matches), matches)
	}
}

func TestGlob_NoMatches(t *testing.T) {
	tmp := t.TempDir()

	p := Provider{Root: tmp}
	matches, err := p.Glob(filepath.Join(tmp, "*.xyz"), false)
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}

	if len(matches) != 0 {
		t.Errorf("Glob() returned %d matches, want 0: %v", len(matches), matches)
	}
}

// --- Remove non-empty directory ---

func TestRemove_NonEmptyDirectory_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "mydir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "child.txt", "data")

	p := Provider{}
	_, _, err := p.Remove(dir, false, "")
	if err == nil {
		t.Fatal("Remove() on non-empty directory should return error")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Errorf("error = %q, want message containing 'not empty'", err)
	}
}

// --- Remove / RemoveAll / Unlink / Write round-trip tests ---
// These are blocked on issue #164 (recovery site fails on macOS SIP).

func TestRemove_RoundTrip(t *testing.T) {
	t.Skip("blocked on issue #164: recovery site creation fails on macOS SIP")
}

func TestRemoveAll_RoundTrip(t *testing.T) {
	t.Skip("blocked on issue #164: recovery site creation fails on macOS SIP")
}

func TestCompensateRemove_RoundTrip(t *testing.T) {
	t.Skip("blocked on issue #164: recovery site creation fails on macOS SIP")
}

func TestCompensateRemoveAll_RoundTrip(t *testing.T) {
	t.Skip("blocked on issue #164: recovery site creation fails on macOS SIP")
}

func TestCompensateUnlink_RoundTrip(t *testing.T) {
	t.Skip("blocked on issue #164: recovery site creation fails on macOS SIP")
}

func TestWriteText_OverwriteExisting_RoundTrip(t *testing.T) {
	t.Skip("blocked on issue #164: recovery site creation fails on macOS SIP")
}

// --- Backup + CompensateBackup round-trip ---

func TestBackup_CompensateBackup_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "myfile.txt")
	if err := os.WriteFile(path, []byte("original content"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	backupPath, state, err := p.Backup(path, ".bak")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	// Original should be gone, backup should exist.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("original file still exists after Backup")
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file does not exist: %v", err)
	}

	// Compensate: should restore original.
	if err := p.CompensateBackup(state); err != nil {
		t.Fatalf("CompensateBackup() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(original) error = %v", err)
	}
	if string(got) != "original content" {
		t.Errorf("restored content = %q, want %q", got, "original content")
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("backup file still exists after compensation")
	}
}

// --- Copy + CompensateCopy round-trip ---

func TestCopy_CompensateCopy_RoundTrip_NewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "new.txt")

	p := Provider{}
	blob := testFileResource(t, []byte("new content"))
	_, state, err := p.Copy(blob, path, 0o644)
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
	}

	// Compensate: file didn't exist before, so it should be removed.
	if err := p.CompensateCopy(state); err != nil {
		t.Fatalf("CompensateCopy() error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after compensating a new-file Copy")
	}
}

func TestCopy_CompensateCopy_RoundTrip_Overwrite(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "existing.txt")
	if err := os.WriteFile(path, []byte("original"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := Provider{Root: tmp}
	blob := testFileResource(t, []byte("replaced"))
	_, state, err := p.Copy(blob, path, 0o644)
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
	}

	// Compensate: should restore original content and mode.
	if err := p.CompensateCopy(state); err != nil {
		t.Fatalf("CompensateCopy() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "original" {
		t.Errorf("restored content = %q, want %q", got, "original")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("restored mode = %o, want %o", info.Mode().Perm(), 0o755)
	}
}

// --- Test Helpers ---

// testFileResource creates a Resource backed by a temp file with the given content.
func testFileResource(t *testing.T, content []byte) Resource {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "file-*")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		t.Fatalf("writing temp file: %v", err)
	}
	f.Close()
	fileResource, err := NewResource(f.Name())
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	return fileResource
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

// --- WalkTree ---

func TestWalkTree_BasicTraversal(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "main.go", "package main")
	mkdirAllTest(t, root, "src")
	writeTestFile(t, root, "src/app.go", "package src")

	p := Provider{}
	var walked []string
	_, stack, err := p.WalkTree(root, Actor(func(path string, entry os.DirEntry) error {
		walked = append(walked, path)
		return nil
	}), false)
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}
	if stack == nil {
		t.Fatal("WalkTree() stack is nil, want non-nil")
	}

	assertHas(t, walked, "main.go")
	assertHas(t, walked, "src")
	assertHas(t, walked, filepath.Join("src", "app.go"))
}

func TestWalkTree_GitignoreFiltering(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".gitignore", "*.log\nvendor/\n")
	writeTestFile(t, root, "main.go", "package main")
	writeTestFile(t, root, "debug.log", "some log")
	mkdirAllTest(t, root, "vendor")
	writeTestFile(t, root, "vendor/lib.go", "package vendor")

	p := Provider{}
	var walked []string
	_, _, err := p.WalkTree(root, Actor(func(path string, entry os.DirEntry) error {
		walked = append(walked, path)
		return nil
	}), true)
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	assertHas(t, walked, "main.go")
	assertNotHas(t, walked, "debug.log")
	assertNotHas(t, walked, "vendor")
	assertNotHas(t, walked, filepath.Join("vendor", "lib.go"))
}

func TestWalkTree_NestedGitignore(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, ".gitignore", "*.tmp\n")
	mkdirAllTest(t, root, "sub")
	writeTestFile(t, root, "sub/.gitignore", "*.bak\n")
	writeTestFile(t, root, "sub/keep.go", "package sub")
	writeTestFile(t, root, "sub/remove.tmp", "temp")
	writeTestFile(t, root, "sub/remove.bak", "backup")
	writeTestFile(t, root, "keep.go", "package main")
	writeTestFile(t, root, "remove.tmp", "temp")

	p := Provider{}
	var walked []string
	_, _, err := p.WalkTree(root, Actor(func(path string, entry os.DirEntry) error {
		if !entry.IsDir() {
			walked = append(walked, path)
		}
		return nil
	}), true)
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	assertHas(t, walked, "keep.go")
	assertHas(t, walked, filepath.Join("sub", "keep.go"))
	assertNotHas(t, walked, "remove.tmp")
	assertNotHas(t, walked, filepath.Join("sub", "remove.tmp"))
	assertNotHas(t, walked, filepath.Join("sub", "remove.bak"))
}

func TestWalkTree_SkipDir(t *testing.T) {
	root := t.TempDir()
	mkdirAllTest(t, root, "a")
	writeTestFile(t, root, "a/file.txt", "a")
	mkdirAllTest(t, root, "b")
	writeTestFile(t, root, "b/file.txt", "b")
	mkdirAllTest(t, root, "c")
	writeTestFile(t, root, "c/file.txt", "c")

	p := Provider{}
	var walked []string
	_, _, err := p.WalkTree(root, Actor(func(path string, entry os.DirEntry) error {
		walked = append(walked, path)
		if entry.IsDir() && path == "b" {
			return SkipDir
		}
		return nil
	}), false)
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	assertHas(t, walked, "b")
	assertNotHas(t, walked, filepath.Join("b", "file.txt"))
	assertHas(t, walked, filepath.Join("a", "file.txt"))
	assertHas(t, walked, filepath.Join("c", "file.txt"))
}

func TestWalkTree_SkipAll(t *testing.T) {
	root := t.TempDir()
	mkdirAllTest(t, root, "a")
	writeTestFile(t, root, "a/file.txt", "a")
	mkdirAllTest(t, root, "b")
	writeTestFile(t, root, "b/file.txt", "b")
	mkdirAllTest(t, root, "c")
	writeTestFile(t, root, "c/file.txt", "c")

	p := Provider{}
	var walked []string
	_, _, err := p.WalkTree(root, Actor(func(path string, entry os.DirEntry) error {
		walked = append(walked, path)
		if entry.IsDir() && path == "b" {
			return SkipAll
		}
		return nil
	}), false)
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	assertHas(t, walked, "a")
	assertHas(t, walked, "b")
	assertNotHas(t, walked, "c")
}

func TestWalkTree_SkipsGitDir(t *testing.T) {
	root := t.TempDir()
	mkdirAllTest(t, root, ".git/objects")
	writeTestFile(t, root, ".git/HEAD", "ref: refs/heads/main")
	writeTestFile(t, root, "file.go", "package main")

	p := Provider{}
	var walked []string
	_, _, err := p.WalkTree(root, Actor(func(path string, entry os.DirEntry) error {
		walked = append(walked, path)
		return nil
	}), false)
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	assertHas(t, walked, "file.go")
	assertNotHas(t, walked, ".git")
	assertNotHas(t, walked, filepath.Join(".git", "HEAD"))
}

func TestWalkTree_FoldAccumulates(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.txt", "hello")
	writeTestFile(t, root, "b.txt", "world")
	mkdirAllTest(t, root, "sub")
	writeTestFile(t, root, "sub/c.txt", "!")

	p := Provider{}
	result, _, err := p.WalkTree(root, func(result any, path string, entry os.DirEntry, stack *op.RecoveryStack) (any, error) {
		count := 0
		if result != nil {
			count = result.(int)
		}
		if !entry.IsDir() {
			count++
		}
		return count, nil
	}, false)
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	count, ok := result.(int)
	if !ok {
		t.Fatalf("result type = %T, want int", result)
	}
	if count != 3 {
		t.Errorf("file count = %d, want 3", count)
	}
}

func TestWalkTree_RecoveryStackIntegration(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.txt", "aaa")
	writeTestFile(t, root, "b.txt", "bbb")

	p := Provider{}
	var pushed []string
	_, stack, err := p.WalkTree(root, func(result any, path string, entry os.DirEntry, stack *op.RecoveryStack) (any, error) {
		if !entry.IsDir() {
			pushed = append(pushed, path)
			stack.Push(
				func(state any) error { return nil },
				nil,
				path,
				nil,
			)
		}
		return nil, nil
	}, false)
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	if len(pushed) != 2 {
		t.Fatalf("pushed %d entries, want 2", len(pushed))
	}
	if stack.Len() != 2 {
		t.Errorf("stack.Len() = %d, want 2", stack.Len())
	}
}

func TestWalkTree_ErrorUnwindsStack(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.txt", "aaa")
	mkdirAllTest(t, root, "sub")
	writeTestFile(t, root, "sub/b.txt", "bbb")

	p := Provider{}
	var compensated []string

	_, stack, err := p.WalkTree(root, func(result any, path string, entry os.DirEntry, stack *op.RecoveryStack) (any, error) {
		if entry.IsDir() {
			return nil, nil
		}
		stack.Push(
			func(state any) error {
				compensated = append(compensated, state.(string))
				return nil
			},
			nil,
			path,
			nil,
		)
		if path == filepath.Join("sub", "b.txt") {
			return nil, errors.New("deliberate failure")
		}
		return nil, nil
	}, false)

	if err == nil {
		t.Fatal("WalkTree() should return error on visitor failure")
	}
	if !strings.Contains(err.Error(), "deliberate failure") {
		t.Errorf("error = %q, want message containing 'deliberate failure'", err)
	}
	if stack == nil {
		t.Fatal("stack should be returned on error so the caller can decide on compensation")
	}
	if stack.Len() == 0 {
		t.Fatal("stack should still contain entries (no auto-unwind)")
	}

	// Caller decides to unwind.
	if err := stack.Unwind(); err != nil {
		t.Fatalf("Unwind() error = %v", err)
	}
	if len(compensated) == 0 {
		t.Fatal("expected compensations to be called after explicit unwind")
	}
}

func TestWalkTree_CompensateWalkTree(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "a.txt", "aaa")
	writeTestFile(t, root, "b.txt", "bbb")

	p := Provider{}
	var compensated []string

	_, stack, err := p.WalkTree(root, func(result any, path string, entry os.DirEntry, stack *op.RecoveryStack) (any, error) {
		if !entry.IsDir() {
			stack.Push(
				func(state any) error {
					compensated = append(compensated, state.(string))
					return nil
				},
				nil,
				path,
				nil,
			)
		}
		return nil, nil
	}, false)
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}
	if stack.Len() != 2 {
		t.Fatalf("stack.Len() = %d, want 2", stack.Len())
	}

	if err := p.CompensateWalkTree(stack); err != nil {
		t.Fatalf("CompensateWalkTree() error = %v", err)
	}

	if len(compensated) != 2 {
		t.Fatalf("expected 2 compensations, got %d", len(compensated))
	}

	// LIFO order: b.txt before a.txt
	if compensated[0] != "b.txt" || compensated[1] != "a.txt" {
		t.Errorf("compensation order = %v, want [b.txt, a.txt]", compensated)
	}
}

func TestWalkTree_CompensateNilStack(t *testing.T) {
	p := Provider{}
	if err := p.CompensateWalkTree(nil); err != nil {
		t.Errorf("CompensateWalkTree(nil) = %v, want nil", err)
	}
}

func TestWalkTree_DirsAndFiles(t *testing.T) {
	root := t.TempDir()
	mkdirAllTest(t, root, "a")
	writeTestFile(t, root, "a/file.txt", "hello")
	mkdirAllTest(t, root, "b")
	writeTestFile(t, root, "b/file.txt", "world")

	p := Provider{}
	var dirs, files []string
	_, _, err := p.WalkTree(root, Actor(func(path string, entry os.DirEntry) error {
		if entry.IsDir() {
			dirs = append(dirs, path)
		} else {
			files = append(files, path)
		}
		return nil
	}), false)
	if err != nil {
		t.Fatalf("WalkTree() error = %v", err)
	}

	if len(dirs) != 2 {
		t.Errorf("expected 2 directories, got %d: %v", len(dirs), dirs)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(files), files)
	}

	assertHas(t, dirs, "a")
	assertHas(t, dirs, "b")
	assertHas(t, files, filepath.Join("a", "file.txt"))
	assertHas(t, files, filepath.Join("b", "file.txt"))
}

// --- WalkTree test helpers ---

func writeTestFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdirAllTest(t *testing.T, root, relPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, relPath), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertHas(t *testing.T, items []string, want string) {
	t.Helper()
	for _, item := range items {
		if item == want {
			return
		}
	}
	t.Errorf("expected %v to contain %q", items, want)
}

func assertNotHas(t *testing.T, items []string, notWant string) {
	t.Helper()
	for _, item := range items {
		if item == notWant {
			t.Errorf("expected %v to NOT contain %q", items, notWant)
			return
		}
	}
}
