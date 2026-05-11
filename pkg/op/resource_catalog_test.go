// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"strings"
	"testing"
)

// fakeResource is a minimal Resource for catalog tests. It embeds [ResourceBase] for identity and adds two
// mutable metadata fields (Size, Checksum) to exercise the pending → resolved transition.
type fakeResource struct {
	ResourceBase
	Size     int64
	Checksum string
}

func newFake(uri string, size int64, checksum string) *fakeResource {
	return &fakeResource{
		ResourceBase: ResourceBase{uri: uri},
		Size:         size,
		Checksum:     checksum,
	}
}

// region Resolve

func TestCatalog_Resolve_NewURIDiscoveryEntry(t *testing.T) {

	c := NewResourceCatalog()
	r := newFake("file:///etc/foo", 0, "")

	got, id := c.Resolve(r)

	if got != Resource(r) {
		t.Fatalf("Resolve on new URI: want passed-in resource, got %p vs %p", got, r)
	}
	if id == "" {
		t.Fatalf("Resolve on new URI: want non-empty id")
	}
	if r.ProducerID() != "" {
		t.Fatalf("Resolve on new URI: want empty producerID (discovery), got %q", r.ProducerID())
	}
	if r.ID() != id {
		t.Fatalf("Resolve on new URI: want ID %q stamped on base, got %q", id, r.ID())
	}
}

func TestCatalog_Resolve_KnownURIReturnsCanonical(t *testing.T) {

	c := NewResourceCatalog()
	first := newFake("file:///etc/foo", 100, "abc")
	second := newFake("file:///etc/foo", 200, "xyz")

	_, firstID := c.Resolve(first)
	canonical, secondID := c.Resolve(second)

	if secondID != firstID {
		t.Fatalf("Resolve on known URI: want id %q, got %q", firstID, secondID)
	}
	if canonical != Resource(first) {
		t.Fatalf("Resolve on known URI: want canonical to be first entry, got different object")
	}
	// Second resource is discarded — its metadata must not leak into the canonical.
	if first.Size != 100 || first.Checksum != "abc" {
		t.Fatalf("Resolve must not mutate canonical: got Size=%d Checksum=%q", first.Size, first.Checksum)
	}
}

func TestCatalog_Resolve_ReturnsShadowedVersionAfterShadow(t *testing.T) {

	c := NewResourceCatalog()
	shadowed := newFake("file:///etc/foo", 0, "")
	if _, err := c.Shadow(shadowed, "node-A"); err != nil {
		t.Fatalf("Shadow: %v", err)
	}

	lookup := newFake("file:///etc/foo", 0, "")
	canonical, _ := c.Resolve(lookup)

	if canonical != Resource(shadowed) {
		t.Fatalf("Resolve after Shadow: want shadowed entry, got different")
	}
	if got := canonical.resourceBase().producerID; got != "node-A" {
		t.Fatalf("Resolve after Shadow: want producerID node-A, got %q", got)
	}
}

// endregion

// region Shadow

func TestCatalog_Shadow_StampsProducerAndID(t *testing.T) {

	c := NewResourceCatalog()
	r := newFake("file:///etc/foo", 0, "")

	id, err := c.Shadow(r, "node-A")
	if err != nil {
		t.Fatalf("Shadow: %v", err)
	}
	if id == "" {
		t.Fatalf("Shadow: want non-empty id")
	}
	if r.ID() != id {
		t.Fatalf("Shadow: want ID %q stamped, got %q", id, r.ID())
	}
	if r.ProducerID() != "node-A" {
		t.Fatalf("Shadow: want producerID node-A, got %q", r.ProducerID())
	}
}

func TestCatalog_Shadow_RejectsEmptyProducer(t *testing.T) {

	c := NewResourceCatalog()
	r := newFake("file:///etc/foo", 0, "")

	if _, err := c.Shadow(r, ""); err == nil {
		t.Fatalf("Shadow with empty producer: want error, got nil")
	}
}

func TestCatalog_Shadow_ConflictOnDifferentProducer(t *testing.T) {

	c := NewResourceCatalog()
	if _, err := c.Shadow(newFake("file:///etc/foo", 0, ""), "node-A"); err != nil {
		t.Fatalf("first Shadow: %v", err)
	}

	_, err := c.Shadow(newFake("file:///etc/foo", 0, ""), "node-B")
	if err == nil {
		t.Fatalf("second Shadow with different producer: want error, got nil")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("second Shadow: want error mentioning conflict, got %q", err.Error())
	}
}

func TestCatalog_Shadow_SameProducerAllowed(t *testing.T) {

	c := NewResourceCatalog()
	if _, err := c.Shadow(newFake("file:///etc/foo", 0, ""), "node-A"); err != nil {
		t.Fatalf("first Shadow: %v", err)
	}
	if _, err := c.Shadow(newFake("file:///etc/foo", 0, ""), "node-A"); err != nil {
		t.Fatalf("second Shadow with same producer: %v", err)
	}
}

