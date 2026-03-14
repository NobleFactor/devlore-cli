// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package service provides platform-agnostic service management actions.
package service

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides platform-agnostic service management.
// Platform-specific behavior is delegated to p.Context().Platform.ServiceManager.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

func NewProvider(ctx op.Context) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

func (p *Provider) serviceManager() (op.ServiceManager, error) {
	plat := p.Context().Platform
	if plat == nil || plat.ServiceManager == nil {
		return nil, fmt.Errorf("no service manager available")
	}
	return plat.ServiceManager, nil
}

// ── Compensable Pairs ────────────────────────────────────────────────

// Disable disables a service from starting at boot.
//
// Parameters:
//   - name: service resource identifying the service
func (p *Provider) Disable(name Resource) (Resource, Tombstone, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return Resource{}, Tombstone{}, err
	}

	wasEnabled := sm.IsEnabled(name.Name)

	r := sm.Disable(name.Name)
	if !r.OK {
		return Resource{}, Tombstone{}, fmt.Errorf("disable %s failed: %s", name.Name, r.Stderr)
	}
	_, _ = fmt.Fprintf(p.Context().Writer, "disabled service %s\n", name.Name) //nolint:errcheck // status output
	return name, Tombstone{Name: name.Name, WasEnabled: wasEnabled}, nil
}

// CompensateDisable undoes a Disable by enabling the service if it was
// enabled before.
func (p *Provider) CompensateDisable(state Tombstone) error {
	if state.Name == "" || !state.WasEnabled {
		return nil
	}
	sm, err := p.serviceManager()
	if err != nil {
		return err
	}
	r := sm.Enable(state.Name)
	if !r.OK {
		return fmt.Errorf("compensate disable (enable %s) failed: %s", state.Name, r.Stderr)
	}
	return nil
}

// Enable enables a service to start at boot.
//
// Parameters:
//   - name: service resource identifying the service
func (p *Provider) Enable(name Resource) (Resource, Tombstone, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return Resource{}, Tombstone{}, err
	}

	wasEnabled := sm.IsEnabled(name.Name)

	r := sm.Enable(name.Name)
	if !r.OK {
		return Resource{}, Tombstone{}, fmt.Errorf("enable %s failed: %s", name.Name, r.Stderr)
	}
	_, _ = fmt.Fprintf(p.Context().Writer, "enabled service %s\n", name.Name) //nolint:errcheck // status output
	return name, Tombstone{Name: name.Name, WasEnabled: wasEnabled}, nil
}

// CompensateEnable undoes an Enable by disabling the service if it
// wasn't enabled before.
func (p *Provider) CompensateEnable(state Tombstone) error {
	if state.Name == "" || state.WasEnabled {
		return nil
	}
	sm, err := p.serviceManager()
	if err != nil {
		return err
	}
	r := sm.Disable(state.Name)
	if !r.OK {
		return fmt.Errorf("compensate enable (disable %s) failed: %s", state.Name, r.Stderr)
	}
	return nil
}

// Restart restarts a service. Compensation is a no-op — if the service
// was restarted, it was already running.
//
// Parameters:
//   - name: service resource identifying the service
func (p *Provider) Restart(name Resource) (Resource, Tombstone, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return Resource{}, Tombstone{}, err
	}

	r := sm.Stop(name.Name)
	if !r.OK {
		return Resource{}, Tombstone{}, fmt.Errorf("stop before restart: %s", r.Stderr)
	}
	r = sm.Start(name.Name)
	if !r.OK {
		return Resource{}, Tombstone{}, fmt.Errorf("start after restart: %s", r.Stderr)
	}
	_, _ = fmt.Fprintf(p.Context().Writer, "restarted service %s\n", name.Name) //nolint:errcheck // status output
	return name, Tombstone{Name: name.Name}, nil
}

// CompensateRestart is a no-op. A restarted service was already running.
func (p *Provider) CompensateRestart(_ Tombstone) error {
	return nil
}

// Start starts a service.
//
// Parameters:
//   - name: service resource identifying the service
func (p *Provider) Start(name Resource) (Resource, Tombstone, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return Resource{}, Tombstone{}, err
	}

	wasRunning := sm.IsRunning(name.Name)

	r := sm.Start(name.Name)
	if !r.OK {
		return Resource{}, Tombstone{}, fmt.Errorf("start %s failed: %s", name.Name, r.Stderr)
	}
	_, _ = fmt.Fprintf(p.Context().Writer, "started service %s\n", name.Name) //nolint:errcheck // status output
	return name, Tombstone{Name: name.Name, WasRunning: wasRunning}, nil
}

// CompensateStart undoes a Start by stopping the service if it wasn't
// running before.
func (p *Provider) CompensateStart(state Tombstone) error {
	if state.Name == "" || state.WasRunning {
		return nil
	}
	sm, err := p.serviceManager()
	if err != nil {
		return err
	}
	r := sm.Stop(state.Name)
	if !r.OK {
		return fmt.Errorf("compensate start (stop %s) failed: %s", state.Name, r.Stderr)
	}
	return nil
}

// Stop stops a service.
//
// Parameters:
//   - name: service resource identifying the service
func (p *Provider) Stop(name Resource) (Resource, Tombstone, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return Resource{}, Tombstone{}, err
	}

	wasRunning := sm.IsRunning(name.Name)

	r := sm.Stop(name.Name)
	if !r.OK {
		return Resource{}, Tombstone{}, fmt.Errorf("stop %s failed: %s", name.Name, r.Stderr)
	}
	_, _ = fmt.Fprintf(p.Context().Writer, "stopped service %s\n", name.Name) //nolint:errcheck // status output
	return name, Tombstone{Name: name.Name, WasRunning: wasRunning}, nil
}

// CompensateStop undoes a Stop by starting the service if it was
// running before.
func (p *Provider) CompensateStop(state Tombstone) error {
	if state.Name == "" || !state.WasRunning {
		return nil
	}
	sm, err := p.serviceManager()
	if err != nil {
		return err
	}
	r := sm.Start(state.Name)
	if !r.OK {
		return fmt.Errorf("compensate stop (start %s) failed: %s", state.Name, r.Stderr)
	}
	return nil
}

// ── Predicates ───────────────────────────────────────────────────────

// Enabled returns true if the named service is enabled to start at boot.
//
// Parameters:
//   - name: service resource to check
func (p *Provider) Enabled(name Resource) (bool, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return false, err
	}
	return sm.IsEnabled(name.Name), nil
}

// Exists returns true if the named service exists on the system.
//
// Parameters:
//   - name: service resource to check
func (p *Provider) Exists(name Resource) (bool, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return false, err
	}
	return sm.Exists(name.Name), nil
}

// Running returns true if the named service is currently running.
//
// Parameters:
//   - name: service resource to check
func (p *Provider) Running(name Resource) (bool, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return false, err
	}
	return sm.IsRunning(name.Name), nil
}
