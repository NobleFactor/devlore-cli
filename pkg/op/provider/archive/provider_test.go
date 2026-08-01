// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	// The blank gen import registers file.Provider so Instance + the compensator index resolve.
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
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
	return op.NewActivationRecord(nil, "", runtimeEnvironment)
}

// createTar builds an uncompressed (plain) tar archive at archivePath containing the given file entries.
func createTar(t *testing.T, archivePath string, entries map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive file: %v", err)
	}
	defer func() { _ = f.Close() }()

	writeTarEntries(t, f, entries)
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

	writeTarEntries(t, gw, entries)
}

// writeTarEntries writes the given file entries (relative path → content) as tar entries on `w`, closing the tar
// writer (and thereby flushing the tar footer) before returning.
func writeTarEntries(t *testing.T, w io.Writer, entries map[string]string) {
	t.Helper()

	tw := tar.NewWriter(w)
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

// createTarXz builds a .tar.xz archive at archivePath containing the given file entries.
func createTarXz(t *testing.T, archivePath string, entries map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive file: %v", err)
	}
	defer func() { _ = f.Close() }()

	xw, err := xz.NewWriter(f)
	if err != nil {
		t.Fatalf("xz writer: %v", err)
	}
	defer func() { _ = xw.Close() }()

	writeTarEntries(t, xw, entries)
}

// createTarZst builds a .tar.zst archive at archivePath containing the given file entries.
func createTarZst(t *testing.T, archivePath string, entries map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive file: %v", err)
	}
	defer func() { _ = f.Close() }()

	zw, err := zstd.NewWriter(f)
	if err != nil {
		t.Fatalf("zstd writer: %v", err)
	}
	defer func() { _ = zw.Close() }()

	writeTarEntries(t, zw, entries)
}

