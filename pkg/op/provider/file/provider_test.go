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

// testRoot creates an unconfined read-write Root for test I/O.
func testRoot(t *testing.T, dir string) op.Root {
	t.Helper()
	return op.NewRootReaderWriter(dir)
}

// testProvider creates a Provider rooted at the given directory.
func testProvider(t *testing.T, dir string) Provider {
	t.Helper()
	root := op.NewRootReaderWriter(dir)
	ctx := &op.ExecutionContext{Root: root, Catalog: op.NewResourceCatalog()}
	ctx.RecoverySite = op.NewRecoverySite(ctx)
	return Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// --- Link ---

func TestLink_CreatesNewSymlink(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "target")
	if err := os.WriteFile(source, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmp, "link")

	p := testProvider(t, tmp)
	result, state, err := p.Link(&Resource{SourcePath: op.NewPath("", source)}, linkPath)
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if result.SourcePath.Abs() != linkPath {
		t.Errorf("result = %q, want %q", result.SourcePath.Abs(), linkPath)
	}

	// Nothing existed before — receipt has resource but no recovery path.
	if state.Resource() == nil {
		t.Fatal("state.Resource() is nil, want non-nil")
	}
	if state.TransactionID() != "" {
		t.Errorf("state.TransactionID() = %q, want empty (nothing to recover)", state.TransactionID())
	}

	got := resolveReadlink(t, linkPath)
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
	result, state, err := p.Link(&Resource{SourcePath: op.NewPath("", newTarget)}, linkPath)
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if result.SourcePath.Abs() != linkPath {
		t.Errorf("result = %q, want %q", result.SourcePath.Abs(), linkPath)
	}

	// Old symlink was moved to recovery.
	if state.TransactionID() == "" {
		t.Error("state.TransactionID() is empty, want non-empty (old symlink moved to recovery)")
	}

	got := resolveReadlink(t, linkPath)
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

	p := testProvider(t, tmp)
	result, state, err := p.Link(&Resource{SourcePath: op.NewPath("", source)}, linkPath)
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if result.SourcePath.Abs() != linkPath {
		t.Errorf("result = %q, want %q", result.SourcePath.Abs(), linkPath)
	}
	if state != (nil) {
		t.Errorf("state = %+v, want zero Receipt (no-op)", state)
	}
}

func TestLink_CreatesParentDirectories(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "target")
	if err := os.WriteFile(source, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmp, "deep", "nested", "link")

	p := testProvider(t, tmp)
	_, _, err := p.Link(&Resource{SourcePath: op.NewPath("", source)}, linkPath)
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}

	got := resolveReadlink(t, linkPath)
	if got != source {
		t.Errorf("symlink target = %q, want %q", got, source)
	}
}

// --- CompensateLink ---

func TestCompensateLink_ZeroState(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)
	if err := p.CompensateLink(nil); err != nil {
		t.Errorf("CompensateLink(zero) = %v, want nil", err)
	}
}

func TestCompensateLink_NewSymlink_RemovesOnCompensate(t *testing.T) {
	tmp := t.TempDir()
	linkPath := filepath.Join(tmp, "link")
	if err := os.Symlink("/some/target", linkPath); err != nil {
		t.Fatal(err)
	}

	// Receipt with no recovery path — symlink didn't exist before.
	resource := &Resource{SourcePath: op.NewPath("", linkPath)}
	state := &Receipt{
		ReceiptBase: op.NewReceiptBase(resource),
	}

	p := testProvider(t, tmp)
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

	// Simulate: old symlink was moved to recovery, new symlink created.
	// TransactionID is the recovery key (as returned by RecoverySite.ArchiveFile).
	recoveryRel := "recovery-link"
	if err := os.Symlink(oldTarget, filepath.Join(tmp, recoveryRel)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/some/new-target", linkPath); err != nil {
		t.Fatal(err)
	}

	// Resource preserves true identity (linkPath); TransactionID is the recovery key.
	resource := &Resource{SourcePath: op.NewPath("", linkPath)}
	state := &Receipt{
		ReceiptBase: op.NewReceiptBase(resource),
	}

	p := testProvider(t, tmp)
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

	if _, err := os.Stat(filepath.Join(tmp, recoveryRel)); !os.IsNotExist(err) {
		t.Error("recovery file still exists after compensation")
	}
}

// --- Copy ---

func TestCopy_WritesNewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")
	fileResource := testFileResource(t, []byte("hello world"))

	p := testProvider(t, tmp)
	result, _, err := p.Copy(fileResource, path, 0o600)
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
	}

	if result.SourcePath.Abs() != path {
		t.Errorf("result.SourcePath.Abs() = %q, want %q", result.SourcePath.Abs(), path)
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

// --- CompensateCopy ---

func TestCompensateCopy_ZeroState_NoPanic(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)
	if err := p.CompensateCopy(nil); err != nil {
		t.Errorf("CompensateCopy(zero) = %v, want nil", err)
	}
}