func TestCatalog_Shadow_SupersedesDiscovery(t *testing.T) {

	c := NewResourceCatalog()
	_, discoveryID := c.Resolve(newFake("file:///etc/foo", 0, ""))
	shadowID, err := c.Shadow(newFake("file:///etc/foo", 0, ""), "node-A")
	if err != nil {
		t.Fatalf("Shadow over discovery: %v", err)
	}
	if shadowID == discoveryID {
		t.Fatalf("Shadow over discovery: want new id, got same id %q", shadowID)
	}
	if c.Current("file:///etc/foo") != shadowID {
		t.Fatalf("Shadow over discovery: want namespace → %q, got %q", shadowID, c.Current("file:///etc/foo"))
	}
}

// endregion

// region Lookup / Current / Len / DiscoveryURIs

func TestCatalog_LookupAndCurrent(t *testing.T) {

	c := NewResourceCatalog()
	r := newFake("file:///etc/foo", 0, "")
	_, id := c.Resolve(r)

	got, ok := c.Lookup(id)
	if !ok || got != Resource(r) {
		t.Fatalf("Lookup(%q): ok=%v got=%p want=%p", id, ok, got, r)
	}
	if c.Current("file:///etc/foo") != id {
		t.Fatalf("Current: want %q, got %q", id, c.Current("file:///etc/foo"))
	}
	if c.Current("file:///etc/none") != "" {
		t.Fatalf("Current on unknown URI: want empty, got %q", c.Current("file:///etc/none"))
	}
	if _, ok := c.Lookup("bogus"); ok {
		t.Fatalf("Lookup on unknown id: want false")
	}
}

func TestCatalog_Len(t *testing.T) {

	c := NewResourceCatalog()
	if c.Len() != 0 {
		t.Fatalf("new catalog: want len 0, got %d", c.Len())
	}
	c.Resolve(newFake("file:///a", 0, ""))
	c.Resolve(newFake("file:///b", 0, ""))
	if c.Len() != 2 {
		t.Fatalf("after 2 Resolves: want len 2, got %d", c.Len())
	}
}

func TestCatalog_DiscoveryURIs(t *testing.T) {

	c := NewResourceCatalog()
	c.Resolve(newFake("file:///discovered", 0, ""))
	if _, err := c.Shadow(newFake("file:///produced", 0, ""), "node-A"); err != nil {
		t.Fatalf("Shadow: %v", err)
	}
	if _, err := c.Shadow(newFake("file:///discovered-then-shadowed", 0, ""), "node-B"); err != nil {
		t.Fatalf("Shadow: %v", err)
	}
	// This one starts as a discovery and then gets shadowed: the shadow supersedes.
	c.Resolve(newFake("file:///discovered-then-shadowed", 0, ""))

	uris := c.DiscoveryURIs()
	if len(uris) != 1 || uris[0] != "file:///discovered" {
		t.Fatalf("DiscoveryURIs: want [file:///discovered], got %v", uris)
	}
}

// endregion

// region ExtractResource

func TestExtractResource_PointerResourceWithProducer(t *testing.T) {

	c := NewResourceCatalog()
	r := newFake("file:///etc/foo", 0, "")
	_, _ = c.Shadow(r, "node-A")

	origin, ok := ExtractResource(r)
	if !ok || origin != "node-A" {
		t.Fatalf("ExtractResource: ok=%v origin=%q, want true/node-A", ok, origin)
	}
}

func TestExtractResource_PointerResourceWithoutOrigin(t *testing.T) {

	r := newFake("file:///etc/foo", 0, "")
	origin, ok := ExtractResource(r)
	if ok || origin != "" {
		t.Fatalf("ExtractResource: ok=%v origin=%q, want false/empty", ok, origin)
	}
}

func TestExtractResource_NilAndNonResource(t *testing.T) {

	cases := []any{nil, "string", 42, []int{1, 2}}
	for _, v := range cases {
		if _, ok := ExtractResource(v); ok {
			t.Fatalf("ExtractResource(%T): want false", v)
		}
	}
}

func TestExtractResource_MapWithProducerID(t *testing.T) {

	m := map[string]any{"producer_id": "node-X"}
	producer, ok := ExtractResource(m)
	if !ok || producer != "node-X" {
		t.Fatalf("ExtractResource(map): ok=%v producer=%q, want true/node-X", ok, producer)
	}
}

func TestExtractResource_MapWithNestedResourceBase(t *testing.T) {

	m := map[string]any{"resource_base": map[string]any{"producer_id": "node-Y"}}
	producer, ok := ExtractResource(m)
	if !ok || producer != "node-Y" {
		t.Fatalf("ExtractResource(nested): ok=%v producer=%q, want true/node-Y", ok, producer)
	}
}

// endregion
