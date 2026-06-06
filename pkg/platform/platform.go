// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package platform models a target platform as a standalone, op-free capability.
//
// A platform is its OS, architecture, distro, and the package and service managers available on it. It is
// constructed by cloning a named default [Spec] (e.g. [Debian], [Fedora], [Darwin], [Windows]) or by
// detecting the host ([Detect]), optionally mutating the spec via its `With*` methods, then sealing it with [New].
// The sealed [Platform] exposes its identity plus one [PackageManager] — the Composite router over the platform's
// leaf drivers — and one [ServiceManager]. `pkg/platform` imports nothing from `pkg/op`; `pkg/op` imports it (the
// same shape as `pkg/result` and `pkg/status`).
//
// Build tags apply only to the per-OS manager primitives (`*_<os>.go`) and host detection (`detect_<os>.go`). The
// manager types and the contract compile on every host, so a graph can target any platform from any host; the
// run-time preflight catches target-vs-host mismatches before a wrong-platform manager is invoked.
package platform

import (
	"fmt"
	"runtime"
)

// knownArches is the fixed architecture vocabulary, modeled on Docker's `--platform` accepted values.
//
// [New] validates `arch` against this set. An empty arch defaults to `runtime.GOARCH`; if the host's GOARCH is not
// in this set (e.g. "wasm"), [New] errors.
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

// knownLinuxDistros is the fixed Linux distro vocabulary.
//
// Anything outside this set errors at [New] time when OS is "linux". Containers (Alpine) are explicitly excluded.
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

// Platform exposes a target's classification and the package and service managers available on it.
//
// Implementations are immutable; construct via [New]. Callers receive a Platform from the runtime environment and
// never construct it directly outside of a named [Spec] factory + [New] or [Detect] + [New].
type Platform interface {

	// OS returns the operating system family ("linux", "darwin", "windows").
	OS() string

	// Arch returns the architecture ("amd64", "arm64", "arm/v7", etc.) per Docker's vocabulary.
	Arch() string

	// Distro returns the distribution identifier ("ubuntu", "fedora", "macos", "windows", etc.).
	Distro() string

	// Version returns the OS or distro version string ("22.04", "14.5", "11", etc.). Empty when unknown.
	Version() string

	// Hostname returns the host's network hostname. Empty when unavailable.
	Hostname() string

	// DefaultConcurrency returns a reasonable concurrency level for parallel operations — typically 4 × NumCPU.
	DefaultConcurrency() int

	// DefaultPurlType returns the purl type of the platform's default native manager (e.g. "deb" on Debian).
	//
	// The veneer uses it to normalize a bare package name into a typed purl.
	DefaultPurlType() string

	// ResolvePurlType maps a caller-supplied manager prefix — a manager name or a purl type — to the canonical
	// purl type, reporting whether the prefix names a known manager on this platform.
	ResolvePurlType(prefix string) (string, bool)

	// PackageManager returns the platform's Composite router over its leaf drivers.
	PackageManager() PackageManager

	// ServiceManager returns the service manager for this platform (systemd, launchd, Service Control Manager).
	ServiceManager() ServiceManager
}

// New validates `spec` and returns the immutable [Platform] it describes.
//
// Validation:
//   - OS must be set and one of "linux", "darwin", "windows".
//   - Arch must be in [knownArches]; empty defaults to `runtime.GOARCH` (which must itself be a known arch).
//   - For OS=="linux", Distro must be in [knownLinuxDistros]; for "darwin" it must be "macos"; for "windows",
//     "windows".
//
// Parameters:
//   - `spec`: the populated [*Spec] (from a named factory or [Detect], optionally mutated via `With*`).
//
// Returns:
//   - `Platform`: the sealed platform value.
//   - `error`: a single descriptive error per failing validation.
func New(spec *Spec) (Platform, error) {

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

	return &platform{
		os:                 spec.os,
		arch:               arch,
		distro:             spec.distro,
		version:            spec.version,
		hostname:           spec.hostname,
		defaultConcurrency: spec.defaultConcurrency,
		router:             newComposite(spec.managers, spec.defaultManager),
		serviceManager:     spec.serviceManager,
	}, nil
}

// platform is the unexported [Platform] implementation returned by [New].
//
// All fields are set at construction; the value is immutable from the caller's perspective.
type platform struct {
	os                 string            // OS family: "linux", "darwin", "windows"
	arch               string            // architecture, Docker vocabulary
	distro             string            // distribution identifier
	version            string            // OS or distro version, or "" when unknown
	hostname           string            // network hostname, or "" when unavailable
	defaultConcurrency int               // suggested parallelism for batch operations
	router             *compositeManager // the Composite package-manager router
	serviceManager     ServiceManager    // the platform's service manager
}

// region EXPORTED METHODS

// region State management