// tarBz2Fixture is a pre-built .tar.bz2 holding hello.txt = "bzip2 payload" — the Go standard library decompresses
// bzip2 but cannot compress it, so the fixture is embedded (generated once via Python's tarfile, mode w:bz2).
const tarBz2Fixture = "QlpoOTFBWSZTWbrE4qEAAHH7gMqAAgBAAXeAAIB2ZN5wCAggAFQ0kyGjQGIaNBvVBJRNDQMgAAH3NA1CDFyEIhxF5JS+ZAhgMI7wcJzBGDUD15zwYyEbVqF72njrcz8Kh7FtCfqaWGCIgH4u5IpwoSF1icVC"

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
func extractInto(t *testing.T, tmp, archivePath string) (*Provider, string, []file.Entry, *op.RecoveryStack) {
	t.Helper()

	prefix := filepath.Join(tmp, "out")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverRegular(p.RuntimeEnvironment(), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	products, stack, err := p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return p, prefix, products, stack
}

// extractIntoExpectingError runs Extract into a fresh prefix, returning the extraction error.
func extractIntoExpectingError(t *testing.T, tmp, archivePath string) error {
	t.Helper()

	prefix := filepath.Join(tmp, "out")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverRegular(p.RuntimeEnvironment(), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix)
	return err
}

// createTarGzHeaders builds a .tar.gz at archivePath from explicit tar headers (special entry kinds), with
// `bodies` supplying regular-file contents by name.
func createTarGzHeaders(t *testing.T, archivePath string, headers []tar.Header, bodies map[string]string) {
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

	for i := range headers {
		hdr := headers[i]
		if body, ok := bodies[hdr.Name]; ok {
			hdr.Size = int64(len(body))
		}
		if err := tw.WriteHeader(&hdr); err != nil {
			t.Fatalf("write tar header %q: %v", hdr.Name, err)
		}
		if body, ok := bodies[hdr.Name]; ok {
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatalf("write tar content %q: %v", hdr.Name, err)
			}
		}
	}
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

// TestExtract_ProducerStamp verifies that under discovery (empty caller id) the produced Resources carry an empty
// producer stamp.
func TestExtract_ProducerStamp(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "stamp.tar.gz")
	createTarGz(t, archivePath, map[string]string{"a.txt": "alpha"})

	_, _, products, _ := extractInto(t, tmp, archivePath)

	for _, product := range products {
		if got := product.ProducerID(); got != "" {
			t.Errorf("producerID for %q = %q, want empty (no caller id)", product.URI(), got)
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

func TestExtract_PlainTar(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.tar")
	entries := map[string]string{"dir/hello.txt": "hello", "root.txt": "root"}
	createTar(t, archivePath, entries)

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
	source, err := file.DiscoverRegular(p.RuntimeEnvironment(), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix); err == nil {
		t.Error("expected error for unsupported archive format")
	}
}

// TestExtract_DetectsMisnamedTarGz proves detection reads content, not names: a tar.gz with no recognizable extension
// still extracts.
func TestExtract_DetectsMisnamedTarGz(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "release-asset")
	createTarGz(t, archivePath, map[string]string{"a.txt": "alpha"})

	_, prefix, _, _ := extractInto(t, tmp, archivePath)

	got, err := os.ReadFile(filepath.Join(prefix, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if string(got) != "alpha" {
		t.Errorf("content = %q, want %q", got, "alpha")
	}
}

// TestExtract_DetectsZipNamedTarGz proves a mislabeled archive routes by content: a zip named .tar.gz takes the zip
// branch instead of failing at the gzip header.
func TestExtract_DetectsZipNamedTarGz(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "mislabeled.tar.gz")
	createZip(t, archivePath, map[string]string{"b.txt": "bravo"})

	_, prefix, _, _ := extractInto(t, tmp, archivePath)

	got, err := os.ReadFile(filepath.Join(prefix, "b.txt"))
	if err != nil {
		t.Fatalf("read b.txt: %v", err)
	}
	if string(got) != "bravo" {
		t.Errorf("content = %q, want %q", got, "bravo")
	}
}

// TestExtract_DetectedButUnsupportedCompression asserts a recognized compression magic whose decompressor has not
// landed yet fails with an error naming the detected format.
// TestExtract_TarBz2 pins the bzip2 branch against the embedded fixture (misnamed on purpose: content detection,
// never the filename, chooses the decompressor).
func TestExtract_TarBz2(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "misnamed.bin")
	raw, err := base64.StdEncoding.DecodeString(tarBz2Fixture)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if err := os.WriteFile(archivePath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	_, prefix, products, _ := extractInto(t, tmp, archivePath)

	if len(products) != 1 {
		t.Fatalf("products has %d entries, want 1", len(products))
	}
	got, err := os.ReadFile(filepath.Join(prefix, "hello.txt"))
	if err != nil || string(got) != "bzip2 payload" {
		t.Errorf("hello.txt = %q (err %v), want %q", got, err, "bzip2 payload")
	}
}

// TestExtract_TarXz pins the xz branch end to end.
func TestExtract_TarXz(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.tar.xz")
	entries := map[string]string{"dir/hello.txt": "xz payload"}
	createTarXz(t, archivePath, entries)

	_, prefix, products, _ := extractInto(t, tmp, archivePath)

	if len(products) != 1 {
		t.Fatalf("products has %d entries, want 1", len(products))
	}
	got, err := os.ReadFile(filepath.Join(prefix, "dir", "hello.txt"))
	if err != nil || string(got) != "xz payload" {
		t.Errorf("dir/hello.txt = %q (err %v), want %q", got, err, "xz payload")
	}
}

// TestExtract_TarZst pins the zstd branch end to end.
func TestExtract_TarZst(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.tar.zst")
	entries := map[string]string{"dir/hello.txt": "zstd payload"}
	createTarZst(t, archivePath, entries)

	_, prefix, products, _ := extractInto(t, tmp, archivePath)

	if len(products) != 1 {
		t.Fatalf("products has %d entries, want 1", len(products))
	}
	got, err := os.ReadFile(filepath.Join(prefix, "dir", "hello.txt"))
	if err != nil || string(got) != "zstd payload" {
		t.Errorf("dir/hello.txt = %q (err %v), want %q", got, err, "zstd payload")
	}
}

// TestExtract_EscapingNameErrorsTarGz pins §10 ruling 3 layer 1: escape intent is a hard error naming the entry —
// never a silent skip — and nothing lands outside the prefix.
func TestExtract_EscapingNameErrorsTarGz(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "evil.tar.gz")
	createTarGz(t, archivePath, map[string]string{"../escape.txt": "escaped"})

	err := extractIntoExpectingError(t, tmp, archivePath)
	if err == nil || !strings.Contains(err.Error(), "escapes the extraction prefix") {
		t.Fatalf("escaping entry = %v; want the escape refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "escape.txt")); statErr == nil {
		t.Error("zip slip: file escaped prefix directory")
	}
}

// TestExtract_EscapingNameErrorsZip pins the same refusal on the zip path.
func TestExtract_EscapingNameErrorsZip(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "evil.zip")
	createZip(t, archivePath, map[string]string{"../escape.txt": "escaped"})

	err := extractIntoExpectingError(t, tmp, archivePath)
	if err == nil || !strings.Contains(err.Error(), "escapes the extraction prefix") {
		t.Fatalf("escaping entry = %v; want the escape refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "escape.txt")); statErr == nil {
		t.Error("zip slip: file escaped prefix directory")
	}
}

// TestExtract_PreexistingSymlinkDivergenceErrors pins §10 ruling 3 layer 2: a pre-existing symlink diverting an
// entry's path is detected (lexical vs. resolved divergence) and errors — never a silent redirect.
func TestExtract_PreexistingSymlinkDivergenceErrors(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.tar.gz")
	createTarGz(t, archivePath, map[string]string{"sub/payload.txt": "payload"})

	prefix := filepath.Join(tmp, "out")
	outside := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(prefix, "sub")); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	source, err := file.DiscoverRegular(p.RuntimeEnvironment(), archivePath)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = p.Extract(testActivation(t, p.RuntimeEnvironment()), source, prefix)
	if err == nil || !strings.Contains(err.Error(), "traverses a symlink") {
		t.Fatalf("divergence = %v; want the symlink-traversal refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "payload.txt")); statErr == nil {
		t.Error("payload landed through the pre-existing symlink")
	}
}

// TestExtract_TarSymlink pins §10 ruling 1a: a tar symlink extracts as a tracked link with its VERBATIM relative
// target — on disk and therefore in the SymbolicLink digest — and compensation removes it.
func TestExtract_TarSymlink(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "links.tar.gz")
	createTarGzHeaders(t, archivePath, []tar.Header{
		{Name: "hello.txt", Typeflag: tar.TypeReg, Mode: 0o644},
		{Name: "alias", Typeflag: tar.TypeSymlink, Linkname: "hello.txt", Mode: 0o777},
	}, map[string]string{"hello.txt": "hello"})

	p, prefix, products, stack := extractInto(t, tmp, archivePath)

	if len(products) != 2 {
		t.Fatalf("products has %d entries, want 2 (file + link)", len(products))
	}

	target, err := os.Readlink(filepath.Join(prefix, "alias"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "hello.txt" {
		t.Errorf("link target = %q, want the verbatim %q", target, "hello.txt")
	}

	if err := p.CompensateExtract(testActivation(t, p.RuntimeEnvironment()), stack); err != nil {
		t.Fatalf("CompensateExtract: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(prefix, "alias")); !os.IsNotExist(err) {
		t.Errorf("link survives compensation (err %v)", err)
	}
}

// TestExtract_SymlinkTargetEscapes pins ruling 1a's containment: an escaping or absolute link target is a hard
// error naming the entry.
func TestExtract_SymlinkTargetEscapes(t *testing.T) {
	tmp := t.TempDir()

	for name, header := range map[string]tar.Header{
		"escaping": {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../../etc/passwd", Mode: 0o777},
		"absolute": {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777},
	} {
		archivePath := filepath.Join(tmp, name+".tar.gz")
		createTarGzHeaders(t, archivePath, []tar.Header{header}, nil)

		err := extractIntoExpectingError(t, filepath.Join(tmp, name), archivePath)
		if err == nil || !strings.Contains(err.Error(), "symlink target") {
			t.Errorf("%s target = %v; want the symlink-target refusal", name, err)
		}
	}
}

// TestExtract_Hardlink pins ruling 1b: a hardlink entry materializes as a content copy of the already-extracted
// referent; a missing referent errors naming both paths.
func TestExtract_Hardlink(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "hard.tar.gz")
	createTarGzHeaders(t, archivePath, []tar.Header{
		{Name: "original.txt", Typeflag: tar.TypeReg, Mode: 0o644},
		{Name: "alias.txt", Typeflag: tar.TypeLink, Linkname: "original.txt", Mode: 0o644},
	}, map[string]string{"original.txt": "shared bytes"})

	_, prefix, products, _ := extractInto(t, tmp, archivePath)

	if len(products) != 2 {
		t.Fatalf("products has %d entries, want 2", len(products))
	}
	got, err := os.ReadFile(filepath.Join(prefix, "alias.txt"))
	if err != nil || string(got) != "shared bytes" {
		t.Errorf("hardlink copy = %q (err %v), want %q", got, err, "shared bytes")
	}

	missingPath := filepath.Join(tmp, "missing.tar.gz")
	createTarGzHeaders(t, missingPath, []tar.Header{
		{Name: "alias.txt", Typeflag: tar.TypeLink, Linkname: "never-extracted.txt", Mode: 0o644},
	}, nil)
	err = extractIntoExpectingError(t, filepath.Join(tmp, "missing"), missingPath)
	if err == nil || !strings.Contains(err.Error(), "hardlink referent") {
		t.Errorf("missing referent = %v; want the referent refusal", err)
	}
}

// TestExtract_DeviceEntryErrors pins ruling 1c: a device entry is a loud refusal naming the kind, not a skip.
func TestExtract_DeviceEntryErrors(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "device.tar.gz")
	createTarGzHeaders(t, archivePath, []tar.Header{
		{Name: "null", Typeflag: tar.TypeChar, Mode: 0o666},
	}, nil)

	err := extractIntoExpectingError(t, tmp, archivePath)
	if err == nil || !strings.Contains(err.Error(), "character device") {
		t.Fatalf("device entry = %v; want the unsupported-kind refusal naming it", err)
	}
}

// TestExtract_ZipSymlink pins the zip half of ruling 1a: a zip symlink entry extracts as a link (its body used to
// be silently written out as a regular FILE).
func TestExtract_ZipSymlink(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "links.zip")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	if w, err := zw.Create("hello.txt"); err != nil {
		t.Fatal(err)
	} else if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	linkHeader := &zip.FileHeader{Name: "alias"}
	linkHeader.SetMode(os.ModeSymlink | 0o777)
	if w, err := zw.CreateHeader(linkHeader); err != nil {
		t.Fatal(err)
	} else if _, err := w.Write([]byte("hello.txt")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	_, prefix, _, _ := extractInto(t, tmp, archivePath)

	target, err := os.Readlink(filepath.Join(prefix, "alias"))
	if err != nil || target != "hello.txt" {
		t.Errorf("zip symlink target = %q (err %v), want %q", target, err, "hello.txt")
	}
}

// TestExtract_GzippedGarbageDiagnostics pins §10 ruling 4: a compressed payload that is not a tar names the
// detected outer format and the missing container.
func TestExtract_GzippedGarbageDiagnostics(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "garbage.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	if _, err := gw.Write([]byte("just some text, no tar here")); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	err = extractIntoExpectingError(t, tmp, archivePath)
	if err == nil || !strings.Contains(err.Error(), "gzip-compressed payload is not a tar archive") {
		t.Fatalf("gzipped garbage = %v; want the format-naming diagnostic", err)
	}
}

// TestExtract_GzippedEmptyDiagnostics pins ruling 4's empty-payload case: an empty decompressed payload errors
// instead of succeeding as a zero-entry extraction.
func TestExtract_GzippedEmptyDiagnostics(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "empty.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	err = extractIntoExpectingError(t, tmp, archivePath)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("gzipped empty = %v; want the empty-payload diagnostic", err)
	}
}

// TestExtractStream_TarGz pins the stream entry point over the tar family: the sniffed prefix stitches back and
// the extraction matches the disk path.
func TestExtractStream_TarGz(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.tar.gz")
	createTarGz(t, archivePath, map[string]string{"dir/hello.txt": "stream hello"})

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	prefix := filepath.Join(tmp, "out")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	products, _, err := p.ExtractStream(testActivation(t, p.RuntimeEnvironment()), f, prefix)
	if err != nil {
		t.Fatalf("ExtractStream: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("products has %d entries, want 1", len(products))
	}
	got, err := os.ReadFile(filepath.Join(prefix, "dir", "hello.txt"))
	if err != nil || string(got) != "stream hello" {
		t.Errorf("dir/hello.txt = %q (err %v), want %q", got, err, "stream hello")
	}
}

// TestExtractStream_Zip pins the stream zip path: the forward-only stream spools to a temporary file (§10 ruling
// 5's escape hatch) and extracts via the random-access reader.
func TestExtractStream_Zip(t *testing.T) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "test.zip")
	createZip(t, archivePath, map[string]string{"hello.txt": "spooled hello"})

	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	prefix := filepath.Join(tmp, "out")
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)
	products, _, err := p.ExtractStream(testActivation(t, p.RuntimeEnvironment()), f, prefix)
	if err != nil {
		t.Fatalf("ExtractStream: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("products has %d entries, want 1", len(products))
	}
	got, err := os.ReadFile(filepath.Join(prefix, "hello.txt"))
	if err != nil || string(got) != "spooled hello" {
		t.Errorf("hello.txt = %q (err %v), want %q", got, err, "spooled hello")
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
		if _, err := os.Stat(product.Path().Abs()); err != nil {
			t.Errorf("expected extracted file %q to exist after Extract: %v", product.Path().Abs(), err)
		}
	}

	if err := p.CompensateExtract(testActivation(t, p.RuntimeEnvironment()), stack); err != nil {
		t.Fatalf("CompensateExtract: %v", err)
	}

	for _, product := range products {
		if _, err := os.Stat(product.Path().Abs()); !os.IsNotExist(err) {
			t.Errorf("extracted file %q should be removed after compensation; stat err = %v", product.Path().Abs(), err)
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
	source, err := file.DiscoverRegular(p.RuntimeEnvironment(), archivePath)
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

	if err := p.CompensateExtract(testActivation(t, p.RuntimeEnvironment()), stack); err != nil {
		t.Fatalf("CompensateExtract: %v", err)
	}

	if got, _ := os.ReadFile(existing); string(got) != "old" {
		t.Errorf("after compensate content = %q; want %q (prior content restored)", got, "old")
	}
}

// --- detectFormat ---

func TestDetectFormat(t *testing.T) {
	ustarHeader := make([]byte, headerSniffLen)
	copy(ustarHeader[tarMagicOffset:], "ustar")

	cases := []struct {
		name    string
		content []byte
		want    archiveFormat
	}{
		{"gzip", []byte{0x1F, 0x8B}, formatGzip},
		{"bzip2", []byte("BZh9"), formatBzip2},
		{"xz", []byte{0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00}, formatXz},
		{"zstd", []byte{0x28, 0xB5, 0x2F, 0xFD}, formatZstd},
		{"zip", []byte{0x50, 0x4B, 0x03, 0x04}, formatZip},
		{"zip empty", []byte{0x50, 0x4B, 0x05, 0x06}, formatZip},
		{"zip spanned", []byte{0x50, 0x4B, 0x07, 0x08}, formatZip},
		{"tar ustar", ustarHeader, formatTar},
		{"unknown", []byte("plain text, no magic"), formatUnknown},
		{"empty", nil, formatUnknown},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sniff")
			if err := os.WriteFile(path, testCase.content, 0o644); err != nil {
				t.Fatal(err)
			}
			archiveFile, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = archiveFile.Close() }()

			got, err := detectFormat(archiveFile)
			if err != nil {
				t.Fatalf("detectFormat: %v", err)
			}
			if got != testCase.want {
				t.Errorf("detectFormat = %v, want %v", got, testCase.want)
			}

			offset, err := archiveFile.Seek(0, io.SeekCurrent)
			if err != nil {
				t.Fatal(err)
			}
			if offset != 0 {
				t.Errorf("detectFormat left the file at offset %d, want 0 (rewound)", offset)
			}
		})
	}
}