func TestCompensateCopy_NewFile_RemovesOnCompensate(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")
	if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Receipt with no recovery path = file didn't exist before, just remove it.
	resource := &Resource{SourcePath: op.NewPath("", path)}
	state := &Receipt{
		ReceiptBase: op.NewReceiptBase(resource),
	}

	p := testProvider(t, tmp)
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

	// Simulate: original was moved to recovery, then overwritten.
	// TransactionID is the recovery key (as returned by RecoverySite.ArchiveFile).
	recoveryRel := "output.txt.recovery"
	if err := os.WriteFile(filepath.Join(tmp, recoveryRel), []byte("original content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replaced content"), 0o644); err != nil {
		t.Fatal(err)
	}

	resource := &Resource{SourcePath: op.NewPath("", path)}
	state := &Receipt{
		ReceiptBase: op.NewReceiptBase(resource),
	}

	p := testProvider(t, tmp)
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

	if _, err := os.Stat(filepath.Join(tmp, recoveryRel)); !os.IsNotExist(err) {
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

	p := testProvider(t, tmp)
	res, err := NewResource(p.ExecutionContext(), path)
	if err != nil {
		t.Fatalf("NewResource error = %v", err)
	}
	if err := res.Resolve(); err != nil {
		t.Fatalf("Resolve error = %v", err)
	}
	result, state, err := p.Backup(res, ".bak")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	if !strings.HasPrefix(result.SourcePath.Abs(), path+".bak.") {
		t.Errorf("backup path = %q, want prefix %q", result.SourcePath.Abs(), path+".bak.")
	}

	// Original should be gone.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("original file still exists after backup")
	}

	// Backup should exist with correct content.
	got, err := os.ReadFile(result.SourcePath.Abs())
	if err != nil {
		t.Fatalf("ReadFile(backup) error = %v", err)
	}
	if string(got) != "backup me" {
		t.Errorf("backup content = %q, want %q", got, "backup me")
	}

	// Receipt resource preserves true identity (original path).
	// TransactionID is the recovery key for the moved data (backup location).
	resourcePath := state.Resource().(*Resource).SourcePath.Abs()
	if resourcePath != path {
		t.Errorf("receipt resource path = %q, want %q (true identity)", resourcePath, path)
	}
	if state.TransactionID() != result.SourcePath.Abs() {
		t.Errorf("receipt recovery path = %q, want %q", state.TransactionID(), result.SourcePath.Abs())
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

	p := testProvider(t, tmp)
	result, _, err := p.Backup(&Resource{SourcePath: op.NewPath("", path)}, "")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	if !strings.HasPrefix(result.SourcePath.Abs(), path+".devlore-backup.") {
		t.Errorf("backup path = %q, want prefix %q (default suffix)", result.SourcePath.Abs(), path+".devlore-backup.")
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

	resource := &Resource{SourcePath: op.NewPath("", originalPath)}
	state := &Receipt{
		ReceiptBase: op.NewReceiptBase(resource),
	}

	p := testProvider(t, tmp)
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

	resource := &Resource{SourcePath: op.NewPath("", originalPath), Checksum: wrongChecksum}
	state := &Receipt{
		ReceiptBase: op.NewReceiptBase(resource),
	}

	p := testProvider(t, tmp)
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

	p := testProvider(t, tmp)
	_, receipt, err := p.Unlink(&Resource{SourcePath: op.NewPath("", linkPath)}, false, &Resource{})
	if err != nil {
		t.Fatalf("Unlink() error = %v", err)
	}
	if receipt.Resource() == nil {
		t.Fatal("receipt.Resource() is nil, want non-nil")
	}

	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("symlink still exists after unlink")
	}
}

func TestUnlink_AlreadyGone(t *testing.T) {
	tmp := t.TempDir()
	linkPath := filepath.Join(tmp, "nonexistent")

	p := testProvider(t, tmp)
	product, receipt, err := p.Unlink(&Resource{SourcePath: op.NewPath("", linkPath)}, false, &Resource{})
	if err != nil {
		t.Fatalf("Unlink() error = %v", err)
	}
	if product != nil {
		t.Errorf("product = %+v, want nil for already-gone", product)
	}
	if receipt != (nil) {
		t.Errorf("receipt = %+v, want empty Receipt (no-op)", receipt)
	}
}

func TestUnlink_NotASymlink_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "regular-file")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	_, _, err := p.Unlink(&Resource{SourcePath: op.NewPath("", path)}, false, &Resource{})
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

	p := testProvider(t, tmp)
	_, receipt, err := p.Remove(&Resource{SourcePath: op.NewPath("", path)}, false, &Resource{})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if receipt.Resource() == nil {
		t.Fatal("receipt.Resource() is nil, want non-nil")
	}
	if receipt.TransactionID() == "" {
		t.Error("receipt.TransactionID() should not be empty")
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after remove")
	}
}