// Arch returns the architecture.
//
// Returns:
//   - `string`: the architecture identifier (e.g. "amd64").
func (p *platform) Arch() string { return p.arch }

// DefaultConcurrency returns the suggested concurrency level for parallel operations.
//
// Returns:
//   - `int`: the concurrency level.
func (p *platform) DefaultConcurrency() int { return p.defaultConcurrency }

// DefaultPurlType returns the purl type of the platform's default native manager.
//
// Returns:
//   - `string`: the default native purl type (e.g. "deb").
func (p *platform) DefaultPurlType() string { return p.router.defaultType }

// Distro returns the distribution identifier.
//
// Returns:
//   - `string`: the distro id (e.g. "ubuntu", "macos", "windows").
func (p *platform) Distro() string { return p.distro }

// Hostname returns the host's network hostname.
//
// Returns:
//   - `string`: the hostname, or "" when unavailable.
func (p *platform) Hostname() string { return p.hostname }

// OS returns the operating system family.
//
// Returns:
//   - `string`: "linux", "darwin", or "windows".
func (p *platform) OS() string { return p.os }

// PackageManager returns the platform's Composite router over its leaf drivers.
//
// Returns:
//   - `PackageManager`: the router.
func (p *platform) PackageManager() PackageManager { return p.router }

// ServiceManager returns the platform's service manager.
//
// Returns:
//   - `ServiceManager`: the service manager (systemd, launchd, or Service Control Manager).
func (p *platform) ServiceManager() ServiceManager { return p.serviceManager }

// Version returns the OS or distro version string.
//
// Returns:
//   - `string`: the version, or "" when unknown.
func (p *platform) Version() string { return p.version }

// endregion

// region Behaviors

// ResolvePurlType maps a manager prefix (name or purl type) to the canonical purl type for this platform.
//
// Parameters:
//   - `prefix`: the manager prefix from a pkg.Resource identifier (e.g. "apt", "deb", "brew").
//
// Returns:
//   - `string`: the canonical purl type the prefix resolves to.
//   - `bool`: true when the prefix names a known manager or type on this platform.
func (p *platform) ResolvePurlType(prefix string) (string, bool) {
	return p.router.resolveType(prefix)
}

// endregion

// endregion

// region SUPPORTING TYPES

// Spec is the mutable builder for a [Platform].
//
// Obtain one by cloning a named default ([Debian], [Fedora], [Darwin], [Windows], …) or via [Detect], chain
// `With*` to override defaults, then pass it to [New]. Specs are mutable during the chain (each `With*` mutates the
// receiver and returns it). The caller should not retain the spec after [New]; the returned [Platform] is the
// durable value. The manager fields are populated by the named factories and are not part of the public `With*`
// surface.
type Spec struct {
	os                 string         // OS family: "linux", "darwin", "windows"
	arch               string         // architecture, or "" for the host arch at New time
	distro             string         // distribution identifier
	version            string         // OS or distro version, or "" when unknown
	hostname           string         // network hostname, or "" when unavailable
	defaultConcurrency int            // suggested parallelism for batch operations
	managers           []leaf         // the platform's leaf drivers
	defaultManager     leaf           // the default native leaf (must appear in managers)
	serviceManager     ServiceManager // the platform's service manager
}

// region EXPORTED METHODS

// region Behaviors

// WithArch sets the architecture. Empty defaults to `runtime.GOARCH` at [New] time.
//
// Parameters:
//   - `arch`: a [knownArches] value, or "" for the host arch.
//
// Returns:
//   - `*Spec`: the receiver, for chaining.
func (s *Spec) WithArch(arch string) *Spec {
	s.arch = arch
	return s
}

// WithDefaultConcurrency sets the suggested concurrency level for parallel operations.
//
// Parameters:
//   - `n`: the concurrency level.
//
// Returns:
//   - `*Spec`: the receiver, for chaining.
func (s *Spec) WithDefaultConcurrency(n int) *Spec {
	s.defaultConcurrency = n
	return s
}

// WithHostname sets the host's network hostname.
//
// Parameters:
//   - `hostname`: the hostname.
//
// Returns:
//   - `*Spec`: the receiver, for chaining.
func (s *Spec) WithHostname(hostname string) *Spec {
	s.hostname = hostname
	return s
}

// WithVersion sets the OS or distro version string ("22.04", "14.5", etc.).
//
// Parameters:
//   - `version`: the version string.
//
// Returns:
//   - `*Spec`: the receiver, for chaining.
func (s *Spec) WithVersion(version string) *Spec {
	s.version = version
	return s
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// validateDistro checks the spec's distro against the OS-specific fixed list.
//
// Returns:
//   - `error`: non-nil when the distro is empty or not valid for the spec's OS.
func (s *Spec) validateDistro() error {

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

// endregion

// endregion
