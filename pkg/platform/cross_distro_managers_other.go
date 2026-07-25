// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !linux

package platform

// Stub shell-out primitives for the cross-distro Linux managers (snap, flatpak) on non-Linux hosts.
//
// These stubs let cross-host plan-time fixtures construct manager instances that satisfy the [leaf] contract.
// Primitives that would shell out on a real Linux host return `false` / "" / nil / an error [PlatformResult]
// instead — usable at plan time but failing loudly at run time. snap and flatpak share the linux stub message
// (defined in linux_managers_other.go) because both tools are Linux-only at run time.

// =============================================================================
// flatpak Package Manager — stub primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports false: flatpak is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *flatpakManager) available(_ string) bool { return false }

// installRaw fails: flatpak is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//   - `kwargs`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *flatpakManager) installRaw(_ []string, _ map[string]any) PlatformResult {
	return PlatformResult{OK: false, Stderr: "flatpak " + linuxStubMessage}
}

// installed reports false: flatpak is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *flatpakManager) installed(_ string) bool { return false }

// removeRaw fails: flatpak is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *flatpakManager) removeRaw(_ []string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "flatpak " + linuxStubMessage}
}

// searchRaw returns nil: flatpak is unavailable on this host.
//
// Parameters:
//   - `query`: ignored.
//   - `limit`: ignored.
//
// Returns:
//   - `[]SearchResult`: always nil.
func (m *flatpakManager) searchRaw(_ string, _ int) []SearchResult { return nil }

// version returns "": flatpak is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `string`: always "".
func (m *flatpakManager) version(_ string) string { return "" }

// endregion

// endregion

// =============================================================================
// snap Package Manager — stub primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports false: snap is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *snapManager) available(_ string) bool { return false }

// installRaw fails: snap is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//   - `kwargs`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *snapManager) installRaw(_ []string, _ map[string]any) PlatformResult {
	return PlatformResult{OK: false, Stderr: "snap " + linuxStubMessage}
}

// installed reports false: snap is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *snapManager) installed(_ string) bool { return false }

// removeRaw fails: snap is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *snapManager) removeRaw(_ []string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "snap " + linuxStubMessage}
}

// searchRaw returns nil: snap is unavailable on this host.
//
// Parameters:
//   - `query`: ignored.
//   - `limit`: ignored.
//
// Returns:
//   - `[]SearchResult`: always nil.
func (m *snapManager) searchRaw(_ string, _ int) []SearchResult { return nil }

// version returns "": snap is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `string`: always "".
func (m *snapManager) version(_ string) string { return "" }

// endregion

// endregion
