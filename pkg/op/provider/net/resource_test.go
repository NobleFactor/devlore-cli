// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package net //nolint:revive // package name is domain-specific

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
}

func TestResourceScheme(t *testing.T) {
	r := mustParse(t, "https://example.com/path")
	if r.Scheme() != op.SchemeNet {
		t.Errorf("Scheme() = %q, want %q", r.Scheme(), op.SchemeNet)
	}
}

func TestResourceHost(t *testing.T) {
	r := mustParse(t, "https://Example.COM/path")
	if r.Host() != "example.com" {
		t.Errorf("Host() = %q, want %q", r.Host(), "example.com")
	}
}

func TestResourcePath(t *testing.T) {
	r := mustParse(t, "https://example.com/some/path")
	if r.Path() != "/some/path" {
		t.Errorf("Path() = %q, want %q", r.Path(), "/some/path")
	}
}

func TestConstructorRoundTrip(t *testing.T) {
	r, err := op.Construct[Resource]("https://example.com/file.tar.gz")
	if err != nil {
		t.Fatalf("Construct: %v", err)
	}
	if r.SourceURL.String() != "https://example.com/file.tar.gz" {
		t.Errorf("SourceURL = %q, want %q", r.SourceURL.String(), "https://example.com/file.tar.gz")
	}
}

func TestConstructorInvalidURL(t *testing.T) {
	_, err := op.Construct[Resource]("://bad")
	if err == nil {
		t.Fatal("Construct: expected error for invalid URL")
	}
}

func TestConstructorWrongType(t *testing.T) {
	_, err := op.Construct[Resource](42)
	if err == nil {
		t.Fatal("Construct: expected error for non-string")
	}
}

func TestURITransportIndependent(t *testing.T) {
	http := mustParse(t, "http://example.com/path")
	https := mustParse(t, "https://example.com/path")
	if http.URI() != https.URI() {
		t.Errorf("http URI %q != https URI %q — should be transport-independent", http.URI(), https.URI())
	}
}

func TestURICanonicalization(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "lowercase host",
			raw:  "https://Example.COM/path",
			want: "net://example.com/path",
		},
		{
			name: "strip default https port",
			raw:  "https://example.com:443/path",
			want: "net://example.com/path",
		},
		{
			name: "strip default http port",
			raw:  "http://example.com:80/path",
			want: "net://example.com/path",
		},
		{
			name: "keep non-default port",
			raw:  "https://example.com:8443/path",
			want: "net://example.com:8443/path",
		},
		{
			name: "strip trailing slash",
			raw:  "https://example.com/path/",
			want: "net://example.com/path",
		},
		{
			name: "collapse double slashes",
			raw:  "https://example.com/a//b///c",
			want: "net://example.com/a/b/c",
		},
		{
			name: "uppercase percent encoding",
			raw:  "https://example.com/p%61th",
			want: "net://example.com/path",
		},
		{
			name: "keep reserved chars encoded",
			raw:  "https://example.com/path%20with%20spaces",
			want: "net://example.com/path%20with%20spaces",
		},
		{
			name: "uppercase hex digits",
			raw:  "https://example.com/%2f%2F",
			want: "net://example.com/%2F%2F",
		},
		{
			name: "sort query parameters",
			raw:  "https://example.com/path?z=1&a=2&m=3",
			want: "net://example.com/path?a=2&m=3&z=1",
		},
		{
			name: "root path stays",
			raw:  "https://example.com",
			want: "net://example.com/",
		},
		{
			name: "all rules combined",
			raw:  "HTTPS://Example.COM:443/A//B/%7e/?z=1&a=2",
			want: "net://example.com/A/B/~?a=2&z=1",
		},
		{
			name: "ftp default port stripped",
			raw:  "ftp://files.example.com:21/pub/file.txt",
			want: "net://files.example.com/pub/file.txt",
		},
		{
			name: "file scheme accepted",
			raw:  "file:///etc/hosts",
			want: "net:///etc/hosts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := mustParse(t, tt.raw)
			got := r.URI()
			if got != tt.want {
				t.Errorf("URI() = %q, want %q", got, tt.want)
			}
		})
	}
}

func mustParse(t *testing.T, raw string) *Resource {
	t.Helper()
	r, err := op.Construct[Resource](raw)
	if err != nil {
		t.Fatalf("Construct(%q): %v", raw, err)
	}
	return &r
}
