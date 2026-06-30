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
//   - `runtimeEnvironment`: the runtime environment that supplies the platform abstraction and status sink.
//
// Returns:
//   - `*Provider`: the initialized provider.
func NewProvider(runtimeEnvironment *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(runtimeEnvironment)}
}

// region EXPORTED METHODS

// region Behaviors

// Compensable actions

// Install installs each package via the platform's Composite router.
//
// Parameters:
//   - `packages`: package resources to install, each carrying its requested version.
//   - `kwargs`: opaque native-installer flags passed through to the routed leaf (e.g. `cask`).
//
// Returns:
//   - `result`: the input packages, each with Type set to the purl type of the leaf that handled it.
//   - `stack`: a [op.RecoveryStack] carrying one self-describing [*Receipt] per package, in input order, so a failed
//     run unwinds it in reverse — each receipt routes to [Provider.CompensatePackageMutation].
//   - `error`: non-nil if no packages were specified, no platform is available, or any package failed to install.
func (p *Provider) Install(
	packages []*Resource,
	kwargs map[string]any,
) (result []*Resource, stack *op.RecoveryStack, err error) {

	plat, err := p.verbPlatform(packages)
	if err != nil {
		return nil, nil, err
	}

	receipts, routerErr := plat.PackageManager().Install(toPURLs(plat, packages), kwargs)

	if result, stack, err = p.buildStack(packages, receipts, MutationInstall); err != nil {
		return result, stack, err
	}

	return result, stack, routerErr
}

// CompensateInstall reverses an install by unwinding its recovery stack.
//
// Each entry is a self-describing [*Receipt] naming [Provider.CompensatePackageMutation], so unwinding removes each
// newly-installed package and restores any pre-existing one whose version the install drifted.
//
// Parameters:
//   - `stack`: the recovery stack [Provider.Install] returned as its complement; a nil stack returns nil.
//
// Returns:
//   - `error`: the joined errors from the per-package compensations, or nil when all succeed.
func (p *Provider) CompensateInstall(stack *op.RecoveryStack) error {

	if stack == nil {
		return nil
	}
	return stack.Unwind()
}

// Remove removes each package via the platform's Composite router.
//
// Parameters:
//   - `packages`: package resources to remove.
//   - `kwargs`: opaque native-installer flags passed through to the routed leaf.
//
// Returns:
//   - `result`: the input packages, each with Type set to the purl type of the leaf that handled it.
//   - `stack`: a [op.RecoveryStack] carrying one self-describing [*Receipt] per package, in input order.
//   - `error`: non-nil if no packages were specified, no platform is available, or any package failed to remove.
func (p *Provider) Remove(
	packages []*Resource,
	kwargs map[string]any,
) (result []*Resource, stack *op.RecoveryStack, err error) {

	plat, err := p.verbPlatform(packages)
	if err != nil {
		return nil, nil, err
	}

	receipts, routerErr := plat.PackageManager().Remove(toPURLs(plat, packages), kwargs)

	if result, stack, err = p.buildStack(packages, receipts, MutationRemove); err != nil {
		return result, stack, err
	}

	return result, stack, routerErr
}

// CompensateRemove reverses a removal by unwinding its recovery stack — each entry reinstalls a package that was
// present before.
//
// Parameters:
//   - `stack`: the recovery stack [Provider.Remove] returned as its complement; a nil stack returns nil.
//
// Returns:
//   - `error`: the joined errors from the per-package compensations, or nil when all succeed.
func (p *Provider) CompensateRemove(stack *op.RecoveryStack) error {

	if stack == nil {
		return nil
	}
	return stack.Unwind()
}

// Upgrade upgrades each package to the latest available version via the platform's Composite router.
//
// Parameters:
//   - `packages`: package resources to upgrade.
//   - `kwargs`: opaque native-installer flags passed through to the routed leaf.
//
// Returns:
//   - `result`: the input packages, each with Type set to the purl type of the leaf that handled it.
//   - `stack`: a [op.RecoveryStack] carrying one self-describing [*Receipt] per package, in input order.
//   - `error`: non-nil if no packages were specified, no platform is available, or any package failed to upgrade.
func (p *Provider) Upgrade(
	packages []*Resource,
	kwargs map[string]any,
) (result []*Resource, stack *op.RecoveryStack, err error) {

	plat, err := p.verbPlatform(packages)
	if err != nil {
		return nil, nil, err
	}

	receipts, routerErr := plat.PackageManager().Upgrade(toPURLs(plat, packages), kwargs)

	if result, stack, err = p.buildStack(packages, receipts, MutationUpgrade); err != nil {
		return result, stack, err
	}

	return result, stack, routerErr
}

// CompensateUpgrade reverses an upgrade by unwinding its recovery stack — each entry best-effort restores its package's
// prior version.
//
// Parameters:
//   - `stack`: the recovery stack [Provider.Upgrade] returned as its complement; a nil stack returns nil.
//
// Returns:
//   - `error`: the joined errors from the per-package compensations, or nil when all succeed.
func (p *Provider) CompensateUpgrade(stack *op.RecoveryStack) error {

	if stack == nil {
		return nil
	}
	return stack.Unwind()
}

