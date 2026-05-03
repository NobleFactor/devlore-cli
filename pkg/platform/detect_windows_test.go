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

	got, err := detectHost()
	if err != nil {
		t.Fatalf("detectHost: %v", err)
	}

	if got.OS() != "windows" {
		t.Errorf("OS() = %q, want windows", got.OS())
	}
	if got.Distro() != "windows" {
		t.Errorf("Distro() = %q, want windows", got.Distro())
	}
	if got.Arch() != runtime.GOARCH {
		t.Errorf("Arch() = %q, want %q (runtime.GOARCH)", got.Arch(), runtime.GOARCH)
	}
	if got.DefaultPackageManager() == nil || got.DefaultPackageManager().Name() != "winget" {
		t.Errorf("DefaultPackageManager = %v, want winget", got.DefaultPackageManager())
	}
	if got.ServiceManager() == nil {
		t.Error("ServiceManager is nil")
	}
	if got.Hostname() == "" {
		t.Error("Hostname is empty; os.Hostname did not populate it")
	}
}

// endregion
