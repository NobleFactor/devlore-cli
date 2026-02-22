// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"io"
	"strings"
	"testing"
)

// mockProvider returns a Provider with test hooks that record commands
// instead of executing them.
func mockProvider(running, enabled bool) (*Provider, *[]string) {
	var log []string
	return &Provider{
		runFn: func(_ io.Writer, name string, args ...string) error {
			log = append(log, name+" "+strings.Join(args, " "))
			return nil
		},
		isRunningFn: func(_ string) bool { return running },
		isEnabledFn: func(_ string) bool { return enabled },
	}, &log
}

// --- Start ---

func TestStartNotRunning(t *testing.T) {
	p, log := mockProvider(false, false)
	_, state, err := p.Start("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	wasRunning, _ := state["was_running"].(bool)
	if wasRunning {
		t.Error("was_running should be false")
	}

	// Compensate: should stop (wasn't running before)
	if err := p.CompensateStart(state); err != nil {
		t.Fatalf("CompensateStart: %v", err)
	}
	if len(*log) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(*log), *log)
	}
	if !strings.Contains((*log)[1], "stop") {
		t.Errorf("expected stop command, got %q", (*log)[1])
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	p, log := mockProvider(true, false)
	_, state, err := p.Start("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	wasRunning, _ := state["was_running"].(bool)
	if !wasRunning {
		t.Error("was_running should be true")
	}

	// Compensate: should be no-op (was already running)
	if err := p.CompensateStart(state); err != nil {
		t.Fatalf("CompensateStart: %v", err)
	}
	if len(*log) != 1 {
		t.Errorf("expected 1 command (start only, no stop), got %d: %v", len(*log), *log)
	}
}

// --- Stop ---

func TestStopWasRunning(t *testing.T) {
	p, log := mockProvider(true, false)
	_, state, err := p.Stop("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	wasRunning, _ := state["was_running"].(bool)
	if !wasRunning {
		t.Error("was_running should be true")
	}

	// Compensate: should start (was running before)
	if err := p.CompensateStop(state); err != nil {
		t.Fatalf("CompensateStop: %v", err)
	}
	if len(*log) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(*log), *log)
	}
	if !strings.Contains((*log)[1], "start") {
		t.Errorf("expected start command, got %q", (*log)[1])
	}
}

func TestStopNotRunning(t *testing.T) {
	p, log := mockProvider(false, false)
	_, state, err := p.Stop("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Compensate: should be no-op (wasn't running)
	if err := p.CompensateStop(state); err != nil {
		t.Fatalf("CompensateStop: %v", err)
	}
	if len(*log) != 1 {
		t.Errorf("expected 1 command (stop only, no start), got %d: %v", len(*log), *log)
	}
}

// --- Restart ---

func TestRestartCompensateNoOp(t *testing.T) {
	p, log := mockProvider(true, false)
	_, state, err := p.Restart("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}

	// Compensate: always no-op
	if err := p.CompensateRestart(state); err != nil {
		t.Fatalf("CompensateRestart: %v", err)
	}
	if len(*log) != 1 {
		t.Errorf("expected 1 command (restart only), got %d: %v", len(*log), *log)
	}
}

// --- Enable ---

func TestEnableNotEnabled(t *testing.T) {
	p, log := mockProvider(false, false)
	_, state, err := p.Enable("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}

	wasEnabled, _ := state["was_enabled"].(bool)
	if wasEnabled {
		t.Error("was_enabled should be false")
	}

	// Compensate: should disable (wasn't enabled before)
	if err := p.CompensateEnable(state); err != nil {
		t.Fatalf("CompensateEnable: %v", err)
	}
	if len(*log) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(*log), *log)
	}
	if !strings.Contains((*log)[1], "disable") {
		t.Errorf("expected disable command, got %q", (*log)[1])
	}
}

func TestEnableAlreadyEnabled(t *testing.T) {
	p, log := mockProvider(false, true)
	_, state, err := p.Enable("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}

	// Compensate: should be no-op (was already enabled)
	if err := p.CompensateEnable(state); err != nil {
		t.Fatalf("CompensateEnable: %v", err)
	}
	if len(*log) != 1 {
		t.Errorf("expected 1 command (enable only, no disable), got %d: %v", len(*log), *log)
	}
}

// --- Disable ---

func TestDisableWasEnabled(t *testing.T) {
	p, log := mockProvider(false, true)
	_, state, err := p.Disable("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Disable: %v", err)
	}

	wasEnabled, _ := state["was_enabled"].(bool)
	if !wasEnabled {
		t.Error("was_enabled should be true")
	}

	// Compensate: should enable (was enabled before)
	if err := p.CompensateDisable(state); err != nil {
		t.Fatalf("CompensateDisable: %v", err)
	}
	if len(*log) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(*log), *log)
	}
	if !strings.Contains((*log)[1], "enable") {
		t.Errorf("expected enable command, got %q", (*log)[1])
	}
}

func TestDisableNotEnabled(t *testing.T) {
	p, log := mockProvider(false, false)
	_, state, err := p.Disable("nginx", io.Discard)
	if err != nil {
		t.Fatalf("Disable: %v", err)
	}

	// Compensate: should be no-op (wasn't enabled)
	if err := p.CompensateDisable(state); err != nil {
		t.Fatalf("CompensateDisable: %v", err)
	}
	if len(*log) != 1 {
		t.Errorf("expected 1 command (disable only, no enable), got %d: %v", len(*log), *log)
	}
}

// --- Nil state safety ---

func TestCompensateNilState(t *testing.T) {
	p, _ := mockProvider(false, false)

	if err := p.CompensateStart(nil); err != nil {
		t.Errorf("CompensateStart(nil): %v", err)
	}
	if err := p.CompensateStop(nil); err != nil {
		t.Errorf("CompensateStop(nil): %v", err)
	}
	if err := p.CompensateRestart(nil); err != nil {
		t.Errorf("CompensateRestart(nil): %v", err)
	}
	if err := p.CompensateEnable(nil); err != nil {
		t.Errorf("CompensateEnable(nil): %v", err)
	}
	if err := p.CompensateDisable(nil); err != nil {
		t.Errorf("CompensateDisable(nil): %v", err)
	}
}
