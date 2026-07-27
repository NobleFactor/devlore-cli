// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package service

import (
	"encoding/json"
	"fmt"

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

// RestoreEncoded reconstructs the receipt from its codec-decoded envelope, resolving its service [Resource] against the
// rehydrated catalog.
//
// It is the [op.Receipt.RestoreEncoded] override the recovery stack drives at re-arm (via [op.reconstructReceipt]) —
// the env is threaded in explicitly as a parameter, not read off the receiver, so the stack path (which loads a bare
// receipt before the catalog is rehydrated) can reconstruct it. The service [Resource] is resolved from
// `base.ResourceURI` (its "svc:" scheme stripped) via [DiscoverResource]; the base is re-seated via [op.NewReceiptBase]
// so [op.ReceiptBase.Restore]'s URI-match check has a live resource, then Restore writes the full base and the
// service-specific `was_running` / `was_enabled` flags are read from `fields`.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment; its catalog must hold (or be able to construct) the resource.
//   - `base`: the codec-decoded base execution state; its `ResourceURI` ("svc:<name>") names the resource.
//   - `fields`: the receipt's whole decoded object; `was_running` / `was_enabled` are read from it.
//
// Returns:
//   - `error`: a missing catalog, a [DiscoverResource] failure, or an [op.ReceiptBase.Restore] failure.
func (r *Receipt) RestoreEncoded(
	runtimeEnvironment *op.RuntimeEnvironment, base op.ReceiptData, fields map[string]any,
) error {

	if runtimeEnvironment == nil || runtimeEnvironment.ResourceCatalog == nil {
		return fmt.Errorf("service.Receipt: RestoreEncoded requires a runtime environment with a catalog")
	}

	// The service resource lives in the rehydrated catalog; resolve it from its URI through the catalog's URI->id
	// namespace (a Resource.URI() is a canonical tag URI, not a DiscoverResource input).
	catalog := runtimeEnvironment.ResourceCatalog
	got, ok := catalog.Lookup(catalog.Current(base.ResourceURI))
	if !ok {
		return fmt.Errorf("service.Receipt: RestoreEncoded: resource %q not in catalog", base.ResourceURI)
	}
	resource, ok := got.(*Resource)
	if !ok {
		return fmt.Errorf("service.Receipt: RestoreEncoded: catalog entry for %q is %T, want *service.Resource",
			base.ResourceURI, got)
	}

	r.ReceiptBase = op.NewReceiptBase(resource)

	if err := r.Restore(base); err != nil {
		return fmt.Errorf("service.Receipt: RestoreEncoded restore: %w", err)
	}

	if raw, ok := fields["was_running"]; ok {
		value, ok := raw.(bool)
		if !ok {
			return fmt.Errorf("service.Receipt: RestoreEncoded field %q: expected bool, got %T", "was_running", raw)
		}
		r.WasRunning = value
	}

	if raw, ok := fields["was_enabled"]; ok {
		value, ok := raw.(bool)
		if !ok {
			return fmt.Errorf("service.Receipt: RestoreEncoded field %q: expected bool, got %T", "was_enabled", raw)
		}
		r.WasEnabled = value
	}

	return nil
}

// endregion

// endregion
