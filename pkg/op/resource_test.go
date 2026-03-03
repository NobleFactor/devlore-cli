// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"path/filepath"
	"strings"
	"sync"
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
		path   string
		want   string
	}{
		{SchemeGit, "github.com/org/repo", "git://github.com/org/repo"},
		{SchemePackage, "brew/vim", "pkg://brew/vim"},
		{SchemeService, "nginx", "svc://nginx"},
		{SchemeMem, "buffer-1", "mem://buffer-1"},
	}
	for _, tt := range tests {
		t.Run(tt.scheme, func(t *testing.T) {
			got := ResourceURI(tt.scheme, tt.path)
			if got != tt.want {
				t.Errorf("ResourceURI(%s, %s) = %q, want %q", tt.scheme, tt.path, got, tt.want)
			}
		})
	}
}

func TestResourceManager_EnsureCataloged(t *testing.T) {
	mgr := NewResourceManager()

	id1 := mgr.EnsureCataloged("file:///a", "")
	id2 := mgr.EnsureCataloged("file:///b", "node-1")
	id3 := mgr.EnsureCataloged("file:///c", "")

	if id1 != "res-1" {
		t.Errorf("first ID = %q, want res-1", id1)
	}
	if id2 != "res-2" {
		t.Errorf("second ID = %q, want res-2", id2)
	}
	if id3 != "res-3" {
		t.Errorf("third ID = %q, want res-3", id3)
	}
}

func TestResourceManager_Lookup(t *testing.T) {
	mgr := NewResourceManager()

	id := mgr.EnsureCataloged("file:///foo", "node-1")

	r, ok := mgr.Lookup(id)
	if !ok {
		t.Fatalf("Lookup(%q) returned false", id)
	}
	if r.URI != "file:///foo" {
		t.Errorf("r.URI = %q, want file:///foo", r.URI)
	}
	if r.ID != id {
		t.Errorf("r.ID = %q, want %q", r.ID, id)
	}
	if r.OriginNodeID != "node-1" {
		t.Errorf("r.OriginNodeID = %q, want node-1", r.OriginNodeID)
	}

	// Unknown ID returns false
	_, ok = mgr.Lookup("res-999")
	if ok {
		t.Error("Lookup(res-999) should return false")
	}
}

func TestResourceManager_LedgerLen(t *testing.T) {
	mgr := NewResourceManager()

	if mgr.LedgerLen() != 0 {
		t.Errorf("empty ledger len = %d, want 0", mgr.LedgerLen())
	}

	mgr.EnsureCataloged("file:///a", "")
	if mgr.LedgerLen() != 1 {
		t.Errorf("after 1 catalog, len = %d, want 1", mgr.LedgerLen())
	}

	mgr.EnsureCataloged("file:///b", "")
	mgr.EnsureCataloged("file:///c", "")
	if mgr.LedgerLen() != 3 {
		t.Errorf("after 3 catalogs, len = %d, want 3", mgr.LedgerLen())
	}
}

func TestResourceManager_ConcurrentAccess(t *testing.T) {
	mgr := NewResourceManager()
	const goroutines = 50
	var wg sync.WaitGroup
	ids := make(chan string, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := mgr.EnsureCataloged("file:///concurrent", "")
			ids <- id
		}()
	}

	wg.Wait()
	close(ids)

	seen := make(map[string]bool)
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate ID from concurrent access: %q", id)
		}
		seen[id] = true
	}

	if len(seen) != goroutines {
		t.Errorf("expected %d unique IDs, got %d", goroutines, len(seen))
	}
}
