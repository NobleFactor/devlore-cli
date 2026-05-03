// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"runtime"
	"strings"
	"testing"
)

// region Linux constructor

func TestLinuxKnownDistroBuilds(t *testing.T) {

	for _, distro := range []string{
		"debian", "ubuntu", "mint",
		"rhel", "fedora", "centos-stream", "almalinux", "rocky",
		"arch", "manjaro",
	} {
		t.Run(distro, func(t *testing.T) {

			p, err := Linux(distro, "amd64")
			if err != nil {
				t.Fatalf("Linux(%q, amd64): %v", distro, err)
			}

			if p.OS() != "linux" {
				t.Errorf("OS = %q, want linux", p.OS())
			}
			if p.Distro() != distro {
				t.Errorf("Distro = %q, want %q", p.Distro(), distro)
			}
			if p.Arch() != "amd64" {
				t.Errorf("Arch = %q, want amd64", p.Arch())
			}
			if p.DefaultPackageManager() == nil {
				t.Error("DefaultPackageManager is nil")
			}
		})
	}
}

func TestLinuxUnknownDistroErrors(t *testing.T) {

	_, err := Linux("alpine", "amd64")

	if err == nil {
		t.Fatal("Linux returned nil error for unknown distro")
	}
	if !strings.Contains(err.Error(), "unknown linux distro") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "unknown linux distro")
	}
}

func TestLinuxEmptyArchDefaultsToRuntimeGOARCH(t *testing.T) {

	p, err := Linux("ubuntu", "")
	if err != nil {
		t.Fatalf("Linux: %v", err)
	}
	if p.Arch() != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q (runtime.GOARCH)", p.Arch(), runtime.GOARCH)
	}
}

func TestLinuxUnknownArchErrors(t *testing.T) {

	_, err := Linux("ubuntu", "wasm")

	if err == nil {
		t.Fatal("Linux returned nil error for unknown arch")
	}
	if !strings.Contains(err.Error(), "unknown architecture") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "unknown architecture")
	}
}

// endregion

// region Darwin constructor

func TestDarwinBuildsMacosPlatform(t *testing.T) {

	p, err := Darwin("arm64")
	if err != nil {
		t.Fatalf("Darwin: %v", err)
	}

	if p.OS() != "darwin" {
		t.Errorf("OS = %q, want darwin", p.OS())
	}
	if p.Distro() != "macos" {
		t.Errorf("Distro = %q, want macos", p.Distro())
	}
	if p.Arch() != "arm64" {
		t.Errorf("Arch = %q, want arm64", p.Arch())
	}
	if p.DefaultPackageManager() == nil || p.DefaultPackageManager().Name() != "brew" {
		t.Errorf("DefaultPackageManager Name = %q, want brew", p.DefaultPackageManager().Name())
	}
	if _, ok := p.AvailablePackageManagers()["port"]; !ok {
		t.Error("AvailablePackageManagers missing port")
	}
}

func TestDarwinEmptyArchDefaultsToRuntimeGOARCH(t *testing.T) {

	p, err := Darwin("")
	if err != nil {
		t.Fatalf("Darwin: %v", err)
	}
	if p.Arch() != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", p.Arch(), runtime.GOARCH)
	}
}

// endregion

// region Windows constructor

func TestWindowsBuildsWindowsPlatform(t *testing.T) {

	p, err := Windows("amd64")
	if err != nil {
		t.Fatalf("Windows: %v", err)
	}

	if p.OS() != "windows" {
		t.Errorf("OS = %q, want windows", p.OS())
	}
	if p.Distro() != "windows" {
		t.Errorf("Distro = %q, want windows", p.Distro())
	}
	if p.Arch() != "amd64" {
		t.Errorf("Arch = %q, want amd64", p.Arch())
	}
	if p.DefaultPackageManager() == nil || p.DefaultPackageManager().Name() != "winget" {
		t.Errorf("DefaultPackageManager Name = %q, want winget", p.DefaultPackageManager().Name())
	}
}

// endregion
