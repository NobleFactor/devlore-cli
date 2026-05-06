// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// Provider provides platform-independent package management.
// Platform-specific behavior is delegated to p.RuntimeEnvironment().Platform.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

// NewProvider constructs a package-management Provider bound to the given runtime environment.
//
// Parameters:
//   - ctx: the runtime environment that supplies the platform abstraction and status sink.
//
// Returns:
//   - *Provider: the initialized provider.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

func (p *Provider) platform() (platform.Platform, error) {
	plat := p.RuntimeEnvironment().Platform
	if plat == nil {
		return nil, fmt.Errorf("no platform available")
	}
	return plat, nil
}

// packageNames extracts the ReceiverName field from each Resource.
func packageNames(resources []*Resource) []string {
	names := make([]string, len(resources))
	for i, r := range resources {
		names[i] = r.Name
	}
	return names
}

// --- Compensable Pairs ---

// Install installs packages using the platform's package manager.
//
// Parameters:
//   - packages: package resources to install
//   - manager: PkgPath manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
//
// Returns:
//   - result: input packages with Type set to the resolved package manager name
//   - state: compensation tombstone recording the requested packages, manager, cask flag, and which packages were
//     already installed before the action
//   - error: non-nil if no packages were specified, no package manager is available, or the underlying "install"
//     command fails
func (p *Provider) Install(packages []*Resource, manager string, cask bool) (result []*Resource, state *Receipt, err error) {

	if len(packages) == 0 {
		return nil, nil, fmt.Errorf("no packages specified")
	}

	plat, err := p.platform()

	if err != nil {
		return nil, nil, err
	}

	packageManager := resolvePlatformManagerForInstall(plat, manager)
	names := packageNames(packages)

	if packageManager == nil {
		return nil, nil, fmt.Errorf("no package manager available")
	}

	// Query which packages are already installed before acting.

	var alreadyInstalled []string

	for _, packageName := range names {
		if packageManager.Installed(packageName) {
			alreadyInstalled = append(alreadyInstalled, packageName)
		}
	}

	if cask {
		if err := runBrewCask("install", names...); err != nil {
			return nil, nil, err
		}
	} else {
		r := packageManager.Install(names...)
		if !r.OK {
			return nil, nil, fmt.Errorf("%s install failed: %s", packageManager.Name(), r.Stderr)
		}
	}

	result = make([]*Resource, len(packages))
	resolvedType := packageManager.Name()

	for i, pkg := range packages {
		result[i] = pkg
		result[i].Type = resolvedType
	}

	return result, &Receipt{
		Packages:         names,
		Manager:          manager,
		Cask:             cask,
		AlreadyInstalled: alreadyInstalled,
	}, nil
}

// CompensateInstall undoes an installation by removing packages that weren't already installed before the action.
func (p *Provider) CompensateInstall(state *Receipt) error {

	if state == nil || len(state.Packages) == 0 {
		return nil
	}

	installed := make(map[string]bool)

	for _, packageName := range state.AlreadyInstalled {
		installed[packageName] = true
	}

	var toRemove []string

	for _, packageName := range state.Packages {
		if !installed[packageName] {
			toRemove = append(toRemove, packageName)
		}
	}

	if len(toRemove) == 0 {
		return nil
	}

	if state.Cask {
		for _, packageName := range toRemove {
			if err := runBrewCask("uninstall", packageName); err != nil {
				return err
			}
		}
		return nil
	}

	plat, err := p.platform()

	if err != nil {
		return err
	}

	packageManager := resolvePlatformManagerForInstall(plat, state.Manager)

	if packageManager == nil {
		return fmt.Errorf("no package manager available for compensation")
	}

	for _, packageName := range toRemove {
		r := packageManager.Remove(packageName)
		if !r.OK {
			return fmt.Errorf("%s remove %s failed: %s", packageManager.Name(), packageName, r.Stderr)
		}
	}

	return nil
}

// Remove removes packages using the platform's package manager.
//
// Parameters:
//   - packages: package resources to remove
//   - manager: PkgPath manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
func (p *Provider) Remove(packages []*Resource, manager string, cask bool) (result []*Resource, state *Receipt, err error) {

	if len(packages) == 0 {
		return nil, nil, fmt.Errorf("no packages specified")
	}

	plat, err := p.platform()

	if err != nil {
		return nil, nil, err
	}

	names := packageNames(packages)

	for _, packageName := range names {
		if cask {
			if err := runBrewCask("uninstall", packageName); err != nil {
				return nil, nil, err
			}
		} else {
			packageManager := resolvePlatformManagerForRemove(plat, manager, packageName)
			if packageManager == nil {
				return nil, nil, fmt.Errorf("no package manager available")
			}
			r := packageManager.Remove(packageName)
			if !r.OK {
				return nil, nil, fmt.Errorf("%s remove %s failed: %s", packageManager.Name(), packageName, r.Stderr)
			}
		}
	}

	resolvedType := manager

	if resolvedType == "" {
		if cask {
			resolvedType = "brew"
		} else if pm := plat.DefaultPackageManager(); pm != nil {
			resolvedType = pm.Name()
		}
	}

	result = make([]*Resource, len(packages))

	for i, pkg := range packages {
		result[i] = pkg
		result[i].Type = resolvedType
	}

	return result, &Receipt{
		Packages: names,
		Manager:  manager,
		Cask:     cask,
	}, nil
}

