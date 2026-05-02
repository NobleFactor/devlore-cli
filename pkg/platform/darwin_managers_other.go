// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !darwin

package platform

// Stub shell-out implementations for the Darwin managers (brew, port, launchd) on non-Darwin hosts.
//
// These stubs let cross-host plan-time fixtures construct manager instances that satisfy the
// PackageManager / ServiceManager interface. Methods that would shell out on a real macOS host return
// `false` / "" / nil / an error PlatformResult instead — the type is usable at plan time but every
// runtime invocation fails loudly with "<tool> not available on this host (target=darwin)". Preflight
// catches target-vs-host mismatches before any provider method is invoked.

const darwinStubMessage = "not available on this host (target=darwin)"

// =============================================================================
// Homebrew Package Manager — stub shell-out methods
// =============================================================================

func (m *brewManager) Installed(_ string) bool { return false }

func (m *brewManager) Available(_ string) bool { return false }

func (m *brewManager) Search(_ string, _ int) []SearchResult { return nil }

func (m *brewManager) Version(_ string) string { return "" }

func (m *brewManager) Install(_ ...string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "brew " + darwinStubMessage}
}

func (m *brewManager) Remove(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "brew " + darwinStubMessage}
}

func (m *brewManager) Update() PlatformResult {
	return PlatformResult{OK: false, Stderr: "brew " + darwinStubMessage}
}

func (m *brewManager) AddRepo(_, _, _ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "brew " + darwinStubMessage}
}

// =============================================================================
// MacPorts Package Manager — stub shell-out methods
// =============================================================================

func (m *portManager) Installed(_ string) bool { return false }

func (m *portManager) Available(_ string) bool { return false }

func (m *portManager) Search(_ string, _ int) []SearchResult { return nil }

func (m *portManager) Version(_ string) string { return "" }

func (m *portManager) Install(_ ...string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "port " + darwinStubMessage}
}

func (m *portManager) Remove(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "port " + darwinStubMessage}
}

func (m *portManager) Update() PlatformResult {
	return PlatformResult{OK: false, Stderr: "port " + darwinStubMessage}
}

func (m *portManager) AddRepo(_, _, _ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "port " + darwinStubMessage}
}

// =============================================================================
// launchd Service Manager — stub shell-out methods
// =============================================================================

func (m *launchdManager) Exists(_ string) bool { return false }

func (m *launchdManager) IsRunning(_ string) bool { return false }

func (m *launchdManager) IsEnabled(_ string) bool { return false }

func (m *launchdManager) Status(_ string) string { return "" }

func (m *launchdManager) Start(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "launchctl " + darwinStubMessage}
}

func (m *launchdManager) Stop(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "launchctl " + darwinStubMessage}
}

func (m *launchdManager) Enable(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "launchctl " + darwinStubMessage}
}

func (m *launchdManager) Disable(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "launchctl " + darwinStubMessage}
}
