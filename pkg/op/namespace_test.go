// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "testing"

func TestNamespace_Resolve_FirstAccess(t *testing.T) {
	mgr := NewResourceManager()
	ns := NewNamespaceMap()

	id := ns.Resolve(mgr, "file:///first")

	r, ok := mgr.Lookup(id)
	if !ok {
		t.Fatalf("Lookup(%q) returned false", id)
	}
	if r.OriginNodeID != "" {
		t.Errorf("discovery resource should have empty OriginNodeID, got %q", r.OriginNodeID)
	}
	if r.URI != "file:///first" {
		t.Errorf("r.URI = %q, want file:///first", r.URI)
	}
}

func TestNamespace_Resolve_Idempotent(t *testing.T) {
	mgr := NewResourceManager()
	ns := NewNamespaceMap()

	id1 := ns.Resolve(mgr, "file:///same")
	id2 := ns.Resolve(mgr, "file:///same")

	if id1 != id2 {
		t.Errorf("Resolve same URI twice: %q != %q", id1, id2)
	}
	if mgr.LedgerLen() != 1 {
		t.Errorf("expected 1 ledger entry after 2 resolves of same URI, got %d", mgr.LedgerLen())
	}
}

func TestNamespace_Shadow(t *testing.T) {
	mgr := NewResourceManager()
	ns := NewNamespaceMap()

	origID := ns.Resolve(mgr, "file:///target")
	shadowID := ns.Shadow(mgr, "file:///target", "writer-node")

	if origID == shadowID {
		t.Error("Shadow should create a new resource ID, got same as original")
	}
	if mgr.LedgerLen() != 2 {
		t.Errorf("expected 2 ledger entries, got %d", mgr.LedgerLen())
	}

	r, ok := mgr.Lookup(shadowID)
	if !ok {
		t.Fatalf("Lookup(%q) returned false", shadowID)
	}
	if r.OriginNodeID != "writer-node" {
		t.Errorf("shadow OriginNodeID = %q, want writer-node", r.OriginNodeID)
	}
}

func TestNamespace_Shadow_OverwritesResolve(t *testing.T) {
	mgr := NewResourceManager()
	ns := NewNamespaceMap()

	ns.Resolve(mgr, "file:///overwrite")
	ns.Shadow(mgr, "file:///overwrite", "nodeA")
	resolvedID := ns.Resolve(mgr, "file:///overwrite")

	// Resolve after Shadow should return the shadow's ID
	r, _ := mgr.Lookup(resolvedID)
	if r.OriginNodeID != "nodeA" {
		t.Errorf("resolve after shadow: OriginNodeID = %q, want nodeA", r.OriginNodeID)
	}
}

func TestNamespace_ImplicitDependency(t *testing.T) {
	mgr := NewResourceManager()
	ns := NewNamespaceMap()

	// Shadow by nodeA creates a resource version owned by nodeA
	ns.Shadow(mgr, "file:///dep", "nodeA")

	// Resolve (as if nodeB is reading) returns nodeA's version
	id := ns.Resolve(mgr, "file:///dep")

	r, _ := mgr.Lookup(id)
	if r.OriginNodeID != "nodeA" {
		t.Errorf("implicit dependency: OriginNodeID = %q, want nodeA", r.OriginNodeID)
	}
}

func TestNamespace_Current_Empty(t *testing.T) {
	ns := NewNamespaceMap()

	if got := ns.Current("file:///unknown"); got != "" {
		t.Errorf("Current for unknown URI = %q, want empty", got)
	}
}

func TestNamespace_Current_AfterResolve(t *testing.T) {
	mgr := NewResourceManager()
	ns := NewNamespaceMap()

	id := ns.Resolve(mgr, "file:///resolved")
	if got := ns.Current("file:///resolved"); got != id {
		t.Errorf("Current after Resolve = %q, want %q", got, id)
	}
}

func TestNamespace_Current_AfterShadow(t *testing.T) {
	mgr := NewResourceManager()
	ns := NewNamespaceMap()

	ns.Resolve(mgr, "file:///shadowed")
	shadowID := ns.Shadow(mgr, "file:///shadowed", "node-1")

	if got := ns.Current("file:///shadowed"); got != shadowID {
		t.Errorf("Current after Shadow = %q, want %q", got, shadowID)
	}
}

func TestNamespace_MultipleURIs(t *testing.T) {
	mgr := NewResourceManager()
	ns := NewNamespaceMap()

	id1 := ns.Resolve(mgr, "file:///alpha")
	id2 := ns.Resolve(mgr, "file:///beta")

	if id1 == id2 {
		t.Error("different URIs should have different IDs")
	}
	if ns.Current("file:///alpha") != id1 {
		t.Error("alpha should still map to its original ID")
	}
	if ns.Current("file:///beta") != id2 {
		t.Error("beta should still map to its original ID")
	}
}
