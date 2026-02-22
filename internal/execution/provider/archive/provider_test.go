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
)

// createTarGz creates a .tar.gz with the given file entries (name → content).
func createTarGz(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	path := filepath.Join(dir, "test.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	tw.Close()
	gw.Close()
	return path
}

// createZip creates a .zip with the given file entries (name → content).
func createZip(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	path := filepath.Join(dir, "test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	return path
}

func TestExtractTarGzAndCompensate(t *testing.T) {
	dir := t.TempDir()
	archive := createTarGz(t, dir, map[string]string{
		"a.txt": "alpha",
		"b.txt": "bravo",
	})

	dest := filepath.Join(dir, "out")
	p := &Provider{}
	_, state, err := p.Extract(archive, dest)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	created, _ := state["created_files"].([]string)
	if len(created) != 2 {
		t.Fatalf("expected 2 created files, got %d: %v", len(created), created)
	}

	// Verify files exist
	for _, f := range created {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected file %s to exist: %v", f, err)
		}
	}

	// Compensate: removes created files
	if err := p.CompensateExtract(state); err != nil {
		t.Fatalf("CompensateExtract: %v", err)
	}

	for _, f := range created {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("file %s should have been removed", f)
		}
	}
}

func TestExtractZipAndCompensate(t *testing.T) {
	dir := t.TempDir()
	archive := createZip(t, dir, map[string]string{
		"x.txt": "xray",
		"y.txt": "yankee",
	})

	dest := filepath.Join(dir, "out")
	p := &Provider{}
	_, state, err := p.Extract(archive, dest)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	created, _ := state["created_files"].([]string)
	if len(created) != 2 {
		t.Fatalf("expected 2 created files, got %d: %v", len(created), created)
	}

	// Compensate
	if err := p.CompensateExtract(state); err != nil {
		t.Fatalf("CompensateExtract: %v", err)
	}

	for _, f := range created {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("file %s should have been removed", f)
		}
	}
}

func TestCompensateExtractNilState(t *testing.T) {
	p := &Provider{}
	if err := p.CompensateExtract(nil); err != nil {
		t.Errorf("CompensateExtract(nil): %v", err)
	}
}
