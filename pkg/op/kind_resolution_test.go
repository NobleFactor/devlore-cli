// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"errors"
	"strings"
	"testing"
)

// region TEST FIXTURES

// unassertedResource is a [KindResolver] fixture: it exists, and it resolves to `resolvesTo` — the shape
// `file.AnyKind` has, without the filesystem.
type unassertedResource struct {
	ResourceBase
	addressingMode AddressingMode
	present        bool
	resolvesTo     Resource
	resolveErr     error
	resolveCalls   *int
}

func (r *unassertedResource) Addressing() AddressingMode { return r.addressingMode }
func (r *unassertedResource) Exists() bool               { return r.present }
func (r *unassertedResource) Resolve() error             { return nil }

func (r *unassertedResource) ResolveKind() (Resource, error) {
	*r.resolveCalls++
	return r.resolvesTo, r.resolveErr
}

// resolvedResource is what an unasserted claim becomes: a distinct concrete type, freshly built, with no
// catalog id and no producer stamp of its own.
type resolvedResource struct {
	ResourceBase
	addressingMode AddressingMode
}

func (r *resolvedResource) Addressing() AddressingMode { return r.addressingMode }
func (r *resolvedResource) Exists() bool               { return true }
func (r *resolvedResource) Resolve() error             { return nil }

// endregion

// region TEST FUNCTIONS

// TestVerifyExistence_ActiveBranchResolvesTheKind pins the transition's resolving half
// (docs/plans/any-entry-claims.md, ruled 2026-08-23): an entry observed to exist becomes the kind the
// observation names, and identity crosses the swap intact.
//
// The identity assertions are the ones that would fail silently if the carry-forward were wrong: the
// resolved object arrives with no id, so without stamping, `byID` would point at a row whose occupant
// answers "" to ID(), and the state would be recorded under the empty id instead of the entry's.
func TestVerifyExistence_ActiveBranchResolvesTheKind(t *testing.T) {

	catalog := NewResourceCatalog()

	resolved := &resolvedResource{
		ResourceBase:   ResourceBase{uri: "test:///claimed#resolved"},
		addressingMode: AddressingLocation,
	}
	calls := 0
	unasserted := &unassertedResource{
		ResourceBase:   ResourceBase{uri: "test:///claimed#unasserted"},
		addressingMode: AddressingLocation,
		present:        true,
		resolvesTo:     resolved,
		resolveCalls:   &calls,
	}

	_, id := catalog.Resolve(unasserted)
	catalog.entries[catalog.byID[id]].resourceBase().producerID = "node-A"

	if err := catalog.VerifyExistence(unasserted); err != nil {
		t.Fatalf("VerifyExistence: %v", err)
	}

	entry, ok := catalog.Lookup(id)
	if !ok {
		t.Fatal("the entry vanished from the ledger")
	}
	if _, isResolved := entry.(*resolvedResource); !isResolved {
		t.Fatalf("ledger holds %T, want the resolved kind", entry)
	}
	if entry.ID() != id {
		t.Errorf("resolved entry ID = %q, want the entry's own %q — identity must cross the swap", entry.ID(), id)
	}
	if entry.ProducerID() != "node-A" {
		t.Errorf("resolved entry producer = %q, want node-A carried forward", entry.ProducerID())
	}
	if got := catalog.State(id); got != Active {
		t.Errorf("state = %v, want Active recorded under the entry's own id", got)
	}
	if catalog.Len() != 1 {
		t.Errorf("ledger length = %d, want 1 — resolution replaces, never appends", catalog.Len())
	}
	if got := catalog.Current("test:///claimed"); got != id {
		t.Errorf("namespace = %q, want %q — the fragment is not part of the key", got, id)
	}
}

