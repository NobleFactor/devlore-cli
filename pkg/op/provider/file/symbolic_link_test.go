// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// discoverSymbolicLink mints a *SymbolicLink for `path` in a fresh catalog-backed environment rooted at `root`.
func discoverSymbolicLink(t *testing.T, root, path string) *SymbolicLink {
	t.Helper()
	p := testProvider(t, root)
	link, err := DiscoverSymbolicLink(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("DiscoverSymbolicLink(%s): %v", path, err)
	}
	return link
}

// TestSymbolicLinkDigest_LiteralTarget pins ruling 5a: the digest is the sha256 of the verbatim readlink result —
// a relative target hashes as written, and retargeting flips the digest.
func TestSymbolicLinkDigest_LiteralTarget(t *testing.T) {

	root := t.TempDir()
	linkPath := filepath.Join(root, "link")
	mkTreeLink(t, "relative/target.txt", linkPath)

	digest, err := discoverSymbolicLink(t, root, linkPath).Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}

	want := sha256.Sum256([]byte("relative/target.txt"))
	if hex.EncodeToString(digest.Bytes) != hex.EncodeToString(want[:]) {
		t.Errorf("Digest = %x; want the hash of the literal target %x", digest.Bytes, want)
	}

	if err := os.Remove(linkPath); err != nil {
		t.Fatal(err)
	}
	mkTreeLink(t, "relative/other.txt", linkPath)

	retargeted, err := discoverSymbolicLink(t, root, linkPath).Digest()
	if err != nil {
		t.Fatalf("Digest after retarget: %v", err)
	}
	if hex.EncodeToString(retargeted.Bytes) == hex.EncodeToString(digest.Bytes) {
		t.Error("retargeting did not flip the digest")
	}
}

// TestSymbolicLinkDigestAndEtag_DanglingIsLegal pins the dangling-link contract: the link is the resource, not its
// referent — both Digest and Etag succeed where the catch-all's follow-based Etag errors (the ruling-5b defect fix).
func TestSymbolicLinkDigestAndEtag_DanglingIsLegal(t *testing.T) {

	root := t.TempDir()
	linkPath := filepath.Join(root, "dangling")
	mkTreeLink(t, filepath.Join(root, "never-created"), linkPath)

	link := discoverSymbolicLink(t, root, linkPath)

	if _, err := link.Digest(); err != nil {
		t.Errorf("Digest over a dangling link = %v; want success (the link is the resource)", err)
	}
	if _, err := link.Etag(); err != nil {
		t.Errorf("Etag over a dangling link = %v; want success (lstat semantics)", err)
	}

	// The contrast that motivated ruling 5b: the catch-all follows and therefore errors.
	p := testProvider(t, root)
	p.RuntimeEnvironment().ResourceCatalog = nil
	base, err := discoverResource(p.RuntimeEnvironment(), linkPath)
	if err != nil {
		t.Fatalf("DiscoverResource: %v", err)
	}
	if _, err := base.Etag(); err == nil {
		t.Error("the catch-all Etag succeeded over a dangling link; the ruling-5b contrast no longer holds")
	}
}

// TestSymbolicLinkKindMismatch pins ruling 5e: asserting SymbolicLink over a regular file errors with the kind
// mismatch on both Digest and Etag.
func TestSymbolicLinkKindMismatch(t *testing.T) {

	root := t.TempDir()
	path := filepath.Join(root, "plain.txt")
	mkTreeFile(t, path, "not a link")

	link := discoverSymbolicLink(t, root, path)

	if _, err := link.Digest(); !errors.Is(err, errKindMismatch) {
		t.Errorf("Digest over a regular file = %v; want the kind mismatch", err)
	}
	if _, err := link.Etag(); !errors.Is(err, errKindMismatch) {
		t.Errorf("Etag over a regular file = %v; want the kind mismatch", err)
	}
}
