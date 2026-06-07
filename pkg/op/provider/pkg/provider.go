// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// Provider is a thin veneer over the platform's Composite package-manager router.
//
// It carries no convergence policy of its own: each verb projects its [*Resource] slice into a [platform.PURL]
// slice, calls the router once, and adapts the router's per-package [platform.Receipt] slice into the provider's
// [*Receipt] compensation state. All convergence and verification live in the platform's leaf drivers.
//
// +devlore:access=planned
type Provider struct {
	op.ProviderBase
}

// NewProvider constructs a package-management Provider bound to the given runtime environment.
//
// Parameters:
//   - runtimeEnvironment: the runtime environment that supplies the platform abstraction and status sink.
//
// Returns:
//   - *Provider: the initialized provider.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// region EXPORTED METHODS

// region Behaviors

// Compensable actions

// Install installs each package via the platform's Composite router.
//
// Parameters:
//   - packages: package resources to install, each carrying its requested version.
//   - kwargs: opaque native-installer flags passed through to the routed leaf (e.g. `cask`).
//
// Returns:
//   - result: the input packages, each with Type set to the purl type of the leaf that handled it.
//   - state: one per-package [*Receipt] recording the manager, pre-install presence, and prior version.
//   - error: non-nil if no packages were specified, no platform is available, or any package failed to install.
func (p *Provider) Install(packages []*Resource, kwargs map[string]any) (result []*Resource, state []*Receipt, err error) {

	plat, err := p.verbPlatform(packages)
	if err != nil {
		return nil, nil, err
	}

	receipts, routerErr := plat.PackageManager().Install(toPURLs(plat, packages), kwargs)

	result, state = p.adaptReceipts(packages, receipts)

	return result, state, routerErr
}

// CompensateInstall reverses an install: newly-installed packages are removed; pre-existing packages whose version
// drifted are reinstalled at their prior version.
//
// For each package that was not present before the action (InstalledBefore false), it is removed. For each package
// that was present but whose currently-installed version differs from the version observed before the action, it is
// reinstalled at PreviousVersion (best-effort; cross-manager downgrade is unreliable). Pre-existing packages whose
// version is unchanged are left in place.
//
// Parameters:
//   - state: the per-package receipts produced by [Provider.Install].
//
// Returns:
//   - error: non-nil when a platform is missing, a removal fails, or a version-restore install fails.
func (p *Provider) CompensateInstall(state []*Receipt) error {

	if len(state) == 0 {
		return nil
	}

	plat, err := p.platform()
	if err != nil {
		return err
	}

	router := plat.PackageManager()

	var (
		toRemove  []platform.PURL
		toRestore []platform.PURL
	)

	for _, receipt := range state {

		resource, ok := receiptResource(receipt)
		if !ok {
			continue
		}

		if !receipt.InstalledBefore {
			toRemove = append(toRemove, platform.PURL{Type: receipt.Manager, Name: resource.Name})
			continue
		}

		// Pre-existing: restore only when the install drifted its version away from what was observed before.
		query := platform.PURL{Type: receipt.Manager, Name: resource.Name}
		if receipt.PreviousVersion != "" && router.Version(query) != receipt.PreviousVersion {
			toRestore = append(toRestore, platform.PURL{Type: receipt.Manager, Name: resource.Name, Version: receipt.PreviousVersion})
		}
	}

	if len(toRemove) > 0 {
		if _, removeErr := router.Remove(toRemove, nil); removeErr != nil {
			return removeErr
		}
	}

	if len(toRestore) > 0 {
		if _, installErr := router.Install(toRestore, nil); installErr != nil {
			return installErr
		}
	}

	return nil
}

// Remove removes each package via the platform's Composite router.
//
// Parameters:
//   - packages: package resources to remove.
//   - kwargs: opaque native-installer flags passed through to the routed leaf.
//
// Returns:
//   - result: the input packages, each with Type set to the purl type of the leaf that handled it.
//   - state: one per-package [*Receipt] recording the manager, prior presence, and prior version.
//   - error: non-nil if no packages were specified, no platform is available, or any package failed to remove.
func (p *Provider) Remove(packages []*Resource, kwargs map[string]any) (result []*Resource, state []*Receipt, err error) {

	plat, err := p.verbPlatform(packages)
	if err != nil {
		return nil, nil, err
	}

	receipts, routerErr := plat.PackageManager().Remove(toPURLs(plat, packages), kwargs)

	result, state = p.adaptReceipts(packages, receipts)

	return result, state, routerErr
}

