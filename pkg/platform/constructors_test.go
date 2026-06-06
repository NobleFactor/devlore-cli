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

			factory, ok := linuxSpecByDistro[distro]
			if !ok {
				t.Fatalf("linuxSpecByDistro missing entry for %q", distro)
			}

			p, err := New(factory().WithArch("amd64"))
			if err != nil {
				t.Fatalf("New(%q spec, amd64): %v", distro, err)
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
			if p.PackageManager() == nil {
				t.Error("PackageManager is nil")
			}
		})
	}
}

func TestLinuxUnknownDistroErrors(t *testing.T) {

	_, err := New(&Spec{os: "linux", distro: "alpine", arch: "amd64"})

	if err == nil {
		t.Fatal("New returned nil error for unknown distro")
	}
	if !strings.Contains(err.Error(), "unknown linux distro") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "unknown linux distro")
	}
}

func TestLinuxEmptyArchDefaultsToRuntimeGOARCH(t *testing.T) {

	p, err := New(Ubuntu().WithArch(""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Arch() != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q (runtime.GOARCH)", p.Arch(), runtime.GOARCH)
	}
}

func TestLinuxUnknownArchErrors(t *testing.T) {

	_, err := New(Ubuntu().WithArch("wasm"))

	if err == nil {
		t.Fatal("New returned nil error for unknown arch")
	}
	if !strings.Contains(err.Error(), "unknown architecture") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "unknown architecture")
	}
}

// endregion

// region Darwin constructor

func TestDarwinBuildsMacosPlatform(t *testing.T) {

	p, err := New(Darwin().WithArch("arm64"))
	if err != nil {
		t.Fatalf("New: %v", err)
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
	if p.DefaultPurlType() != "brew" {
		t.Errorf("DefaultPurlType = %q, want brew", p.DefaultPurlType())
	}
	if got, ok := p.ResolvePurlType("port"); !ok || got != "port" {
		t.Errorf("ResolvePurlType(port) = (%q, %v), want (port, true)", got, ok)
	}
}

func TestDarwinEmptyArchDefaultsToRuntimeGOARCH(t *testing.T) {

	p, err := New(Darwin().WithArch(""))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Arch() != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", p.Arch(), runtime.GOARCH)
	}
}

// endregion

// region Windows constructor

func TestWindowsBuildsWindowsPlatform(t *testing.T) {

	p, err := New(Windows().WithArch("amd64"))
	if err != nil {
		t.Fatalf("New: %v", err)
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
	if p.DefaultPurlType() != "winget" {
		t.Errorf("DefaultPurlType = %q, want winget", p.DefaultPurlType())
	}
}

// endregion
