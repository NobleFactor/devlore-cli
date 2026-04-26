// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package appnet

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- Interface guards ---

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

// --- Constructor ---

func TestConstructorRoundTrip(t *testing.T) {

	r, err := NewResource(&op.ExecutionContext{}, "https://example.com/file.tar.gz")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	if r.SourceURL.String() != "https://example.com/file.tar.gz" {
		t.Errorf("SourceURL = %q, want %q", r.SourceURL.String(), "https://example.com/file.tar.gz")
	}
	if r.ReachabilityURI() != "https://example.com/file.tar.gz" {
		t.Errorf("ReachabilityURI() = %q, want %q", r.ReachabilityURI(), "https://example.com/file.tar.gz")
	}
}

func TestConstructorInvalidURL(t *testing.T) {

	if _, err := NewResource(&op.ExecutionContext{}, "://bad"); err == nil {
		t.Fatal("NewResource: expected error for invalid URL")
	}
}

func TestConstructorWrongType(t *testing.T) {

	if _, err := NewResource(&op.ExecutionContext{}, 42); err == nil {
		t.Fatal("NewResource: expected error for non-string")
	}
}

func TestConstructorMissingScheme(t *testing.T) {

	if _, err := NewResource(&op.ExecutionContext{}, "example.com/path"); err == nil {
		t.Fatal("NewResource: expected error for schemeless URL")
	}
}

// --- URI shape ---

func TestURI_TransportDifferentiates(t *testing.T) {

	http := mustParse(t, "http://example.com/path")
	https := mustParse(t, "https://example.com/path")

	if http.URI() == https.URI() {
		t.Errorf("http and https URIs should differ under URI-ensures-reachability: both %q", http.URI())
	}
}

// --- Canonicalization ---

func TestURI_Canonicalization(t *testing.T) {

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "lowercase host",
			raw:  "https://Example.COM/path",
			want: "https://example.com/path",
		},
		{
			name: "strip default https port",
			raw:  "https://example.com:443/path",
			want: "https://example.com/path",
		},
		{
			name: "strip default http port",
			raw:  "http://example.com:80/path",
			want: "http://example.com/path",
		},
		{
			name: "keep non-default port",
			raw:  "https://example.com:8443/path",
			want: "https://example.com:8443/path",
		},
		{
			name: "strip trailing slash",
			raw:  "https://example.com/path/",
			want: "https://example.com/path",
		},
		{
			name: "collapse double slashes",
			raw:  "https://example.com/a//b///c",
			want: "https://example.com/a/b/c",
		},
		{
			name: "uppercase percent encoding decodes unreserved",
			raw:  "https://example.com/p%61th",
			want: "https://example.com/path",
		},
		{
			name: "keep reserved chars encoded",
			raw:  "https://example.com/path%20with%20spaces",
			want: "https://example.com/path%20with%20spaces",
		},
		{
			name: "uppercase hex digits in remaining percent-encoding",
			raw:  "https://example.com/%2f%2F",
			want: "https://example.com/%2F%2F",
		},
		{
			name: "sort query parameters",
			raw:  "https://example.com/path?z=1&a=2&m=3",
			want: "https://example.com/path?a=2&m=3&z=1",
		},
		{
			name: "root path stays",
			raw:  "https://example.com",
			want: "https://example.com/",
		},
		{
			name: "all rules combined",
			raw:  "HTTPS://Example.COM:443/A//B/%7e/?z=1&a=2",
			want: "https://example.com/A/B/~?a=2&z=1",
		},
		{
			name: "ftp default port stripped",
			raw:  "ftp://files.example.com:21/pub/file.txt",
			want: "ftp://files.example.com/pub/file.txt",
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

// --- Helper ---

func mustParse(t *testing.T, raw string) *Resource {
	t.Helper()
	r, err := NewResource(&op.ExecutionContext{}, raw)
	if err != nil {
		t.Fatalf("NewResource(%q): %v", raw, err)
	}
	return r
}
