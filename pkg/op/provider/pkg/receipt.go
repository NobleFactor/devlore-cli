// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package pkg

import (
	"encoding/json"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Receipt holds package-specific compensation state for [Provider.Install], [Provider.Remove], and [Provider.Upgrade]
// calls.
//
// The embedded [op.ReceiptBase] carries the affected [Resource] and the opaque [op.ReceiptBase.TransactionID] minted
// at [op.ReceiptBase.Commit] time. Packages, Manager, Cask, AlreadyInstalled, and PreviousVersions record the
// per-call state needed to reverse the operation: the package list (the receipt models a multi-package action so the
// list cannot be derived from the single embedded resource), the manager that performed the action, whether it
// targeted casks, the subset that was already installed before the call (so unwind does not remove packages the user
// already had), and the prior versions for upgrades.
type Receipt struct {
	op.ReceiptBase
	Packages         []string
	Manager          string
	Cask             bool
	AlreadyInstalled []string
	PreviousVersions map[string]string
}

// MarshalJSON encodes the receipt as JSON: the base envelope (action, resource_uri, transaction_id) extended with
// the package-specific fields.
//
// Delegates to [Receipt.MarshalYAML] for the wire-shape value, then runs [json.Marshal] over it.
//
// Returns:
//   - []byte: JSON-encoded object.
//   - error: any error from [Receipt.MarshalYAML] or [json.Marshal].
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
//   - any: the populated anonymous struct for the YAML encoder to walk.
//   - error: nil under normal conditions.
func (r *Receipt) MarshalYAML() (any, error) {

	base := r.Snapshot()

	return struct {
		Action           string            `json:"action"            yaml:"action"`
		ResourceURI      string            `json:"resource_uri"      yaml:"resource_uri"`
		TransactionID    string            `json:"transaction_id"    yaml:"transaction_id"`
		Packages         []string          `json:"packages"          yaml:"packages"`
		Manager          string            `json:"manager"           yaml:"manager"`
		Cask             bool              `json:"cask"              yaml:"cask"`
		AlreadyInstalled []string          `json:"already_installed" yaml:"already_installed"`
		PreviousVersions map[string]string `json:"previous_versions" yaml:"previous_versions"`
	}{
		Action:           base.Action,
		ResourceURI:      base.ResourceURI,
		TransactionID:    base.TransactionID,
		Packages:         r.Packages,
		Manager:          r.Manager,
		Cask:             r.Cask,
		AlreadyInstalled: r.AlreadyInstalled,
		PreviousVersions: r.PreviousVersions,
	}, nil
}

// UnmarshalJSON decodes a JSON document produced by [Receipt.MarshalJSON] back into the receiver.
//
// The receiver MUST be pre-seeded with an [op.RuntimeEnvironment]-bearing zero [Resource] so the unmarshaler can
// rehydrate the wire URI via [NewResource] when the wire carries a non-empty resource_uri.
//
// Parameters:
//   - data: the JSON-encoded receipt bytes.
//
// Returns:
//   - error: any decode error, an unwrappable URI, a malformed UUID, or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalJSON(data []byte) error {

	var aux struct {
		Action           string            `json:"action"`
		ResourceURI      string            `json:"resource_uri"`
		TransactionID    string            `json:"transaction_id"`
		Packages         []string          `json:"packages"`
		Manager          string            `json:"manager"`
		Cask             bool              `json:"cask"`
		AlreadyInstalled []string          `json:"already_installed"`
		PreviousVersions map[string]string `json:"previous_versions"`
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("pkg.Receipt: unmarshal JSON: %w", err)
	}

	return r.hydrate(aux.Action, aux.ResourceURI, aux.TransactionID, aux.Packages, aux.Manager, aux.Cask, aux.AlreadyInstalled, aux.PreviousVersions)
}

