// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"strings"
	"sync"
	"testing"
)

// catRes constructs a minimal typed resource with the given URI for catalog tests.
func catRes(uri string) *testGraphResource {
	return newTestGraphResource(uri)
}

func TestCatalog_Resolve_FirstAccess(t *testing.T) {
	cat := NewResourceCatalog()

	_, id := cat.Resolve(catRes("file:///first"))

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

	_, id1 := cat.Resolve(catRes("file:///same"))
	_, id2 := cat.Resolve(catRes("file:///same"))

	if id1 != id2 {
		t.Errorf("Resolve same URI twice: %q != %q", id1, id2)
	}
	if cat.Len() != 1 {
		t.Errorf("expected 1 catalog entry after 2 resolves of same URI, got %d", cat.Len())
	}
}

func TestCatalog_Shadow(t *testing.T) {
	cat := NewResourceCatalog()

	_, origID := cat.Resolve(catRes("file:///target"))
	shadowID, err := cat.Shadow(catRes("file:///target"), "writer-node")
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

	cat.Resolve(catRes("file:///overwrite"))
	if _, err := cat.Shadow(catRes("file:///overwrite"), "nodeA"); err != nil {
		t.Fatalf("Shadow error: %v", err)
	}
	_, resolvedID := cat.Resolve(catRes("file:///overwrite"))

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
	if _, err := cat.Shadow(catRes("file:///dep"), "nodeA"); err != nil {
		t.Fatalf("Shadow error: %v", err)
	}

	// Resolve (as if nodeB is reading) returns nodeA's version
	_, id := cat.Resolve(catRes("file:///dep"))

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

	_, id := cat.Resolve(catRes("file:///resolved"))
	if got := cat.Current("file:///resolved"); got != id {
		t.Errorf("Current after Resolve = %q, want %q", got, id)
	}
}

func TestCatalog_Current_AfterShadow(t *testing.T) {
	cat := NewResourceCatalog()

	cat.Resolve(catRes("file:///shadowed"))
	shadowID, err := cat.Shadow(catRes("file:///shadowed"), "node-1")
	if err != nil {
		t.Fatalf("Shadow error: %v", err)
	}

	if got := cat.Current("file:///shadowed"); got != shadowID {
		t.Errorf("Current after Shadow = %q, want %q", got, shadowID)
	}
}

func TestCatalog_MultipleURIs(t *testing.T) {
	cat := NewResourceCatalog()

	_, id1 := cat.Resolve(catRes("file:///alpha"))
	_, id2 := cat.Resolve(catRes("file:///beta"))

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

	cat.Resolve(catRes("file:///a"))
	if cat.Len() != 1 {
		t.Errorf("after 1 resolve, len = %d, want 1", cat.Len())
	}

	cat.Resolve(catRes("file:///b"))
	cat.Resolve(catRes("file:///c"))
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
			id, _ := cat.Shadow(catRes("file:///concurrent"), "")
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

	cat.Resolve(catRes("file:///source1"))
	cat.Resolve(catRes("file:///source2"))
	// Shadow a different URI — not a supersede.
	if _, err := cat.Shadow(catRes("file:///target"), "node-1"); err != nil {
		t.Fatalf("Shadow error: %v", err)
	}

	uris := cat.DiscoveryURIs()
	if len(uris) != 2 {
		t.Fatalf("DiscoveryURIs() returned %d, want 2", len(uris))
	}
}

func TestCatalog_DiscoveryURIs_ShadowSupersedes(t *testing.T) {
	cat := NewResourceCatalog()

	cat.Resolve(catRes("file:///source"))
	if _, err := cat.Shadow(catRes("file:///source"), "node-1"); err != nil {
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

	_, id1 := cat.Resolve(catRes("file:///a"))
	if id1 != "res-1" {
		t.Errorf("first ID = %q, want res-1", id1)
	}

	id2, err := cat.Shadow(catRes("file:///b"), "node-1")
	if err != nil {
		t.Fatalf("Shadow error: %v", err)
	}
	if id2 != "res-2" {
		t.Errorf("second ID = %q, want res-2", id2)
	}

	_, id3 := cat.Resolve(catRes("file:///c"))
	if id3 != "res-3" {
		t.Errorf("third ID = %q, want res-3", id3)
	}
}

func TestCatalog_Shadow_ConflictDetection(t *testing.T) {
	cat := NewResourceCatalog()

	// First shadow by nodeA — should succeed.
	_, err := cat.Shadow(catRes("file:///conflict"), "nodeA")
	if err != nil {
		t.Fatalf("first Shadow error: %v", err)
	}

	// Second shadow by nodeB on the same URI — should conflict.
	_, err = cat.Shadow(catRes("file:///conflict"), "nodeB")
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
	_, err := cat.Shadow(catRes("file:///same"), "nodeA")
	if err != nil {
		t.Fatalf("first Shadow error: %v", err)
	}

	_, err = cat.Shadow(catRes("file:///same"), "nodeA")
	if err != nil {
		t.Errorf("same-origin Shadow should not conflict, got: %v", err)
	}
}

func TestCatalog_Shadow_DiscoveryThenShadowNoConflict(t *testing.T) {
	cat := NewResourceCatalog()

	// Resolve creates a discovery entry (empty originID).
	cat.Resolve(catRes("file:///discovered"))

	// Shadow by nodeA should NOT conflict with discovery.
	_, err := cat.Shadow(catRes("file:///discovered"), "nodeA")
	if err != nil {
		t.Errorf("shadow after discovery should not conflict, got: %v", err)
	}
}

func TestCatalog_Shadow_EmptyOriginNoConflict(t *testing.T) {
	cat := NewResourceCatalog()

	// Shadow with empty originID (discovery-like) should never conflict.
	_, err := cat.Shadow(catRes("file:///empty-origin"), "nodeA")
	if err != nil {
		t.Fatalf("first Shadow error: %v", err)
	}

	_, err = cat.Shadow(catRes("file:///empty-origin"), "")
	if err != nil {
		t.Errorf("empty-origin Shadow should not conflict, got: %v", err)
	}
}
