// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !windows

package platform

// Stub shell-out implementations for the Windows managers (winget, sc.exe) on non-Windows hosts.
//
// These stubs let cross-host plan-time fixtures construct manager instances that satisfy the
// PackageManager / ServiceManager interface. Methods that would shell out on a real Windows host return
// `false` / "" / nil / an error PlatformResult instead — the type is usable at plan time but every
// runtime invocation fails loudly with "<tool> not available on this host (target=windows)". Preflight
// catches target-vs-host mismatches before any provider method is invoked.

const windowsStubMessage = "not available on this host (target=windows)"

// =============================================================================
// winget Package Manager — stub shell-out methods
// =============================================================================

func (m *wingetManager) Installed(_ string) bool { return false }

func (m *wingetManager) Available(_ string) bool { return false }

func (m *wingetManager) Search(_ string, _ int) []SearchResult { return nil }

func (m *wingetManager) Version(_ string) string { return "" }

func (m *wingetManager) Install(_ ...string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "winget " + windowsStubMessage}
}

func (m *wingetManager) Remove(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "winget " + windowsStubMessage}
}

func (m *wingetManager) Update() PlatformResult {
	return PlatformResult{OK: false, Stderr: "winget " + windowsStubMessage}
}

func (m *wingetManager) AddRepo(_, _, _ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "winget " + windowsStubMessage}
}

// =============================================================================
// Windows Service Manager — stub shell-out methods
// =============================================================================

func (m *windowsServiceManager) Exists(_ string) bool { return false }

func (m *windowsServiceManager) IsRunning(_ string) bool { return false }

func (m *windowsServiceManager) IsEnabled(_ string) bool { return false }

func (m *windowsServiceManager) Status(_ string) string { return "" }

func (m *windowsServiceManager) Start(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "sc " + windowsStubMessage}
}

func (m *windowsServiceManager) Stop(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "sc " + windowsStubMessage}
}

func (m *windowsServiceManager) Enable(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "sc " + windowsStubMessage}
}

func (m *windowsServiceManager) Disable(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "sc " + windowsStubMessage}
}
