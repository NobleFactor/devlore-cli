// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"strings"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// ResourceCatalog is the graph-level owner of the append-only [Resource] ledger and the URI→ID addressing namespace.
//
// One catalog per [Graph]. Created at plan time by the planner, consumed at execution time by the executor's preflight
// pass and post-dispatch transition. See docs/architecture/4-resource-management.md §6.1-§6.5, §6.8.
//
// The catalog holds [Resource] interface values, which are pointers to concrete resource structs (e.g.,
// [*file.Resource]). Preflight and node execution populate metadata fields on those structs in place; all holders of
// the pointer see the updated fields. The ledger's append-only property refers to the sequence of distinct resources,
// not to the mutability of their metadata.
//
// Two entry classes, derived from an entry's producer:
//
//   - Discovery: producerID == "". The entry was registered without a production claim — by [ResourceCatalog.Discover],
//     by a discovery-style provider call, or by reference handles in CLI tools. The catalog tracks the URI but no
//     dispatch claims to have created it.
//   - Production: producerID != "". The entry was created by [ResourceCatalog.GetOrCreate] from a producer dispatch
//     context. The producerID is the dispatching [ExecutableUnit]'s ID (typically a graph node ID, occasionally a
//     subgraph ID) and is the answer to "who created this URI?" for downstream producer→consumer edge derivation.
//
// Orthogonal to that split, every entry carries a per-run lifecycle state ([ResourceState]: [Pending] → [Active] /
// [Gone]), owned by the catalog — read via [ResourceCatalog.State], driven by [ResourceCatalog.VerifyExistence] on the
// discovery side (the executor's pre-flight resolve pass) and by [GetOrCreate] on the production side.
type ResourceCatalog struct {
	mu      sync.Mutex
	entries []Resource               // append-only ledger
	byID    map[string]int           // id → index in entries
	ns      map[string]string        // namespace key → current id; see [namespaceKey] for the per-addressing keying regime
	states  map[string]ResourceState // id → per-run lifecycle state; independent of Resource identity
	nextID  int                      // monotonic counter for id generation
}

// NewResourceCatalog creates an empty catalog.
//
// Returns:
//   - `*ResourceCatalog`: the empty catalog.
func NewResourceCatalog() *ResourceCatalog {
	return &ResourceCatalog{
		byID:   make(map[string]int),
		ns:     make(map[string]string),
		states: make(map[string]ResourceState),
	}
}

// region EXPORTED METHODS

// region State management

// Clone returns a shallow copy of this catalog with a fresh mutex.
//
// The returned catalog has its own `entries`, `byID`, `ns`, and `nextID` — distinct from the receiver's — so subsequent
// appends, namespace updates, and producer-stamp changes on either catalog do not affect the other. The [Resource]
// values themselves are shared by pointer: each Resource's identity-bearing fields (URI, the `producerID` stamped by
// [GetOrCreate] / [Shadow]) are plan-time-fixed and effectively immutable, but mutable metadata fields populated by
// `Resource.Resolve` (size, mod-time, checksum, etc.) are not deep-copied. Concurrent runs that share Resource
// instances would race on those metadata writes — single-run cloning is the supported usage (the planning catalog
// handed off via [Graph.ResourceCatalog] and cloned into [RuntimeEnvironment.ResourceCatalog] at each
// [GraphExecutor.Run] invocation).
//
// Locks the receiver's mutex for the duration of the copy so the snapshot is internally consistent; the cloned catalog
// gets a fresh zero-value mutex.
//
// Returns:
//   - `*ResourceCatalog`: a new catalog with the receiver's ledger structure shallow-copied. Returns nil when the
//     receiver is nil so callers can chain Clone on optional catalogs without a nil-guard.
func (c *ResourceCatalog) Clone() *ResourceCatalog {

	if c == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entries := make([]Resource, len(c.entries))
	copy(entries, c.entries)

	byID := make(map[string]int, len(c.byID))
	for k, v := range c.byID {
		byID[k] = v
	}

	ns := make(map[string]string, len(c.ns))
	for k, v := range c.ns {
		ns[k] = v
	}

	states := make(map[string]ResourceState, len(c.states))
	for k, v := range c.states {
		states[k] = v
	}

	return &ResourceCatalog{
		entries: entries,
		byID:    byID,
		ns:      ns,
		states:  states,
		nextID:  c.nextID,
	}
}

