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

	m := &aptManager{}
	argv, account := captureRefresh(t, m, m.refresh)

	if !slices.Equal(argv, []string{"apt-get", "update"}) || account != AccountRoot {
		t.Errorf("apt refresh = (%v, account=%s), want (%v, root)", argv, account, []string{"apt-get", "update"})
	}
}

// TestDnfRefreshIssuesElevatedMakecache verifies dnf rebuilds its metadata cache under sudo.
func TestDnfRefreshIssuesElevatedMakecache(t *testing.T) {

	m := &dnfManager{}
	argv, account := captureRefresh(t, m, m.refresh)

	if !slices.Equal(argv, []string{"dnf", "makecache"}) || account != AccountRoot {
		t.Errorf("dnf refresh = (%v, account=%s), want (%v, root)", argv, account, []string{"dnf", "makecache"})
	}
}

// TestPacmanRefreshIssuesElevatedNonInteractiveSync verifies pacman syncs its databases under sudo, non-interactively.
func TestPacmanRefreshIssuesElevatedNonInteractiveSync(t *testing.T) {

	m := &pacmanManager{}
	argv, account := captureRefresh(t, m, m.refresh)

	if !slices.Equal(argv, []string{"pacman", "-Sy", "--noconfirm"}) || account != AccountRoot {
		t.Errorf("pacman refresh = (%v, account=%s), want (%v, root)", argv, account, []string{"pacman", "-Sy", "--noconfirm"})
	}
}
