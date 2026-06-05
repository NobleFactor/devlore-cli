// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"fmt"
	"sync"
)

// leaf is a [PackageManager] that also exposes its identity for the router's routing table.
//
// `name` is the user-facing manager name (the pkg.Resource prefix, e.g. "apt"); `purlType` is the purl type that
// keys routing and appears in resource URIs (e.g. "deb"). They coincide for the extension managers (brew, port,
// winget, snap, flatpak) and diverge for the native Linux managers — apt→deb, dnf→rpm, pacman→alpm.
type leaf interface {
	PackageManager
	name() string
	purlType() string
}

// compositeManager is a platform's Composite router over its leaf drivers, and is itself a [PackageManager].
//
// A verb groups the incoming `[]PURL` by `Purl.Type`, fans each slice out to the leaf registered for that type
// concurrently, and concatenates the leaves' receipts into one unified result preserving input order. An unknown
// purl type fails only that package's receipt. Queries route by `Purl.Type` to the single owning leaf; Search fans
// out across every leaf, each hit already tagged with its manager.
type compositeManager struct {
	byType      map[string]leaf   // purl type → owning leaf driver (the routing table)
	nameToType  map[string]string // manager name → purl type (prefix resolution)
	defaultType string            // the default native manager's purl type
}

// newComposite builds a [compositeManager] from a platform's leaves and its default native leaf.
//
// Parameters:
//   - `leaves`: the platform's leaf drivers, keyed into the routing table by purl type.
//   - `defaultLeaf`: the default native leaf whose purl type normalizes bare package names; may be nil.
//
// Returns:
//   - `*compositeManager`: the constructed router.
func newComposite(leaves []leaf, defaultLeaf leaf) *compositeManager {

	byType := make(map[string]leaf, len(leaves))
	nameToType := make(map[string]string, len(leaves))

	for _, l := range leaves {
		byType[l.purlType()] = l
		nameToType[l.name()] = l.purlType()
	}

	router := &compositeManager{byType: byType, nameToType: nameToType}

	if defaultLeaf != nil {
		router.defaultType = defaultLeaf.purlType()
	}

	return router
}

// region EXPORTED METHODS

// region Behaviors

// Available reports whether the package identified by `p` exists in its manager's index, routing by `p.Type`.
//
// Parameters:
//   - `p`: the package [PURL] to query.
//
// Returns:
//   - `bool`: true when the owning leaf reports it available; false when unavailable or the type is unknown.
func (c *compositeManager) Available(p PURL) bool {
	if l, ok := c.byType[p.Type]; ok {
		return l.Available(p)
	}
	return false
}

// Install routes each package to its leaf and converges it to present at the requested version.
//
// Parameters:
//   - `packages`: the packages to install, each carrying its resolved [PURL].
//   - `kwargs`: opaque native-installer flags passed through to each leaf.
//
// Returns:
//   - `[]Receipt`: one receipt per package in input order.
//   - `error`: non-nil when any receipt failed.
func (c *compositeManager) Install(packages []PURL, kwargs map[string]any) ([]Receipt, error) {
	return c.dispatch(packages, kwargs, leaf.Install)
}

// Installed reports whether the package identified by `p` is installed, routing by `p.Type`.
//
// Parameters:
//   - `p`: the package [PURL] to query.
//
// Returns:
//   - `bool`: true when the owning leaf reports it installed; false when installed nowhere or the type is unknown.
func (c *compositeManager) Installed(p PURL) bool {
	if l, ok := c.byType[p.Type]; ok {
		return l.Installed(p)
	}
	return false
}

// Remove routes each package to its leaf and converges it to absent.
//
// Parameters:
//   - `packages`: the packages to remove, each carrying its resolved [PURL].
//   - `kwargs`: opaque native-installer flags passed through to each leaf.
//
// Returns:
//   - `[]Receipt`: one receipt per package in input order.
//   - `error`: non-nil when any receipt failed.
func (c *compositeManager) Remove(packages []PURL, kwargs map[string]any) ([]Receipt, error) {
	return c.dispatch(packages, kwargs, leaf.Remove)
}

