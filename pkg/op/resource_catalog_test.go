// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"strings"
	"sync"
	"testing"
)

func TestCatalog_Resolve_FirstAccess(t *testing.T) {
	cat := NewResourceCatalog()

	id := cat.Resolve("file:///first")

	r, ok := cat.Lookup(id)
	if !ok {
		t.Fatalf("Lookup(%q) returned false", id)
	}
	if r.URI() != "file:///first" {
		t.Errorf("URI() = %q, want file:///first", r.URI())
	}
	// Discovery entry has no origin.
	base := r.resourceBase()
	if base.originID != "" {
		t.Errorf("discovery resource should have empty originID, got %q", base.originID)
	}
}

func TestCatalog_Resolve_Idempotent(t *testing.T) {
	cat := NewResourceCatalog()

	id1 := cat.Resolve("file:///same")
	id2 := cat.Resolve("file:///same")

	if id1 != id2 {
		t.Errorf("Resolve same URI twice: %q != %q", id1, id2)
	}
	if cat.Len() != 1 {
		t.Errorf("expected 1 catalog entry after 2 resolves of same URI, got %d", cat.Len())
	}
}

func TestCatalog_Shadow(t *testing.T) {
	cat := NewResourceCatalog()

	origID := cat.Resolve("file:///target")
	res := &testEmbeddingResource{SourcePath: "/target"}
	shadowID, err := cat.Shadow(res, "writer-node")
	if err != nil {
		t.Fatalf("Shadow error: %v", err)
	}

	if origID == shadowID {
		t.Error("Shadow should create a new resource ID, got same as original")
	}
	if cat.Len() != 2 {
		t.Errorf("expected 2 catalog entries, got %d", cat.Len())
	}

	r, ok := cat.Lookup(shadowID)
	if !ok {
		t.Fatalf("Lookup(%q) returned false", shadowID)
	}
	base := r.resourceBase()
	if base.originID != "writer-node" {
		t.Errorf("shadow originID = %q, want writer-node", base.originID)
	}
}

func TestCatalog_Shadow_OverwritesResolve(t *testing.T) {
	cat := NewResourceCatalog()

	cat.Resolve("file:///overwrite")
	res := &testEmbeddingResource{SourcePath: "/overwrite"}
	if _, err := cat.Shadow(res, "nodeA"); err != nil {
		t.Fatalf("Shadow error: %v", err)
	}
	resolvedID := cat.Resolve("file:///overwrite")

	// Resolve after Shadow should return the shadow's ID
	r, _ := cat.Lookup(resolvedID)
	base := r.resourceBase()
	if base.originID != "nodeA" {
		t.Errorf("resolve after shadow: originID = %q, want nodeA", base.originID)
	}
}

func TestCatalog_ImplicitDependency(t *testing.T) {
	cat := NewResourceCatalog()

	// Shadow by nodeA creates a resource version owned by nodeA
	res := &testEmbeddingResource{SourcePath: "/dep"}
	if _, err := cat.Shadow(res, "nodeA"); err != nil {
		t.Fatalf("Shadow error: %v", err)
	}

	// Resolve (as if nodeB is reading) returns nodeA's version
	id := cat.Resolve("file:///dep")

	r, _ := cat.Lookup(id)
	base := r.resourceBase()
	if base.originID != "nodeA" {
		t.Errorf("implicit dependency: originID = %q, want nodeA", base.originID)
	}
}

func TestCatalog_Current_Empty(t *testing.T) {
	cat := NewResourceCatalog()

	if got := cat.Current("file:///unknown"); got != "" {
		t.Errorf("Current for unknown URI = %q, want empty", got)
	}
}

func TestCatalog_Current_AfterResolve(t *testing.T) {
	cat := NewResourceCatalog()

	id := cat.Resolve("file:///resolved")
	if got := cat.Current("file:///resolved"); got != id {
		t.Errorf("Current after Resolve = %q, want %q", got, id)
	}
}

func TestCatalog_Current_AfterShadow(t *testing.T) {
	cat := NewResourceCatalog()

	cat.Resolve("file:///shadowed")
	res := &testEmbeddingResource{SourcePath: "/shadowed"}
	shadowID, err := cat.Shadow(res, "node-1")
	if err != nil {
		t.Fatalf("Shadow error: %v", err)
	}

	if got := cat.Current("file:///shadowed"); got != shadowID {
		t.Errorf("Current after Shadow = %q, want %q", got, shadowID)
	}
}

func TestCatalog_MultipleURIs(t *testing.T) {
	cat := NewResourceCatalog()

	id1 := cat.Resolve("file:///alpha")
	id2 := cat.Resolve("file:///beta")

	if id1 == id2 {
		t.Error("different URIs should have different IDs")
	}
	if cat.Current("file:///alpha") != id1 {
		t.Error("alpha should still map to its original ID")
	}
	if cat.Current("file:///beta") != id2 {
		t.Error("beta should still map to its original ID")
	}
}

func TestCatalog_LedgerLen(t *testing.T) {
	cat := NewResourceCatalog()

	if cat.Len() != 0 {
		t.Errorf("empty catalog length = %d, want 0", cat.Len())
	}

	cat.Resolve("file:///a")
	if cat.Len() != 1 {
		t.Errorf("after 1 resolve, len = %d, want 1", cat.Len())
	}

	cat.Resolve("file:///b")
	cat.Resolve("file:///c")
	if cat.Len() != 3 {
		t.Errorf("after 3 resolves, len = %d, want 3", cat.Len())
	}
}

