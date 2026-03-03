// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// NamespaceMap maps URIs to the most recent resource ID during planning.
// One per Graph, used only during planning (not serialized).
type NamespaceMap struct {
	current map[string]string // URI → resource ID
}

// NewNamespaceMap creates an empty namespace.
func NewNamespaceMap() *NamespaceMap {
	return &NamespaceMap{
		current: make(map[string]string),
	}
}

// Resolve returns the current resource ID for a URI. If the URI has
// never been seen, catalogs a discovery in the manager (originNodeID = "")
// and returns the new ID.
func (ns *NamespaceMap) Resolve(mgr *ResourceManager, uri string) string {
	if id, ok := ns.current[uri]; ok {
		return id
	}
	id := mgr.EnsureCataloged(uri, "")
	ns.current[uri] = id
	return id
}

// Shadow creates a new resource version in the manager, updates the
// namespace to point to it, and returns the new resource ID.
func (ns *NamespaceMap) Shadow(mgr *ResourceManager, uri string, producerNodeID string) string {
	id := mgr.EnsureCataloged(uri, producerNodeID)
	ns.current[uri] = id
	return id
}

// Current returns the resource ID currently mapped to a URI, or "".
func (ns *NamespaceMap) Current(uri string) string {
	return ns.current[uri]
}
