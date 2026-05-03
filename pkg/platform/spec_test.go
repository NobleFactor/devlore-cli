// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"runtime"
	"strings"
	"testing"
)

// region PlatformSpec.Build validation — error paths

func TestBuildErrorsOnMissingOS(t *testing.T) {

	spec := &PlatformSpec{distro: "ubuntu", arch: "amd64"}

	_, err := spec.Build()

	if err == nil {
		t.Fatal("Build returned nil error, want missing-OS error")
	}
	if !strings.Contains(err.Error(), "missing OS") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "missing OS")
	}
}

func TestBuildErrorsOnUnknownOS(t *testing.T) {

	spec := &PlatformSpec{os: "freebsd", distro: "ubuntu", arch: "amd64"}

	_, err := spec.Build()

	if err == nil {
		t.Fatal("Build returned nil error, want unknown-OS error")
	}
	if !strings.Contains(err.Error(), "unknown OS") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "unknown OS")
	}
}

func TestBuildErrorsOnUnknownArch(t *testing.T) {

	spec := &PlatformSpec{os: "linux", distro: "ubuntu", arch: "wasm"}

	_, err := spec.Build()

	if err == nil {
		t.Fatal("Build returned nil error, want unknown-arch error")
	}
	if !strings.Contains(err.Error(), "unknown architecture") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "unknown architecture")
	}
}

func TestBuildErrorsOnMissingDistro(t *testing.T) {

	spec := &PlatformSpec{os: "linux", arch: "amd64"}

	_, err := spec.Build()

	if err == nil {
		t.Fatal("Build returned nil error, want missing-distro error")
	}
	if !strings.Contains(err.Error(), "missing distro") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "missing distro")
	}
}

func TestBuildErrorsOnUnknownLinuxDistro(t *testing.T) {

	spec := &PlatformSpec{os: "linux", distro: "alpine", arch: "amd64"}

	_, err := spec.Build()

	if err == nil {
		t.Fatal("Build returned nil error, want unknown-linux-distro error")
	}
	if !strings.Contains(err.Error(), "unknown linux distro") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "unknown linux distro")
	}
}

func TestBuildErrorsOnDarwinDistroNotMacos(t *testing.T) {

	spec := &PlatformSpec{os: "darwin", distro: "ubuntu", arch: "amd64"}

	_, err := spec.Build()

	if err == nil {
		t.Fatal("Build returned nil error, want darwin-distro error")
	}
	if !strings.Contains(err.Error(), "darwin distro must be") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "darwin distro must be")
	}
}

func TestBuildErrorsOnWindowsDistroNotWindows(t *testing.T) {

	spec := &PlatformSpec{os: "windows", distro: "ubuntu", arch: "amd64"}

	_, err := spec.Build()

	if err == nil {
		t.Fatal("Build returned nil error, want windows-distro error")
	}
	if !strings.Contains(err.Error(), "windows distro must be") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "windows distro must be")
	}
}

func TestBuildErrorsWhenDefaultPMNotInAvailable(t *testing.T) {

	apt := &aptManager{}
	dnf := &dnfManager{}
	spec := &PlatformSpec{
		os:                    "linux",
		distro:                "ubuntu",
		arch:                  "amd64",
		defaultPackageManager: dnf,
		availablePackageManagers: map[string]PackageManager{
			apt.Name(): apt, // dnf is the default but only apt is in the available map
		},
	}

	_, err := spec.Build()

	if err == nil {
		t.Fatal("Build returned nil error, want default-not-in-available error")
	}
	if !strings.Contains(err.Error(), "not in available set") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "not in available set")
	}
}

// endregion

// region PlatformSpec.Build success paths

func TestBuildEmptyArchDefaultsToRuntimeGOARCH(t *testing.T) {

	spec := defaultLinuxPlatforms["ubuntu"]()
	spec.WithArch("")

	p, err := spec.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if p.Arch() != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q (runtime.GOARCH)", p.Arch(), runtime.GOARCH)
	}
}

func TestBuildPopulatesPlatformFields(t *testing.T) {

	spec := defaultLinuxPlatforms["ubuntu"]()
	spec.WithArch("amd64").
		WithVersion("22.04").
		WithHostname("dev-box").
		WithDefaultConcurrency(8)

	p, err := spec.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if p.OS() != "linux" {
		t.Errorf("OS = %q, want linux", p.OS())
	}
	if p.Arch() != "amd64" {
		t.Errorf("Arch = %q, want amd64", p.Arch())
	}
	if p.Distro() != "ubuntu" {
		t.Errorf("Distro = %q, want ubuntu", p.Distro())
	}
	if p.Version() != "22.04" {
		t.Errorf("Version = %q, want 22.04", p.Version())
	}
	if p.Hostname() != "dev-box" {
		t.Errorf("Hostname = %q, want dev-box", p.Hostname())
	}
	if p.DefaultConcurrency() != 8 {
		t.Errorf("DefaultConcurrency = %d, want 8", p.DefaultConcurrency())
	}
}

// endregion

// region With* chain methods

func TestWith_methodsReturnReceiver(t *testing.T) {

	spec := &PlatformSpec{}

	if spec.WithArch("amd64") != spec {
		t.Error("WithArch did not return receiver")
	}
	if spec.WithVersion("1.0") != spec {
		t.Error("WithVersion did not return receiver")
	}
	if spec.WithHostname("h") != spec {
		t.Error("WithHostname did not return receiver")
	}
	if spec.WithDefaultConcurrency(4) != spec {
		t.Error("WithDefaultConcurrency did not return receiver")
	}
	if spec.WithDefaultPackageManager(&aptManager{}) != spec {
		t.Error("WithDefaultPackageManager did not return receiver")
	}
	if spec.WithAvailablePackageManagers(map[string]PackageManager{}) != spec {
		t.Error("WithAvailablePackageManagers did not return receiver")
	}
	if spec.WithServiceManager(&systemdManager{}) != spec {
		t.Error("WithServiceManager did not return receiver")
	}
}

// endregion
