// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"errors"
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

// region Resolve freshness cascade (k.10)

// addressableResource is a test fixture for the addressing-aware Resolve cascade. It overrides Addressing,
// Etag, and Digest with caller-supplied values, and counts how many times Etag and Digest are called so the
// fast-path assertions can verify "not called."
type addressableResource struct {
	ResourceBase
	addressingMode AddressingMode
	etagValue      string
	digestHex      string
	etagCalls      int
	digestCalls    int
}

func (r *addressableResource) Addressing() AddressingMode { return r.addressingMode }

func (r *addressableResource) Etag() (string, error) {
	r.etagCalls++
	return r.etagValue, nil
}

func (r *addressableResource) Digest() (Digest, error) {
	r.digestCalls++
	if r.digestHex == "" {
		return Digest{}, nil
	}
	return ParseDigest("sha256:" + r.digestHex)
}

const (
	testDigestA = "0000000000000000000000000000000000000000000000000000000000000001"
	testDigestB = "0000000000000000000000000000000000000000000000000000000000000002"
)

func newAddressable(uri string, mode AddressingMode, etag, digestHex string) *addressableResource {
	return &addressableResource{
		ResourceBase:   ResourceBase{uri: uri},
		addressingMode: mode,
		etagValue:      etag,
		digestHex:      digestHex,
	}
}

func TestCatalog_Resolve_ContentAddressing_SkipsEtagAndDigest(t *testing.T) {

	c := NewResourceCatalog()
	first := newAddressable("tag:..:sha256:abc#mem", AddressingContent, "any", testDigestA)
	c.Resolve(first) // populate

	probe := newAddressable("tag:..:sha256:abc#mem", AddressingContent, "different", testDigestB)
	got, _ := c.Resolve(probe)

	if got != Resource(first) {
		t.Errorf("Resolve: returned %p, want canonical %p", got, first)
	}
	if probe.etagCalls != 0 {
		t.Errorf("Etag called %d times, want 0 on content-addressed fast path", probe.etagCalls)
	}
	if probe.digestCalls != 0 {
		t.Errorf("Digest called %d times, want 0 on content-addressed fast path", probe.digestCalls)
	}
}

func TestCatalog_Resolve_LocationAddressing_EtagMatch_SkipsDigest(t *testing.T) {

	c := NewResourceCatalog()
	first := newAddressable("file:///etc/foo", AddressingLocation, "etag-1", testDigestA)
	c.Resolve(first)

	probe := newAddressable("file:///etc/foo", AddressingLocation, "etag-1", testDigestB)
	got, _ := c.Resolve(probe)

	if got != Resource(first) {
		t.Errorf("Resolve: returned %p, want canonical %p", got, first)
	}
	if probe.etagCalls == 0 {
		t.Errorf("Etag never called; expected exactly 1 call on cache hit")
	}
	if probe.digestCalls != 0 {
		t.Errorf("Digest called %d times, want 0 when Etag matches", probe.digestCalls)
	}
}

func TestCatalog_Resolve_LocationAddressing_EtagMismatch_TriggersDigest(t *testing.T) {

	c := NewResourceCatalog()
	first := newAddressable("file:///etc/foo", AddressingLocation, "etag-1", testDigestA)
	c.Resolve(first)

	probe := newAddressable("file:///etc/foo", AddressingLocation, "etag-2", testDigestA)
	got, _ := c.Resolve(probe)

	if got != Resource(first) {
		t.Errorf("Resolve: returned %p, want canonical %p", got, first)
	}
	if probe.etagCalls == 0 {
		t.Errorf("Etag never called; expected exactly 1 call")
	}
	if probe.digestCalls == 0 {
		t.Errorf("Digest never called; expected the cascade to compute Digest on Etag mismatch")
	}
}

func TestCatalog_Resolve_LocationAddressing_GenuineDrift_PreservesCanonical(t *testing.T) {

	c := NewResourceCatalog()
	first := newAddressable("file:///etc/foo", AddressingLocation, "etag-1", testDigestA)
	c.Resolve(first)

	// Probe disagrees on both Etag and Digest — genuine content drift.

	probe := newAddressable("file:///etc/foo", AddressingLocation, "etag-2", testDigestB)
	got, _ := c.Resolve(probe)

	// Per spec: Resolve preserves cached identity. The drift will surface in a future reconciliation pass.

	if got != Resource(first) {
		t.Errorf("Resolve: returned %p, want canonical %p (Resolve preserves cached identity on drift)", got, first)
	}

	if probe.digestCalls == 0 {
		t.Errorf("Digest never called; expected the cascade to verify before declaring drift")
	}
}