func TestRemove_AlreadyGone(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nonexistent")

	p := testProvider(t, tmp)
	product, receipt, err := p.Remove(&Resource{SourcePath: op.NewPath("", path)}, false, &Resource{})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if product != nil {
		t.Errorf("product = %+v, want nil for already-gone", product)
	}
	if receipt != (nil) {
		t.Errorf("receipt = %+v, want empty Receipt (no-op)", receipt)
	}
}

// --- Write ---

func TestWriteText_WritesContentToNewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "output.txt")

	p := testProvider(t, tmp)
	result, state, err := p.WriteText(path, "hello world", 0o644)
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}
	if result.SourcePath.Abs() != path {
		t.Errorf("result.SourcePath.Abs() = %q, want %q", result.SourcePath.Abs(), path)
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

	p := testProvider(t, tmp)
	result, state, err := p.WriteBytes(path, "binary data", 0o600)
	if err != nil {
		t.Fatalf("WriteBytes() error = %v", err)
	}
	if result.SourcePath.Abs() != path {
		t.Errorf("result.SourcePath.Abs() = %q, want %q", result.SourcePath.Abs(), path)
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

	p := testProvider(t, tmp)
	srcRes, resErr := NewResource(p.ExecutionContext(), src)
	if resErr != nil {
		t.Fatalf("NewResource error = %v", resErr)
	}
	if resErr = srcRes.Resolve(); resErr != nil {
		t.Fatalf("Resolve error = %v", resErr)
	}

	result, state, err := p.Move(srcRes, dst)
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	if result.SourcePath.Abs() != dst {
		t.Errorf("result = %q, want %q", result.SourcePath.Abs(), dst)
	}

	// Receipt resource preserves true identity (source path).
	// TransactionID is the recovery key for the moved data (destination).
	resourcePath := state.Resource().(*Resource).SourcePath.Abs()
	if resourcePath != src {
		t.Errorf("receipt resource path = %q, want %q (true identity)", resourcePath, src)
	}
	if state.TransactionID() != dst {
		t.Errorf("receipt recovery path = %q, want %q", state.TransactionID(), dst)
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
	tmp := t.TempDir()
	p := testProvider(t, tmp)
	if err := p.CompensateMove(nil); err != nil {
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

	resource := &Resource{SourcePath: op.NewPath("", src), Checksum: wrongChecksum}
	state := &Receipt{
		ReceiptBase: op.NewReceiptBase(resource),
	}

	p := testProvider(t, tmp)
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

	p := testProvider(t, tmp)
	srcRes, resErr := NewResource(p.ExecutionContext(), src)
	if resErr != nil {
		t.Fatalf("NewResource error = %v", resErr)
	}
	if resErr = srcRes.Resolve(); resErr != nil {
		t.Fatalf("Resolve error = %v", resErr)
	}

	_, state, err := p.Move(srcRes, dst)
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}

	// undo: should move back.
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
	tmp := t.TempDir()
	p := testProvider(t, tmp)
	if err := p.CompensateWriteText(nil); err != nil {
		t.Errorf("CompensateWriteText(zero) = %v, want nil", err)
	}
}

func TestCompensateWriteBytes_ZeroState(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)
	if err := p.CompensateWriteBytes(nil); err != nil {
		t.Errorf("CompensateWriteBytes(zero) = %v, want nil", err)
	}
}

func TestWriteText_CreatesParentDirectories(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nested", "deep", "file.txt")

	p := testProvider(t, tmp)
	result, _, err := p.WriteText(path, "nested content", 0o644)
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}
	if result.SourcePath.Abs() != path {
		t.Errorf("result.SourcePath.Abs() = %q, want %q", result.SourcePath.Abs(), path)
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

	p := testProvider(t, tmp)
	_, state, err := p.WriteText(path, "to be undone", 0o644)
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}

	// File should exist after write.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist after WriteText: %v", err)
	}

	// undo: new file should be removed.
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

	p := testProvider(t, tmp)
	_, state, err := p.WriteBytes(path, "to be undone", 0o600)
	if err != nil {
		t.Fatalf("WriteBytes() error = %v", err)
	}

	// File should exist after write.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist after WriteBytes: %v", err)
	}

	// undo: new file should be removed.
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

	p := testProvider(t, tmp)
	got, err := p.Exists(&Resource{SourcePath: op.NewPath("", path)})
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

	p := testProvider(t, tmp)
	got, err := p.Exists(&Resource{SourcePath: op.NewPath("", path)})
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

	p := testProvider(t, tmp)
	got, err := p.Exists(&Resource{SourcePath: op.NewPath("", link)})
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("Exists() = false, want true for symlink")
	}
}

