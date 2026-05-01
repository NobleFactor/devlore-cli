// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package appnet

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestNewResource(t *testing.T) {

	ctx := &op.ExecutionContext{}

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
			got, err := NewResource(ctx, tt.raw)
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

// --- Helper ---

func mustParse(t *testing.T, raw string) op.Resource {
	t.Helper()
	r, err := NewResource(&op.ExecutionContext{}, raw)
	if err != nil {
		t.Fatalf("NewResource(%q): %v", raw, err)
	}

	return r
}
