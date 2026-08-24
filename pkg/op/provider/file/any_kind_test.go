// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// region TEST FUNCTIONS

// TestAny_Exists_IsPermissiveButTaxonomyBounded pins the unasserted claim's predicate
// (docs/plans/any-entry-claims.md, ruled 2026-08-23): every taxonomy kind admits, and nothing else does.
//
// The two sharp rows are the dangling link — present, because the LINK is the entry, and a following
// stat would wrongly call it absent — and the missing path. The FIFO row (an entry the taxonomy has no
// variant for) is pinned separately in the unix-only file, since it needs mkfifo.
func TestAny_Exists_IsPermissiveButTaxonomyBounded(t *testing.T) {

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
			if got := anyKindAt(t, environment, tc.path).Exists(); got != tc.want {
				t.Errorf("Exists() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestAny_ContentIdentityDelegatesToTheObservedKind pins that an unasserted claim still owes content
// identity: Digest and Etag answer as whatever the disk holds answers, so drift detection is not lost
// while the claim is unresolved.
func TestAny_ContentIdentityDelegatesToTheObservedKind(t *testing.T) {

	dir := t.TempDir()
	environment := testEnvironment(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "regular.txt"), []byte("delegated"), 0o600); err != nil {
		t.Fatal(err)
	}

	unasserted := anyKindAt(t, environment, "regular.txt")

	kindedBase, err := buildCandidateAs(environment, "regular.txt", reflect.TypeFor[*Regular]())
	if err != nil {
		t.Fatalf("candidate: %v", err)
	}
	kinded := &Regular{resource: *kindedBase}

	unassertedDigest, err := unasserted.Digest()
	if err != nil {
		t.Fatalf("AnyKind.Digest: %v", err)
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
		t.Fatalf("AnyKind.Etag: %v", err)
	}
	kindedEtag, err := kinded.Etag()
	if err != nil {
		t.Fatalf("Regular.Etag: %v", err)
	}
	if unassertedEtag != kindedEtag {
		t.Errorf("etag = %q, want the observed kind's %q", unassertedEtag, kindedEtag)
	}
}

// TestAny_ContentIdentityOnAMissingPathErrors pins the delegation's failure direction: there is no
// observed kind to delegate to, so the tiers report the lstat failure rather than inventing an identity.
func TestAny_ContentIdentityOnAMissingPathErrors(t *testing.T) {

	environment := testEnvironment(t, t.TempDir())

	if _, err := anyKindAt(t, environment, "absent.txt").Digest(); err == nil {
		t.Error("Digest() over a missing path returned no error")
	}
	if _, err := anyKindAt(t, environment, "absent.txt").Etag(); err == nil {
		t.Error("Etag() over a missing path returned no error")
	}
}

// endregion

// region HELPER FUNCTIONS

// anyKindAt builds an UNLINKED [*AnyKind] for `rel` — no interning, so a test may probe several kinds
// at several paths under one environment without tripping the catalog's cross-kind claim rule.
func anyKindAt(t *testing.T, environment *op.RuntimeEnvironment, rel string) *AnyKind {

	t.Helper()

	base, err := buildCandidateAs(environment, rel, reflect.TypeFor[*AnyKind]())
	if err != nil {
		t.Fatalf("candidate %s: %v", rel, err)
	}

	return &AnyKind{resource: *base}
}

// endregion

// TestAny_ResolvesToTheObservedKindAtTheTransition pins the seam end to end against a real filesystem:
// an unasserted claim, verified through the catalog, becomes the variant the disk holds — and the ledger
// keeps one entry with one identity throughout.
//
// The catalog-level mechanics are pinned in pkg/op; this is the half that proves file.AnyKind actually
// resolves, and that the URI's type fragment moves with it (which is how the trace comes to say
// "Regular" where the graph's intent said "AnyKind").
func TestAny_ResolvesToTheObservedKindAtTheTransition(t *testing.T) {

	dir := t.TempDir()
	environment := testEnvironment(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "claimed.txt"), []byte("observed"), 0o600); err != nil {
		t.Fatal(err)
	}

	claim, err := DiscoverAnyKind(environment, "claimed.txt")
	if err != nil {
		t.Fatalf("DiscoverAnyKind: %v", err)
	}
	id := claim.ID()

	if err := environment.ResourceCatalog.VerifyExistence(claim); err != nil {
		t.Fatalf("VerifyExistence: %v", err)
	}

	entry, ok := environment.ResourceCatalog.Lookup(id)
	if !ok {
		t.Fatal("the entry vanished from the ledger")
	}
	if _, isRegular := entry.(*Regular); !isRegular {
		t.Fatalf("ledger holds %T, want *Regular — the claim must become what the disk holds", entry)
	}
	if entry.ID() != id {
		t.Errorf("resolved entry ID = %q, want %q — one identity throughout", entry.ID(), id)
	}
	if environment.ResourceCatalog.Len() != 1 {
		t.Errorf("ledger length = %d, want 1 — resolution replaces, never appends",
			environment.ResourceCatalog.Len())
	}
	if !strings.Contains(entry.URI(), "file.Regular") {
		t.Errorf("resolved URI = %q, want the observed kind in its type fragment", entry.URI())
	}
}

// TestAny_AnUnmetClaimStaysUnasserted pins the Gone direction with the real predicate: nothing is at the
// path, so there is nothing to resolve to, and the entry records unmet intent as what it was claimed as.
func TestAny_AnUnmetClaimStaysUnasserted(t *testing.T) {

	environment := testEnvironment(t, t.TempDir())

	claim, err := DiscoverAnyKind(environment, "absent.txt")
	if err != nil {
		t.Fatalf("DiscoverAnyKind: %v", err)
	}
	id := claim.ID()

	if err := environment.ResourceCatalog.VerifyExistence(claim); err == nil {
		t.Fatal("VerifyExistence returned no error for a path holding nothing")
	}

	entry, _ := environment.ResourceCatalog.Lookup(id)
	if _, stillAny := entry.(*AnyKind); !stillAny {
		t.Errorf("ledger holds %T, want *AnyKind — an unmet claim has nothing to become", entry)
	}
	if got := environment.ResourceCatalog.State(id); got != op.Gone {
		t.Errorf("state = %v, want Gone", got)
	}
}
