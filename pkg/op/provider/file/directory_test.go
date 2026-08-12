// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// mkTreeFile writes `content` at `path`, creating parents.
func mkTreeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mkTreeLink creates a symlink at `link` pointing at `target`, creating parents.
func mkTreeLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// discoverDirectory mints a *Directory for `path` in a fresh catalog-backed environment rooted at `root`.
func discoverDirectory(t *testing.T, root, path string) *Directory {
	t.Helper()
	p := testProvider(t, root)
	directory, err := DiscoverDirectory(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("DiscoverDirectory(%s): %v", path, err)
	}
	return directory
}

// digestOf returns the Merkle root of `path` under a fresh environment rooted at `root`.
func digestOf(t *testing.T, root, path string) op.Digest {
	t.Helper()
	digest, err := discoverDirectory(t, root, path).Digest()
	if err != nil {
		t.Fatalf("Directory.Digest(%s): %v", path, err)
	}
	return digest
}

// TestDirectoryDigest_IdenticalTreesAgree pins location independence: two identically shaped trees at different
// absolute paths produce the same Merkle root — only the tree's own shape and content participate.
func TestDirectoryDigest_IdenticalTreesAgree(t *testing.T) {

	root := t.TempDir()
	for _, tree := range []string{"a", "b"} {
		mkTreeFile(t, filepath.Join(root, tree, "top.txt"), "alpha")
		mkTreeFile(t, filepath.Join(root, tree, "sub", "nested.txt"), "beta")
		mkTreeLink(t, "top.txt", filepath.Join(root, tree, "link"))
	}

	first := digestOf(t, root, filepath.Join(root, "a"))
	second := digestOf(t, root, filepath.Join(root, "b"))

	if first.String() != second.String() {
		t.Errorf("identical trees diverge: %s vs %s", first, second)
	}
}

// TestDirectoryDigest_ContentChangeFlipsRoot pins content sensitivity through the recursion: editing a nested file
// flips the top-level root.
func TestDirectoryDigest_ContentChangeFlipsRoot(t *testing.T) {

	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	mkTreeFile(t, filepath.Join(tree, "sub", "nested.txt"), "before")

	before := digestOf(t, root, tree)
	mkTreeFile(t, filepath.Join(tree, "sub", "nested.txt"), "after")
	after := digestOf(t, root, tree)

	if before.String() == after.String() {
		t.Error("nested content change did not flip the Merkle root")
	}
}

// TestDirectoryDigest_RenameFlipsRoot pins name sensitivity: the same bytes under a different name is a different
// tree.
func TestDirectoryDigest_RenameFlipsRoot(t *testing.T) {

	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	mkTreeFile(t, filepath.Join(tree, "old.txt"), "same bytes")

	before := digestOf(t, root, tree)
	if err := os.Rename(filepath.Join(tree, "old.txt"), filepath.Join(tree, "new.txt")); err != nil {
		t.Fatal(err)
	}
	after := digestOf(t, root, tree)

	if before.String() == after.String() {
		t.Error("rename did not flip the Merkle root")
	}
}

// TestDirectoryDigest_StructureChangeFlipsRoot pins shape sensitivity: the same file at the top level vs. nested one
// directory down yields different roots.
func TestDirectoryDigest_StructureChangeFlipsRoot(t *testing.T) {

	root := t.TempDir()
	flat := filepath.Join(root, "flat")
	nested := filepath.Join(root, "nested")
	mkTreeFile(t, filepath.Join(flat, "payload.txt"), "payload")
	mkTreeFile(t, filepath.Join(nested, "sub", "payload.txt"), "payload")

	if digestOf(t, root, flat).String() == digestOf(t, root, nested).String() {
		t.Error("structural difference did not flip the Merkle root")
	}
}

