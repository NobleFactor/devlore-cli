// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestRegularDigest_HashesContent pins the carried-over semantics: the digest is the sha256 of the file's bytes.
func TestRegularDigest_HashesContent(t *testing.T) {

	root := t.TempDir()
	path := filepath.Join(root, "payload.txt")
	mkTreeFile(t, path, "hello taxonomy")

	p := testProvider(t, root)
	regular, err := DiscoverRegular(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("DiscoverRegular: %v", err)
	}

	digest, err := regular.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}

	want := sha256.Sum256([]byte("hello taxonomy"))
	if hex.EncodeToString(digest.Bytes) != hex.EncodeToString(want[:]) {
		t.Errorf("Digest = %x; want %x", digest.Bytes, want)
	}
}

// TestRegularDigest_KindMismatch pins ruling 5e with lstat semantics: a directory and a symlink-to-regular-file both
// fail the Regular assertion — a link to a file is kind symbolic-link, not kind regular.
func TestRegularDigest_KindMismatch(t *testing.T) {

	root := t.TempDir()
	dir := filepath.Join(root, "dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	referent := filepath.Join(root, "referent.txt")
	mkTreeFile(t, referent, "real file")
	link := filepath.Join(root, "link")
	mkTreeLink(t, referent, link)

	p := testProvider(t, root)
	for _, path := range []string{dir, link} {
		regular, err := DiscoverRegular(p.RuntimeEnvironment(), path)
		if err != nil {
			t.Fatalf("DiscoverRegular(%s): %v", path, err)
		}
		if _, err := regular.Digest(); !errors.Is(err, errKindMismatch) {
			t.Errorf("Digest(%s) = %v; want the kind mismatch", path, err)
		}
		if _, err := regular.Etag(); !errors.Is(err, errKindMismatch) {
			t.Errorf("Etag(%s) = %v; want the kind mismatch", path, err)
		}
	}
}

// TestRegularEtag_ChangesWithContentSize pins the stat-tuple token: growing the file moves the token.
func TestRegularEtag_ChangesWithContentSize(t *testing.T) {

	root := t.TempDir()
	path := filepath.Join(root, "grow.txt")
	mkTreeFile(t, path, "v1")

	p := testProvider(t, root)
	regular, err := DiscoverRegular(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("DiscoverRegular: %v", err)
	}

	before, err := regular.Etag()
	if err != nil {
		t.Fatalf("Etag: %v", err)
	}
	mkTreeFile(t, path, "version two, longer")
	after, err := regular.Etag()
	if err != nil {
		t.Fatalf("Etag after grow: %v", err)
	}

	if before == after {
		t.Error("Etag did not move when the file grew")
	}
}

// TestRegularEqual_StrictType pins cross-kind inequality: the same URI held by the catch-all base (or another kind)
// does not match a *Regular.
func TestRegularEqual_StrictType(t *testing.T) {

	root := t.TempDir()
	path := filepath.Join(root, "same.txt")
	mkTreeFile(t, path, "same")

	// Catalog-free environments so each construction yields an independent unlinked value.
	p := testProvider(t, root)
	runtimeEnvironment := p.RuntimeEnvironment()
	runtimeEnvironment.ResourceCatalog = nil

	first, err := DiscoverRegular(runtimeEnvironment, path)
	if err != nil {
		t.Fatalf("DiscoverRegular: %v", err)
	}
	second, err := DiscoverRegular(runtimeEnvironment, path)
	if err != nil {
		t.Fatalf("DiscoverRegular: %v", err)
	}
	base, err := discoverResource(runtimeEnvironment, path)
	if err != nil {
		t.Fatalf("DiscoverResource: %v", err)
	}

	if !concrete[*regular](t, first).Equal(second) {
		t.Error("two Regulars over the same URI are unequal")
	}
	if concrete[*regular](t, first).Equal(base) {
		t.Error("a Regular matched the catch-all base over the same URI (strict typing violated)")
	}
	if base.Equal(first) {
		t.Error("the catch-all base matched a Regular over the same URI (strict typing violated)")
	}
}

// TestDiscover_CrossKindCollisionErrors pins the catalog conflict feature: the same URI discovered as two different
// kinds is an error naming both types, not a silent aliasing.
func TestDiscover_CrossKindCollisionErrors(t *testing.T) {

	root := t.TempDir()
	path := filepath.Join(root, "contested")
	mkTreeFile(t, path, "contested")

	p := testProvider(t, root)
	if _, err := DiscoverRegular(p.RuntimeEnvironment(), path); err != nil {
		t.Fatalf("DiscoverRegular: %v", err)
	}

	_, err := DiscoverDirectory(p.RuntimeEnvironment(), path)
	if err == nil {
		t.Fatal("DiscoverDirectory over a URI held as Regular = nil error; want the cross-kind collision")
	}
	if !strings.Contains(err.Error(), "file.regular") || !strings.Contains(err.Error(), "file.directory") {
		t.Errorf("collision error %q does not name both kinds", err)
	}
}

// TestDiscoverRegular_CacheHitReturnsCanonical pins catalog interning: a second discovery over the same URI returns
// the same canonical entry.
func TestDiscoverRegular_CacheHitReturnsCanonical(t *testing.T) {

	root := t.TempDir()
	path := filepath.Join(root, "canon.txt")
	mkTreeFile(t, path, "canon")

	p := testProvider(t, root)
	first, err := DiscoverRegular(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("DiscoverRegular: %v", err)
	}
	second, err := DiscoverRegular(p.RuntimeEnvironment(), path)
	if err != nil {
		t.Fatalf("DiscoverRegular (second): %v", err)
	}

	if first != second {
		t.Error("second discovery returned a different entry than the canonical")
	}
}

// TestRegularCanConvertFrom_NilReceiverSafe pins the cheap-probe contract: the probe must survive a nil receiver
// (the plan-time interconvertibility check calls it that way) — a promoted method would panic here.
func TestRegularCanConvertFrom_NilReceiverSafe(t *testing.T) {

	var nilRegular *regular

	if !nilRegular.CanConvertFrom(reflect.TypeFor[string]()) {
		t.Error("CanConvertFrom(string) = false; want true")
	}
	if nilRegular.CanConvertFrom(nil) {
		t.Error("CanConvertFrom(nil) = true; want false")
	}
}

// TestRegularConvertFrom_Unlinked pins the projection contract: a string projects to an unlinked regular carrying
// the path.
func TestRegularConvertFrom_Unlinked(t *testing.T) {

	projected, err := (*regular)(nil).ConvertFrom("/some/path.txt")
	if err != nil {
		t.Fatalf("ConvertFrom: %v", err)
	}

	got, ok := projected.(*regular)
	if !ok {
		t.Fatalf("ConvertFrom returned %T; want *regular", projected)
	}
	// Abs is OS-native, so the expectation is platform-correct rather than a slash literal.
	if want := filepath.FromSlash("/some/path.txt"); got.SourcePath.Abs() != want {
		t.Errorf("projected path = %q; want %q", got.SourcePath.Abs(), want)
	}

	if _, err := (*regular)(nil).ConvertFrom(42); err == nil {
		t.Error("ConvertFrom(42) = nil error; want the non-string rejection")
	}
}
