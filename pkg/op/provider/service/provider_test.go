// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"io"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// mockServiceManager implements op.ServiceManager for testing.
type mockServiceManager struct {
	exists      map[string]bool
	running     map[string]bool
	enabled     map[string]bool
	startFail   bool
	stopFail    bool
	enableFail  bool
	disableFail bool
}

func newMockServiceManager() *mockServiceManager {
	return &mockServiceManager{
		exists:  make(map[string]bool),
		running: make(map[string]bool),
		enabled: make(map[string]bool),
	}
}

func (m *mockServiceManager) Exists(name string) bool    { return m.exists[name] }
func (m *mockServiceManager) IsRunning(name string) bool { return m.running[name] }
func (m *mockServiceManager) IsEnabled(name string) bool { return m.enabled[name] }

func (m *mockServiceManager) Status(name string) string {
	if m.running[name] {
		return "running"
	}
	return "stopped"
}

func (m *mockServiceManager) Start(name string) op.PlatformResult {
	if m.startFail {
		return op.PlatformResult{OK: false, Stderr: "permission denied", Code: 1}
	}
	m.running[name] = true
	return op.PlatformResult{OK: true}
}

func (m *mockServiceManager) Stop(name string) op.PlatformResult {
	if m.stopFail {
		return op.PlatformResult{OK: false, Stderr: "stop failed", Code: 1}
	}
	m.running[name] = false
	return op.PlatformResult{OK: true}
}

func (m *mockServiceManager) Enable(name string) op.PlatformResult {
	if m.enableFail {
		return op.PlatformResult{OK: false, Stderr: "enable failed", Code: 1}
	}
	m.enabled[name] = true
	return op.PlatformResult{OK: true}
}

func (m *mockServiceManager) Disable(name string) op.PlatformResult {
	if m.disableFail {
		return op.PlatformResult{OK: false, Stderr: "disable failed", Code: 1}
	}
	m.enabled[name] = false
	return op.PlatformResult{OK: true}
}

func (m *mockServiceManager) NeedsSudo() bool { return false }

// newTestProvider creates a Provider wired to the given mock.
func newTestProvider(serviceManager *mockServiceManager) *Provider {
	return &Provider{
		Platform: &op.Platform{
			ServiceManager: serviceManager,
		},
	}
}

