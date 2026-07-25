// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"runtime"
	"strings"
	"testing"
)

// region New validation — error paths

func TestBuildErrorsOnMissingOS(t *testing.T) {

	spec := &Spec{distro: "ubuntu", arch: "amd64"}

	_, err := New(spec)

	if err == nil {
		t.Fatal("New returned nil error, want missing-OS error")
	}
	if !strings.Contains(err.Error(), "missing OS") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "missing OS")
	}
}

func TestBuildErrorsOnUnknownOS(t *testing.T) {

	spec := &Spec{os: "freebsd", distro: "ubuntu", arch: "amd64"}

	_, err := New(spec)

	if err == nil {
		t.Fatal("New returned nil error, want unknown-OS error")
	}
	if !strings.Contains(err.Error(), "unknown OS") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "unknown OS")
	}
}

func TestBuildErrorsOnUnknownArch(t *testing.T) {

	spec := &Spec{os: "linux", distro: "ubuntu", arch: "wasm"}

	_, err := New(spec)

	if err == nil {
		t.Fatal("New returned nil error, want unknown-arch error")
	}
	if !strings.Contains(err.Error(), "unknown architecture") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "unknown architecture")
	}
}

func TestBuildErrorsOnMissingDistro(t *testing.T) {

	spec := &Spec{os: "linux", arch: "amd64"}

	_, err := New(spec)

	if err == nil {
		t.Fatal("New returned nil error, want missing-distro error")
	}
	if !strings.Contains(err.Error(), "missing distro") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "missing distro")
	}
}

func TestBuildErrorsOnUnknownLinuxDistro(t *testing.T) {

	spec := &Spec{os: "linux", distro: "alpine", arch: "amd64"}

	_, err := New(spec)

	if err == nil {
		t.Fatal("New returned nil error, want unknown-linux-distro error")
	}
	if !strings.Contains(err.Error(), "unknown linux distro") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "unknown linux distro")
	}
}

func TestBuildErrorsOnDarwinDistroNotMacos(t *testing.T) {

	spec := &Spec{os: "darwin", distro: "ubuntu", arch: "amd64"}

	_, err := New(spec)

	if err == nil {
		t.Fatal("New returned nil error, want darwin-distro error")
	}
	if !strings.Contains(err.Error(), "darwin distro must be") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "darwin distro must be")
	}
}

func TestBuildErrorsOnWindowsDistroNotWindows(t *testing.T) {

	spec := &Spec{os: "windows", distro: "ubuntu", arch: "amd64"}

	_, err := New(spec)

	if err == nil {
		t.Fatal("New returned nil error, want windows-distro error")
	}
	if !strings.Contains(err.Error(), "windows distro must be") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "windows distro must be")
	}
}

// endregion

// region New success paths

func TestBuildEmptyArchDefaultsToRuntimeGOARCH(t *testing.T) {

	spec := Ubuntu().WithArch("")

	p, err := New(spec)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if p.Arch() != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q (runtime.GOARCH)", p.Arch(), runtime.GOARCH)
	}
}

func TestBuildPopulatesPlatformFields(t *testing.T) {

	spec := Ubuntu().
		WithArch("amd64").
		WithVersion("22.04").
		WithHostname("dev-box").
		WithDefaultConcurrency(8)

	p, err := New(spec)
	if err != nil {
		t.Fatalf("New: %v", err)
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

	spec := &Spec{}

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
}

// endregion