func TestExists_Directory(t *testing.T) {
	tmp := t.TempDir()

	p := testProvider(t, tmp)
	got, err := p.Exists(&Resource{SourcePath: op.NewPath("", tmp)})
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

	p := testProvider(t, tmp)
	got, err := p.IsDir(&Resource{SourcePath: op.NewPath("", tmp)})
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

	p := testProvider(t, tmp)
	got, err := p.IsDir(&Resource{SourcePath: op.NewPath("", path)})
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("IsDir() = true, want false for regular file")
	}
}

func TestIsDir_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)
	got, err := p.IsDir(&Resource{SourcePath: op.NewPath("", "/nonexistent/path")})
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

	p := testProvider(t, tmp)
	got, err := p.IsFile(&Resource{SourcePath: op.NewPath("", path)})
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("IsFile() = false, want true for regular file")
	}
}

func TestIsFile_Directory(t *testing.T) {
	tmp := t.TempDir()

	p := testProvider(t, tmp)
	got, err := p.IsFile(&Resource{SourcePath: op.NewPath("", tmp)})
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Error("IsFile() = true, want false for directory")
	}
}

func TestIsFile_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)
	got, err := p.IsFile(&Resource{SourcePath: op.NewPath("", "/nonexistent/path")})
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
	if err := os.Symlink("target", link); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	// Symlink to regular file resolves via os.Stat, so IsFile returns true.
	got, err := p.IsFile(&Resource{SourcePath: op.NewPath("", link)})
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("IsFile() = false, want true for symlink to regular file")
	}
}

// --- Join ---

func TestJoin(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	if got := p.Join("a", "b", "c"); got != filepath.Join("a", "b", "c") {
		t.Errorf("Join(a,b,c) = %q, want %q", got, filepath.Join("a", "b", "c"))
	}
}

func TestJoin_Empty(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	if got := p.Join(); got != "" {
		t.Errorf("Join() = %q, want empty string", got)
	}
}

func TestJoin_SinglePart(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	if got := p.Join("only"); got != "only" {
		t.Errorf("Join(only) = %q, want %q", got, "only")
	}
}

// --- ReceiverName ---

func TestName(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

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
			t.Errorf("ReceiverName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// --- Parent ---

func TestParent(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

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

	p := testProvider(t, tmp)
	product, _, err := p.Mkdir(path, 0o755)
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if product.SourcePath.Abs() != path {
		t.Errorf("product.SourcePath.Abs() = %q, want %q", product.SourcePath.Abs(), path)
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

	p := testProvider(t, tmp)
	_, _, err := p.Mkdir(path, 0o755)
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

	p := testProvider(t, tmp)
	_, _, err := p.Mkdir(path, 0o755)
	if err != nil {
		t.Fatalf("Mkdir() on existing directory error = %v", err)
	}
}

// --- ReadText ---

func TestReadText_ReturnsFileContents(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	content, err := p.ReadText(&Resource{SourcePath: op.NewPath("", path)})
	if err != nil {
		t.Fatalf("ReadText() error = %v", err)
	}

	if content != "hello" {
		t.Errorf("ReadText() = %q, want %q", content, "hello")
	}
}

func TestReadText_NonExistent_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)
	_, err := p.ReadText(&Resource{SourcePath: op.NewPath("", filepath.Join(tmp, "nonexistent.txt"))})
	if err == nil {
		t.Fatal("ReadText() on non-existent file: expected error, got nil")
	}
}

// --- ReadBytes ---

func TestReadBytes_ReturnsFileContents(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "file.bin")
	data := []byte{0x00, 0x01, 0x02, 0xff}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	result, err := p.ReadBytes(&Resource{SourcePath: op.NewPath("", path)})
	if err != nil {
		t.Fatalf("ReadBytes() error = %v", err)
	}

	if len(result) != len(data) {
		t.Fatalf("ReadBytes() len = %d, want %d", len(result), len(data))
	}
	for i, b := range result {
		if b != data[i] {
			t.Errorf("ReadBytes()[%d] = %#x, want %#x", i, b, data[i])
		}
	}
}

func TestReadBytes_NonExistent_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)
	_, err := p.ReadBytes(&Resource{SourcePath: op.NewPath("", filepath.Join(tmp, "nonexistent.bin"))})
	if err == nil {
		t.Fatal("ReadBytes() on non-existent file: expected error, got nil")
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

// --- Find ---

func TestFind_RelativePattern_ResolvesAgainstScopedRoot(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, "a.go", "package a")
	writeTestFile(t, tmp, "b.go", "package b")
	writeTestFile(t, tmp, "c.txt", "text")

	p := testProvider(t, tmp)
	matches, err := p.Find("*.go", false)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("Find() returned %d matches, want 2: %v", len(matches), matches)
	}
}

