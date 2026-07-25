// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

// Linux managers (apt, dnf, pacman, systemd) split across three files for cross-host build support:
//
//   - linux_managers.go        types + identity + driver wiring (this file, always compiled)
//   - linux_managers_linux.go  real shell-out primitives (Linux only)
//   - linux_managers_other.go  stub primitives (every non-Linux host)
//
// On Linux: this file + linux_managers_linux.go combine; primitives are real.
// On any other host: this file + linux_managers_other.go combine; the shell-out primitives return false / "" / nil
// / an error PlatformResult so cross-host fixtures construct successfully but fail loudly at run time.

// Interface guards: each type satisfies its interface on every host (real on Linux, stubbed elsewhere).
var (
	_ leaf           = (*aptManager)(nil)
	_ leaf           = (*dnfManager)(nil)
	_ leaf           = (*pacmanManager)(nil)
	_ ServiceManager = (*systemdManager)(nil)
	_ ServiceManager = (*sysVinitManager)(nil)
)

// =============================================================================
// APT (Debian, Ubuntu, Mint) — purl type "deb"
// =============================================================================

type aptManager struct{ driver }

// newAptManager constructs an apt leaf with its [driver] wired to its own primitives.
//
// Returns:
//   - `*aptManager`: the wired leaf.
func newAptManager() *aptManager {
	m := &aptManager{}
	m.driver = newDriver(m)
	return m
}

// region UNEXPORTED METHODS

// region State management

// name returns the manager's name — the user-facing pkg.Resource prefix.
//
// Returns:
//   - `string`: "apt".
func (m *aptManager) name() string { return "apt" }

// purlType returns the manager's purl type — the routing key and the URI type.
//
// Returns:
//   - `string`: "deb".
func (m *aptManager) purlType() string { return "deb" }

// endregion

// endregion

// =============================================================================
// DNF (Fedora, RHEL, CentOS, Alma, Rocky) — purl type "rpm"
// =============================================================================

type dnfManager struct{ driver }

// newDnfManager constructs a dnf leaf with its [driver] wired to its own primitives.
//
// Returns:
//   - `*dnfManager`: the wired leaf.
func newDnfManager() *dnfManager {
	m := &dnfManager{}
	m.driver = newDriver(m)
	return m
}

// region UNEXPORTED METHODS

// region State management

// name returns the manager's name — the user-facing pkg.Resource prefix.
//
// Returns:
//   - `string`: "dnf".
func (m *dnfManager) name() string { return "dnf" }

// purlType returns the manager's purl type — the routing key and the URI type.
//
// Returns:
//   - `string`: "rpm".
func (m *dnfManager) purlType() string { return "rpm" }

// endregion

// endregion

// =============================================================================
// Pacman (Arch, Manjaro) — purl type "alpm"
// =============================================================================

type pacmanManager struct{ driver }

// newPacmanManager constructs a pacman leaf with its [driver] wired to its own primitives.
//
// Returns:
//   - `*pacmanManager`: the wired leaf.
func newPacmanManager() *pacmanManager {
	m := &pacmanManager{}
	m.driver = newDriver(m)
	return m
}

// region UNEXPORTED METHODS

// region State management

// name returns the manager's name — the user-facing pkg.Resource prefix.
//
// Returns:
//   - `string`: "pacman".
func (m *pacmanManager) name() string { return "pacman" }

// purlType returns the manager's purl type — the routing key and the URI type.
//
// Returns:
//   - `string`: "alpm".
func (m *pacmanManager) purlType() string { return "alpm" }

// endregion

// endregion

// =============================================================================
// systemd Service Manager
// =============================================================================

type systemdManager struct{}

// region EXPORTED METHODS

// region Behaviors

// NeedsSudo reports that systemd service mutations require elevation.
//
// Returns:
//   - `bool`: always true.
func (m *systemdManager) NeedsSudo() bool { return true }

// endregion

// endregion

// =============================================================================
// SysVinit Service Manager
// =============================================================================

type sysVinitManager struct{}

// region EXPORTED METHODS

// region Behaviors

// NeedsSudo reports that SysVinit service mutations require elevation.
//
// Returns:
//   - `bool`: always true.
func (m *sysVinitManager) NeedsSudo() bool { return true }

// endregion

// endregion
