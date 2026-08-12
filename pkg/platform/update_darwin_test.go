// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build darwin

package platform

import "testing"

// TestBrewRefreshIssuesUnelevatedUpdate verifies brew refreshes without sudo (Homebrew is user-owned).
func TestBrewRefreshIssuesUnelevatedUpdate(t *testing.T) {

	cmd, sudo := captureRefresh(t, (&brewManager{}).refresh)

	if cmd != "brew update" || sudo {
		t.Errorf("brew refresh = (%q, sudo=%v), want (%q, sudo=false)", cmd, sudo, "brew update")
	}
}

// TestPortRefreshIssuesElevatedNonInteractiveSelfupdate verifies port refreshes under sudo, non-interactively.
//
// MacPorts lives under /opt/local (root-owned), so the refresh requires elevation; `-N` keeps it non-interactive.
func TestPortRefreshIssuesElevatedNonInteractiveSelfupdate(t *testing.T) {

	cmd, sudo := captureRefresh(t, (&portManager{}).refresh)

	if cmd != "port -N selfupdate" || !sudo {
		t.Errorf("port refresh = (%q, sudo=%v), want (%q, sudo=true)", cmd, sudo, "port -N selfupdate")
	}
}
