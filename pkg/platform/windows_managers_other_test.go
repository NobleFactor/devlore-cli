// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !windows

package platform

import (
	"strings"
	"testing"
)

// Stub-method coverage for the Windows managers (winget, sc.exe) when the host is non-Windows. The query
// methods report empty/false and Install surfaces a failing receipt naming the missing tool and
// target=windows. winget queries a live store rather than a local index, so it is not a refresher:
// Update is a no-op (nil error) even on a non-native host.

// region winget stubs

func TestWingetStubsReturnNotAvailable(t *testing.T) {

	m := newWingetManager()
	pkg := PURL{Type: "winget", Namespace: "Microsoft", Name: "VisualStudioCode"}

	if m.Installed(pkg) {
		t.Error("Installed should be false on non-Windows host")
	}
	if m.Available(pkg) {
		t.Error("Available should be false on non-Windows host")
	}
	if got := m.Search("vscode", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version(pkg); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	receipts, err := m.Install([]PURL{pkg}, nil)
	if err == nil {
		t.Error("Install returned nil error on non-Windows host")
	}
	if len(receipts) != 1 || receipts[0].Err == nil {
		t.Fatalf("Install receipts = %v, want one failed receipt", receipts)
	}
	if msg := receipts[0].Err.Error(); !strings.Contains(msg, "winget") || !strings.Contains(msg, "target=windows") {
		t.Errorf("Install receipt Err = %q, want substrings %q and %q", msg, "winget", "target=windows")
	}

	// winget queries a live store, so Update is a no-op on every host.
	if updateErr := m.Update(); updateErr != nil {
		t.Errorf("Update returned %v, want nil (winget is not a refresher)", updateErr)
	}
}

// endregion

// region windows service manager stubs

func TestWindowsServiceManagerStubsReturnNotAvailable(t *testing.T) {

	m := &windowsServiceManager{}

	if m.Exists("Spooler") {
		t.Error("Exists should be false on non-Windows host")
	}
	if m.IsRunning("Spooler") {
		t.Error("IsRunning should be false on non-Windows host")
	}
	if m.IsEnabled("Spooler") {
		t.Error("IsEnabled should be false on non-Windows host")
	}
	if got := m.Status("Spooler"); got != "" {
		t.Errorf("Status = %q, want empty", got)
	}

	for name, result := range map[string]PlatformResult{
		"Start":   m.Start("Spooler"),
		"Stop":    m.Stop("Spooler"),
		"Enable":  m.Enable("Spooler"),
		"Disable": m.Disable("Spooler"),
	} {
		if result.OK {
			t.Errorf("%s.OK = true on non-Windows host", name)
		}
		if !strings.Contains(result.Stderr, "sc") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "sc")
		}
		if !strings.Contains(result.Stderr, "target=windows") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "target=windows")
		}
	}
}

// endregion
