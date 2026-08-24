// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

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

// newFake constructs a [*fakeResource] with `uri`, `size`, and `checksum` populated.
//
// Parameters:
//   - `uri`: the resource URI to seed [ResourceBase] with.
//   - `size`: the resource size in bytes.
//   - `checksum`: the resource checksum string.
//
// Returns:
//   - *fakeResource: the constructed fixture.
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

	c.Shadow(shadowed, "node-A")

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

// region Link

func TestCatalog_Link_ReturnsCanonicalEntry(t *testing.T) {

	c := NewResourceCatalog()
	first := newFake("file:///etc/foo", 100, "abc")

	// First sighting: Link catalogs the input as a discovery entry and returns it.
	if got := c.Link(first); got != Resource(first) {
		t.Fatalf("Link on new URI: want the passed-in resource back, got a different object")
	}

	// Second sighting of the same URI: the input is discarded in favor of the canonical entry — the convenience
	// matches [ResourceCatalog.Resolve] with the catalog ID dropped.
	second := newFake("file:///etc/foo", 200, "xyz")
	if got := c.Link(second); got != Resource(first) {
		t.Fatalf("Link on known URI: want the canonical first entry, got a different object")
	}
}

// endregion

// region Shadow

func TestCatalog_Shadow_StampsProducerAndID(t *testing.T) {

	c := NewResourceCatalog()
	r := newFake("file:///etc/foo", 0, "")

	id := c.Shadow(r, "node-A")

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

// TestCatalog_Shadow_EmptyProducerAcceptedAsDiscovery confirms that Shadow accepts an empty producerID:
// the resource is appended as a discovery entry without claim, and is therefore eligible to be silently
// superseded by a future non-empty Shadow on the same URI.
func TestCatalog_Shadow_EmptyProducerAcceptedAsDiscovery(t *testing.T) {

	c := NewResourceCatalog()
	r := newFake("file:///etc/foo", 0, "")

	id := c.Shadow(r, "")
	if id == "" {
		t.Fatalf("Shadow with empty producer: want non-empty catalog id, got %q", id)
	}
	if got := r.ProducerID(); got != "" {
		t.Fatalf("ProducerID after empty Shadow: want %q, got %q", "", got)
	}
}

// TestCatalog_Shadow_EmptyProducerDefersToExistingClaim confirms the non-claiming-vs-claim rule: when a
// non-empty producer has already shadowed a URI, a subsequent empty-producer Shadow returns the existing
// catalog id without changing the namespace or appending a new ledger entry.
func TestCatalog_Shadow_EmptyProducerDefersToExistingClaim(t *testing.T) {

	c := NewResourceCatalog()

	claimedID := c.Shadow(newFake("file:///etc/foo", 0, ""), "node-A")

	deferredID := c.Shadow(newFake("file:///etc/foo", 0, ""), "")
	if deferredID != claimedID {
		t.Fatalf("empty-producer Shadow: want catalog id %q (defer), got %q", claimedID, deferredID)
	}
}

// TestCatalog_Shadow_DifferentProducersAppendGenerations pins run-time versioning (§4, revised
// 2026-08-20): two producers writing one URI are generations in the ledger — a fresh entry per producer,
// the namespace repointed to the newest, the prior generation surviving as history. The former
// write-write conflict error was the superseded plan-time output model's residue.
func TestCatalog_Shadow_DifferentProducersAppendGenerations(t *testing.T) {

	c := NewResourceCatalog()

	firstID := c.Shadow(newFake("file:///etc/foo", 0, ""), "node-A")
	secondID := c.Shadow(newFake("file:///etc/foo", 0, ""), "node-B")

	if secondID == firstID {
		t.Fatalf("second producer's Shadow: want a fresh generation, got the first id %q", firstID)
	}
	if got := c.Current("file:///etc/foo"); got != secondID {
		t.Fatalf("namespace after second Shadow: want %q (the newest generation), got %q", secondID, got)
	}
	if first, ok := c.Lookup(firstID); !ok || first.ProducerID() != "node-A" {
		t.Fatalf("first generation must survive as history with its producer stamp; ok=%v", ok)
	}
	if c.Len() != 2 {
		t.Fatalf("ledger length = %d, want 2 (both generations)", c.Len())
	}
}

func TestCatalog_Shadow_SameProducerAppendsGeneration(t *testing.T) {

	c := NewResourceCatalog()

	firstID := c.Shadow(newFake("file:///etc/foo", 0, ""), "node-A")
	secondID := c.Shadow(newFake("file:///etc/foo", 0, ""), "node-A")

	if secondID == firstID {
		t.Fatalf("same-producer re-production: want a fresh generation, got the first id %q", firstID)
	}
	if c.Len() != 2 {
		t.Fatalf("ledger length = %d, want 2 (idempotent re-claim appends)", c.Len())
	}
}

// TestCatalog_Shadow_ProducerlessOverGoneAppends pins the deference's Gone guard: a producerless act
// never adopts a Gone generation — Gone is terminal, so revival always appends a fresh generation and
// repoints the namespace (§3's production matrix).
func TestCatalog_Shadow_ProducerlessOverGoneAppends(t *testing.T) {

	c := NewResourceCatalog()

	first := newFake("file:///etc/foo", 0, "")
	firstID := c.Shadow(first, "node-A")
	c.markActive(first)
	c.markGone(first)

	revivedID := c.Shadow(newFake("file:///etc/foo", 0, ""), "")

	if revivedID == firstID {
		t.Fatalf("producerless Shadow over a Gone entry adopted it; want a fresh revival generation")
	}
	if got := c.Current("file:///etc/foo"); got != revivedID {
		t.Fatalf("namespace after revival = %q, want %q", got, revivedID)
	}
	if c.State(firstID) != Gone {
		t.Fatalf("the Gone generation's state = %v, want Gone (history is never rewritten)", c.State(firstID))
	}
}

func TestCatalog_Shadow_SupersedesDiscovery(t *testing.T) {

	c := NewResourceCatalog()
	_, discoveryID := c.Resolve(newFake("file:///etc/foo", 0, ""))

	shadowID := c.Shadow(newFake("file:///etc/foo", 0, ""), "node-A")

	if shadowID == discoveryID {
		t.Fatalf("Shadow over discovery: want new id, got same id %q", shadowID)
	}

	if c.Current("file:///etc/foo") != shadowID {
		t.Fatalf("Shadow over discovery: want namespace → %q, got %q", shadowID, c.Current("file:///etc/foo"))
	}
}

// endregion

// region Lookup / Current / Len

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

// TestCatalog_Clone_NilReceiverReturnsNil documents the nil-safe behavior callers rely on when
// chaining Clone over an optional [*Graph.ResourceCatalog] reference.
func TestCatalog_Clone_NilReceiverReturnsNil(t *testing.T) {

	var c *ResourceCatalog
	if got := c.Clone(); got != nil {
		t.Fatalf("nil.Clone() = %v, want nil", got)
	}
}

// TestCatalog_Clone_CopiesLedgerAndNamespace verifies the snapshot includes the entries slice,
// the byID index, the namespace map, and the nextID counter — every piece of state another
// caller might observe through the catalog's public surface.
func TestCatalog_Clone_CopiesLedgerAndNamespace(t *testing.T) {

	src := NewResourceCatalog()
	src.Shadow(newFake("file:///a", 0, ""), "node-A")
	src.Shadow(newFake("file:///b", 0, ""), "node-B")

	clone := src.Clone()

	if got, want := clone.Len(), src.Len(); got != want {
		t.Fatalf("Clone().Len() = %d, want %d", got, want)
	}
	for _, uri := range []string{"file:///a", "file:///b"} {
		if got, want := clone.Current(uri), src.Current(uri); got != want {
			t.Errorf("Clone().Current(%q) = %q, want %q", uri, got, want)
		}
	}
}

// TestCatalog_Clone_StatesAreIndependent verifies the load-bearing immutability invariant: state
// transitions on the clone do not leak back to the source catalog. This is what makes Graph.ResourceCatalog
// safe to share as the "plan-time identity record" across multiple runs, each of which gets a fresh
// per-run state map via Clone().
func TestCatalog_Clone_StatesAreIndependent(t *testing.T) {

	src := NewResourceCatalog()
	r := newLifecycle("file:///shared", AddressingLocation)
	_, id := src.Resolve(r)

	clone := src.Clone()

	clone.markActive(r)

	if got := src.State(id); got != Pending {
		t.Errorf("src.State(%q) = %v after clone.markActive; want Pending (state must not leak)", id, got)
	}
	if got := clone.State(id); got != Active {
		t.Errorf("clone.State(%q) = %v, want Active", id, got)
	}
}

// TestCatalog_Clone_IsIndependent verifies that mutations to either catalog after Clone do not
// leak into the other — distinct entries / byID / ns / nextID storage is the load-bearing
// invariant for per-run cloning.
func TestCatalog_Clone_IsIndependent(t *testing.T) {

	src := NewResourceCatalog()
	src.Shadow(newFake("file:///shared", 0, ""), "src-producer")

	clone := src.Clone()

	src.Shadow(newFake("file:///src-only", 0, ""), "src-producer")
	clone.Shadow(newFake("file:///clone-only", 0, ""), "clone-producer")

	if clone.Current("file:///src-only") != "" {
		t.Error("clone should not see entries added to src after Clone")
	}
	if src.Current("file:///clone-only") != "" {
		t.Error("src should not see entries added to clone after Clone")
	}

	if src.Current("file:///shared") == "" {
		t.Error("src lost the shared entry after Clone — Clone should not mutate the source")
	}
	if clone.Current("file:///shared") == "" {
		t.Error("clone lost the shared entry — Clone should preserve pre-existing state")
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

// Addressing returns the caller-supplied [AddressingMode] for this fixture.
//
// Returns:
//   - AddressingMode: the configured mode.
func (r *addressableResource) Addressing() AddressingMode { return r.addressingMode }

// Etag returns the caller-supplied etag string and increments the call counter for fast-path assertions.
//
// Returns:
//   - `string`: the configured etag value.
//   - `error`: always nil for this fixture.
func (r *addressableResource) Etag() (string, error) {

	r.etagCalls++
	return r.etagValue, nil
}

// Digest returns a [Digest] parsed from the caller-supplied hex and increments the call counter for
// fast-path assertions.
//
// Returns:
//   - Digest: the parsed digest, or the zero value when no hex was configured.
//   - `error`: a parse error if the configured hex is malformed; nil otherwise.
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

// newAddressable constructs a [*addressableResource] with the supplied URI, addressing mode, etag,
// and digest hex.
//
// Parameters:
//   - `uri`: the resource URI to seed [ResourceBase] with.
//   - `mode`: the [AddressingMode] to report from [addressableResource.Addressing].
//   - `etag`: the etag string to return from [addressableResource.Etag].
//   - `digestHex`: the 64-char sha256 hex to parse from [addressableResource.Digest]; empty means
//     return the zero [Digest].
//
// Returns:
//   - *addressableResource: the constructed fixture.
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

// lifecycleResource is a Resource fixture that lets tests control the Addressing(), Resolve(), and Exists() returns.
type lifecycleResource struct {
	ResourceBase
	addressingMode AddressingMode
	resolveErr     error
	resolveCalls   int
	present        bool

	// mismatches is what MismatchesKind reports — the fixture's way of saying "something else is at the
	// path" rather than "nothing is".
	mismatches bool

	// existsCalls counts Exists probes through a shared cell: the activation binding probes a
	// kind-preserving COPY of the entry (the step-4 copy-on-bind ruling), so a plain int field would
	// count on the copy while the test's original read zero. Allocated at construction; a fixture built
	// without a cell fails loudly on first probe.
	existsCalls *int
}

// Addressing returns the caller-supplied [AddressingMode] for this fixture.
//
// Returns:
//   - AddressingMode: the configured mode.
func (r *lifecycleResource) Addressing() AddressingMode { return r.addressingMode }

// Resolve returns the caller-supplied resolve error and increments the call counter for assertions.
//
// Returns:
//   - `error`: the configured resolve error (may be nil).
func (r *lifecycleResource) Resolve() error {

	r.resolveCalls++
	return r.resolveErr
}

// Exists returns the caller-supplied presence and increments the call counter for assertions.
//
// Returns:
//   - `bool`: the configured presence.
//
// MismatchesKind reports the fixture's configured mismatch, satisfying [KindMismatcher].
func (r *lifecycleResource) MismatchesKind() bool { return r.mismatches }

func (r *lifecycleResource) Exists() bool {

	*r.existsCalls++
	return r.present
}

// newLifecycle constructs a [*lifecycleResource] fixture with the supplied URI, addressing mode,
// and resolve-error.
//
// Parameters:
//   - `uri`: the resource URI to seed [ResourceBase] with.
//   - `mode`: the [AddressingMode] to report from [lifecycleResource.Addressing].
//
// Returns:
//   - *lifecycleResource: the constructed fixture.
func newLifecycle(uri string, mode AddressingMode) *lifecycleResource {
	return &lifecycleResource{
		ResourceBase:   ResourceBase{uri: uri},
		addressingMode: mode,
		existsCalls:    new(int),
	}
}

func TestState_ZeroValueIsPending(t *testing.T) {
	var s ResourceState
	if s != Pending {
		t.Errorf("zero value State = %v, want Pending", s)
	}
}

func TestCatalog_FreshlyCatalogedEntryIsPending(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///x", AddressingLocation)

	_, id := c.Resolve(r)
	if got := c.State(id); got != Pending {
		t.Errorf("State(%q) = %v, want Pending (zero value for a freshly cataloged entry)", id, got)
	}
}

func TestCatalog_markActive_TransitionsToActive(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///x", AddressingLocation)
	c.markActive(r)

	if got := c.State(r.ID()); got != Active {
		t.Errorf("State() = %v, want Active", got)
	}
}

func TestCatalog_markGone_TransitionsToGone(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///x", AddressingLocation)
	c.markGone(r)

	if got := c.State(r.ID()); got != Gone {
		t.Errorf("State() = %v, want Gone", got)
	}
}

// TestMarkGone_RecordsDeletionFromAnyState pins the mutator-side transition (step 23, ruling 3): a cataloged entry
// moves to Gone from Pending (a delete needs no prior verification) and from Active alike, and re-marking is
// idempotent.
func TestMarkGone_RecordsDeletionFromAnyState(t *testing.T) {

	c := NewResourceCatalog()

	pending := newLifecycle("file:///pending", AddressingLocation)
	_, pendingID := c.Resolve(pending)
	if got := c.State(pendingID); got != Pending {
		t.Fatalf("precondition: State = %v, want Pending", got)
	}
	c.MarkGone(pending, "unit-9")
	if got := c.State(pendingID); got != Gone {
		t.Errorf("State after MarkGone from Pending = %v, want Gone", got)
	}

	active := newLifecycle("file:///active", AddressingLocation)
	_, activeID := c.Resolve(active)
	c.markActive(active)
	c.MarkGone(active, "unit-9")
	if got := c.State(activeID); got != Gone {
		t.Errorf("State after MarkGone from Active = %v, want Gone", got)
	}

	c.MarkGone(active, "unit-9") // idempotent re-mark
	if got := c.State(activeID); got != Gone {
		t.Errorf("State after re-MarkGone = %v, want Gone", got)
	}
}

// TestMarkGone_GoneIsTerminalForDiscovery pins the terminal contract: a discovery over a deleted entry's URI is
// refused with the known-gone error rather than reviving or re-introducing it.
func TestMarkGone_GoneIsTerminalForDiscovery(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///deleted", AddressingLocation)
	c.Resolve(r)
	c.MarkGone(r, "unit-9")

	_, err := c.Discover(r.URI(), func() (Resource, error) {
		return newLifecycle("file:///deleted", AddressingLocation), nil
	})
	if err == nil || !strings.Contains(err.Error(), "known-gone") {
		t.Errorf("Discover over a Gone entry = %v, want the known-gone refusal", err)
	}
}

// TestMarkGone_RevivalIsAProductionAct pins the shadow-path revival: after a deletion, GetOrCreate appends a fresh
// generation (new id, Active) while the terminated generation stays Gone in the ledger.
func TestMarkGone_RevivalIsAProductionAct(t *testing.T) {

	c := NewResourceCatalog()
	first := newLifecycle("file:///revived", AddressingLocation)
	_, firstID := c.Resolve(first)
	c.MarkGone(first, "unit-9")

	revived, err := c.GetOrCreate("", first.URI(), func() (Resource, error) {
		return newLifecycle("file:///revived", AddressingLocation), nil
	})
	if err != nil {
		t.Fatalf("GetOrCreate over a Gone entry: %v", err)
	}

	if revived.ID() == firstID {
		t.Error("revival reused the terminated generation's id; want a fresh generation")
	}
	if got := c.State(revived.ID()); got != Active {
		t.Errorf("revived generation State = %v, want Active", got)
	}
	if got := c.State(firstID); got != Gone {
		t.Errorf("terminated generation State = %v, want Gone (the ledger keeps history)", got)
	}
}

// TestMarkGone_UncatalogedIsAProgrammingError pins the precondition: recording a deletion for a resource that was
// never interned panics with the assertion error instead of silently minting state.
func TestMarkGone_UncatalogedIsAProgrammingError(t *testing.T) {

	c := NewResourceCatalog()
	defer func() {
		if recover() == nil {
			t.Error("MarkGone over an uncataloged resource did not panic")
		}
	}()

	c.MarkGone(newLifecycle("file:///never-interned", AddressingLocation), "unit-9")
}

func TestCatalog_VerifyExistence_PresentMarksActive(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///x", AddressingLocation)
	r.present = true
	_, id := c.Resolve(r)

	if err := c.VerifyExistence(r); err != nil {
		t.Fatalf("VerifyExistence() error = %v, want nil", err)
	}
	if got := c.State(id); got != Active {
		t.Errorf("State() = %v, want Active (an existing resource resolves Pending → Active)", got)
	}
}

func TestCatalog_VerifyExistence_MissingMarksGone(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///x", AddressingLocation)
	r.present = false
	_, id := c.Resolve(r)

	if err := c.VerifyExistence(r); err == nil {
		t.Fatal("VerifyExistence() = nil, want an error for a missing resource")
	}
	if got := c.State(id); got != Gone {
		t.Errorf("State() = %v, want Gone (a missing resource resolves Pending → Gone)", got)
	}
}

func TestCatalog_VerifyExistence_ActiveShortCircuits(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///x", AddressingLocation)
	r.present = true
	c.Resolve(r)

	if err := c.VerifyExistence(r); err != nil {
		t.Fatalf("first VerifyExistence() error = %v", err)
	}
	if err := c.VerifyExistence(r); err != nil {
		t.Fatalf("second VerifyExistence() error = %v", err)
	}
	if *r.existsCalls != 1 {
		t.Errorf("existsCalls = %d, want 1 (an Active entry is not re-checked)", *r.existsCalls)
	}
}

// --- Discover lifecycle ---

// TestCatalog_Discover_CacheMiss_InternsAsPending confirms the post-19.4 contract: Discover
// constructs the candidate, interns it via [ResourceCatalog.Link], and returns it without driving
// any state transition. Pending → Active / Gone is now the framework's preflight responsibility
// (provider Observe + catalog state writes), not the catalog's own job.
func TestCatalog_Discover_CacheMiss_InternsAsPending(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///hit", AddressingLocation)
	factory := func() (Resource, error) { return r, nil }

	got, err := c.Discover(r.URI(), factory)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if c.State(got.ID()) != Pending {
		t.Errorf("State() = %v, want Pending (Discover no longer drives Active)", c.State(got.ID()))
	}
}

// TestCatalog_Discover_CacheHitActive_ReturnsExisting confirms cache-hit fast path.
func TestCatalog_Discover_CacheHitActive_ReturnsExisting(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///active", AddressingLocation)
	c.Resolve(r)
	c.markActive(r)

	probe := newLifecycle("file:///active", AddressingLocation)
	factory := func() (Resource, error) { return probe, nil }

	got, err := c.Discover(r.URI(), factory)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if got != Resource(r) {
		t.Error("Discover did not return cached canonical")
	}
}

// TestCatalog_Discover_CacheHitGone_ReturnsError confirms Gone is terminal at the cache-hit branch.
func TestCatalog_Discover_CacheHitGone_ReturnsError(t *testing.T) {

	c := NewResourceCatalog()
	r := newLifecycle("file:///gone", AddressingLocation)
	c.Resolve(r)
	c.markGone(r)
	factory := func() (Resource, error) { return r, nil }

	_, err := c.Discover(r.URI(), factory)
	if err == nil {
		t.Fatal("expected error on Gone cache hit")
	}
}

// --- GetOrCreate lifecycle ---

func TestCatalog_Shadow_StampsActiveAndProducer(t *testing.T) {

	// Producer-stamping is a property of [ResourceCatalog.Shadow], which takes the producerID
	// directly. [ResourceCatalog.GetOrCreate] delegates to Shadow on cache miss, passing
	// `activation.CallerID.ID()`; testing Shadow directly covers the producer-stamping behavior
	// without needing to construct a Unit-bearing activation.

	c := NewResourceCatalog()
	r := newLifecycle("file:///out", AddressingLocation)

	c.Shadow(r, "node-A")
	c.markActive(r)

	if c.State(r.ID()) != Active {
		t.Errorf("State() = %v, want Active", c.State(r.ID()))
	}
	if r.resourceBase().producerID != "node-A" {
		t.Errorf("ProducerID() = %q, want %q", r.resourceBase().producerID, "node-A")
	}
}

func TestCatalog_GetOrCreate_CASHit_ReturnsExisting(t *testing.T) {

	// CAS-hit "return existing, preserve first writer's producer" is independent of the second
	// caller's activation — we set up the first entry via Shadow with the producer of record,
	// then call GetOrCreate with an empty activation to confirm the existing entry is returned
	// unchanged.

	c := NewResourceCatalog()
	first := newLifecycle("tag:..:sha256:abc#mem", AddressingContent)
	c.Shadow(first, "node-A")
	c.markActive(first)

	probe := newLifecycle("tag:..:sha256:abc#mem", AddressingContent)
	got, err := c.GetOrCreate("", probe.URI(), func() (Resource, error) { return probe, nil })
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
	first := newLifecycle("file:///out", AddressingLocation)
	c.Shadow(first, "node-A")
	c.markActive(first)

	// Same URI, second producer: production at an occupied location shadows (§3's production matrix) —
	// a fresh generation is canonical, the first survives as history.
	second := newLifecycle("file:///out", AddressingLocation)

	got, err := c.GetOrCreate("node-B", second.URI(), func() (Resource, error) { return second, nil })
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	if got != Resource(second) {
		t.Error("location-based hit did not shadow; expected second entry to be canonical")
	}
	if c.State(got.ID()) != Active {
		t.Errorf("new entry state = %v, want Active", c.State(got.ID()))
	}
	if c.Len() != 2 {
		t.Errorf("ledger length = %d, want 2 (the first generation survives as history)", c.Len())
	}
}

// TestCatalog_GetOrCreate_ProducerlessHitAdoptsCanonical pins the deference's fixed identity: a
// producerless GetOrCreate over a produced, non-Gone entry returns the EXISTING canonical — never the
// raw candidate (pre-fix, the un-interned candidate leaked out and Active was stamped under an empty id).
func TestCatalog_GetOrCreate_ProducerlessHitAdoptsCanonical(t *testing.T) {

	c := NewResourceCatalog()
	first := newLifecycle("file:///out", AddressingLocation)
	c.Shadow(first, "node-A")
	c.markActive(first)

	candidate := newLifecycle("file:///out", AddressingLocation)

	got, err := c.GetOrCreate("", candidate.URI(), func() (Resource, error) { return candidate, nil })
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	if got != Resource(first) {
		t.Error("producerless hit must adopt the existing canonical, not hand out the raw candidate")
	}
	if c.Len() != 1 {
		t.Errorf("ledger length = %d, want 1 (adoption appends nothing)", c.Len())
	}
}

func TestCatalog_GetOrCreate_GoneHit_RevivesByShadow(t *testing.T) {

	c := NewResourceCatalog()
	first := newLifecycle("tag:..:sha256:abc#mem", AddressingContent)
	c.Shadow(first, "node-A")
	c.markActive(first)
	c.markGone(first)

	// Same URI, Gone state. Should shadow (revive).
	revival := newLifecycle("tag:..:sha256:abc#mem", AddressingContent)

	got, err := c.GetOrCreate("", revival.URI(), func() (Resource, error) { return revival, nil })
	if err != nil {
		t.Fatalf("GetOrCreate (Gone revive): %v", err)
	}

	if got != Resource(revival) {
		t.Error("Gone hit did not revive via shadow; expected new entry to be canonical")
	}
	if c.State(got.ID()) != Active {
		t.Errorf("revived entry state = %v, want Active", c.State(got.ID()))
	}

	// Old entry stays Gone in history.
	if c.State(first.ID()) != Gone {
		t.Errorf("old entry state = %v, want Gone (terminal)", c.State(first.ID()))
	}
}

// endregion
