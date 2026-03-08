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

// testProvider creates a Provider rooted at the given directory.
func testProvider(t *testing.T, root string) Provider {
	t.Helper()
	rootResource := NewResource(root)
	if err := rootResource.Resolve(); err != nil {
		t.Fatalf("NewResource(%q).Resolve(): %v", root, err)
	}
	return Provider{Root: rootResource}
}

// --- Link ---

func TestLink_CreatesNewSymlink(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "target")
	if err := os.WriteFile(source, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmp, "link")

	p := Provider{}
	result, state, err := p.Link(Resource{SourcePath: source}, Resource{SourcePath: linkPath})
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if result.SourcePath != linkPath {
		t.Errorf("result = %q, want %q", result.SourcePath, linkPath)
	}

	// Nothing existed before — tombstone has resource but no recovery path.
	if state.Resource() == nil {
		t.Fatal("state.Resource() is nil, want non-nil")
	}
	if state.RecoveryPath != "" {
		t.Errorf("state.RecoveryPath = %q, want empty (nothing to recover)", state.RecoveryPath)
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

	p := testProvider(t, tmp)
	result, state, err := p.Link(Resource{SourcePath: newTarget}, Resource{SourcePath: linkPath})
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if result.SourcePath != linkPath {
		t.Errorf("result = %q, want %q", result.SourcePath, linkPath)
	}

	// Old symlink was moved to recovery.
	if state.RecoveryPath == "" {
		t.Error("state.RecoveryPath is empty, want non-empty (old symlink moved to recovery)")
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
	result, state, err := p.Link(Resource{SourcePath: source}, Resource{SourcePath: linkPath})
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if result.SourcePath != linkPath {
		t.Errorf("result = %q, want %q", result.SourcePath, linkPath)
	}
	if state != (Tombstone{}) {
		t.Errorf("state = %+v, want zero Tombstone (no-op)", state)
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
	_, _, err := p.Link(Resource{SourcePath: source}, Resource{SourcePath: linkPath})
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

func TestCompensateLink_ZeroState(t *testing.T) {
	p := Provider{}
	if err := p.CompensateLink(Tombstone{}); err != nil {
		t.Errorf("CompensateLink(zero) = %v, want nil", err)
	}
}

func TestCompensateLink_NewSymlink_RemovesOnCompensate(t *testing.T) {
	tmp := t.TempDir()
	linkPath := filepath.Join(tmp, "link")
	if err := os.Symlink("/some/target", linkPath); err != nil {
		t.Fatal(err)
	}

	// Tombstone with no recovery path — symlink didn't exist before.
	resource := Resource{SourcePath: linkPath}
	state := Tombstone{
		TombstoneBase: op.NewTombstoneBase(&resource),
	}

	p := Provider{}
	if err := p.CompensateLink(state); err != nil {
		t.Fatalf("CompensateLink() error = %v", err)
	}
	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("symlink still exists after compensation")
	}
}

func TestCompensateLink_ExistedBefore_RestoresFromRecovery(t *testing.T) {
	tmp := t.TempDir()
	linkPath := filepath.Join(tmp, "link")
	oldTarget := filepath.Join(tmp, "old-target")
	recoveryPath := filepath.Join(tmp, "recovery-link")

	// Simulate: old symlink was moved to recovery, new symlink created.
	if err := os.Symlink(oldTarget, recoveryPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/some/new-target", linkPath); err != nil {
		t.Fatal(err)
	}

	// Resource preserves true identity (linkPath). RecoveryPath = temporary location.
	resource := Resource{SourcePath: linkPath}
	state := Tombstone{
		TombstoneBase: op.NewTombstoneBase(&resource),
		RecoveryPath:  recoveryPath,
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

	if _, err := os.Stat(recoveryPath); !os.IsNotExist(err) {
		t.Error("recovery file still exists after compensation")
	}
}

// --- Copy ---

func TestCopy_WritesNewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")
	fileResource := testFileResource(t, []byte("hello world"))

	p := Provider{}
	result, _, err := p.Copy(fileResource, Resource{SourcePath: path}, 0o600)
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

	p := testProvider(t, tmp)
	blob := testFileResource(t, []byte("replaced"))
	_, _, err := p.Copy(blob, Resource{SourcePath: path}, 0o644)
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
	_, _, err := p.Copy(blob, Resource{SourcePath: path}, 0)
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

func TestCompensateCopy_ZeroState_NoPanic(t *testing.T) {
	p := Provider{}
	if err := p.CompensateCopy(Tombstone{}); err != nil {
		t.Errorf("CompensateCopy(zero) = %v, want nil", err)
	}
}

func TestCompensateCopy_NewFile_RemovesOnCompensate(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")
	if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Tombstone with no recovery path = file didn't exist before, just remove it.
	resource := Resource{SourcePath: path}
	state := Tombstone{
		TombstoneBase: op.NewTombstoneBase(&resource),
	}

	p := Provider{}
	if err := p.CompensateCopy(state); err != nil {
		t.Fatalf("CompensateCopy() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after compensation")
	}
}

func TestCompensateCopy_Overwrite_RestoresOriginal(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")
	recoveryPath := filepath.Join(tmp, "output.txt.recovery")

	// Simulate: original was moved to recovery, then overwritten.
	if err := os.WriteFile(recoveryPath, []byte("original content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replaced content"), 0o644); err != nil {
		t.Fatal(err)
	}

	resource := Resource{SourcePath: path}
	state := Tombstone{
		TombstoneBase: op.NewTombstoneBase(&resource),
		RecoveryPath:  recoveryPath,
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

	if _, err := os.Stat(recoveryPath); !os.IsNotExist(err) {
		t.Error("recovery file still exists after compensation")
	}
}

// --- Backup ---

func TestBackup_MovesFileToTimestampedBackup(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "myfile.txt")
	if err := os.WriteFile(path, []byte("backup me"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := NewResource(path)
	if err := res.Resolve(); err != nil {
		t.Fatalf("NewResource().Resolve() error = %v", err)
	}

	p := Provider{}
	result, state, err := p.Backup(res, ".bak")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	if !strings.HasPrefix(result.SourcePath, path+".bak.") {
		t.Errorf("backup path = %q, want prefix %q", result.SourcePath, path+".bak.")
	}

	// Original should be gone.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("original file still exists after backup")
	}

	// Backup should exist with correct content.
	got, err := os.ReadFile(result.SourcePath)
	if err != nil {
		t.Fatalf("ReadFile(backup) error = %v", err)
	}
	if string(got) != "backup me" {
		t.Errorf("backup content = %q, want %q", got, "backup me")
	}

	// Tombstone resource preserves true identity (original path).
	// RecoveryPath records where data was moved to (backup location).
	resourcePath := state.Resource().(*Resource).SourcePath
	if resourcePath != path {
		t.Errorf("tombstone resource path = %q, want %q (true identity)", resourcePath, path)
	}
	if state.RecoveryPath != result.SourcePath {
		t.Errorf("tombstone recovery path = %q, want %q", state.RecoveryPath, result.SourcePath)
	}

	// Checksum should match the original file content.
	h := sha256.Sum256([]byte("backup me"))
	wantChecksum := "sha256:" + hex.EncodeToString(h[:])
	resourceChecksum := state.Resource().(*Resource).Checksum
	if resourceChecksum != wantChecksum {
		t.Errorf("resource checksum = %q, want %q", resourceChecksum, wantChecksum)
	}
}

func TestBackup_DefaultSuffix(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "myfile.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	result, _, err := p.Backup(Resource{SourcePath: path}, "")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	if !strings.HasPrefix(result.SourcePath, path+".devlore-backup.") {
		t.Errorf("backup path = %q, want prefix %q (default suffix)", result.SourcePath, path+".devlore-backup.")
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

	resource := Resource{SourcePath: originalPath}
	state := Tombstone{
		TombstoneBase: op.NewTombstoneBase(&resource),
		RecoveryPath:  backupPath,
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

	resource := Resource{SourcePath: originalPath, Checksum: wrongChecksum}
	state := Tombstone{
		TombstoneBase: op.NewTombstoneBase(&resource),
		RecoveryPath:  backupPath,
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
	result, _, err := p.Unlink(Resource{SourcePath: linkPath}, false, Resource{})
	if err != nil {
		t.Fatalf("Unlink() error = %v", err)
	}
	if result.Resource() == nil {
		t.Fatal("result.Resource() is nil, want non-nil")
	}

	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("symlink still exists after unlink")
	}
}

func TestUnlink_AlreadyGone(t *testing.T) {
	tmp := t.TempDir()
	linkPath := filepath.Join(tmp, "nonexistent")

	p := Provider{}
	result, state, err := p.Unlink(Resource{SourcePath: linkPath}, false, Resource{})
	if err != nil {
		t.Fatalf("Unlink() error = %v", err)
	}
	if result != (Tombstone{}) {
		t.Errorf("result = %+v, want empty Tombstone", result)
	}
	if state != (Tombstone{}) {
		t.Errorf("state = %+v, want empty Tombstone (no-op)", state)
	}
}

func TestUnlink_NotASymlink_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "regular-file")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	_, _, err := p.Unlink(Resource{SourcePath: path}, false, Resource{})
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
	result, _, err := p.Remove(Resource{SourcePath: path}, false, Resource{})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if result.Resource() == nil {
		t.Fatal("result.Resource() is nil, want non-nil")
	}
	if result.RecoveryPath == "" {
		t.Error("result.RecoveryPath should not be empty")
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after remove")
	}
}

func TestRemove_AlreadyGone(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nonexistent")

	p := Provider{}
	result, state, err := p.Remove(Resource{SourcePath: path}, false, Resource{})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if result != (Tombstone{}) {
		t.Errorf("result = %+v, want empty Tombstone", result)
	}
	if state != (Tombstone{}) {
		t.Errorf("state = %+v, want empty Tombstone (no-op)", state)
	}
}

// --- Write ---

func TestWriteText_WritesContentToNewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")

	p := Provider{}
	result, state, err := p.WriteText(Resource{SourcePath: path}, "hello world", 0o644)
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}
	if result.SourcePath != path {
		t.Errorf("result.SourcePath = %q, want %q", result.SourcePath, path)
	}

	if state.Resource() == nil {
		t.Fatal("state.Resource() is nil, want non-nil")
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
	result, state, err := p.WriteBytes(Resource{SourcePath: path}, "binary data", 0o600)
	if err != nil {
		t.Fatalf("WriteBytes() error = %v", err)
	}
	if result.SourcePath != path {
		t.Errorf("result.SourcePath = %q, want %q", result.SourcePath, path)
	}

	if state.Resource() == nil {
		t.Fatal("state.Resource() is nil, want non-nil")
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

	srcRes := NewResource(src)
	if resErr := srcRes.Resolve(); resErr != nil {
		t.Fatalf("NewResource().Resolve() error = %v", resErr)
	}

	p := Provider{}
	result, state, err := p.Move(srcRes, Resource{SourcePath: dst})
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if result.SourcePath != dst {
		t.Errorf("result = %q, want %q", result.SourcePath, dst)
	}

	// Tombstone resource preserves true identity (source path).
	// RecoveryPath records where data was moved to (destination).
	resourcePath := state.Resource().(*Resource).SourcePath
	if resourcePath != src {
		t.Errorf("tombstone resource path = %q, want %q (true identity)", resourcePath, src)
	}
	if state.RecoveryPath != dst {
		t.Errorf("tombstone recovery path = %q, want %q", state.RecoveryPath, dst)
	}

	// Checksum should match the original file content.
	h := sha256.Sum256([]byte("move me"))
	wantChecksum := "sha256:" + hex.EncodeToString(h[:])
	resourceChecksum := state.Resource().(*Resource).Checksum
	if resourceChecksum != wantChecksum {
		t.Errorf("resource checksum = %q, want %q", resourceChecksum, wantChecksum)
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

func TestCompensateMove_ZeroState(t *testing.T) {
	p := Provider{}
	if err := p.CompensateMove(Tombstone{}); err != nil {
		t.Errorf("CompensateMove(zero) = %v, want nil", err)
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

	resource := Resource{SourcePath: src, Checksum: wrongChecksum}
	state := Tombstone{
		TombstoneBase: op.NewTombstoneBase(&resource),
		RecoveryPath:  dst,
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

	srcRes := NewResource(src)
	if resErr := srcRes.Resolve(); resErr != nil {
		t.Fatalf("NewResource().Resolve() error = %v", resErr)
	}

	p := Provider{}
	_, state, err := p.Move(srcRes, Resource{SourcePath: dst})
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

func TestCompensateWriteText_ZeroState(t *testing.T) {
	p := Provider{}
	if err := p.CompensateWriteText(Tombstone{}); err != nil {
		t.Errorf("CompensateWriteText(zero) = %v, want nil", err)
	}
}

func TestCompensateWriteBytes_ZeroState(t *testing.T) {
	p := Provider{}
	if err := p.CompensateWriteBytes(Tombstone{}); err != nil {
		t.Errorf("CompensateWriteBytes(zero) = %v, want nil", err)
	}
}

func TestWriteText_DefaultModeWhenZero(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "default-mode.txt")

	p := Provider{}
	_, _, err := p.WriteText(Resource{SourcePath: path}, "content", 0)
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
	_, _, err := p.WriteBytes(Resource{SourcePath: path}, "content", 0)
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
	result, _, err := p.WriteText(Resource{SourcePath: path}, "nested content", 0o644)
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
	_, state, err := p.WriteText(Resource{SourcePath: path}, "to be undone", 0o644)
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
	_, state, err := p.WriteBytes(Resource{SourcePath: path}, "to be undone", 0o600)
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
	got, err := p.Exists(Resource{SourcePath: path})
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("Exists() = false, want true for existing file")
	}
}

func TestExists_FileDoesNotExist(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nonexistent.txt")

	p := Provider{}
	got, err := p.Exists(Resource{SourcePath: path})
	if err != nil {
		t.Fatal(err)
	}
	if got {
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
	got, err := p.Exists(Resource{SourcePath: link})
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("Exists() = false, want true for symlink")
	}
}

func TestExists_Directory(t *testing.T) {
	tmp := t.TempDir()

	p := Provider{}
	got, err := p.Exists(Resource{SourcePath: tmp})
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("Exists() = false, want true for existing directory")
	}
}

// --- IsDir ---

func TestIsDir_Directory(t *testing.T) {
	tmp := t.TempDir()

	p := Provider{}
	got, err := p.IsDir(Resource{SourcePath: tmp})
	if err != nil {
		t.Fatal(err)
	}
	if !got {
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
	got, err := p.IsDir(Resource{SourcePath: path})
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("IsDir() = true, want false for regular file")
	}
}

func TestIsDir_NonExistent(t *testing.T) {
	p := Provider{}
	got, err := p.IsDir(Resource{SourcePath: "/nonexistent/path"})
	if err != nil {
		t.Fatal(err)
	}
	if got {
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
	got, err := p.IsFile(Resource{SourcePath: path})
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("IsFile() = false, want true for regular file")
	}
}

func TestIsFile_Directory(t *testing.T) {
	tmp := t.TempDir()

	p := Provider{}
	got, err := p.IsFile(Resource{SourcePath: tmp})
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("IsFile() = true, want false for directory")
	}
}

func TestIsFile_NonExistent(t *testing.T) {
	p := Provider{}
	got, err := p.IsFile(Resource{SourcePath: "/nonexistent/path"})
	if err != nil {
		t.Fatal(err)
	}
	if got {
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
	got, err := p.IsFile(Resource{SourcePath: link})
	if err != nil {
		t.Fatal(err)
	}
	if !got {
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
	result, err := p.Mkdir(Resource{SourcePath: path}, 0o755)
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if result.SourcePath != path {
		t.Errorf("result.SourcePath = %q, want %q", result.SourcePath, path)
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
	_, err := p.Mkdir(Resource{SourcePath: path}, 0o755)
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
	_, err := p.Mkdir(Resource{SourcePath: path}, 0o755)
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
	blob, err := p.Read(Resource{SourcePath: path})
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
	blob, err := p.Read(Resource{SourcePath: "/nonexistent/file.txt"})
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

func TestRead_Directory_ReturnsResource(t *testing.T) {
	tmp := t.TempDir()

	p := Provider{}
	result, err := p.Read(Resource{SourcePath: tmp})
	if err != nil {
		t.Fatalf("Read() on directory error = %v", err)
	}
	if result.SourcePath != tmp {
		t.Errorf("result.SourcePath = %q, want %q", result.SourcePath, tmp)
	}
}

// --- Glob ---

func TestGlob_MatchesFiles(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, "a.go", "package a")
	writeTestFile(t, tmp, "b.go", "package b")
	writeTestFile(t, tmp, "c.txt", "text")

	p := testProvider(t, tmp)
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

	p := testProvider(t, tmp)
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
	_, _, err := p.Remove(Resource{SourcePath: dir}, false, Resource{})
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
	tmp := t.TempDir()
	path := filepath.Join(tmp, "remove-rt.txt")
	if err := os.WriteFile(path, []byte("remove round-trip"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	_, state, err := p.Remove(Resource{SourcePath: path}, false, Resource{})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after Remove")
	}

	if err := p.CompensateRemove(state); err != nil {
		t.Fatalf("CompensateRemove() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v — file should be restored", err)
	}
	if string(got) != "remove round-trip" {
		t.Errorf("restored content = %q, want %q", got, "remove round-trip")
	}
}

func TestRemoveAll_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "removedir-rt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, dir, "child.txt", "child content")

	p := Provider{}
	_, state, err := p.RemoveAll(Resource{SourcePath: dir}, false, Resource{})
	if err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("directory still exists after RemoveAll")
	}

	if err := p.CompensateRemoveAll(state); err != nil {
		t.Fatalf("CompensateRemoveAll() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "child.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v — child should be restored", err)
	}
	if string(got) != "child content" {
		t.Errorf("restored child content = %q, want %q", got, "child content")
	}
}

func TestCompensateRemove_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "comp-remove.txt")
	if err := os.WriteFile(path, []byte("compensate me"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	_, state, err := p.Remove(Resource{SourcePath: path}, false, Resource{})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Tombstone preserves true identity — SourcePath is the original home.
	if state.Resource().(*Resource).SourcePath != path {
		t.Errorf("tombstone resource path = %q, want %q (true identity)", state.Resource().(*Resource).SourcePath, path)
	}

	// Verify recovery site holds the data.
	recoveryPath := state.RecoveryPath
	if _, err := os.Stat(recoveryPath); err != nil {
		t.Fatalf("recovery site missing: %v", err)
	}

	if err := p.CompensateRemove(state); err != nil {
		t.Fatalf("CompensateRemove() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v — file should be restored", err)
	}
	if string(got) != "compensate me" {
		t.Errorf("restored content = %q, want %q", got, "compensate me")
	}

	// Recovery site should be gone after restoration.
	if _, err := os.Stat(recoveryPath); !os.IsNotExist(err) {
		t.Error("recovery site still exists after compensation")
	}
}

func TestCompensateRemoveAll_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "comp-removedir")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "sub"), "nested.txt", "nested data")

	p := Provider{}
	_, state, err := p.RemoveAll(Resource{SourcePath: dir}, false, Resource{})
	if err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	// Tombstone preserves true identity — SourcePath is the original home.
	if state.Resource().(*Resource).SourcePath != dir {
		t.Errorf("tombstone resource path = %q, want %q (true identity)", state.Resource().(*Resource).SourcePath, dir)
	}

	recoveryPath := state.RecoveryPath
	if _, err := os.Stat(recoveryPath); err != nil {
		t.Fatalf("recovery site missing: %v", err)
	}

	if err := p.CompensateRemoveAll(state); err != nil {
		t.Fatalf("CompensateRemoveAll() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "sub", "nested.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v — nested file should be restored", err)
	}
	if string(got) != "nested data" {
		t.Errorf("restored content = %q, want %q", got, "nested data")
	}

	if _, err := os.Stat(recoveryPath); !os.IsNotExist(err) {
		t.Error("recovery site still exists after compensation")
	}
}

func TestCompensateUnlink_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target.txt")
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmp, "comp-unlink.txt")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	_, state, err := p.Unlink(Resource{SourcePath: linkPath}, false, Resource{})
	if err != nil {
		t.Fatalf("Unlink() error = %v", err)
	}

	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("symlink still exists after Unlink")
	}

	if err := p.CompensateUnlink(state); err != nil {
		t.Fatalf("CompensateUnlink() error = %v", err)
	}

	resolved, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v — symlink should be restored", err)
	}
	if resolved != target {
		t.Errorf("restored symlink target = %q, want %q", resolved, target)
	}
}

