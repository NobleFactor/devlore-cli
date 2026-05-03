// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !linux

package platform

import (
	"strings"
	"testing"
)

// Stub-method coverage for the Linux managers (apt, dnf, pacman, systemd) when the host is non-Linux.
// Every shell-out method must return a fixed-shape PlatformResult that identifies the missing tool and
// the target OS — preflight relies on this format to surface the cross-host mismatch before any
// provider actually invokes the manager.

// region apt stubs

func TestAptStubsReturnNotAvailable(t *testing.T) {

	m := &aptManager{}

	if m.Installed("jq") {
		t.Error("Installed should be false on non-Linux host")
	}
	if m.Available("jq") {
		t.Error("Available should be false on non-Linux host")
	}
	if got := m.Search("jq", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version("jq"); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	for name, result := range map[string]PlatformResult{
		"Install": m.Install("jq"),
		"Remove":  m.Remove("jq"),
		"Update":  m.Update(),
		"AddRepo": m.AddRepo("https://example/repo", "https://example/key", "example"),
	} {
		if result.OK {
			t.Errorf("%s.OK = true on non-Linux host", name)
		}
		if !strings.Contains(result.Stderr, "apt-get") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "apt-get")
		}
		if !strings.Contains(result.Stderr, "target=linux") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "target=linux")
		}
	}
}

// endregion

// region dnf stubs

func TestDnfStubsReturnNotAvailable(t *testing.T) {

	m := &dnfManager{}

	if m.Installed("jq") {
		t.Error("Installed should be false on non-Linux host")
	}
	if m.Available("jq") {
		t.Error("Available should be false on non-Linux host")
	}
	if got := m.Search("jq", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version("jq"); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	for name, result := range map[string]PlatformResult{
		"Install": m.Install("jq"),
		"Remove":  m.Remove("jq"),
		"Update":  m.Update(),
		"AddRepo": m.AddRepo("https://example/repo", "https://example/key", "example"),
	} {
		if result.OK {
			t.Errorf("%s.OK = true on non-Linux host", name)
		}
		if !strings.Contains(result.Stderr, "dnf") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "dnf")
		}
		if !strings.Contains(result.Stderr, "target=linux") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "target=linux")
		}
	}
}

// endregion

// region pacman stubs

func TestPacmanStubsReturnNotAvailable(t *testing.T) {

	m := &pacmanManager{}

	if m.Installed("jq") {
		t.Error("Installed should be false on non-Linux host")
	}
	if m.Available("jq") {
		t.Error("Available should be false on non-Linux host")
	}
	if got := m.Search("jq", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version("jq"); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	for name, result := range map[string]PlatformResult{
		"Install": m.Install("jq"),
		"Remove":  m.Remove("jq"),
		"Update":  m.Update(),
		"AddRepo": m.AddRepo("https://example/repo", "https://example/key", "example"),
	} {
		if result.OK {
			t.Errorf("%s.OK = true on non-Linux host", name)
		}
		if !strings.Contains(result.Stderr, "pacman") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "pacman")
		}
		if !strings.Contains(result.Stderr, "target=linux") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "target=linux")
		}
	}
}

// endregion

// region systemd stubs

func TestSystemdStubsReturnNotAvailable(t *testing.T) {

	m := &systemdManager{}

	if m.Exists("nginx") {
		t.Error("Exists should be false on non-Linux host")
	}
	if m.IsRunning("nginx") {
		t.Error("IsRunning should be false on non-Linux host")
	}
	if m.IsEnabled("nginx") {
		t.Error("IsEnabled should be false on non-Linux host")
	}
	if got := m.Status("nginx"); got != "" {
		t.Errorf("Status = %q, want empty", got)
	}

	for name, result := range map[string]PlatformResult{
		"Start":   m.Start("nginx"),
		"Stop":    m.Stop("nginx"),
		"Enable":  m.Enable("nginx"),
		"Disable": m.Disable("nginx"),
	} {
		if result.OK {
			t.Errorf("%s.OK = true on non-Linux host", name)
		}
		if !strings.Contains(result.Stderr, "systemctl") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "systemctl")
		}
		if !strings.Contains(result.Stderr, "target=linux") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "target=linux")
		}
	}
}

// endregion
