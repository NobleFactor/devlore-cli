// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

// PackageManager is the Composite package-management contract.
//
// The same surface serves a single leaf driver and the platform's router over many leaves.
// Routing is by purl: every package reaching a manager is a [PURL] whose `Type` selects the leaf (the router
// dispatches; a leaf ignores routing and acts on whatever slice it is handed). The mutating triad —
// Install / Remove / Upgrade — is best-effort across its slice and returns one [Receipt] per package; partial
// failure is normal. Index refresh is automatic and staleness-gated per leaf before index-consuming operations;
// [PackageManager.Update] is the manual force-refresh override (the router fans it out to every leaf). The query
// methods report a single package's observed state and back the veneer's predicates (Installed / Version /
// Available) and federated search.
type PackageManager interface {

	// Install converges each package to present at the requested version.
	//
	// Parameters:
	//   - `packages`: the packages to install, each carrying its resolved [PURL].
	//   - `kwargs`: opaque native-installer flags passed through verbatim per manager (e.g. `cask`).
	//
	// Returns:
	//   - `receipts`: one [Receipt] per package, in input order; a receipt's `Err` is set from the observed
	//     post-state, not the command's exit code.
	//   - `err`: non-nil when any receipt failed.
	Install(packages []PURL, kwargs map[string]any) (receipts []Receipt, err error)

	// Remove converges each package to absent.
	//
	// Parameters:
	//   - `packages`: the packages to remove, each carrying its resolved [PURL].
	//   - `kwargs`: opaque native-installer flags passed through verbatim per manager.
	//
	// Returns:
	//   - `receipts`: one [Receipt] per package, in input order.
	//   - `err`: non-nil when any receipt failed.
	Remove(packages []PURL, kwargs map[string]any) (receipts []Receipt, err error)

	// Upgrade moves each named package to the latest available version.
	//
	// Parameters:
	//   - `packages`: the packages to upgrade, each carrying its resolved [PURL].
	//   - `kwargs`: opaque native-installer flags passed through verbatim per manager.
	//
	// Returns:
	//   - `receipts`: one [Receipt] per package, in input order.
	//   - `err`: non-nil when any receipt failed.
	Upgrade(packages []PURL, kwargs map[string]any) (receipts []Receipt, err error)

	// Update forces an immediate index refresh, bypassing the automatic staleness gate.
	//
	// On a leaf it refreshes that leaf's index now — a no-op for a manager with no local index (a live-store
	// manager such as snap / flatpak / winget). On the router it fans out to every leaf.
	//
	// Returns:
	//   - `error`: aggregated per-leaf refresh failures; nil when every refresh succeeded or had nothing to do.
	Update() error

	// Installed reports whether the package identified by `p` is installed.
	//
	// Parameters:
	//   - `p`: the package [PURL] to query.
	//
	// Returns:
	//   - `bool`: true when the package is installed.
	Installed(p PURL) bool

	// Version returns the installed version of the package identified by `p`, or empty when it is not installed.
	//
	// Parameters:
	//   - `p`: the package [PURL] to query.
	//
	// Returns:
	//   - `string`: the installed version, or "" when absent.
	Version(p PURL) string

	// Available reports whether the package identified by `p` exists in the manager's index.
	//
	// Parameters:
	//   - `p`: the package [PURL] to query.
	//
	// Returns:
	//   - `bool`: true when the package is available to install.
	Available(p PURL) bool

	// Search returns up to `limit` packages whose name or description matches `query`.
	//
	// Parameters:
	//   - `query`: the search term.
	//   - `limit`: the maximum number of results; `limit` <= 0 means no limit.
	//
	// Returns:
	//   - `[]SearchResult`: the matches, each tagged with the `Manager` that produced it; nil when none match.
	Search(query string, limit int) []SearchResult
}

// ServiceManager abstracts service-management operations.
//
// Concrete implementations exist for systemd (Linux), launchd (Darwin), and Service Control Manager (Windows).
type ServiceManager interface {

	// Exists reports whether a service with the given name is registered.
	//
	// Parameters:
	//   - `name`: the service name.
	//
	// Returns:
	//   - `bool`: true when the service exists.
	Exists(name string) bool

	// IsRunning reports whether the named service is currently running.
	//
	// Parameters:
	//   - `name`: the service name.
	//
	// Returns:
	//   - `bool`: true when the service is running.
	IsRunning(name string) bool

	// IsEnabled reports whether the named service is enabled to start at boot.
	//
	// Parameters:
	//   - `name`: the service name.
	//
	// Returns:
	//   - `bool`: true when the service is enabled.
	IsEnabled(name string) bool

	// Status returns a coarse, human-facing status for the named service.
	//
	// Parameters:
	//   - `name`: the service name.
	//
	// Returns:
	//   - `string`: the status (e.g. "running", "stopped").
	Status(name string) string

	// Start starts the named service.
	//
	// Parameters:
	//   - `name`: the service name.
	//
	// Returns:
	//   - `PlatformResult`: the command result.
	Start(name string) PlatformResult

	// Stop stops the named service.
	//
	// Parameters:
	//   - `name`: the service name.
	//
	// Returns:
	//   - `PlatformResult`: the command result.
	Stop(name string) PlatformResult

	// Enable enables the named service to start at boot.
	//
	// Parameters:
	//   - `name`: the service name.
	//
	// Returns:
	//   - `PlatformResult`: the command result.
	Enable(name string) PlatformResult

	// Disable disables the named service from starting at boot.
	//
	// Parameters:
	//   - `name`: the service name.
	//
	// Returns:
	//   - `PlatformResult`: the command result.
	Disable(name string) PlatformResult

	// NeedsSudo reports whether mutating service operations require elevation.
	//
	// Returns:
	//   - `bool`: true when elevation is required.
	NeedsSudo() bool
}

// region SUPPORTING TYPES

// Receipt records the outcome of one package operation.
//
// State is observed by re-query, never by screen-scraping command output: a leaf pre-queries the installed
// version, runs the (idempotent) command, then re-queries — `PriorVersion` and `Version` are those two
// observations, and `Err` is set when the post-state did not reach what the package's [PURL] requested. One
// Receipt is produced per package; the Composite router concatenates the leaves' receipts into one unified result.
type Receipt struct {

	// Purl is the package the operation acted on; its `Type` identifies the leaf that handled it.
	Purl PURL

	// PriorVersion is the installed version observed before the operation, or "" when the package was absent.
	PriorVersion string

	// Version is the installed version observed after the operation, or "" when the package is absent (removed).
	Version string

	// Err is non-nil when the observed post-state did not reach what the package's [PURL] requested.
	Err error
}

// PlatformResult represents a command-execution result.
//
// It is returned by a leaf's raw shell-out primitives and by [ServiceManager] mutators.
type PlatformResult struct {
	OK     bool   // whether the command exited zero
	Stdout string // captured standard output, trailing newline trimmed
	Stderr string // captured standard error, trailing newline trimmed
	Code   int    // the process exit code (-1 when the command could not be launched)
}

// SearchResult represents a package found by [PackageManager.Search].
//
// Manager records the leaf that produced the hit (the purl type, e.g. "deb", "brew"), so a federated search across
// the router's leaves yields results that self-identify their source.
type SearchResult struct {
	Name        string // the package name
	Version     string // the available version, when the manager reports one
	Description string // a short description, when the manager reports one
	Manager     string // the purl type of the leaf that produced the hit (e.g. "deb", "brew")
}

// endregion