// Current returns the catalog ID authoritative for the given URI, or the empty string if the URI is unknown.
//
// Two keying regimes coexist behind this lookup (see [namespaceKey]): location-addressed entries key on the
// fragment-stripped URI, content-addressed entries on the full URI. Callers pass whichever form they hold — the
// exact key is tried first, then the fragment-stripped form, so a full canonical tag URI finds a location entry
// keyed without its fragment.
//
// Parameters:
//   - `uri`: the URI to look up (full or fragment-stripped form).
//
// Returns:
//   - `string`: the current catalog ID for `uri`, or "" if not found.
func (c *ResourceCatalog) Current(uri string) string {

	c.mu.Lock()
	defer c.mu.Unlock()

	if id, ok := c.ns[uri]; ok {
		return id
	}

	return c.ns[stripFragment(uri)]
}

// Discover returns the canonical catalog entry for uri, introducing it as [Pending] when unseen.
//
// Discover is the consumption-side counterpart to [GetOrCreate]. Use it from non-production callsites: plan-time slot
// coercion (a string path becoming a typed resource), receipt rehydration during unmarshal, and any other path where
// there is no producing node. The returned entry has no producerID stamped (or carries whatever stamp a previous
// GetOrCreate already applied) — discovery records identity, not authorship.
//
// Discover is introduction-only — it never verifies existence (phase-8 step 22, Ruling 2): a plan-time resource is
// deliberately unresolved and is expected to resolve at runtime, when the executor's pre-flight resolve pass drives
// the [Pending] → [Active]/[Gone] transition through [ResourceCatalog.VerifyExistence]. Cache-hit behavior branches on
// the entry's [ResourceState]: [Active] and [Pending] return the existing entry as-is; [Gone] returns an error
// ([Gone] is terminal — reviving a URI is a production act, via [GetOrCreate]'s shadow path). Cache-miss constructs a
// fresh candidate via factory and links it as [Pending].
//
// Parameters:
//   - `uri`: the URI to look up. Must not be empty (asserted).
//   - `factory`: closure invoked on cache miss to construct a fresh [Resource]. Must be non-nil (asserted).
//
// Returns:
//   - `Resource`: the canonical catalog entry for `uri` ([Active] or [Pending]); nil when the entry is known-[Gone].
//   - `error`: any factory error (returned untouched), or a known-gone error when the URI's existing entry is [Gone].
//
// Panics with an [*assert.AssertionError] when any precondition is violated — these are programming errors at the call
// site, not runtime conditions.
func (c *ResourceCatalog) Discover(uri string, factory func() (Resource, error)) (Resource, error) {

	assert.True("uri not empty", uri != "")
	assert.True("factory required", factory != nil)

	// Cache hit: branch on state per the DiscoverResource rules (Rule 3 + Rule 4).
	if id := c.Current(uri); id != "" {
		if existing, ok := c.Lookup(id); ok {
			switch c.State(id) {
			case Active:
				return existing, nil
			case Gone:
				return nil, fmt.Errorf("discover %q: resource is known-gone", uri)
			case Pending:
				return existing, nil
			}
		}
	}

	// Cache miss: construct and intern as Pending. Discover never verifies existence — the executor's pre-flight
	// resolve pass drives the Pending → Active / Gone transition through VerifyExistence (phase-8 step 22).
	candidate, err := factory()
	if err != nil {
		return nil, err
	}

	return c.Link(candidate), nil
}

