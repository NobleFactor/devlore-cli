// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResourceBase_URI(t *testing.T) {
	base := NewResourceBase("file:///foo")
	if base.URI() != "file:///foo" {
		t.Errorf("URI() = %q, want file:///foo", base.URI())
	}
}

func TestResourceBase_ComponentsParsedFromURI(t *testing.T) {
	base := NewResourceBase("file:///usr/local/bin")
	if base.Scheme() != "file" {
		t.Errorf("Scheme() = %q, want file", base.Scheme())
	}
	if base.Host() != "" {
		t.Errorf("Host() = %q, want empty", base.Host())
	}
	if base.Path() != "/usr/local/bin" {
		t.Errorf("Path() = %q, want /usr/local/bin", base.Path())
	}
}

func TestResourceBase_ComponentsWithHost(t *testing.T) {
	base := NewResourceBase("git://github.com/org/repo")
	if base.Scheme() != "git" {
		t.Errorf("Scheme() = %q, want git", base.Scheme())
	}
	if base.Host() != "github.com" {
		t.Errorf("Host() = %q, want github.com", base.Host())
	}
	if base.Path() != "/org/repo" {
		t.Errorf("Path() = %q, want /org/repo", base.Path())
	}
}

func TestResourceBase_NewURI(t *testing.T) {
	base := NewResourceBase("")
	r := &testResource{scheme: "svc", host: "", path: "/nginx"}
	uri := base.NewURI(r)
	if uri != "svc:///nginx" {
		t.Errorf("NewURI() = %q, want svc:///nginx", uri)
	}
}

func TestResourceBase_NewURI_WithHost(t *testing.T) {
	base := NewResourceBase("")
	r := &testResource{scheme: "git", host: "github.com", path: "/org/repo"}
	uri := base.NewURI(r)
	if uri != "git://github.com/org/repo" {
		t.Errorf("NewURI() = %q, want git://github.com/org/repo", uri)
	}
}

func TestResourceBase_NewURI_File(t *testing.T) {
	base := NewResourceBase("")
	r := &testResource{scheme: "file", host: "", path: "/usr/local/bin"}
	uri := base.NewURI(r)
	if uri != "file:///usr/local/bin" {
		t.Errorf("NewURI() = %q, want file:///usr/local/bin", uri)
	}
}

func TestResourceBase_NewURI_RelativeFilePath(t *testing.T) {
	// A testResource that returns a relative path gets a relative URI.
	// Real file.Resource.Path() always canonicalizes via filepath.Abs.
	base := NewResourceBase("")
	r := &testResource{scheme: "file", host: "", path: "/etc/foo"}
	uri := base.NewURI(r)
	if !strings.HasPrefix(uri, "file:///") {
		t.Errorf("NewURI() = %q, want file:/// prefix", uri)
	}
}

func TestResourceBase_SatisfiesInterface(t *testing.T) {
	base := NewResourceBase("file:///bar")
	var r Resource = &base
	if r.URI() != "file:///bar" {
		t.Errorf("Resource.URI() = %q, want file:///bar", r.URI())
	}
}

// testResource is a concrete Resource implementation for testing NewURI.
type testResource struct {
	ResourceBase
	scheme string
	host   string
	path   string
}

func (r *testResource) URI() string    { return r.NewURI(r) }
func (r *testResource) Scheme() string { return r.scheme }
func (r *testResource) Host() string   { return r.host }
func (r *testResource) Path() string   { return r.path }

func TestConcreteResource_URI(t *testing.T) {
	r := &testResource{scheme: "file", host: "", path: "/usr/local/bin"}
	if r.URI() != "file:///usr/local/bin" {
		t.Errorf("URI() = %q, want file:///usr/local/bin", r.URI())
	}

	// Verify it satisfies the Resource interface
	var iface Resource = r
	if iface.URI() != "file:///usr/local/bin" {
		t.Errorf("Resource.URI() = %q, want file:///usr/local/bin", iface.URI())
	}
}

// testEmbeddingResource embeds ResourceBase, like file.Resource does.
type testEmbeddingResource struct {
	ResourceBase
	SourcePath string
}

func (r *testEmbeddingResource) URI() string    { return r.NewURI(r) }
func (r *testEmbeddingResource) Scheme() string { return SchemeFile }
func (r *testEmbeddingResource) Host() string   { return "" }
func (r *testEmbeddingResource) Path() string {
	abs, err := filepath.Abs(r.SourcePath)
	if err != nil {
		return r.SourcePath
	}
	return abs
}

func TestEmbeddingResource_URIComputedFromComponents(t *testing.T) {
	r := &testEmbeddingResource{SourcePath: "/etc/foo"}
	if r.URI() != "file:///etc/foo" {
		t.Errorf("URI() = %q, want file:///etc/foo", r.URI())
	}
	if r.Scheme() != "file" {
		t.Errorf("Scheme() = %q, want file", r.Scheme())
	}

	var iface Resource = r
	if iface.URI() != "file:///etc/foo" {
		t.Errorf("Resource.URI() = %q, want file:///etc/foo", iface.URI())
	}
}
