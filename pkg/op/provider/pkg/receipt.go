// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"encoding/json"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Receipt holds per-package compensation state for [Provider.Install], [Provider.Remove], and [Provider.Upgrade].
//
// The embedded [op.ReceiptBase] carries the affected [Resource] and the opaque [op.ReceiptBase.TransactionID] minted
// at [op.ReceiptBase.Commit] time. One Receipt records one package's operation: `Manager` is the purl type of the
// leaf that handled it, `InstalledBefore` records whether the package was present before the action (so unwind does
// not remove a package the user already had), and `PreviousVersion` is the version observed before the action (so an
// upgrade can be best-effort restored). A multi-package verb returns a `[]*Receipt`, one per package in input order.
type Receipt struct {
	op.ReceiptBase

	// Manager is the purl type of the leaf that handled the package.
	Manager string

	// InstalledBefore records whether the package was present before the action.
	InstalledBefore bool

	// PreviousVersion is the version observed before the action.
	PreviousVersion string
}

// region EXPORTED METHODS

// region Behaviors

// MarshalJSON encodes the receipt as JSON: the base envelope (action, resource_uri, transaction_id) extended with
// the package-specific fields.
//
// Delegates to [Receipt.MarshalYAML] for the serialized-shape value, then runs [json.Marshal] over it.
//
// Returns:
//   - `[]byte`: JSON-encoded object.
//   - `error`: any error from [Receipt.MarshalYAML] or [json.Marshal].
func (r *Receipt) MarshalJSON() ([]byte, error) {

	v, err := r.MarshalYAML()
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// MarshalYAML returns the receipt's full state as an anonymous struct value the YAML encoder serializes.
//
// Returns:
//   - `any`: the populated anonymous struct for the YAML encoder to walk.
//   - `error`: nil under normal conditions.
func (r *Receipt) MarshalYAML() (any, error) {

	base := r.Snapshot()

	return struct {
		Action          string `json:"action"           yaml:"action"`
		ResourceURI     string `json:"resource_uri"     yaml:"resource_uri"`
		TransactionID   string `json:"transaction_id"   yaml:"transaction_id"`
		Manager         string `json:"manager"          yaml:"manager"`
		InstalledBefore bool   `json:"installed_before" yaml:"installed_before"`
		PreviousVersion string `json:"previous_version" yaml:"previous_version"`
	}{
		Action:          base.Action,
		ResourceURI:     base.ResourceURI,
		TransactionID:   base.TransactionID,
		Manager:         r.Manager,
		InstalledBefore: r.InstalledBefore,
		PreviousVersion: r.PreviousVersion,
	}, nil
}

// UnmarshalJSON decodes a JSON document produced by [Receipt.MarshalJSON] back into the receiver.
//
// The receiver MUST be pre-seeded with an [op.RuntimeEnvironment]-bearing zero [Resource] so the unmarshaler can
// rehydrate the serialized URI via [DiscoverResource] when the payload carries a non-empty resource_uri.
//
// Parameters:
//   - `data`: the JSON-encoded receipt bytes.
//
// Returns:
//   - `error`: any decode error, an unwrappable URI, a malformed UUID, or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalJSON(data []byte) error {

	var aux struct {
		Action          string `json:"action"`
		ResourceURI     string `json:"resource_uri"`
		TransactionID   string `json:"transaction_id"`
		Manager         string `json:"manager"`
		InstalledBefore bool   `json:"installed_before"`
		PreviousVersion string `json:"previous_version"`
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("pkg.Receipt: unmarshal JSON: %w", err)
	}

	return r.hydrate(aux.Action, aux.ResourceURI, aux.TransactionID, aux.Manager, aux.InstalledBefore, aux.PreviousVersion)
}

// UnmarshalYAML decodes a YAML node produced by [Receipt.MarshalYAML] back into the receiver via
// [op.ReceiptBase.Restore].
//
// The receiver MUST be pre-seeded with an [op.RuntimeEnvironment]-bearing zero [Resource]; see
// [Receipt.UnmarshalJSON] for the contract.
//
// Parameters:
//   - `unmarshal`: the YAML library's decode-into callback.
//
// Returns:
//   - `error`: any decode error, [DiscoverResource] error, or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalYAML(unmarshal func(any) error) error {

	var aux struct {
		Action          string `yaml:"action"`
		ResourceURI     string `yaml:"resource_uri"`
		TransactionID   string `yaml:"transaction_id"`
		Manager         string `yaml:"manager"`
		InstalledBefore bool   `yaml:"installed_before"`
		PreviousVersion string `yaml:"previous_version"`
	}

	if err := unmarshal(&aux); err != nil {
		return fmt.Errorf("pkg.Receipt: unmarshal YAML: %w", err)
	}

	return r.hydrate(aux.Action, aux.ResourceURI, aux.TransactionID, aux.Manager, aux.InstalledBefore, aux.PreviousVersion)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// hydrate reconstructs the receiver's embedded [op.ReceiptBase] from the decoded base envelope.
//
// The [Resource] is pulled from the [op.ResourceCatalog] on the pre-seeded [op.RuntimeEnvironment] when the
// envelope carries a non-empty resource_uri — existing entries are re-used (Resource identity is URI-interned);
// URIs not yet in the catalog are constructed and registered via [DiscoverResource]. An empty resource_uri leaves
// the rehydrated base with no resource. The base is re-seated via [op.NewReceiptBase], the serialized-primitive triplet
// is handed to Restore, and the package-specific fields are assigned.
//
// Parameters:
//   - `action`: the canonical action name from the decoded envelope.
//   - `resourceURI`: the resource's URI string from the decoded envelope; empty when the receipt has no anchoring
//     resource.
//   - `transactionID`: the canonical UUIDv7 string from the decoded envelope.
//   - `manager`, `installedBefore`, `previousVersion`: package-specific fields from the envelope.
//
// Returns:
//   - `error`: a missing-context error, a missing-catalog error, a [DiscoverResource] error, or an
//     [op.ReceiptBase.Restore] failure.
func (r *Receipt) hydrate(
	action, resourceURI, transactionID, manager string,
	installedBefore bool,
	previousVersion string,
) error {

	existing := r.Resource()
	if existing == nil || existing.RuntimeEnvironment() == nil {
		return fmt.Errorf("pkg.Receipt: unmarshal requires RuntimeEnvironment on receiver")
	}

	runtimeEnvironment := existing.RuntimeEnvironment()
	if runtimeEnvironment.ResourceCatalog == nil {
		return fmt.Errorf("pkg.Receipt: unmarshal requires Catalog on RuntimeEnvironment")
	}

	var resource op.Resource
	if resourceURI != "" {
		// DiscoverResource handles construction + Catalog.Discover internally; no wrapping factory needed.
		got, err := DiscoverResource(runtimeEnvironment, resourceURI)
		if err != nil {
			return fmt.Errorf("pkg.Receipt: rehydrate resource %q: %w", resourceURI, err)
		}
		resource = got
	}

	r.ReceiptBase = op.NewReceiptBase(resource)

	if err := r.Restore(op.ReceiptData{
		Action:        action,
		ResourceURI:   resourceURI,
		TransactionID: transactionID,
	}); err != nil {
		return fmt.Errorf("pkg.Receipt: restore: %w", err)
	}

	r.Manager = manager
	r.InstalledBefore = installedBefore
	r.PreviousVersion = previousVersion

	return nil
}

// endregion

// endregion
