// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"fmt"
	"runtime"
)

// knownArches is the fixed architecture vocabulary, modeled on Docker's `--platform` accepted values.
//
// `WithArch` validates against this set at [PlatformSpec.Build] time. `WithArch("")` defaults to
// `runtime.GOARCH`; if the host's GOARCH is not in this set (e.g., "wasm"), Build() errors.
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

// knownLinuxDistros is the fixed Linux distro vocabulary. Anything outside this set errors at
// [PlatformSpec.Build] time when OS is "linux". Containers (Alpine) are explicitly excluded; see the
// 13.0(i) plan-row design discussion.
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

// PlatformSpec is the fluent builder for a [Platform]. Construct via the named convenience entry points
// ([Linux], [Darwin], [Windows]) or via [Detect]; chain `With*` methods to override pre-baked defaults;
// terminate with [PlatformSpec.Build] to produce the immutable [Platform] value.
//
// Specs are mutable during the chain (each `With*` mutates the receiver and returns it). The caller
// should not retain a reference to the spec after Build(); the returned [Platform] is the durable value.
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

// region EXPORTED METHODS

// region State management

// WithArch sets the architecture. Empty string defaults to runtime.GOARCH at [PlatformSpec.Build] time.
// Build validates against [knownArches] (Docker vocabulary).
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

// WithDefaultPackageManager sets the package manager used when a pkg.Resource URI omits the manager
// prefix. Build() enforces the invariant that the default appears as a value in the available map.
func (s *PlatformSpec) WithDefaultPackageManager(pm PackageManager) *PlatformSpec {
	s.defaultPackageManager = pm
	return s
}

// WithAvailablePackageManagers sets the map of package managers available on this platform, keyed by
// manager name. Replaces any prior value (does not merge). The default manager (if set) must appear as
// a value in this map.
func (s *PlatformSpec) WithAvailablePackageManagers(managers map[string]PackageManager) *PlatformSpec {
	s.availablePackageManagers = managers
	return s
}

// WithServiceManager sets the service manager (systemd, launchd, Service Control Manager).
func (s *PlatformSpec) WithServiceManager(sm ServiceManager) *PlatformSpec {
	s.serviceManager = sm
	return s
}

// endregion

// region Behaviors

// Build validates the spec and returns the immutable [Platform].
//
// Validation:
//   - OS must be set and one of "linux", "darwin", "windows".
//   - Arch must be in [knownArches]; empty defaults to runtime.GOARCH (which must itself be a known
//     arch, otherwise Build errors).
//   - For OS=="linux", Distro must be in [knownLinuxDistros]; for OS=="darwin", Distro must be "macos";
//     for OS=="windows", Distro must be "windows".
//   - If a default package manager is set, it must also appear as a value in the available-managers
//     map (default-must-be-available invariant).
//
// Returns:
//   - Platform: the constructed platform value, ready to install on a [op.RuntimeEnvironmentSpec].
//   - error: a single descriptive error per failing validation.
func (s *PlatformSpec) Build() (Platform, error) {

	if s.os == "" {
		return nil, fmt.Errorf("platform: spec missing OS")
	}

	if s.os != "linux" && s.os != "darwin" && s.os != "windows" {
		return nil, fmt.Errorf("platform: unknown OS %q; expected one of linux, darwin, windows", s.os)
	}

	arch := s.arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	if _, ok := knownArches[arch]; !ok {
		return nil, fmt.Errorf("platform: unknown architecture %q; expected one of amd64, arm64, arm/v7, arm/v6, 386, ppc64le, s390x, mips64le, riscv64", arch)
	}

	if err := s.validateDistro(); err != nil {
		return nil, err
	}

	if s.defaultPackageManager != nil {
		defaultName := s.defaultPackageManager.Name()
		if _, ok := s.availablePackageManagers[defaultName]; !ok {
			return nil, fmt.Errorf("platform: default package manager %q not in available set", defaultName)
		}
	}

	return &platform{
		os:                       s.os,
		arch:                     arch,
		distro:                   s.distro,
		version:                  s.version,
		hostname:                 s.hostname,
		defaultConcurrency:       s.defaultConcurrency,
		defaultPackageManager:    s.defaultPackageManager,
		availablePackageManagers: s.availablePackageManagers,
		serviceManager:           s.serviceManager,
	}, nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

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

// endregion
