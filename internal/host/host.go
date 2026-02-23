// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package host provides platform-specific bindings for lore's Starlark runtime.
//
// The host package abstracts OS-specific operations behind a common interface,
// allowing Starlark phase scripts to be platform-agnostic. The correct
// implementation is selected at build time via Go build tags.
//
// Design decisions:
//   - ADR-010: Host Bindings API (see lore-design-decisions.md)
//   - ADR-005: Windows Package Manager Choice (winget preferred over choco)
//
// Unsettled decisions affecting this package:
//   - ADR-011: Package Authoring (may affect how package.* bindings work)
//   - ADR-014: Registry Infrastructure Strategy (affects lore.* bindings)
package host

import "runtime"

// Platform holds system information using Go's naming conventions.
// Exposed to Starlark as the read-only `platform` object.
type Platform struct {
	OS       string // GOOS: "darwin", "linux", "windows"
	Arch     string // GOARCH: "amd64", "arm64", "386"
	Distro   string // Distribution: "debian", "ubuntu", "fedora", "macos", "windows"
	Version  string // OS version string
	Hostname string // Machine hostname
}

// Result represents a command execution result.
// Returned by shell, package, and service operations.
type Result struct {
	OK     bool
	Stdout string
	Stderr string
	Code   int
}

// SearchResult represents a package found by search.
type SearchResult struct {
	Name        string // Package name
	Version     string // Available version (may be empty)
	Description string // Package description
}

// PackageManager abstracts package manager operations.
// Each platform provides its own implementation.
type PackageManager interface {
	// Name returns the package manager identifier.
	Name() string

	// Installed checks if a package is installed.
	Installed(name string) bool

	// Version returns the installed version of a package.
	Version(name string) string

	// Available checks if a package exists in the package manager's repositories.
	// This verifies the package can be installed, not that it is installed.
	Available(name string) bool

	// Search finds packages matching a query string.
	// Returns up to limit results (0 = no limit).
	Search(query string, limit int) []SearchResult

	// Install installs one or more packages.
	Install(packages ...string) Result

	// Remove removes a package.
	Remove(name string) Result

	// Update refreshes the package index.
	Update() Result

	// AddRepo adds a package repository.
	AddRepo(url, keyURL, name string) Result

	// NeedsSudo returns true if operations require privilege elevation.
	NeedsSudo() bool
}

// ServiceManager abstracts service management operations.
// Each platform provides its own implementation (systemd, launchd, Windows Services).
type ServiceManager interface {
	// Exists checks if a service exists.
	Exists(name string) bool

	// IsRunning returns true if the named service is currently running.
	IsRunning(name string) bool

	// IsEnabled returns true if the named service is enabled to start at boot.
	IsEnabled(name string) bool

	// Status returns the service status.
	Status(name string) string

	// Start starts a service.
	Start(name string) Result

	// Stop stops a service.
	Stop(name string) Result

	// Enable enables a service at boot.
	Enable(name string) Result

	// Disable disables a service at boot.
	Disable(name string) Result

	// NeedsSudo returns true if operations require privilege elevation.
	NeedsSudo() bool
}

// Host provides the full set of platform-specific operations.
// Created via NewHost() which returns the appropriate implementation.
type Host interface {
	// Platform returns system information.
	Platform() Platform

	// PackageManager returns the preferred package manager for this platform.
	// On Darwin, this is port if installed, otherwise brew.
	PackageManager() PackageManager

	// InstalledBy returns the package manager that installed the named package.
	// On platforms with multiple PMs (Darwin), returns the preferred PM if the
	// package is installed by multiple managers. Returns nil if not installed.
	InstalledBy(name string) PackageManager

	// AllInstalledBy returns all package managers that have the package installed.
	// On Darwin, a package may be installed by both Homebrew and MacPorts.
	// Used for warnings and comprehensive cleanup (decommission).
	AllInstalledBy(name string) []PackageManager

	// GetPackageManager returns a specific package manager by name.
	// On Darwin: "brew" or "port". Returns nil if not available.
	// Used to honor explicit prefixes like brew:pkg or port:pkg.
	GetPackageManager(name string) PackageManager

	// ServiceManager returns the service manager for this platform.
	ServiceManager() ServiceManager

	// RunCommand executes a shell command.
	RunCommand(command string, sudo bool) Result

	// ExpandPath expands ~ to home directory.
	ExpandPath(path string) string

	// HomeDir returns the user's home directory.
	HomeDir() string
}

// NewHost returns the appropriate Host implementation for the current platform.
// This is the main entry point for platform-specific functionality.
func NewHost() Host {
	switch runtime.GOOS {
	case "darwin":
		return newDarwinHost()
	case "linux":
		return newLinuxHost()
	case "windows":
		return newWindowsHost()
	default:
		// Fallback to Linux implementation for unknown platforms
		return newLinuxHost()
	}
}

// DetectPlatform returns current system information.
// Delegates to platform-specific detection.
func DetectPlatform() Platform {
	return NewHost().Platform()
}
