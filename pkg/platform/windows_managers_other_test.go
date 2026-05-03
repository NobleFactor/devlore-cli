// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !windows

package platform

import (
	"strings"
	"testing"
)

// Stub-method coverage for the Windows managers (winget, sc.exe) when the host is non-Windows.

// region winget stubs

func TestWingetStubsReturnNotAvailable(t *testing.T) {

	m := &wingetManager{}

	if m.Installed("Microsoft.VisualStudioCode") {
		t.Error("Installed should be false on non-Windows host")
	}
	if m.Available("Microsoft.VisualStudioCode") {
		t.Error("Available should be false on non-Windows host")
	}
	if got := m.Search("vscode", 5); got != nil {
		t.Errorf("Search = %v, want nil", got)
	}
	if got := m.Version("Microsoft.VisualStudioCode"); got != "" {
		t.Errorf("Version = %q, want empty", got)
	}

	for name, result := range map[string]PlatformResult{
		"Install": m.Install("Microsoft.VisualStudioCode"),
		"Remove":  m.Remove("Microsoft.VisualStudioCode"),
		"Update":  m.Update(),
		"AddRepo": m.AddRepo("ignored", "ignored", "ignored"),
	} {
		if result.OK {
			t.Errorf("%s.OK = true on non-Windows host", name)
		}
		if !strings.Contains(result.Stderr, "winget") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "winget")
		}
		if !strings.Contains(result.Stderr, "target=windows") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "target=windows")
		}
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
