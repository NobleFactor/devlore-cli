// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import "runtime"

// The named spec factories. Each returns a FRESH, mutable [*Spec] cloned from its catalog default — a fresh leaf
// set with its own [driver] wiring — so `With*` chaining never leaks across calls. Seal a spec into a [Platform]
// with [New]. Arch and Manjaro are deferred as exported factories (re-addable later); their internal builders
// remain wired into [linuxSpecByDistro] so [Detect] still recognizes those hosts.
//
// Per-distro mapping (default | available):
//
//	debian        -> apt    | apt
//	ubuntu        -> apt    | apt, snap
//	mint          -> apt    | apt, flatpak
//	rhel          -> dnf    | dnf, flatpak
//	fedora        -> dnf    | dnf, flatpak
//	centos-stream -> dnf    | dnf, flatpak
//	almalinux     -> dnf    | dnf, flatpak
//	rocky         -> dnf    | dnf, flatpak
//	arch          -> pacman | pacman
//	manjaro       -> pacman | pacman, snap, flatpak
//	macos         -> brew   | brew, port
//	windows       -> winget | winget

// linuxSpecByDistro maps each known Linux distro id (the /etc/os-release ID) to its spec factory.
//
// [Detect] uses it to build the host spec; the named factories below are its exported entry points.
var linuxSpecByDistro = map[string]func() *Spec{
	"debian":        Debian,
	"ubuntu":        Ubuntu,
	"mint":          Mint,
	"rhel":          RHEL,
	"fedora":        Fedora,
	"centos-stream": CentOSStream,
	"almalinux":     AlmaLinux,
	"rocky":         Rocky,
	"arch":          archSpec,
	"manjaro":       manjaroSpec,
}

// AlmaLinux returns a fresh [*Spec] for AlmaLinux (dnf default; dnf + flatpak available).
//
// Returns:
//   - `*Spec`: the fresh, mutable spec.
func AlmaLinux() *Spec {
	dnf, flatpak := newDnfManager(), newFlatpakManager()
	return linuxSpec("almalinux", dnf, []leaf{dnf, flatpak})
}

// CentOSStream returns a fresh [*Spec] for CentOS Stream (dnf default; dnf + flatpak available).
//
// Returns:
//   - `*Spec`: the fresh, mutable spec.
func CentOSStream() *Spec {
	dnf, flatpak := newDnfManager(), newFlatpakManager()
	return linuxSpec("centos-stream", dnf, []leaf{dnf, flatpak})
}

// Darwin returns a fresh [*Spec] for macOS (brew default; brew + port available; launchd services).
//
// Returns:
//   - `*Spec`: the fresh, mutable spec.
func Darwin() *Spec {
	brew, port := newBrewManager(), newPortManager()
	return &Spec{
		os:                 "darwin",
		distro:             "macos",
		defaultConcurrency: 4 * runtime.NumCPU(),
		managers:           []leaf{brew, port},
		defaultManager:     brew,
		serviceManager:     &launchdManager{},
	}
}

// Debian returns a fresh [*Spec] for Debian (apt default; apt available).
//
// Returns:
//   - `*Spec`: the fresh, mutable spec.
func Debian() *Spec {
	apt := newAptManager()
	return linuxSpec("debian", apt, []leaf{apt})
}

// Fedora returns a fresh [*Spec] for Fedora (dnf default; dnf + flatpak available).
//
// Returns:
//   - `*Spec`: the fresh, mutable spec.
func Fedora() *Spec {
	dnf, flatpak := newDnfManager(), newFlatpakManager()
	return linuxSpec("fedora", dnf, []leaf{dnf, flatpak})
}

// Mint returns a fresh [*Spec] for Linux Mint (apt default; apt + flatpak available).
//
// Returns:
//   - `*Spec`: the fresh, mutable spec.
func Mint() *Spec {
	apt, flatpak := newAptManager(), newFlatpakManager()
	return linuxSpec("mint", apt, []leaf{apt, flatpak})
}

// RHEL returns a fresh [*Spec] for Red Hat Enterprise Linux (dnf default; dnf + flatpak available).
//
// Returns:
//   - `*Spec`: the fresh, mutable spec.
func RHEL() *Spec {
	dnf, flatpak := newDnfManager(), newFlatpakManager()
	return linuxSpec("rhel", dnf, []leaf{dnf, flatpak})
}

// Rocky returns a fresh [*Spec] for Rocky Linux (dnf default; dnf + flatpak available).
//
// Returns:
//   - `*Spec`: the fresh, mutable spec.
func Rocky() *Spec {
	dnf, flatpak := newDnfManager(), newFlatpakManager()
	return linuxSpec("rocky", dnf, []leaf{dnf, flatpak})
}

// Ubuntu returns a fresh [*Spec] for Ubuntu (apt default; apt + snap available).
//
// Returns:
//   - `*Spec`: the fresh, mutable spec.
func Ubuntu() *Spec {
	apt, snap := newAptManager(), newSnapManager()
	return linuxSpec("ubuntu", apt, []leaf{apt, snap})
}

// Windows returns a fresh [*Spec] for Windows (winget default and only; Service Control Manager services).
//
// Returns:
//   - `*Spec`: the fresh, mutable spec.
func Windows() *Spec {
	winget := newWingetManager()
	return &Spec{
		os:                 "windows",
		distro:             "windows",
		defaultConcurrency: 4 * runtime.NumCPU(),
		managers:           []leaf{winget},
		defaultManager:     winget,
		serviceManager:     &windowsServiceManager{},
	}
}

// archSpec returns a fresh [*Spec] for Arch Linux (pacman default; pacman available).
//
// Internal — Arch is deferred as an exported factory but still recognized by [Detect].
//
// Returns:
//   - `*Spec`: the fresh, mutable spec.
func archSpec() *Spec {
	pacman := newPacmanManager()
	return linuxSpec("arch", pacman, []leaf{pacman})
}

// manjaroSpec returns a fresh [*Spec] for Manjaro (pacman default; pacman + snap + flatpak available).
//
// Internal — Manjaro is deferred as an exported factory but still recognized by [Detect].
//
// Returns:
//   - `*Spec`: the fresh, mutable spec.
func manjaroSpec() *Spec {
	pacman, snap, flatpak := newPacmanManager(), newSnapManager(), newFlatpakManager()
	return linuxSpec("manjaro", pacman, []leaf{pacman, snap, flatpak})
}

// linuxSpec assembles a Linux [*Spec] with the given manager set and systemd as the declared service manager.
//
// systemd is the declared default for every supported distro; [Detect] overrides it on a live host by probing the
// active init, so a systemd-less box (container, WSL, minimal/CI) resolves to SysVinit instead.
//
// Parameters:
//   - `distro`: the distro id.
//   - `defaultManager`: the default native leaf.
//   - `managers`: the full leaf set (must include `defaultManager`).
//
// Returns:
//   - `*Spec`: the assembled spec.
func linuxSpec(distro string, defaultManager leaf, managers []leaf) *Spec {
	return &Spec{
		os:                 "linux",
		distro:             distro,
		defaultConcurrency: 4 * runtime.NumCPU(),
		managers:           managers,
		defaultManager:     defaultManager,
		serviceManager:     &systemdManager{},
	}
}
