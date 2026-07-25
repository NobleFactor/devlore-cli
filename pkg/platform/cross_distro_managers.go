// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

// Cross-distro Linux managers (snap, flatpak) split across three files for cross-host build support:
//
//   - cross_distro_managers.go        types + identity + driver wiring (this file, always compiled)
//   - cross_distro_managers_linux.go  real shell-out primitives (Linux only)
//   - cross_distro_managers_other.go  stub primitives (every non-Linux host)
//
// snap and flatpak are Linux-only at run time, so the implementation file uses the Linux build tag and the stub
// file uses //go:build !linux — the same shape as the linux_managers split, just different managers.

// Interface guards: each type satisfies its interface on every host (real on Linux, stubbed elsewhere).
var (
	_ leaf = (*flatpakManager)(nil)
	_ leaf = (*snapManager)(nil)
)

// =============================================================================
// flatpak Package Manager — purl type "flatpak"
// =============================================================================
//
// Flatpak is the default secondary on Fedora Workstation, openSUSE, and Mint. Remotes (Flathub being the canonical
// one) supply the app catalog. Names are reverse-DNS application ids (e.g. org.gimp.GIMP).

type flatpakManager struct{ driver }

// newFlatpakManager constructs a flatpak leaf with its [driver] wired to its own primitives.
//
// Returns:
//   - `*flatpakManager`: the wired leaf.
func newFlatpakManager() *flatpakManager {
	m := &flatpakManager{}
	m.driver = newDriver(m)
	return m
}

// region UNEXPORTED METHODS

// region State management

// name returns the manager's name — the user-facing pkg.Resource prefix.
//
// Returns:
//   - `string`: "flatpak".
func (m *flatpakManager) name() string { return "flatpak" }

// purlType returns the manager's purl type — the routing key and the URI type.
//
// Returns:
//   - `string`: "flatpak".
func (m *flatpakManager) purlType() string { return "flatpak" }

// endregion

// endregion

// =============================================================================
// snap Package Manager — purl type "snap"
// =============================================================================
//
// Snap is the default secondary on Ubuntu (pre-installed since 16.04) and Manjaro (via pamac). Available on most
// other distros via snapd, but not pre-shipped.

type snapManager struct{ driver }

// newSnapManager constructs a snap leaf with its [driver] wired to its own primitives.
//
// Returns:
//   - `*snapManager`: the wired leaf.
func newSnapManager() *snapManager {
	m := &snapManager{}
	m.driver = newDriver(m)
	return m
}

// region UNEXPORTED METHODS

// region State management

// name returns the manager's name — the user-facing pkg.Resource prefix.
//
// Returns:
//   - `string`: "snap".
func (m *snapManager) name() string { return "snap" }

// purlType returns the manager's purl type — the routing key and the URI type.
//
// Returns:
//   - `string`: "snap".
func (m *snapManager) purlType() string { return "snap" }

// endregion

// endregion
