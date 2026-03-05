// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"reflect"
	"sync"
)

// ResourceCatalog is the append-only catalog of all resources created during
// a single planning session. One per Graph. It owns the ledger (the log of
// all resource versions) and the namespace (URI → current resource ID).
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

// Resolve returns the current resource ID for a URI. If the URI has never
// been seen, a discovery entry ([ResourceBase] with URI only, no origin)
// is created and cataloged.
func (c *ResourceCatalog) Resolve(uri string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if id, ok := c.ns[uri]; ok {
		return id
	}
	base := NewResourceBase(uri)
	return c.catalogLocked(&base, "")
}

// Shadow catalogs a new resource version, updates the namespace to point to
// it, and returns the new resource ID. The resource must embed [ResourceBase]
// with its URI already set via [NewResourceBase].
func (c *ResourceCatalog) Shadow(r Resource, originID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.catalogLocked(r, originID)
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

// DiscoveryURIs returns the URIs of catalog entries that were discovered
// (created by Resolve) but not yet shadowed. These are inputs that should
// exist on the target machine before execution begins. URIs whose current
// entry has been superseded by Shadow are excluded.
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

// catalogLocked adds a resource to the ledger, stamps its id and originID,
// updates the namespace, and returns the assigned ID. Caller must hold c.mu.
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

// extractResource checks whether v carries resource identity and returns
// the originID if found. It is used by [FillSlot] to create implicit edges
// when a resource produced by one node flows to another.
//
// It handles three forms:
//   - Values implementing the [Resource] interface (pointer types)
//   - Struct values whose pointer type implements [Resource] (value types
//     returned by provider methods, stamped by [shadowResult])
//   - map[string]any with "uri"/"id"/"origin_id" keys (produced by
//     unmarshal when a starlarkstruct.Struct is decoded to *any)
func extractResource(v any) (originID string, ok bool) {
	if v == nil {
		return "", false
	}

	// Interface match — pointer types.
	if r, ok := v.(Resource); ok {
		base := r.resourceBase()
		return base.originID, base.originID != ""
	}

	// Struct value whose pointer satisfies Resource. Provider methods
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
