// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !linux

package platform

import (
	"strings"
	"testing"
)

// Stub-method coverage for the cross-distro Linux managers (snap, flatpak) when the host is non-Linux.
// The query methods report empty/false and Install surfaces a failing receipt naming the missing tool
// and target=linux. snap and flatpak query a live store rather than a local index, so they are not
// refreshers: Update is a no-op (nil error) even on a non-native host.

// region snap stubs

func TestSnapStubsReturnNotAvailable(t *testing.T) {

	m := newSnapManager()
	pkg := PURL{Type: "snap", Name: "firefox"}

	if m.Installed(pkg) {
		t.Error("Installed should be false on non-Linux host")
	}
	if m.Available(pkg) {
		t.Error("Available should be false on non-Linux host")
	}
	if got := m.Search("firefox", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version(pkg); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	receipts, err := m.Install([]PURL{pkg}, nil)
	if err == nil {
		t.Error("Install returned nil error on non-Linux host")
	}
	if len(receipts) != 1 || receipts[0].Err == nil {
		t.Fatalf("Install receipts = %v, want one failed receipt", receipts)
	}
	if msg := receipts[0].Err.Error(); !strings.Contains(msg, "snap") || !strings.Contains(msg, "target=linux") {
		t.Errorf("Install receipt Err = %q, want substrings %q and %q", msg, "snap", "target=linux")
	}

	// snap has no local index to refresh, so Update is a no-op on every host.
	if updateErr := m.Update(); updateErr != nil {
		t.Errorf("Update returned %v, want nil (snap is not a refresher)", updateErr)
	}
}

// endregion

// region flatpak stubs

func TestFlatpakStubsReturnNotAvailable(t *testing.T) {

	m := newFlatpakManager()
	pkg := PURL{Type: "flatpak", Name: "org.gimp.GIMP"}

	if m.Installed(pkg) {
		t.Error("Installed should be false on non-Linux host")
	}
	if m.Available(pkg) {
		t.Error("Available should be false on non-Linux host")
	}
	if got := m.Search("gimp", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version(pkg); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	receipts, err := m.Install([]PURL{pkg}, nil)
	if err == nil {
		t.Error("Install returned nil error on non-Linux host")
	}
	if len(receipts) != 1 || receipts[0].Err == nil {
		t.Fatalf("Install receipts = %v, want one failed receipt", receipts)
	}
	if msg := receipts[0].Err.Error(); !strings.Contains(msg, "flatpak") || !strings.Contains(msg, "target=linux") {
		t.Errorf("Install receipt Err = %q, want substrings %q and %q", msg, "flatpak", "target=linux")
	}

	// flatpak has no local index to refresh, so Update is a no-op on every host.
	if updateErr := m.Update(); updateErr != nil {
		t.Errorf("Update returned %v, want nil (flatpak is not a refresher)", updateErr)
	}
}

// endregion
