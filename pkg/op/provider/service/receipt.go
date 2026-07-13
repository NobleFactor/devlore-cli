// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Receipt holds service-specific compensation state for [Provider.Start], [Provider.Stop], [Provider.Enable], and
// [Provider.Disable] calls.
//
// The embedded [op.ReceiptBase] carries the affected service [Resource] and the opaque [op.ReceiptBase.TransactionID]
// minted at [op.ReceiptBase.Commit] time. The service name is read through the resource — no per-receipt name field.
// WasRunning and WasEnabled record the pre-call running and enabled flags so the corresponding Compensate methods
// can restore the service to its prior state.
type Receipt struct {
	op.ReceiptBase

	// WasRunning records whether the service was running before the action.
	WasRunning bool

	// WasEnabled records whether the service was enabled before the action.
	WasEnabled bool
}

// region EXPORTED METHODS

// region Behaviors

// MarshalJSON encodes the receipt as JSON: the base envelope (action, resource_uri, transaction_id) extended with
// was_running and was_enabled.
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

	// The base owns forward_action, resource_uri (service resolves by URI via DiscoverResource), transaction_id, and the
	// execution state; the compensator is not serialized — the recovery tree nests it structurally (phase-8 step 42
	// slice 3b).
	base := r.Snapshot()
	base.Compensator = nil

	return struct {
		op.ReceiptData `yaml:",inline"`
		WasRunning     bool `json:"was_running" yaml:"was_running"`
		WasEnabled     bool `json:"was_enabled" yaml:"was_enabled"`
	}{
		ReceiptData: base,
		WasRunning:  r.WasRunning,
		WasEnabled:  r.WasEnabled,
	}, nil
}

// UnmarshalJSON decodes a JSON document produced by [Receipt.MarshalJSON] back into the receiver via
// [op.ReceiptBase.Restore].
//
// The receiver MUST be pre-seeded with an [op.RuntimeEnvironment]-bearing zero [Resource] so the unmarshaler can
// rehydrate the encoded URI via [op.ResourceCatalog.GetOrCreate].
//
// Parameters:
//   - `data`: the JSON-encoded receipt bytes.
//
// Returns:
//   - `error`: any decode, [NewResource], or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalJSON(data []byte) error {

	var aux struct {
		op.ReceiptData
		WasRunning bool `json:"was_running"`
		WasEnabled bool `json:"was_enabled"`
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("service.Receipt: unmarshal JSON: %w", err)
	}

	return r.hydrate(aux.ReceiptData, aux.WasRunning, aux.WasEnabled)
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
//   - `error`: any decode, [NewResource], or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalYAML(unmarshal func(any) error) error {

	var aux struct {
		op.ReceiptData `yaml:",inline"`
		WasRunning     bool `yaml:"was_running"`
		WasEnabled     bool `yaml:"was_enabled"`
	}

	if err := unmarshal(&aux); err != nil {
		return fmt.Errorf("service.Receipt: unmarshal YAML: %w", err)
	}

	return r.hydrate(aux.ReceiptData, aux.WasRunning, aux.WasEnabled)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// hydrate reconstructs the receiver's embedded [op.ReceiptBase] from the decoded base envelope. The service
// [Resource] is pulled from the [op.ResourceCatalog] on the pre-seeded [op.RuntimeEnvironment] — existing
// entries are re-used (Resource identity is URI-interned); URIs not yet in the catalog are constructed via
// [NewResource] and registered through [op.ResourceCatalog.GetOrCreate]. The base is re-seated via
// [op.NewReceiptBase] so [op.ReceiptBase.Restore]'s URI-match check has a live resource to compare against,
// the serialized-primitive triplet is handed to Restore, and the service-specific fields are assigned.
//
// [NewResource] takes the bare service name; the "svc:" scheme is stripped from the encoded URI before the
// factory closure runs.
//
// Parameters:
//   - `base`: the decoded base execution state ([op.ReceiptData]); its `ResourceURI` (canonical "svc:<name>" form)
//     names the resource to rehydrate.
//   - `wasRunning`: the pre-call running flag from the decoded envelope.
//   - `wasEnabled`: the pre-call enabled flag from the decoded envelope.
//
// Returns:
//   - `error`: a missing-context error, a missing-catalog error, a [NewResource] error, or an
//     [op.ReceiptBase.Restore] failure.
func (r *Receipt) hydrate(base op.ReceiptData, wasRunning, wasEnabled bool) error {

	existing := r.Resource()
	if existing == nil || existing.RuntimeEnvironment() == nil {
		return fmt.Errorf("service.Receipt: unmarshal requires RuntimeEnvironment on receiver")
	}

	runtimeEnvironment := existing.RuntimeEnvironment()
	if runtimeEnvironment.ResourceCatalog == nil {
		return fmt.Errorf("service.Receipt: unmarshal requires Catalog on RuntimeEnvironment")
	}

	// DiscoverResource handles construction + Catalog.Discover internally; no wrapping factory needed.
	resource, err := DiscoverResource(runtimeEnvironment, strings.TrimPrefix(base.ResourceURI, "svc:"))
	if err != nil {
		return fmt.Errorf("service.Receipt: rehydrate resource %q: %w", base.ResourceURI, err)
	}

	r.ReceiptBase = op.NewReceiptBase(resource)

	if err := r.Restore(base); err != nil {
		return fmt.Errorf("service.Receipt: restore: %w", err)
	}

	r.WasRunning = wasRunning
	r.WasEnabled = wasEnabled

	return nil
}

// endregion

// endregion
