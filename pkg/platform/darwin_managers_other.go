// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !darwin

package platform

// Stub shell-out primitives for the Darwin managers (brew, port, launchd) on non-Darwin hosts.
//
// These stubs let cross-host plan-time fixtures construct manager instances that satisfy the [leaf] /
// [ServiceManager] contract. Primitives that would shell out on a real macOS host return `false` / "" / nil / an
// error [PlatformResult] instead — the type is usable at plan time but every run-time invocation fails loudly with
// "<tool> not available on this host (target=darwin)". Preflight catches target-vs-host mismatches before any
// provider method is invoked.

const darwinStubMessage = "not available on this host (target=darwin)"

// =============================================================================
// Homebrew Package Manager — stub primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports false: Homebrew is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *brewManager) available(_ string) bool { return false }

// installRaw fails: Homebrew is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//   - `kwargs`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *brewManager) installRaw(_ []string, _ map[string]any) PlatformResult {
	return PlatformResult{OK: false, Stderr: "brew " + darwinStubMessage}
}

// installed reports false: Homebrew is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *brewManager) installed(_ string) bool { return false }

// removeRaw fails: Homebrew is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *brewManager) removeRaw(_ []string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "brew " + darwinStubMessage}
}

// searchRaw returns nil: Homebrew is unavailable on this host.
//
// Parameters:
//   - `query`: ignored.
//   - `limit`: ignored.
//
// Returns:
//   - `[]SearchResult`: always nil.
func (m *brewManager) searchRaw(_ string, _ int) []SearchResult { return nil }

// version returns "": Homebrew is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `string`: always "".
func (m *brewManager) version(_ string) string { return "" }

// endregion

// endregion

// =============================================================================
// launchd Service Manager — stub methods
// =============================================================================

// region EXPORTED METHODS

// region Behaviors

// Disable fails: launchd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *launchdManager) Disable(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "launchctl " + darwinStubMessage}
}

// Enable fails: launchd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *launchdManager) Enable(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "launchctl " + darwinStubMessage}
}

// Exists reports false: launchd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *launchdManager) Exists(_ string) bool { return false }

// IsEnabled reports false: launchd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *launchdManager) IsEnabled(_ string) bool { return false }

// IsRunning reports false: launchd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *launchdManager) IsRunning(_ string) bool { return false }

// Start fails: launchd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *launchdManager) Start(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "launchctl " + darwinStubMessage}
}

// Status returns "": launchd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `string`: always "".
func (m *launchdManager) Status(_ string) string { return "" }

// Stop fails: launchd is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *launchdManager) Stop(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "launchctl " + darwinStubMessage}
}

// endregion

// endregion

// =============================================================================
// MacPorts Package Manager — stub primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports false: MacPorts is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *portManager) available(_ string) bool { return false }

// installRaw fails: MacPorts is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//   - `kwargs`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *portManager) installRaw(_ []string, _ map[string]any) PlatformResult {
	return PlatformResult{OK: false, Stderr: "port " + darwinStubMessage}
}

// installed reports false: MacPorts is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *portManager) installed(_ string) bool { return false }

// removeRaw fails: MacPorts is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *portManager) removeRaw(_ []string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "port " + darwinStubMessage}
}

// searchRaw returns nil: MacPorts is unavailable on this host.
//
// Parameters:
//   - `query`: ignored.
//   - `limit`: ignored.
//
// Returns:
//   - `[]SearchResult`: always nil.
func (m *portManager) searchRaw(_ string, _ int) []SearchResult { return nil }

// version returns "": MacPorts is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `string`: always "".
func (m *portManager) version(_ string) string { return "" }

// endregion

// endregion
