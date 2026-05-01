// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// newTestRecoverySite creates a RecoverySite backed by a RootReaderWriter at a temp directory.
func newTestRecoverySite(t *testing.T) (*RecoverySite, Root) {
	t.Helper()
	tmp := t.TempDir()
	root := NewRootReaderWriter(tmp)
	ctx := &ExecutionContext{Root: root}
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

	// Recovery ID is an opaque UUID v7; the on-disk archive lives at <root>/.devlore/recovery/<uuid>.
	if _, err := uuid.Parse(recoveryID); err != nil {
		t.Errorf("recoveryID = %q, want parseable UUID: %v", recoveryID, err)
	}

	// Recovery file should exist with same content.
	absRecovery := filepath.Join(tmp, ".devlore", "recovery", recoveryID)
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

	// Recovery ID is an opaque UUID v7; presence under .devlore/recovery/ is verified by RestoreFile and
	// content-read tests elsewhere in this file.
	if _, err := uuid.Parse(recoveryID); err != nil {
		t.Errorf("recoveryID = %q, want parseable UUID: %v", recoveryID, err)
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
	absRecovery := filepath.Join(tmp, ".devlore", "recovery", recoveryID)
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
	if err := os.RemoveAll(filepath.Join(tmp, "a")); err != nil {
		t.Fatal(err)
	}

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

	got, err := os.ReadFile(filepath.Join(tmp, ".devlore", "recovery", recoveryID))
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

// --- ArchiveStream ---

func TestArchiveStream_EmptyReader(t *testing.T) {
	site, root := newTestRecoverySite(t)

	recoveryID, err := site.ArchiveStream(bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("ArchiveStream(empty) error = %v", err)
	}
	if recoveryID == "" {
		t.Fatal("recoveryID is empty")
	}

	info, err := os.Stat(filepath.Join(root.Name(), ".devlore", "recovery", recoveryID))
	if err != nil {
		t.Fatalf("stat recovery file: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("recovery file size = %d, want 0", info.Size())
	}
}

func TestArchiveStream_SmallReader(t *testing.T) {
	site, root := newTestRecoverySite(t)

	content := []byte("streamed content")

	recoveryID, err := site.ArchiveStream(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ArchiveStream error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root.Name(), ".devlore", "recovery", recoveryID))
	if err != nil {
		t.Fatalf("read recovery file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("recovery file content = %q, want %q", got, content)
	}
}

func TestArchiveStream_LargeReader(t *testing.T) {
	// Exercise streaming by writing a payload larger than any reasonable in-memory default buffer.
	// 1 MiB is enough to verify io.Copy drains chunk-by-chunk.
	site, root := newTestRecoverySite(t)

	const size = 1 << 20
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i)
	}

	recoveryID, err := site.ArchiveStream(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("ArchiveStream error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root.Name(), ".devlore", "recovery", recoveryID))
	if err != nil {
		t.Fatalf("read recovery file: %v", err)
	}
	if len(got) != size {
		t.Fatalf("recovery file size = %d, want %d", len(got), size)
	}
	for i, b := range got {
		if b != byte(i) {
			t.Fatalf("byte %d = %d, want %d", i, b, byte(i))
		}
	}
}

func TestArchiveStream_ReaderError(t *testing.T) {
	site, _ := newTestRecoverySite(t)

	_, err := site.ArchiveStream(&erroringReader{after: 5})
	if err == nil {
		t.Fatal("expected error from erroring reader")
	}
	if !strings.Contains(err.Error(), "stream to recovery") {
		t.Errorf("error = %q, want message containing 'stream to recovery'", err)
	}
}

// erroringReader yields `after` bytes of zeros and then returns a synthetic error — exercises the mid-stream
// error path in ArchiveStream.
type erroringReader struct {
	after int
	read  int
}

func (r *erroringReader) Read(p []byte) (int, error) {
	if r.read >= r.after {
		return 0, errSyntheticRead
	}
	n := len(p)
	if r.read+n > r.after {
		n = r.after - r.read
	}
	for i := range n {
		p[i] = 0
	}
	r.read += n
	return n, nil
}

var errSyntheticRead = errors.New("synthetic read error")
