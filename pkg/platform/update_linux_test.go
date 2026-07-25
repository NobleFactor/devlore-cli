// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build linux

package platform

import "testing"

// TestAptRefreshIssuesElevatedUpdate verifies apt refreshes the index under sudo.
func TestAptRefreshIssuesElevatedUpdate(t *testing.T) {

	cmd, sudo := captureRefresh(t, (&aptManager{}).refresh)

	if cmd != "apt-get update" || !sudo {
		t.Errorf("apt refresh = (%q, sudo=%v), want (%q, sudo=true)", cmd, sudo, "apt-get update")
	}
}

// TestDnfRefreshIssuesElevatedMakecache verifies dnf rebuilds its metadata cache under sudo.
func TestDnfRefreshIssuesElevatedMakecache(t *testing.T) {

	cmd, sudo := captureRefresh(t, (&dnfManager{}).refresh)

	if cmd != "dnf makecache" || !sudo {
		t.Errorf("dnf refresh = (%q, sudo=%v), want (%q, sudo=true)", cmd, sudo, "dnf makecache")
	}
}

// TestPacmanRefreshIssuesElevatedNonInteractiveSync verifies pacman syncs its databases under sudo, non-interactively.
func TestPacmanRefreshIssuesElevatedNonInteractiveSync(t *testing.T) {

	cmd, sudo := captureRefresh(t, (&pacmanManager{}).refresh)

	if cmd != "pacman -Sy --noconfirm" || !sudo {
		t.Errorf("pacman refresh = (%q, sudo=%v), want (%q, sudo=true)", cmd, sudo, "pacman -Sy --noconfirm")
	}
}
