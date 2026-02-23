// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package service provides platform-agnostic service management actions.
package service

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
)

// Provider provides platform-agnostic service management.
// Platform detection happens at runtime — callers don't need to know
// whether launchd, systemd, or Windows services are being used.
//
// Compensable Forward methods return (string, map[string]any, error):
// the service name, the compensation receipt, and an error. The map is
// opaque to the executor, meaningful only to the corresponding
// Compensate* Backward method.
type Provider struct {
	// Test hooks. Nil means use real platform implementation.
	runFn       func(io.Writer, string, ...string) error
	isRunningFn func(string) bool
	isEnabledFn func(string) bool
}

// Start starts a service. Returns compensation state with pre-action
// running status.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
//
// +devlore:access=planned
func (p *Provider) Start(name string, output io.Writer) (result string, state map[string]any, retErr error) {
	wasRunning := p.isRunning(name)

	if err := p.run(output, startArgs(name)...); err != nil {
		return "", nil, err
	}
	return name, map[string]any{
		"name":        name,
		"was_running": wasRunning,
	}, nil
}

// CompensateStart undoes a Start by stopping the service if it wasn't
// running before.
func (p *Provider) CompensateStart(state any) error {
	s, ok := state.(map[string]any)
	if !ok || s == nil {
		return nil
	}
	name, ok := s["name"].(string)
	if !ok || name == "" {
		return nil
	}
	wasRunning, ok := s["was_running"].(bool)
	if ok && wasRunning {
		return nil // Was already running — no-op
	}
	return p.run(io.Discard, stopArgs(name)...)
}

// Stop stops a service. Returns compensation state with pre-action
// running status.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
//
// +devlore:access=planned
func (p *Provider) Stop(name string, output io.Writer) (result string, state map[string]any, retErr error) {
	wasRunning := p.isRunning(name)

	if err := p.run(output, stopArgs(name)...); err != nil {
		return "", nil, err
	}
	return name, map[string]any{
		"name":        name,
		"was_running": wasRunning,
	}, nil
}

// CompensateStop undoes a Stop by starting the service if it was
// running before.
func (p *Provider) CompensateStop(state any) error {
	s, ok := state.(map[string]any)
	if !ok || s == nil {
		return nil
	}
	name, ok := s["name"].(string)
	if !ok || name == "" {
		return nil
	}
	wasRunning, ok := s["was_running"].(bool)
	if !ok || !wasRunning {
		return nil // Wasn't running — no-op
	}
	return p.run(io.Discard, startArgs(name)...)
}

// Restart restarts a service. Returns compensation state. Compensation
// is a no-op — if the service was restarted, it was already running.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
//
// +devlore:access=planned
func (p *Provider) Restart(name string, output io.Writer) (result string, state map[string]any, retErr error) {
	if err := p.run(output, restartArgs(name)...); err != nil {
		return "", nil, err
	}
	return name, map[string]any{
		"name": name,
	}, nil
}

// CompensateRestart is a no-op. A restarted service was already running;
// the service is still running after restart — nothing to undo.
func (p *Provider) CompensateRestart(_ any) error {
	return nil
}

// Enable enables a service to start at boot. Returns compensation state
// with pre-action enabled status.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
//
// +devlore:access=planned
func (p *Provider) Enable(name string, output io.Writer) (result string, state map[string]any, retErr error) {
	wasEnabled := p.isEnabled(name)

	if err := p.run(output, enableArgs(name)...); err != nil {
		return "", nil, err
	}
	return name, map[string]any{
		"name":        name,
		"was_enabled": wasEnabled,
	}, nil
}

// CompensateEnable undoes an Enable by disabling the service if it
// wasn't enabled before.
func (p *Provider) CompensateEnable(state any) error {
	s, ok := state.(map[string]any)
	if !ok || s == nil {
		return nil
	}
	name, ok := s["name"].(string)
	if !ok || name == "" {
		return nil
	}
	wasEnabled, ok := s["was_enabled"].(bool)
	if ok && wasEnabled {
		return nil // Was already enabled — no-op
	}
	return p.run(io.Discard, disableArgs(name)...)
}

// Disable disables a service from starting at boot. Returns
// compensation state with pre-action enabled status.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
//
// +devlore:access=planned
func (p *Provider) Disable(name string, output io.Writer) (result string, state map[string]any, retErr error) {
	wasEnabled := p.isEnabled(name)

	if err := p.run(output, disableArgs(name)...); err != nil {
		return "", nil, err
	}
	return name, map[string]any{
		"name":        name,
		"was_enabled": wasEnabled,
	}, nil
}

