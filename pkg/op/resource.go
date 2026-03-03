// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"path/filepath"
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
