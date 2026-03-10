// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package appnet

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func init() { op.RegisterConstructor(ResourceFromValue) }

func TestResourceImplementsInterface(t *testing.T) {
	var _ op.Resource = (*Resource)(nil)
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

func TestURIOpaqueScheme(t *testing.T) {
	r := mustParse(t, "https://example.com/path")
	if r.Scheme() != "appnet" {
		t.Errorf("Scheme() = %q, want %q", r.Scheme(), "appnet")
	}
	if r.Opaque() == "" {
		t.Error("Opaque() is empty — expected opaque URI")
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
			want: "appnet:example.com/path",
		},
		{
			name: "strip default https port",
			raw:  "https://example.com:443/path",
			want: "appnet:example.com/path",
		},
		{
			name: "strip default http port",
			raw:  "http://example.com:80/path",
			want: "appnet:example.com/path",
		},
		{
			name: "keep non-default port",
			raw:  "https://example.com:8443/path",
			want: "appnet:example.com:8443/path",
		},
		{
			name: "strip trailing slash",
			raw:  "https://example.com/path/",
			want: "appnet:example.com/path",
		},
		{
			name: "collapse double slashes",
			raw:  "https://example.com/a//b///c",
			want: "appnet:example.com/a/b/c",
		},
		{
			name: "uppercase percent encoding",
			raw:  "https://example.com/p%61th",
			want: "appnet:example.com/path",
		},
		{
			name: "keep reserved chars encoded",
			raw:  "https://example.com/path%20with%20spaces",
			want: "appnet:example.com/path%20with%20spaces",
		},
		{
			name: "uppercase hex digits",
			raw:  "https://example.com/%2f%2F",
			want: "appnet:example.com/%2F%2F",
		},
		{
			name: "sort query parameters escaped",
			raw:  "https://example.com/path?z=1&a=2&m=3",
			want: "appnet:example.com/path%3Fa=2&m=3&z=1",
		},
		{
			name: "root path stays",
			raw:  "https://example.com",
			want: "appnet:example.com/",
		},
		{
			name: "all rules combined",
			raw:  "HTTPS://Example.COM:443/A//B/%7e/?z=1&a=2",
			want: "appnet:example.com/A/B/~%3Fa=2&z=1",
		},
		{
			name: "ftp default port stripped",
			raw:  "ftp://files.example.com:21/pub/file.txt",
			want: "appnet:files.example.com/pub/file.txt",
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
