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
// All platform-specific behavior is delegated to the ServiceManagerProvider
// injected via op.Context.Host.
//
// Compensable Forward methods return (string, map[string]any, error):
// the service name, the compensation receipt, and an error. The map is
// opaque to the executor, meaningful only to the corresponding
// Compensate* Backward method.
//
// +devlore:access=both
type Provider struct{}

// ── Compensable Pairs ────────────────────────────────────────────────

// Disable disables a service from starting at boot. Returns
// compensation state with pre-action enabled status.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
func (p *Provider) Disable(svc op.ServiceManagerProvider, name string, output io.Writer) (result string, state map[string]any, err error) {
	wasEnabled := svc.IsEnabled(name)

	if err := svc.Disable(name); err != nil {
		return "", nil, err
	}
	_, _ = fmt.Fprintf(output, "disabled service %s\n", name) //nolint:errcheck // status output to writer
	return name, map[string]any{
		"name":        name,
		"was_enabled": wasEnabled,
	}, nil
}

// CompensateDisable undoes a Disable by enabling the service if it was
// enabled before.
func (p *Provider) CompensateDisable(svc op.ServiceManagerProvider, state any) error {
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
	return svc.Enable(name)
}

// Enable enables a service to start at boot. Returns compensation state
// with pre-action enabled status.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
func (p *Provider) Enable(svc op.ServiceManagerProvider, name string, output io.Writer) (result string, state map[string]any, err error) {
	wasEnabled := svc.IsEnabled(name)

	if err := svc.Enable(name); err != nil {
		return "", nil, err
	}
	_, _ = fmt.Fprintf(output, "enabled service %s\n", name) //nolint:errcheck // status output to writer
	return name, map[string]any{
		"name":        name,
		"was_enabled": wasEnabled,
	}, nil
}

// CompensateEnable undoes an Enable by disabling the service if it
// wasn't enabled before.
func (p *Provider) CompensateEnable(svc op.ServiceManagerProvider, state any) error {
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
	return svc.Disable(name)
}

// Restart restarts a service. Returns compensation state. Compensation
// is a no-op — if the service was restarted, it was already running.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
func (p *Provider) Restart(svc op.ServiceManagerProvider, name string, output io.Writer) (result string, state map[string]any, err error) {
	if err := svc.Stop(name); err != nil {
		return "", nil, fmt.Errorf("stop before restart: %w", err)
	}
	if err := svc.Start(name); err != nil {
		return "", nil, fmt.Errorf("start after restart: %w", err)
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
func (p *Provider) Start(svc op.ServiceManagerProvider, name string, output io.Writer) (result string, state map[string]any, err error) {
	wasRunning := svc.IsRunning(name)

	if err := svc.Start(name); err != nil {
		return "", nil, err
	}
	_, _ = fmt.Fprintf(output, "started service %s\n", name) //nolint:errcheck // status output to writer
	return name, map[string]any{
		"name":        name,
		"was_running": wasRunning,
	}, nil
}

// CompensateStart undoes a Start by stopping the service if it wasn't
// running before.
func (p *Provider) CompensateStart(svc op.ServiceManagerProvider, state any) error {
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
	return svc.Stop(name)
}

// Stop stops a service. Returns compensation state with pre-action
// running status.
//
// Parameters:
//   - name: Service name (e.g., launchd label, systemd unit, Windows service)
func (p *Provider) Stop(svc op.ServiceManagerProvider, name string, output io.Writer) (result string, state map[string]any, err error) {
	wasRunning := svc.IsRunning(name)

	if err := svc.Stop(name); err != nil {
		return "", nil, err
	}
	_, _ = fmt.Fprintf(output, "stopped service %s\n", name) //nolint:errcheck // status output to writer
	return name, map[string]any{
		"name":        name,
		"was_running": wasRunning,
	}, nil
}

// CompensateStop undoes a Stop by starting the service if it was
// running before.
func (p *Provider) CompensateStop(svc op.ServiceManagerProvider, state any) error {
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
	return svc.Start(name)
}

// ── Predicates ───────────────────────────────────────────────────────

// Enabled returns true if the named service is enabled to start at boot.
//
// Parameters:
//   - name: Service name to check
func (p *Provider) Enabled(svc op.ServiceManagerProvider, name string) (bool, error) {
	return svc.IsEnabled(name), nil
}

// Exists returns true if the named service exists on the system.
//
// Parameters:
//   - name: Service name to check
func (p *Provider) Exists(svc op.ServiceManagerProvider, name string) (bool, error) {
	return svc.Exists(name), nil
}

// Running returns true if the named service is currently running.
//
// Parameters:
//   - name: Service name to check
func (p *Provider) Running(svc op.ServiceManagerProvider, name string) (bool, error) {
	return svc.IsRunning(name), nil
}
