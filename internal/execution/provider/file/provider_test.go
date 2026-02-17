// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// --- Link ---

func TestLinkNewSymlink(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(source, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")

	p := &Provider{}
	state, err := p.Link(source, link)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state for new symlink")
	}

	// Verify symlink created
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != source {
		t.Errorf("target = %q, want %q", target, source)
	}

	// Compensate: should remove the new symlink
	if err := p.CompensateLink(state); err != nil {
		t.Fatalf("CompensateLink: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("symlink should be removed after compensation")
	}
}

func TestLinkReplaceExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	oldTarget := filepath.Join(dir, "old.txt")
	newTarget := filepath.Join(dir, "new.txt")
	link := filepath.Join(dir, "link")

	os.WriteFile(oldTarget, []byte("old"), 0644)
	os.WriteFile(newTarget, []byte("new"), 0644)
	os.Symlink(oldTarget, link)

	p := &Provider{}
	state, err := p.Link(newTarget, link)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}

	// Verify new symlink
	target, _ := os.Readlink(link)
	if target != newTarget {
		t.Errorf("target = %q, want %q", target, newTarget)
	}

	// Compensate: should restore old symlink target
	if err := p.CompensateLink(state); err != nil {
		t.Fatalf("CompensateLink: %v", err)
	}
	target, _ = os.Readlink(link)
	if target != oldTarget {
		t.Errorf("restored target = %q, want %q", target, oldTarget)
	}
}

func TestLinkIdempotent(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	link := filepath.Join(dir, "link")

	os.WriteFile(source, []byte("content"), 0644)
	os.Symlink(source, link)

	p := &Provider{}
	state, err := p.Link(source, link)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if state != nil {
		t.Error("expected nil state for idempotent no-op")
	}
}

// --- Copy ---

func TestCopyNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	content := []byte("hello world")

	p := &Provider{}
	checksum, state, err := p.Copy(path, 0644, content)
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if checksum == "" {
		t.Error("expected non-empty checksum")
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}

	written, _ := os.ReadFile(path)
	if !bytes.Equal(written, content) {
		t.Error("file content mismatch")
	}

	// Compensate: should remove the new file
	if err := p.CompensateCopy(state); err != nil {
		t.Fatalf("CompensateCopy: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be removed after compensation")
	}
}

func TestCopyOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	original := []byte("original content")
	replacement := []byte("replacement content")

	os.WriteFile(path, original, 0644)

	p := &Provider{}
	_, state, err := p.Copy(path, 0644, replacement)
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}

	written, _ := os.ReadFile(path)
	if !bytes.Equal(written, replacement) {
		t.Error("file should contain replacement content")
	}

	// Compensate: should restore original content
	if err := p.CompensateCopy(state); err != nil {
		t.Fatalf("CompensateCopy: %v", err)
	}
	restored, _ := os.ReadFile(path)
	if !bytes.Equal(restored, original) {
		t.Errorf("restored content = %q, want %q", restored, original)
	}
}

// --- Backup ---

func TestBackupRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	content := []byte("important data")
	os.WriteFile(path, content, 0644)

	p := &Provider{}
	backupPath, state, err := p.Backup(path, "")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if backupPath == "" {
		t.Error("expected non-empty backup path")
	}

	// Original should be gone, backup should exist
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("original file should be moved")
	}
	backupContent, _ := os.ReadFile(backupPath)
	if !bytes.Equal(backupContent, content) {
		t.Error("backup content mismatch")
	}

	// Compensate: should move backup back to original
	if err := p.CompensateBackup(state); err != nil {
		t.Fatalf("CompensateBackup: %v", err)
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("backup file should be gone after compensation")
	}
	restored, _ := os.ReadFile(path)
	if !bytes.Equal(restored, content) {
		t.Errorf("restored content mismatch")
	}
}

// --- Unlink ---

func TestUnlinkRoundTrip(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	link := filepath.Join(dir, "link")

	os.WriteFile(source, []byte("content"), 0644)
	os.Symlink(source, link)

	p := &Provider{}
	state, err := p.Unlink(link, false, "")
	if err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}

	// Symlink should be gone
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Error("symlink should be removed")
	}

	// Compensate: should re-create the symlink
	if err := p.CompensateUnlink(state); err != nil {
		t.Fatalf("CompensateUnlink: %v", err)
	}
	target, _ := os.Readlink(link)
	if target != source {
		t.Errorf("restored target = %q, want %q", target, source)
	}
}

