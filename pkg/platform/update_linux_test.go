// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build linux

package platform

import (
	"slices"
	"testing"
)

// TestAptRefreshIssuesElevatedUpdate verifies apt refreshes the index under sudo.
func TestAptRefreshIssuesElevatedUpdate(t *testing.T) {

	argv, sudo := captureRefresh(t, (&aptManager{}).refresh)

	if !slices.Equal(argv, []string{"apt-get", "update"}) || !sudo {
		t.Errorf("apt refresh = (%v, sudo=%v), want (%v, sudo=true)", argv, sudo, []string{"apt-get", "update"})
	}
}

// TestDnfRefreshIssuesElevatedMakecache verifies dnf rebuilds its metadata cache under sudo.
func TestDnfRefreshIssuesElevatedMakecache(t *testing.T) {

	argv, sudo := captureRefresh(t, (&dnfManager{}).refresh)

	if !slices.Equal(argv, []string{"dnf", "makecache"}) || !sudo {
		t.Errorf("dnf refresh = (%v, sudo=%v), want (%v, sudo=true)", argv, sudo, []string{"dnf", "makecache"})
	}
}

// TestPacmanRefreshIssuesElevatedNonInteractiveSync verifies pacman syncs its databases under sudo, non-interactively.
func TestPacmanRefreshIssuesElevatedNonInteractiveSync(t *testing.T) {

	argv, sudo := captureRefresh(t, (&pacmanManager{}).refresh)

	if !slices.Equal(argv, []string{"pacman", "-Sy", "--noconfirm"}) || !sudo {
		t.Errorf("pacman refresh = (%v, sudo=%v), want (%v, sudo=true)", argv, sudo, []string{"pacman", "-Sy", "--noconfirm"})
	}
}
