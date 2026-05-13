// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
)

// ResourceCatalog is the graph-level compositor that owns the append-only ledger of [Resource] entries and the
// URI→ID namespace that makes URIs addressable.
//
// One catalog per [Graph]. Created at plan time by the planner, consumed at execution time by the executor's
// preflight pass and post-dispatch transition. See docs/architecture/4-resource-management.md §6.1-§6.5, §6.8.
//
// The catalog holds [Resource] interface values, which are pointers to concrete resource structs (e.g.,
// [*file.Resource]). Preflight and node execution populate metadata fields on those structs in place; all
// holders of the pointer see the updated fields. The ledger's append-only property refers to the sequence of
// distinct resources, not to the mutability of their metadata.
//
// Two observable states, derived from an entry's producer:
//
//   - Discovery: producerID == "". The entry was registered without a production claim — by
//     [ResourceCatalog.Discover], by a discovery-style provider call, or by reference handles in CLI tools.
//     The catalog tracks the URI but no dispatch claims to have created it.
//   - Production: producerID != "". The entry was created by [ResourceCatalog.GetOrCreate] from a producer
//     dispatch context. The producerID is the dispatch's SiteID (typically the graph node ID) and is the
//     answer to "who created this URI?" for downstream producer→consumer edge derivation.
//
// The catalog does not expose a [State] enum. States are a property of an entry's producerID being empty
// or set; the catalog only tracks identity and lineage.
type ResourceCatalog struct {
	mu      sync.Mutex
	entries []Resource        // append-only ledger
	byID    map[string]int    // id → index in entries
	ns      map[string]string // URI → current id (the namespace)
	nextID  int               // monotonic counter for id generation
}

// NewResourceCatalog creates an empty catalog.
//
// Returns:
//   - *ResourceCatalog: the empty catalog.
func NewResourceCatalog() *ResourceCatalog {
	return &ResourceCatalog{
		byID: make(map[string]int),
		ns:   make(map[string]string),
	}
}

// region EXPORTED METHODS

// region State management

// Resolve returns the canonical resource for the given resource's URI, along with its catalog ID.
//
// If the URI has never been seen, r is cataloged as a discovery entry (no origin) and returned as-is.
// If the URI was previously cataloged — either as a discovery or shadowed by a producer — the canonical entry
// is returned and r is discarded. Callers should always use the returned Resource, not the one they passed
// in, so downstream consumers observe the authoritative version.
//
// The caller is responsible for type-tagging the input: a raw string path becomes a *file.Resource via the
// resource type's registered constructor before reaching the catalog. The catalog never fabricates a concrete
// Resource type itself — the concrete type flows in from the caller.
//
// Resolve is the link-time lookup operation: planner dispatches use it to convert typed-but-unresolved inputs
// into the catalog's canonical entries, picking up any `producerID` that a producer has already stamped and so
// creating implicit edges via URI matching.
//
// Freshness cascade on cache hit (per [ResourceBase.Etag]'s contract): the catalog branches on
// `r.Addressing()`. For [AddressingContent], the URI carries the digest, so URI lookup is the complete identity
// check — no Etag or Digest call is needed. For [AddressingLocation], the canonical entry's freshness is
// verified via the Etag-mismatch-then-Digest cascade: compare the input's Etag to the canonical's Etag; on
// match, fast-pass; on mismatch, compute Digest on both sides; on Digest match, the mismatch is metadata
// drift only and the canonical is returned unchanged; on Digest mismatch, the canonical is still returned
// (Resolve preserves the cached identity), but the drift will be visible to a future reconciliation pass.
// Etag and Digest calls happen outside the catalog mutex so they cannot block other namespace operations.
//
// Parameters:
//   - r: a typed resource with its URI set.
//
// Returns:
//   - Resource: the canonical entry for r's URI.
//   - string: the canonical entry's catalog ID.
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

