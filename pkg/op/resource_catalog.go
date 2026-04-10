// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"sync"
)

// resourceType is cached for result-type classification.
var resourceType = reflect.TypeOf((*Resource)(nil)).Elem()

// ResourceCatalog is the append-only catalog of all resources created during a single planning session.
//
// It owns the ledger (the log of all resource versions) and the namespace (URI → current resource ID). There is one
// [ResourceCatalog] per [Graph].
type ResourceCatalog struct {
	mu      sync.Mutex
	entries []Resource        // append-only ledger
	byID    map[string]int    // resource ID → index in entries
	ns      map[string]string // URI → current resource ID (namespace)
	nextID  int               // monotonic counter
}

// NewResourceCatalog creates an empty catalog.
func NewResourceCatalog() *ResourceCatalog {
	return &ResourceCatalog{
		byID: make(map[string]int),
		ns:   make(map[string]string),
	}
}

// Resolve returns the canonical resource for the given resource's URI, along with its catalog ID.
//
// If the URI has never been seen, r is cataloged as a discovery entry (no origin) and returned as-is.
// If the URI was previously cataloged — either as a discovery or shadowed by a producer — the canonical entry is
// returned and r is discarded. Callers should use the returned Resource, not the one they passed in, to ensure they
// see the authoritative version.
//
// Resolve is the link-time lookup operation: a caller that has type-tagged a string into a typed resource uses this
// method to get the catalog's canonical instance for that identity. It never fabricates placeholder entries — the
// caller always provides a real, typed resource.
func (c *ResourceCatalog) Resolve(r Resource) (Resource, string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if id, ok := c.ns[r.URI()]; ok {
		if idx, ok := c.byID[id]; ok {
			return c.entries[idx], id
		}
	}
	id := c.catalogLocked(r, "")
	return r, id
}

// Shadow catalogs a new resource version, updates the namespace to point to it, and returns the new resource ID.
//
// The resource must embed [ResourceBase] with its URI already set via [NewResourceBase].
//
// Write-write conflict detection:
//
// If the URI is already shadowed by a different origin (non-empty originID), Shadow returns an error. This catches
// plan-time conflicts where two nodes target the same output. Discovery entries (originID == "") are silently
// superseded.
func (c *ResourceCatalog) Shadow(r Resource, originID string) (string, error) {

	c.mu.Lock()
	defer c.mu.Unlock()

	// Detect write-write conflicts.

	if originID != "" {
		uri := r.URI()
		if existingID, ok := c.ns[uri]; ok {
			if idx, ok := c.byID[existingID]; ok {
				existingOrigin := c.entries[idx].resourceBase().originID
				if existingOrigin != "" && existingOrigin != originID {
					return "", fmt.Errorf(
						"resource conflict: URI %q is targeted by both %q and %q",
						uri, existingOrigin, originID,
					)
				}
			}
		}
	}

	return c.catalogLocked(r, originID), nil
}

// Lookup returns the resource with the given ID, or false if not found.
func (c *ResourceCatalog) Lookup(id string) (Resource, bool) {

	c.mu.Lock()
	defer c.mu.Unlock()

	idx, ok := c.byID[id]
	if !ok {
		return nil, false
	}
	return c.entries[idx], true
}

// Len returns the count of resources in the catalog.
func (c *ResourceCatalog) Len() int {

	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.entries)
}

// Current returns the current resource ID for a URI, or "".
func (c *ResourceCatalog) Current(uri string) string {

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ns[uri]
}

// DiscoveryURIs returns the URIs of catalog entries that were discovered (created by Resolve) but not yet shadowed.
//
// These are inputs that should exist on the target machine before execution begins. URIs whose current entry has been
// superseded by Shadow are excluded.
func (c *ResourceCatalog) DiscoveryURIs() []string {

	c.mu.Lock()
	defer c.mu.Unlock()

	var uris []string
	for uri, id := range c.ns {
		idx, ok := c.byID[id]
		if !ok {
			continue
		}
		base := c.entries[idx].resourceBase()
		if base.originID == "" {
			uris = append(uris, uri)
		}
	}
	return uris
}

// ExtractResource checks whether v carries resource identity and returns the originID if found.
//
// It is used by [FillSlot] to create implicit edges when a resource produced by one node flows to another.
//
// It handles three forms:
//   - Values implementing the [Resource] interface (pointer receiverTypes) - Struct values whose pointer type implements
//     [Resource] (value receiverTypes returned by provider methods, stamped by [shadowResult])
//   - map[string]any with "uri"/"id"/"origin_id" keys (produced by unmarshal when a starlarkstruct.Struct is decoded to
//     *any)
func ExtractResource(v any) (originID string, ok bool) {

	if v == nil {
		return "", false
	}

	// Interface match — pointer receiverTypes.
	if r, ok := v.(Resource); ok {
		base := r.resourceBase()
		return base.originID, base.originID != ""
	}

	// Struct value whose pointer satisfies Resource. ReceiverType methods
	// return resources by value; shadowResult stamps id/originID on the
	// embedded ResourceBase. Create a temporary pointer to access it.
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Struct && reflect.PointerTo(rv.Type()).Implements(resourceType) {
		ptr := reflect.New(rv.Type())
		ptr.Elem().Set(rv)
		r := ptr.Interface().(Resource)
		base := r.resourceBase()
		return base.originID, base.originID != ""
	}

	// map[string]any from Unmarshal of a starlark struct.
	if m, ok := v.(map[string]any); ok {
		origin, _ := m["origin_id"].(string)
		if origin != "" {
			return origin, true
		}
		// Check nested "resource_base" key for embedded ResourceBase.
		if nested, ok := m["resource_base"].(map[string]any); ok {
			origin, _ = nested["origin_id"].(string)
			if origin != "" {
				return origin, true
			}
		}
		return "", false
	}

	return "", false
}

// catalogLocked adds a resource to the ledger, stamps its id and originID, updates the namespace, and returns the
// assigned ID. Caller must hold c.mu.
func (c *ResourceCatalog) catalogLocked(r Resource, originID string) string {

	c.nextID++
	id := fmt.Sprintf("res-%d", c.nextID)

	base := r.resourceBase()
	base.id = id
	base.originID = originID

	c.byID[id] = len(c.entries)
	c.entries = append(c.entries, r)
	c.ns[r.URI()] = id
	return id
}
