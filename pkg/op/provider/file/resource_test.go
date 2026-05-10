// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Interface guards ---

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

func TestReceiptImplementsInterface(t *testing.T) {
	var _ op.Receipt = (*Receipt)(nil)
}

// --- Addressing ---

func TestResource_Addressing_IsLocation(t *testing.T) {

	tmp := t.TempDir()
	p := testProvider(t, tmp)

	r, err := NewResource(p.RuntimeEnvironment(), filepath.Join(tmp, "anything.txt"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	if got := r.Addressing(); got != op.AddressingLocation {
		t.Errorf("Addressing() = %v, want AddressingLocation", got)
	}
}

// --- Digest ---

func TestResource_Digest_MatchesContent(t *testing.T) {

	tmp := t.TempDir()
	path := filepath.Join(tmp, "content.txt")

	content := []byte("the quick brown fox")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)

	r, err := NewResource(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	got, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}

	want := sha256.Sum256(content)
	expected := op.Digest{Algorithm: "sha256", Bytes: want[:]}

	if !got.Equal(expected) {
		t.Errorf("Digest = %s, want %s", got.String(), expected.String())
	}
}

func TestResource_Digest_StableAcrossCalls(t *testing.T) {

	tmp := t.TempDir()
	path := filepath.Join(tmp, "stable.txt")

	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)

	r, err := NewResource(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	first, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (first): %v", err)
	}

	second, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (second): %v", err)
	}

	if !first.Equal(second) {
		t.Errorf("two Digest calls disagree: %s vs %s", first.String(), second.String())
	}
}

func TestResource_Digest_DiffersAcrossContent(t *testing.T) {

	tmp := t.TempDir()
	pathA := filepath.Join(tmp, "a.txt")
	pathB := filepath.Join(tmp, "b.txt")

	if err := os.WriteFile(pathA, []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, []byte("beta"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)

	rA, err := NewResource(p.RuntimeEnvironment(), pathA)
	if err != nil {
		t.Fatalf("NewResource(A): %v", err)
	}
	rB, err := NewResource(p.RuntimeEnvironment(), pathB)
	if err != nil {
		t.Fatalf("NewResource(B): %v", err)
	}

	dA, err := rA.Digest()
	if err != nil {
		t.Fatalf("Digest(A): %v", err)
	}
	dB, err := rB.Digest()
	if err != nil {
		t.Fatalf("Digest(B): %v", err)
	}

	if dA.Equal(dB) {
		t.Errorf("digests for different content collided: %s", dA.String())
	}
}

func TestResource_Digest_DirectoryReturnsErrUnimplemented(t *testing.T) {

	tmp := t.TempDir()
	dirPath := filepath.Join(tmp, "subdir")

	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)

	r, err := NewResource(p.RuntimeEnvironment(), dirPath)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	_, err = r.Digest()
	if err == nil {
		t.Fatal("Digest of directory succeeded; want ErrUnimplemented")
	}
	if !errors.Is(err, op.ErrUnimplemented) {
		t.Errorf("Digest of directory error = %v, want ErrUnimplemented", err)
	}
}

func TestResource_Digest_FileMissing(t *testing.T) {

	tmp := t.TempDir()
	p := testProvider(t, tmp)

	r, err := NewResource(p.RuntimeEnvironment(), filepath.Join(tmp, "missing.txt"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	_, err = r.Digest()
	if err == nil {
		t.Fatal("Digest on missing file succeeded; want error")
	}
	if errors.Is(err, op.ErrUnimplemented) {
		t.Errorf("Digest on missing file returned ErrUnimplemented; want a stat-error")
	}
}

// --- Etag ---

func TestResource_Etag_StableAcrossCalls(t *testing.T) {

	tmp := t.TempDir()
	path := filepath.Join(tmp, "stable.txt")

	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)

	r, err := NewResource(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	first, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag (first): %v", err)
	}

	second, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag (second): %v", err)
	}

	if first != second {
		t.Errorf("two Etag calls on unchanged file disagree: %q vs %q", first, second)
	}
}

func TestResource_Etag_ChangesOnTouch(t *testing.T) {

	tmp := t.TempDir()
	path := filepath.Join(tmp, "touched.txt")

	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)

	r, err := NewResource(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	before, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag (before): %v", err)
	}

	// Re-write with different bytes — size and mtime both change.
	if err := os.WriteFile(path, []byte("hello, world"), 0o644); err != nil {
		t.Fatal(err)
	}

	after, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag (after): %v", err)
	}

	if before == after {
		t.Errorf("Etag did not change after rewrite: %q", before)
	}
}

func TestResource_Etag_FileMissing(t *testing.T) {

	tmp := t.TempDir()
	p := testProvider(t, tmp)

	r, err := NewResource(p.RuntimeEnvironment(), filepath.Join(tmp, "missing.txt"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	if _, err := r.Etag(); err == nil {
		t.Error("Etag on missing file succeeded; want error")
	}
}

// TestResource_Etag_FreshNotCached confirms Etag reads stat at call time and does not depend on
// previously-resolved cached fields. Constructs a Resource without calling Resolve() — Size/ModTime/Inode
// remain zero — and verifies Etag still returns a meaningful value (i.e., it actually stats).
func TestResource_Etag_FreshNotCached(t *testing.T) {

	tmp := t.TempDir()
	path := filepath.Join(tmp, "fresh.txt")

	if err := os.WriteFile(path, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Backdate mtime to ensure the value differs from the zero time.
	pastTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(path, pastTime, pastTime); err != nil {
		t.Fatal(err)
	}

	p := testProvider(t, tmp)

	r, err := NewResource(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	// Confirm Resolve was not called — cached snapshot is empty.
	if !r.ModTime.IsZero() {
		t.Fatal("test setup error: r.ModTime was unexpectedly populated")
	}

	etag, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag: %v", err)
	}
	if etag == "" {
		t.Error("Etag returned empty string for a real file")
	}
}