// TestDirectoryDigest_EmptyDirectoryDeterministic pins the empty-tree definition: two empty directories agree, and
// an empty subdirectory is itself visible structure.
func TestDirectoryDigest_EmptyDirectoryDeterministic(t *testing.T) {

	root := t.TempDir()
	for _, name := range []string{"empty-a", "empty-b", "with-sub/sub"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	emptyA := digestOf(t, root, filepath.Join(root, "empty-a"))
	emptyB := digestOf(t, root, filepath.Join(root, "empty-b"))
	withSub := digestOf(t, root, filepath.Join(root, "with-sub"))

	if emptyA.String() != emptyB.String() {
		t.Errorf("empty directories diverge: %s vs %s", emptyA, emptyB)
	}
	if emptyA.String() == withSub.String() {
		t.Error("an empty subdirectory left the root indistinguishable from empty")
	}
}

// TestDirectoryDigest_SymlinkHashesByTarget pins ruling 5a inside the tree: the link contributes its literal target,
// not its referent's content — retargeting flips the root, referent edits outside the tree do not, and a dangling
// target is legal.
func TestDirectoryDigest_SymlinkHashesByTarget(t *testing.T) {

	root := t.TempDir()
	outside := filepath.Join(root, "outside.txt")
	mkTreeFile(t, outside, "referent v1")

	tree := filepath.Join(root, "tree")
	mkTreeLink(t, outside, filepath.Join(tree, "link"))

	before := digestOf(t, root, tree)

	// Referent content changes outside the tree: the root must not move.
	mkTreeFile(t, outside, "referent v2")
	if digestOf(t, root, tree).String() != before.String() {
		t.Error("a referent edit outside the tree moved the Merkle root (the link was followed)")
	}

	// Retargeting — even to a dangling path — must flip it.
	if err := os.Remove(filepath.Join(tree, "link")); err != nil {
		t.Fatal(err)
	}
	mkTreeLink(t, filepath.Join(root, "does-not-exist"), filepath.Join(tree, "link"))
	if digestOf(t, root, tree).String() == before.String() {
		t.Error("retargeting the link did not flip the Merkle root")
	}
}

// TestDirectoryDigest_CoversDotGit pins ruling 5d: no skips — content under `.git` participates like any other.
func TestDirectoryDigest_CoversDotGit(t *testing.T) {

	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	mkTreeFile(t, filepath.Join(tree, ".git", "HEAD"), "ref: refs/heads/main")
	mkTreeFile(t, filepath.Join(tree, "tracked.txt"), "tracked")

	before := digestOf(t, root, tree)
	mkTreeFile(t, filepath.Join(tree, ".git", "HEAD"), "ref: refs/heads/other")
	after := digestOf(t, root, tree)

	if before.String() == after.String() {
		t.Error("a change under .git did not flip the Merkle root (5d: the digest must cover everything)")
	}
}

// TestDirectoryDigest_KindMismatch pins ruling 5e: asserting Directory over a regular file errors with the kind
// mismatch, not a walk failure.
func TestDirectoryDigest_KindMismatch(t *testing.T) {

	root := t.TempDir()
	path := filepath.Join(root, "actually-a-file")
	mkTreeFile(t, path, "not a directory")

	_, err := discoverDirectory(t, root, path).Digest()
	if !errors.Is(err, errKindMismatch) {
		t.Errorf("Digest over a regular file = %v; want the kind mismatch", err)
	}
}

// TestDirectoryEtag_StatTupleAndKindMismatch pins the cheap token: a directory yields a stable hex token, and the
// kind gate errors over a regular file.
func TestDirectoryEtag_StatTupleAndKindMismatch(t *testing.T) {

	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}

	directory := discoverDirectory(t, root, tree)
	first, err := directory.Etag()
	if err != nil || first == "" {
		t.Fatalf("Etag = %q (err %v); want a non-empty token", first, err)
	}
	second, err := directory.Etag()
	if err != nil || second != first {
		t.Errorf("Etag not stable over an unchanged directory: %q then %q (err %v)", first, second, err)
	}

	filePath := filepath.Join(root, "plain.txt")
	mkTreeFile(t, filePath, "plain")
	_, err = discoverDirectory(t, root, filePath).Etag()
	if !errors.Is(err, errKindMismatch) {
		t.Errorf("Etag over a regular file = %v; want the kind mismatch", err)
	}
}