func TestCatalog_Lookup_NotFound(t *testing.T) {
	cat := NewResourceCatalog()
	_, ok := cat.Lookup("res-999")
	if ok {
		t.Error("Lookup(res-999) should return false")
	}
}

func TestCatalog_ConcurrentAccess(t *testing.T) {
	cat := NewResourceCatalog()
	const goroutines = 50
	var wg sync.WaitGroup
	ids := make(chan string, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			base := NewResourceBase("file:///concurrent")
			id, _ := cat.Shadow(&base, "")
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

func TestCatalog_DiscoveryURIs_ReturnsUnshadowed(t *testing.T) {
	cat := NewResourceCatalog()

	cat.Resolve("file:///source1")
	cat.Resolve("file:///source2")
	// Shadow a different URI — not a supersede.
	res := &testEmbeddingResource{SourcePath: "/target"}
	if _, err := cat.Shadow(res, "node-1"); err != nil {
		t.Fatalf("Shadow error: %v", err)
	}

	uris := cat.DiscoveryURIs()
	if len(uris) != 2 {
		t.Fatalf("DiscoveryURIs() returned %d, want 2", len(uris))
	}
}

func TestCatalog_DiscoveryURIs_ShadowSupersedes(t *testing.T) {
	cat := NewResourceCatalog()

	cat.Resolve("file:///source")
	res := &testEmbeddingResource{SourcePath: "/source"}
	if _, err := cat.Shadow(res, "node-1"); err != nil {
		t.Fatalf("Shadow error: %v", err)
	}

	uris := cat.DiscoveryURIs()
	if len(uris) != 0 {
		t.Fatalf("DiscoveryURIs() returned %d, want 0 (shadow superseded)", len(uris))
	}
}

func TestCatalog_DiscoveryURIs_Empty(t *testing.T) {
	cat := NewResourceCatalog()

	uris := cat.DiscoveryURIs()
	if len(uris) != 0 {
		t.Fatalf("DiscoveryURIs() returned %d, want 0", len(uris))
	}
}

func TestCatalog_IDsAreMonotonic(t *testing.T) {
	cat := NewResourceCatalog()

	id1 := cat.Resolve("file:///a")
	if id1 != "res-1" {
		t.Errorf("first ID = %q, want res-1", id1)
	}

	base := NewResourceBase("file:///b")
	id2, err := cat.Shadow(&base, "node-1")
	if err != nil {
		t.Fatalf("Shadow error: %v", err)
	}
	if id2 != "res-2" {
		t.Errorf("second ID = %q, want res-2", id2)
	}

	id3 := cat.Resolve("file:///c")
	if id3 != "res-3" {
		t.Errorf("third ID = %q, want res-3", id3)
	}
}

func TestCatalog_Shadow_ConflictDetection(t *testing.T) {
	cat := NewResourceCatalog()

	// First shadow by nodeA — should succeed.
	res1 := &testEmbeddingResource{SourcePath: "/conflict"}
	_, err := cat.Shadow(res1, "nodeA")
	if err != nil {
		t.Fatalf("first Shadow error: %v", err)
	}

	// Second shadow by nodeB on the same URI — should conflict.
	res2 := &testEmbeddingResource{SourcePath: "/conflict"}
	_, err = cat.Shadow(res2, "nodeB")
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "resource conflict") {
		t.Errorf("error = %q, want 'resource conflict' substring", err)
	}
	if !strings.Contains(err.Error(), "nodeA") || !strings.Contains(err.Error(), "nodeB") {
		t.Errorf("error = %q, want both node IDs mentioned", err)
	}
}

func TestCatalog_Shadow_SameOriginNoConflict(t *testing.T) {
	cat := NewResourceCatalog()

	// Same origin shadowing twice — should NOT conflict.
	res1 := &testEmbeddingResource{SourcePath: "/same"}
	_, err := cat.Shadow(res1, "nodeA")
	if err != nil {
		t.Fatalf("first Shadow error: %v", err)
	}

	res2 := &testEmbeddingResource{SourcePath: "/same"}
	_, err = cat.Shadow(res2, "nodeA")
	if err != nil {
		t.Errorf("same-origin Shadow should not conflict, got: %v", err)
	}
}

func TestCatalog_Shadow_DiscoveryThenShadowNoConflict(t *testing.T) {
	cat := NewResourceCatalog()

	// Resolve creates a discovery entry (empty originID).
	cat.Resolve("file:///discovered")

	// Shadow by nodeA should NOT conflict with discovery.
	res := &testEmbeddingResource{SourcePath: "/discovered"}
	_, err := cat.Shadow(res, "nodeA")
	if err != nil {
		t.Errorf("shadow after discovery should not conflict, got: %v", err)
	}
}

func TestCatalog_Shadow_EmptyOriginNoConflict(t *testing.T) {
	cat := NewResourceCatalog()

	// Shadow with empty originID (discovery-like) should never conflict.
	res1 := &testEmbeddingResource{SourcePath: "/empty-origin"}
	_, err := cat.Shadow(res1, "nodeA")
	if err != nil {
		t.Fatalf("first Shadow error: %v", err)
	}

	res2 := &testEmbeddingResource{SourcePath: "/empty-origin"}
	_, err = cat.Shadow(res2, "")
	if err != nil {
		t.Errorf("empty-origin Shadow should not conflict, got: %v", err)
	}
}
