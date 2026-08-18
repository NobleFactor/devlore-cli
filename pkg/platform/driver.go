// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package platform

import (
	"fmt"
	"os/exec"
	"time"
)

// rawDriver is the host-specific mechanism a leaf supplies: its identity plus the shell-out primitives.
//
// The primitives are split across build-tagged files — real on the manager's native host (`*_linux.go`,
// `*_darwin.go`, `*_windows.go`), stubbed everywhere else (`*_other.go`). The [driver] wrapper turns these
// primitives into the full [PackageManager] surface, so a concrete manager implements the whole contract by
// supplying only this small set.
type rawDriver interface {
	name() string
	purlType() string
	executable() string
	installed(name string) bool
	version(name string) string
	available(name string) bool
	searchRaw(query string, limit int) []SearchResult
	installRaw(names []string, kwargs map[string]any) Result
	removeRaw(names []string) Result
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
func (d driver) Available(p PURL) bool {
	d.ensureIndex()
	return d.available(d.tokenFor(p))
}

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
	d.ensureFresh()
	return bracket(packages, d.tokenFor, d.version, func(names []string) Result { return d.installRaw(names, kwargs) }, present)
}

// Installed reports whether the package identified by `p` is installed.
//
// Parameters:
//   - `p`: the package [PURL] to query.
//
// Returns:
//   - `bool`: true when the package is installed.
func (d driver) Installed(p PURL) bool { return d.installed(d.tokenFor(p)) }

// Present reports whether this manager's executable is on the PATH.
//
// A PATH lookup and nothing more — no subprocess, no index, no network — so it is cheap enough to call per
// resolution and safe to call from a test.
//
// Returns:
//   - `bool`: true when the executable resolves on the PATH.
func (d driver) Present() bool {
	_, err := exec.LookPath(d.executable())
	return err == nil
}

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
	return bracket(packages, d.tokenFor, d.version, d.removeRaw, absent)
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
	d.ensureIndex()
	return tagManager(d.searchRaw(query, limit), d.purlType())
}

// Update forces an immediate index refresh, bypassing the staleness gate.
//
// Refresh is the manager's concern: a leaf that maintains a local index implements [refresher]; one that queries a
// live store (snap, flatpak, winget) does not, and Update is a no-op for it. The same primitive backs the automatic
// staleness-gated refresh.
//
// Returns:
//   - `error`: non-nil when the refresh command failed; nil when it succeeded or the manager has no index to refresh.
func (d driver) Update() error {

	r, ok := d.rawDriver.(refresher)
	if !ok {
		return nil
	}

	if result := r.refresh(); !result.OK {
		return fmt.Errorf("%s: index refresh failed: %s", d.name(), result.Stderr)
	}

	return nil
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
	d.ensureFresh()
	return bracket(packages, d.tokenFor, d.version, func(names []string) Result { return d.installRaw(names, kwargs) }, present)
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

// ensureFresh refreshes the leaf's index before a mutating operation when it is stale or missing.
//
// Freshness is load-bearing for [driver.Install] and [driver.Upgrade] and for nothing else: both resolve a specific
// version, so an index that has fallen behind installs a superseded build or fetches one the mirror has since
// withdrawn. "Latest", for Upgrade, is a claim the index alone defines.
func (d driver) ensureFresh() {
	d.refreshIndexWhen(func(age time.Duration) bool { return age > refreshTTL })
}

// ensureIndex refreshes the leaf's index before a query only when there is no index to read.
//
// A query asks whether a package exists ([driver.Available]) or what matches a term ([driver.Search]); neither
// carries a version — see [driver.tokenFor] — so neither is version-sensitive, and age costs them almost nothing.
// Absence costs them everything: with no index, Available reports false for every package on the machine, which
// reads exactly like an authoritative "no such package".
//
// The distinction is not academic. A CI runner's image ships with the package lists cleaned, so an implicit
// staleness refresh here turned a read into a multi-minute network fetch — long enough to blow a test deadline,
// and long enough to strand anyone running an interactive search.
func (d driver) ensureIndex() {
	d.refreshIndexWhen(indexIsMissing)
}

// refreshIndexWhen refreshes the leaf's index when `needed` accepts its current age.
//
// The shared mechanism behind both gates. A leaf participates only when it is both a [refresher] and
// [stalenessAware]; brew and dnf self-manage their freshness and implement the former without the latter, so they
// are never gated automatically. The refresh is best-effort — a failure is non-fatal and the operation proceeds
// against whatever index exists. A successful refresh updates the index mtime, so a batch of operations triggers at
// most one.
//
// Parameters:
//   - `needed`: predicate over the leaf's index age deciding whether to refresh.
func (d driver) refreshIndexWhen(needed func(age time.Duration) bool) {

	r, ok := d.rawDriver.(refresher)
	if !ok {
		return
	}

	s, ok := d.rawDriver.(stalenessAware)
	if !ok {
		return
	}

	if needed(s.indexAge()) {
		_ = r.refresh()
	}
}

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

// refresher is implemented by leaves that maintain a local package index they can refresh.
//
// Managers with a refreshable index (apt, dnf, pacman, brew, port) implement it; live-store managers (snap,
// flatpak, winget) do not, and [driver.Update] is a no-op for them. The same primitive powers both the manual
// force-refresh and the automatic staleness-gated refresh.
type refresher interface {
	refresh() Result
}

// stalenessAware is implemented by refresher leaves that can report their local index's age, enabling the automatic
// staleness-gated refresh ([driver.ensureFresh]). A refresher without it — brew and dnf self-manage their freshness,
// and port's ports-tree age is not yet detected — is refreshed only by a manual [driver.Update], never by the gate.
type stalenessAware interface {
	indexAge() time.Duration
}

// endregion
