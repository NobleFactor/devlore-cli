// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
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
// Compensable Forward methods return (map[string]any, error).
// The map is the compensation receipt — opaque to the executor,
// meaningful only to the corresponding Compensate* Backward method.
type Provider struct {
	// Test hooks. Nil means use real platform implementation.
	runFn       func(io.Writer, string, ...string) error
	isRunningFn func(string) bool
	isEnabledFn func(string) bool
}

// Start starts a service. Returns compensation state with pre-action
// running status.
func (p *Provider) Start(name string, output io.Writer) (map[string]any, error) {
	wasRunning := p.isRunning(name)

	if err := p.run(output, startArgs(name)...); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":        name,
		"was_running": wasRunning,
	}, nil
}

// CompensateStart undoes a Start by stopping the service if it wasn't
// running before.
func (p *Provider) CompensateStart(state map[string]any, output io.Writer) error {
	name, _ := state["name"].(string)
	if name == "" {
		return nil
	}
	wasRunning, _ := state["was_running"].(bool)
	if wasRunning {
		return nil // Was already running — no-op
	}
	return p.run(output, stopArgs(name)...)
}

// Stop stops a service. Returns compensation state with pre-action
// running status.
func (p *Provider) Stop(name string, output io.Writer) (map[string]any, error) {
	wasRunning := p.isRunning(name)

	if err := p.run(output, stopArgs(name)...); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":        name,
		"was_running": wasRunning,
	}, nil
}

// CompensateStop undoes a Stop by starting the service if it was
// running before.
func (p *Provider) CompensateStop(state map[string]any, output io.Writer) error {
	name, _ := state["name"].(string)
	if name == "" {
		return nil
	}
	wasRunning, _ := state["was_running"].(bool)
	if !wasRunning {
		return nil // Wasn't running — no-op
	}
	return p.run(output, startArgs(name)...)
}

// Restart restarts a service. Returns compensation state. Compensation
// is a no-op — if the service was restarted, it was already running.
func (p *Provider) Restart(name string, output io.Writer) (map[string]any, error) {
	if err := p.run(output, restartArgs(name)...); err != nil {
		return nil, err
	}
	return map[string]any{
		"name": name,
	}, nil
}

// CompensateRestart is a no-op. A restarted service was already running;
// the service is still running after restart — nothing to undo.
func (p *Provider) CompensateRestart(_ map[string]any, _ io.Writer) error {
	return nil
}

// Enable enables a service to start at boot. Returns compensation state
// with pre-action enabled status.
func (p *Provider) Enable(name string, output io.Writer) (map[string]any, error) {
	wasEnabled := p.isEnabled(name)

	if err := p.run(output, enableArgs(name)...); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":        name,
		"was_enabled": wasEnabled,
	}, nil
}

// CompensateEnable undoes an Enable by disabling the service if it
// wasn't enabled before.
func (p *Provider) CompensateEnable(state map[string]any, output io.Writer) error {
	name, _ := state["name"].(string)
	if name == "" {
		return nil
	}
	wasEnabled, _ := state["was_enabled"].(bool)
	if wasEnabled {
		return nil // Was already enabled — no-op
	}
	return p.run(output, disableArgs(name)...)
}

// Disable disables a service from starting at boot. Returns
// compensation state with pre-action enabled status.
func (p *Provider) Disable(name string, output io.Writer) (map[string]any, error) {
	wasEnabled := p.isEnabled(name)

	if err := p.run(output, disableArgs(name)...); err != nil {
		return nil, err
	}
	return map[string]any{
		"name":        name,
		"was_enabled": wasEnabled,
	}, nil
}

// CompensateDisable undoes a Disable by enabling the service if it was
// enabled before.
func (p *Provider) CompensateDisable(state map[string]any, output io.Writer) error {
	name, _ := state["name"].(string)
	if name == "" {
		return nil
	}
	wasEnabled, _ := state["was_enabled"].(bool)
	if !wasEnabled {
		return nil // Wasn't enabled — no-op
	}
	return p.run(output, enableArgs(name)...)
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
	cmd := exec.Command(args[0], args[1:]...)
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
		return exec.Command("systemctl", "is-active", "--quiet", name).Run() == nil
	case "darwin":
		out, err := exec.Command("launchctl", "list", name).Output()
		if err != nil {
			return false
		}
		// launchctl list <name> shows PID in first column; "-" means not running
		fields := strings.Fields(string(out))
		return len(fields) > 0 && fields[0] != "-"
	case "windows":
		out, err := exec.Command("sc", "query", name).Output()
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
		return exec.Command("systemctl", "is-enabled", "--quiet", name).Run() == nil
	case "windows":
		out, err := exec.Command("sc", "qc", name).Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), "AUTO_START")
	default:
		// macOS launchd has no clean is-enabled query — conservative default
		return false
	}
}
