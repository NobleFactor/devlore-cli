// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

// rawDriver is the host-specific mechanism a leaf supplies: its identity plus the shell-out primitives.
//
// The primitives are split across build-tagged files — real on the manager's native host (`*_linux.go`,
// `*_darwin.go`, `*_windows.go`), stubbed everywhere else (`*_other.go`). The [driver] wrapper turns these
// primitives into the full [PackageManager] surface, so a concrete manager implements the whole contract by
// supplying only this small set.
type rawDriver interface {
	name() string
	purlType() string
	installed(name string) bool
	version(name string) string
	available(name string) bool
	searchRaw(query string, limit int) []SearchResult
	installRaw(names []string, kwargs map[string]any) PlatformResult
	removeRaw(names []string) PlatformResult
}

// driver adapts a [rawDriver] into a full [PackageManager] / [leaf].
//
// Concrete managers embed driver by value and wire its [rawDriver] back to themselves at construction (via
// [newDriver]), so the promoted verb and query methods dispatch to the manager's own host-specific primitives.
// Install and Upgrade share installRaw (converge to present); Remove uses removeRaw (converge to absent); every
// verb verifies its outcome by re-query through [bracket]. The query and Search methods route straight to the
// primitives, with Search tagging each hit with the manager's purl type.
type driver struct {
	rawDriver
}

// newDriver wires a [driver] to its concrete [rawDriver].
//
// Parameters:
//   - `raw`: the concrete leaf supplying the host-specific primitives (the embedding manager itself).
//
// Returns:
//   - `driver`: the wired wrapper.
func newDriver(raw rawDriver) driver {
	return driver{rawDriver: raw}
}

// region EXPORTED METHODS

// region Behaviors

// Available reports whether the package identified by `p` exists in the manager's index.
//
// Parameters:
//   - `p`: the package [PURL] to query.
//
// Returns:
//   - `bool`: true when the package is available to install.
func (d driver) Available(p PURL) bool { return d.available(d.tokenFor(p)) }

// Install converges each package to present, verifying the outcome by re-query.
//
// Parameters:
//   - `packages`: the packages to install, each carrying its resolved [PURL].
//   - `kwargs`: opaque native-installer flags passed through to the primitive.
//
// Returns:
//   - `[]Receipt`: one receipt per package, in input order.
//   - `error`: non-nil when any receipt failed.
func (d driver) Install(packages []PURL, kwargs map[string]any) ([]Receipt, error) {
	return bracket(packages, d.tokenFor, d.version, func(names []string) PlatformResult { return d.installRaw(names, kwargs) }, present)
}

// Installed reports whether the package identified by `p` is installed.
//
// Parameters:
//   - `p`: the package [PURL] to query.
//
// Returns:
//   - `bool`: true when the package is installed.
func (d driver) Installed(p PURL) bool { return d.installed(d.tokenFor(p)) }

// Remove converges each package to absent, verifying the outcome by re-query.
//
// Parameters:
//   - `packages`: the packages to remove, each carrying its resolved [PURL].
//   - `kwargs`: opaque native-installer flags passed through to the primitive.
//
// Returns:
//   - `[]Receipt`: one receipt per package, in input order.
//   - `error`: non-nil when any receipt failed.
func (d driver) Remove(packages []PURL, kwargs map[string]any) ([]Receipt, error) {
	return bracket(packages, d.tokenFor, d.version, func(names []string) PlatformResult { return d.removeRaw(names) }, absent)
}

// Search returns up to `limit` matches for `query`, each tagged with the manager's purl type.
//
// Parameters:
//   - `query`: the search term.
//   - `limit`: the maximum number of results; <= 0 means no limit.
//
// Returns:
//   - `[]SearchResult`: the matches, each tagged with the manager's purl type; nil when none match.
func (d driver) Search(query string, limit int) []SearchResult {
	return tagManager(d.searchRaw(query, limit), d.purlType())
}

// Upgrade converges each package to the latest available version (via installRaw), verifying by re-query.
//
// Parameters:
//   - `packages`: the packages to upgrade, each carrying its resolved [PURL].
//   - `kwargs`: opaque native-installer flags passed through to the primitive.
//
// Returns:
//   - `[]Receipt`: one receipt per package, in input order.
//   - `error`: non-nil when any receipt failed.
func (d driver) Upgrade(packages []PURL, kwargs map[string]any) ([]Receipt, error) {
	return bracket(packages, d.tokenFor, d.version, func(names []string) PlatformResult { return d.installRaw(names, kwargs) }, present)
}

// Version returns the installed version of the package identified by `p`, or "" when absent.
//
// Parameters:
//   - `p`: the package [PURL] to query.
//
// Returns:
//   - `string`: the installed version, or "" when absent.
func (d driver) Version(p PURL) string { return d.version(d.tokenFor(p)) }

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// tokenFor derives the native install token for `p`.
//
// Most managers address a package by its purl name; managers whose native identifier folds in the publisher
// (winget) implement [namespacer] to override this. The default is `p.Name`.
//
// Parameters:
//   - `p`: the package whose native token to derive.
//
// Returns:
//   - `string`: the native install token (e.g. "curl", or "Microsoft.VisualStudioCode" for winget).
func (d driver) tokenFor(p PURL) string {
	if n, ok := d.rawDriver.(namespacer); ok {
		return n.token(p)
	}
	return p.Name
}

// endregion

// endregion

// region SUPPORTING TYPES

// namespacer is implemented by leaves whose native package token folds in the purl namespace.
//
// Winget is the example ("Publisher.Name"). Leaves that address packages by bare name do not implement it;
// [driver.tokenFor] then defaults to the purl name.
type namespacer interface {
	token(p PURL) string
}

// endregion
