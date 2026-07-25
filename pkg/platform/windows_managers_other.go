// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !windows

package platform

// Stub primitives for the Windows managers (winget, sc.exe) on non-Windows hosts.
//
// These stubs let cross-host plan-time fixtures construct manager instances that satisfy the [leaf] /
// [ServiceManager] contract. Primitives that would shell out on a real Windows host return `false` / "" / nil / an
// error [PlatformResult] instead — usable at plan time but failing loudly at run time. Preflight catches
// target-vs-host mismatches before any provider method is invoked.

const windowsStubMessage = "not available on this host (target=windows)"

// =============================================================================
// Windows Service Manager — stub methods
// =============================================================================

// region EXPORTED METHODS

// region Behaviors

// Disable fails: Service Control Manager is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *windowsServiceManager) Disable(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "sc " + windowsStubMessage}
}

// Enable fails: Service Control Manager is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *windowsServiceManager) Enable(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "sc " + windowsStubMessage}
}

// Exists reports false: Service Control Manager is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *windowsServiceManager) Exists(_ string) bool { return false }

// IsEnabled reports false: Service Control Manager is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *windowsServiceManager) IsEnabled(_ string) bool { return false }

// IsRunning reports false: Service Control Manager is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *windowsServiceManager) IsRunning(_ string) bool { return false }

// Start fails: Service Control Manager is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *windowsServiceManager) Start(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "sc " + windowsStubMessage}
}

// Status returns "": Service Control Manager is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `string`: always "".
func (m *windowsServiceManager) Status(_ string) string { return "" }

// Stop fails: Service Control Manager is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *windowsServiceManager) Stop(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "sc " + windowsStubMessage}
}

// endregion

// endregion

// =============================================================================
// winget Package Manager — stub primitives
// =============================================================================

// region UNEXPORTED METHODS

// region Behaviors

// available reports false: winget is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *wingetManager) available(_ string) bool { return false }

// installRaw fails: winget is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//   - `kwargs`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *wingetManager) installRaw(_ []string, _ map[string]any) PlatformResult {
	return PlatformResult{OK: false, Stderr: "winget " + windowsStubMessage}
}

// installed reports false: winget is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `bool`: always false.
func (m *wingetManager) installed(_ string) bool { return false }

// removeRaw fails: winget is unavailable on this host.
//
// Parameters:
//   - `names`: ignored.
//
// Returns:
//   - `PlatformResult`: an error result naming the missing tool.
func (m *wingetManager) removeRaw(_ []string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "winget " + windowsStubMessage}
}

// searchRaw returns nil: winget is unavailable on this host.
//
// Parameters:
//   - `query`: ignored.
//   - `limit`: ignored.
//
// Returns:
//   - `[]SearchResult`: always nil.
func (m *wingetManager) searchRaw(_ string, _ int) []SearchResult { return nil }

// version returns "": winget is unavailable on this host.
//
// Parameters:
//   - `name`: ignored.
//
// Returns:
//   - `string`: always "".
func (m *wingetManager) version(_ string) string { return "" }

// endregion

// endregion