// lookupOrCatalog performs the namespace lookup under the catalog mutex. On hit returns the canonical entry;
// on miss interns r as a discovery entry and returns it. Caller must run any freshness cascade outside the
// returned value, since Etag/Digest calls may do I/O.
//
// Parameters:
//   - r: a typed resource with its URI set.
//
// Returns:
//   - Resource: the canonical entry on hit, or r itself on miss (now cataloged).
//   - string: the canonical catalog ID.
//   - bool: true if r.URI() was already cataloged; false if r was just interned.
func (c *ResourceCatalog) lookupOrCatalog(r Resource) (Resource, string, bool) {

	c.mu.Lock()
	defer c.mu.Unlock()

	uri := r.URI()

	if id, ok := c.ns[uri]; ok {
		if idx, ok := c.byID[id]; ok {
			return c.entries[idx], id, true
		}
	}

	id := c.catalogLocked(r, "")
	return r, id, false
}

// verifyLocationFreshness runs the Etag-mismatch-then-Digest cascade for [AddressingLocation] entries on
// cache hit.
//
// The cascade is informational under Resolve's contract: any mismatch is recorded by the function's side
// effects (none today — the drift signal is left to a future reconciliation pass) but does not change the
// caller-visible return. Etag and Digest calls run here, outside the catalog mutex.
//
// Parameters:
//   - canonical: the catalog's stored Resource for the URI.
//   - observed: the input Resource the caller passed to Resolve.
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

	// Genuine content drift. Resolve preserves cached identity; the drift will surface in a future
	// reconciliation pass (k.15). No side effect here today.
}

// GetOrCreate returns the canonical catalog entry for uri after recording the producer's claim.
//
// GetOrCreate is the production-claim hook. Forward-method outputs flow through it via each provider's
// `NewResource(activation, ...)` constructor. The catalog stays type-neutral; the factory closure resolves
// the concrete-type-to-construct decision at the call site, where the type is statically known. The
// producerID stamp on the resulting entry is `activation.SiteID` — the executor sets that to the dispatching
// graph node's ID (or to a synthesized label for non-graph dispatch contexts like the starlark immediate-
// mode bridge or test fixtures).
//
// Cache-hit behavior branches on the existing entry's [Addressing] × [State] per
// docs/architecture/4-resource-management.md §6.2's behavior matrix. The factory is invoked on cache miss,
// on location-based hits (any state), and on Gone hits (either addressing — Gone is terminal, so revival
// appends a new ledger entry via [Shadow]). Content-addressable hits on Pending or Active return the
// existing entry without invoking the factory (singleton). The new or revived entry transitions to Active
// via [markActive] before returning.
//
// A non-nil factory error short-circuits without touching the catalog. A different producer claiming the
// same URI surfaces as a Shadow conflict (write-write detection).
//
// Parameters:
//   - activation: per-dispatch [ActivationRecord] for the producing dispatch. Must be non-nil with a
//     non-empty SiteID — GetOrCreate is the production-side hook and asserts these invariants. Discovery
//     callsites (receipt rehydration, scanner-style URI lookups) must use [ResourceCatalog.Discover] instead.
//   - uri: the URI to look up. Must not be empty (asserted).
//   - factory: closure invoked on cache miss (or location/Gone shadow path) to construct a fresh [Resource].
//     Must be non-nil (asserted).
//
// Returns:
//   - Resource: the canonical catalog entry for uri, in state Active.
//   - error: any factory error (returned untouched), or a [Shadow] conflict if a different producer already
//     claimed the same URI.
//
// Panics with an [*assert.AssertionError] when any precondition is violated — these are programming errors
// at the call site, not runtime conditions.
func (c *ResourceCatalog) GetOrCreate(activation *ActivationRecord, uri string, factory func() (Resource, error)) (Resource, error) {

	assert.NotNil("activation", activation)
	assert.True("activation.SiteID not empty", activation.SiteID != "")
	assert.True("uri not empty", uri != "")
	assert.NotNil("factory", factory)

	// Cache hit: content-addressable singletons return existing for non-Gone states (Rule 6).
	// Location-based — and Gone on either addressing — fall through to shadow (Rules 7 and "Gone is terminal,
	// revive via shadow"). See docs/architecture/4-resource-management.md §6.2.
	if id := c.Current(uri); id != "" {
		if existing, ok := c.Lookup(id); ok {
			if existing.Addressing() == AddressingContent && existing.State() != Gone {
				return existing, nil
			}
		}
	}

	// Cache miss, or cache hit on location-based any-state, or cache hit on Gone (either addressing).
	candidate, err := factory()
	if err != nil {
		return nil, err
	}

	if _, err := c.Shadow(candidate, activation.SiteID); err != nil {
		return nil, err
	}
	c.markActive(candidate)
	return candidate, nil
}

