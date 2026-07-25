// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"sort"
	"testing"
)

// TestLinuxSpecByDistroTableMatchesPlan verifies the per-distro default-PM table from the
// 13.0(i) plan-row is encoded correctly in linuxSpecByDistro. Source of truth:
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
func TestLinuxSpecByDistroTableMatchesPlan(t *testing.T) {

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

			factory, ok := linuxSpecByDistro[tc.distro]
			if !ok {
				t.Fatalf("linuxSpecByDistro missing entry for %q", tc.distro)
			}

			spec := factory()

			if spec.os != "linux" {
				t.Errorf("os = %q, want linux", spec.os)
			}
			if spec.distro != tc.distro {
				t.Errorf("distro = %q, want %q", spec.distro, tc.distro)
			}
			if spec.defaultManager == nil {
				t.Fatal("defaultManager is nil")
			}
			if spec.defaultManager.name() != tc.defaultPM {
				t.Errorf("defaultManager.name() = %q, want %q",
					spec.defaultManager.name(), tc.defaultPM)
			}

			gotAvailable := make([]string, 0, len(spec.managers))
			for _, manager := range spec.managers {
				gotAvailable = append(gotAvailable, manager.name())
			}
			sort.Strings(gotAvailable)

			wantAvailable := append([]string{}, tc.available...)
			sort.Strings(wantAvailable)

			if !equalSlices(gotAvailable, wantAvailable) {
				t.Errorf("manager names = %v, want %v", gotAvailable, wantAvailable)
			}

			// systemd is the canonical Linux service manager for every distro in the table.
			if spec.serviceManager == nil {
				t.Error("serviceManager is nil")
			}
		})
	}
}

func TestLinuxSpecByDistroHasAllTenDistros(t *testing.T) {

	if len(linuxSpecByDistro) != 10 {
		t.Errorf("linuxSpecByDistro has %d distros, want 10", len(linuxSpecByDistro))
	}
}

func TestLinuxSpecByDistroFactoriesProduceFreshSpecs(t *testing.T) {

	// Two consecutive factory calls must produce independent specs — a With* mutation on one must
	// not leak into the other.
	a := linuxSpecByDistro["ubuntu"]()
	b := linuxSpecByDistro["ubuntu"]()

	if a == b {
		t.Fatal("factory returned identical spec pointers; expected fresh allocations")
	}

	a.WithVersion("22.04")
	if b.version == "22.04" {
		t.Error("WithVersion mutation on a leaked into b — factories share state")
	}
}

func TestDarwinDefault(t *testing.T) {

	spec := Darwin()

	if spec.os != "darwin" {
		t.Errorf("os = %q, want darwin", spec.os)
	}
	if spec.distro != "macos" {
		t.Errorf("distro = %q, want macos", spec.distro)
	}
	if spec.defaultManager == nil || spec.defaultManager.name() != "brew" {
		t.Errorf("defaultManager = %v, want brew", spec.defaultManager)
	}
	if !managerNamesContain(spec.managers, "port") {
		t.Error("managers missing port")
	}
}

func TestWindowsDefault(t *testing.T) {

	spec := Windows()

	if spec.os != "windows" {
		t.Errorf("os = %q, want windows", spec.os)
	}
	if spec.distro != "windows" {
		t.Errorf("distro = %q, want windows", spec.distro)
	}
	if spec.defaultManager == nil || spec.defaultManager.name() != "winget" {
		t.Errorf("defaultManager = %v, want winget", spec.defaultManager)
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

// managerNamesContain reports whether any leaf in managers has the given name.
func managerNamesContain(managers []leaf, name string) bool {
	for _, manager := range managers {
		if manager.name() == name {
			return true
		}
	}
	return false
}
