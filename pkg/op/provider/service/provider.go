// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package service provides platform-agnostic service management actions.
package service

import (
	"fmt"
	"io"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides platform-agnostic service management.
// All platform-specific behavior is delegated to the Platform
// injected at construction time from op.Context.Platform.
//
// Compensable Forward methods return (string, map[string]any, error):
// the service name, the compensation receipt, and an error. The map is
// opaque to the executor, meaningful only to the corresponding
// Compensate* Backward method.
//
// +devlore:access=both
type Provider struct {
	Platform *op.Platform
}

// ── Compensable Pairs ────────────────────────────────────────────────

// Disable disables a service from starting at boot. Returns
// compensation state with pre-action enabled status.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
func (p *Provider) Disable(name string, output io.Writer) (result string, state map[string]any, err error) {
	serviceManager := p.Platform.ServiceManager
	if serviceManager == nil {
		return "", nil, fmt.Errorf("no service manager available")
	}

	wasEnabled := serviceManager.IsEnabled(name)

	r := serviceManager.Disable(name)
	if !r.OK {
		return "", nil, fmt.Errorf("disable %s failed: %s", name, r.Stderr)
	}
	_, _ = fmt.Fprintf(output, "disabled service %s\n", name) //nolint:errcheck // status output to writer
	return name, map[string]any{
		"name":        name,
		"was_enabled": wasEnabled,
	}, nil
}

// CompensateDisable undoes a Disable by enabling the service if it was
// enabled before.
func (p *Provider) CompensateDisable(state any) error {
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	name := op.StateString(s, "name")
	if name == "" {
		return nil
	}
	if !op.StateBool(s, "was_enabled") {
		return nil // Wasn't enabled — no-op
	}

	serviceManager := p.Platform.ServiceManager
	if serviceManager == nil {
		return fmt.Errorf("no service manager available for compensation")
	}
	r := serviceManager.Enable(name)
	if !r.OK {
		return fmt.Errorf("compensate disable (enable %s) failed: %s", name, r.Stderr)
	}
	return nil
}

// Enable enables a service to start at boot. Returns compensation state
// with pre-action enabled status.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
func (p *Provider) Enable(name string, output io.Writer) (result string, state map[string]any, err error) {
	serviceManager := p.Platform.ServiceManager
	if serviceManager == nil {
		return "", nil, fmt.Errorf("no service manager available")
	}

	wasEnabled := serviceManager.IsEnabled(name)

	r := serviceManager.Enable(name)
	if !r.OK {
		return "", nil, fmt.Errorf("enable %s failed: %s", name, r.Stderr)
	}
	_, _ = fmt.Fprintf(output, "enabled service %s\n", name) //nolint:errcheck // status output to writer
	return name, map[string]any{
		"name":        name,
		"was_enabled": wasEnabled,
	}, nil
}

// CompensateEnable undoes an Enable by disabling the service if it
// wasn't enabled before.
func (p *Provider) CompensateEnable(state any) error {
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	name := op.StateString(s, "name")
	if name == "" {
		return nil
	}
	if op.StateBool(s, "was_enabled") {
		return nil // Was already enabled — no-op
	}

	serviceManager := p.Platform.ServiceManager
	if serviceManager == nil {
		return fmt.Errorf("no service manager available for compensation")
	}
	r := serviceManager.Disable(name)
	if !r.OK {
		return fmt.Errorf("compensate enable (disable %s) failed: %s", name, r.Stderr)
	}
	return nil
}

// Restart restarts a service. Returns compensation state. Compensation
// is a no-op — if the service was restarted, it was already running.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
func (p *Provider) Restart(name string, output io.Writer) (result string, state map[string]any, err error) {
	serviceManager := p.Platform.ServiceManager
	if serviceManager == nil {
		return "", nil, fmt.Errorf("no service manager available")
	}

	r := serviceManager.Stop(name)
	if !r.OK {
		return "", nil, fmt.Errorf("stop before restart: %s", r.Stderr)
	}
	r = serviceManager.Start(name)
	if !r.OK {
		return "", nil, fmt.Errorf("start after restart: %s", r.Stderr)
	}
	_, _ = fmt.Fprintf(output, "restarted service %s\n", name) //nolint:errcheck // status output to writer
	return name, map[string]any{
		"name": name,
	}, nil
}

// CompensateRestart is a no-op. A restarted service was already running;
// the service is still running after restart — nothing to undo.
func (p *Provider) CompensateRestart(_ any) error {
	return nil
}

// Start starts a service. Returns compensation state with pre-action
// running status.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
func (p *Provider) Start(name string, output io.Writer) (result string, state map[string]any, err error) {
	serviceManager := p.Platform.ServiceManager
	if serviceManager == nil {
		return "", nil, fmt.Errorf("no service manager available")
	}

	wasRunning := serviceManager.IsRunning(name)

	r := serviceManager.Start(name)
	if !r.OK {
		return "", nil, fmt.Errorf("start %s failed: %s", name, r.Stderr)
	}
	_, _ = fmt.Fprintf(output, "started service %s\n", name) //nolint:errcheck // status output to writer
	return name, map[string]any{
		"name":        name,
		"was_running": wasRunning,
	}, nil
}

// CompensateStart undoes a Start by stopping the service if it wasn't
// running before.
func (p *Provider) CompensateStart(state any) error {
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	name := op.StateString(s, "name")
	if name == "" {
		return nil
	}
	if op.StateBool(s, "was_running") {
		return nil // Was already running — no-op
	}

	serviceManager := p.Platform.ServiceManager
	if serviceManager == nil {
		return fmt.Errorf("no service manager available for compensation")
	}
	r := serviceManager.Stop(name)
	if !r.OK {
		return fmt.Errorf("compensate start (stop %s) failed: %s", name, r.Stderr)
	}
	return nil
}

// Stop stops a service. Returns compensation state with pre-action
// running status.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
func (p *Provider) Stop(name string, output io.Writer) (result string, state map[string]any, err error) {
	serviceManager := p.Platform.ServiceManager
	if serviceManager == nil {
		return "", nil, fmt.Errorf("no service manager available")
	}

	wasRunning := serviceManager.IsRunning(name)

	r := serviceManager.Stop(name)
	if !r.OK {
		return "", nil, fmt.Errorf("stop %s failed: %s", name, r.Stderr)
	}
	_, _ = fmt.Fprintf(output, "stopped service %s\n", name) //nolint:errcheck // status output to writer
	return name, map[string]any{
		"name":        name,
		"was_running": wasRunning,
	}, nil
}

// CompensateStop undoes a Stop by starting the service if it was
// running before.
func (p *Provider) CompensateStop(state any) error {
	s := op.AsStateMap(state)
	if s == nil {
		return nil
	}
	name := op.StateString(s, "name")
	if name == "" {
		return nil
	}
	if !op.StateBool(s, "was_running") {
		return nil // Wasn't running — no-op
	}

	serviceManager := p.Platform.ServiceManager
	if serviceManager == nil {
		return fmt.Errorf("no service manager available for compensation")
	}
	r := serviceManager.Start(name)
	if !r.OK {
		return fmt.Errorf("compensate stop (start %s) failed: %s", name, r.Stderr)
	}
	return nil
}

// ── Predicates ───────────────────────────────────────────────────────

// Enabled returns true if the named service is enabled to start at boot.
//
// Parameters:
//   - name: Service name to check
func (p *Provider) Enabled(name string) (bool, error) {
	serviceManager := p.Platform.ServiceManager
	if serviceManager == nil {
		return false, fmt.Errorf("no service manager available")
	}
	return serviceManager.IsEnabled(name), nil
}

// Exists returns true if the named service exists on the system.
//
// Parameters:
//   - name: Service name to check
func (p *Provider) Exists(name string) (bool, error) {
	serviceManager := p.Platform.ServiceManager
	if serviceManager == nil {
		return false, fmt.Errorf("no service manager available")
	}
	return serviceManager.Exists(name), nil
}

// Running returns true if the named service is currently running.
//
// Parameters:
//   - name: Service name to check
func (p *Provider) Running(name string) (bool, error) {
	serviceManager := p.Platform.ServiceManager
	if serviceManager == nil {
		return false, fmt.Errorf("no service manager available")
	}
	return serviceManager.IsRunning(name), nil
}
