// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !linux

package platform

// Stub shell-out implementations for the cross-distro Linux managers (snap, flatpak) on non-Linux
// hosts.
//
// These stubs let cross-host plan-time fixtures construct manager instances that satisfy the
// PackageManager interface. Methods that would shell out on a real Linux host return `false` / "" /
// nil / an error PlatformResult instead — the type is usable at plan time but every runtime invocation
// fails loudly with "<tool> not available on this host (target=linux)". snap and flatpak share the
// same target stub message because both tools are Linux-only at runtime.

// =============================================================================
// snap Package Manager — stub shell-out methods
// =============================================================================

func (m *snapManager) Installed(_ string) bool { return false }

func (m *snapManager) Available(_ string) bool { return false }

func (m *snapManager) Search(_ string, _ int) []SearchResult { return nil }

func (m *snapManager) Version(_ string) string { return "" }

func (m *snapManager) Install(_ ...string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "snap " + linuxStubMessage}
}

func (m *snapManager) Remove(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "snap " + linuxStubMessage}
}

func (m *snapManager) Update() PlatformResult {
	return PlatformResult{OK: false, Stderr: "snap " + linuxStubMessage}
}

func (m *snapManager) AddRepo(_, _, _ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "snap " + linuxStubMessage}
}

// =============================================================================
// flatpak Package Manager — stub shell-out methods
// =============================================================================

func (m *flatpakManager) Installed(_ string) bool { return false }

func (m *flatpakManager) Available(_ string) bool { return false }

func (m *flatpakManager) Search(_ string, _ int) []SearchResult { return nil }

func (m *flatpakManager) Version(_ string) string { return "" }

func (m *flatpakManager) Install(_ ...string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "flatpak " + linuxStubMessage}
}

func (m *flatpakManager) Remove(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "flatpak " + linuxStubMessage}
}

func (m *flatpakManager) Update() PlatformResult {
	return PlatformResult{OK: false, Stderr: "flatpak " + linuxStubMessage}
}

func (m *flatpakManager) AddRepo(_, _, _ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "flatpak " + linuxStubMessage}
}
