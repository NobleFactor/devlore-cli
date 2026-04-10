// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"os"
	"strings"
	"testing"
)

func TestResolveResources_NilCatalog(t *testing.T) {
	err := ResolveResources(nil)
	if err != nil {
		t.Fatalf("ResolveResources(nil) = %v, want nil", err)
	}
}

func TestResolveResources_AllExist(t *testing.T) {

	f, err := os.CreateTemp(t.TempDir(), "preflight-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	catalog := NewResourceCatalog()
	catalog.Resolve(newTestGraphResource("file://" + f.Name()))

	if err := ResolveResources(catalog); err != nil {
		t.Fatalf("ResolveResources() = %v, want nil", err)
	}
}

func TestResolveResources_MissingSource(t *testing.T) {
	catalog := NewResourceCatalog()
	catalog.Resolve(newTestGraphResource("file:///nonexistent/path/to/missing"))

	err := ResolveResources(catalog)
	if err == nil {
		t.Fatal("ResolveResources() = nil, want error for missing source")
	}
	if !strings.Contains(err.Error(), "missing source") {
		t.Errorf("error = %v, want to contain 'missing source'", err)
	}
	if !strings.Contains(err.Error(), "/nonexistent/path/to/missing") {
		t.Errorf("error = %v, want to contain the missing path", err)
	}
}

func TestResolveResources_MultipleMissing(t *testing.T) {
	catalog := NewResourceCatalog()
	catalog.Resolve(newTestGraphResource("file:///missing/alpha"))
	catalog.Resolve(newTestGraphResource("file:///missing/beta"))

	err := ResolveResources(catalog)
	if err == nil {
		t.Fatal("ResolveResources() = nil, want error")
	}
	if !strings.Contains(err.Error(), "2 missing source") {
		t.Errorf("error = %v, want to contain '2 missing source'", err)
	}
}

func TestResolveResources_NonFileScheme_Skipped(t *testing.T) {
	catalog := NewResourceCatalog()
	catalog.Resolve(newTestGraphResource("git://github.com/org/repo"))
	catalog.Resolve(newTestGraphResource("pkg:///homebrew/nginx"))
	catalog.Resolve(newTestGraphResource("svc:///nginx"))

	if err := ResolveResources(catalog); err != nil {
		t.Fatalf("ResolveResources() = %v, want nil (non-file schemes skipped)", err)
	}
}

func TestResolveResources_ShadowedEntry_Skipped(t *testing.T) {

	catalog := NewResourceCatalog()
	// Resolve then shadow — the shadow supersedes the discovery entry.
	catalog.Resolve(newTestGraphResource("file:///nonexistent/file"))

	_, _ = catalog.Shadow(newTestGraphResource("file:///nonexistent/file"), "writer-node")

	// The discovery entry is superseded; no file check needed.
	if err := ResolveResources(catalog); err != nil {
		t.Fatalf("ResolveResources() = %v, want nil (shadowed entry)", err)
	}
}

func TestResolveResources_MixedSchemes(t *testing.T) {

	f, err := os.CreateTemp(t.TempDir(), "preflight-*")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	catalog := NewResourceCatalog()
	catalog.Resolve(newTestGraphResource("file://" + f.Name()))        // exists
	catalog.Resolve(newTestGraphResource("git://github.com/org/repo")) // non-file, skipped
	catalog.Resolve(newTestGraphResource("file:///nonexistent/path"))  // missing

	err = ResolveResources(catalog)
	if err == nil {
		t.Fatal("ResolveResources() = nil, want error for missing source")
	}
	if !strings.Contains(err.Error(), "1 missing source") {
		t.Errorf("error = %v, want '1 missing source'", err)
	}
}