// TestVerifyExistence_GoneBranchResolvesNothing pins the other direction: nothing was observed, so an
// unasserted claim has nothing to become and stays unasserted — the honest record of unmet intent.
func TestVerifyExistence_GoneBranchResolvesNothing(t *testing.T) {

	catalog := NewResourceCatalog()

	calls := 0
	unasserted := &unassertedResource{
		ResourceBase:   ResourceBase{uri: "test:///absent#unasserted"},
		addressingMode: AddressingLocation,
		present:        false,
		resolvesTo:     &resolvedResource{ResourceBase: ResourceBase{uri: "test:///absent#resolved"}},
		resolveCalls:   &calls,
	}

	_, id := catalog.Resolve(unasserted)

	err := catalog.VerifyExistence(unasserted)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("err = %v, want the catalog's does-not-exist verdict", err)
	}
	if calls != 0 {
		t.Errorf("ResolveKind called %d times on the Gone branch, want 0", calls)
	}

	entry, _ := catalog.Lookup(id)
	if _, stillUnasserted := entry.(*unassertedResource); !stillUnasserted {
		t.Errorf("ledger holds %T after a Gone transition, want the unasserted entry unchanged", entry)
	}
	if got := catalog.State(id); got != Gone {
		t.Errorf("state = %v, want Gone", got)
	}
}

// TestVerifyExistence_ResolutionHappensExactlyOnce pins idempotence, which is free from the
// already-Active early return: a second verification of the same entry neither re-resolves nor re-swaps.
func TestVerifyExistence_ResolutionHappensExactlyOnce(t *testing.T) {

	catalog := NewResourceCatalog()

	calls := 0
	unasserted := &unassertedResource{
		ResourceBase:   ResourceBase{uri: "test:///once#unasserted"},
		addressingMode: AddressingLocation,
		present:        true,
		resolvesTo: &resolvedResource{
			ResourceBase:   ResourceBase{uri: "test:///once#resolved"},
			addressingMode: AddressingLocation,
		},
		resolveCalls: &calls,
	}

	catalog.Resolve(unasserted)

	for range 3 {
		if err := catalog.VerifyExistence(unasserted); err != nil {
			t.Fatalf("VerifyExistence: %v", err)
		}
	}

	if calls != 1 {
		t.Errorf("ResolveKind called %d times, want exactly 1 — the transition happens once", calls)
	}
}

// TestVerifyExistence_AResolutionFailureIsNotAnExistenceFailure pins the tolerance: the entry has
// already been observed to exist, so a resolver that errors leaves it Active and unasserted rather than
// converting a kind problem into a missing-resource verdict.
func TestVerifyExistence_AResolutionFailureIsNotAnExistenceFailure(t *testing.T) {

	catalog := NewResourceCatalog()

	calls := 0
	unasserted := &unassertedResource{
		ResourceBase:   ResourceBase{uri: "test:///unresolvable#unasserted"},
		addressingMode: AddressingLocation,
		present:        true,
		resolveErr:     errUnresolvableKind,
		resolveCalls:   &calls,
	}

	_, id := catalog.Resolve(unasserted)

	if err := catalog.VerifyExistence(unasserted); err != nil {
		t.Fatalf("VerifyExistence = %v, want nil — existence was observed", err)
	}
	if got := catalog.State(id); got != Active {
		t.Errorf("state = %v, want Active", got)
	}

	entry, _ := catalog.Lookup(id)
	if _, stillUnasserted := entry.(*unassertedResource); !stillUnasserted {
		t.Errorf("ledger holds %T, want the entry left as it was", entry)
	}
}

// TestVerifyExistence_ANonResolverIsUntouched pins that every scheme without a kind axis — which is
// every scheme but file — transitions exactly as it did before the seam existed.
func TestVerifyExistence_ANonResolverIsUntouched(t *testing.T) {

	catalog := NewResourceCatalog()

	plain := newLifecycle("test:///plain", AddressingLocation)
	plain.present = true

	_, id := catalog.Resolve(plain)

	if err := catalog.VerifyExistence(plain); err != nil {
		t.Fatalf("VerifyExistence: %v", err)
	}

	entry, _ := catalog.Lookup(id)
	if entry != Resource(plain) {
		t.Errorf("ledger holds %p, want the original %p — a non-resolver is never swapped", entry, plain)
	}
	if got := catalog.State(id); got != Active {
		t.Errorf("state = %v, want Active", got)
	}
	if _, isResolver := Resource(plain).(KindResolver); isResolver {
		t.Error("the fixture unexpectedly implements KindResolver; the assertion above proves nothing")
	}
}

// endregion

// errUnresolvableKind stands in for a kind the taxonomy cannot name — the failure a real resolver
// reports when the disk holds something no variant covers.
var errUnresolvableKind = errors.New("unresolvable kind")
