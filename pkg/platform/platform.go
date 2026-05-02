// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package platform models the host platform — OS, architecture, distro, and the package and service managers
// available on it.
//
// Concrete platforms are constructed via the named convenience functions ([Linux], [Darwin], [Windows]) for
// explicit fixtures or via [Detect] for host detection at runtime. The fluent [PlatformSpec] builder is the
// underlying mechanism; the named constructors are thin wrappers over pre-baked specs from
// [defaultPlatforms].
//
// Build tags apply only to host-detection code (`detect_<os>.go`). The manager types and shell wrappers
// compile on every host, so a graph plan can target any platform from any host. The runtime preflight
// catches target-vs-host mismatches before execution attempts to invoke a wrong-platform manager.
package platform

// Platform exposes the host's classification and the package and service managers available to providers.
//
// Implementations are immutable; construct via [PlatformSpec.Build]. Callers receive a Platform from the
// runtime environment and never construct it directly outside of [Detect] / [Linux] / [Darwin] / [Windows].
type Platform interface {

	// OS returns the operating system family ("linux", "darwin", "windows").
	OS() string

	// Arch returns the architecture ("amd64", "arm64", "arm/v7", etc.) per Docker's vocabulary.
	Arch() string

	// Distro returns the distribution identifier ("ubuntu", "fedora", "macos", "windows", etc.) — the
	// value of /etc/os-release ID on Linux, "macos" on Darwin, "windows" on Windows.
	Distro() string

	// Version returns the OS or distro version string ("22.04", "14.5", "11", etc.). Empty when unknown.
	Version() string

	// Hostname returns the host's network hostname. Empty when unavailable.
	Hostname() string

	// DefaultConcurrency returns a reasonable concurrency level for parallel operations on this host —
	// typically 4 × NumCPU.
	DefaultConcurrency() int

	// DefaultPackageManager returns the package manager used when a pkg.Resource URI omits the manager
	// prefix (e.g., "jq" rather than "snap:jq"). Distro convention sets the default; the spec can override
	// it via [PlatformSpec.WithDefaultPackageManager].
	DefaultPackageManager() PackageManager

	// AvailablePackageManagers returns the package managers available on this platform, keyed by manager
	// name (e.g., "apt", "snap", "flatpak"). The default manager is always one of the values.
	AvailablePackageManagers() map[string]PackageManager

	// PackageManagerByName returns the package manager registered under name, or nil if absent.
	//
	// Used by pkg.Resource to dispatch URI prefixes to the right manager (e.g., "snap:firefox" calls
	// PackageManagerByName("snap")).
	PackageManagerByName(name string) PackageManager

	// InstalledBy returns the first available manager that reports the named package as installed, or nil
	// if no manager reports it installed. The default manager is checked first; remaining managers iterate
	// in unspecified order.
	InstalledBy(name string) PackageManager

	// AllInstalledBy returns every available manager that reports the named package as installed. Useful
	// for diagnostics where a package may be installed via multiple managers.
	AllInstalledBy(name string) []PackageManager

	// ServiceManager returns the service manager for this platform — systemd on Linux, launchd on Darwin,
	// Service Control Manager on Windows.
	ServiceManager() ServiceManager
}

// platform is the unexported implementation of [Platform] returned by [PlatformSpec.Build].
//
// All fields are set at construction; the value is immutable from the caller's perspective (no setters on
// the interface).
type platform struct {
	os                       string
	arch                     string
	distro                   string
	version                  string
	hostname                 string
	defaultConcurrency       int
	defaultPackageManager    PackageManager
	availablePackageManagers map[string]PackageManager
	serviceManager           ServiceManager
}

// region EXPORTED METHODS

// region State accessors

func (p *platform) OS() string                            { return p.os }
func (p *platform) Arch() string                          { return p.arch }
func (p *platform) Distro() string                        { return p.distro }
func (p *platform) Version() string                       { return p.version }
func (p *platform) Hostname() string                      { return p.hostname }
func (p *platform) DefaultConcurrency() int               { return p.defaultConcurrency }
func (p *platform) DefaultPackageManager() PackageManager { return p.defaultPackageManager }

func (p *platform) AvailablePackageManagers() map[string]PackageManager {
	return p.availablePackageManagers
}

func (p *platform) ServiceManager() ServiceManager { return p.serviceManager }

// endregion

// region Behaviors

// PackageManagerByName returns the manager registered under name, or nil if no such manager is available.
func (p *platform) PackageManagerByName(name string) PackageManager {

	if p.availablePackageManagers == nil {
		return nil
	}
	return p.availablePackageManagers[name]
}

// InstalledBy returns the first available manager that reports name as installed, or nil if none do.
//
// Checks the default first (most common case), then iterates the remaining managers. Iteration order over
// the map is unspecified after the default check.
func (p *platform) InstalledBy(name string) PackageManager {

	if p.defaultPackageManager != nil && p.defaultPackageManager.Installed(name) {
		return p.defaultPackageManager
	}

	for _, manager := range p.availablePackageManagers {
		if manager == p.defaultPackageManager {
			continue
		}
		if manager.Installed(name) {
			return manager
		}
	}

	return nil
}

// AllInstalledBy returns every available manager that reports name as installed.
//
// The returned slice is empty when no manager reports it installed.
func (p *platform) AllInstalledBy(name string) []PackageManager {

	var managers []PackageManager
	for _, manager := range p.availablePackageManagers {
		if manager.Installed(name) {
			managers = append(managers, manager)
		}
	}
	return managers
}

// endregion

// endregion