// endregion

// region Lifecycle (k.13)

// lifecycleResource is a Resource fixture that lets tests control Addressing() and Resolve() return.
type lifecycleResource struct {
	ResourceBase
	addressingMode AddressingMode
	resolveErr     error
	resolveCalls   int
}

func (r *lifecycleResource) Addressing() AddressingMode { return r.addressingMode }

func (r *lifecycleResource) Resolve() error {
	r.resolveCalls++
	return r.resolveErr
}

func newLifecycle(uri string, mode AddressingMode, resolveErr error) *lifecycleResource {
	return &lifecycleResource{
		ResourceBase:   ResourceBase{uri: uri},
		addressingMode: mode,
		resolveErr:     resolveErr,
	}
}

func TestState_ZeroValueIsPending(t *testing.T) {
	var s State
	if s != Pending {
		t.Errorf("zero value State = %v, want Pending", s)
	}
}

func TestResourceBase_StateBornPending(t *testing.T) {

	r := &lifecycleResource{ResourceBase: ResourceBase{uri: "x"}}

	if got := r.State(); got != Pending {
		t.Errorf("State() = %v, want Pending", got)
	}
}

func TestCatalog_markActive_TransitionsToActive(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///x", AddressingLocation, nil)
	c.markActive(r)

	if got := r.State(); got != Active {
		t.Errorf("State() = %v, want Active", got)
	}
}

func TestCatalog_markGone_TransitionsToGone(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///x", AddressingLocation, nil)
	c.markGone(r)

	if got := r.State(); got != Gone {
		t.Errorf("State() = %v, want Gone", got)
	}
}

// --- Discover lifecycle ---

func TestCatalog_Discover_CacheMiss_ResolveOK_ReturnsActive(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///hit", AddressingLocation, nil)
	factory := func() (Resource, error) { return r, nil }

	got, err := c.Discover(r.URI(), factory)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if got.State() != Active {
		t.Errorf("State() = %v, want Active", got.State())
	}

	if r.resolveCalls != 1 {
		t.Errorf("resolveCalls = %d, want 1", r.resolveCalls)
	}
}

func TestCatalog_Discover_CacheMiss_ResolveFail_ReturnsErrorAndMarksGone(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///missing", AddressingLocation, errors.New("not found"))
	factory := func() (Resource, error) { return r, nil }

	_, err := c.Discover(r.URI(), factory)
	if err == nil {
		t.Fatal("expected error from Discover when Resolve fails")
	}

	// The entry should be in the catalog as Gone.

	id := c.Current(r.URI())

	if id == "" {
		t.Fatal("entry was not appended on Resolve failure")
	}

	got, _ := c.Lookup(id)

	if got.State() != Gone {
		t.Errorf("entry state = %v, want Gone", got.State())
	}
}

func TestCatalog_Discover_CacheHitPending_ResolveOK_TransitionsActive(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///pending", AddressingLocation, nil)

	// Pre-populate: Resolve fails the first time through, so it lands as Gone — wait, we want Pending. Instead, just
	// intern via Resolve (catalog method) which uses catalogLocked and doesn't call r.Resolve().

	c.Resolve(r) // Pure namespace intern; state stays Pending.

	if r.State() != Pending {
		t.Fatalf("setup: state = %v, want Pending", r.State())
	}

	// Now call Discover on the same URI; the cached entry is Pending; Resolve should be called.

	probe := newLifecycle("file:///pending", AddressingLocation, nil)
	factory := func() (Resource, error) { return probe, nil }

	got, err := c.Discover(r.URI(), factory)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if got != Resource(r) {
		t.Error("Discover did not return the cached canonical")
	}

	if got.State() != Active {
		t.Errorf("State() = %v, want Active", got.State())
	}

	// Resolve must have been called on the cached canonical, not on the probe.

	if r.resolveCalls != 1 {
		t.Errorf("canonical resolveCalls = %d, want 1", r.resolveCalls)
	}

	if probe.resolveCalls != 0 {
		t.Errorf("probe resolveCalls = %d, want 0 (probe is discarded)", probe.resolveCalls)
	}
}

