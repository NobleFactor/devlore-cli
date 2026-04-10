// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"testing"
)

func TestResourceBase_URI(t *testing.T) {
	base := NewResourceBase(nil, "file:///foo")
	if base.URI() != "file:///foo" {
		t.Errorf("URI() = %q, want file:///foo", base.URI())
	}
}


func TestResourceBase_ParseHierarchicalURI(t *testing.T) {
	base := NewResourceBase(nil, "file:///usr/local/bin")
	if base.Scheme() != "file" {
		t.Errorf("Scheme() = %q, want file", base.Scheme())
	}
	if base.Host() != "" {
		t.Errorf("Host() = %q, want empty", base.Host())
	}
	if base.Path() != "/usr/local/bin" {
		t.Errorf("Path() = %q, want /usr/local/bin", base.Path())
	}
	if base.Opaque() != "" {
		t.Errorf("Opaque() = %q, want empty (hierarchical URI)", base.Opaque())
	}
}

func TestResourceBase_ParseOpaqueURI(t *testing.T) {
	base := NewResourceBase(nil, "pkg:brew/jq@1.7")
	if base.Scheme() != "pkg" {
		t.Errorf("Scheme() = %q, want pkg", base.Scheme())
	}
	if base.Opaque() != "brew/jq@1.7" {
		t.Errorf("Opaque() = %q, want brew/jq@1.7", base.Opaque())
	}
	if base.Host() != "" {
		t.Errorf("Host() = %q, want empty (opaque URI)", base.Host())
	}
	if base.Path() != "" {
		t.Errorf("Path() = %q, want empty (opaque URI)", base.Path())
	}
}

func TestResourceBase_ParseFragment(t *testing.T) {
	base := NewResourceBase(nil, "mem:callable/file.Reducer/myfn#node1")
	if base.Scheme() != "mem" {
		t.Errorf("Scheme() = %q, want mem", base.Scheme())
	}
	if base.Fragment() != "node1" {
		t.Errorf("Fragment() = %q, want node1", base.Fragment())
	}
}

func TestResourceBase_SatisfiesInterface(t *testing.T) {
	var r Resource = new(NewResourceBase(nil, "file:///bar"))
	if r.URI() != "file:///bar" {
		t.Errorf("Resource.URI() = %q, want file:///bar", r.URI())
	}
}

func TestResourceBase_ParseInvalidURI(t *testing.T) {
	base := NewResourceBase(nil, "://bad")
	if base.Scheme() != "" {
		t.Errorf("Scheme() = %q, want empty for invalid URI", base.Scheme())
	}
}

// testEmbeddingResource is a minimal Resource used by resource_catalog_test.go.
type testEmbeddingResource struct {
	ResourceBase
	SourcePath string
}

func (r *testEmbeddingResource) URI() string {
	return r.ResourceBase.URI()
}
