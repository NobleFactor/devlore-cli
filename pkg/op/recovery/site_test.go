// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package recovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveFile_MovesFile(t *testing.T) {
	tmp := t.TempDir()
	site := NewSite(tmp)

	srcPath := filepath.Join(tmp, "original.txt")
	if err := os.WriteFile(srcPath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	recoveryPath, err := site.ArchiveFile(srcPath)
	if err != nil {
		t.Fatalf("ArchiveFile() error = %v", err)
	}

	// Original should be gone.
	if _, err := os.Lstat(srcPath); !os.IsNotExist(err) {
		t.Error("original file still exists after archive")
	}

	// Recovery path should exist with same content.
	data, err := os.ReadFile(recoveryPath)
	if err != nil {
		t.Fatalf("ReadFile(recovery) error = %v", err)
	}
	if string(data) != "content" {
		t.Errorf("recovered content = %q, want %q", data, "content")
	}
}

func TestArchiveFile_CreatesRecoveryDir(t *testing.T) {
	tmp := t.TempDir()
	site := NewSite(tmp)

	srcPath := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(srcPath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	recoveryPath, err := site.ArchiveFile(srcPath)
	if err != nil {
		t.Fatalf("ArchiveFile() error = %v", err)
	}

	// Recovery path should be under .devlore/recovery/.
	dir := filepath.Join(tmp, ".devlore", "recovery")
	rel, err := filepath.Rel(dir, recoveryPath)
	if err != nil || filepath.IsAbs(rel) {
		t.Errorf("recovery path %q not under %q", recoveryPath, dir)
	}
}

func TestRestoreFile_MovesBack(t *testing.T) {
	tmp := t.TempDir()
	site := NewSite(tmp)

	srcPath := filepath.Join(tmp, "original.txt")
	if err := os.WriteFile(srcPath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	recoveryPath, err := site.ArchiveFile(srcPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := site.RestoreFile(srcPath, recoveryPath); err != nil {
		t.Fatalf("RestoreFile() error = %v", err)
	}

	// Original should be back.
	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("ReadFile(original) error = %v", err)
	}
	if string(data) != "content" {
		t.Errorf("restored content = %q, want %q", data, "content")
	}

	// Recovery path should be gone.
	if _, err := os.Lstat(recoveryPath); !os.IsNotExist(err) {
		t.Error("recovery file still exists after restore")
	}
}

func TestRestoreFile_RecreatesParentDir(t *testing.T) {
	tmp := t.TempDir()
	site := NewSite(tmp)

	nested := filepath.Join(tmp, "a", "b", "file.txt")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	recoveryPath, err := site.ArchiveFile(nested)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate pruneEmptyParents removing the parent directory.
	os.RemoveAll(filepath.Join(tmp, "a"))

	if err := site.RestoreFile(nested, recoveryPath); err != nil {
		t.Fatalf("RestoreFile() error = %v", err)
	}

	data, err := os.ReadFile(nested)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "data" {
		t.Errorf("content = %q, want %q", data, "data")
	}
}

func TestRestoreFile_ErrorOnEmptyPaths(t *testing.T) {
	site := NewSite(t.TempDir())

	if err := site.RestoreFile("", "some/path"); err == nil {
		t.Error("RestoreFile(\"\", ...) should error")
	}

	if err := site.RestoreFile("some/path", ""); err == nil {
		t.Error("RestoreFile(..., \"\") should error")
	}
}

func TestRestoreFile_ErrorOnMissingRecovery(t *testing.T) {
	site := NewSite(t.TempDir())

	err := site.RestoreFile("/some/original", "/nonexistent/recovery")
	if err == nil {
		t.Error("RestoreFile with missing recovery should error")
	}
}

func TestArchiveData_WritesBytes(t *testing.T) {
	tmp := t.TempDir()
	site := NewSite(tmp)

	data := []byte("hello, recovery")

	recoveryPath, err := site.ArchiveData(data)
	if err != nil {
		t.Fatalf("ArchiveData() error = %v", err)
	}

	got, err := os.ReadFile(recoveryPath)
	if err != nil {
		t.Fatalf("ReadFile(recovery) error = %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("archived data = %q, want %q", got, data)
	}
}

func TestRestoreData_ReadsBytes(t *testing.T) {
	tmp := t.TempDir()
	site := NewSite(tmp)

	original := []byte("round-trip data")

	recoveryPath, err := site.ArchiveData(original)
	if err != nil {
		t.Fatal(err)
	}

	got, err := site.RestoreData(recoveryPath)
	if err != nil {
		t.Fatalf("RestoreData() error = %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("restored data = %q, want %q", got, original)
	}
}
