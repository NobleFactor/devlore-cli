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

	m := &brewManager{}
	argv, account := captureRefresh(t, m, m.refresh)

	if !slices.Equal(argv, []string{"brew", "update"}) || account != AccountCaller {
		t.Errorf("brew refresh = (%v, account=%s), want (%q, caller)", argv, account, []string{"brew", "update"})
	}
}

// TestPortRefreshIssuesElevatedNonInteractiveSelfupdate verifies port refreshes under sudo, non-interactively.
//
// MacPorts lives under /opt/local (root-owned), so the refresh requires elevation; `-N` keeps it non-interactive.
func TestPortRefreshIssuesElevatedNonInteractiveSelfupdate(t *testing.T) {

	m := &portManager{}
	argv, account := captureRefresh(t, m, m.refresh)

	if !slices.Equal(argv, []string{"port", "-N", "selfupdate"}) || account != AccountRoot {
		t.Errorf("port refresh = (%v, account=%s), want (%v, root)", argv, account, []string{"port", "-N", "selfupdate"})
	}
}