// GetOrCreate returns the canonical catalog entry for uri after recording the producer's claim.
//
// GetOrCreate is the production-claim hook. Forward-method outputs flow through it via each provider's
// `NewResource(env, unit, ...)` constructor. The catalog stays type-neutral; the factory closure resolves the
// concrete-type-to-construct decision at the call site, where the type is statically known. The producerID stamp on the
// resulting entry is `unit.ID()` when `unit` is non-nil; non-graph dispatches (the starlark immediate-mode bridge, test
// fixtures, CLI runners) pass a nil `unit` and the resulting entry carries an empty producer stamp — see the
// discovery-vs-production split documented on [ResourceCatalog].
//
// Cache-hit behavior branches on the existing entry's [Addressing] × [ResourceState] per
// docs/architecture/4-resource-management.md §6.2's behavior matrix. The factory is invoked on cache miss, on
// location-based hits (any state), and on Gone hits (either addressing — Gone is terminal, so revival appends a new
// ledger entry via [Shadow]). Content-addressable hits on Pending or Active return the existing entry without invoking
// the factory (singleton). The new or revived entry transitions to Active via [markActive] before returning.
//
// A non-nil factory error short-circuits without touching the catalog. A different producer claiming the same URI
// surfaces as a Shadow conflict (write-write detection).
//
// Parameters:
//   - `producerID`: the producing caller's id (`activation.CallerID` — a unit id or a starlark call-site), or ""
//     carries `unit.ID()` as its producer stamp; when nil the stamp is empty. Discovery call sites that need to query
//     existence without claiming production use [ResourceCatalog.Discover] instead.
//   - `uri`: the URI to look up. Must not be empty (asserted).
//   - `factory`: closure invoked on cache miss (or location/Gone shadow path) to construct a fresh [Resource]. Must be
//     non-nil (asserted).
//
// Returns:
//   - `Resource`: the canonical catalog entry for `uri`, in state Active.
//   - `error`: any factory error (returned untouched), or a [Shadow] conflict if a different producer already claimed
//     the same URI.
//
// Panics with an [*assert.AssertionError] when any precondition is violated — these are programming errors at the call
// site, not runtime conditions.
func (c *ResourceCatalog) GetOrCreate(producerID string, uri string, factory func() (Resource, error)) (Resource, error) {

	assert.True("uri not empty", uri != "")
	assert.True("factory required", factory != nil)

	// Cache hit: content-addressable singletons return existing for non-Gone states (Rule 6). Location-based — and Gone on
	// either addressing — fall through to shadow (Rules 7 and "Gone is terminal, revive via shadow"). See
	// docs/architecture/4-resource-management.md §6.2.
	if id := c.Current(uri); id != "" {
		if existing, ok := c.Lookup(id); ok {
			if existing.Addressing() == AddressingContent && c.State(id) != Gone {
				return existing, nil
			}
		}
	}

	// Cache miss, or cache hit on location-based any-state, or cache hit on Gone (either addressing).
	candidate, err := factory()
	if err != nil {
		return nil, err
	}

	if _, err := c.Shadow(candidate, producerID); err != nil {
		return nil, err
	}
	c.markActive(candidate)
	return candidate, nil
}

// Len returns the number of entries in the ledger.
//
// Returns:
//   - `int`: the entry count.
func (c *ResourceCatalog) Len() int {

	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.entries)
}

// Link interns the given resource and returns the canonical catalog entry, discarding the catalog ID.
//
// Link is a thin convenience over [ResourceCatalog.Resolve] for callers that only need the linked Resource — notably
// the slot-fill path in the plan provider's dispatch and the rehydration path in plan.load_definition. Behavior matches
// Resolve exactly: first sighting of a URI catalogs the input as a discovery entry; subsequent sightings discard the
// input in favor of the canonical entry, which may already carry a producerID stamped by a producer node's Planned
// companion. The producerID stays on the returned Resource for downstream consumers to observe.
//
// Parameters:
//   - `resource`: the resource to intern. URI must be set.
//
// Returns:
//   - `Resource`: the canonical entry for `resource`'s URI.
func (c *ResourceCatalog) Link(resource Resource) Resource {

	linked, _ := c.Resolve(resource)
	return linked
}

// Lookup returns the resource with the given catalog ID, or false if no entry exists for that ID.
//
// Parameters:
//   - `id`: the catalog ID to look up.
//
// Returns:
//   - `Resource`: the resource at that ID.
//   - `bool`: true if the ID is known.
func (c *ResourceCatalog) Lookup(id string) (Resource, bool) {

	c.mu.Lock()
	defer c.mu.Unlock()

	idx, ok := c.byID[id]
	if !ok {
		return nil, false
	}
	return c.entries[idx], true
}

// MarkGone records a successful deletion: the entry's lifecycle state transitions to [Gone].
//
// The mutator-side counterpart of [ResourceCatalog.VerifyExistence]'s discovery-side transition (phase-8 step 23,
// ruling 3): a provider that has removed the disk entry behind an interned resource reports the termination, so the
// catalog reflects what the run DID, not only what it observed. Any state may transition in — deleting a [Pending]
// entry is legal (the delete itself just observed the disk) — and re-marking a [Gone] entry is idempotent. Gone is
// terminal: no catalog operation transitions out of it, and reviving the URI is a production act that appends a
// fresh generation via [ResourceCatalog.GetOrCreate]'s shadow path.
//
// Parameters:
//   - `r`: the cataloged [Resource] whose deletion is being recorded. Must carry a catalog id — route candidates
//     through [ResourceCatalog.Discover] or [ResourceCatalog.GetOrCreate] first.
//
// Panics with an [*assert.AssertionError] when `r` is nil or not cataloged — a programming error at the call site,
// not a runtime condition.
func (c *ResourceCatalog) MarkGone(r Resource) {

	assert.True("resource required", r != nil)
	assert.True("resource is cataloged", r.resourceBase().id != "")

	c.markGone(r)
}

