// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/internal/host"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// NewHostProvider wraps a host.Host in the op.HostProvider interface.
// Returns nil if h is nil.
func NewHostProvider(h host.Host) op.HostProvider {
	if h == nil {
		return nil
	}
	return &hostAdapter{h: h}
}

// hostAdapter adapts host.Host to op.HostProvider.
type hostAdapter struct {
	h host.Host
}

func (a *hostAdapter) PackageManager() op.PackageManagerProvider {
	pm := a.h.PackageManager()
	if pm == nil {
		return nil
	}
	return &pmAdapter{pm: pm}
}

func (a *hostAdapter) InstalledBy(name string) op.PackageManagerProvider {
	pm := a.h.InstalledBy(name)
	if pm == nil {
		return nil
	}
	return &pmAdapter{pm: pm}
}

func (a *hostAdapter) AllInstalledBy(name string) []op.PackageManagerProvider {
	pms := a.h.AllInstalledBy(name)
	if len(pms) == 0 {
		return nil
	}
	result := make([]op.PackageManagerProvider, len(pms))
	for i, pm := range pms {
		result[i] = &pmAdapter{pm: pm}
	}
	return result
}

func (a *hostAdapter) GetPackageManager(name string) op.PackageManagerProvider {
	pm := a.h.GetPackageManager(name)
	if pm == nil {
		return nil
	}
	return &pmAdapter{pm: pm}
}

func (a *hostAdapter) ServiceManager() op.ServiceManagerProvider {
	sm := a.h.ServiceManager()
	if sm == nil {
		return nil
	}
	return &smAdapter{sm: sm}
}

// pmAdapter adapts host.PackageManager to op.PackageManagerProvider.
type pmAdapter struct {
	pm host.PackageManager
}

func (a *pmAdapter) Name() string               { return a.pm.Name() }
func (a *pmAdapter) Installed(name string) bool { return a.pm.Installed(name) }
func (a *pmAdapter) Version(name string) string { return a.pm.Version(name) }
func (a *pmAdapter) Available(name string) bool { return a.pm.Available(name) }
func (a *pmAdapter) NeedsSudo() bool            { return a.pm.NeedsSudo() }

func (a *pmAdapter) Install(packages ...string) error {
	r := a.pm.Install(packages...)
	if !r.OK {
		return fmt.Errorf("%s install failed: %s", a.pm.Name(), r.Stderr)
	}
	return nil
}

func (a *pmAdapter) Remove(name string) error {
	r := a.pm.Remove(name)
	if !r.OK {
		return fmt.Errorf("%s remove %s failed: %s", a.pm.Name(), name, r.Stderr)
	}
	return nil
}

func (a *pmAdapter) Update() error {
	r := a.pm.Update()
	if !r.OK {
		return fmt.Errorf("%s update failed: %s", a.pm.Name(), r.Stderr)
	}
	return nil
}

// smAdapter adapts host.ServiceManager to op.ServiceManagerProvider.
type smAdapter struct {
	sm host.ServiceManager
}

func (a *smAdapter) Exists(name string) bool    { return a.sm.Exists(name) }
func (a *smAdapter) IsRunning(name string) bool { return a.sm.IsRunning(name) }
func (a *smAdapter) IsEnabled(name string) bool { return a.sm.IsEnabled(name) }

func (a *smAdapter) Start(name string) error {
	r := a.sm.Start(name)
	if !r.OK {
		return fmt.Errorf("start %s failed: %s", name, r.Stderr)
	}
	return nil
}

func (a *smAdapter) Stop(name string) error {
	r := a.sm.Stop(name)
	if !r.OK {
		return fmt.Errorf("stop %s failed: %s", name, r.Stderr)
	}
	return nil
}

func (a *smAdapter) Enable(name string) error {
	r := a.sm.Enable(name)
	if !r.OK {
		return fmt.Errorf("enable %s failed: %s", name, r.Stderr)
	}
	return nil
}

func (a *smAdapter) Disable(name string) error {
	r := a.sm.Disable(name)
	if !r.OK {
		return fmt.Errorf("disable %s failed: %s", name, r.Stderr)
	}
	return nil
}
