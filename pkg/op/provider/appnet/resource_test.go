// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package appnet

import (
	"crypto/sha256"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Test helpers ---

// testActivation returns a non-nil [op.ActivationRecord] suitable for production-claim test calls.
// RuntimeEnvironment is empty (nil-Catalog tolerance returns the unlinked candidate); Unit is nil
// (non-graph dispatch — Resources interned through this activation carry an empty producer stamp).
func testActivation(t *testing.T) *op.ActivationRecord {
	t.Helper()
	return op.NewActivationRecord(nil, nil, &op.RuntimeEnvironment{})
}

// newRes constructs a *Resource for a URL string. Uses DiscoverResource because the test isn't claiming
// production — it's setting up a fixture handle.
func newRes(t *testing.T, url string) *Resource {
	t.Helper()
	r, err := DiscoverResource(op.NewActivationRecord(nil, nil, &op.RuntimeEnvironment{}), url)
	if err != nil {
		t.Fatalf("DiscoverResource(%q): %v", url, err)
	}
	return r
}

// mustParse constructs an op.Resource for raw or fails the test. Uses DiscoverResource for the same reason
// as [newRes].
func mustParse(t *testing.T, raw string) op.Resource {
	t.Helper()
	r, err := DiscoverResource(op.NewActivationRecord(nil, nil, &op.RuntimeEnvironment{}), raw)
	if err != nil {
		t.Fatalf("DiscoverResource(%q): %v", raw, err)
	}
	return r
}

// --- NewResource ---

func TestNewResource(t *testing.T) {

	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"valid https", "https://example.com/path", false},
		{"valid http", "http://example.com/path", false},
		{"valid ftp", "ftp://files.example.com/pub/file.txt", false},
		{"invalid url", "://invalid", true},
		{"missing scheme", "example.com/path", true},
		// unsupported_scheme: NewResourceBase doesn't validate schemes; it just wraps.
		// So mailto: is accepted as a string.
		{"mailto", "mailto:user@example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewResource(testActivation(t), tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewResource() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got == nil {
				t.Fatal("NewResource() returned nil without error")
			}
		})
	}
}

func TestURI_Canonicalization(t *testing.T) {

	const tagURIPrefix = "tag:devlore.noblefactor.com,2026-01-01:"
	const suffix = "#github.com/NobleFactor/devlore-cli/pkg/op/provider/appnet.Resource"

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "lowercase host",
			raw:  "HTTPS://Example.COM/path",
			want: tagURIPrefix + "https://example.com/path" + suffix,
		},
		{
			name: "strip default https port",
			raw:  "https://example.com:443/path",
			want: tagURIPrefix + "https://example.com/path" + suffix,
		},
		{
			name: "strip default http port",
			raw:  "http://example.com:80/path",
			want: tagURIPrefix + "http://example.com/path" + suffix,
		},
		{
			name: "keep non-default port",
			raw:  "https://example.com:8443/path",
			want: tagURIPrefix + "https://example.com:8443/path" + suffix,
		},
		{
			name: "strip trailing slash",
			raw:  "https://example.com/path/",
			want: tagURIPrefix + "https://example.com/path" + suffix,
		},
		{
			name: "collapse double slashes",
			raw:  "https://example.com/a//b///c",
			want: tagURIPrefix + "https://example.com/a/b/c" + suffix,
		},
		{
			name: "uppercase percent encoding decodes unreserved",
			raw:  "https://example.com/p%61th",
			want: tagURIPrefix + "https://example.com/path" + suffix,
		},
		{
			name: "keep reserved chars encoded",
			raw:  "https://example.com/path%20with%20spaces",
			want: tagURIPrefix + "https://example.com/path%20with%20spaces" + suffix,
		},
		{
			name: "uppercase hex digits in remaining percent-encoding",
			raw:  "https://example.com/%2f%2f",
			want: tagURIPrefix + "https://example.com/%2F%2F" + suffix,
		},
		{
			name: "sort query parameters",
			raw:  "https://example.com/path?z=1&a=2&m=3",
			want: tagURIPrefix + "https://example.com/path?a=2&m=3&z=1" + suffix,
		},
		{
			name: "root path stays",
			raw:  "https://example.com/",
			want: tagURIPrefix + "https://example.com/" + suffix,
		},
		{
			name: "all rules combined",
			raw:  "HTTPS://Example.COM:443/A//B/%7e/?z=1&a=2",
			want: tagURIPrefix + "https://example.com/A/B/~?a=2&z=1" + suffix,
		},
		{
			name: "ftp default port stripped",
			raw:  "ftp://files.example.com:21/pub/file.txt",
			want: tagURIPrefix + "ftp://files.example.com/pub/file.txt" + suffix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := mustParse(t, tt.raw)
			if got := r.URI(); got != tt.want {
				t.Errorf("URI() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- Addressing ---

func TestResource_Addressing_IsLocation(t *testing.T) {

	r := newRes(t, "https://example.com/foo")

	if got := r.Addressing(); got != op.AddressingLocation {
		t.Errorf("Addressing() = %v, want AddressingLocation", got)
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