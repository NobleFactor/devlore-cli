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

// TestResolveDispatchResource_ResourceValueResolvesByID pins the captured-object half: a Resource slot value
// resolves by its catalog id to the run clone's canonical, never dispatching the captured object itself -- the
// aliasing between planning catalog and run clone is severed, not load-bearing -- and never by URI, which would
// name whichever generation is current (#735).
func TestResolveDispatchResource_ResourceValueResolvesByID(t *testing.T) {

	catalog := NewResourceCatalog()
	canonical := newLifecycle("test:///claimed", AddressingLocation)
	_, id := catalog.Resolve(canonical)
	captured := newLifecycle("test:///claimed", AddressingLocation) // the same identity, a different object
	captured.id = id
	activation := &ActivationRecord{Graph: &Graph{}, RuntimeEnvironment: &RuntimeEnvironment{ResourceCatalog: catalog}}

	resolved, applied, err := resolveDispatchResource(activation, captured, reflect.TypeFor[*lifecycleResource]())
	if !applied || err != nil {
		t.Fatalf("resolveDispatchResource(resource) = applied %t, err %v; want applied, nil", applied, err)
	}
	if resolved != Resource(canonical) {
		t.Errorf("resolved %p is not the canonical %p — the captured object must not dispatch", resolved, canonical)
	}

	uncataloged := newLifecycle("test:///claimed", AddressingLocation)
	if _, applied, err := resolveDispatchResource(activation, uncataloged, reflect.TypeFor[*lifecycleResource]()); !applied || err == nil {
		t.Errorf("an uncataloged resource (no id) must be refused, got applied %t, err %v", applied, err)
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

// TestResolveDispatchResource_ARecordedIDResolvesByID pins the resume half of #712 item 4: a resource a paused run's
// trace held in a variable comes back as a recorded id, and dispatch resolves it against the run catalog by that id --
// the same seam a receipt's recorded resource uses. A miss is the catalog's verdict.
func TestResolveDispatchResource_ARecordedIDResolvesByID(t *testing.T) {

	catalog := NewResourceCatalog()
	canonical := newLifecycle("test:///claimed", AddressingLocation)
	_, id := catalog.Resolve(canonical)
	activation := &ActivationRecord{Graph: &Graph{}, RuntimeEnvironment: &RuntimeEnvironment{ResourceCatalog: catalog}}

	resolved, applied, err := resolveDispatchResource(activation, recordedResourceID(id), reflect.TypeFor[*lifecycleResource]())
	if !applied || err != nil {
		t.Fatalf("resolveDispatchResource(recorded id) = applied %t, err %v; want applied, nil", applied, err)
	}
	if resolved != Resource(canonical) {
		t.Errorf("resolved %p is not the canonical %p", resolved, canonical)
	}
	_, applied, err = resolveDispatchResource(activation, recordedResourceID("res-404"), reflect.TypeFor[*lifecycleResource]())
	if !applied || err == nil || !strings.Contains(err.Error(), "not in the run catalog") {
		t.Errorf("a recorded id the run catalog lacks must be refused; got applied %t, err %v", applied, err)
	}
}

// TestResolveDispatchResource_AResourceBindsItsOwnGenerationAfterShadow pins #735's first two acceptance criteria at
// the dispatch seam: after a URI is re-produced -- a second generation shadows the first -- a slot written against
// either generation resolves to that generation by id, while a run-time string key resolves to whichever generation
// is current, which is what a key means (§5.6).
func TestResolveDispatchResource_AResourceBindsItsOwnGenerationAfterShadow(t *testing.T) {

	catalog := NewResourceCatalog()
	first := newLifecycle("test:///versioned", AddressingLocation)
	_, firstID := catalog.Resolve(first)
	second := newLifecycle("test:///versioned", AddressingLocation)
	secondID := catalog.Shadow(second, "writer")
	if firstID == secondID {
		t.Fatalf("Shadow minted no new generation: both ids are %s", firstID)
	}
	activation := &ActivationRecord{Graph: &Graph{}, RuntimeEnvironment: &RuntimeEnvironment{ResourceCatalog: catalog}}
	target := reflect.TypeFor[*lifecycleResource]()

	for _, testCase := range []struct {
		name string
		id   string
		want *lifecycleResource
	}{
		{"the first generation", firstID, first},
		{"the second generation", secondID, second},
	} {
		captured := newLifecycle("test:///versioned", AddressingLocation)
		captured.id = testCase.id
		resolved, applied, err := resolveDispatchResource(activation, captured, target)
		if !applied || err != nil {
			t.Fatalf("%s: applied %t, err %v; want applied, nil", testCase.name, applied, err)
		}
		if resolved != Resource(testCase.want) {
			t.Errorf("%s: a slot written against %s resolved to %p, not its own generation %p", testCase.name, testCase.id, resolved, testCase.want)
		}
	}
	resolved, applied, err := resolveDispatchResource(activation, "test:///versioned", target)
	if !applied || err != nil || resolved != Resource(second) {
		t.Errorf("a run-time key resolved to %v (applied %t, err %v); want the current generation %p", resolved, applied, err, second)
	}
}