// Resolve returns the canonical resource for the given resource's URI, along with its catalog ID.
//
// If the URI has never been seen, r is cataloged as a discovery entry (no origin) and returned as-is. If the URI was
// previously cataloged — either as a discovery or shadowed by a producer — the canonical entry is returned and r is
// discarded. Callers should always use the returned Resource, not the one they passed in, so downstream consumers
// observe the authoritative version.
//
// The caller is responsible for type-tagging the input: a raw string path becomes a *file.Resource via the resource
// type's registered constructor before reaching the catalog. The catalog never fabricates a concrete Resource type
// itself — the concrete type flows in from the caller.
//
// Resolve is the link-time lookup operation: planner dispatches use it to convert typed-but-unresolved inputs into the
// catalog's canonical entries, picking up any `producerID` that a producer has already stamped and so creating implicit
// edges via URI matching.
//
// Freshness cascade on cache hit (per [ResourceBase.Etag]'s contract): the catalog branches on `r.Addressing()`. For
// [AddressingContent], the URI carries the digest, so URI lookup is the complete identity check — no Etag or Digest
// call is needed. For [AddressingLocation], the canonical entry's freshness is verified via the
// Etag-mismatch-then-Digest cascade: compare the input's Etag to the canonical's Etag; on match, fast-pass; on
// mismatch, compute Digest on both sides; on Digest match, the mismatch is metadata drift only and the canonical is
// returned unchanged; on Digest mismatch, the canonical is still returned (Resolve preserves the cached identity), but
// the drift will be visible to a future reconciliation pass. Etag and Digest calls happen outside the catalog mutex so
// they cannot block other namespace operations.
//
// Parameters:
//   - `r`: a typed resource with its URI set.
//
// Returns:
//   - `Resource`: the canonical entry for `r`'s URI.
//   - `string`: the canonical entry's catalog ID.
func (c *ResourceCatalog) Resolve(r Resource) (Resource, string) {

	canonical, id, hit := c.lookupOrCatalog(r)
	if !hit {
		return canonical, id
	}

	if r.Addressing() == AddressingContent {
		return canonical, id
	}

	verifyLocationFreshness(canonical, r)
	return canonical, id
}

// Shadow catalogs a new resource version under the given producer and updates the namespace to point to it.
//
// Shadow is the plan-time output registration operation: a node's Planned companion constructs the identity of the
// resource the node will produce, and the planner hands that identity to Shadow so subsequent [ResourceCatalog.Resolve]
// calls for the same URI return the shadowed version — wiring downstream readers to the producer via the stamped
// `producerID`.
//
// `producerID` may be empty. An empty `producerID` denotes a non-claiming dispatch — typically a bridge-side or test
// [ActivationRecord] whose Unit is nil. Non-claiming dispatches defer to any existing claim on the same URI and never
// produce a write-write conflict.
//
// Conflict, supersede, and defer semantics:
//   - both empty, no existing entry → append a discovery entry, point namespace at it
//   - both empty, existing also empty → idempotent re-discovery (append new ledger entry, repoint namespace)
//   - incoming non-empty over existing empty → silently supersede (discovery yields to the producer claim)
//   - incoming non-empty matches existing non-empty → idempotent re-claim (append new ledger entry)
//   - incoming non-empty differs from existing non-empty → conflict error
//   - incoming empty over existing non-empty → defer to the existing claim (no new entry, no namespace change)
//
// Parameters:
//   - `r`: the resource whose identity should be shadowed. URI must be set.
//   - `producerID`: the node ID claiming ownership of the URI, or empty for a non-claiming dispatch.
//
// Returns:
//   - `string`: the catalog ID of either the newly-shadowed entry or the existing claim deferred to.
//   - `error`: non-nil only on a non-empty/non-empty mismatch.
func (c *ResourceCatalog) Shadow(r Resource, producerID string) (string, error) {

	c.mu.Lock()
	defer c.mu.Unlock()

	uri := r.URI()

	if existingID, ok := c.ns[namespaceKey(r)]; ok {
		if idx, ok := c.byID[existingID]; ok {
			existingProducer := c.entries[idx].resourceBase().producerID
			switch {
			case existingProducer != "" && producerID == "":
				return existingID, nil
			case existingProducer != "" && producerID != "" && existingProducer != producerID:
				return "", fmt.Errorf(
					"resource conflict: URI %q is targeted by both %q and %q",
					uri, existingProducer, producerID,
				)
			}
		}
	}

	return c.catalogLocked(r, producerID), nil
}

