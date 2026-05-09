// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package appnet

import (
	"crypto/sha256"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// newRes constructs a *Resource for a URL string against a bare runtime environment.
func newRes(t *testing.T, url string) *Resource {
	t.Helper()
	r, err := NewResource(&op.RuntimeEnvironment{}, url)
	if err != nil {
		t.Fatalf("NewResource(%q): %v", url, err)
	}
	return r
}

// --- Addressing ---

func TestResource_Addressing_IsLocation(t *testing.T) {

	r := newRes(t, "https://example.com/foo")

	if got := r.Addressing(); got != op.AddressingLocation {
		t.Errorf("Addressing() = %v, want AddressingLocation", got)
	}
}

// --- Etag ---

func TestResource_Etag_IsURI(t *testing.T) {

	r := newRes(t, "https://example.com/foo")

	got, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag: %v", err)
	}

	if got != r.URI() {
		t.Errorf("Etag() = %q, want URI() = %q", got, r.URI())
	}
}

func TestResource_Etag_StableAcrossCalls(t *testing.T) {

	r := newRes(t, "https://example.com/foo")

	first, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag (first): %v", err)
	}

	second, err := r.Etag()
	if err != nil {
		t.Fatalf("Etag (second): %v", err)
	}

	if first != second {
		t.Errorf("Etag drifted across calls: %q vs %q", first, second)
	}
}

func TestResource_Etag_DiffersAcrossURLs(t *testing.T) {

	a := newRes(t, "https://example.com/foo")
	b := newRes(t, "https://example.com/bar")

	etagA, err := a.Etag()
	if err != nil {
		t.Fatalf("Etag(a): %v", err)
	}
	etagB, err := b.Etag()
	if err != nil {
		t.Fatalf("Etag(b): %v", err)
	}

	if etagA == etagB {
		t.Errorf("Etag collided across distinct URLs: %q", etagA)
	}
}

// TestResource_Etag_HTTPvsHTTPSDiffer confirms the canonicalization preserves transport scheme — two URLs
// differing only in scheme produce distinct Resources and therefore distinct Etags.
func TestResource_Etag_HTTPvsHTTPSDiffer(t *testing.T) {

	httpRes := newRes(t, "http://example.com/foo")
	httpsRes := newRes(t, "https://example.com/foo")

	httpEtag, _ := httpRes.Etag()
	httpsEtag, _ := httpsRes.Etag()

	if httpEtag == httpsEtag {
		t.Errorf("http and https URLs produced identical Etags: %q", httpEtag)
	}
}

// --- Digest ---

func TestResource_Digest_IsSha256OfURI(t *testing.T) {

	r := newRes(t, "https://example.com/foo")

	got, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}

	want := sha256.Sum256([]byte(r.URI()))
	expected := op.Digest{Algorithm: "sha256", Bytes: want[:]}

	if !got.Equal(expected) {
		t.Errorf("Digest = %s, want %s", got.String(), expected.String())
	}
}

func TestResource_Digest_StableAcrossCalls(t *testing.T) {

	r := newRes(t, "https://example.com/foo")

	first, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (first): %v", err)
	}

	second, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest (second): %v", err)
	}

	if !first.Equal(second) {
		t.Errorf("Digest drifted across calls: %s vs %s", first.String(), second.String())
	}
}

func TestResource_Digest_DiffersAcrossURLs(t *testing.T) {

	a := newRes(t, "https://example.com/foo")
	b := newRes(t, "https://example.com/bar")

	digestA, _ := a.Digest()
	digestB, _ := b.Digest()

	if digestA.Equal(digestB) {
		t.Errorf("Digest collided across distinct URLs: %s", digestA.String())
	}
}

// TestResource_Digest_RoundTripsThroughParseDigest confirms the digest serializes through the canonical
// "<algo>:<hex>" form and round-trips via op.ParseDigest. Catalog persistence depends on this.
func TestResource_Digest_RoundTripsThroughParseDigest(t *testing.T) {

	r := newRes(t, "https://example.com/foo")

	got, err := r.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}

	roundTrip, err := op.ParseDigest(got.String())
	if err != nil {
		t.Fatalf("ParseDigest(%q): %v", got.String(), err)
	}

	if !roundTrip.Equal(got) {
		t.Errorf("ParseDigest round-trip changed value: got %s after parsing %s", roundTrip.String(), got.String())
	}
}