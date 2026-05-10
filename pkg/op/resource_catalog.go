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
// Three observable states, derived from an entry's producer and metadata:
//
//   - Unresolved: producerID == "", metadata empty. A discovery entry created when the planner first sees a URI
//     via [ResourceCatalog.Resolve]. The executor's preflight pass stats the target and populates metadata
//     in place.
//   - Pending: producerID != "", metadata empty. A shadow entry created when a node's Planned companion
//     constructs the identity of a resource the node will produce. [ResourceCatalog.Transition] populates its
//     metadata in place after the forward method runs.
//   - Resolved: metadata populated. Reached by preflight (from Unresolved) or by [ResourceCatalog.Transition]
//     (from Pending).
//
// The catalog does not expose a [State] enum. States are a property of the underlying resource; the catalog
// only tracks identity and lineage.
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
// Parameters:
//   - r: a typed resource with its URI set.
//
// Returns:
//   - Resource: the canonical entry for r's URI.
//   - string: the canonical entry's catalog ID.
func (c *ResourceCatalog) Resolve(r Resource) (Resource, string) {

	c.mu.Lock()
	defer c.mu.Unlock()

	uri := r.URI()

	if id, ok := c.ns[uri]; ok {
		if idx, ok := c.byID[id]; ok {
			return c.entries[idx], id
		}
	}

	id := c.catalogLocked(r, "")
	return r, id
}

// GetOrCreate returns the canonical catalog entry for uri, invoking factory only on cache miss.
//
// GetOrCreate is the consumer-side read-or-discover hook: callers that want catalog identity for a URI
// (an unmarshaler rehydrating a saved receipt, a scanner observing existing filesystem state, etc.)
// supply a factory that constructs a fresh [Resource] of the appropriate concrete type, and GetOrCreate
// either returns the existing entry or interns the factory's result via [ResourceCatalog.Link]. The
// catalog stays type-neutral; the factory closure resolves the concrete-type-to-construct decision at
// the call site, where the type is statically known.
//
// On cache hit the factory is not called — no allocation for the fresh candidate, no [NewResource]
// validation cost. On cache miss the factory runs once; its returned Resource flows through [Link]
// (which may further deduplicate if a competing call interned the same URI between the lookup and the
// Link). A non-nil factory error short-circuits without touching the catalog.
//
// GetOrCreate is the wrong tool for forward-method outputs — those flow through the plan-time
// [ResourceCatalog.Shadow] / post-dispatch [ResourceCatalog.Transition] path. Mixing the two write
// paths corrupts producerID tracking. Use GetOrCreate for read-or-discover; let the executor handle
// production.
//
// Parameters:
//   - activation: the per-dispatch [ActivationRecord] for the producing dispatch. Must be non-nil with a non-empty
//     `SiteID` — GetOrCreate is the production-side hook and asserts these invariants. Discovery callsites
//     (receipt rehydration, scanner-style URI lookups) must use [ResourceCatalog.Discover] instead.
//   - uri: the URI to look up. Must not be empty (asserted).
//   - factory: closure invoked on cache miss to construct a fresh [Resource]. Must be non-nil (asserted).
//
// Returns:
//   - Resource: the canonical catalog entry for uri.
//   - error: any factory error (returned untouched), or a [Shadow] error if a competing producer already claimed
//     the same URI.
//
// Panics with an [*assert.AssertionError] when any precondition is violated — these are programming errors at
// the call site, not runtime conditions.
func (c *ResourceCatalog) GetOrCreate(activation *ActivationRecord, uri string, factory func() (Resource, error)) (Resource, error) {

	assert.NotNil("activation", activation)
	assert.True("activation.SiteID not empty", activation.SiteID != "")
	assert.True("uri not empty", uri != "")
	assert.NotNil("factory", factory)

	if id := c.Current(uri); id != "" {
		if existing, ok := c.Lookup(id); ok {
			return existing, nil
		}
	}

	candidate, err := factory()
	if err != nil {
		return nil, err
	}

	if _, err := c.Shadow(candidate, activation.SiteID); err != nil {
		return nil, err
	}
	return candidate, nil
}