func TestCatalog_Discover_CacheHitActive_SkipsResolve(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///active", AddressingLocation, nil)
	c.Resolve(r)
	c.markActive(r)
	r.resolveCalls = 0 // reset

	probe := newLifecycle("file:///active", AddressingLocation, nil)
	factory := func() (Resource, error) { return probe, nil }

	got, err := c.Discover(r.URI(), factory)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if got != Resource(r) {
		t.Error("Discover did not return cached canonical")
	}

	if r.resolveCalls != 0 {
		t.Errorf("canonical resolveCalls = %d, want 0 on Active hit", r.resolveCalls)
	}
}

func TestCatalog_Discover_CacheHitGone_ReturnsErrorWithoutResolve(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///gone", AddressingLocation, nil)
	c.Resolve(r)
	c.markGone(r)
	r.resolveCalls = 0
	factory := func() (Resource, error) { return r, nil }

	_, err := c.Discover(r.URI(), factory)
	if err == nil {
		t.Fatal("expected error on Gone cache hit")
	}
	if r.resolveCalls != 0 {
		t.Errorf("resolveCalls = %d, want 0 on Gone hit (no Resolve)", r.resolveCalls)
	}
}

// --- GetOrCreate lifecycle ---

func TestCatalog_GetOrCreate_CacheMiss_StampsActiveAndProducer(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///out", AddressingLocation, nil)
	activation := &ActivationRecord{SiteID: "node-A"}
	factory := func() (Resource, error) { return r, nil }

	got, err := c.GetOrCreate(activation, r.URI(), factory)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	if got.State() != Active {
		t.Errorf("State() = %v, want Active", got.State())
	}

	if got.resourceBase().producerID != "node-A" {
		t.Errorf("ProducerID() = %q, want %q", got.resourceBase().producerID, "node-A")
	}
}

func TestCatalog_GetOrCreate_CASHit_ReturnsExisting(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("tag:..:sha256:abc#mem", AddressingContent, nil)
	first, _ := c.GetOrCreate(&ActivationRecord{SiteID: "node-A"}, r.URI(), func() (Resource, error) { return r, nil })

	// Second producer with same URI (same content by CAS guarantee).

	probe := newLifecycle("tag:..:sha256:abc#mem", AddressingContent, nil)

	got, err := c.GetOrCreate(&ActivationRecord{SiteID: "node-B"}, probe.URI(), func() (Resource, error) { return probe, nil })
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	if got != first {
		t.Error("CAS singleton not returned; expected first entry")
	}

	if got.resourceBase().producerID != "node-A" {
		t.Errorf("ProducerID() = %q, want %q (first-writer-wins for CAS)", got.resourceBase().producerID, "node-A")
	}
}

func TestCatalog_GetOrCreate_LocationHit_Shadows(t *testing.T) {

	c := NewResourceCatalog()
	first := newLifecycle("file:///out", AddressingLocation, nil)
	_, _ = c.GetOrCreate(&ActivationRecord{SiteID: "node-A"}, first.URI(), func() (Resource, error) { return first, nil })

	// Same URI, different producer. Should shadow.

	second := newLifecycle("file:///out", AddressingLocation, nil)

	got, err := c.GetOrCreate(&ActivationRecord{SiteID: "node-A"}, second.URI(), func() (Resource, error) { return second, nil })
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	if got != Resource(second) {
		t.Error("location-based hit did not shadow; expected second entry to be canonical")
	}

	if got.State() != Active {
		t.Errorf("new entry state = %v, want Active", got.State())
	}
}

func TestCatalog_GetOrCreate_GoneHit_RevivesByShadow(t *testing.T) {

	c := NewResourceCatalog()
	first := newLifecycle("tag:..:sha256:abc#mem", AddressingContent, nil)
	_, _ = c.GetOrCreate(&ActivationRecord{SiteID: "node-A"}, first.URI(), func() (Resource, error) { return first, nil })
	c.markGone(first)

	// Same URI, Gone state. Should shadow (revive).

	revival := newLifecycle("tag:..:sha256:abc#mem", AddressingContent, nil)

	got, err := c.GetOrCreate(&ActivationRecord{SiteID: "node-A"}, revival.URI(), func() (Resource, error) { return revival, nil })
	if err != nil {
		t.Fatalf("GetOrCreate (Gone revive): %v", err)
	}

	if got != Resource(revival) {
		t.Error("Gone hit did not revive via shadow; expected new entry to be canonical")
	}

	if got.State() != Active {
		t.Errorf("revived entry state = %v, want Active", got.State())
	}

	// Old entry stays Gone in history.

	if first.State() != Gone {
		t.Errorf("old entry state = %v, want Gone (terminal)", first.State())
	}
}

// endregion