// CompensateRemove reinstalls every package that was present before the removal, at its prior version.
//
// Parameters:
//   - state: the per-package receipts produced by [Provider.Remove].
//
// Returns:
//   - error: non-nil when a platform is missing or a reinstall fails.
func (p *Provider) CompensateRemove(state []*Receipt) error {

	toRestore := purlsToReverse(state, func(r *Receipt) bool { return r.InstalledBefore })
	if len(toRestore) == 0 {
		return nil
	}

	plat, err := p.platform()
	if err != nil {
		return err
	}

	_, installErr := plat.PackageManager().Install(toRestore, nil)

	return installErr
}

// Upgrade upgrades each package to the latest available version via the platform's Composite router.
//
// Parameters:
//   - packages: package resources to upgrade.
//   - kwargs: opaque native-installer flags passed through to the routed leaf.
//
// Returns:
//   - result: the input packages, each with Type set to the purl type of the leaf that handled it.
//   - state: one per-package [*Receipt] recording the manager, prior presence, and prior version.
//   - error: non-nil if no packages were specified, no platform is available, or any package failed to upgrade.
func (p *Provider) Upgrade(packages []*Resource, kwargs map[string]any) (result []*Resource, state []*Receipt, err error) {

	plat, err := p.verbPlatform(packages)
	if err != nil {
		return nil, nil, err
	}

	receipts, routerErr := plat.PackageManager().Upgrade(toPURLs(plat, packages), kwargs)

	result, state = p.adaptReceipts(packages, receipts)

	return result, state, routerErr
}

// CompensateUpgrade best-effort restores each upgraded package to its prior version.
//
// Cross-manager downgrade is unreliable (not every manager can pin an arbitrary prior version), so this is a
// best-effort install at the recorded prior version; failures are returned but the contract is diagnostic.
//
// Parameters:
//   - state: the per-package receipts produced by [Provider.Upgrade].
//
// Returns:
//   - error: non-nil when a platform is missing or a restore fails.
func (p *Provider) CompensateUpgrade(state []*Receipt) error {

	var toRestore []platform.PURL

	for _, receipt := range state {
		resource, ok := receiptResource(receipt)
		if !ok || receipt.PreviousVersion == "" {
			continue
		}
		toRestore = append(toRestore, platform.PURL{Type: receipt.Manager, Name: resource.Name, Version: receipt.PreviousVersion})
	}

	if len(toRestore) == 0 {
		return nil
	}

	plat, err := p.platform()
	if err != nil {
		return err
	}

	_, installErr := plat.PackageManager().Install(toRestore, nil)

	return installErr
}

// Fallible actions

// Installed reports whether the named package is installed, querying the router by purl.
//
// Parameters:
//   - name: the package resource to check.
//
// Returns:
//   - bool: true when the package is installed.
//   - error: non-nil when no platform is available.
func (p *Provider) Installed(name *Resource) (bool, error) {

	plat, err := p.platform()
	if err != nil {
		return false, err
	}

	return plat.PackageManager().Installed(toQueryPURL(plat, name)), nil
}

// NotInstalled reports whether the named package is not installed, querying the router by purl.
//
// Parameters:
//   - name: the package resource to check.
//
// Returns:
//   - bool: true when the package is not installed.
//   - error: non-nil when no platform is available.
func (p *Provider) NotInstalled(name *Resource) (bool, error) {

	plat, err := p.platform()
	if err != nil {
		return false, err
	}

	return !plat.PackageManager().Installed(toQueryPURL(plat, name)), nil
}

// Observe captures the runtime-observed state of `resource` as an [*Observation].
//
// Asks the platform's Composite router for the installed version of the package identified by `resource`. When a
// platform exists and the router reports a non-empty version, the Observation carries `Exists=true` and the version
// string; otherwise it carries `Exists=false`.
//
// Parameters:
//   - resource: the [*Resource] whose installed state to observe.
//
// Returns:
//   - *Observation: the constructed observation; never nil on a nil-error return.
//   - error: any [NewObservation] construction failure.
func (p *Provider) Observe(resource *Resource) (*Observation, error) {

	runtimeEnvironment := p.RuntimeEnvironment()

	if runtimeEnvironment == nil || runtimeEnvironment.Platform == nil {
		return NewObservation(runtimeEnvironment, resource, false, "")
	}

	version := runtimeEnvironment.Platform.PackageManager().Version(toQueryPURL(runtimeEnvironment.Platform, resource))
	if version == "" {
		return NewObservation(runtimeEnvironment, resource, false, "")
	}

	return NewObservation(runtimeEnvironment, resource, true, version)
}