// Discover returns the canonical catalog entry for uri, invoking factory only on cache miss, without producer
// stamping. The discovery counterpart to [GetOrCreate].
//
// Use Discover from non-production callsites: receipt rehydration during unmarshal, scanner-style URI lookups
// during preflight, and any other path where there is no producing node. The returned entry has no `producerID`
// stamped (or carries whatever stamp a previous GetOrCreate already applied).
//
// Parameters:
//   - uri: the URI to look up. Must not be empty (asserted).
//   - factory: closure invoked on cache miss to construct a fresh [Resource]. Must be non-nil (asserted).
//
// Returns:
//   - Resource: the canonical catalog entry for uri.
//   - error: any factory error (returned untouched).
//
// Panics with an [*assert.AssertionError] when any precondition is violated.
func (c *ResourceCatalog) Discover(uri string, factory func() (Resource, error)) (Resource, error) {

	assert.True("uri not empty", uri != "")
	assert.NotNil("factory", factory)

	if id := c.Current(uri); id != "" {
		if existing, ok := c.Lookup(id); ok {
			return existing, nil
		}
	}

	candidate, err := factory()
	if err != nil {
		return nil, err
	}

	return c.Link(candidate), nil
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
// entries (empty producer) are silently superseded. Re-shadowing with the same producer is permitted so the
// executor's post-dispatch [ResourceCatalog.Transition] does not have to fight the conflict check.
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

// Transition fills the metadata of a pending entry with the metadata from the resolved resource returned by a
// forward method, in place.
//
// Called by the executor's post-dispatch pass after the forward method returns. The pending entry — created
// at plan time by [ResourceCatalog.Shadow] via the Planned companion — is located by resolved's URI. The
// producer must match: only the node that shadowed the URI may transition it. The catalog's identity fields
// (`id`, `producerID`) on the pending entry are preserved; every other field is overwritten by a struct copy
// from resolved via reflection.
//
// The mutation is in place: the interface value in the ledger and every outstanding pointer held by slots,
// promises, and the planner all observe the resolved metadata immediately. No new ledger entry is appended.
//
// Parameters:
//   - resolved: the fully-populated resource returned by the forward method. Its URI must match an existing
//     pending entry, and its concrete type must match the pending entry's concrete type.
//   - producerID: the node ID that claimed the URI at plan time. Must equal the pending entry's `producerID`.
//
// Returns:
//   - error: non-nil if the URI is unknown, the entry has been removed, the producer does not match, or the
//     concrete types differ.
func (c *ResourceCatalog) Transition(resolved Resource, producerID string) error {

	c.mu.Lock()
	defer c.mu.Unlock()

	if producerID == "" {
		return fmt.Errorf("transition: producerID must not be empty")
	}

	uri := resolved.URI()

	id, ok := c.ns[uri]
	if !ok {
		return fmt.Errorf("transition: no catalog entry for URI %q", uri)
	}

	idx, ok := c.byID[id]
	if !ok {
		return fmt.Errorf("transition: catalog id %q not in ledger", id)
	}

	existing := c.entries[idx]
	existingBase := existing.resourceBase()

	if existingBase.producerID == "" {
		return fmt.Errorf("transition: entry %q for URI %q is a discovery, not a pending shadow", id, uri)
	}

	if existingBase.producerID != producerID {
		return fmt.Errorf(
			"transition: producer mismatch for URI %q: entry owned by %q, transition requested by %q",
			uri, existingBase.producerID, producerID,
		)
	}

	existingVal := reflect.ValueOf(existing)
	resolvedVal := reflect.ValueOf(resolved)

	if existingVal.Kind() != reflect.Ptr || resolvedVal.Kind() != reflect.Ptr {
		return fmt.Errorf("transition: resources must be pointers, got existing=%T resolved=%T", existing, resolved)
	}

	if existingVal.Type() != resolvedVal.Type() {
		return fmt.Errorf("transition: type mismatch for URI %q: existing=%T resolved=%T", uri, existing, resolved)
	}

	// Preserve the catalog identity before the struct copy.
	preservedBase := *existingBase

	// In-place struct copy: mutates the concrete value behind the existing interface pointer. All outstanding
	// holders of the pointer (slots, promises, planner references) see the populated metadata immediately.
	existingVal.Elem().Set(resolvedVal.Elem())

	// Restore the catalog identity. The resolved resource from the forward method may not have the catalog's
	// id/producerID stamped (if it came from a fresh construction path); preserving them ensures that Lookup
	// by id continues to work and that shadowing lineage is not erased.
	*existingBase = preservedBase

	return nil
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
