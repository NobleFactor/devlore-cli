// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

// Darwin managers (brew, port, launchd) split across three files for cross-host build support:
//
//   - darwin_managers.go         types + identity + driver wiring (this file, always compiled)
//   - darwin_managers_darwin.go  real shell-out primitives (Darwin only)
//   - darwin_managers_other.go   stub primitives (every non-Darwin host)
//
// On Darwin: this file + darwin_managers_darwin.go combine; primitives are real.
// On any other host: this file + darwin_managers_other.go combine; the shell-out primitives return error
// PlatformResults so cross-host fixtures construct successfully but fail loudly at run time.

// Interface guards: each type satisfies its interface on every host (real on Darwin, stubbed elsewhere).
var (
	_ leaf           = (*brewManager)(nil)
	_ leaf           = (*portManager)(nil)
	_ ServiceManager = (*launchdManager)(nil)
)

// =============================================================================
// Homebrew Package Manager — purl type "brew"
// =============================================================================

type brewManager struct{ driver }

// newBrewManager constructs a brew leaf with its [driver] wired to its own primitives.
//
// Returns:
//   - `*brewManager`: the wired leaf.
func newBrewManager() *brewManager {
	m := &brewManager{}
	m.driver = newDriver(m)
	return m
}

// region UNEXPORTED METHODS

// region State management

// name returns the manager's name — the user-facing pkg.Resource prefix.
//
// Returns:
//   - `string`: "brew".
func (m *brewManager) name() string { return "brew" }

// purlType returns the manager's purl type — the routing key and the URI type.
//
// Returns:
//   - `string`: "brew".
func (m *brewManager) purlType() string { return "brew" }

// endregion

// endregion

// =============================================================================
// launchd Service Manager
// =============================================================================

type launchdManager struct{}

// region EXPORTED METHODS

// region Behaviors

// NeedsSudo reports that launchd user-agent operations do not require elevation.
//
// Returns:
//   - `bool`: always false.
func (m *launchdManager) NeedsSudo() bool { return false }

// endregion

// endregion

// =============================================================================
// MacPorts Package Manager — purl type "port"
// =============================================================================

type portManager struct{ driver }

// newPortManager constructs a MacPorts leaf with its [driver] wired to its own primitives.
//
// Returns:
//   - `*portManager`: the wired leaf.
func newPortManager() *portManager {
	m := &portManager{}
	m.driver = newDriver(m)
	return m
}

// region UNEXPORTED METHODS

// region State management

// name returns the manager's name — the user-facing pkg.Resource prefix.
//
// Returns:
//   - `string`: "port".
func (m *portManager) name() string { return "port" }

// purlType returns the manager's purl type — the routing key and the URI type.
//
// Returns:
//   - `string`: "port".
func (m *portManager) purlType() string { return "port" }

// endregion

// endregion