// Update forces an immediate index refresh on every leaf via the platform's Composite router.
//
// Returns:
//   - error: aggregated per-leaf refresh failures, or non-nil when no platform is available.
func (p *Provider) Update() error {

	plat, err := p.platform()
	if err != nil {
		return err
	}

	return plat.PackageManager().Update()
}

// VersionGTE reports whether the installed version of `name` is greater than or equal to `version`.
//
// Parameters:
//   - name: the package resource to check.
//   - version: the minimum version string to compare against.
//
// Returns:
//   - bool: true when the installed version is non-empty and >= `version`.
//   - error: non-nil when no platform is available.
func (p *Provider) VersionGTE(name *Resource, version string) (bool, error) {

	plat, err := p.platform()
	if err != nil {
		return false, err
	}

	current := plat.PackageManager().Version(toQueryPURL(plat, name))
	if current == "" {
		return false, nil
	}

	return current >= version, nil
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// adaptReceipts pairs each input resource with its router receipt, stamping the resolved type onto the resource and
// projecting one [*Receipt] of compensation state per package.
//
// Parameters:
//   - packages: the input resources, in order.
//   - receipts: the router's per-package receipts, in input order.
//
// Returns:
//   - []*Resource: the input resources with Type set to the leaf's purl type.
//   - []*Receipt: one per-package receipt of compensation state.
func (p *Provider) adaptReceipts(packages []*Resource, receipts []platform.Receipt) ([]*Resource, []*Receipt) {

	result := make([]*Resource, len(packages))
	state := make([]*Receipt, len(packages))

	for i, resource := range packages {

		resolvedType := receipts[i].Purl.Type
		resource.Type = resolvedType
		result[i] = resource

		state[i] = &Receipt{
			ReceiptBase:     op.NewReceiptBase(resource),
			Manager:         resolvedType,
			InstalledBefore: receipts[i].PriorVersion != "",
			PreviousVersion: receipts[i].PriorVersion,
		}
	}

	return result, state
}

// platform returns the runtime environment's [platform.Platform], or an error when none is configured.
//
// Returns:
//   - platform.Platform: the configured platform.
//   - error: non-nil when no platform is available.
func (p *Provider) platform() (platform.Platform, error) {

	plat := p.RuntimeEnvironment().Platform
	if plat == nil {
		return nil, fmt.Errorf("no platform available")
	}

	return plat, nil
}

// verbPlatform validates a mutating verb's package slice and returns the platform.
//
// Parameters:
//   - packages: the verb's package slice.
//
// Returns:
//   - platform.Platform: the configured platform.
//   - error: non-nil when the slice is empty or no platform is available.
func (p *Provider) verbPlatform(packages []*Resource) (platform.Platform, error) {

	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages specified")
	}

	return p.platform()
}

// endregion

// endregion

// region HELPER FUNCTIONS

// purlsToReverse collects the versionless query purls of the receipts that `keep` selects, for compensation.
//
// Parameters:
//   - state: the per-package receipts to filter.
//   - keep: the predicate selecting which receipts contribute a purl.
//
// Returns:
//   - []platform.PURL: the selected purls; nil when none match.
func purlsToReverse(state []*Receipt, keep func(*Receipt) bool) []platform.PURL {

	var purls []platform.PURL

	for _, receipt := range state {
		resource, ok := receiptResource(receipt)
		if !ok || !keep(receipt) {
			continue
		}
		purls = append(purls, platform.PURL{Type: receipt.Manager, Name: resource.Name})
	}

	return purls
}

// receiptResource returns the [*Resource] a receipt anchors, reporting false for a nil receipt or a non-pkg resource.
//
// Parameters:
//   - receipt: the receipt to unwrap.
//
// Returns:
//   - *Resource: the anchoring resource.
//   - bool: true when the receipt is non-nil and anchors a [*Resource].
func receiptResource(receipt *Receipt) (*Resource, bool) {

	if receipt == nil {
		return nil, false
	}

	resource, ok := receipt.Resource().(*Resource)

	return resource, ok
}

// toQueryPURL projects a [*Resource] into a versionless [platform.PURL] for an installed-state query.
//
// Queries report a single package's observed state by identity, so the requested version is omitted.
//
// Parameters:
//   - plat: the target platform, for type resolution.
//   - resource: the resource to project.
//
// Returns:
//   - platform.PURL: the versionless query purl.
func toQueryPURL(plat platform.Platform, resource *Resource) platform.PURL {
	return platform.PURL{Type: resolveType(plat, resource.Type), Name: resource.Name}
}

// endregion
