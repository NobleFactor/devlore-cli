// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build !darwin

package platform

import (
	"strings"
	"testing"
)

// Stub-method coverage for the Darwin managers (brew, port, launchd) when the host is non-Darwin.

// region brew stubs

func TestBrewStubsReturnNotAvailable(t *testing.T) {

	m := &brewManager{}

	if m.Installed("jq") {
		t.Error("Installed should be false on non-Darwin host")
	}
	if m.Available("jq") {
		t.Error("Available should be false on non-Darwin host")
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
		"AddRepo": m.AddRepo("https://example/tap", "", "example"),
	} {
		if result.OK {
			t.Errorf("%s.OK = true on non-Darwin host", name)
		}
		if !strings.Contains(result.Stderr, "brew") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "brew")
		}
		if !strings.Contains(result.Stderr, "target=darwin") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "target=darwin")
		}
	}
}

// endregion

// region port stubs

func TestPortStubsReturnNotAvailable(t *testing.T) {

	m := &portManager{}

	if m.Installed("jq") {
		t.Error("Installed should be false on non-Darwin host")
	}
	if m.Available("jq") {
		t.Error("Available should be false on non-Darwin host")
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
		"AddRepo": m.AddRepo("https://example/tap", "", "example"),
	} {
		if result.OK {
			t.Errorf("%s.OK = true on non-Darwin host", name)
		}
		if !strings.Contains(result.Stderr, "port") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "port")
		}
		if !strings.Contains(result.Stderr, "target=darwin") {
			t.Errorf("%s.Stderr = %q, want substring %q", name, result.Stderr, "target=darwin")
		}
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