func TestStart(t *testing.T) {
	serviceManager := newMockServiceManager()
	serviceManager.exists["nginx"] = true
	serviceManager.running["nginx"] = false

	p := newTestProvider(serviceManager)
	name, state, err := p.Start("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if name != "nginx" {
		t.Errorf("Start() name = %q, want %q", name, "nginx")
	}
	wasRunning, _ := state["was_running"].(bool)
	if wasRunning {
		t.Error("Start() was_running = true, want false")
	}
	if !serviceManager.running["nginx"] {
		t.Error("service should be running after Start()")
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	serviceManager := newMockServiceManager()
	serviceManager.exists["nginx"] = true
	serviceManager.running["nginx"] = true

	p := newTestProvider(serviceManager)
	name, state, err := p.Start("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if name != "nginx" {
		t.Errorf("Start() name = %q, want %q", name, "nginx")
	}
	wasRunning, _ := state["was_running"].(bool)
	if !wasRunning {
		t.Error("Start() was_running = false, want true")
	}
}

func TestStartError(t *testing.T) {
	serviceManager := newMockServiceManager()
	serviceManager.startFail = true

	p := newTestProvider(serviceManager)
	_, _, err := p.Start("nginx", io.Discard)
	if err == nil {
		t.Fatal("Start() expected error, got nil")
	}
}

func TestCompensateStart(t *testing.T) {
	t.Run("was_running false calls Stop", func(t *testing.T) {
		serviceManager := newMockServiceManager()
		serviceManager.running["nginx"] = true

		p := newTestProvider(serviceManager)
		state := map[string]any{"name": "nginx", "was_running": false}
		if err := p.CompensateStart(state); err != nil {
			t.Fatalf("CompensateStart() error = %v", err)
		}
		if serviceManager.running["nginx"] {
			t.Error("service should be stopped after compensating a fresh start")
		}
	})

	t.Run("was_running true is no-op", func(t *testing.T) {
		serviceManager := newMockServiceManager()
		serviceManager.running["nginx"] = true

		p := newTestProvider(serviceManager)
		state := map[string]any{"name": "nginx", "was_running": true}
		if err := p.CompensateStart(state); err != nil {
			t.Fatalf("CompensateStart() error = %v", err)
		}
		if !serviceManager.running["nginx"] {
			t.Error("service should still be running when was_running=true")
		}
	})

	t.Run("nil state is no-op", func(t *testing.T) {
		serviceManager := newMockServiceManager()
		p := newTestProvider(serviceManager)
		if err := p.CompensateStart(nil); err != nil {
			t.Fatalf("CompensateStart(nil) error = %v", err)
		}
	})
}

func TestStop(t *testing.T) {
	serviceManager := newMockServiceManager()
	serviceManager.running["nginx"] = true

	p := newTestProvider(serviceManager)
	name, state, err := p.Stop("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if name != "nginx" {
		t.Errorf("Stop() name = %q, want %q", name, "nginx")
	}
	wasRunning, _ := state["was_running"].(bool)
	if !wasRunning {
		t.Error("Stop() was_running = false, want true")
	}
	if serviceManager.running["nginx"] {
		t.Error("service should be stopped after Stop()")
	}
}

func TestCompensateStop(t *testing.T) {
	t.Run("was_running true calls Start", func(t *testing.T) {
		serviceManager := newMockServiceManager()
		serviceManager.running["nginx"] = false

		p := newTestProvider(serviceManager)
		state := map[string]any{"name": "nginx", "was_running": true}
		if err := p.CompensateStop(state); err != nil {
			t.Fatalf("CompensateStop() error = %v", err)
		}
		if !serviceManager.running["nginx"] {
			t.Error("service should be running after compensating a stop")
		}
	})

	t.Run("was_running false is no-op", func(t *testing.T) {
		serviceManager := newMockServiceManager()
		serviceManager.running["nginx"] = false

		p := newTestProvider(serviceManager)
		state := map[string]any{"name": "nginx", "was_running": false}
		if err := p.CompensateStop(state); err != nil {
			t.Fatalf("CompensateStop() error = %v", err)
		}
		if serviceManager.running["nginx"] {
			t.Error("service should remain stopped when was_running=false")
		}
	})
}

func TestRestart(t *testing.T) {
	serviceManager := newMockServiceManager()
	serviceManager.running["nginx"] = true

	p := newTestProvider(serviceManager)
	name, state, err := p.Restart("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if name != "nginx" {
		t.Errorf("Restart() name = %q, want %q", name, "nginx")
	}
	if state == nil {
		t.Fatal("Restart() state is nil")
	}
	stateName, _ := state["name"].(string)
	if stateName != "nginx" {
		t.Errorf("Restart() state name = %q, want %q", stateName, "nginx")
	}
	if !serviceManager.running["nginx"] {
		t.Error("service should be running after Restart()")
	}
}

func TestRestartError(t *testing.T) {
	t.Run("Stop fails", func(t *testing.T) {
		serviceManager := newMockServiceManager()
		serviceManager.stopFail = true

		p := newTestProvider(serviceManager)
		_, _, err := p.Restart("nginx", io.Discard)
		if err == nil {
			t.Fatal("Restart() expected error from Stop, got nil")
		}
	})

	t.Run("Stop OK but Start fails", func(t *testing.T) {
		serviceManager := newMockServiceManager()
		serviceManager.startFail = true

		p := newTestProvider(serviceManager)
		_, _, err := p.Restart("nginx", io.Discard)
		if err == nil {
			t.Fatal("Restart() expected error from Start, got nil")
		}
	})
}

func TestEnable(t *testing.T) {
	serviceManager := newMockServiceManager()
	serviceManager.enabled["nginx"] = false

	p := newTestProvider(serviceManager)
	name, state, err := p.Enable("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if name != "nginx" {
		t.Errorf("Enable() name = %q, want %q", name, "nginx")
	}
	wasEnabled, _ := state["was_enabled"].(bool)
	if wasEnabled {
		t.Error("Enable() was_enabled = true, want false")
	}
	if !serviceManager.enabled["nginx"] {
		t.Error("service should be enabled after Enable()")
	}
}

func TestCompensateEnable(t *testing.T) {
	t.Run("was_enabled false calls Disable", func(t *testing.T) {
		serviceManager := newMockServiceManager()
		serviceManager.enabled["nginx"] = true

		p := newTestProvider(serviceManager)
		state := map[string]any{"name": "nginx", "was_enabled": false}
		if err := p.CompensateEnable(state); err != nil {
			t.Fatalf("CompensateEnable() error = %v", err)
		}
		if serviceManager.enabled["nginx"] {
			t.Error("service should be disabled after compensating a fresh enable")
		}
	})

	t.Run("was_enabled true is no-op", func(t *testing.T) {
		serviceManager := newMockServiceManager()
		serviceManager.enabled["nginx"] = true

		p := newTestProvider(serviceManager)
		state := map[string]any{"name": "nginx", "was_enabled": true}
		if err := p.CompensateEnable(state); err != nil {
			t.Fatalf("CompensateEnable() error = %v", err)
		}
		if !serviceManager.enabled["nginx"] {
			t.Error("service should still be enabled when was_enabled=true")
		}
	})
}

func TestDisable(t *testing.T) {
	serviceManager := newMockServiceManager()
	serviceManager.enabled["nginx"] = true

	p := newTestProvider(serviceManager)
	name, state, err := p.Disable("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if name != "nginx" {
		t.Errorf("Disable() name = %q, want %q", name, "nginx")
	}
	wasEnabled, _ := state["was_enabled"].(bool)
	if !wasEnabled {
		t.Error("Disable() was_enabled = false, want true")
	}
	if serviceManager.enabled["nginx"] {
		t.Error("service should be disabled after Disable()")
	}
}

func TestCompensateDisable(t *testing.T) {
	t.Run("was_enabled true calls Enable", func(t *testing.T) {
		serviceManager := newMockServiceManager()
		serviceManager.enabled["nginx"] = false

		p := newTestProvider(serviceManager)
		state := map[string]any{"name": "nginx", "was_enabled": true}
		if err := p.CompensateDisable(state); err != nil {
			t.Fatalf("CompensateDisable() error = %v", err)
		}
		if !serviceManager.enabled["nginx"] {
			t.Error("service should be enabled after compensating a disable")
		}
	})

	t.Run("was_enabled false is no-op", func(t *testing.T) {
		serviceManager := newMockServiceManager()
		serviceManager.enabled["nginx"] = false

		p := newTestProvider(serviceManager)
		state := map[string]any{"name": "nginx", "was_enabled": false}
		if err := p.CompensateDisable(state); err != nil {
			t.Fatalf("CompensateDisable() error = %v", err)
		}
		if serviceManager.enabled["nginx"] {
			t.Error("service should remain disabled when was_enabled=false")
		}
	})
}

func TestPredicates(t *testing.T) {
	serviceManager := newMockServiceManager()
	serviceManager.exists["nginx"] = true
	serviceManager.exists["missing"] = false
	serviceManager.running["nginx"] = true
	serviceManager.running["stopped"] = false
	serviceManager.enabled["nginx"] = true
	serviceManager.enabled["disabled"] = false

	p := newTestProvider(serviceManager)

	t.Run("Exists true", func(t *testing.T) {
		got, err := p.Exists("nginx")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if !got {
			t.Error("Exists(nginx) = false, want true")
		}
	})

	t.Run("Exists false", func(t *testing.T) {
		got, err := p.Exists("missing")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if got {
			t.Error("Exists(missing) = true, want false")
		}
	})

	t.Run("Running true", func(t *testing.T) {
		got, err := p.Running("nginx")
		if err != nil {
			t.Fatalf("Running() error = %v", err)
		}
		if !got {
			t.Error("Running(nginx) = false, want true")
		}
	})

	t.Run("Running false", func(t *testing.T) {
		got, err := p.Running("stopped")
		if err != nil {
			t.Fatalf("Running() error = %v", err)
		}
		if got {
			t.Error("Running(stopped) = true, want false")
		}
	})

	t.Run("Enabled true", func(t *testing.T) {
		got, err := p.Enabled("nginx")
		if err != nil {
			t.Fatalf("Enabled() error = %v", err)
		}
		if !got {
			t.Error("Enabled(nginx) = false, want true")
		}
	})

	t.Run("Enabled false", func(t *testing.T) {
		got, err := p.Enabled("disabled")
		if err != nil {
			t.Fatalf("Enabled() error = %v", err)
		}
		if got {
			t.Error("Enabled(disabled) = true, want false")
		}
	})
}
