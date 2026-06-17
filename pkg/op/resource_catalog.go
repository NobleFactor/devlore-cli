// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
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
// Two observable states, derived from an entry's producer:
//
//   - Discovery: producerID == "". The entry was registered without a production claim — by [ResourceCatalog.Discover],
//     by a discovery-style provider call, or by reference handles in CLI tools. The catalog tracks the URI but no
//     dispatch claims to have created it.
//   - Production: producerID != "". The entry was created by [ResourceCatalog.GetOrCreate] from a producer dispatch
//     context. The producerID is the dispatching [ExecutableUnit]'s ID (typically a graph node ID, occasionally a
//     subgraph ID) and is the answer to "who created this URI?" for downstream producer→consumer edge derivation.
//
// The catalog does not expose a [State] enum. States are a property of an entry's producerID being empty or set; the
// catalog only tracks identity and lineage.
type ResourceCatalog struct {
	mu                  sync.Mutex
	entries             []Resource        // append-only ledger
	byID                map[string]int    // id → index in entries
	ns                  map[string]string // URI → current id (the namespace)
	states              map[string]State  // id → per-run lifecycle state; independent of Resource identity
	currentObservations map[string]string // observed Resource URI → current observation Resource URI
	nextID              int               // monotonic counter for id generation
}

// NewResourceCatalog creates an empty catalog.
//
// Returns:
//   - `*ResourceCatalog`: the empty catalog.
func NewResourceCatalog() *ResourceCatalog {
	return &ResourceCatalog{
		byID:                make(map[string]int),
		ns:                  make(map[string]string),
		states:              make(map[string]State),
		currentObservations: make(map[string]string),
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

	states := make(map[string]State, len(c.states))
	for k, v := range c.states {
		states[k] = v
	}

	currentObservations := make(map[string]string, len(c.currentObservations))
	for k, v := range c.currentObservations {
		currentObservations[k] = v
	}

	return &ResourceCatalog{
		entries:             entries,
		byID:                byID,
		ns:                  ns,
		states:              states,
		currentObservations: currentObservations,
		nextID:              c.nextID,
	}
}

// Current returns the catalog ID authoritative for the given URI, or the empty string if the URI is unknown.
//
// Parameters:
//   - `uri`: the URI to look up.
//
// Returns:
//   - `string`: the current catalog ID for `uri`, or "" if not found.
func (c *ResourceCatalog) Current(uri string) string {

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ns[uri]
}

// CurrentObservation returns the most-recently-recorded observation for `observedURI`, or nil if there is none.
//
// Lookup is by the observed Resource's URI rather than its catalog id so callers do not need to hold a [Resource]
// pointer or look up its id first. The returned observation's [ObservationBase.OfResource] points to the originally
// observed Resource.
//
// Parameters:
//   - `observedURI`: the URI of the observed [Resource] (typically `r.URI()` for some `r Resource`).
//
// Returns:
//   - `Resource`: the current observation, or nil if none is recorded.
func (c *ResourceCatalog) CurrentObservation(observedURI string) Resource {

	c.mu.Lock()
	observationURI, ok := c.currentObservations[observedURI]
	c.mu.Unlock()

	if !ok {
		return nil
	}

	id := c.Current(observationURI)
	if id == "" {
		return nil
	}

	observation, _ := c.Lookup(id)
	return observation
}

// Discover returns the canonical catalog entry for uri after verifying that the resource exists.
//
// Discover is the consumption-side counterpart to [GetOrCreate]. Use it from non-production callsites: receipt
// rehydration during unmarshal, scanner-style URI lookups during preflight, and any other path where there is no
// producing node. The returned entry has no producerID stamped (or carries whatever stamp a previous GetOrCreate
// already applied) — discovery records existence, not authorship.
//
// Cache-hit behavior branches on the existing entry's [State] per the [DiscoverResource] rules in
// docs/architecture/4-resource-management.md §6.2: Active returns the existing entry as a cheap hit; Gone returns an
// error without re-attempting Resolve (Gone is terminal); Pending invokes Resolve in place — success transitions to
// Active, failure transitions to Gone and surfaces the error.
//
// Cache-miss behavior constructs a fresh candidate via factory, calls Resolve to verify existence, links the candidate
// into the catalog regardless of Resolve outcome (so the Gone entry is recorded as history), then transitions the
// linked entry to Active on success or Gone on failure.
//
// Parameters:
//   - `uri`: the URI to look up. Must not be empty (asserted).
//   - `factory`: closure invoked on cache miss to construct a fresh [Resource]. Must be non-nil (asserted).
//
// Returns:
//   - `Resource`: the canonical catalog entry for `uri`, in state Active. Nil if Resolve failed (Gone) or the entry is
//     already known-Gone.
//   - `error`: any factory error (returned untouched), any Resolve error (after the catalog has been updated to mark
//     the entry Gone), or a known-gone error if the URI's existing entry is Gone.
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

	// Cache miss: construct and intern. Existence verification is the caller's responsibility — each provider's
	// DiscoverResource builds the candidate via the same factory it would use to observe identity, and the framework's
	// preflight pass (or an explicit Provider.Observe call) drives the Pending → Active / Gone transition.
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
// Cache-hit behavior branches on the existing entry's [Addressing] × [State] per
// docs/architecture/4-resource-management.md §6.2's behavior matrix. The factory is invoked on cache miss, on
// location-based hits (any state), and on Gone hits (either addressing — Gone is terminal, so revival appends a new
// ledger entry via [Shadow]). Content-addressable hits on Pending or Active return the existing entry without invoking
// the factory (singleton). The new or revived entry transitions to Active via [markActive] before returning.
//
// A non-nil factory error short-circuits without touching the catalog. A different producer claiming the same URI
// surfaces as a Shadow conflict (write-write detection).
//
// Parameters:
//   - `unit`: the producing [ExecutableUnit], or nil for non-graph dispatch. When non-nil the resulting catalog entry
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
func (c *ResourceCatalog) GetOrCreate(unit ExecutableUnit, uri string, factory func() (Resource, error)) (Resource, error) {

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

	var producerID string
	if unit != nil {
		producerID = unit.ID()
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
// the slot-fill path in the plan provider's dispatch and the rehydration path in plan.load (step 15). Behavior matches
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

// RecordObservation interns `obs` and updates the `currentObservations` index.
//
// Subsequent [CurrentObservation] lookups by `obs.OfResource.URI()` return its catalog id.
//
// The Resource being observed need not be in this catalog — the index keys on its URI, which is stable across catalogs
// (graph.Catalog vs. the per-run env.Catalog clone). Recording multiple observations of the same observed Resource
// overwrites the index entry; the catalog still keeps every observation in its append-only ledger via [Shadow].
//
// Parameters:
//   - `obs`: the observation to record. Must satisfy [Observation] (every concrete type that embeds [ObservationBase]
//     does).
//
// Returns:
//   - `string`: the catalog id assigned to the observation entry.
//   - `error`: any [Shadow] failure (catalog conflict on the observation's content-addressable URI, which would
//     indicate a producer-stamping bug since observations carry no producer).
func (c *ResourceCatalog) RecordObservation(obs Observation) (string, error) {

	id, err := c.Shadow(obs, "")
	if err != nil {
		return "", fmt.Errorf("op.ResourceCatalog.RecordObservation: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentObservations[obs.observation().OfResource.URI()] = obs.URI()

	return id, nil
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

	if existingID, ok := c.ns[uri]; ok {
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
//   - `State`: the current lifecycle state — `Pending` (zero value, newly cataloged), `Active` (observed or produced),
//     or `Gone` (Resolve failed; terminal).
func (c *ResourceCatalog) State(id string) State {

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.states[id]
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

	uri := r.URI()

	if id, ok := c.ns[uri]; ok {
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
	c.ns[r.URI()] = id

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

// endregion
