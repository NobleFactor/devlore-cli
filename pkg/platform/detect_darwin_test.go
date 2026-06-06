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

	spec, err := detectHost()
	if err != nil {
		t.Fatalf("detectHost: %v", err)
	}

	if spec.os != "darwin" {
		t.Errorf("os = %q, want darwin", spec.os)
	}
	if spec.distro != "macos" {
		t.Errorf("distro = %q, want macos", spec.distro)
	}
	if spec.version == "" {
		t.Error("version is empty; sw_vers -productVersion did not populate it")
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
	if p.DefaultPurlType() != "brew" {
		t.Errorf("DefaultPurlType() = %q, want brew", p.DefaultPurlType())
	}
	if p.ServiceManager() == nil {
		t.Error("ServiceManager is nil")
	}
}

// endregion
