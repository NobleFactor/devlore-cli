// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// createTarGz builds a tar.gz archive at archivePath containing the given entries.
// Each entry is a relative path; directories end with "/".
func createTarGz(t *testing.T, archivePath string, entries map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive file: %v", err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	for name, content := range entries {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header %q: %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write tar content %q: %v", name, err)
		}
	}
}

// createZip builds a zip archive at archivePath containing the given entries.
func createZip(t *testing.T, archivePath string, entries map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive file: %v", err)
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	defer func() { _ = zw.Close() }()

	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip content %q: %v", name, err)
		}
	}
}

func TestExtractTarGz(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.tar.gz")
	entries := map[string]string{
		"dir/hello.txt":   "hello",
		"dir/goodbye.txt": "goodbye",
	}
	createTarGz(t, archivePath, entries)

	prefix := filepath.Join(tmp, "out")
	p := &Provider{}
	dest, state, err := p.Extract(file.Resource{SourcePath: op.NewPath("", archivePath)}, file.Resource{SourcePath: op.NewPath("", prefix)})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if dest.SourcePath.Abs() != prefix {
		t.Errorf("dest = %q, want %q", dest.SourcePath.Abs(), prefix)
	}

	// Verify files exist.
	for name, wantContent := range entries {
		path := filepath.Join(prefix, name)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %q: %v", name, err)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("content of %q = %q, want %q", name, got, wantContent)
		}
	}

	// Verify state contains created files.
	if len(state.CreatedFiles) != len(entries) {
		t.Errorf("CreatedFiles has %d entries, want %d", len(state.CreatedFiles), len(entries))
	}
}

func TestExtractZip(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.zip")
	entries := map[string]string{
		"sub/a.txt": "alpha",
		"sub/b.txt": "bravo",
	}
	createZip(t, archivePath, entries)

	prefix := filepath.Join(tmp, "out")
	p := &Provider{}
	dest, state, err := p.Extract(file.Resource{SourcePath: op.NewPath("", archivePath)}, file.Resource{SourcePath: op.NewPath("", prefix)})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if dest.SourcePath.Abs() != prefix {
		t.Errorf("dest = %q, want %q", dest.SourcePath.Abs(), prefix)
	}

	for name, wantContent := range entries {
		path := filepath.Join(prefix, name)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %q: %v", name, err)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("content of %q = %q, want %q", name, got, wantContent)
		}
	}

	if len(state.CreatedFiles) != len(entries) {
		t.Errorf("CreatedFiles has %d entries, want %d", len(state.CreatedFiles), len(entries))
	}
}

func TestExtractUnsupportedFormat(t *testing.T) {
	p := &Provider{}
	_, _, err := p.Extract(file.Resource{SourcePath: op.NewPath("", "foo.rar")}, file.Resource{SourcePath: op.NewPath("", t.TempDir())})
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	want := "unsupported archive format"
	if got := err.Error(); got != "unsupported archive format: foo.rar" {
		t.Errorf("error = %q, want to contain %q", got, want)
	}
}

func TestZipSlipProtectionTarGz(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "malicious.tar.gz")

	// Create tar.gz with a path-traversal entry.
	entries := map[string]string{
		"../escape.txt": "escaped",
		"safe.txt":      "safe",
	}
	createTarGz(t, archivePath, entries)

	prefix := filepath.Join(tmp, "out")
	p := &Provider{}
	_, _, err := p.Extract(file.Resource{SourcePath: op.NewPath("", archivePath)}, file.Resource{SourcePath: op.NewPath("", prefix)})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// The traversal file must NOT exist outside prefix.
	escapedPath := filepath.Join(tmp, "escape.txt")
	if _, err := os.Stat(escapedPath); err == nil {
		t.Error("zip slip: file escaped prefix directory")
	}

	// The safe file should exist.
	safePath := filepath.Join(prefix, "safe.txt")
	if _, err := os.Stat(safePath); err != nil {
		t.Errorf("safe.txt not found: %v", err)
	}
}

func TestZipSlipProtectionZip(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "malicious.zip")

	entries := map[string]string{
		"../escape.txt": "escaped",
		"safe.txt":      "safe",
	}
	createZip(t, archivePath, entries)

	prefix := filepath.Join(tmp, "out")
	p := &Provider{}
	_, _, err := p.Extract(file.Resource{SourcePath: op.NewPath("", archivePath)}, file.Resource{SourcePath: op.NewPath("", prefix)})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	escapedPath := filepath.Join(tmp, "escape.txt")
	if _, err := os.Stat(escapedPath); err == nil {
		t.Error("zip slip: file escaped prefix directory")
	}

	safePath := filepath.Join(prefix, "safe.txt")
	if _, err := os.Stat(safePath); err != nil {
		t.Errorf("safe.txt not found: %v", err)
	}
}

func TestCompensateExtractRemovesFiles(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "sub")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	file1 := filepath.Join(dir, "a.txt")
	file2 := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(file1, []byte("a"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(file2, []byte("b"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	state := Tombstone{
		Dest:         tmp,
		CreatedFiles: []string{file1, file2},
	}

	p := &Provider{}
	if err := p.CompensateExtract(state); err != nil {
		t.Fatalf("CompensateExtract: %v", err)
	}

	if _, err := os.Stat(file1); !os.IsNotExist(err) {
		t.Errorf("file1 still exists after compensation")
	}
	if _, err := os.Stat(file2); !os.IsNotExist(err) {
		t.Errorf("file2 still exists after compensation")
	}
}

func TestCompensateExtractEmptyState(t *testing.T) {
	p := &Provider{}
	if err := p.CompensateExtract(Tombstone{}); err != nil {
		t.Fatalf("CompensateExtract(empty) = %v, want nil", err)
	}
}

func TestCompensateExtractCleansEmptyDirs(t *testing.T) {
	tmp := t.TempDir()
	deepDir := filepath.Join(tmp, "a", "b", "c")
	if err := os.MkdirAll(deepDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	f := filepath.Join(deepDir, "only.txt")
	if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	state := Tombstone{
		Dest:         tmp,
		CreatedFiles: []string{f},
	}

	p := &Provider{}
	if err := p.CompensateExtract(state); err != nil {
		t.Fatalf("CompensateExtract: %v", err)
	}

	// After removing the only file, the empty dir chain should be cleaned.
	if _, err := os.Stat(deepDir); !os.IsNotExist(err) {
		t.Errorf("empty directory %q still exists after compensation", deepDir)
	}
}
