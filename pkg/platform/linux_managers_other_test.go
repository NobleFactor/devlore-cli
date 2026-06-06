// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !linux

package platform

import (
	"strings"
	"testing"
)

// Stub-method coverage for the Linux managers (apt, dnf, pacman, systemd) when the host is non-Linux.
// On a non-native host every primitive degrades: the query methods report not-installed / not-available
// / no-version / no-hits, and the mutating verbs surface a receipt error (and Update a refresh error,
// since these managers maintain a refreshable index) that names the missing tool and target=linux —
// preflight relies on this format to surface the cross-host mismatch before any provider invokes the
// manager.

// region apt stubs

func TestAptStubsReturnNotAvailable(t *testing.T) {

	m := newAptManager()
	pkg := PURL{Type: "deb", Name: "jq"}

	if m.Installed(pkg) {
		t.Error("Installed should be false on non-Linux host")
	}
	if m.Available(pkg) {
		t.Error("Available should be false on non-Linux host")
	}
	if got := m.Search("jq", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version(pkg); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	receipts, err := m.Install([]PURL{pkg}, nil)
	if err == nil {
		t.Error("Install returned nil error on non-Linux host")
	}
	if len(receipts) != 1 || receipts[0].Err == nil {
		t.Fatalf("Install receipts = %v, want one failed receipt", receipts)
	}
	if msg := receipts[0].Err.Error(); !strings.Contains(msg, "apt-get") || !strings.Contains(msg, "target=linux") {
		t.Errorf("Install receipt Err = %q, want substrings %q and %q", msg, "apt-get", "target=linux")
	}

	updateErr := m.Update()
	if updateErr == nil {
		t.Fatal("Update returned nil error on non-Linux host")
	}
	if msg := updateErr.Error(); !strings.Contains(msg, "apt-get") || !strings.Contains(msg, "target=linux") {
		t.Errorf("Update error = %q, want substrings %q and %q", msg, "apt-get", "target=linux")
	}
}

// endregion

// region dnf stubs

func TestDnfStubsReturnNotAvailable(t *testing.T) {

	m := newDnfManager()
	pkg := PURL{Type: "rpm", Name: "jq"}

	if m.Installed(pkg) {
		t.Error("Installed should be false on non-Linux host")
	}
	if m.Available(pkg) {
		t.Error("Available should be false on non-Linux host")
	}
	if got := m.Search("jq", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version(pkg); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	receipts, err := m.Install([]PURL{pkg}, nil)
	if err == nil {
		t.Error("Install returned nil error on non-Linux host")
	}
	if len(receipts) != 1 || receipts[0].Err == nil {
		t.Fatalf("Install receipts = %v, want one failed receipt", receipts)
	}
	if msg := receipts[0].Err.Error(); !strings.Contains(msg, "dnf") || !strings.Contains(msg, "target=linux") {
		t.Errorf("Install receipt Err = %q, want substrings %q and %q", msg, "dnf", "target=linux")
	}

	updateErr := m.Update()
	if updateErr == nil {
		t.Fatal("Update returned nil error on non-Linux host")
	}
	if msg := updateErr.Error(); !strings.Contains(msg, "dnf") || !strings.Contains(msg, "target=linux") {
		t.Errorf("Update error = %q, want substrings %q and %q", msg, "dnf", "target=linux")
	}
}

// endregion

// region pacman stubs

func TestPacmanStubsReturnNotAvailable(t *testing.T) {

	m := newPacmanManager()
	pkg := PURL{Type: "alpm", Name: "jq"}

	if m.Installed(pkg) {
		t.Error("Installed should be false on non-Linux host")
	}
	if m.Available(pkg) {
		t.Error("Available should be false on non-Linux host")
	}
	if got := m.Search("jq", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version(pkg); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	receipts, err := m.Install([]PURL{pkg}, nil)
	if err == nil {
		t.Error("Install returned nil error on non-Linux host")
	}
	if len(receipts) != 1 || receipts[0].Err == nil {
		t.Fatalf("Install receipts = %v, want one failed receipt", receipts)
	}
	if msg := receipts[0].Err.Error(); !strings.Contains(msg, "pacman") || !strings.Contains(msg, "target=linux") {
		t.Errorf("Install receipt Err = %q, want substrings %q and %q", msg, "pacman", "target=linux")
	}

	updateErr := m.Update()
	if updateErr == nil {
		t.Fatal("Update returned nil error on non-Linux host")
	}
	if msg := updateErr.Error(); !strings.Contains(msg, "pacman") || !strings.Contains(msg, "target=linux") {
		t.Errorf("Update error = %q, want substrings %q and %q", msg, "pacman", "target=linux")
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