func TestWriteText_OverwriteExisting_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "overwrite-rt.txt")
	if err := os.WriteFile(path, []byte("original content"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	_, state, err := p.WriteText(Resource{SourcePath: path}, "replaced content", 0o644)
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}

	// Verify the overwrite happened.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "replaced content" {
		t.Errorf("overwritten content = %q, want %q", got, "replaced content")
	}

	// Compensate: should restore the original.
	if err := p.CompensateWriteText(state); err != nil {
		t.Fatalf("CompensateWriteText() error = %v", err)
	}

	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v — file should be restored", err)
	}
	if string(got) != "original content" {
		t.Errorf("restored content = %q, want %q", got, "original content")
	}
}

// --- Backup + CompensateBackup round-trip ---

func TestBackup_CompensateBackup_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "myfile.txt")
	if err := os.WriteFile(path, []byte("original content"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := Provider{}
	result, state, err := p.Backup(Resource{SourcePath: path}, ".bak")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	// Original should be gone, backup should exist.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("original file still exists after Backup")
	}
	if _, err := os.Stat(result.SourcePath); err != nil {
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
	if _, err := os.Stat(result.SourcePath); !os.IsNotExist(err) {
		t.Error("backup file still exists after compensation")
	}
}

// --- Copy + CompensateCopy round-trip ---

func TestCopy_CompensateCopy_RoundTrip_NewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "new.txt")

	p := Provider{}
	blob := testFileResource(t, []byte("new content"))
	_, state, err := p.Copy(blob, Resource{SourcePath: path}, 0o644)
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

	p := testProvider(t, tmp)
	blob := testFileResource(t, []byte("replaced"))
	_, state, err := p.Copy(blob, Resource{SourcePath: path}, 0o644)
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
	}

	// Compensation restores the original file from recovery.
	if err := p.CompensateCopy(state); err != nil {
		t.Fatalf("CompensateCopy() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v — file should be restored", err)
	}
	if string(got) != "original" {
		t.Errorf("restored content = %q, want %q", got, "original")
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
	fileResource := NewResource(f.Name())
	if err := fileResource.Resolve(); err != nil {
		t.Fatalf("NewResource.Resolve: %v", err)
	}
	return fileResource
}

