// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResourceURI_File(t *testing.T) {
	// Relative path should become absolute
	uri := ResourceURI(SchemeFile, "relative/path")
	if !strings.HasPrefix(uri, "file://") {
		t.Errorf("ResourceURI(file, relative/path) = %q, want file:// prefix", uri)
	}
	path := strings.TrimPrefix(uri, "file://")
	if !filepath.IsAbs(path) {
		t.Errorf("ResourceURI(file, relative/path) produced non-absolute path: %q", path)
	}

	// Clean resolves ../
	uri2 := ResourceURI(SchemeFile, "/etc/../etc/foo")
	if uri2 != "file:///etc/foo" {
		t.Errorf("ResourceURI(file, /etc/../etc/foo) = %q, want file:///etc/foo", uri2)
	}

	// Absolute path stays absolute
	uri3 := ResourceURI(SchemeFile, "/usr/local/bin")
	if uri3 != "file:///usr/local/bin" {
		t.Errorf("ResourceURI(file, /usr/local/bin) = %q, want file:///usr/local/bin", uri3)
	}
}

func TestResourceURI_OtherSchemes(t *testing.T) {
	tests := []struct {
		scheme string
		id     string
		want   string
	}{
		{SchemeGit, "github.com/org/repo", "git:///github.com/org/repo"},
		{SchemePackage, "brew/vim", "pkg:///brew/vim"},
		{SchemeService, "nginx", "svc:///nginx"},
		{SchemeMem, "buffer-1", "mem:///buffer-1"},
	}
	for _, tt := range tests {
		t.Run(tt.scheme, func(t *testing.T) {
			got := ResourceURI(tt.scheme, tt.id)
			if got != tt.want {
				t.Errorf("ResourceURI(%s, %s) = %q, want %q", tt.scheme, tt.id, got, tt.want)
			}
		})
	}
}

func TestResourceBase_URI(t *testing.T) {
	base := NewResourceBase("file:///foo")
	if base.URI() != "file:///foo" {
		t.Errorf("URI() = %q, want file:///foo", base.URI())
	}
}

func TestResourceBase_SatisfiesInterface(t *testing.T) {
	base := NewResourceBase("file:///bar")
	var r Resource = &base
	if r.URI() != "file:///bar" {
		t.Errorf("Resource.URI() = %q, want file:///bar", r.URI())
	}
}

// testEmbeddingResource embeds ResourceBase, like file.Resource does.
type testEmbeddingResource struct {
	ResourceBase
	Extra string
}

func TestResourceBase_EmbeddingSatisfiesInterface(t *testing.T) {
	r := &testEmbeddingResource{
		ResourceBase: NewResourceBase("file:///embedded"),
		Extra:        "metadata",
	}
	var iface Resource = r
	if iface.URI() != "file:///embedded" {
		t.Errorf("Resource.URI() = %q, want file:///embedded", iface.URI())
	}
}
