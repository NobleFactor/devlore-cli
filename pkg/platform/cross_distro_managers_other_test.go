// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !linux

package platform

import (
	"strings"
	"testing"
)

// Stub-method coverage for the cross-distro Linux managers (snap, flatpak) when the host is non-Linux.

// region snap stubs

func TestSnapStubsReturnNotAvailable(t *testing.T) {

	m := &snapManager{}

	if m.Installed("firefox") {
		t.Error("Installed should be false on non-Linux host")
	}
	if m.Available("firefox") {
		t.Error("Available should be false on non-Linux host")
	}
	if got := m.Search("firefox", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version("firefox"); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	for name, result := range map[string]PlatformResult{
		"Install": m.Install("firefox"),
		"Remove":  m.Remove("firefox"),
		"Update":  m.Update(),
		"AddRepo": m.AddRepo("ignored", "ignored", "ignored"),
	} {
		if result.OK {
			t.Errorf("%s.OK = true on non-Linux host", name)
		}
		if !strings.Contains(result.Stderr, "snap") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "snap")
		}
		if !strings.Contains(result.Stderr, "target=linux") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "target=linux")
		}
	}
}

// endregion

// region flatpak stubs

func TestFlatpakStubsReturnNotAvailable(t *testing.T) {

	m := &flatpakManager{}

	if m.Installed("org.gimp.GIMP") {
		t.Error("Installed should be false on non-Linux host")
	}
	if m.Available("org.gimp.GIMP") {
		t.Error("Available should be false on non-Linux host")
	}
	if got := m.Search("gimp", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version("org.gimp.GIMP"); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	for name, result := range map[string]PlatformResult{
		"Install": m.Install("org.gimp.GIMP"),
		"Remove":  m.Remove("org.gimp.GIMP"),
		"Update":  m.Update(),
		"AddRepo": m.AddRepo("https://flathub.org/repo/flathub.flatpakrepo", "", "flathub"),
	} {
		if result.OK {
			t.Errorf("%s.OK = true on non-Linux host", name)
		}
		if !strings.Contains(result.Stderr, "flatpak") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "flatpak")
		}
		if !strings.Contains(result.Stderr, "target=linux") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "target=linux")
		}
	}
}

// endregion
