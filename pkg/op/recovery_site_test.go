// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestRecoverySite creates a RecoverySite backed by a RootReaderWriter at a temp directory.
func newTestRecoverySite(t *testing.T) (*RecoverySite, Root) {
	t.Helper()
	tmp := t.TempDir()
	root := NewRootReaderWriter(tmp)
	ctx := Context{ContextBase: ContextBase{Root: root}}
	return NewRecoverySite(ctx), root
}

func TestArchiveFile_MovesFile(t *testing.T) {
	site, root := newTestRecoverySite(t)
	tmp := root.Name()

	srcPath := filepath.Join(tmp, "original.txt")
	if err := os.WriteFile(srcPath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	recoveryID, err := site.ArchiveFile(root.NewPath("original.txt"))
	if err != nil {
		t.Fatalf("ArchiveFile() error = %v", err)
	}

	// Original should be gone.
	if _, err := os.Lstat(srcPath); !os.IsNotExist(err) {
		t.Error("original file still exists after archive")
	}

	// Recovery ID should be root-relative under .devlore/recovery/.
	if !strings.HasPrefix(recoveryID, ".devlore/recovery/") {
		t.Errorf("recovery ID %q does not start with .devlore/recovery/", recoveryID)
	}

	// Recovery file should exist with same content.
	absRecovery := filepath.Join(tmp, recoveryID)
	data, err := os.ReadFile(absRecovery)
	if err != nil {
		t.Fatalf("ReadFile(recovery) error = %v", err)
	}
	if string(data) != "content" {
		t.Errorf("recovered content = %q, want %q", data, "content")
	}
}

func TestArchiveFile_CreatesRecoveryDir(t *testing.T) {
	site, root := newTestRecoverySite(t)
	tmp := root.Name()

	if err := os.WriteFile(filepath.Join(tmp, "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	recoveryID, err := site.ArchiveFile(root.NewPath("file.txt"))
	if err != nil {
		t.Fatalf("ArchiveFile() error = %v", err)
	}

	// Recovery ID should be under .devlore/recovery/.
	if !strings.HasPrefix(recoveryID, ".devlore/recovery/") {
		t.Errorf("recovery ID %q not under .devlore/recovery/", recoveryID)
	}
}

func TestRestoreFile_MovesBack(t *testing.T) {
	site, root := newTestRecoverySite(t)
	tmp := root.Name()

	srcPath := filepath.Join(tmp, "original.txt")
	if err := os.WriteFile(srcPath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := root.NewPath("original.txt")
	recoveryID, err := site.ArchiveFile(p)
	if err != nil {
		t.Fatal(err)
	}

	if err := site.RestoreFile(p, recoveryID); err != nil {
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

	// Recovery file should be gone.
	absRecovery := filepath.Join(tmp, recoveryID)
	if _, err := os.Lstat(absRecovery); !os.IsNotExist(err) {
		t.Error("recovery file still exists after restore")
	}
}

func TestRestoreFile_RecreatesParentDir(t *testing.T) {
	site, root := newTestRecoverySite(t)
	tmp := root.Name()

	nested := filepath.Join(tmp, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := root.NewPath("a/b/file.txt")
	recoveryID, err := site.ArchiveFile(p)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate pruneEmptyParents removing the parent directory.
	os.RemoveAll(filepath.Join(tmp, "a"))

	if err := site.RestoreFile(p, recoveryID); err != nil {
		t.Fatalf("RestoreFile() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "a", "b", "file.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "data" {
		t.Errorf("content = %q, want %q", data, "data")
	}
}

func TestRestoreFile_ErrorOnEmptyPaths(t *testing.T) {
	site, root := newTestRecoverySite(t)

	if err := site.RestoreFile(root.NewPath(""), "some/path"); err == nil {
		t.Error("RestoreFile(empty path, ...) should error")
	}

	if err := site.RestoreFile(root.NewPath("some/path"), ""); err == nil {
		t.Error("RestoreFile(..., \"\") should error")
	}
}

func TestRestoreFile_ErrorOnMissingRecovery(t *testing.T) {
	site, root := newTestRecoverySite(t)

	err := site.RestoreFile(root.NewPath("some/original"), "nonexistent/recovery")
	if err == nil {
		t.Error("RestoreFile with missing recovery should error")
	}
}

func TestArchiveData_WritesBytes(t *testing.T) {
	site, root := newTestRecoverySite(t)
	tmp := root.Name()

	data := []byte("hello, recovery")

	recoveryID, err := site.ArchiveData(data)
	if err != nil {
		t.Fatalf("ArchiveData() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tmp, recoveryID))
	if err != nil {
		t.Fatalf("ReadFile(recovery) error = %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("archived data = %q, want %q", got, data)
	}
}

func TestRestoreData_ReadsBytes(t *testing.T) {
	site, _ := newTestRecoverySite(t)

	original := []byte("round-trip data")

	recoveryID, err := site.ArchiveData(original)
	if err != nil {
		t.Fatal(err)
	}

	got, err := site.RestoreData(recoveryID)
	if err != nil {
		t.Fatalf("RestoreData() error = %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("restored data = %q, want %q", got, original)
	}
}
