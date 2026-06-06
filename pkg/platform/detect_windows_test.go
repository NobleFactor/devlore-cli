// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build windows

package platform

import (
	"runtime"
	"testing"
)

// region detectHost (windows)

// TestDetectHostWindowsReturnsWindowsPlatform exercises the host-resident detection logic on a windows
// host. `cmd /c ver` and `os.Hostname` are expected to succeed on any Windows environment.
func TestDetectHostWindowsReturnsWindowsPlatform(t *testing.T) {

	spec, err := detectHost()
	if err != nil {
		t.Fatalf("detectHost: %v", err)
	}

	if spec.os != "windows" {
		t.Errorf("os = %q, want windows", spec.os)
	}
	if spec.distro != "windows" {
		t.Errorf("distro = %q, want windows", spec.distro)
	}
	if spec.hostname == "" {
		t.Error("hostname is empty; os.Hostname did not populate it")
	}

	p, err := New(spec)
	if err != nil {
		t.Fatalf("New(detected spec): %v", err)
	}

	if p.Arch() != runtime.GOARCH {
		t.Errorf("Arch() = %q, want %q (runtime.GOARCH)", p.Arch(), runtime.GOARCH)
	}
	if p.DefaultPurlType() != "winget" {
		t.Errorf("DefaultPurlType() = %q, want winget", p.DefaultPurlType())
	}
	if p.ServiceManager() == nil {
		t.Error("ServiceManager is nil")
	}
}

// endregion