// Discover returns the canonical catalog entry for uri after verifying that the resource exists.
//
// Discover is the consumption-side counterpart to [GetOrCreate]. Use it from non-production callsites:
// receipt rehydration during unmarshal, scanner-style URI lookups during preflight, and any other path
// where there is no producing node. The returned entry has no producerID stamped (or carries whatever
// stamp a previous GetOrCreate already applied) — discovery records existence, not authorship.
//
// Cache-hit behavior branches on the existing entry's [State] per the [DiscoverResource] rules in
// docs/architecture/4-resource-management.md §6.2: Active returns the existing entry as a cheap hit;
// Gone returns an error without re-attempting Resolve (Gone is terminal); Pending invokes
// Resolve in place — success transitions to Active, failure transitions to Gone and surfaces the error.
//
// Cache-miss behavior constructs a fresh candidate via factory, calls Resolve to verify existence, links
// the candidate into the catalog regardless of Resolve outcome (so the Gone entry is recorded as
// history), then transitions the linked entry to Active on success or Gone on failure.
//
// Parameters:
//   - uri: the URI to look up. Must not be empty (asserted).
//   - factory: closure invoked on cache miss to construct a fresh [Resource]. Must be non-nil (asserted).
//
// Returns:
//   - Resource: the canonical catalog entry for uri, in state Active. Nil if Resolve failed (Gone) or the
//     entry is already known-Gone.
//   - error: any factory error (returned untouched), any Resolve error (after the catalog has been
//     updated to mark the entry Gone), or a known-gone error if the URI's existing entry is Gone.
//
// Panics with an [*assert.AssertionError] when any precondition is violated — these are programming
// errors at the call site, not runtime conditions.
func (c *ResourceCatalog) Discover(uri string, factory func() (Resource, error)) (Resource, error) {

	assert.True("uri not empty", uri != "")
	assert.NotNil("factory", factory)

	// Cache hit: branch on state per the DiscoverResource rules (Rule 3 + Rule 4).
	if id := c.Current(uri); id != "" {
		if existing, ok := c.Lookup(id); ok {
			switch existing.State() {
			case Active:
				return existing, nil
			case Gone:
				return nil, fmt.Errorf("discover %q: resource is known-gone", uri)
			case Pending:
				if err := existing.Resolve(); err != nil {
					c.markGone(existing)
					return nil, err
				}
				c.markActive(existing)
				return existing, nil
			}
		}
	}

	// Cache miss: construct + Resolve to verify existence.
	candidate, err := factory()
	if err != nil {
		return nil, err
	}

	if err := candidate.Resolve(); err != nil {
		linked := c.Link(candidate)
		c.markGone(linked)
		return nil, err
	}

	linked := c.Link(candidate)
	c.markActive(linked)
	return linked, nil
}

