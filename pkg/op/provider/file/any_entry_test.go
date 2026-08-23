// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// region TEST FUNCTIONS

// TestAnyEntry_Exists_IsPermissiveButTaxonomyBounded pins the unasserted claim's predicate
// (docs/plans/any-entry-claims.md, ruled 2026-08-23): every taxonomy kind admits, and nothing else does.
//
// The two sharp rows are the dangling link — present, because the LINK is the entry, and a following
// stat would wrongly call it absent — and the missing path. The FIFO row (an entry the taxonomy has no
// variant for) is pinned separately in the unix-only file, since it needs mkfifo.
func TestAnyEntry_Exists_IsPermissiveButTaxonomyBounded(t *testing.T) {

	dir := t.TempDir()
	environment := testEnvironment(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "regular.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "a-directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "regular.txt"), filepath.Join(dir, "good-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "nowhere.txt"), filepath.Join(dir, "dangling-link")); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"a regular file", "regular.txt", true},
		{"a directory", "a-directory", true},
		{"a symbolic link", "good-link", true},
		{"a DANGLING symbolic link — the link is the entry", "dangling-link", true},
		{"nothing at all", "absent.txt", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := anyEntryAt(t, environment, tc.path).Exists(); got != tc.want {
				t.Errorf("Exists() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestAnyEntry_ContentIdentityDelegatesToTheObservedKind pins that an unasserted claim still owes content
// identity: Digest and Etag answer as whatever the disk holds answers, so drift detection is not lost
// while the claim is unresolved.
func TestAnyEntry_ContentIdentityDelegatesToTheObservedKind(t *testing.T) {

	dir := t.TempDir()
	environment := testEnvironment(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "regular.txt"), []byte("delegated"), 0o600); err != nil {
		t.Fatal(err)
	}

	unasserted := anyEntryAt(t, environment, "regular.txt")

	kindedBase, err := buildCandidateAs(environment, "regular.txt", reflect.TypeFor[*Regular]())
	if err != nil {
		t.Fatalf("candidate: %v", err)
	}
	kinded := &Regular{entry: *kindedBase}

	unassertedDigest, err := unasserted.Digest()
	if err != nil {
		t.Fatalf("AnyEntry.Digest: %v", err)
	}
	kindedDigest, err := kinded.Digest()
	if err != nil {
		t.Fatalf("Regular.Digest: %v", err)
	}
	if unassertedDigest.String() != kindedDigest.String() {
		t.Errorf("digest = %s, want the observed kind's %s", unassertedDigest, kindedDigest)
	}

	unassertedEtag, err := unasserted.Etag()
	if err != nil {
		t.Fatalf("AnyEntry.Etag: %v", err)
	}
	kindedEtag, err := kinded.Etag()
	if err != nil {
		t.Fatalf("Regular.Etag: %v", err)
	}
	if unassertedEtag != kindedEtag {
		t.Errorf("etag = %q, want the observed kind's %q", unassertedEtag, kindedEtag)
	}
}

// TestAnyEntry_ContentIdentityOnAMissingPathErrors pins the delegation's failure direction: there is no
// observed kind to delegate to, so the tiers report the lstat failure rather than inventing an identity.
func TestAnyEntry_ContentIdentityOnAMissingPathErrors(t *testing.T) {

	environment := testEnvironment(t, t.TempDir())

	if _, err := anyEntryAt(t, environment, "absent.txt").Digest(); err == nil {
		t.Error("Digest() over a missing path returned no error")
	}
	if _, err := anyEntryAt(t, environment, "absent.txt").Etag(); err == nil {
		t.Error("Etag() over a missing path returned no error")
	}
}

// endregion

// region HELPER FUNCTIONS

// anyEntryAt builds an UNLINKED [*AnyEntry] for `rel` — no interning, so a test may probe several kinds
// at several paths under one environment without tripping the catalog's cross-kind claim rule.
func anyEntryAt(t *testing.T, environment *op.RuntimeEnvironment, rel string) *AnyEntry {

	t.Helper()

	base, err := buildCandidateAs(environment, rel, reflect.TypeFor[*AnyEntry]())
	if err != nil {
		t.Fatalf("candidate %s: %v", rel, err)
	}

	return &AnyEntry{entry: *base}
}

// endregion