func TestFind_DoubleStarRecurses(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "src", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, tmp, "top.go", "package top")
	writeTestFile(t, filepath.Join(tmp, "src"), "mid.go", "package mid")
	writeTestFile(t, filepath.Join(tmp, "src", "pkg"), "deep.go", "package deep")

	p := testProvider(t, tmp)
	matches, err := p.Find("**/*.go", false)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}

	if len(matches) != 3 {
		t.Fatalf("Find() returned %d matches, want 3 (top.go, mid.go, deep.go): %v", len(matches), matches)
	}
}

func TestFind_AbsolutePatternInsideScope_Works(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, "x.go", "package x")

	p := testProvider(t, tmp)
	matches, err := p.Find(filepath.Join(tmp, "*.go"), false)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("Find() returned %d matches, want 1: %v", len(matches), matches)
	}
}

func TestFind_AbsolutePatternOutsideScope_Errors(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	_, err := p.Find("/etc/**/*.conf", false)
	if err == nil {
		t.Fatal("Find() should error for absolute path outside scoped root")
	}
	if !strings.Contains(err.Error(), "lies outside scoped root") {
		t.Errorf("error = %v, want substring \"lies outside scoped root\"", err)
	}
}

func TestFind_DotDotPatternEscapingScope_Errors(t *testing.T) {
	parent := t.TempDir()
	scoped := filepath.Join(parent, "scoped")
	if err := os.MkdirAll(scoped, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, parent, "outside.go", "package outside")

	p := testProvider(t, scoped)
	_, err := p.Find("../*.go", false)
	if err == nil {
		t.Fatal("Find() should error for relative pattern that escapes scope via ..")
	}
	if !strings.Contains(err.Error(), "lies outside scoped root") {
		t.Errorf("error = %v, want substring \"lies outside scoped root\"", err)
	}
}

func TestFind_NoMatches_ReturnsEmpty(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, "a.go", "package a")

	p := testProvider(t, tmp)
	matches, err := p.Find("*.xyz", false)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("Find() returned %d matches, want 0: %v", len(matches), matches)
	}
}

func TestFind_DirectoriesNeverMatch(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, tmp, "file", "content")

	p := testProvider(t, tmp)
	matches, err := p.Find("*", false)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}

	for _, r := range matches {
		if filepath.Base(r.SourcePath.Abs()) == "subdir" {
			t.Errorf("Find() returned directory %q; should match files only", r.SourcePath.Abs())
		}
	}
}

func TestFind_HonorGitignore_FiltersIgnoredFiles(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, ".gitignore", "ignored.go\n")
	writeTestFile(t, tmp, "kept.go", "package kept")
	writeTestFile(t, tmp, "ignored.go", "package ignored")

	p := testProvider(t, tmp)
	matches, err := p.Find("*.go", true)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}

	for _, r := range matches {
		if filepath.Base(r.SourcePath.Abs()) == "ignored.go" {
			t.Errorf("Find() returned %q despite gitignore", r.SourcePath.Abs())
		}
	}

	gotKept := false
	for _, r := range matches {
		if filepath.Base(r.SourcePath.Abs()) == "kept.go" {
			gotKept = true
		}
	}
	if !gotKept {
		t.Errorf("Find() did not return kept.go; matches = %v", matches)
	}
}