// Link interns the given resource and returns the canonical catalog entry, discarding the catalog ID.
//
// Link is a thin convenience over [ResourceCatalog.Resolve] for callers that only need the linked Resource —
// notably the slot-fill path in [starlarkbridge.NodeBuilder] and the rehydration path in plan.load (step 16).
// Behavior matches Resolve exactly: first sighting of a URI catalogs the input as a discovery entry; subsequent
// sightings discard the input in favor of the canonical entry, which may already carry a producerID stamped by
// a producer node's Planned companion. The producerID stays on the returned Resource for downstream consumers to
// observe (extracted via [ExtractResource] at plan.run materialization to derive producer→consumer edges).
//
// Parameters:
//   - resource: the resource to intern. URI must be set.
//
// Returns:
//   - Resource: the canonical entry for resource's URI.
func (c *ResourceCatalog) Link(resource Resource) Resource {

	linked, _ := c.Resolve(resource)
	return linked
}

// Shadow catalogs a new resource version under the given producer and updates the namespace to point to it.
//
// Shadow is the plan-time output registration operation: a node's Planned companion constructs the identity
// of the resource the node will produce, and the planner hands that identity to Shadow so subsequent
// [ResourceCatalog.Resolve] calls for the same URI return the shadowed version — wiring downstream readers
// to the producer via the stamped `producerID`.
//
// Write-write conflict detection: if the URI is already shadowed by a different non-empty producer, Shadow
// returns an error. Two nodes targeting the same output URI collide immediately with a clear error. Discovery
// entries (empty producer) are silently superseded. Re-shadowing with the same producer is permitted (idempotent
// re-claims, e.g., a producer method called twice for the same target).
//
// Parameters:
//   - r: the resource whose identity should be shadowed. URI must be set.
//   - producerID: the node ID claiming ownership of the URI. Must not be empty.
//
// Returns:
//   - string: the catalog ID assigned to the newly-shadowed entry.
//   - error: non-nil if another producer already shadows the same URI.
func (c *ResourceCatalog) Shadow(r Resource, producerID string) (string, error) {

	c.mu.Lock()
	defer c.mu.Unlock()

	if producerID == "" {
		return "", fmt.Errorf("shadow: producerID must not be empty")
	}

	uri := r.URI()

	if existingID, ok := c.ns[uri]; ok {
		if idx, ok := c.byID[existingID]; ok {
			existingProducer := c.entries[idx].resourceBase().producerID
			if existingProducer != "" && existingProducer != producerID {
				return "", fmt.Errorf(
					"resource conflict: URI %q is targeted by both %q and %q",
					uri, existingProducer, producerID,
				)
			}
		}
	}

	return c.catalogLocked(r, producerID), nil
}

// Lookup returns the resource with the given catalog ID, or false if no entry exists for that ID.
//
// Parameters:
//   - id: the catalog ID to look up.
//
// Returns:
//   - Resource: the resource at that ID.
//   - bool: true if the ID is known.
func (c *ResourceCatalog) Lookup(id string) (Resource, bool) {

	c.mu.Lock()
	defer c.mu.Unlock()

	idx, ok := c.byID[id]
	if !ok {
		return nil, false
	}
	return c.entries[idx], true
}

// Current returns the catalog ID of the entry currently authoritative for the given URI, or the empty string
// if the URI has never been seen.
//
// Parameters:
//   - uri: the URI to look up.
//
// Returns:
//   - string: the current catalog ID for uri, or "" if not found.
func (c *ResourceCatalog) Current(uri string) string {

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ns[uri]
}

// Len returns the number of entries in the ledger.
//
// Returns:
//   - int: the entry count.
func (c *ResourceCatalog) Len() int {

	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.entries)
}

// DiscoveryURIs returns the URIs of catalog entries that were cataloged as discoveries (producerID == "") and
// are still authoritative for their URI.
//
// A URI whose current entry has been shadowed by a producer is excluded — that URI is an output, not an
// input. Used by the executor's preflight pass to stat each discovered URI against the target machine before
// any node runs.
//
// Returns:
//   - []string: the discovery URIs. Order is not guaranteed.
func (c *ResourceCatalog) DiscoveryURIs() []string {

	c.mu.Lock()
	defer c.mu.Unlock()

	var uris []string
	for uri, id := range c.ns {
		idx, ok := c.byID[id]
		if !ok {
			continue
		}
		if c.entries[idx].resourceBase().producerID == "" {
			uris = append(uris, uri)
		}
	}
	return uris
}