// Snapshot projects the catalog into a serializable [*ResourceLedgerSnapshot] — every generation, keyed by id.
//
// The recovery stack references ledger entries by id; a resource URI is not a unique identity, because [Shadow]
// re-catalogs an existing URI as a fresh generation and the URI→id namespace tracks only the current one. Snapshot
// therefore captures every entry in append order (each as id, URI, producerID, and lifecycle state) plus the
// observation index and the id counter, so the live ledger can be rebuilt on resume with ids preserved.
//
// Active entries additionally record both content-identity tiers — [Resource.Etag] and [Resource.Digest] — best
// effort (phase-8 step 48): an error leaves the field empty; Pending has nothing on disk and Gone cannot be
// read, so both record neither. The tier calls do I/O, so they run after the catalog mutex is released
// (mirroring [verifyLocationFreshness]'s discipline).
//
// Returns:
//   - `*ResourceLedgerSnapshot`: the serializable ledger projection.
func (c *ResourceCatalog) Snapshot() *ResourceLedgerSnapshot {

	c.mu.Lock()

	type captured struct {
		resource Resource
		entry    LedgerEntrySnapshot
	}

	pending := make([]captured, 0, len(c.entries))
	for _, resource := range c.entries {
		base := resource.resourceBase()
		pending = append(pending, captured{
			resource: resource,
			entry: LedgerEntrySnapshot{
				ID:         base.id,
				URI:        resource.URI(),
				ProducerID: base.producerID,
				State:      c.states[base.id],
			},
		})
	}
	nextID := c.nextID

	c.mu.Unlock()

	entries := make([]LedgerEntrySnapshot, 0, len(pending))
	for _, p := range pending {
		if p.entry.State == Active {
			if etag, err := p.resource.Etag(); err == nil {
				p.entry.Etag = etag
			}
			if digest, err := p.resource.Digest(); err == nil {
				p.entry.Digest = digest.String()
			}
		}
		entries = append(entries, p.entry)
	}

	return &ResourceLedgerSnapshot{
		Entries: entries,
		NextID:  nextID,
	}
}

// State returns the lifecycle state for the catalog entry with the given id.
//
// The state is per-catalog (per-run): a Clone starts with its own fresh state map, so a run's transitions never leak
// back to the source catalog. Unknown ids (never cataloged here, or cataloged in a sibling catalog) return the
// zero-value [Pending].
//
// Parameters:
//   - `id`: the catalog id stamped on the resource by [GetOrCreate] / [Shadow] (read via [ResourceBase.ID]).
//
// Returns:
//   - `ResourceState`: the current lifecycle state — `Pending` (zero value, newly cataloged), `Active` (observed or
//     produced), or `Gone` (Resolve failed; terminal).
func (c *ResourceCatalog) State(id string) ResourceState {

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.states[id]
}

// VerifyExistence resolves a cataloged resource's lifecycle from its [Resource.Exists] verdict — the discovery-side
// counterpart to production's [ResourceCatalog.markActive] (phase-8 step 22).
//
// An entry already resolved to [Active] is left as-is (no re-check); otherwise the resource's [Resource.Exists]
// predicate drives the catalog-owned transition — [markActive] when the resource exists, [markGone] plus an error when
// it does not — so a [Pending] entry becomes [Active] or [Gone] per §6.2 of
// docs/architecture/4-resource-management.md. The caller owns the reaction to a missing resource: the executor's
// pre-flight resolve pass records the [Gone] mark and decides independently whether the run proceeds (a `Gone`
// resource is a recorded fact; consumers of it fail on their own).
//
// Only resources whose type implements a real [Resource.Exists] participate: the [ResourceBase] default is a loud
// [assert.Unimplemented] stub, so a not-yet-migrated type must not be routed here (step 22 wires file only).
//
// Parameters:
//   - `resource`: the cataloged resource to verify; its [Resource.ID] must be stamped (already interned).
//
// Returns:
//   - `error`: non-nil (and the entry marked [Gone]) when the resource does not exist.
func (c *ResourceCatalog) VerifyExistence(resource Resource) error {

	if c.State(resource.ID()) == Active {
		return nil
	}

	if resource.Exists() {
		c.markActive(resource)
		return nil
	}

	c.markGone(resource)
	return fmt.Errorf("verify existence: resource %q does not exist", resource.URI())
}