func TestFind_HonorGitignoreFalse_IncludesIgnoredFiles(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, ".gitignore", "ignored.go\n")
	writeTestFile(t, tmp, "kept.go", "package kept")
	writeTestFile(t, tmp, "ignored.go", "package ignored")

	p := testProvider(t, tmp)
	matches, err := p.Find("*.go", false)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}

	if len(matches) != 2 {
		t.Errorf("Find(honorGitignore=false) returned %d matches, want 2 (both files): %v", len(matches), matches)
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

	p := testProvider(t, tmp)
	_, _, err := p.Remove(&Resource{SourcePath: op.NewPath("", dir)}, false, &Resource{})
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

	p := testProvider(t, tmp)
	_, state, err := p.Remove(&Resource{SourcePath: op.NewPath("", path)}, false, &Resource{})
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

	p := testProvider(t, tmp)
	_, state, err := p.RemoveAll(&Resource{SourcePath: op.NewPath("", dir)}, false, &Resource{})
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

	p := testProvider(t, tmp)
	_, state, err := p.Remove(&Resource{SourcePath: op.NewPath("", path)}, false, &Resource{})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Receipt preserves true identity — SourcePath is the original home.
	if state.Resource().(*Resource).SourcePath.Abs() != path {
		t.Errorf("receipt resource path = %q, want %q (true identity)", state.Resource().(*Resource).SourcePath.Abs(), path)
	}

	// Verify recovery site holds the data; TransactionID is the recovery key.
	recoveryPath := state.TransactionID()
	if _, err := os.Stat(filepath.Join(tmp, recoveryPath)); err != nil {
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
	if _, err := os.Stat(filepath.Join(tmp, recoveryPath)); !os.IsNotExist(err) {
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

	p := testProvider(t, tmp)
	_, state, err := p.RemoveAll(&Resource{SourcePath: op.NewPath("", dir)}, false, &Resource{})
	if err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	// Receipt preserves true identity — SourcePath is the original home.
	if state.Resource().(*Resource).SourcePath.Abs() != dir {
		t.Errorf("receipt resource path = %q, want %q (true identity)", state.Resource().(*Resource).SourcePath.Abs(), dir)
	}

	// TransactionID is the recovery key.
	recoveryPath := state.TransactionID()
	if _, err := os.Stat(filepath.Join(tmp, recoveryPath)); err != nil {
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

	if _, err := os.Stat(filepath.Join(tmp, recoveryPath)); !os.IsNotExist(err) {
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

	p := testProvider(t, tmp)
	_, state, err := p.Unlink(&Resource{SourcePath: op.NewPath("", linkPath)}, false, &Resource{})
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
	_, state, err := p.WriteText(path, "replaced content", 0o644)
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

	// undo: should restore the original.
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

	p := testProvider(t, tmp)
	result, state, err := p.Backup(&Resource{SourcePath: op.NewPath("", path)}, ".bak")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	// Original should be gone, backup should exist.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("original file still exists after Backup")
	}
	if _, err := os.Stat(result.SourcePath.Abs()); err != nil {
		t.Fatalf("backup file does not exist: %v", err)
	}

	// undo: should restore original.
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
	if _, err := os.Stat(result.SourcePath.Abs()); !os.IsNotExist(err) {
		t.Error("backup file still exists after compensation")
	}
}

// --- Copy + CompensateCopy round-trip ---

func TestCopy_CompensateCopy_RoundTrip_NewFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "new.txt")

	p := testProvider(t, tmp)
	blob := testFileResource(t, []byte("new content"))
	_, state, err := p.Copy(blob, path, 0o644)
	if err != nil {
		t.Fatalf("Copy() error = %v", err)
	}

	// undo: file didn't exist before, so it should be removed.
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
	_, state, err := p.Copy(blob, path, 0o644)
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
func testFileResource(t *testing.T, content []byte) *Resource {
	t.Helper()
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "file-*")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		t.Fatalf("writing temp file: %v", err)
	}
	_ = f.Close()
	root := testRoot(t, dir)
	ctx := &op.ExecutionContext{Root: root}
	fileResource, err := NewResource(ctx, f.Name())
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	return fileResource
}

// --- Helpers ---

func TestChecksumFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "checksum.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := op.NewRootReaderWriter(tmp)
	got := checksumFile(root, path)
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
	root := op.NewRootReaderWriter(t.TempDir())
	got := checksumFile(root, "/nonexistent/file.txt")
	if got != "" {
		t.Errorf("checksumFile(nonexistent) = %q, want empty string", got)
	}
}