// endregion

// endregion

// ExtractResource reports whether v carries resource identity and, if so, returns its producer node ID.
//
// Used by the planner's promise-filling path to create implicit edges: when a slot value is a resource whose
// URI was produced by another node (non-empty producerID), the planner adds an edge from the producer to the
// consumer even though the developer never wired it explicitly.
//
// Three forms are accepted:
//
//   - Values that implement [Resource] directly (pointer receivers, the common case).
//   - Struct values whose pointer type implements [Resource] — provider methods that return resources by
//     value rather than pointer. A temporary addressable copy is created to read the producerID.
//   - map[string]any decoded from an unmarshaled starlark struct, with a "producer_id" key (optionally nested
//     under "resource_base").
//
// Parameters:
//   - v: any value.
//
// Returns:
//   - string: the producerID extracted from v, or "" if v carries no resource identity.
//   - bool: true if producerID is non-empty.
func ExtractResource(v any) (producerID string, ok bool) {

	if v == nil {
		return "", false
	}

	// Interface match — pointer receivers.
	if r, isResource := v.(Resource); isResource {
		producer := r.resourceBase().producerID
		return producer, producer != ""
	}

	// Struct value whose pointer type satisfies Resource.
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Struct && reflect.PointerTo(rv.Type()).Implements(resourceInterfaceType) {
		ptr := reflect.New(rv.Type())
		ptr.Elem().Set(rv)
		r := ptr.Interface().(Resource)
		producer := r.resourceBase().producerID
		return producer, producer != ""
	}

	// map[string]any decoded from a starlark struct.
	if m, isMap := v.(map[string]any); isMap {
		if producer, _ := m["producer_id"].(string); producer != "" {
			return producer, true
		}
		if nested, nestedOK := m["resource_base"].(map[string]any); nestedOK {
			if producer, _ := nested["producer_id"].(string); producer != "" {
				return producer, true
			}
		}
	}

	return "", false
}

// region HELPER FUNCTIONS

// markActive transitions r's state to Active.
//
// Package-private — only catalog operations call this; provider code has no setter for the state field. Safe
// to call without holding the catalog mutex: state is a single int field, and concurrent operations on the
// same Resource are not part of the catalog's contract.
//
// Parameters:
//   - r: the Resource whose state to transition.
func (c *ResourceCatalog) markActive(r Resource) {
	r.resourceBase().state = Active
}

// markGone transitions r's state to Gone.
//
// Package-private; same locking notes as [markActive]. Gone is terminal — no catalog operation transitions
// out of it; reviving a Gone URI requires a NewResource call, which appends a fresh entry via Shadow rather
// than mutating the existing one.
//
// Parameters:
//   - r: the Resource whose state to transition.
func (c *ResourceCatalog) markGone(r Resource) {
	r.resourceBase().state = Gone
}

// catalogLocked appends r to the ledger, stamps its catalog id and producerID on the embedded ResourceBase,
// and updates the URI namespace to point to the new entry. Caller must hold c.mu.
//
// Parameters:
//   - r: the resource to catalog.
//   - producerID: the producer to stamp on r's ResourceBase. Empty for discoveries, set for shadows.
//
// Returns:
//   - string: the catalog ID assigned to the new entry.
func (c *ResourceCatalog) catalogLocked(r Resource, producerID string) string {

	c.nextID++
	id := fmt.Sprintf("res-%d", c.nextID)

	base := r.resourceBase()
	base.id = id
	base.producerID = producerID

	c.byID[id] = len(c.entries)
	c.entries = append(c.entries, r)
	c.ns[r.URI()] = id

	return id
}

// endregion