// lookupOrCatalog performs the namespace lookup under the catalog mutex.
//
// On hit returns the canonical entry; on miss interns r as a discovery entry and returns it. Caller must run any
// freshness cascade outside the returned value, since Etag/Digest calls may do I/O.
//
// Parameters:
//   - `r`: a typed resource with its URI set.
//
// Returns:
//   - `Resource`: the canonical entry on hit, or `r` itself on miss (now cataloged).
//   - `string`: the canonical catalog ID.
//   - `bool`: true if `r`.URI() was already cataloged; false if `r` was just interned.
func (c *ResourceCatalog) lookupOrCatalog(r Resource) (Resource, string, bool) {

	c.mu.Lock()
	defer c.mu.Unlock()

	if id, ok := c.ns[namespaceKey(r)]; ok {
		if idx, ok := c.byID[id]; ok {
			return c.entries[idx], id, true
		}
	}

	id := c.catalogLocked(r, "")
	return r, id, false
}

// verifyLocationFreshness runs the Etag-mismatch-then-Digest cascade for [AddressingLocation] entries on cache hit.
//
// The cascade is informational under Resolve's contract: any mismatch is recorded by the function's side effects (none
// today — the drift signal is left to a future reconciliation pass) but does not change the caller-visible return. Etag
// and Digest calls run here, outside the catalog mutex.
//
// Parameters:
//   - `canonical`: the catalog's stored Resource for the URI.
//   - `observed`: the input Resource the caller passed to Resolve.
func verifyLocationFreshness(canonical, observed Resource) {

	observedEtag, err := observed.Etag()
	if err != nil {
		return
	}

	canonicalEtag, err := canonical.Etag()
	if err != nil {
		return
	}

	if observedEtag == canonicalEtag {
		return
	}

	observedDigest, err := observed.Digest()
	if err != nil {
		return
	}
	canonicalDigest, err := canonical.Digest()
	if err != nil {
		return
	}

	if observedDigest.String() == canonicalDigest.String() {
		return
	}

	// Genuine content drift: Etag and Digest both differ. Resolve keeps the cached canonical identity and takes no
	// action on the drift here; surfacing it is a separate reconciliation concern.
}

// endregion

// endregion

// region HELPER FUNCTIONS

// catalogLocked appends r to the ledger, stamps its catalog id and producerID, and repoints the URI namespace.
//
// Stamps land on the embedded ResourceBase. Caller must hold c.mu.
//
// Parameters:
//   - `r`: the resource to catalog.
//   - `producerID`: the producer to stamp on `r`'s ResourceBase. Empty for discoveries, set for shadows.
//
// Returns:
//   - `string`: the catalog ID assigned to the new entry.
func (c *ResourceCatalog) catalogLocked(r Resource, producerID string) string {

	c.nextID++
	id := fmt.Sprintf("res-%d", c.nextID)

	base := r.resourceBase()
	base.id = id
	base.producerID = producerID

	c.byID[id] = len(c.entries)
	c.entries = append(c.entries, r)
	c.ns[namespaceKey(r)] = id

	return id
}

// markActive transitions r's state to Active.
//
// Package-private — only catalog operations call this; provider code has no setter for the state field. Safe to call
// without holding the catalog mutex: state is a single int field, and concurrent operations on the same Resource are
// not part of the catalog's contract.
//
// Parameters:
//   - `r`: the Resource whose state to transition.
func (c *ResourceCatalog) markActive(r Resource) {

	c.mu.Lock()
	defer c.mu.Unlock()

	c.states[r.resourceBase().id] = Active
}

// markGone transitions r's state to Gone.
//
// Package-private; same locking notes as [markActive]. Gone is terminal — no catalog operation transitions out of it;
// reviving a Gone URI requires a NewResource call, which appends a fresh entry via Shadow rather than mutating the
// existing one.
//
// Parameters:
//   - `r`: the Resource whose state to transition.
func (c *ResourceCatalog) markGone(r Resource) {

	c.mu.Lock()
	defer c.mu.Unlock()

	c.states[r.resourceBase().id] = Gone
}