// CompensateDisable undoes a Disable by enabling the service if it was
// enabled before.
func (p *Provider) CompensateDisable(state any) error {
	s, ok := state.(map[string]any)
	if !ok || s == nil {
		return nil
	}
	name, ok := s["name"].(string)
	if !ok || name == "" {
		return nil
	}
	wasEnabled, ok := s["was_enabled"].(bool)
	if !ok || !wasEnabled {
		return nil // Wasn't enabled — no-op
	}
	return p.run(io.Discard, enableArgs(name)...)
}

// --- Predicates ---

// Exists returns true if the named service exists on the system.
//
// Parameters:
//   - name: Service name to check
//
// +devlore:access=both
func (p *Provider) Exists(name string) (bool, error) {
	switch runtime.GOOS {
	case "linux":
		return exec.CommandContext(context.Background(), "systemctl", "cat", name).Run() == nil, nil //nolint:gosec // G204: command built from provider inputs
	case "darwin":
		_, err := exec.CommandContext(context.Background(), "launchctl", "list", name).Output() //nolint:gosec // G204: command built from provider inputs
		return err == nil, nil
	case "windows":
		return exec.CommandContext(context.Background(), "sc", "query", name).Run() == nil, nil //nolint:gosec // G204: command built from provider inputs
	default:
		return false, nil
	}
}

// Running returns true if the named service is currently running.
//
// Parameters:
//   - name: Service name to check
//
// +devlore:access=both
func (p *Provider) Running(name string) (bool, error) {
	return p.isRunning(name), nil
}

// Enabled returns true if the named service is enabled to start at boot.
//
// Parameters:
//   - name: Service name to check
//
// +devlore:access=both
func (p *Provider) Enabled(name string) (bool, error) {
	return p.isEnabled(name), nil
}

// --- Platform-specific command builders ---

func startArgs(name string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"launchctl", "start", name}
	case "linux":
		return []string{"sudo", "systemctl", "start", name}
	case "windows":
		return []string{"sc", "start", name}
	default:
		return nil
	}
}

func stopArgs(name string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"launchctl", "stop", name}
	case "linux":
		return []string{"sudo", "systemctl", "stop", name}
	case "windows":
		return []string{"sc", "stop", name}
	default:
		return nil
	}
}

func restartArgs(name string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"launchctl", "kickstart", "-k", "gui/" + name}
	case "linux":
		return []string{"sudo", "systemctl", "restart", name}
	case "windows":
		// Windows has no native restart — handled in run()
		return []string{"sc", "start", name}
	default:
		return nil
	}
}

func enableArgs(name string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"launchctl", "enable", "gui/" + name}
	case "linux":
		return []string{"sudo", "systemctl", "enable", name}
	case "windows":
		return []string{"sc", "config", name, "start=auto"}
	default:
		return nil
	}
}

func disableArgs(name string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"launchctl", "disable", "gui/" + name}
	case "linux":
		return []string{"sudo", "systemctl", "disable", name}
	case "windows":
		return []string{"sc", "config", name, "start=disabled"}
	default:
		return nil
	}
}

// --- Internal helpers ---

func (p *Provider) run(output io.Writer, args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	if p.runFn != nil {
		return p.runFn(output, args[0], args[1:]...)
	}
	cmd := exec.CommandContext(context.Background(), args[0], args[1:]...) //nolint:gosec // G204: service management requires dynamic command construction
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

func (p *Provider) isRunning(name string) bool {
	if p.isRunningFn != nil {
		return p.isRunningFn(name)
	}
	switch runtime.GOOS {
	case "linux":
		return exec.CommandContext(context.Background(), "systemctl", "is-active", "--quiet", name).Run() == nil //nolint:gosec // G204: command built from provider inputs
	case "darwin":
		out, err := exec.CommandContext(context.Background(), "launchctl", "list", name).Output() //nolint:gosec // G204: command built from provider inputs
		if err != nil {
			return false
		}
		// launchctl list <name> shows PID in first column; "-" means not running
		fields := strings.Fields(string(out))
		return len(fields) > 0 && fields[0] != "-"
	case "windows":
		out, err := exec.CommandContext(context.Background(), "sc", "query", name).Output() //nolint:gosec // G204: command built from provider inputs
		if err != nil {
			return false
		}
		return strings.Contains(string(out), "RUNNING")
	default:
		return false
	}
}

func (p *Provider) isEnabled(name string) bool {
	if p.isEnabledFn != nil {
		return p.isEnabledFn(name)
	}
	switch runtime.GOOS {
	case "linux":
		return exec.CommandContext(context.Background(), "systemctl", "is-enabled", "--quiet", name).Run() == nil //nolint:gosec // G204: command built from provider inputs
	case "windows":
		out, err := exec.CommandContext(context.Background(), "sc", "qc", name).Output() //nolint:gosec // G204: command built from provider inputs
		if err != nil {
			return false
		}
		return strings.Contains(string(out), "AUTO_START")
	default:
		// macOS launchd has no clean is-enabled query — conservative default
		return false
	}
}
