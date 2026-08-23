// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"reflect"
	"strings"
	"testing"
)

// region TEST FUNCTIONS

// TestResolveDispatchResource_StringKeyHitReturnsTheCanonical pins the key half of the §5.6 seam: a
// string slot value at graph dispatch is an identity, and resolution hands back the run catalog's
// canonical entry — the very object the ledger holds.
func TestResolveDispatchResource_StringKeyHitReturnsTheCanonical(t *testing.T) {

	catalog := NewResourceCatalog()
	entry := newLifecycle("test:///claimed", AddressingLocation)
	catalog.Resolve(entry)

	activation := &ActivationRecord{Graph: &Graph{}, RuntimeEnvironment: &RuntimeEnvironment{ResourceCatalog: catalog}}

	resolved, applied, err := resolveDispatchResource(activation, "test:///claimed", reflect.TypeFor[*lifecycleResource]())
	if !applied || err != nil {
		t.Fatalf("resolveDispatchResource(key) = applied %t, err %v; want applied, nil", applied, err)
	}
	if resolved != Resource(entry) {
		t.Errorf("resolved %p is not the canonical entry %p — dispatch must hand out the ledger's object", resolved, entry)
	}
}

// TestResolveDispatchResource_ResourceValueResolvesByURI pins the captured-object half: a Resource slot
// value resolves by its URI to the canonical, never dispatching the captured object itself — the aliasing
// between planning catalog and run clone is severed, not load-bearing.
func TestResolveDispatchResource_ResourceValueResolvesByURI(t *testing.T) {

	catalog := NewResourceCatalog()
	canonical := newLifecycle("test:///claimed", AddressingLocation)
	catalog.Resolve(canonical)

	captured := newLifecycle("test:///claimed", AddressingLocation) // the same identity, a different object

	activation := &ActivationRecord{Graph: &Graph{}, RuntimeEnvironment: &RuntimeEnvironment{ResourceCatalog: catalog}}

	resolved, applied, err := resolveDispatchResource(activation, captured, reflect.TypeFor[*lifecycleResource]())
	if !applied || err != nil {
		t.Fatalf("resolveDispatchResource(resource) = applied %t, err %v; want applied, nil", applied, err)
	}
	if resolved != Resource(canonical) {
		t.Errorf("resolved %p is not the canonical %p — the captured object must not dispatch", resolved, canonical)
	}
}

// TestResolveDispatchResource_MissRefuses pins the refusal: a graph-dispatch catalog miss is the
// catalog's verdict naming the key — never fresh construction.
func TestResolveDispatchResource_MissRefuses(t *testing.T) {

	activation := &ActivationRecord{
		Graph:              &Graph{},
		RuntimeEnvironment: &RuntimeEnvironment{ResourceCatalog: NewResourceCatalog()},
	}

	_, applied, err := resolveDispatchResource(activation, "test:///ghost", reflect.TypeFor[*lifecycleResource]())
	if !applied {
		t.Fatal("resolveDispatchResource(miss) did not apply — the seam must own resource-typed slots at graph dispatch")
	}
	if err == nil || !strings.Contains(err.Error(), "not in the run catalog") {
		t.Errorf("miss error = %v, want the catalog's verdict naming the key", err)
	}
}

// TestResolveDispatchResource_NonIdentityValueRefuses pins the sealing: a value that is neither a
// Resource nor a string cannot name a resource at graph dispatch.
func TestResolveDispatchResource_NonIdentityValueRefuses(t *testing.T) {

	activation := &ActivationRecord{
		Graph:              &Graph{},
		RuntimeEnvironment: &RuntimeEnvironment{ResourceCatalog: NewResourceCatalog()},
	}

	_, applied, err := resolveDispatchResource(activation, 42, reflect.TypeFor[*lifecycleResource]())
	if !applied {
		t.Fatal("resolveDispatchResource(non-identity) did not apply")
	}
	if err == nil || !strings.Contains(err.Error(), "cannot name a resource") {
		t.Errorf("non-identity error = %v, want the cannot-name refusal", err)
	}
}

// TestResolveDispatchResource_SessionDispatchFallsThrough pins the gate: a nil-Graph activation —
// immediate mode's shape — leaves the cascade untouched, so session construction survives (§5.6's
// second carve-out).
func TestResolveDispatchResource_SessionDispatchFallsThrough(t *testing.T) {

	activation := &ActivationRecord{RuntimeEnvironment: &RuntimeEnvironment{ResourceCatalog: NewResourceCatalog()}}

	_, applied, _ := resolveDispatchResource(activation, "a/path", reflect.TypeFor[*lifecycleResource]())
	if applied {
		t.Error("resolveDispatchResource applied on a session (nil-Graph) activation — immediate mode must construct")
	}
}

// endregion