// namespaceKey returns the URI→id namespace key for `r` under its addressing mode's keying regime.
//
// [AddressingLocation] entries key on the fragment-stripped URI — architecture/4.1-resource-identity.md §2's rule
// (the fragment is metadata, never part of the catalog key), restored by phase-8 step 23 after 22(k)'s canonical
// tag form baked the concrete Go type id into the fragment and silently split the per-path namespace by kind: one
// path is one entry no matter which taxonomy variant claims it, so cross-kind claims collide in the constructors'
// typed assertions and [ResourceCatalog.Shadow]'s write-write detection sees one key per path.
//
// [AddressingContent] (and unknown-addressing) entries keep full-URI keying for now. The canonical-form identity
// ruling (2026-07-18: the codec specifier is metadata, not identity — equal canonical content shadows across
// json/yaml/protobuf) is settled but its content-hit shadow mechanics land after phase 8; until then the fragment
// keeps equal-hash entries of different formats apart, exactly as 22(k) built it.
//
// Parameters:
//   - `r`: the resource being keyed.
//
// Returns:
//   - `string`: the namespace key.
func namespaceKey(r Resource) string {

	if r.Addressing() == AddressingLocation {
		return stripFragment(r.URI())
	}

	return r.URI()
}

// pendingEntries returns a snapshot of every ledger entry whose lifecycle state is [Pending], in append order.
//
// The executor's pre-flight resolve pass iterates this snapshot and drives each participating entry through
// [ResourceCatalog.VerifyExistence] (phase-8 step 22). The copy is taken under the catalog mutex; verification runs
// against the returned slice afterward, so per-entry I/O never holds the lock.
//
// Returns:
//   - []Resource: the [Pending] entries; empty when none are pending.
func (c *ResourceCatalog) pendingEntries() []Resource {

	c.mu.Lock()
	defer c.mu.Unlock()

	var pending []Resource
	for _, resource := range c.entries {
		if c.states[resource.resourceBase().id] == Pending {
			pending = append(pending, resource)
		}
	}

	return pending
}

// restoreEntry appends a reconstructed generation to the ledger with its saved id, producerID, and lifecycle state.
//
// It is the rehydration counterpart to [catalogLocked]: where catalogLocked mints a fresh id, restoreEntry preserves
// the id captured in a [ResourceLedgerSnapshot] so the recovery stack's id references resolve via [Lookup] after a
// save/load/resume. Entries are restored in append order, so the URI→id namespace's current-generation pointer is
// reproduced (last writer for a URI wins). The receiver is a freshly constructed catalog, not yet shared, so no lock
// is taken.
//
// Parameters:
//   - `id`: the saved catalog id (`res-N`).
//   - `resource`: the reconstructed Resource object (its URI re-derived from the snapshot).
//   - `producerID`: the saved producer stamp, or empty for a discovery entry.
//   - `state`: the saved lifecycle state.
func (c *ResourceCatalog) restoreEntry(id string, resource Resource, producerID string, state ResourceState) {

	base := resource.resourceBase()
	base.id = id
	base.producerID = producerID

	c.byID[id] = len(c.entries)
	c.entries = append(c.entries, resource)
	c.ns[namespaceKey(resource)] = id
	c.states[id] = state
}

// stripFragment returns `uri` with any fragment component removed.
//
// Parameters:
//   - `uri`: the URI to strip.
//
// Returns:
//   - `string`: `uri` up to (not including) the first `#`, or `uri` unchanged when it carries no fragment.
func stripFragment(uri string) string {

	if i := strings.IndexByte(uri, '#'); i >= 0 {
		return uri[:i]
	}

	return uri
}

// endregion

// region SUPPORTING TYPES

// ResourceLedgerSnapshot is the serializable projection of a [ResourceCatalog] — every generation, keyed by id.
//
// It is the [Trace] field that lets a paused run's resource ledger survive save → load → resume. A resource URI is not
// a unique identity (see [ResourceCatalog.Snapshot]), so entries are keyed and referenced by id; the recovery stack's
// receipt references resolve against the rehydrated ledger by id.
type ResourceLedgerSnapshot struct {

	// Entries is every ledger generation in append order. Replaying that order reproduces the URI→id namespace's
	// current-generation pointer (last writer for a URI wins).
	Entries []LedgerEntrySnapshot `json:"entries" yaml:"entries"`

	// NextID is the monotonic id counter, restored so post-resume production continues the id sequence.
	NextID int `json:"next_id" yaml:"next_id"`
}

