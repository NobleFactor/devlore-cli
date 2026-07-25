// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build linux

package platform

import (
	"runtime"
	"testing"
)

// region detectHost (linux)

// TestDetectHostLinuxReturnsKnownDistro exercises the host-resident detection logic on a linux host.
// The host's /etc/os-release ID must resolve to one of the 10 known distros (or one of the aliased
// values). Tests run only when the host is a known distro; on any other Linux flavor the test is
// skipped rather than failed (CI may run on minimal containers with unusual ID values).
func TestDetectHostLinuxReturnsKnownDistro(t *testing.T) {

	spec, err := detectHost()
	if err != nil {
		t.Skipf("detectHost on this linux host: %v (likely an unsupported distro)", err)
	}

	if spec.os != "linux" {
		t.Errorf("os = %q, want linux", spec.os)
	}
	if _, ok := linuxSpecByDistro[spec.distro]; !ok {
		t.Errorf("distro = %q, want one of the known distros", spec.distro)
	}
	if spec.serviceManager == nil {
		t.Error("serviceManager is nil")
	}

	p, err := New(spec)
	if err != nil {
		t.Fatalf("New(detected spec): %v", err)
	}

	if p.Arch() != runtime.GOARCH {
		t.Errorf("Arch() = %q, want %q (runtime.GOARCH)", p.Arch(), runtime.GOARCH)
	}
	if p.PackageManager() == nil {
		t.Error("PackageManager is nil")
	}
	if p.ServiceManager() == nil {
		t.Error("ServiceManager is nil")
	}
}

// endregion

// region linuxDistroAliases

func TestLinuxDistroAliasesContainsExpectedEntries(t *testing.T) {

	for raw, want := range map[string]string{
		"linuxmint": "mint",
		"centos":    "centos-stream",
	} {
		got, ok := linuxDistroAliases[raw]
		if !ok {
			t.Errorf("linuxDistroAliases missing %q", raw)
			continue
		}
		if got != want {
			t.Errorf("linuxDistroAliases[%q] = %q, want %q", raw, got, want)
		}
	}
}

// endregion

// region isServerVariant constant cases

func TestIsServerVariantConstantCases(t *testing.T) {

	for _, tc := range []struct {
		variantID string
		want      bool
	}{
		{"workstation", false},
		{"silverblue", false},
		{"kinoite", false},
		{"iot", false},
		{"cloud", false},
		{"server", true},
		{"coreos", true},
	} {
		t.Run(tc.variantID, func(t *testing.T) {
			if got := isServerVariant(tc.variantID); got != tc.want {
				t.Errorf("isServerVariant(%q) = %v, want %v", tc.variantID, got, tc.want)
			}
		})
	}
}

// endregion

// region stripDesktopOnly

func TestStripDesktopOnlyRemovesFlatpak(t *testing.T) {

	in := []leaf{newAptManager(), newSnapManager(), newFlatpakManager()}

	got := stripDesktopOnly(in)

	names := make(map[string]bool, len(got))
	for _, manager := range got {
		names[manager.name()] = true
	}

	if names["flatpak"] {
		t.Error("stripDesktopOnly retained flatpak")
	}
	if !names["apt"] {
		t.Error("stripDesktopOnly removed apt")
	}
	if !names["snap"] {
		t.Error("stripDesktopOnly removed snap (should be retained — Ubuntu Server pre-installs snapd)")
	}

	if len(in) != 3 {
		t.Error("stripDesktopOnly mutated input slice")
	}
}

func TestStripDesktopOnlyOnEmptySlice(t *testing.T) {

	got := stripDesktopOnly([]leaf{})
	if len(got) != 0 {
		t.Errorf("stripDesktopOnly(empty) = %v, want empty", got)
	}
}

// endregion
