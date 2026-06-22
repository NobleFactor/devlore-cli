// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package service provides platform-agnostic service management actions.
package service

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// Provider provides platform-agnostic service management.
//
// Platform-specific behavior is delegated to p.RuntimeEnvironment().Platform.ServiceManager.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a service provider bound to the given runtime environment.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// region EXPORTED METHODS

// region Behaviors

// Compensable actions

// Disable disables a service from starting at boot.
//
// Parameters:
//   - `name`: service resource identifying the service.
//
// Returns:
//   - `*Resource`: the service resource (identity unchanged).
//   - `*Receipt`: compensation state recording whether the service was enabled before.
//   - `error`: non-nil when no service manager is available or the disable command fails.
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
	p.RuntimeEnvironment().Status.Succeed(fmt.Sprintf("disabled service %s", name.Name))
	return name, &Receipt{ReceiptBase: op.NewReceiptBase(name), WasEnabled: wasEnabled}, nil
}

// CompensateDisable undoes a Disable by re-enabling the service if it was enabled before.
//
// Parameters:
//   - `receipt`: the [*Receipt] from [Provider.Disable]; a no-op when serviceless or not enabled.
//
// Returns:
//   - `error`: non-nil when no service manager is available or the enable command fails.
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
//   - `name`: service resource identifying the service.
//
// Returns:
//   - `*Resource`: the service resource (identity unchanged).
//   - `*Receipt`: compensation state recording whether the service was enabled before.
//   - `error`: non-nil when no service manager is available or the enable command fails.
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
	p.RuntimeEnvironment().Status.Succeed(fmt.Sprintf("enabled service %s", name.Name))
	return name, &Receipt{ReceiptBase: op.NewReceiptBase(name), WasEnabled: wasEnabled}, nil
}

// CompensateEnable undoes an Enable by disabling the service if it wasn't enabled before.
//
// Parameters:
//   - `receipt`: the [*Receipt] from [Provider.Enable]; a no-op when serviceless or already enabled.
//
// Returns:
//   - `error`: non-nil when no service manager is available or the disable command fails.
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

// Restart restarts a service by stopping then starting it.
//
// Parameters:
//   - `name`: service resource identifying the service.
//
// Returns:
//   - `*Resource`: the service resource (identity unchanged).
//   - `*Receipt`: compensation state; [Provider.CompensateRestart] is a no-op (the service was already running).
//   - `error`: non-nil when no service manager is available or the stop/start commands fail.
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
	p.RuntimeEnvironment().Status.Succeed(fmt.Sprintf("restarted service %s", name.Name))
	return name, &Receipt{ReceiptBase: op.NewReceiptBase(name)}, nil
}

// CompensateRestart is a no-op. A restarted service was already running.
//
// Parameters:
//   - `receipt`: the [*Receipt] from [Provider.Restart]; ignored.
//
// Returns:
//   - `error`: always nil.
func (p *Provider) CompensateRestart(_ *Receipt) error {
	return nil
}

// Start starts a service.
//
// Parameters:
//   - `name`: service resource identifying the service.
//
// Returns:
//   - `*Resource`: the service resource (identity unchanged).
//   - `*Receipt`: compensation state recording whether the service was running before.
//   - `error`: non-nil when no service manager is available or the start command fails.
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
	p.RuntimeEnvironment().Status.Succeed(fmt.Sprintf("started service %s", name.Name))
	return name, &Receipt{ReceiptBase: op.NewReceiptBase(name), WasRunning: wasRunning}, nil
}

// CompensateStart undoes a Start by stopping the service if it wasn't running before.
//
// Parameters:
//   - `receipt`: the [*Receipt] from [Provider.Start]; a no-op when serviceless or already running.
//
// Returns:
//   - `error`: non-nil when no service manager is available or the stop command fails.
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
//   - `name`: service resource identifying the service.
//
// Returns:
//   - `*Resource`: the service resource (identity unchanged).
//   - `*Receipt`: compensation state recording whether the service was running before.
//   - `error`: non-nil when no service manager is available or the stop command fails.
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
	p.RuntimeEnvironment().Status.Succeed(fmt.Sprintf("stopped service %s", name.Name))
	return name, &Receipt{ReceiptBase: op.NewReceiptBase(name), WasRunning: wasRunning}, nil
}

// CompensateStop undoes a Stop by starting the service if it was running before.
//
// Parameters:
//   - `receipt`: the [*Receipt] from [Provider.Stop]; a no-op when serviceless or not running.
//
// Returns:
//   - `error`: non-nil when no service manager is available or the start command fails.
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

// Fallible actions

// Enabled returns true if the named service is enabled to start at boot.
//
// Parameters:
//   - `name`: service resource to check.
//
// Returns:
//   - `bool`: true when the service is enabled.
//   - `error`: non-nil when no service manager is available.
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
//   - `name`: service resource to check.
//
// Returns:
//   - `bool`: true when the service exists.
//   - `error`: non-nil when no service manager is available.
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
//   - `name`: service resource to check.
//
// Returns:
//   - `bool`: true when the service is running.
//   - `error`: non-nil when no service manager is available.
func (p *Provider) Running(name *Resource) (bool, error) {
	sm, err := p.serviceManager()
	if err != nil {
		return false, err
	}
	return sm.IsRunning(name.Name), nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// serviceManager returns the platform's [platform.ServiceManager], or an error when none is configured.
//
// Returns:
//   - `platform.ServiceManager`: the active service manager.
//   - `error`: non-nil when the runtime environment has no platform or no service manager.
func (p *Provider) serviceManager() (platform.ServiceManager, error) {
	plat := p.RuntimeEnvironment().Platform
	if plat == nil {
		return nil, fmt.Errorf("no service manager available")
	}
	sm := plat.ServiceManager()
	if sm == nil {
		return nil, fmt.Errorf("no service manager available")
	}
	return sm, nil
}

// endregion

// endregion

// region HELPER FUNCTIONS

// resourceName returns the service name carried by the receipt's affected [Resource], or empty when the receipt
// has no resource attached.
//
// Compensation methods use this to bail early on zero receipts and to look up the service in the platform's manager.
//
// Parameters:
//   - `receipt`: the receipt whose affected resource carries the service name.
//
// Returns:
//   - `string`: the service name, or "" when the receipt has no [*Resource].
func resourceName(receipt *Receipt) string {
	r, ok := receipt.Resource().(*Resource)
	if !ok || r == nil {
		return ""
	}
	return r.Name
}

// endregion