// --- Helpers ---

func TestIsSubpath(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		root   string
		expect bool
	}{
		{"exact match", "/foo/bar", "/foo/bar", false},
		{"child", "/foo/bar/baz", "/foo/bar", true},
		{"sibling", "/foo/baz", "/foo/bar", false},
		{"parent", "/foo", "/foo/bar", false},
		{"root slash", "/foo/bar", "/", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSubpath(tt.path, tt.root); got != tt.expect {
				t.Errorf("isSubpath(%q, %q) = %v, want %v", tt.path, tt.root, got, tt.expect)
			}
		})
	}
}

func TestChecksumFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "checksum.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := checksumFile(path)
	if got == "" {
		t.Fatal("checksumFile() returned empty string")
	}

	h := sha256.Sum256([]byte("hello"))
	want := "sha256:" + hex.EncodeToString(h[:])
	if got != want {
		t.Errorf("checksumFile() = %q, want %q", got, want)
	}
}

func TestChecksumFile_NonExistent(t *testing.T) {
	got := checksumFile("/nonexistent/file.txt")
	if got != "" {
		t.Errorf("checksumFile(nonexistent) = %q, want empty string", got)
	}
}

func TestIsDirAndNotEmpty(t *testing.T) {
	tmp := t.TempDir()

	// Empty directory
	emptyDir := filepath.Join(tmp, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	isNonEmpty, err := isDirAndNotEmpty(emptyDir)
	if err != nil {
		t.Fatalf("isDirAndNotEmpty(empty) error = %v", err)
	}
	if isNonEmpty {
		t.Error("isDirAndNotEmpty(empty) = true, want false")
	}

	// Non-empty directory
	nonEmptyDir := filepath.Join(tmp, "notempty")
	if err := os.Mkdir(nonEmptyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, nonEmptyDir, "child.txt", "data")
	isNonEmpty, err = isDirAndNotEmpty(nonEmptyDir)
	if err != nil {
		t.Fatalf("isDirAndNotEmpty(notempty) error = %v", err)
	}
	if !isNonEmpty {
		t.Error("isDirAndNotEmpty(notempty) = false, want true")
	}

	// Regular file
	filePath := filepath.Join(tmp, "file.txt")
	writeTestFile(t, tmp, "file.txt", "data")
	isNonEmpty, err = isDirAndNotEmpty(filePath)
	if err != nil {
		t.Fatalf("isDirAndNotEmpty(file) error = %v", err)
	}
	if isNonEmpty {
		t.Error("isDirAndNotEmpty(file) = true, want false for regular file")
	}

	// Nonexistent
	_, err = isDirAndNotEmpty(filepath.Join(tmp, "no-such-thing"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("isDirAndNotEmpty(nonexistent) error = %v, want os.ErrNotExist", err)
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
