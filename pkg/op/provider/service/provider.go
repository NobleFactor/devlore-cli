// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package service provides platform-agnostic service management actions.
package service

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides platform-agnostic service management.
// Platform-specific behavior is delegated to p.ExecutionContext().Platform.ServiceManager.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

func NewProvider(ctx *op.ExecutionContext) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

func (p *Provider) serviceManager() (op.ServiceManager, error) {
	plat := p.ExecutionContext().Platform
	if plat == nil || plat.ServiceManager == nil {
		return nil, fmt.Errorf("no service manager available")
	}
	return plat.ServiceManager, nil
}

// resourceName returns the service name carried by the receipt's affected [Resource], or empty when the receipt
// has no resource attached. Compensation methods use this to bail early on zero receipts and to look up the service
// in the platform's manager.
func resourceName(receipt *Receipt) string {
	r, ok := receipt.Resource().(*Resource)
	if !ok || r == nil {
		return ""
	}
	return r.Name
}

// --- Compensable Pairs ---

// Disable disables a service from starting at boot.
//
// Parameters:
//   - name: service resource identifying the service
func (p *Provider) Disable(name *Resource) (*Resource, *Receipt, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return nil, nil, err
	}

	wasEnabled := sm.IsEnabled(name.Name)

	r := sm.Disable(name.Name)
	if !r.OK {
		return nil, nil, fmt.Errorf("disable %s failed: %s", name.Name, r.Stderr)
	}
	_, _ = fmt.Fprintf(p.ExecutionContext().Writer, "disabled service %s\n", name.Name) //nolint:errcheck // status output
	return name, &Receipt{ReceiptBase: op.NewReceiptBase(name), WasEnabled: wasEnabled}, nil
}

// CompensateDisable undoes a Disable by enabling the service if it was
// enabled before.
func (p *Provider) CompensateDisable(receipt *Receipt) error {
	name := resourceName(receipt)
	if name == "" || !receipt.WasEnabled {
		return nil
	}
	sm, err := p.serviceManager()
	if err != nil {
		return err
	}
	r := sm.Enable(name)
	if !r.OK {
		return fmt.Errorf("compensate disable (enable %s) failed: %s", name, r.Stderr)
	}
	return nil
}

// Enable enables a service to start at boot.
//
// Parameters:
//   - name: service resource identifying the service
func (p *Provider) Enable(name *Resource) (*Resource, *Receipt, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return nil, nil, err
	}

	wasEnabled := sm.IsEnabled(name.Name)

	r := sm.Enable(name.Name)
	if !r.OK {
		return nil, nil, fmt.Errorf("enable %s failed: %s", name.Name, r.Stderr)
	}
	_, _ = fmt.Fprintf(p.ExecutionContext().Writer, "enabled service %s\n", name.Name) //nolint:errcheck // status output
	return name, &Receipt{ReceiptBase: op.NewReceiptBase(name), WasEnabled: wasEnabled}, nil
}

// CompensateEnable undoes an Enable by disabling the service if it
// wasn't enabled before.
func (p *Provider) CompensateEnable(receipt *Receipt) error {
	name := resourceName(receipt)
	if name == "" || receipt.WasEnabled {
		return nil
	}
	sm, err := p.serviceManager()
	if err != nil {
		return err
	}
	r := sm.Disable(name)
	if !r.OK {
		return fmt.Errorf("compensate enable (disable %s) failed: %s", name, r.Stderr)
	}
	return nil
}

// Restart restarts a service. Compensation is a no-op — if the service
// was restarted, it was already running.
//
// Parameters:
//   - name: service resource identifying the service
func (p *Provider) Restart(name *Resource) (*Resource, *Receipt, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return nil, nil, err
	}

	r := sm.Stop(name.Name)
	if !r.OK {
		return nil, nil, fmt.Errorf("stop before restart: %s", r.Stderr)
	}
	r = sm.Start(name.Name)
	if !r.OK {
		return nil, nil, fmt.Errorf("start after restart: %s", r.Stderr)
	}
	_, _ = fmt.Fprintf(p.ExecutionContext().Writer, "restarted service %s\n", name.Name) //nolint:errcheck // status output
	return name, &Receipt{ReceiptBase: op.NewReceiptBase(name)}, nil
}

// CompensateRestart is a no-op. A restarted service was already running.
func (p *Provider) CompensateRestart(_ *Receipt) error {
	return nil
}

// Start starts a service.
//
// Parameters:
//   - name: service resource identifying the service
func (p *Provider) Start(name *Resource) (*Resource, *Receipt, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return nil, nil, err
	}

	wasRunning := sm.IsRunning(name.Name)

	r := sm.Start(name.Name)
	if !r.OK {
		return nil, nil, fmt.Errorf("start %s failed: %s", name.Name, r.Stderr)
	}
	_, _ = fmt.Fprintf(p.ExecutionContext().Writer, "started service %s\n", name.Name) //nolint:errcheck // status output
	return name, &Receipt{ReceiptBase: op.NewReceiptBase(name), WasRunning: wasRunning}, nil
}

// CompensateStart undoes a Start by stopping the service if it wasn't running before.
func (p *Provider) CompensateStart(receipt *Receipt) error {
	name := resourceName(receipt)
	if name == "" || receipt.WasRunning {
		return nil
	}
	sm, err := p.serviceManager()
	if err != nil {
		return err
	}
	r := sm.Stop(name)
	if !r.OK {
		return fmt.Errorf("compensate start (stop %s) failed: %s", name, r.Stderr)
	}
	return nil
}

// Stop stops a service.
//
// Parameters:
//   - name: service resource identifying the service
func (p *Provider) Stop(name *Resource) (*Resource, *Receipt, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return nil, nil, err
	}

	wasRunning := sm.IsRunning(name.Name)

	r := sm.Stop(name.Name)
	if !r.OK {
		return nil, nil, fmt.Errorf("stop %s failed: %s", name.Name, r.Stderr)
	}
	_, _ = fmt.Fprintf(p.ExecutionContext().Writer, "stopped service %s\n", name.Name) //nolint:errcheck // status output
	return name, &Receipt{ReceiptBase: op.NewReceiptBase(name), WasRunning: wasRunning}, nil
}

// CompensateStop undoes a Stop by starting the service if it was
// running before.
func (p *Provider) CompensateStop(receipt *Receipt) error {
	name := resourceName(receipt)
	if name == "" || !receipt.WasRunning {
		return nil
	}
	sm, err := p.serviceManager()
	if err != nil {
		return err
	}
	r := sm.Start(name)
	if !r.OK {
		return fmt.Errorf("compensate stop (start %s) failed: %s", name, r.Stderr)
	}
	return nil
}

// --- Predicates ---

// Enabled returns true if the named service is enabled to start at boot.
//
// Parameters:
//   - name: service resource to check
func (p *Provider) Enabled(name *Resource) (bool, error) {
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
func (p *Provider) Exists(name *Resource) (bool, error) {
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
func (p *Provider) Running(name *Resource) (bool, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return false, err
	}
	return sm.IsRunning(name.Name), nil
}