func TestIsDirAndNotEmpty(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	// Empty directory
	emptyDir := filepath.Join(tmp, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	isNonEmpty, err := p.isDirAndNotEmpty(emptyDir)
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
	isNonEmpty, err = p.isDirAndNotEmpty(nonEmptyDir)
	if err != nil {
		t.Fatalf("isDirAndNotEmpty(notempty) error = %v", err)
	}
	if !isNonEmpty {
		t.Error("isDirAndNotEmpty(notempty) = false, want true")
	}

	// Regular file
	filePath := filepath.Join(tmp, "file.txt")
	writeTestFile(t, tmp, "file.txt", "data")
	isNonEmpty, err = p.isDirAndNotEmpty(filePath)
	if err != nil {
		t.Fatalf("isDirAndNotEmpty(file) error = %v", err)
	}
	if isNonEmpty {
		t.Error("isDirAndNotEmpty(file) = true, want false for regular file")
	}

	// Nonexistent
	_, err = p.isDirAndNotEmpty(filepath.Join(tmp, "no-such-thing"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("isDirAndNotEmpty(nonexistent) error = %v, want os.ErrNotExist", err)
	}
}

// resolveReadlink reads the symlink target and resolves relative targets to absolute paths.
func resolveReadlink(t *testing.T, linkPath string) string {
	t.Helper()

	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}

	if !filepath.IsAbs(got) {
		got = filepath.Clean(filepath.Join(filepath.Dir(linkPath), got))
	}

	return got
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- closestExistingDir ---

func TestClosestExistingDir_PathExists_ReturnsPath(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	ancestor, info, err := p.closestExistingDir(tmp)
	if err != nil {
		t.Fatalf("closestExistingDir() error = %v", err)
	}
	if ancestor.SourcePath.Abs() != tmp {
		t.Errorf("ancestor = %q, want %q", ancestor.SourcePath.Abs(), tmp)
	}
	if info == nil || !info.IsDir() {
		t.Error("info should report directory")
	}
}

func TestClosestExistingDir_PathDoesNotExist_ReturnsNearestAncestor(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "a", "b", "c", "d")
	p := testProvider(t, tmp)

	ancestor, _, err := p.closestExistingDir(missing)
	if err != nil {
		t.Fatalf("closestExistingDir() error = %v", err)
	}
	if ancestor.SourcePath.Abs() != tmp {
		t.Errorf("ancestor = %q, want %q (nearest existing)", ancestor.SourcePath.Abs(), tmp)
	}
}

func TestClosestExistingDir_PartialChain_ReturnsLowestExisting(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(tmp, "a", "b", "c", "d")
	p := testProvider(t, tmp)

	ancestor, info, err := p.closestExistingDir(target)
	if err != nil {
		t.Fatalf("closestExistingDir() error = %v", err)
	}
	want := filepath.Join(tmp, "a", "b")
	if ancestor.SourcePath.Abs() != want {
		t.Errorf("ancestor = %q, want %q", ancestor.SourcePath.Abs(), want)
	}
	if info == nil || !info.IsDir() {
		t.Error("info should report directory")
	}
}

func TestClosestExistingDir_PathOutsideRoot_Errors(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	outside := "/not/under/root"
	_, _, err := p.closestExistingDir(outside)
	if err == nil {
		t.Fatal("closestExistingDir() should error for path outside scoped root")
	}
	if !strings.Contains(err.Error(), "lies outside scoped root") {
		t.Errorf("error = %v, want substring \"lies outside scoped root\"", err)
	}
}

func TestClosestExistingDir_RegularFile_ReturnsFileInfo(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, "hello.txt", "hi")
	target := filepath.Join(tmp, "hello.txt")
	p := testProvider(t, tmp)

	ancestor, info, err := p.closestExistingDir(target)
	if err != nil {
		t.Fatalf("closestExistingDir() error = %v", err)
	}
	if ancestor.SourcePath.Abs() != target {
		t.Errorf("ancestor = %q, want %q", ancestor.SourcePath.Abs(), target)
	}
	if info == nil || info.IsDir() {
		t.Error("info should report regular file (not directory)")
	}
}

// --- CompensateMkdir ---

func TestCompensateMkdir_RoundTrip_RemovesCreatedChain(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "a", "b", "c")
	p := testProvider(t, tmp)

	_, receipt, err := p.Mkdir(target, 0o755)
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	if err := p.CompensateMkdir(receipt); err != nil {
		t.Fatalf("CompensateMkdir() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmp, "a")); !os.IsNotExist(err) {
		t.Errorf("created chain not removed; stat error = %v, want IsNotExist", err)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Errorf("scoped root removed by compensation; stat error = %v", err)
	}
}

func TestCompensateMkdir_StopsAtBoundary_PreservesPreExisting(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(tmp, "a", "b", "c", "d")
	p := testProvider(t, tmp)

	_, receipt, err := p.Mkdir(target, 0o755)
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	if err := p.CompensateMkdir(receipt); err != nil {
		t.Fatalf("CompensateMkdir() error = %v", err)
	}

	for _, dir := range []string{"a", filepath.Join("a", "b")} {
		if _, err := os.Stat(filepath.Join(tmp, dir)); err != nil {
			t.Errorf("pre-existing %q removed by compensation; stat error = %v", dir, err)
		}
	}
	for _, dir := range []string{filepath.Join("a", "b", "c"), filepath.Join("a", "b", "c", "d")} {
		if _, err := os.Stat(filepath.Join(tmp, dir)); !os.IsNotExist(err) {
			t.Errorf("created %q not removed; stat error = %v, want IsNotExist", dir, err)
		}
	}
}

func TestCompensateMkdir_AlreadyExists_NoOp(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "existing")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	p := testProvider(t, tmp)

	_, receipt, err := p.Mkdir(target, 0o755)
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if receipt != nil {
		t.Errorf("idempotent Mkdir produced a non-nil receipt: %#v", receipt)
	}

	if err := p.CompensateMkdir(receipt); err != nil {
		t.Errorf("CompensateMkdir() on idempotent receipt should be no-op; got error = %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("pre-existing directory removed by no-op compensation; stat error = %v", err)
	}
}

