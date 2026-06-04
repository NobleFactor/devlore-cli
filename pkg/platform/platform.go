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

import (
	"fmt"
	"runtime"
)

// knownArches is the fixed architecture vocabulary, modeled on Docker's `--platform` accepted values.
//
// `WithArch` validates against this set at [NewPlatform] time. `WithArch("")` defaults to `runtime.GOARCH`;
// if the host's GOARCH is not in this set (e.g., "wasm"), [NewPlatform] errors.
var knownArches = map[string]struct{}{
	"amd64":    {},
	"arm64":    {},
	"arm/v7":   {},
	"arm/v6":   {},
	"386":      {},
	"ppc64le":  {},
	"s390x":    {},
	"mips64le": {},
	"riscv64":  {},
}

// knownLinuxDistros is the fixed Linux distro vocabulary. Anything outside this set errors at [NewPlatform]
// time when OS is "linux". Containers (Alpine) are explicitly excluded; see the 13.0(i) plan-row design.
var knownLinuxDistros = map[string]struct{}{
	"debian":        {},
	"ubuntu":        {},
	"mint":          {},
	"rhel":          {},
	"fedora":        {},
	"centos-stream": {},
	"almalinux":     {},
	"rocky":         {},
	"arch":          {},
	"manjaro":       {},
}

// Platform exposes the host's classification and the package and service managers available to providers.
//
// Implementations are immutable; construct via [NewPlatform]. Callers receive a Platform from the
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

// platform is the unexported implementation of [Platform] returned by [NewPlatform].
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

// NewPlatform validates `spec` and returns the immutable [Platform] it describes.
//
// Validation:
//   - OS must be set and one of "linux", "darwin", "windows".
//   - Arch must be in [knownArches]; empty defaults to runtime.GOARCH (which must itself be a known arch,
//     otherwise this errors).
//   - For OS=="linux", Distro must be in [knownLinuxDistros]; for OS=="darwin", Distro must be "macos"; for
//     OS=="windows", Distro must be "windows".
//   - If a default package manager is set, it must also appear as a value in the available-managers map.
//
// Parameters:
//   - `spec`: the populated [*PlatformSpec] (built via [Linux] / [Darwin] / [Windows] / [Detect] + With*).
//
// Returns:
//   - `Platform`: the constructed platform value.
//   - `error`: a single descriptive error per failing validation.
func NewPlatform(spec *PlatformSpec) (Platform, error) {

	if spec.os == "" {
		return nil, fmt.Errorf("platform: spec missing OS")
	}

	if spec.os != "linux" && spec.os != "darwin" && spec.os != "windows" {
		return nil, fmt.Errorf("platform: unknown OS %q; expected one of linux, darwin, windows", spec.os)
	}

	arch := spec.arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	if _, ok := knownArches[arch]; !ok {
		return nil, fmt.Errorf("platform: unknown architecture %q; expected one of amd64, arm64, arm/v7, arm/v6, 386, ppc64le, s390x, mips64le, riscv64", arch)
	}

	if err := spec.validateDistro(); err != nil {
		return nil, err
	}

	if spec.defaultPackageManager != nil {
		defaultName := spec.defaultPackageManager.Name()
		if _, ok := spec.availablePackageManagers[defaultName]; !ok {
			return nil, fmt.Errorf("platform: default package manager %q not in available set", defaultName)
		}
	}

	return &platform{
		os:                       spec.os,
		arch:                     arch,
		distro:                   spec.distro,
		version:                  spec.version,
		hostname:                 spec.hostname,
		defaultConcurrency:       spec.defaultConcurrency,
		defaultPackageManager:    spec.defaultPackageManager,
		availablePackageManagers: spec.availablePackageManagers,
		serviceManager:           spec.serviceManager,
	}, nil
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

// region SUPPORTING TYPES

// PlatformSpec is the fluent builder for a [Platform]. Construct via the named convenience entry points
// ([Linux], [Darwin], [Windows]) or via [Detect]; chain `With*` methods to override pre-baked defaults; pass
// the result to [NewPlatform] to produce the immutable [Platform] value.
//
// Specs are mutable during the chain (each `With*` mutates the receiver and returns it). The caller should not
// retain a reference to the spec after [NewPlatform]; the returned [Platform] is the durable value.
type PlatformSpec struct {
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

// WithArch sets the architecture. Empty string defaults to runtime.GOARCH at [NewPlatform] time, which
// validates it against [knownArches] (Docker vocabulary).
func (s *PlatformSpec) WithArch(arch string) *PlatformSpec {
	s.arch = arch
	return s
}

// WithVersion sets the OS or distro version string ("22.04", "14.5", etc.).
func (s *PlatformSpec) WithVersion(version string) *PlatformSpec {
	s.version = version
	return s
}

// WithHostname sets the host's network hostname.
func (s *PlatformSpec) WithHostname(hostname string) *PlatformSpec {
	s.hostname = hostname
	return s
}

// WithDefaultConcurrency sets the suggested concurrency level for parallel operations on this host.
func (s *PlatformSpec) WithDefaultConcurrency(n int) *PlatformSpec {
	s.defaultConcurrency = n
	return s
}

// WithDefaultPackageManager sets the package manager used when a pkg.Resource URI omits the manager prefix.
// [NewPlatform] enforces the invariant that the default appears as a value in the available map.
func (s *PlatformSpec) WithDefaultPackageManager(pm PackageManager) *PlatformSpec {
	s.defaultPackageManager = pm
	return s
}

// WithAvailablePackageManagers sets the map of package managers available on this platform, keyed by manager
// name. Replaces any prior value (does not merge). The default manager (if set) must appear as a value in this
// map.
func (s *PlatformSpec) WithAvailablePackageManagers(managers map[string]PackageManager) *PlatformSpec {
	s.availablePackageManagers = managers
	return s
}

// WithServiceManager sets the service manager (systemd, launchd, Service Control Manager).
func (s *PlatformSpec) WithServiceManager(sm ServiceManager) *PlatformSpec {
	s.serviceManager = sm
	return s
}

// validateDistro checks the spec's Distro against the OS-specific fixed list.
func (s *PlatformSpec) validateDistro() error {

	if s.distro == "" {
		return fmt.Errorf("platform: spec missing distro")
	}

	switch s.os {
	case "linux":
		if _, ok := knownLinuxDistros[s.distro]; !ok {
			return fmt.Errorf("platform: unknown linux distro %q; expected one of debian, ubuntu, mint, rhel, fedora, centos-stream, almalinux, rocky, arch, manjaro", s.distro)
		}
	case "darwin":
		if s.distro != "macos" {
			return fmt.Errorf("platform: darwin distro must be %q, got %q", "macos", s.distro)
		}
	case "windows":
		if s.distro != "windows" {
			return fmt.Errorf("platform: windows distro must be %q, got %q", "windows", s.distro)
		}
	}

	return nil
}

// endregion
