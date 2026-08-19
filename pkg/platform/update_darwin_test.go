// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build darwin

package platform

import (
	"slices"
	"testing"
)

// TestBrewRefreshIssuesUnelevatedUpdate verifies brew refreshes without sudo (Homebrew is user-owned).
func TestBrewRefreshIssuesUnelevatedUpdate(t *testing.T) {

	argv, sudo := captureRefresh(t, (&brewManager{}).refresh)

	if !slices.Equal(argv, []string{"brew", "update"}) || sudo {
		t.Errorf("brew refresh = (%v, sudo=%v), want (%q, sudo=false)", argv, sudo, []string{"brew", "update"})
	}
}

// TestPortRefreshIssuesElevatedNonInteractiveSelfupdate verifies port refreshes under sudo, non-interactively.
//
// MacPorts lives under /opt/local (root-owned), so the refresh requires elevation; `-N` keeps it non-interactive.
func TestPortRefreshIssuesElevatedNonInteractiveSelfupdate(t *testing.T) {

	argv, sudo := captureRefresh(t, (&portManager{}).refresh)

	if !slices.Equal(argv, []string{"port", "-N", "selfupdate"}) || !sudo {
		t.Errorf("port refresh = (%v, sudo=%v), want (%v, sudo=true)", argv, sudo, []string{"port", "-N", "selfupdate"})
	}
}
