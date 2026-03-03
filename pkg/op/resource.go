// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
)

// Resource represents a logical data item identified by a URI and tracked across distributed nodes.
//
// It serves as a reference entry in the ResourceLedger with origin tracking information.
type Resource struct {
	URI          string // logical address of the resource (e.g., a file URL)
	ID           string // unique identifier in the flat ResourceLedger
	OriginNodeID string // ID of the node that created this resource
}

// URI scheme constants.
const (
	SchemeFile    = "file"
	SchemeGit     = "git"
	SchemePackage = "pkg"
	SchemeService = "svc"
	SchemeMem     = "mem"
)

// ResourceURI builds a canonicalized URI from a scheme and path.
// For file:// URIs, path is resolved via filepath.Abs + filepath.Clean.
// Other schemes use the path as-is.
func ResourceURI(scheme, path string) string {
	if scheme == SchemeFile {
		abs, err := filepath.Abs(path)
		if err == nil {
			path = abs
		}
		path = filepath.Clean(path)
	}
	return fmt.Sprintf("%s://%s", scheme, path)
}

// ResourceManager owns the append-only ledger of all resources created
// during a single planning session. One per Graph.
type ResourceManager struct {
	mu      sync.Mutex
	entries []Resource     // append-only ledger; index = sequence number
	byID    map[string]int // resource ID → index in entries
	nextID  int            // monotonic counter
}

// NewResourceManager creates an empty resource manager.
func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		byID: make(map[string]int),
	}
}

// EnsureCataloged creates a new Resource in the ledger with a unique ID.
// Returns the new resource's ID.
func (m *ResourceManager) EnsureCataloged(uri string, originNodeID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	id := fmt.Sprintf("res-%d", m.nextID)
	r := Resource{
		URI:          uri,
		ID:           id,
		OriginNodeID: originNodeID,
	}
	m.byID[id] = len(m.entries)
	m.entries = append(m.entries, r)
	return id
}

// Lookup returns the Resource with the given ID, or false.
func (m *ResourceManager) Lookup(id string) (Resource, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx, ok := m.byID[id]
	if !ok {
		return Resource{}, false
	}
	return m.entries[idx], true
}

// LedgerLen returns the count of resources in the ledger.
func (m *ResourceManager) LedgerLen() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

// resourceType is the reflect.Type for Resource, used by extractResource.
var resourceType = reflect.TypeOf(Resource{})

// extractResource checks whether v carries resource identity and returns
// the embedded Resource if found.
//
// It handles three forms:
//   - Direct op.Resource value
//   - Go struct embedding op.Resource (e.g., file.Resource)
//   - map[string]any with "uri"/"id"/"origin_node_id" keys (produced by
//     Unmarshal when a starlarkstruct.Struct is decoded to *any)
func extractResource(v any) (Resource, bool) {
	if v == nil {
		return Resource{}, false
	}
	// Direct type match.
	if r, ok := v.(Resource); ok {
		return r, true
	}

	// map[string]any from Unmarshal of a starlark struct.
	if m, ok := v.(map[string]any); ok {
		// Direct Resource fields at top level (plain op.Resource).
		uri, _ := m["uri"].(string)
		id, _ := m["id"].(string)
		origin, _ := m["origin_node_id"].(string)
		if uri != "" || id != "" {
			return Resource{URI: uri, ID: id, OriginNodeID: origin}, true
		}
		// Embedded Resource (struct embedding op.Resource serializes as
		// a nested "resource" key).
		if nested, ok := m["resource"].(map[string]any); ok {
			uri, _ = nested["uri"].(string)
			id, _ = nested["id"].(string)
			origin, _ = nested["origin_node_id"].(string)
			if uri != "" || id != "" {
				return Resource{URI: uri, ID: id, OriginNodeID: origin}, true
			}
		}
		return Resource{}, false
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return Resource{}, false
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return Resource{}, false
	}

	// Walk the struct fields looking for an embedded Resource.
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Anonymous && f.Type == resourceType {
			res := rv.Field(i).Interface().(Resource)
			return res, true
		}
	}
	return Resource{}, false
}
