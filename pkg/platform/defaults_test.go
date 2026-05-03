// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"sort"
	"testing"
)

// TestDefaultLinuxPlatformsTableMatchesPlan verifies the per-distro default-PM table from the
// 13.0(i) plan-row is encoded correctly in defaultLinuxPlatforms. Source of truth:
//
//	debian        -> apt              | apt
//	ubuntu        -> apt              | apt, snap
//	mint          -> apt              | apt, flatpak
//	rhel          -> dnf              | dnf, flatpak
//	fedora        -> dnf              | dnf, flatpak
//	centos-stream -> dnf              | dnf, flatpak
//	almalinux     -> dnf              | dnf, flatpak
//	rocky         -> dnf              | dnf, flatpak
//	arch          -> pacman           | pacman
//	manjaro       -> pacman           | pacman, snap, flatpak
func TestDefaultLinuxPlatformsTableMatchesPlan(t *testing.T) {

	for _, tc := range []struct {
		distro    string
		defaultPM string
		available []string
	}{
		{"debian", "apt", []string{"apt"}},
		{"ubuntu", "apt", []string{"apt", "snap"}},
		{"mint", "apt", []string{"apt", "flatpak"}},
		{"rhel", "dnf", []string{"dnf", "flatpak"}},
		{"fedora", "dnf", []string{"dnf", "flatpak"}},
		{"centos-stream", "dnf", []string{"dnf", "flatpak"}},
		{"almalinux", "dnf", []string{"dnf", "flatpak"}},
		{"rocky", "dnf", []string{"dnf", "flatpak"}},
		{"arch", "pacman", []string{"pacman"}},
		{"manjaro", "pacman", []string{"pacman", "snap", "flatpak"}},
	} {
		t.Run(tc.distro, func(t *testing.T) {

			factory, ok := defaultLinuxPlatforms[tc.distro]
			if !ok {
				t.Fatalf("defaultLinuxPlatforms missing entry for %q", tc.distro)
			}

			spec := factory()

			if spec.os != "linux" {
				t.Errorf("os = %q, want linux", spec.os)
			}
			if spec.distro != tc.distro {
				t.Errorf("distro = %q, want %q", spec.distro, tc.distro)
			}
			if spec.defaultPackageManager == nil {
				t.Fatal("defaultPackageManager is nil")
			}
			if spec.defaultPackageManager.Name() != tc.defaultPM {
				t.Errorf("defaultPackageManager.Name() = %q, want %q",
					spec.defaultPackageManager.Name(), tc.defaultPM)
			}

			gotAvailable := make([]string, 0, len(spec.availablePackageManagers))
			for name := range spec.availablePackageManagers {
				gotAvailable = append(gotAvailable, name)
			}
			sort.Strings(gotAvailable)

			wantAvailable := append([]string{}, tc.available...)
			sort.Strings(wantAvailable)

			if !equalSlices(gotAvailable, wantAvailable) {
				t.Errorf("availablePackageManagers keys = %v, want %v", gotAvailable, wantAvailable)
			}

			// systemd is the canonical Linux service manager for every distro in the table.
			if spec.serviceManager == nil {
				t.Error("serviceManager is nil")
			}
		})
	}
}

func TestDefaultLinuxPlatformsHasAllTenDistros(t *testing.T) {

	if len(defaultLinuxPlatforms) != 10 {
		t.Errorf("defaultLinuxPlatforms has %d distros, want 10", len(defaultLinuxPlatforms))
	}
}

func TestDefaultLinuxPlatformsFactoriesProduceFreshSpecs(t *testing.T) {

	// Two consecutive factory calls must produce independent specs — a With* mutation on one must
	// not leak into the other.
	a := defaultLinuxPlatforms["ubuntu"]()
	b := defaultLinuxPlatforms["ubuntu"]()

	if a == b {
		t.Fatal("factory returned identical spec pointers; expected fresh allocations")
	}

	a.WithVersion("22.04")
	if b.version == "22.04" {
		t.Error("WithVersion mutation on a leaked into b — factories share state")
	}

	if &a.availablePackageManagers == &b.availablePackageManagers {
		t.Error("availablePackageManagers map shared across factory calls")
	}
}

func TestNewDarwinDefault(t *testing.T) {

	spec := newDarwinDefault()

	if spec.os != "darwin" {
		t.Errorf("os = %q, want darwin", spec.os)
	}
	if spec.distro != "macos" {
		t.Errorf("distro = %q, want macos", spec.distro)
	}
	if spec.defaultPackageManager == nil || spec.defaultPackageManager.Name() != "brew" {
		t.Errorf("defaultPackageManager = %v, want brew", spec.defaultPackageManager)
	}
	if _, ok := spec.availablePackageManagers["port"]; !ok {
		t.Error("availablePackageManagers missing port")
	}
}

func TestNewWindowsDefault(t *testing.T) {

	spec := newWindowsDefault()

	if spec.os != "windows" {
		t.Errorf("os = %q, want windows", spec.os)
	}
	if spec.distro != "windows" {
		t.Errorf("distro = %q, want windows", spec.distro)
	}
	if spec.defaultPackageManager == nil || spec.defaultPackageManager.Name() != "winget" {
		t.Errorf("defaultPackageManager = %v, want winget", spec.defaultPackageManager)
	}
}

// equalSlices reports whether a and b have the same string elements in the same order.
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
