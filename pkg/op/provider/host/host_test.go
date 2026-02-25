// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package host

import (
	"runtime"
	"testing"
)

func TestNewHost(t *testing.T) {
	h := NewHost()
	if h == nil {
		t.Fatal("expected NewHost to return non-nil Host")
	}
}

func TestHostPlatform(t *testing.T) {
	h := NewHost()
	p := h.Platform()

	// OS should match runtime.GOOS
	if p.OS != runtime.GOOS {
		t.Errorf("expected OS %q, got %q", runtime.GOOS, p.OS)
	}

	// Arch should match runtime.GOARCH
	if p.Arch != runtime.GOARCH {
		t.Errorf("expected Arch %q, got %q", runtime.GOARCH, p.Arch)
	}

	// Distro should be non-empty
	if p.Distro == "" {
		t.Error("expected Distro to be non-empty")
	}
}

func TestDetectPlatform(t *testing.T) {
	p := DetectPlatform()

	if p.OS == "" {
		t.Error("expected OS to be non-empty")
	}
	if p.Arch == "" {
		t.Error("expected Arch to be non-empty")
	}
}

func TestHostHomeDir(t *testing.T) {
	h := NewHost()
	home := h.HomeDir()

	if home == "" {
		t.Error("expected HomeDir to return non-empty path")
	}
}

func TestHostExpandPath(t *testing.T) {
	h := NewHost()
	home := h.HomeDir()

	expanded := h.ExpandPath("~/test")
	expected := home + "/test"
	if expanded != expected {
		t.Errorf("expected ExpandPath(\"~/test\") = %q, got %q", expected, expanded)
	}

	// Non-tilde paths should be unchanged
	literal := "/absolute/path"
	if h.ExpandPath(literal) != literal {
		t.Errorf("expected ExpandPath(%q) = %q, got %q", literal, literal, h.ExpandPath(literal))
	}
}

func TestHostPackageManager(t *testing.T) {
	h := NewHost()
	pm := h.PackageManager()

	// On most systems there should be a package manager
	// On Darwin: brew or port
	// On Linux: apt, dnf, pacman, etc.
	// On Windows: winget
	// Skip if no PM found (some minimal systems may not have one)
	if pm == nil {
		t.Skip("no package manager detected on this system")
	}

	// Name should be non-empty
	if pm.Name() == "" {
		t.Error("expected PackageManager.Name() to return non-empty string")
	}
}

func TestHostServiceManager(t *testing.T) {
	h := NewHost()
	sm := h.ServiceManager()

	// Service manager may be nil on some systems
	if sm == nil {
		t.Skip("no service manager detected on this system")
	}
}

func TestPlatformStruct(t *testing.T) {
	p := Platform{
		OS:       "darwin",
		Arch:     "arm64",
		Distro:   "macos",
		Version:  "14.0",
		Hostname: "test-host",
	}

	if p.OS != "darwin" {
		t.Errorf("expected OS 'darwin', got %q", p.OS)
	}
	if p.Arch != "arm64" {
		t.Errorf("expected Arch 'arm64', got %q", p.Arch)
	}
	if p.Distro != "macos" {
		t.Errorf("expected Distro 'macos', got %q", p.Distro)
	}
}

func TestResultStruct(t *testing.T) {
	r := Result{
		OK:     true,
		Stdout: "output",
		Stderr: "",
		Code:   0,
	}

	if !r.OK {
		t.Error("expected OK to be true")
	}
	if r.Stdout != "output" {
		t.Errorf("expected Stdout 'output', got %q", r.Stdout)
	}
	if r.Code != 0 {
		t.Errorf("expected Code 0, got %d", r.Code)
	}

	// Failed result
	failed := Result{
		OK:     false,
		Stderr: "error",
		Code:   1,
	}
	if failed.OK {
		t.Error("expected OK to be false")
	}
}

func TestSearchResultStruct(t *testing.T) {
	sr := SearchResult{
		Name:        "curl",
		Version:     "8.0.0",
		Description: "Command line tool for transferring data",
	}

	if sr.Name != "curl" {
		t.Errorf("expected Name 'curl', got %q", sr.Name)
	}
	if sr.Version != "8.0.0" {
		t.Errorf("expected Version '8.0.0', got %q", sr.Version)
	}
}
