// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"errors"
	"io"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// mockSvcManager implements op.ServiceManagerProvider for testing.
type mockSvcManager struct {
	exists     map[string]bool
	running    map[string]bool
	enabled    map[string]bool
	startErr   error
	stopErr    error
	enableErr  error
	disableErr error
}

func newMockSvcManager() *mockSvcManager {
	return &mockSvcManager{
		exists:  make(map[string]bool),
		running: make(map[string]bool),
		enabled: make(map[string]bool),
	}
}

func (m *mockSvcManager) Exists(name string) bool    { return m.exists[name] }
func (m *mockSvcManager) IsRunning(name string) bool { return m.running[name] }
func (m *mockSvcManager) IsEnabled(name string) bool { return m.enabled[name] }

func (m *mockSvcManager) Start(name string) error {
	if m.startErr != nil {
		return m.startErr
	}
	m.running[name] = true
	return nil
}

func (m *mockSvcManager) Stop(name string) error {
	if m.stopErr != nil {
		return m.stopErr
	}
	m.running[name] = false
	return nil
}

func (m *mockSvcManager) Enable(name string) error {
	if m.enableErr != nil {
		return m.enableErr
	}
	m.enabled[name] = true
	return nil
}

func (m *mockSvcManager) Disable(name string) error {
	if m.disableErr != nil {
		return m.disableErr
	}
	m.enabled[name] = false
	return nil
}