// LedgerEntrySnapshot is one ledger generation's serializable identity and lifecycle state.
type LedgerEntrySnapshot struct {

	// ID is the catalog id (`res-N`) — the stable identity the recovery stack references.
	ID string `json:"id" yaml:"id"`

	// URI is the resource's URI, from which the concrete Resource object is rebuilt on rehydration.
	URI string `json:"uri" yaml:"uri"`

	// ProducerID is the producing unit's id, or empty for a discovery entry.
	ProducerID string `json:"producer_id,omitempty" yaml:"producer_id,omitempty"`

	// State is the entry's lifecycle state at capture time.
	State ResourceState `json:"state" yaml:"state"`

	// Etag is the entry's cheap change-detection token at capture time (phase-8 step 48). Drift consumers
	// compare a live Etag against this first and compute a Digest only on mismatch — the catalog's own cascade.
	// A recorded Etag equal to the entry's URI is the uninformative [ResourceBase] default; consumers bypass
	// the screen and compare digests directly. Captured best effort for Active entries only; reporting
	// metadata — [ResourceLedgerSnapshot.Rehydrate] ignores it.
	Etag string `json:"etag,omitempty" yaml:"etag,omitempty"`

	// Digest is the entry's honest content identity at capture time, in the canonical "<algo>:<hex>" form
	// (phase-8 step 48) — the as-deployed record drift attribution compares against (source-changed vs.
	// target-modified). Captured best effort for Active entries only (a digest error — e.g. the directory case,
	// deferred to step 23's Merkle deliverable — leaves it empty); reporting metadata —
	// [ResourceLedgerSnapshot.Rehydrate] ignores it.
	Digest string `json:"digest,omitempty" yaml:"digest,omitempty"`
}

// Rehydrate rebuilds a live [*ResourceCatalog] from the snapshot, preserving every generation's id.
//
// Each entry's Resource object is reconstructed from its URI: [ExtractTagSpecific] splits the URI into its `specific`
// identity and the type id, the type id resolves the registered [ResourceConstructor] (see
// [receiverRegistry.ResourceConstructorByTypeID]), and the constructor rebuilds the object from the `specific` part.
// Interning is disabled during reconstruction (a nil catalog makes the constructor return the bare candidate), so
// [ResourceCatalog.restoreEntry] stamps the saved id rather than minting a fresh one. Reconstruction in append order
// reproduces the namespace's current-generation pointer.
//
// Location-addressed resources (e.g. file) round-trip exactly when the resume root resolves paths as the capture root
// did. A content-addressed resource whose `specific` is a digest can be rebuilt only if its constructor accepts that
// `specific` as a value; otherwise it surfaces here as a reconstruction error.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment; its catalog is detached during reconstruction and restored before
//     return, so the caller installs the returned catalog.
//
// Returns:
//   - `*ResourceCatalog`: the rebuilt ledger, ids preserved.
//   - `error`: a malformed URI, an unregistered type id, or a constructor failure.
func (s *ResourceLedgerSnapshot) Rehydrate(runtimeEnvironment *RuntimeEnvironment) (*ResourceCatalog, error) {

	restored := NewResourceCatalog()

	// Disable interning so each constructor returns a bare candidate; restoreEntry then stamps the saved id rather than
	// minting a fresh one. Restored before return — the caller installs the returned catalog.
	savedCatalog := runtimeEnvironment.ResourceCatalog
	runtimeEnvironment.ResourceCatalog = nil
	defer func() { runtimeEnvironment.ResourceCatalog = savedCatalog }()

	for _, entry := range s.Entries {

		specific, typeID, err := ExtractTagSpecific(entry.URI)
		if err != nil {
			return nil, fmt.Errorf("op.ResourceLedgerSnapshot.Rehydrate: entry %q: %w", entry.ID, err)
		}

		construct, ok := ReceiverRegistry().ResourceConstructorByTypeID(typeID)
		if !ok {
			return nil, fmt.Errorf(
				"op.ResourceLedgerSnapshot.Rehydrate: entry %q: no resource type registered for %q", entry.ID, typeID)
		}

		resource, err := construct(runtimeEnvironment, specific)
		if err != nil {
			return nil, fmt.Errorf(
				"op.ResourceLedgerSnapshot.Rehydrate: entry %q: reconstruct %q: %w", entry.ID, entry.URI, err)
		}

		restored.restoreEntry(entry.ID, resource, entry.ProducerID, entry.State)
	}

	restored.nextID = s.NextID

	return restored, nil
}

// endregion
