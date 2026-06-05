// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !linux

package platform

// Stub shell-out primitives for the Linux managers (apt, dnf, pacman, systemd) on non-Linux hosts.
//
// These stubs let cross-host plan-time fixtures construct manager instances that satisfy the [leaf] /
// [ServiceManager] contract. Primitives that would shell out on a real Linux host return `false` / "" / nil / an
// error [PlatformResult] instead — usable at plan time but failing loudly at run time with "<tool> not available on
// this host (target=linux)". Preflight catches target-vs-host mismatches before any provider method is invoked.

const linuxStubMessage = "not available on this host (target=linux)"

// =============================================================================
// APT Package Manager — stub primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports false: apt is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *aptManager) available(_ string) bool { return false }

// installRaw fails: apt is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//   - `kwargs`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *aptManager) installRaw(_ []string, _ map[string]any) PlatformResult {
	return PlatformResult{OK: false, Stderr: "apt-get " + linuxStubMessage}
}

// installed reports false: apt is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *aptManager) installed(_ string) bool { return false }

// removeRaw fails: apt is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *aptManager) removeRaw(_ []string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "apt-get " + linuxStubMessage}
}

// searchRaw returns nil: apt is unavailable on this host.
//
// Parameters:
//   - `query`: ignored.
//   - `limit`: ignored.
//
// Returns:
//   - `[]SearchResult`: always nil.
func (m *aptManager) searchRaw(_ string, _ int) []SearchResult { return nil }

// version returns "": apt is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `string`: always "".
func (m *aptManager) version(_ string) string { return "" }

// endregion

// endregion

// =============================================================================
// DNF Package Manager — stub primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports false: dnf is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *dnfManager) available(_ string) bool { return false }

// installRaw fails: dnf is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//   - `kwargs`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *dnfManager) installRaw(_ []string, _ map[string]any) PlatformResult {
	return PlatformResult{OK: false, Stderr: "dnf " + linuxStubMessage}
}

// installed reports false: dnf is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *dnfManager) installed(_ string) bool { return false }

// removeRaw fails: dnf is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *dnfManager) removeRaw(_ []string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "dnf " + linuxStubMessage}
}

// searchRaw returns nil: dnf is unavailable on this host.
//
// Parameters:
//   - `query`: ignored.
//   - `limit`: ignored.
//
// Returns:
//   - `[]SearchResult`: always nil.
func (m *dnfManager) searchRaw(_ string, _ int) []SearchResult { return nil }

// version returns "": dnf is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `string`: always "".
func (m *dnfManager) version(_ string) string { return "" }

// endregion

// endregion

// =============================================================================
// Pacman Package Manager — stub primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports false: pacman is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *pacmanManager) available(_ string) bool { return false }

// installRaw fails: pacman is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//   - `kwargs`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *pacmanManager) installRaw(_ []string, _ map[string]any) PlatformResult {
	return PlatformResult{OK: false, Stderr: "pacman " + linuxStubMessage}
}

// installed reports false: pacman is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *pacmanManager) installed(_ string) bool { return false }

// removeRaw fails: pacman is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *pacmanManager) removeRaw(_ []string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "pacman " + linuxStubMessage}
}

// searchRaw returns nil: pacman is unavailable on this host.
//
// Parameters:
//   - `query`: ignored.
//   - `limit`: ignored.
//
// Returns:
//   - `[]SearchResult`: always nil.
func (m *pacmanManager) searchRaw(_ string, _ int) []SearchResult { return nil }

// version returns "": pacman is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `string`: always "".
func (m *pacmanManager) version(_ string) string { return "" }

// endregion

// endregion

// =============================================================================
// systemd Service Manager — stub methods
// =============================================================================

// region EXPORTED METHODS

// region Behaviors

// Disable fails: systemd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *systemdManager) Disable(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "systemctl " + linuxStubMessage}
}

// Enable fails: systemd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *systemdManager) Enable(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "systemctl " + linuxStubMessage}
}

// Exists reports false: systemd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *systemdManager) Exists(_ string) bool { return false }

// IsEnabled reports false: systemd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *systemdManager) IsEnabled(_ string) bool { return false }

// IsRunning reports false: systemd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *systemdManager) IsRunning(_ string) bool { return false }

// Start fails: systemd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *systemdManager) Start(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "systemctl " + linuxStubMessage}
}

// Status returns "": systemd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `string`: always "".
func (m *systemdManager) Status(_ string) string { return "" }

// Stop fails: systemd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *systemdManager) Stop(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "systemctl " + linuxStubMessage}
}

// endregion

// endregion
