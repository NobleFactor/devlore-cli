// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Platform carries platform info plus runtime package/service managers.
// Serializable fields are stored in graphs; runtime fields are injected
// at execution time by platform.New().
type Platform struct {
	// Serializable info (used by Graph)
	OS       string `json:"os" yaml:"os"`
	Arch     string `json:"arch" yaml:"arch"`
	Distro   string `json:"distro,omitempty" yaml:"distro,omitempty"`
	Version  string `json:"version,omitempty" yaml:"version,omitempty"`
	Hostname string `json:"hostname,omitempty" yaml:"hostname,omitempty"`

	// Runtime — not serialized
	PackageManager  PackageManager            `json:"-" yaml:"-"`
	PackageManagers map[string]PackageManager `json:"-" yaml:"-"`
	ServiceManager  ServiceManager            `json:"-" yaml:"-"`
}

// GetPackageManager returns a specific package manager by name.
// Returns nil if unavailable.
func (p *Platform) GetPackageManager(name string) PackageManager {
	if p.PackageManagers == nil {
		return nil
	}
	return p.PackageManagers[name]
}

// InstalledBy returns the package manager that installed the named package.
// Returns nil if not installed by any known manager.
func (p *Platform) InstalledBy(name string) PackageManager {
	// Check preferred package manager first.
	if p.PackageManager != nil && p.PackageManager.Installed(name) {
		return p.PackageManager
	}
	for _, manager := range p.PackageManagers {
		if manager.Installed(name) {
			return manager
		}
	}
	return nil
}

// AllInstalledBy returns all package managers that have the package installed.
func (p *Platform) AllInstalledBy(name string) []PackageManager {
	var managers []PackageManager
	for _, manager := range p.PackageManagers {
		if manager.Installed(name) {
			managers = append(managers, manager)
		}
	}
	return managers
}

// PackageManager abstracts package manager operations.
type PackageManager interface {
	Name() string
	Installed(name string) bool
	Version(name string) string
	Available(name string) bool
	Search(query string, limit int) []SearchResult
	Install(packages ...string) PlatformResult
	Remove(name string) PlatformResult
	Update() PlatformResult
	AddRepo(url, keyURL, name string) PlatformResult
	NeedsSudo() bool
}

// ServiceManager abstracts service management operations.
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

// PlatformResult represents a command execution result.
type PlatformResult struct {
	OK     bool
	Stdout string
	Stderr string
	Code   int
}

// SearchResult represents a package found by search.
type SearchResult struct {
	Name        string
	Version     string
	Description string
}
