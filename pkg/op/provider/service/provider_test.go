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
func newTestProvider(sm *mockServiceManager) *Provider {
	return &Provider{
		ProviderBase: op.NewProviderBase(op.Context{
			ContextBase: op.ContextBase{
				Writer: io.Discard,
				Platform: &op.Platform{
					ServiceManager: sm,
				},
			},
		}),
	}
}

func TestStart(t *testing.T) {
	sm := newMockServiceManager()
	sm.exists["nginx"] = true
	sm.running["nginx"] = false

	p := newTestProvider(sm)
	result, state, err := p.Start(Resource{Name: "nginx"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if result.Name != "nginx" {
		t.Errorf("Start() result.ReceiverName = %q, want %q", result.Name, "nginx")
	}
	if state.WasRunning {
		t.Error("Start() WasRunning = true, want false")
	}
	if !sm.running["nginx"] {
		t.Error("service should be running after Start()")
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	sm := newMockServiceManager()
	sm.exists["nginx"] = true
	sm.running["nginx"] = true

	p := newTestProvider(sm)
	result, state, err := p.Start(Resource{Name: "nginx"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if result.Name != "nginx" {
		t.Errorf("Start() result.ReceiverName = %q, want %q", result.Name, "nginx")
	}
	if !state.WasRunning {
		t.Error("Start() WasRunning = false, want true")
	}
}

func TestStartError(t *testing.T) {
	sm := newMockServiceManager()
	sm.startFail = true

	p := newTestProvider(sm)
	_, _, err := p.Start(Resource{Name: "nginx"})
	if err == nil {
		t.Fatal("Start() expected error, got nil")
	}
}

func TestCompensateStart(t *testing.T) {
	t.Run("WasRunning false calls Stop", func(t *testing.T) {
		sm := newMockServiceManager()
		sm.running["nginx"] = true

		p := newTestProvider(sm)
		if err := p.CompensateStart(Tombstone{Name: "nginx", WasRunning: false}); err != nil {
			t.Fatalf("CompensateStart() error = %v", err)
		}
		if sm.running["nginx"] {
			t.Error("service should be stopped after compensating a fresh start")
		}
	})

	t.Run("WasRunning true is no-op", func(t *testing.T) {
		sm := newMockServiceManager()
		sm.running["nginx"] = true

		p := newTestProvider(sm)
		if err := p.CompensateStart(Tombstone{Name: "nginx", WasRunning: true}); err != nil {
			t.Fatalf("CompensateStart() error = %v", err)
		}
		if !sm.running["nginx"] {
			t.Error("service should still be running when WasRunning=true")
		}
	})

	t.Run("empty name is no-op", func(t *testing.T) {
		sm := newMockServiceManager()
		p := newTestProvider(sm)
		if err := p.CompensateStart(Tombstone{}); err != nil {
			t.Fatalf("CompensateStart(empty) error = %v", err)
		}
	})
}

func TestStop(t *testing.T) {
	sm := newMockServiceManager()
	sm.running["nginx"] = true

	p := newTestProvider(sm)
	result, state, err := p.Stop(Resource{Name: "nginx"})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if result.Name != "nginx" {
		t.Errorf("Stop() result.ReceiverName = %q, want %q", result.Name, "nginx")
	}
	if !state.WasRunning {
		t.Error("Stop() WasRunning = false, want true")
	}
	if sm.running["nginx"] {
		t.Error("service should be stopped after Stop()")
	}
}

func TestCompensateStop(t *testing.T) {
	t.Run("WasRunning true calls Start", func(t *testing.T) {
		sm := newMockServiceManager()
		sm.running["nginx"] = false

		p := newTestProvider(sm)
		if err := p.CompensateStop(Tombstone{Name: "nginx", WasRunning: true}); err != nil {
			t.Fatalf("CompensateStop() error = %v", err)
		}
		if !sm.running["nginx"] {
			t.Error("service should be running after compensating a stop")
		}
	})

	t.Run("WasRunning false is no-op", func(t *testing.T) {
		sm := newMockServiceManager()
		sm.running["nginx"] = false

		p := newTestProvider(sm)
		if err := p.CompensateStop(Tombstone{Name: "nginx", WasRunning: false}); err != nil {
			t.Fatalf("CompensateStop() error = %v", err)
		}
		if sm.running["nginx"] {
			t.Error("service should remain stopped when WasRunning=false")
		}
	})
}

func TestRestart(t *testing.T) {
	sm := newMockServiceManager()
	sm.running["nginx"] = true

	p := newTestProvider(sm)
	result, state, err := p.Restart(Resource{Name: "nginx"})
	if err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if result.Name != "nginx" {
		t.Errorf("Restart() result.ReceiverName = %q, want %q", result.Name, "nginx")
	}
	if state.Name != "nginx" {
		t.Errorf("Restart() state.ReceiverName = %q, want %q", state.Name, "nginx")
	}
	if !sm.running["nginx"] {
		t.Error("service should be running after Restart()")
	}
}

func TestRestartError(t *testing.T) {
	t.Run("Stop fails", func(t *testing.T) {
		sm := newMockServiceManager()
		sm.stopFail = true

		p := newTestProvider(sm)
		_, _, err := p.Restart(Resource{Name: "nginx"})
		if err == nil {
			t.Fatal("Restart() expected error from Stop, got nil")
		}
	})

	t.Run("Stop OK but Start fails", func(t *testing.T) {
		sm := newMockServiceManager()
		sm.startFail = true

		p := newTestProvider(sm)
		_, _, err := p.Restart(Resource{Name: "nginx"})
		if err == nil {
			t.Fatal("Restart() expected error from Start, got nil")
		}
	})
}

func TestEnable(t *testing.T) {
	sm := newMockServiceManager()
	sm.enabled["nginx"] = false

	p := newTestProvider(sm)
	result, state, err := p.Enable(Resource{Name: "nginx"})
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if result.Name != "nginx" {
		t.Errorf("Enable() result.ReceiverName = %q, want %q", result.Name, "nginx")
	}
	if state.WasEnabled {
		t.Error("Enable() WasEnabled = true, want false")
	}
	if !sm.enabled["nginx"] {
		t.Error("service should be enabled after Enable()")
	}
}

func TestCompensateEnable(t *testing.T) {
	t.Run("WasEnabled false calls Disable", func(t *testing.T) {
		sm := newMockServiceManager()
		sm.enabled["nginx"] = true

		p := newTestProvider(sm)
		if err := p.CompensateEnable(Tombstone{Name: "nginx", WasEnabled: false}); err != nil {
			t.Fatalf("CompensateEnable() error = %v", err)
		}
		if sm.enabled["nginx"] {
			t.Error("service should be disabled after compensating a fresh enable")
		}
	})

	t.Run("WasEnabled true is no-op", func(t *testing.T) {
		sm := newMockServiceManager()
		sm.enabled["nginx"] = true

		p := newTestProvider(sm)
		if err := p.CompensateEnable(Tombstone{Name: "nginx", WasEnabled: true}); err != nil {
			t.Fatalf("CompensateEnable() error = %v", err)
		}
		if !sm.enabled["nginx"] {
			t.Error("service should still be enabled when WasEnabled=true")
		}
	})
}

func TestDisable(t *testing.T) {
	sm := newMockServiceManager()
	sm.enabled["nginx"] = true

	p := newTestProvider(sm)
	result, state, err := p.Disable(Resource{Name: "nginx"})
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if result.Name != "nginx" {
		t.Errorf("Disable() result.ReceiverName = %q, want %q", result.Name, "nginx")
	}
	if !state.WasEnabled {
		t.Error("Disable() WasEnabled = false, want true")
	}
	if sm.enabled["nginx"] {
		t.Error("service should be disabled after Disable()")
	}
}

func TestCompensateDisable(t *testing.T) {
	t.Run("WasEnabled true calls Enable", func(t *testing.T) {
		sm := newMockServiceManager()
		sm.enabled["nginx"] = false

		p := newTestProvider(sm)
		if err := p.CompensateDisable(Tombstone{Name: "nginx", WasEnabled: true}); err != nil {
			t.Fatalf("CompensateDisable() error = %v", err)
		}
		if !sm.enabled["nginx"] {
			t.Error("service should be enabled after compensating a disable")
		}
	})

	t.Run("WasEnabled false is no-op", func(t *testing.T) {
		sm := newMockServiceManager()
		sm.enabled["nginx"] = false

		p := newTestProvider(sm)
		if err := p.CompensateDisable(Tombstone{Name: "nginx", WasEnabled: false}); err != nil {
			t.Fatalf("CompensateDisable() error = %v", err)
		}
		if sm.enabled["nginx"] {
			t.Error("service should remain disabled when WasEnabled=false")
		}
	})
}

func TestPredicates(t *testing.T) {
	sm := newMockServiceManager()
	sm.exists["nginx"] = true
	sm.exists["missing"] = false
	sm.running["nginx"] = true
	sm.running["stopped"] = false
	sm.enabled["nginx"] = true
	sm.enabled["disabled"] = false

	p := newTestProvider(sm)

	t.Run("Exists true", func(t *testing.T) {
		got, err := p.Exists(Resource{Name: "nginx"})
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if !got {
			t.Error("Exists(nginx) = false, want true")
		}
	})

	t.Run("Exists false", func(t *testing.T) {
		got, err := p.Exists(Resource{Name: "missing"})
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if got {
			t.Error("Exists(missing) = true, want false")
		}
	})

	t.Run("Running true", func(t *testing.T) {
		got, err := p.Running(Resource{Name: "nginx"})
		if err != nil {
			t.Fatalf("Running() error = %v", err)
		}
		if !got {
			t.Error("Running(nginx) = false, want true")
		}
	})

	t.Run("Running false", func(t *testing.T) {
		got, err := p.Running(Resource{Name: "stopped"})
		if err != nil {
			t.Fatalf("Running() error = %v", err)
		}
		if got {
			t.Error("Running(stopped) = true, want false")
		}
	})

	t.Run("Enabled true", func(t *testing.T) {
		got, err := p.Enabled(Resource{Name: "nginx"})
		if err != nil {
			t.Fatalf("Enabled() error = %v", err)
		}
		if !got {
			t.Error("Enabled(nginx) = false, want true")
		}
	})

	t.Run("Enabled false", func(t *testing.T) {
		got, err := p.Enabled(Resource{Name: "disabled"})
		if err != nil {
			t.Fatalf("Enabled() error = %v", err)
		}
		if got {
			t.Error("Enabled(disabled) = true, want false")
		}
	})
}