func TestStart(t *testing.T) {
	svc := newMockSvcManager()
	svc.exists["nginx"] = true
	svc.running["nginx"] = false

	p := &Provider{}
	name, state, err := p.Start(svc, "nginx", io.Discard)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if name != "nginx" {
		t.Errorf("Start() name = %q, want %q", name, "nginx")
	}
	if op.StateBool(state, "was_running") {
		t.Error("Start() was_running = true, want false")
	}
	if !svc.running["nginx"] {
		t.Error("service should be running after Start()")
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	svc := newMockSvcManager()
	svc.exists["nginx"] = true
	svc.running["nginx"] = true

	p := &Provider{}
	name, state, err := p.Start(svc, "nginx", io.Discard)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if name != "nginx" {
		t.Errorf("Start() name = %q, want %q", name, "nginx")
	}
	if !op.StateBool(state, "was_running") {
		t.Error("Start() was_running = false, want true")
	}
}

func TestStartError(t *testing.T) {
	svc := newMockSvcManager()
	svc.startErr = errors.New("permission denied")

	p := &Provider{}
	_, _, err := p.Start(svc, "nginx", io.Discard)
	if err == nil {
		t.Fatal("Start() expected error, got nil")
	}
	if err.Error() != "permission denied" {
		t.Errorf("Start() error = %q, want %q", err.Error(), "permission denied")
	}
}

func TestCompensateStart(t *testing.T) {
	t.Run("was_running false calls Stop", func(t *testing.T) {
		svc := newMockSvcManager()
		svc.running["nginx"] = true

		p := &Provider{}
		state := map[string]any{"name": "nginx", "was_running": false}
		if err := p.CompensateStart(svc, state); err != nil {
			t.Fatalf("CompensateStart() error = %v", err)
		}
		if svc.running["nginx"] {
			t.Error("service should be stopped after compensating a fresh start")
		}
	})

	t.Run("was_running true is no-op", func(t *testing.T) {
		svc := newMockSvcManager()
		svc.running["nginx"] = true

		p := &Provider{}
		state := map[string]any{"name": "nginx", "was_running": true}
		if err := p.CompensateStart(svc, state); err != nil {
			t.Fatalf("CompensateStart() error = %v", err)
		}
		if !svc.running["nginx"] {
			t.Error("service should still be running when was_running=true")
		}
	})

	t.Run("nil state is no-op", func(t *testing.T) {
		svc := newMockSvcManager()
		p := &Provider{}
		if err := p.CompensateStart(svc, nil); err != nil {
			t.Fatalf("CompensateStart(nil) error = %v", err)
		}
	})
}

func TestStop(t *testing.T) {
	svc := newMockSvcManager()
	svc.running["nginx"] = true

	p := &Provider{}
	name, state, err := p.Stop(svc, "nginx", io.Discard)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if name != "nginx" {
		t.Errorf("Stop() name = %q, want %q", name, "nginx")
	}
	if !op.StateBool(state, "was_running") {
		t.Error("Stop() was_running = false, want true")
	}
	if svc.running["nginx"] {
		t.Error("service should be stopped after Stop()")
	}
}

func TestCompensateStop(t *testing.T) {
	t.Run("was_running true calls Start", func(t *testing.T) {
		svc := newMockSvcManager()
		svc.running["nginx"] = false

		p := &Provider{}
		state := map[string]any{"name": "nginx", "was_running": true}
		if err := p.CompensateStop(svc, state); err != nil {
			t.Fatalf("CompensateStop() error = %v", err)
		}
		if !svc.running["nginx"] {
			t.Error("service should be running after compensating a stop")
		}
	})

	t.Run("was_running false is no-op", func(t *testing.T) {
		svc := newMockSvcManager()
		svc.running["nginx"] = false

		p := &Provider{}
		state := map[string]any{"name": "nginx", "was_running": false}
		if err := p.CompensateStop(svc, state); err != nil {
			t.Fatalf("CompensateStop() error = %v", err)
		}
		if svc.running["nginx"] {
			t.Error("service should remain stopped when was_running=false")
		}
	})
}

func TestRestart(t *testing.T) {
	svc := newMockSvcManager()
	svc.running["nginx"] = true

	p := &Provider{}
	name, state, err := p.Restart(svc, "nginx", io.Discard)
	if err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if name != "nginx" {
		t.Errorf("Restart() name = %q, want %q", name, "nginx")
	}
	if state == nil {
		t.Fatal("Restart() state is nil")
	}
	if op.StateString(state, "name") != "nginx" {
		t.Errorf("Restart() state name = %q, want %q", op.StateString(state, "name"), "nginx")
	}
	if !svc.running["nginx"] {
		t.Error("service should be running after Restart()")
	}
}

func TestRestartError(t *testing.T) {
	t.Run("Stop fails", func(t *testing.T) {
		svc := newMockSvcManager()
		svc.stopErr = errors.New("stop failed")

		p := &Provider{}
		_, _, err := p.Restart(svc, "nginx", io.Discard)
		if err == nil {
			t.Fatal("Restart() expected error from Stop, got nil")
		}
	})

	t.Run("Stop OK but Start fails", func(t *testing.T) {
		svc := newMockSvcManager()
		svc.startErr = errors.New("start failed")

		p := &Provider{}
		_, _, err := p.Restart(svc, "nginx", io.Discard)
		if err == nil {
			t.Fatal("Restart() expected error from Start, got nil")
		}
	})
}

func TestEnable(t *testing.T) {
	svc := newMockSvcManager()
	svc.enabled["nginx"] = false

	p := &Provider{}
	name, state, err := p.Enable(svc, "nginx", io.Discard)
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if name != "nginx" {
		t.Errorf("Enable() name = %q, want %q", name, "nginx")
	}
	if op.StateBool(state, "was_enabled") {
		t.Error("Enable() was_enabled = true, want false")
	}
	if !svc.enabled["nginx"] {
		t.Error("service should be enabled after Enable()")
	}
}

func TestCompensateEnable(t *testing.T) {
	t.Run("was_enabled false calls Disable", func(t *testing.T) {
		svc := newMockSvcManager()
		svc.enabled["nginx"] = true

		p := &Provider{}
		state := map[string]any{"name": "nginx", "was_enabled": false}
		if err := p.CompensateEnable(svc, state); err != nil {
			t.Fatalf("CompensateEnable() error = %v", err)
		}
		if svc.enabled["nginx"] {
			t.Error("service should be disabled after compensating a fresh enable")
		}
	})

	t.Run("was_enabled true is no-op", func(t *testing.T) {
		svc := newMockSvcManager()
		svc.enabled["nginx"] = true

		p := &Provider{}
		state := map[string]any{"name": "nginx", "was_enabled": true}
		if err := p.CompensateEnable(svc, state); err != nil {
			t.Fatalf("CompensateEnable() error = %v", err)
		}
		if !svc.enabled["nginx"] {
			t.Error("service should still be enabled when was_enabled=true")
		}
	})
}

func TestDisable(t *testing.T) {
	svc := newMockSvcManager()
	svc.enabled["nginx"] = true

	p := &Provider{}
	name, state, err := p.Disable(svc, "nginx", io.Discard)
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if name != "nginx" {
		t.Errorf("Disable() name = %q, want %q", name, "nginx")
	}
	if !op.StateBool(state, "was_enabled") {
		t.Error("Disable() was_enabled = false, want true")
	}
	if svc.enabled["nginx"] {
		t.Error("service should be disabled after Disable()")
	}
}

func TestCompensateDisable(t *testing.T) {
	t.Run("was_enabled true calls Enable", func(t *testing.T) {
		svc := newMockSvcManager()
		svc.enabled["nginx"] = false

		p := &Provider{}
		state := map[string]any{"name": "nginx", "was_enabled": true}
		if err := p.CompensateDisable(svc, state); err != nil {
			t.Fatalf("CompensateDisable() error = %v", err)
		}
		if !svc.enabled["nginx"] {
			t.Error("service should be enabled after compensating a disable")
		}
	})

	t.Run("was_enabled false is no-op", func(t *testing.T) {
		svc := newMockSvcManager()
		svc.enabled["nginx"] = false

		p := &Provider{}
		state := map[string]any{"name": "nginx", "was_enabled": false}
		if err := p.CompensateDisable(svc, state); err != nil {
			t.Fatalf("CompensateDisable() error = %v", err)
		}
		if svc.enabled["nginx"] {
			t.Error("service should remain disabled when was_enabled=false")
		}
	})
}

func TestPredicates(t *testing.T) {
	svc := newMockSvcManager()
	svc.exists["nginx"] = true
	svc.exists["missing"] = false
	svc.running["nginx"] = true
	svc.running["stopped"] = false
	svc.enabled["nginx"] = true
	svc.enabled["disabled"] = false

	p := &Provider{}

	t.Run("Exists true", func(t *testing.T) {
		got, err := p.Exists(svc, "nginx")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if !got {
			t.Error("Exists(nginx) = false, want true")
		}
	})

	t.Run("Exists false", func(t *testing.T) {
		got, err := p.Exists(svc, "missing")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if got {
			t.Error("Exists(missing) = true, want false")
		}
	})

	t.Run("Running true", func(t *testing.T) {
		got, err := p.Running(svc, "nginx")
		if err != nil {
			t.Fatalf("Running() error = %v", err)
		}
		if !got {
			t.Error("Running(nginx) = false, want true")
		}
	})

	t.Run("Running false", func(t *testing.T) {
		got, err := p.Running(svc, "stopped")
		if err != nil {
			t.Fatalf("Running() error = %v", err)
		}
		if got {
			t.Error("Running(stopped) = true, want false")
		}
	})

	t.Run("Enabled true", func(t *testing.T) {
		got, err := p.Enabled(svc, "nginx")
		if err != nil {
			t.Fatalf("Enabled() error = %v", err)
		}
		if !got {
			t.Error("Enabled(nginx) = false, want true")
		}
	})

	t.Run("Enabled false", func(t *testing.T) {
		got, err := p.Enabled(svc, "disabled")
		if err != nil {
			t.Fatalf("Enabled() error = %v", err)
		}
		if got {
			t.Error("Enabled(disabled) = true, want false")
		}
	})
}