func TestCompensateMkdir_NotADirectory_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	writeTestFile(t, tmp, "regular", "content")
	target := filepath.Join(tmp, "regular")
	p := testProvider(t, tmp)

	_, _, err := p.Mkdir(target, 0o755)
	if err == nil {
		t.Fatal("Mkdir() on a regular file should error")
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("error = %v, want substring \"is not a directory\"", err)
	}
}

func TestCompensateMkdir_TamperedBoundary_Errors(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "siblings"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(tmp, "a", "b", "c")
	p := testProvider(t, tmp)

	_, _, err := p.Mkdir(target, 0o755)
	if err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	wrongResource, err := NewResource(p.ExecutionContext(), target)
	if err != nil {
		t.Fatal(err)
	}
	wrongBoundary, err := NewResource(p.ExecutionContext(), filepath.Join(tmp, "siblings"))
	if err != nil {
		t.Fatal(err)
	}
	tampered := NewReceiptWithBoundary(wrongResource, wrongBoundary)

	if err := p.CompensateMkdir(tampered); err == nil || !strings.Contains(err.Error(), "is not under boundary") {
		t.Errorf("expected tamper-guard error \"is not under boundary\", got %v", err)
	}
}

func TestCompensateMkdir_EmptyReceipt_NoOp(t *testing.T) {
	tmp := t.TempDir()
	p := testProvider(t, tmp)

	if err := p.CompensateMkdir(nil); err != nil {
		t.Errorf("CompensateMkdir(nil) should be no-op; got error = %v", err)
	}
}

// --- compensateWrite boundary walk ---

func TestCompensateWriteText_RoundTrip_RemovesParentDirectories(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "a", "b", "c", "hello.txt")
	p := testProvider(t, tmp)

	_, receipt, err := p.WriteText(target, "hi", 0o644)
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}

	if err := p.CompensateWriteText(receipt); err != nil {
		t.Fatalf("CompensateWriteText() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmp, "a")); !os.IsNotExist(err) {
		t.Errorf("created parent chain not removed; stat error = %v, want IsNotExist", err)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Errorf("scoped root removed by compensation; stat error = %v", err)
	}
}

func TestCompensateWriteText_StopsAtBoundary_PreservesPreExisting(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(tmp, "a", "b", "c", "hello.txt")
	p := testProvider(t, tmp)

	_, receipt, err := p.WriteText(target, "hi", 0o644)
	if err != nil {
		t.Fatalf("WriteText() error = %v", err)
	}

	if err := p.CompensateWriteText(receipt); err != nil {
		t.Fatalf("CompensateWriteText() error = %v", err)
	}

	for _, dir := range []string{"a", filepath.Join("a", "b")} {
		if _, err := os.Stat(filepath.Join(tmp, dir)); err != nil {
			t.Errorf("pre-existing %q removed by compensation; stat error = %v", dir, err)
		}
	}
	if _, err := os.Stat(filepath.Join(tmp, "a", "b", "c")); !os.IsNotExist(err) {
		t.Errorf("created \"a/b/c\" not removed; stat error = %v, want IsNotExist", err)
	}
}

func TestCompensateLink_RoundTrip_RemovesParentDirectories(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "src.txt")
	writeTestFile(t, tmp, "src.txt", "content")
	target := filepath.Join(tmp, "a", "b", "link")

	p := testProvider(t, tmp)
	srcResource, err := NewResource(p.ExecutionContext(), source)
	if err != nil {
		t.Fatal(err)
	}

	_, receipt, err := p.Link(srcResource, target)
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}

	if err := p.CompensateLink(receipt); err != nil {
		t.Fatalf("CompensateLink() error = %v", err)
	}

	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("symlink not removed; lstat error = %v, want IsNotExist", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "a")); !os.IsNotExist(err) {
		t.Errorf("created parent chain not removed; stat error = %v, want IsNotExist", err)
	}
}

func TestCompensateMove_RoundTrip_RemovesCreatedParents(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "src.txt")
	writeTestFile(t, tmp, "src.txt", "content")
	dest := filepath.Join(tmp, "a", "b", "moved.txt")

	p := testProvider(t, tmp)
	srcResource, err := NewResource(p.ExecutionContext(), source)
	if err != nil {
		t.Fatal(err)
	}
	if err := srcResource.Resolve(); err != nil {
		t.Fatal(err)
	}

	_, receipt, err := p.Move(srcResource, dest)
	if err != nil {
		t.Fatalf("Move() error = %v", err)
	}

	if err := p.CompensateMove(receipt); err != nil {
		t.Fatalf("CompensateMove() error = %v", err)
	}

	if _, err := os.Stat(source); err != nil {
		t.Errorf("source not restored; stat error = %v", err)
	}
}
