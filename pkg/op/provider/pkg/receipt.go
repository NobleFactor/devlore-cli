// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"encoding/json"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// MutationKind identifies the package mutation a [Receipt] records, so [Provider.CompensatePackageMutation] can invert
// it: remove a newly-installed package, restore a pre-existing one's prior version, reinstall a removed package, or
// best-effort restore an upgraded package's prior version.
type MutationKind string

const (
	// MutationInstall records an install; its undo removes a newly-installed package, or restores a pre-existing one's
	// prior version when the install drifted it.
	MutationInstall MutationKind = "install"

	// MutationRemove records a removal; its undo reinstalls a package that was present before.
	MutationRemove MutationKind = "remove"

	// MutationUpgrade records an upgrade; its undo best-effort restores the package's prior version.
	MutationUpgrade MutationKind = "upgrade"
)

// compensatePackageMutationAction is the dotted compensating-action name every pkg.Receipt declares at construction.
//
// [Provider.CompensatePackageMutation] inverts any package mutation by dispatching on the receipt's [MutationKind];
// the name matches the registry's compensating-action-index key (provider name + snake method name), so a package
// receipt routes to that one compensating action regardless of which verb or dispatcher created it.
const compensatePackageMutationAction = "pkg.compensate_package_mutation"

// Receipt holds the per-package compensation state [Provider.CompensatePackageMutation] needs to undo one package
// mutation.
//
// The embedded [op.ReceiptBase] carries the affected [Resource] and the opaque [op.ReceiptBase.TransactionID] minted at
// [op.ReceiptBase.Commit]. One Receipt records one package: `kind` is the mutation it undoes, `Manager` is the purl type
// of the leaf that handled it, `InstalledBefore` records whether the package was present before the action (so unwind
// does not remove a package the user already had), and `PreviousVersion` is the version observed before the action (so
// an upgrade or a drifted install can be best-effort restored). A multi-package verb pushes one Receipt per package onto
// a [op.RecoveryStack].
type Receipt struct {
	op.ReceiptBase

	// kind records which package mutation produced this receipt, so compensation inverts the right one.
	kind MutationKind

	// Manager is the purl type of the leaf that handled the package.
	Manager string

	// InstalledBefore records whether the package was present before the action.
	InstalledBefore bool

	// PreviousVersion is the version observed before the action.
	PreviousVersion string
}

// NewReceipt builds a per-package [*Receipt] that declares its undo at construction.
//
// The receipt names [compensatePackageMutationAction] as its compensating action (so
// [Provider.CompensatePackageMutation] inverts it regardless of which verb or dispatcher created it) and records the
// mutation kind and the package fields. The transactionID is minted later at [op.ReceiptBase.Commit].
//
// Parameters:
//   - `resource`: the package [*Resource] the mutation affected.
//   - `kind`: the [MutationKind] the receipt records.
//   - `manager`: the purl type of the leaf that handled the package.
//   - `installedBefore`: whether the package was present before the action.
//   - `previousVersion`: the version observed before the action.
//
// Returns:
//   - `*Receipt`: the constructed receipt, born naming its compensator.
func NewReceipt(resource *Resource, kind MutationKind, manager string, installedBefore bool, previousVersion string) *Receipt {
	return &Receipt{
		ReceiptBase:     op.NewReceiptBaseWithCompensator(resource, compensatePackageMutationAction),
		kind:            kind,
		Manager:         manager,
		InstalledBefore: installedBefore,
		PreviousVersion: previousVersion,
	}
}

// region EXPORTED METHODS

// region State management

// Kind returns the [MutationKind] this receipt records, or "" when unset.
//
// Returns:
//   - `MutationKind`: the recorded mutation kind.
func (r *Receipt) Kind() MutationKind {
	return r.kind
}

// endregion

// region Behaviors