// CompensatePackageMutation inverts one package mutation, dispatching on the receipt's [MutationKind]: remove a
// newly-installed package or restore a pre-existing one's drifted version (install), reinstall a removed package
// (remove), or best-effort restore an upgraded package's prior version (upgrade). It is the single undo named by every
// package receipt; the verb companions ([Provider.CompensateInstall] / [Provider.CompensateRemove] /
// [Provider.CompensateUpgrade]) just unwind the stack of these.
//
// Parameters:
//   - `receipt`: the package [*Receipt] to invert; a nil receipt or nil resource is a no-op.
//
// Returns:
//   - `error`: a missing platform, an unknown kind, or any removal / reinstall failure.
func (p *Provider) CompensatePackageMutation(receipt *Receipt) error {

	if receipt == nil {
		return nil
	}

	resource, ok := receiptResource(receipt)
	if !ok {
		return nil
	}

	plat, err := p.platform()
	if err != nil {
		return err
	}

	router := plat.PackageManager()
	query := platform.PURL{Type: receipt.Manager, Name: resource.Name}
	restore := platform.PURL{Type: receipt.Manager, Name: resource.Name, Version: receipt.PreviousVersion}

	switch receipt.Kind() {

	case MutationInstall:
		// Newly installed → remove. Pre-existing whose version the install drifted → restore the prior version.
		if !receipt.InstalledBefore {
			_, removeErr := router.Remove([]platform.PURL{query}, nil)
			return removeErr
		}
		if receipt.PreviousVersion != "" && router.Version(query) != receipt.PreviousVersion {
			_, installErr := router.Install([]platform.PURL{restore}, nil)
			return installErr
		}
		return nil

	case MutationRemove:
		// Present before the removal → reinstall.
		if !receipt.InstalledBefore {
			return nil
		}
		_, installErr := router.Install([]platform.PURL{query}, nil)
		return installErr

	case MutationUpgrade:
		// Best-effort restore the prior version.
		if receipt.PreviousVersion == "" {
			return nil
		}
		_, installErr := router.Install([]platform.PURL{restore}, nil)
		return installErr

	default:
		return fmt.Errorf("compensate package mutation: unknown kind %q", receipt.Kind())
	}
}

// Fallible actions

// Installed reports whether the named package is installed, querying the router by purl.
//
// Parameters:
//   - `name`: the package resource to check.
//
// Returns:
//   - `bool`: true when the package is installed.
//   - `error`: non-nil when no platform is available.
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
//   - `name`: the package resource to check.
//
// Returns:
//   - `bool`: true when the package is not installed.
//   - `error`: non-nil when no platform is available.
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
//   - `resource`: the [*Resource] whose installed state to observe.
//
// Returns:
//   - `*Observation`: the constructed observation; never nil on a nil-error return.
//   - `error`: any [NewObservation] construction failure.
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
//   - `error`: aggregated per-leaf refresh failures, or non-nil when no platform is available.
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
//   - `name`: the package resource to check.
//   - `version`: the minimum version string to compare against.
//
// Returns:
//   - `bool`: true when the installed version is non-empty and >= `version`.
//   - `error`: non-nil when no platform is available.
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

// buildStack stamps the resolved purl type onto each input resource and builds a [op.RecoveryStack] of one
// self-describing [*Receipt] per package.
//
// Each receipt names [Provider.CompensatePackageMutation] as its undo (via [NewReceipt]) and is committed before it is
// pushed — the self-complement that Commit records is what makes it compensable at unwind. The verb supplies the
// [MutationKind] every package in the call shares. No activation record is needed: the receipt routes by its
// constructor-stamped compensator, not the dispatch action.
//
// Parameters:
//   - `packages`: the input resources, in order.
//   - `receipts`: the router's per-package receipts, in input order.
//   - `kind`: the [MutationKind] of the verb (install / remove / upgrade).
//
// Returns:
//   - `[]*Resource`: the input resources with Type set to the leaf's purl type.
//   - `*op.RecoveryStack`: the stack of committed per-package receipts, in input order.
//   - `error`: any receipt commit or push failure.
func (p *Provider) buildStack(
	packages []*Resource, receipts []platform.Receipt, kind MutationKind,
) ([]*Resource, *op.RecoveryStack, error) {

	result := make([]*Resource, len(packages))
	stack := op.NewRecoveryStack()
	runtimeEnvironment := p.RuntimeEnvironment()

	for i, resource := range packages {

		resolvedType := receipts[i].Purl.Type
		resource.Type = resolvedType
		result[i] = resource

		receipt := NewReceipt(resource, kind, resolvedType, receipts[i].PriorVersion != "", receipts[i].PriorVersion)

		if err := receipt.Commit(nil, resource, receipt, nil); err != nil {
			return result, stack, fmt.Errorf("pkg: commit receipt %q: %w", resource.Name, err)
		}

		if err := stack.Push(receipt, runtimeEnvironment); err != nil {
			return result, stack, fmt.Errorf("pkg: push receipt %q: %w", resource.Name, err)
		}
	}

	return result, stack, nil
}

// platform returns the runtime environment's [platform.Platform], or an error when none is configured.
//
// Returns:
//   - `platform.Platform`: the configured platform.
//   - `error`: non-nil when no platform is available.
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
//   - `packages`: the verb's package slice.
//
// Returns:
//   - `platform.Platform`: the configured platform.
//   - `error`: non-nil when the slice is empty or no platform is available.
func (p *Provider) verbPlatform(packages []*Resource) (platform.Platform, error) {

	if len(packages) == 0 {
		return nil, fmt.Errorf("no packages specified")
	}

	return p.platform()
}

// endregion

// endregion

// region HELPER FUNCTIONS

// receiptResource returns the [*Resource] a receipt anchors, reporting false for a nil receipt or a non-pkg resource.
//
// Parameters:
//   - `receipt`: the receipt to unwrap.
//
// Returns:
//   - `*Resource`: the anchoring resource.
//   - `bool`: true when the receipt is non-nil and anchors a [*Resource].
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
//   - `plat`: the target platform, for type resolution.
//   - `resource`: the resource to project.
//
// Returns:
//   - `platform.PURL`: the versionless query purl.
func toQueryPURL(plat platform.Platform, resource *Resource) platform.PURL {
	return platform.PURL{Type: resolveType(plat, resource.Type), Name: resource.Name}
}

// endregion
