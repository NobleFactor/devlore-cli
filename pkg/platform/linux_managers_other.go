// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !linux

package platform

// Stub shell-out implementations for the Linux managers (apt, dnf, pacman, systemd) on non-Linux hosts.
//
// These stubs let cross-host plan-time fixtures construct manager instances that satisfy the
// PackageManager / ServiceManager interface. Methods that would shell out on a real Linux host return
// `false` / "" / nil / an error PlatformResult instead — the type is usable at plan time but every
// runtime invocation fails loudly with "<tool> not available on this host (target=linux)". Preflight
// catches target-vs-host mismatches before any provider method is invoked.

const linuxStubMessage = "not available on this host (target=linux)"

// =============================================================================
// APT Package Manager — stub shell-out methods
// =============================================================================

func (m *aptManager) Installed(_ string) bool { return false }

func (m *aptManager) Available(_ string) bool { return false }

func (m *aptManager) Search(_ string, _ int) []SearchResult { return nil }

func (m *aptManager) Version(_ string) string { return "" }

func (m *aptManager) Install(_ ...string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "apt-get " + linuxStubMessage}
}

func (m *aptManager) Remove(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "apt-get " + linuxStubMessage}
}

func (m *aptManager) Update() PlatformResult {
	return PlatformResult{OK: false, Stderr: "apt-get " + linuxStubMessage}
}

func (m *aptManager) AddRepo(_, _, _ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "apt-get " + linuxStubMessage}
}

// =============================================================================
// DNF Package Manager — stub shell-out methods
// =============================================================================

func (m *dnfManager) Installed(_ string) bool { return false }

func (m *dnfManager) Available(_ string) bool { return false }

func (m *dnfManager) Search(_ string, _ int) []SearchResult { return nil }

func (m *dnfManager) Version(_ string) string { return "" }

func (m *dnfManager) Install(_ ...string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "dnf " + linuxStubMessage}
}

func (m *dnfManager) Remove(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "dnf " + linuxStubMessage}
}

func (m *dnfManager) Update() PlatformResult {
	return PlatformResult{OK: false, Stderr: "dnf " + linuxStubMessage}
}

func (m *dnfManager) AddRepo(_, _, _ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "dnf " + linuxStubMessage}
}

// =============================================================================
// Pacman Package Manager — stub shell-out methods
// =============================================================================

func (m *pacmanManager) Installed(_ string) bool { return false }

func (m *pacmanManager) Available(_ string) bool { return false }

func (m *pacmanManager) Search(_ string, _ int) []SearchResult { return nil }

func (m *pacmanManager) Version(_ string) string { return "" }

func (m *pacmanManager) Install(_ ...string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "pacman " + linuxStubMessage}
}

func (m *pacmanManager) Remove(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "pacman " + linuxStubMessage}
}

func (m *pacmanManager) Update() PlatformResult {
	return PlatformResult{OK: false, Stderr: "pacman " + linuxStubMessage}
}

func (m *pacmanManager) AddRepo(_, _, _ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "pacman " + linuxStubMessage}
}

// =============================================================================
// systemd Service Manager — stub shell-out methods
// =============================================================================

func (m *systemdManager) Exists(_ string) bool { return false }

func (m *systemdManager) IsRunning(_ string) bool { return false }

func (m *systemdManager) IsEnabled(_ string) bool { return false }

func (m *systemdManager) Status(_ string) string { return "" }

func (m *systemdManager) Start(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "systemctl " + linuxStubMessage}
}

func (m *systemdManager) Stop(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "systemctl " + linuxStubMessage}
}

func (m *systemdManager) Enable(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "systemctl " + linuxStubMessage}
}

func (m *systemdManager) Disable(_ string) PlatformResult {
	return PlatformResult{OK: false, Stderr: "systemctl " + linuxStubMessage}
}