// MarshalJSON encodes the receipt's compensation state as JSON.
//
// Delegates to [Receipt.MarshalYAML] for the serialized-shape value, then runs [json.Marshal] over it.
//
// Returns:
//   - `[]byte`: JSON-encoded object carrying the receipt's resource URI and package fields.
//   - `error`: any error from [Receipt.MarshalYAML] or [json.Marshal].
func (r *Receipt) MarshalJSON() ([]byte, error) {

	v, err := r.MarshalYAML()
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// MarshalYAML returns the receipt's compensation state as an anonymous struct value the encoder serializes.
//
// This is the `receipt` sub-field the recovery stack embeds: the resource URI, transaction id, mutation kind, and the
// package fields. The base execution state (`action`/`compensating_action`/`result`/`status`) rides the stack-owned
// envelope, so it is not repeated here; resume reconstructs the receipt in [Receipt.RestoreEncoded].
//
// Returns:
//   - `any`: the populated anonymous struct for the encoder to walk.
//   - `error`: nil under normal conditions.
func (r *Receipt) MarshalYAML() (any, error) {

	// The base owns resource_uri (pkg resolves its resource by URI via DiscoverResource) and transaction_id; the
	// compensator is not serialized — the recovery tree nests it structurally (phase-8 step 42 slice 3b).
	base := r.Snapshot()
	base.Compensator = nil

	return struct {
		op.ReceiptData  `yaml:",inline"`
		Kind            string `json:"kind,omitempty"   yaml:"kind,omitempty"`
		Manager         string `json:"manager"          yaml:"manager"`
		InstalledBefore bool   `json:"installed_before" yaml:"installed_before"`
		PreviousVersion string `json:"previous_version" yaml:"previous_version"`
	}{
		ReceiptData:     base,
		Kind:            string(r.kind),
		Manager:         r.Manager,
		InstalledBefore: r.InstalledBefore,
		PreviousVersion: r.PreviousVersion,
	}, nil
}

// RestoreEncoded reconstructs the receipt from its codec-decoded envelope.
//
// It is the [op.Receipt.RestoreEncoded] override for package receipts. The recovery stack already decoded the envelope,
// so this consumes decoded values, never bytes: `base` carries the execution state and `fields` the receipt sub-field.
// It rehydrates the resource from its URI via [DiscoverResource], seeds the base via [op.NewReceiptBase] +
// [op.ReceiptBase.Restore], and restores the kind and package fields.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment; its catalog must hold (or be able to construct) the resource.
//   - `base`: the codec-decoded base execution state.
//   - `fields`: the receipt sub-field, decoded to a format-neutral map.
//
// Returns:
//   - `error`: a missing catalog or a [DiscoverResource] failure (an external-system interaction). The envelope
//     itself arrives post-[op.LoadTrace] — checksum-verified — so document-derived failures panic
//     (docs/architecture/5-graph-trace-integrity.md).
func (r *Receipt) RestoreEncoded(
	runtimeEnvironment *op.RuntimeEnvironment, base op.ReceiptData, fields map[string]any,
) error {

	if runtimeEnvironment == nil || runtimeEnvironment.ResourceCatalog == nil {
		return fmt.Errorf("pkg.Receipt: RestoreEncoded requires a runtime environment with a catalog")
	}

	resourceURI := stringField(fields, "resource_uri")

	var resource op.Resource
	if resourceURI != "" {
		got, err := DiscoverResource(runtimeEnvironment, resourceURI)
		if err != nil {
			return fmt.Errorf("pkg.Receipt: RestoreEncoded resource %q: %w", resourceURI, err)
		}
		resource = got
	}

	r.ReceiptBase = op.NewReceiptBase(resource)
	assert.NoError("pkg.Receipt: restore base", r.Restore(op.ReceiptData{
		ForwardAction:      base.ForwardAction,
		CompensatingAction: base.CompensatingAction,
		UnitID:             base.UnitID,
		Result:             base.Result,
		ResultType:         base.ResultType,
		Status:             base.Status,
		CompensationError:  base.CompensationError,
		ResourceURI:        resourceURI,
		TransactionID:      stringField(fields, "transaction_id"),
	}))

	r.kind = MutationKind(stringField(fields, "kind"))
	r.Manager = stringField(fields, "manager")
	r.InstalledBefore = boolField(fields, "installed_before")
	r.PreviousVersion = stringField(fields, "previous_version")

	return nil
}

// endregion

// endregion

// region HELPER FUNCTIONS

// stringField returns the string value at `key` in a decoded receipt sub-field, or "" when absent or not a string.
//
// Parameters:
//   - `fields`: the decoded receipt sub-field.
//   - `key`: the field name to read.
//
// Returns:
//   - `string`: the value, or "" when absent. A present non-string panics via [assert.Type] — the envelope is
//     checksum-verified by [op.LoadTrace], so a mistype is a serialization bug
//     (docs/architecture/5-graph-trace-integrity.md § The Checksum Trust Boundary).
func stringField(fields map[string]any, key string) string {

	raw, ok := fields[key]
	if !ok {
		return ""
	}

	return assert.Type[string]("receipt field "+key, raw)
}

// boolField returns the bool value at `key` in a decoded receipt sub-field, or false when absent or not a bool.
//
// Parameters:
//   - `fields`: the decoded receipt sub-field.
//   - `key`: the field name to read.
//
// Returns:
//   - `bool`: the value, or false when absent. A present non-bool panics via [assert.Type] — the envelope is
//     checksum-verified by [op.LoadTrace], so a mistype is a serialization bug
//     (docs/architecture/5-graph-trace-integrity.md § The Checksum Trust Boundary).
func boolField(fields map[string]any, key string) bool {

	raw, ok := fields[key]
	if !ok {
		return false
	}

	return assert.Type[bool]("receipt field "+key, raw)
}

// endregion
