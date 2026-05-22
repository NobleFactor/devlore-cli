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

// testProvider creates a Provider rooted at the given directory with a Catalog and RecoverySite.
func testProvider(t *testing.T, dir string) *Provider {
	t.Helper()
	root := op.NewRootReaderWriter(dir)
	ctx := &op.RuntimeEnvironment{Root: root, Catalog: op.NewResourceCatalog()}
	ctx.RecoverySite = op.NewRecoverySite(ctx)
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// testActivation returns an [op.ActivationRecord] that satisfies the strict producer contract: non-nil with a
// non-empty SiteID derived from the test name. Test producer calls pass this in lieu of the real per-dispatch
// activation that the framework would build.
// testActivation wraps ctx in an [op.ActivationRecord] for non-graph dispatch. Graph and Unit are
// nil — Resources produced through this activation carry an empty producer stamp; tests that need a
// specific producer stamp call [op.ResourceCatalog.Shadow] directly.
func testActivation(t *testing.T, ctx *op.RuntimeEnvironment) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, nil, ctx)
}

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
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverResource(op.NewActivationRecord(nil, nil, p.RuntimeEnvironment()), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	products, receipts, err := p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(products) != len(entries) {
		t.Errorf("products has %d entries, want %d", len(products), len(entries))
	}
	if len(receipts) != len(entries) {
		t.Errorf("receipts has %d entries, want %d", len(receipts), len(entries))
	}

	// Verify files exist with expected content.
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
}

// TestProducerStamp_Extract verifies the m.5(iii) contract for archive: Extract is a true producer (creates
// new file URIs at the destination), and each produced *file.Resource is stamped with the activation's
// SiteID via the file.NewResource(activation, ...) call inside Extract's loop.
func TestProducerStamp_Extract(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "stamp.tar.gz")
	createTarGz(t, archivePath, map[string]string{"a.txt": "alpha"})

	prefix := filepath.Join(tmp, "out")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverResource(op.NewActivationRecord(nil, nil, p.RuntimeEnvironment()), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	activation := testActivation(t, p.RuntimeEnvironment())
	products, _, err := p.Extract(activation, source, prefix)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Non-graph dispatch (testActivation has nil Unit) → Resources carry an empty producer stamp.
	for _, product := range products {
		if got := product.ProducerID(); got != "" {
			t.Errorf("producerID for %q = %q, want empty (nil Unit)", product.URI(), got)
		}
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
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverResource(op.NewActivationRecord(nil, nil, p.RuntimeEnvironment()), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	products, receipts, err := p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(products) != len(entries) {
		t.Errorf("products has %d entries, want %d", len(products), len(entries))
	}
	if len(receipts) != len(entries) {
		t.Errorf("receipts has %d entries, want %d", len(receipts), len(entries))
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
}

func TestExtractUnsupportedFormat(t *testing.T) {
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
	source, err := file.DiscoverResource(op.NewActivationRecord(nil, nil, p.RuntimeEnvironment()), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix); err == nil {
		t.Error("expected error for unsupported archive format")
	}
}

func TestZipSlipProtectionTarGz(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "evil.tar.gz")
	entries := map[string]string{
		"../escape.txt": "escaped content",
		"safe.txt":      "safe content",
	}
	createTarGz(t, archivePath, entries)

	prefix := filepath.Join(tmp, "out")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverResource(op.NewActivationRecord(nil, nil, p.RuntimeEnvironment()), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix); err != nil {
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

func TestZipSlipProtectionZip(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "evil.zip")
	entries := map[string]string{
		"../escape.txt": "escaped content",
		"safe.txt":      "safe content",
	}
	createZip(t, archivePath, entries)

	prefix := filepath.Join(tmp, "out")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverResource(op.NewActivationRecord(nil, nil, p.RuntimeEnvironment()), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix); err != nil {
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

func TestExtractProducesFileReceiptsWithBoundary(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.tar.gz")
	entries := map[string]string{
		"x.txt":     "x",
		"sub/y.txt": "y",
	}
	createTarGz(t, archivePath, entries)

	prefix := filepath.Join(tmp, "out")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverResource(op.NewActivationRecord(nil, nil, p.RuntimeEnvironment()), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	_, receipts, err := p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	for i, r := range receipts {
		fr, ok := r.(*file.Receipt)
		if !ok {
			t.Errorf("receipts[%d] is %T, want *file.Receipt", i, r)
			continue
		}
		if fr.Boundary() == nil {
			t.Errorf("receipts[%d].Boundary() is nil; expected the destination directory", i)
		}
	}
}

func TestExtract_CompensateExtract_RoundTrip_NewFiles(t *testing.T) {

	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.tar.gz")
	entries := map[string]string{
		"hello.txt":     "hello",
		"sub/world.txt": "world",
	}
	createTarGz(t, archivePath, entries)

	prefix := filepath.Join(tmp, "out")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverResource(op.NewActivationRecord(nil, nil, p.RuntimeEnvironment()), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	products, receipts, err := p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	for _, product := range products {
		if _, statErr := os.Stat(product.SourcePath.Abs()); statErr != nil {
			t.Errorf("expected extracted file %q to exist after Extract: %v", product.SourcePath.Abs(), statErr)
		}
	}

	for i, r := range receipts {
		fr, ok := r.(*file.Receipt)
		if !ok {
			t.Fatalf("receipts[%d] is %T, want *file.Receipt", i, r)
		}
		if compensateErr := p.CompensateExtract(fr); compensateErr != nil {
			t.Errorf("CompensateExtract receipts[%d]: %v", i, compensateErr)
		}
	}

	for _, product := range products {
		if _, statErr := os.Stat(product.SourcePath.Abs()); !os.IsNotExist(statErr) {
			t.Errorf("extracted file %q should be removed after compensation; stat error = %v", product.SourcePath.Abs(), statErr)
		}
	}
}