// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

// PackageManager abstracts package manager operations.
//
// Concrete implementations exist for apt (Debian/Ubuntu/Mint), dnf (RHEL/Fedora/CentOS/Alma/Rocky), pacman
// (Arch/Manjaro), brew (macOS), port (macOS MacPorts), winget (Windows). All shell out to the underlying
// binary; methods that mutate state require sudo on Linux per [PackageManager.NeedsSudo].
type PackageManager interface {

	// Name returns the manager's identifier ("apt", "dnf", "pacman", "brew", "port", "winget", "snap",
	// "flatpak"). Used as the URI prefix in pkg.Resource ("snap:firefox").
	Name() string

	// ParsePURL converts a manager-specific package identifier into a [PURL]. Pure string parsing; no
	// shell or filesystem access.
	ParsePURL(id string) PURL

	// Installed reports whether the named package is installed.
	Installed(name string) bool

	// Version returns the installed version of the named package, or empty if not installed.
	Version(name string) string

	// Available reports whether the named package is available in the manager's index.
	Available(name string) bool

	// Search returns up to limit packages matching query. limit ≤ 0 means no limit.
	Search(query string, limit int) []SearchResult

	// Install installs one or more packages.
	Install(packages ...string) PlatformResult

	// Remove uninstalls a single package.
	Remove(name string) PlatformResult

	// Update refreshes the manager's package index.
	Update() PlatformResult

	// AddRepo registers an external repository with the manager.
	AddRepo(url, keyURL, name string) PlatformResult

	// NeedsSudo reports whether mutating operations require elevated privileges.
	NeedsSudo() bool
}

// ServiceManager abstracts service-management operations.
//
// Concrete implementations exist for systemd (Linux), launchd (Darwin), and Service Control Manager
// (Windows).
type ServiceManager interface {
	Exists(name string) bool
	IsRunning(name string) bool
	IsEnabled(name string) bool
	Status(name string) string
	Start(name string) PlatformResult
	Stop(name string) PlatformResult
	Enable(name string) PlatformResult
	Disable(name string) PlatformResult
	NeedsSudo() bool
}

// PlatformResult represents a command-execution result returned by [PackageManager] and [ServiceManager]
// mutators.
type PlatformResult struct {
	OK     bool
	Stdout string
	Stderr string
	Code   int
}

// SearchResult represents a package found by [PackageManager.Search].
type SearchResult struct {
	Name        string
	Version     string
	Description string
}