func TestUnlinkAlreadyGone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent")

	p := &Provider{}
	state, err := p.Unlink(path, false, "")
	if err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if state != nil {
		t.Error("expected nil state for already-gone symlink")
	}
}

// --- Remove ---

func TestRemoveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	content := []byte("precious data")
	os.WriteFile(path, content, 0600)

	p := &Provider{}
	state, err := p.Remove(path, false, "")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}

	// File should be gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be removed")
	}

	// Compensate: should re-create the file
	if err := p.CompensateRemove(state); err != nil {
		t.Fatalf("CompensateRemove: %v", err)
	}
	restored, _ := os.ReadFile(path)
	if !bytes.Equal(restored, content) {
		t.Error("restored content mismatch")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Errorf("restored mode = %v, want %v", info.Mode().Perm(), os.FileMode(0600))
	}
}

func TestRemoveAlreadyGone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent")

	p := &Provider{}
	state, err := p.Remove(path, false, "")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if state != nil {
		t.Error("expected nil state for already-gone file")
	}
}

// --- Write ---

func TestWriteNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	p := &Provider{}
	state, err := p.Write("hello world", path, 0644)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}

	written, _ := os.ReadFile(path)
	if string(written) != "hello world" {
		t.Errorf("written = %q, want %q", written, "hello world")
	}

	// Compensate: should remove the new file
	if err := p.CompensateWrite(state); err != nil {
		t.Fatalf("CompensateWrite: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be removed after compensation")
	}
}

func TestWriteOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")
	os.WriteFile(path, []byte("original"), 0644)

	p := &Provider{}
	state, err := p.Write("replacement", path, 0644)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	written, _ := os.ReadFile(path)
	if string(written) != "replacement" {
		t.Errorf("written = %q, want %q", written, "replacement")
	}

	// Compensate: should restore original content
	if err := p.CompensateWrite(state); err != nil {
		t.Fatalf("CompensateWrite: %v", err)
	}
	restored, _ := os.ReadFile(path)
	if string(restored) != "original" {
		t.Errorf("restored = %q, want %q", restored, "original")
	}
}

// --- Move ---

func TestMoveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	dest := filepath.Join(dir, "dest.txt")
	content := []byte("movable data")
	os.WriteFile(source, content, 0644)

	p := &Provider{}
	state, err := p.Move(nil, source, dest)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}

	// Source should be gone, dest should exist
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Error("source should be gone after move")
	}
	moved, _ := os.ReadFile(dest)
	if !bytes.Equal(moved, content) {
		t.Error("moved content mismatch")
	}

	// Compensate: should move back from dest to source
	if err := p.CompensateMove(state); err != nil {
		t.Fatalf("CompensateMove: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("dest should be gone after compensation")
	}
	restored, _ := os.ReadFile(source)
	if !bytes.Equal(restored, content) {
		t.Error("restored content mismatch")
	}
}

// --- Nil state safety ---

func TestCompensateNilState(t *testing.T) {
	p := &Provider{}

	// All Compensate methods must handle nil gracefully
	if err := p.CompensateLink(nil); err != nil {
		t.Errorf("CompensateLink(nil): %v", err)
	}
	if err := p.CompensateCopy(nil); err != nil {
		t.Errorf("CompensateCopy(nil): %v", err)
	}
	if err := p.CompensateBackup(nil); err != nil {
		t.Errorf("CompensateBackup(nil): %v", err)
	}
	if err := p.CompensateUnlink(nil); err != nil {
		t.Errorf("CompensateUnlink(nil): %v", err)
	}
	if err := p.CompensateRemove(nil); err != nil {
		t.Errorf("CompensateRemove(nil): %v", err)
	}
	if err := p.CompensateWrite(nil); err != nil {
		t.Errorf("CompensateWrite(nil): %v", err)
	}
	if err := p.CompensateMove(nil); err != nil {
		t.Errorf("CompensateMove(nil): %v", err)
	}
}

// --- Non-compensable methods (unchanged) ---

func TestSourceUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	content := []byte("read me")
	os.WriteFile(path, content, 0644)

	p := &Provider{}
	result, err := p.Source(path)
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if !bytes.Equal(result, content) {
		t.Error("Source content mismatch")
	}
}

func TestMkdirUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "nested")

	p := &Provider{}
	if err := p.Mkdir(path, 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}
