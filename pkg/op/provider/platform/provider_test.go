// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"runtime"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestNew(t *testing.T) {
	p := New()
	if p == nil {
		t.Fatal("expected New() to return non-nil Platform")
	}
}

func TestPlatformInfo(t *testing.T) {
	p := New()

	if p.OS != runtime.GOOS {
		t.Errorf("expected OS %q, got %q", runtime.GOOS, p.OS)
	}
	if p.Arch != runtime.GOARCH {
		t.Errorf("expected Arch %q, got %q", runtime.GOARCH, p.Arch)
	}
	if p.Distro == "" {
		t.Error("expected Distro to be non-empty")
	}
}

func TestPackageManager(t *testing.T) {
	p := New()
	pm := p.PackageManager
	if pm == nil {
		t.Skip("no package manager detected on this system")
	}
	if pm.Name() == "" {
		t.Error("expected PackageManager.Name() to return non-empty string")
	}
}

func TestServiceManager(t *testing.T) {
	p := New()
	if p.ServiceManager == nil {
		t.Skip("no service manager detected on this system")
	}
}

func TestPlatformStruct(t *testing.T) {
	p := &op.Platform{
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

func TestPlatformResultStruct(t *testing.T) {
	r := op.PlatformResult{
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

	failed := op.PlatformResult{
		OK:     false,
		Stderr: "error",
		Code:   1,
	}
	if failed.OK {
		t.Error("expected OK to be false")
	}
}

func TestSearchResultStruct(t *testing.T) {
	sr := op.SearchResult{
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

func TestGetPackageManager(t *testing.T) {
	p := New()
	if p.PackageManager == nil {
		t.Skip("no package manager detected")
	}

	name := p.PackageManager.Name()
	pm := p.GetPackageManager(name)
	if pm == nil {
		t.Errorf("GetPackageManager(%q) returned nil, expected non-nil", name)
	}

	pm = p.GetPackageManager("nonexistent")
	if pm != nil {
		t.Errorf("GetPackageManager(\"nonexistent\") returned non-nil")
	}
}