// CompensateRemove undoes a Remove by reinstalling the removed packages.
func (p *Provider) CompensateRemove(state *Receipt) error {

	if state == nil || len(state.Packages) == 0 {
		return nil
	}

	if state.Cask {
		return runBrewCask("install", state.Packages...)
	}

	plat, err := p.platform()

	if err != nil {
		return err
	}

	packageManager := resolvePlatformManagerForInstall(plat, state.Manager)

	if packageManager == nil {
		return fmt.Errorf("no package manager available for compensation")
	}

	r := packageManager.Install(state.Packages...)

	if !r.OK {
		return fmt.Errorf("%s install failed: %s", packageManager.Name(), r.Stderr)
	}

	return nil
}

// Upgrade upgrades packages using the platform's package manager.
// Returns compensation state with pre-upgrade versions per package.
//
// Parameters:
//   - packages: package resources to upgrade
//   - manager: PkgPath manager override (empty for auto-detect)
//   - cask: If true, use Homebrew cask for macOS GUI apps
func (p *Provider) Upgrade(packages []*Resource, manager string, cask bool) (result []*Resource, state *Receipt, err error) {

	if len(packages) == 0 {
		return nil, nil, fmt.Errorf("no packages specified")
	}

	plat, err := p.platform()

	if err != nil {
		return nil, nil, err
	}

	names := packageNames(packages)
	packageManager := resolvePlatformManagerForUpgrade(plat, manager, names)

	if packageManager == nil {
		return nil, nil, fmt.Errorf("no package manager available")
	}

	// Capture current versions before upgrading.

	previousVersions := make(map[string]string)

	for _, packageName := range names {
		if v := packageManager.Version(packageName); v != "" {
			previousVersions[packageName] = v
		}
	}

	if cask {
		if err := runBrewCask("upgrade", names...); err != nil {
			return nil, nil, err
		}
	} else {
		r := packageManager.Install(names...)
		if !r.OK {
			return nil, nil, fmt.Errorf("%s upgrade failed: %s", packageManager.Name(), r.Stderr)
		}
	}

	result = make([]*Resource, len(packages))
	resolvedType := packageManager.Name()

	for i, pkg := range packages {
		result[i] = pkg
		result[i].Type = resolvedType
	}

	return result, &Receipt{
		Packages:         names,
		Manager:          manager,
		Cask:             cask,
		PreviousVersions: previousVersions,
	}, nil
}

// CompensateUpgrade is a diagnostic no-op. Previous versions are captured
// in state for manual recovery, but automatic downgrade is not reliable
// across package managers.
func (p *Provider) CompensateUpgrade(_ *Receipt) error {
	return nil
}

// --- Standalone Methods ---

// Update refreshes the package manager index.
//
// Parameters:
//   - manager: PkgPath manager override (empty for auto-detect)
func (p *Provider) Update(manager string) (string, error) {

	plat, err := p.platform()

	if err != nil {
		return "", err
	}

	packageManager := resolvePlatformManagerForInstall(plat, manager)

	if packageManager == nil {
		return "", fmt.Errorf("no package manager available")
	}

	r := packageManager.Update()

	if !r.OK {
		return "", fmt.Errorf("%s update failed: %s", packageManager.Name(), r.Stderr)
	}

	return packageManager.Name(), nil
}

// --- Predicates ---

// Installed returns true if the named package is installed.
//
// Parameters:
//   - name: package resource to check
func (p *Provider) Installed(name *Resource) (bool, error) {

	plat, err := p.platform()

	if err != nil {
		return false, err
	}

	pm := plat.DefaultPackageManager()
	if pm == nil {
		return false, fmt.Errorf("no package manager available")
	}

	return pm.Installed(name.Name), nil
}

// NotInstalled returns true if the named package is not installed.
//
// Parameters:
//   - name: package resource to check
func (p *Provider) NotInstalled(name *Resource) (bool, error) {

	plat, err := p.platform()

	if err != nil {
		return false, err
	}

	pm := plat.DefaultPackageManager()
	if pm == nil {
		return false, fmt.Errorf("no package manager available")
	}

	return !pm.Installed(name.Name), nil
}

// VersionGTE returns true if the installed version of name is >= version.
//
// Parameters:
//   - name: package resource to check
//   - version: Minimum version string to compare against
func (p *Provider) VersionGTE(name *Resource, version string) (bool, error) {

	plat, err := p.platform()

	if err != nil {
		return false, err
	}

	pm := plat.DefaultPackageManager()
	if pm == nil {
		return false, fmt.Errorf("no package manager available")
	}

	current := pm.Version(name.Name)

	if current == "" {
		return false, nil
	}

	return current >= version, nil
}