// UnmarshalYAML decodes a YAML node produced by [Receipt.MarshalYAML] back into the receiver via
// [op.ReceiptBase.Restore].
//
// The receiver MUST be pre-seeded with an [op.RuntimeEnvironment]-bearing zero [Resource]; see
// [Receipt.UnmarshalJSON] for the contract.
//
// Parameters:
//   - unmarshal: the YAML library's decode-into callback.
//
// Returns:
//   - error: any decode error, [NewResource] error, or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalYAML(unmarshal func(any) error) error {

	var aux struct {
		Action           string            `yaml:"action"`
		ResourceURI      string            `yaml:"resource_uri"`
		TransactionID    string            `yaml:"transaction_id"`
		Packages         []string          `yaml:"packages"`
		Manager          string            `yaml:"manager"`
		Cask             bool              `yaml:"cask"`
		AlreadyInstalled []string          `yaml:"already_installed"`
		PreviousVersions map[string]string `yaml:"previous_versions"`
	}

	if err := unmarshal(&aux); err != nil {
		return fmt.Errorf("pkg.Receipt: unmarshal YAML: %w", err)
	}

	return r.hydrate(aux.Action, aux.ResourceURI, aux.TransactionID, aux.Packages, aux.Manager, aux.Cask, aux.AlreadyInstalled, aux.PreviousVersions)
}

// hydrate reconstructs the receiver's embedded [op.ReceiptBase] from the decoded base envelope. The
// [Resource] is pulled from the [op.ResourceCatalog] on the pre-seeded [op.RuntimeEnvironment] when the
// envelope carries a non-empty resource_uri — existing entries are re-used (Resource identity is
// URI-interned); URIs not yet in the catalog are constructed via [NewResource] and registered through
// [op.ResourceCatalog.GetOrCreate]. An empty resource_uri leaves the rehydrated base with no resource
// (the receipt records a multi-package action with no single anchoring resource). The base is re-seated
// via [op.NewReceiptBase], the wire-primitive triplet is handed to Restore, and the package-specific
// fields are assigned.
//
// Parameters:
//   - action: the canonical action name from the decoded envelope.
//   - resourceURI: the resource's URI string from the decoded envelope; empty when the receipt has no
//     anchoring resource.
//   - transactionID: the canonical UUIDv7 string from the decoded envelope.
//   - packages, manager, cask, alreadyInstalled, previousVersions: package-specific fields from the envelope.
//
// Returns:
//   - error: a missing-context error, a missing-catalog error, a [NewResource] error, or an
//     [op.ReceiptBase.Restore] failure.
func (r *Receipt) hydrate(action, resourceURI, transactionID string, packages []string, manager string, cask bool, alreadyInstalled []string, previousVersions map[string]string) error {

	existing := r.Resource()
	if existing == nil || existing.RuntimeEnvironment() == nil {
		return fmt.Errorf("pkg.Receipt: unmarshal requires RuntimeEnvironment on receiver")
	}

	ctx := existing.RuntimeEnvironment()
	if ctx.Catalog == nil {
		return fmt.Errorf("pkg.Receipt: unmarshal requires Catalog on RuntimeEnvironment")
	}

	var resource op.Resource
	if resourceURI != "" {
		// DiscoverResource handles construction + Catalog.Discover internally; no wrapping factory needed.
		got, err := DiscoverResource(&op.ActivationRecord{Runtime: ctx}, resourceURI)
		if err != nil {
			return fmt.Errorf("pkg.Receipt: rehydrate resource %q: %w", resourceURI, err)
		}
		resource = got
	}

	r.ReceiptBase = op.NewReceiptBase(resource)

	if err := r.Restore(struct {
		Action        string `json:"action"         yaml:"action"`
		ResourceURI   string `json:"resource_uri"   yaml:"resource_uri"`
		TransactionID string `json:"transaction_id" yaml:"transaction_id"`
	}{
		Action:        action,
		ResourceURI:   resourceURI,
		TransactionID: transactionID,
	}); err != nil {
		return fmt.Errorf("pkg.Receipt: restore: %w", err)
	}

	r.Packages = packages
	r.Manager = manager
	r.Cask = cask
	r.AlreadyInstalled = alreadyInstalled
	r.PreviousVersions = previousVersions

	return nil
}
