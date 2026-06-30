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

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen" // registers file.Provider so Instance + the compensator index resolve
)

// testProvider creates a Provider rooted at the given directory with a Catalog and RecoverySite.
func testProvider(t *testing.T, dir string) *Provider {
	t.Helper()
	root := fsroot.OpenWritableUnconfined(dir)
	runtimeEnvironment := &op.RuntimeEnvironment{
		Root:            root,
		ResourceCatalog: op.NewResourceCatalog(),
	}
	runtimeEnvironment.RecoverySite = op.NewRecoverySite(runtimeEnvironment)
	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// testActivation wraps `runtimeEnvironment` in an [*op.ActivationRecord] for non-graph dispatch.
//
// `Graph` and `Unit` are both nil — Resources produced through this activation carry an empty producer stamp.
func testActivation(t *testing.T, runtimeEnvironment *op.RuntimeEnvironment) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, nil, runtimeEnvironment)
}

// createTarGz builds a tar.gz archive at archivePath containing the given file entries (relative path → content).
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
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header %q: %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("write tar content %q: %v", name, err)
		}
	}
}

// createZip builds a zip archive at archivePath containing the given file entries.
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

// extractInto creates a fresh `out` prefix under tmp, discovers the source archive, and runs Extract.
func extractInto(t *testing.T, tmp, archivePath string) (*Provider, string, []*file.Resource, *op.RecoveryStack) {
	t.Helper()

	prefix := filepath.Join(tmp, "out")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverResource(p.RuntimeEnvironment(), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	products, stack, err := p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return p, prefix, products, stack
}

// --- Extract ---

func TestExtract_TarGz(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.tar.gz")
	entries := map[string]string{"dir/hello.txt": "hello", "dir/goodbye.txt": "goodbye"}
	createTarGz(t, archivePath, entries)

	_, prefix, products, _ := extractInto(t, tmp, archivePath)

	if len(products) != len(entries) {
		t.Errorf("products has %d entries, want %d", len(products), len(entries))
	}
	for name, wantContent := range entries {
		got, err := os.ReadFile(filepath.Join(prefix, name))
		if err != nil {
			t.Errorf("read %q: %v", name, err)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("content of %q = %q, want %q", name, got, wantContent)
		}
	}
}

// TestExtract_ProducerStamp verifies that under non-graph dispatch (nil Unit) the produced Resources carry an empty
// producer stamp.
func TestExtract_ProducerStamp(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "stamp.tar.gz")
	createTarGz(t, archivePath, map[string]string{"a.txt": "alpha"})

	_, _, products, _ := extractInto(t, tmp, archivePath)

	for _, product := range products {
		if got := product.ProducerID(); got != "" {
			t.Errorf("producerID for %q = %q, want empty (nil Unit)", product.URI(), got)
		}
	}
}

func TestExtract_Zip(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.zip")
	entries := map[string]string{"sub/a.txt": "alpha", "sub/b.txt": "bravo"}
	createZip(t, archivePath, entries)

	_, prefix, products, _ := extractInto(t, tmp, archivePath)

	if len(products) != len(entries) {
		t.Errorf("products has %d entries, want %d", len(products), len(entries))
	}
	for name, wantContent := range entries {
		got, err := os.ReadFile(filepath.Join(prefix, name))
		if err != nil {
			t.Errorf("read %q: %v", name, err)
			continue
		}
		if string(got) != wantContent {
			t.Errorf("content of %q = %q, want %q", name, got, wantContent)
		}
	}
}

func TestExtract_UnsupportedFormat(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.unknown")
	if err := os.WriteFile(archivePath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(tmp, "out")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverResource(p.RuntimeEnvironment(), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix); err == nil {
		t.Error("expected error for unsupported archive format")
	}
}

func TestExtract_ZipSlipProtectionTarGz(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "evil.tar.gz")
	createTarGz(t, archivePath, map[string]string{"../escape.txt": "escaped", "safe.txt": "safe"})

	_, prefix, _, _ := extractInto(t, tmp, archivePath)

	if _, err := os.Stat(filepath.Join(tmp, "escape.txt")); err == nil {
		t.Error("zip slip: file escaped prefix directory")
	}
	if _, err := os.Stat(filepath.Join(prefix, "safe.txt")); err != nil {
		t.Errorf("safe.txt not found: %v", err)
	}
}

func TestExtract_ZipSlipProtectionZip(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "evil.zip")
	createZip(t, archivePath, map[string]string{"../escape.txt": "escaped", "safe.txt": "safe"})

	_, prefix, _, _ := extractInto(t, tmp, archivePath)

	if _, err := os.Stat(filepath.Join(tmp, "escape.txt")); err == nil {
		t.Error("zip slip: file escaped prefix directory")
	}
	if _, err := os.Stat(filepath.Join(prefix, "safe.txt")); err != nil {
		t.Errorf("safe.txt not found: %v", err)
	}
}

// --- CompensateExtract ---

// TestCompensateExtract_RoundTrip_NewFiles extracts brand-new files (and a created subdirectory), then asserts
// compensation removes the files and prunes the directory the extraction created.
func TestCompensateExtract_RoundTrip_NewFiles(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.tar.gz")
	createTarGz(t, archivePath, map[string]string{"hello.txt": "hello", "sub/world.txt": "world"})

	p, prefix, products, stack := extractInto(t, tmp, archivePath)

	for _, product := range products {
		if _, err := os.Stat(product.SourcePath.Abs()); err != nil {
			t.Errorf("expected extracted file %q to exist after Extract: %v", product.SourcePath.Abs(), err)
		}
	}

	if err := p.CompensateExtract(stack); err != nil {
		t.Fatalf("CompensateExtract: %v", err)
	}

	for _, product := range products {
		if _, err := os.Stat(product.SourcePath.Abs()); !os.IsNotExist(err) {
			t.Errorf("extracted file %q should be removed after compensation; stat err = %v", product.SourcePath.Abs(), err)
		}
	}
	if _, err := os.Stat(filepath.Join(prefix, "sub")); !os.IsNotExist(err) {
		t.Errorf("created subdirectory sub/ should be pruned after compensation; stat err = %v", err)
	}
}

// TestCompensateExtract_RoundTrip_DisplacedFiles is the #277 proof: extracting over an existing file archives the prior
// content, and compensation restores it (the old archive recorded the recovery id but never threaded it onto the
// receipt, so compensation was a no-op).
func TestCompensateExtract_RoundTrip_DisplacedFiles(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.tar.gz")
	createTarGz(t, archivePath, map[string]string{"hello.txt": "new"})

	prefix := filepath.Join(tmp, "out")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(prefix, "hello.txt")
	if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverResource(p.RuntimeEnvironment(), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	_, stack, err := p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if got, _ := os.ReadFile(existing); string(got) != "new" {
		t.Fatalf("after extract content = %q; want %q", got, "new")
	}

	if err := p.CompensateExtract(stack); err != nil {
		t.Fatalf("CompensateExtract: %v", err)
	}

	if got, _ := os.ReadFile(existing); string(got) != "old" {
		t.Errorf("after compensate content = %q; want %q (prior content restored)", got, "old")
	}
}
