// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"strings"
	"testing"
)

// testEmbeddingResource is a minimal Resource used by resource_test.go and resource_catalog_test.go.
type testEmbeddingResource struct {
	ResourceBase
	SourcePath string
}

func (r *testEmbeddingResource) URI() string {
	return r.ResourceBase.URI()
}

func TestNewResourceBase_MintsTagURI(t *testing.T) {

	base, err := NewResourceBase(nil, "file:///foo", reflect.TypeFor[*testEmbeddingResource]())
	if err != nil {
		t.Fatalf("NewResourceBase: %v", err)
	}

	expectedTypeID := "github.com/NobleFactor/devlore-cli/pkg/op.testEmbeddingResource"
	expectedURI := "tag:devlore.noblefactor.com,2026-01-01:file:///foo#" + expectedTypeID

	if got := base.URI(); got != expectedURI {
		t.Errorf("URI() = %q, want %q", got, expectedURI)
	}
	if got := base.ReachabilityURI(); got != "file:///foo" {
		t.Errorf("ReachabilityURI() = %q, want %q", got, "file:///foo")
	}
	if got := base.ResourceType(); got != expectedTypeID {
		t.Errorf("ResourceType() = %q, want %q", got, expectedTypeID)
	}
}

func TestNewResourceBase_EmptySpecific(t *testing.T) {

	base, err := NewResourceBase(nil, "", reflect.TypeFor[*testEmbeddingResource]())
	if err != nil {
		t.Fatalf("NewResourceBase: %v", err)
	}

	if got := base.ReachabilityURI(); got != "" {
		t.Errorf("ReachabilityURI() = %q, want empty (deferred)", got)
	}
	if !strings.HasSuffix(base.URI(), "#github.com/NobleFactor/devlore-cli/pkg/op.testEmbeddingResource") {
		t.Errorf("URI() = %q does not end with the expected fragment", base.URI())
	}
}

func TestNewResourceBase_RejectsFragmentInSpecific(t *testing.T) {

	_, err := NewResourceBase(nil, "bad#value", reflect.TypeFor[*testEmbeddingResource]())
	if err == nil {
		t.Fatal("NewResourceBase: expected error for '#' in specific, got nil")
	}
	if !strings.Contains(err.Error(), "#") {
		t.Errorf("error %q does not mention '#'", err)
	}
}

func TestExtractTagSpecific_RoundTrip(t *testing.T) {

	base, err := NewResourceBase(nil, "mem:x/y", reflect.TypeFor[*testEmbeddingResource]())
	if err != nil {
		t.Fatalf("NewResourceBase: %v", err)
	}

	specific, typeID, err := ExtractTagSpecific(base.URI())
	if err != nil {
		t.Fatalf("ExtractTagSpecific: %v", err)
	}
	if specific != "mem:x/y" {
		t.Errorf("specific = %q, want %q", specific, "mem:x/y")
	}
	if typeID != "github.com/NobleFactor/devlore-cli/pkg/op.testEmbeddingResource" {
		t.Errorf("typeID = %q, want %q", typeID, "github.com/NobleFactor/devlore-cli/pkg/op.testEmbeddingResource")
	}
}

func TestExtractTagSpecific_Rejections(t *testing.T) {

	cases := []struct {
		name string
		uri  string
	}{
		{"wrong prefix", "http://example.com"},
		{"missing fragment", "tag:devlore.noblefactor.com,2026-01-01:some/specific"},
		{"empty fragment", "tag:devlore.noblefactor.com,2026-01-01:some/specific#"},
		{"wrong authority", "tag:example.com,2026-01-01:s#pkg.Type"},
		{"wrong date", "tag:devlore.noblefactor.com,2020-01-01:s#pkg.Type"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := ExtractTagSpecific(c.uri); err == nil {
				t.Errorf("ExtractTagSpecific(%q) = nil, want error", c.uri)
			}
		})
	}
}

func TestDefer_ProducesDeferredForm(t *testing.T) {

	r := Defer[testEmbeddingResource, *testEmbeddingResource](nil)

	if r == nil {
		t.Fatal("Defer returned nil")
	}
	if got := r.ReachabilityURI(); got != "" {
		t.Errorf("ReachabilityURI() = %q, want empty (deferred)", got)
	}
	if got := r.ResourceType(); got != "github.com/NobleFactor/devlore-cli/pkg/op.testEmbeddingResource" {
		t.Errorf("ResourceType() = %q, want op.testEmbeddingResource", got)
	}
}

func TestResourceBase_SatisfiesInterface(t *testing.T) {

	base, err := NewResourceBase(nil, "file:///bar", reflect.TypeFor[*testEmbeddingResource]())
	if err != nil {
		t.Fatalf("NewResourceBase: %v", err)
	}

	var r Resource = &testEmbeddingResource{ResourceBase: base}
	if !strings.Contains(r.URI(), "file:///bar") {
		t.Errorf("Resource.URI() = %q, want to contain %q", r.URI(), "file:///bar")
	}
}
