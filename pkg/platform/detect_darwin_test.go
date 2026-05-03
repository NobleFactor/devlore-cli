// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build darwin

package platform

import (
	"runtime"
	"testing"
)

// region detectHost (darwin)

// TestDetectHostDarwinReturnsMacosPlatform exercises the host-resident detection logic on a darwin
// host. `sw_vers -productVersion` and `os.Hostname` are both expected to succeed on any reasonable
// macOS environment, so we assert that the version and hostname fields are populated.
func TestDetectHostDarwinReturnsMacosPlatform(t *testing.T) {

	got, err := detectHost()
	if err != nil {
		t.Fatalf("detectHost: %v", err)
	}

	if got.OS() != "darwin" {
		t.Errorf("OS() = %q, want darwin", got.OS())
	}
	if got.Distro() != "macos" {
		t.Errorf("Distro() = %q, want macos", got.Distro())
	}
	if got.Arch() != runtime.GOARCH {
		t.Errorf("Arch() = %q, want %q (runtime.GOARCH)", got.Arch(), runtime.GOARCH)
	}
	if got.DefaultPackageManager() == nil || got.DefaultPackageManager().Name() != "brew" {
		t.Errorf("DefaultPackageManager = %v, want brew", got.DefaultPackageManager())
	}
	if got.ServiceManager() == nil {
		t.Error("ServiceManager is nil")
	}
	if got.Version() == "" {
		t.Error("Version is empty; sw_vers -productVersion did not populate it")
	}
	if got.Hostname() == "" {
		t.Error("Hostname is empty; os.Hostname did not populate it")
	}
}

// endregion
