// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// HostProvider exposes platform abstractions to action providers.
// Implemented by the adapter in internal/execution wrapping internal/host.Host.
type HostProvider interface {
	// PackageManager returns the preferred package manager.
	PackageManager() PackageManagerProvider

	// InstalledBy returns the PM that installed the named package (nil if not installed).
	InstalledBy(name string) PackageManagerProvider

	// AllInstalledBy returns all PMs that have the package installed.
	AllInstalledBy(name string) []PackageManagerProvider

	// GetPackageManager returns a specific PM by name (nil if unavailable).
	GetPackageManager(name string) PackageManagerProvider

	// ServiceManager returns the service manager for this platform.
	ServiceManager() ServiceManagerProvider
}

// PackageManagerProvider abstracts package manager operations.
type PackageManagerProvider interface {
	// Name returns the package manager identifier (e.g., "brew", "apt").
	Name() string

	// Installed checks if a package is installed.
	Installed(name string) bool

	// Version returns the installed version of a package.
	Version(name string) string

	// Available checks if a package exists in the repositories.
	Available(name string) bool

	// Install installs one or more packages.
	Install(packages ...string) error

	// Remove removes a package.
	Remove(name string) error

	// Update refreshes the package index.
	Update() error

	// NeedsSudo returns true if operations require privilege elevation.
	NeedsSudo() bool
}

// ServiceManagerProvider abstracts service management operations.
type ServiceManagerProvider interface {
	// Exists checks if a service exists.
	Exists(name string) bool

	// IsRunning returns true if the named service is currently running.
	IsRunning(name string) bool

	// IsEnabled returns true if the named service is enabled to start at boot.
	IsEnabled(name string) bool

	// Start starts a service.
	Start(name string) error

	// Stop stops a service.
	Stop(name string) error

	// Enable enables a service at boot.
	Enable(name string) error

	// Disable disables a service at boot.
	Disable(name string) error
}