// Search fans `query` out across every leaf and concatenates the results.
//
// Parameters:
//   - `query`: the search term.
//   - `limit`: the maximum number of results per leaf; `limit` <= 0 means no limit.
//
// Returns:
//   - `[]SearchResult`: the union of the leaves' matches, each tagged with its `Manager`.
func (c *compositeManager) Search(query string, limit int) []SearchResult {

	var results []SearchResult

	for _, l := range c.byType {
		results = append(results, l.Search(query, limit)...)
	}

	return results
}

// Upgrade routes each package to its leaf and moves it to the latest available version.
//
// Parameters:
//   - `packages`: the packages to upgrade, each carrying its resolved [PURL].
//   - `kwargs`: opaque native-installer flags passed through to each leaf.
//
// Returns:
//   - `[]Receipt`: one receipt per package in input order.
//   - `error`: non-nil when any receipt failed.
func (c *compositeManager) Upgrade(packages []PURL, kwargs map[string]any) ([]Receipt, error) {
	return c.dispatch(packages, kwargs, leaf.Upgrade)
}

// Version returns the installed version of the package identified by `p`, routing by `p.Type`.
//
// Parameters:
//   - `p`: the package [PURL] to query.
//
// Returns:
//   - `string`: the installed version, or "" when absent or the type is unknown.
func (c *compositeManager) Version(p PURL) string {
	if l, ok := c.byType[p.Type]; ok {
		return l.Version(p)
	}
	return ""
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// dispatch routes `packages` to their owning leaves, runs `verb` concurrently, and reassembles receipts in order.
//
// dispatch groups packages by purl type. An unknown purl type fails only that package's receipt; the rest still
// run. The aggregate error is the first failing receipt's error.
//
// Parameters:
//   - `packages`: the packages to route.
//   - `kwargs`: opaque native-installer flags forwarded to each leaf.
//   - `verb`: the leaf method to invoke (Install / Remove / Upgrade).
//
// Returns:
//   - `[]Receipt`: one receipt per package in input order.
//   - `error`: the first failing receipt's error, or nil when every package succeeded.
func (c *compositeManager) dispatch(packages []PURL, kwargs map[string]any, verb func(leaf, []PURL, map[string]any) ([]Receipt, error)) ([]Receipt, error) {

	type group struct {
		owner   leaf
		indices []int
		purls   []PURL
	}

	groups := make(map[string]*group)
	receipts := make([]Receipt, len(packages))

	var firstErr error

	for i, p := range packages {

		owner, ok := c.byType[p.Type]
		if !ok {
			receipts[i] = Receipt{Purl: p, Err: fmt.Errorf("platform: no package manager for purl type %q (package %q)", p.Type, p.Name)}
			if firstErr == nil {
				firstErr = receipts[i].Err
			}
			continue
		}

		g := groups[p.Type]
		if g == nil {
			g = &group{owner: owner}
			groups[p.Type] = g
		}

		g.indices = append(g.indices, i)
		g.purls = append(g.purls, p)
	}

	var (
		wg    sync.WaitGroup
		mutex sync.Mutex
	)

	for _, g := range groups {

		wg.Add(1)

		go func(g *group) {

			defer wg.Done()

			leafReceipts, err := verb(g.owner, g.purls, kwargs)

			mutex.Lock()
			defer mutex.Unlock()

			for j, idx := range g.indices {
				if j < len(leafReceipts) {
					receipts[idx] = leafReceipts[j]
				} else {
					receipts[idx] = Receipt{Purl: g.purls[j]}
				}
			}

			if err != nil && firstErr == nil {
				firstErr = err
			}
		}(g)
	}

	wg.Wait()

	return receipts, firstErr
}

// resolveType maps a caller-supplied prefix — a manager name or a purl type — to the canonical purl type.
//
// Parameters:
//   - `prefix`: the manager prefix from a pkg.Resource identifier (e.g. "apt", "deb", "brew").
//
// Returns:
//   - `string`: the canonical purl type the prefix resolves to.
//   - `bool`: true when the prefix names a known manager or type.
func (c *compositeManager) resolveType(prefix string) (string, bool) {

	if _, ok := c.byType[prefix]; ok {
		return prefix, true
	}

	if purlType, ok := c.nameToType[prefix]; ok {
		return purlType, true
	}

	return "", false
}

// endregion

// endregion
