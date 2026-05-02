// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import "strings"

// Cross-distro Linux managers (snap, flatpak) split across three files for cross-host build support:
//
//   - cross_distro_managers.go        types + pure methods (this file, always compiled)
//   - cross_distro_managers_linux.go  real shell-out implementations (Linux only)
//   - cross_distro_managers_other.go  stub shell-out implementations (every non-Linux host)
//
// snap and flatpak are Linux-only at runtime, so the implementation file uses the Linux build tag and
// the stub file uses //go:build !linux — same shape as the linux_managers split, just different
// managers.

// Compile-time interface guards — on every host, each type implements its full interface (real on
// Linux, stubbed on every other host).
var (
	_ PackageManager = (*snapManager)(nil)
	_ PackageManager = (*flatpakManager)(nil)
)

// =============================================================================
// snap Package Manager (Canonical's universal Linux store)
// =============================================================================
//
// Snap is the default secondary on Ubuntu (pre-installed since 16.04+) and Manjaro (via pamac).
// Available on most other distros via snapd installation but not pre-shipped.

type snapManager struct{}

func (m *snapManager) Name() string { return "snap" }

func (m *snapManager) ParsePURL(id string) PURL {

	name, version, _ := strings.Cut(id, "@")
	return PURL{Type: "snap", Name: name, Version: version}
}

func (m *snapManager) NeedsSudo() bool { return true }

// =============================================================================
// flatpak Package Manager (Sandboxed desktop apps via Flathub or other remotes)
// =============================================================================
//
// Flatpak is the default secondary on Fedora Workstation, openSUSE, and Mint. Available cross-distro;
// remotes (Flathub being the canonical one) supply the app catalog.

type flatpakManager struct{}

func (m *flatpakManager) Name() string { return "flatpak" }

func (m *flatpakManager) ParsePURL(id string) PURL {

	name, version, _ := strings.Cut(id, "@")
	return PURL{Type: "flatpak", Name: name, Version: version}
}

// NeedsSudo is false because flatpak defaults to user-level installs (~/.local/share/flatpak).
// System-wide installs (--system flag) would need sudo, but those are not the default path.
func (m *flatpakManager) NeedsSudo() bool { return false }
