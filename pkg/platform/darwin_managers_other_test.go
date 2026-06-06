// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !darwin

package platform

import (
	"strings"
	"testing"
)

// Stub-method coverage for the Darwin managers (brew, port, launchd) when the host is non-Darwin. The
// query methods report empty/false, and the mutating verbs (and Update, since brew and port maintain a
// refreshable index) surface an error naming the missing tool and target=darwin.

// region brew stubs

func TestBrewStubsReturnNotAvailable(t *testing.T) {

	m := newBrewManager()
	pkg := PURL{Type: "brew", Name: "jq"}

	if m.Installed(pkg) {
		t.Error("Installed should be false on non-Darwin host")
	}
	if m.Available(pkg) {
		t.Error("Available should be false on non-Darwin host")
	}
	if got := m.Search("jq", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version(pkg); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	receipts, err := m.Install([]PURL{pkg}, nil)
	if err == nil {
		t.Error("Install returned nil error on non-Darwin host")
	}
	if len(receipts) != 1 || receipts[0].Err == nil {
		t.Fatalf("Install receipts = %v, want one failed receipt", receipts)
	}
	if msg := receipts[0].Err.Error(); !strings.Contains(msg, "brew") || !strings.Contains(msg, "target=darwin") {
		t.Errorf("Install receipt Err = %q, want substrings %q and %q", msg, "brew", "target=darwin")
	}

	updateErr := m.Update()
	if updateErr == nil {
		t.Fatal("Update returned nil error on non-Darwin host")
	}
	if msg := updateErr.Error(); !strings.Contains(msg, "brew") || !strings.Contains(msg, "target=darwin") {
		t.Errorf("Update error = %q, want substrings %q and %q", msg, "brew", "target=darwin")
	}
}

// endregion

// region port stubs

func TestPortStubsReturnNotAvailable(t *testing.T) {

	m := newPortManager()
	pkg := PURL{Type: "port", Name: "jq"}

	if m.Installed(pkg) {
		t.Error("Installed should be false on non-Darwin host")
	}
	if m.Available(pkg) {
		t.Error("Available should be false on non-Darwin host")
	}
	if got := m.Search("jq", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version(pkg); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	receipts, err := m.Install([]PURL{pkg}, nil)
	if err == nil {
		t.Error("Install returned nil error on non-Darwin host")
	}
	if len(receipts) != 1 || receipts[0].Err == nil {
		t.Fatalf("Install receipts = %v, want one failed receipt", receipts)
	}
	if msg := receipts[0].Err.Error(); !strings.Contains(msg, "port") || !strings.Contains(msg, "target=darwin") {
		t.Errorf("Install receipt Err = %q, want substrings %q and %q", msg, "port", "target=darwin")
	}

	updateErr := m.Update()
	if updateErr == nil {
		t.Fatal("Update returned nil error on non-Darwin host")
	}
	if msg := updateErr.Error(); !strings.Contains(msg, "port") || !strings.Contains(msg, "target=darwin") {
		t.Errorf("Update error = %q, want substrings %q and %q", msg, "port", "target=darwin")
	}
}

// endregion

// region launchd stubs

func TestLaunchdStubsReturnNotAvailable(t *testing.T) {

	m := &launchdManager{}

	if m.Exists("com.example.daemon") {
		t.Error("Exists should be false on non-Darwin host")
	}
	if m.IsRunning("com.example.daemon") {
		t.Error("IsRunning should be false on non-Darwin host")
	}
	if m.IsEnabled("com.example.daemon") {
		t.Error("IsEnabled should be false on non-Darwin host")
	}
	if got := m.Status("com.example.daemon"); got != "" {
		t.Errorf("Status = %q, want empty", got)
	}

	for name, result := range map[string]PlatformResult{
		"Start":   m.Start("com.example.daemon"),
		"Stop":    m.Stop("com.example.daemon"),
		"Enable":  m.Enable("com.example.daemon"),
		"Disable": m.Disable("com.example.daemon"),
	} {
		if result.OK {
			t.Errorf("%s.OK = true on non-Darwin host", name)
		}
		if !strings.Contains(result.Stderr, "launchctl") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "launchctl")
		}
		if !strings.Contains(result.Stderr, "target=darwin") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "target=darwin")
		}
	}
}

// endregion
