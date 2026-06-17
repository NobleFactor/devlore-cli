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
	WasRunning bool
	WasEnabled bool
}

// MarshalJSON encodes the receipt as JSON: the base envelope (action, resource_uri, transaction_id) extended with
// was_running and was_enabled.
//
// Delegates to [Receipt.MarshalYAML] for the serialized-shape value, then runs [json.Marshal] over it.
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
		Action        string `json:"action"         yaml:"action"`
		ResourceURI   string `json:"resource_uri"   yaml:"resource_uri"`
		TransactionID string `json:"transaction_id" yaml:"transaction_id"`
		WasRunning    bool   `json:"was_running"    yaml:"was_running"`
		WasEnabled    bool   `json:"was_enabled"    yaml:"was_enabled"`
	}{
		Action:        base.Action,
		ResourceURI:   base.ResourceURI,
		TransactionID: base.TransactionID,
		WasRunning:    r.WasRunning,
		WasEnabled:    r.WasEnabled,
	}, nil
}

// UnmarshalJSON decodes a JSON document produced by [Receipt.MarshalJSON] back into the receiver via
// [op.ReceiptBase.Restore].
//
// The receiver MUST be pre-seeded with an [op.RuntimeEnvironment]-bearing zero [Resource] so the unmarshaler can
// rehydrate the encoded URI via [op.ResourceCatalog.GetOrCreate].
//
// Parameters:
//   - data: the JSON-encoded receipt bytes.
//
// Returns:
//   - error: any decode, [NewResource], or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalJSON(data []byte) error {

	var aux struct {
		Action        string `json:"action"`
		ResourceURI   string `json:"resource_uri"`
		TransactionID string `json:"transaction_id"`
		WasRunning    bool   `json:"was_running"`
		WasEnabled    bool   `json:"was_enabled"`
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("service.Receipt: unmarshal JSON: %w", err)
	}

	return r.hydrate(aux.Action, aux.ResourceURI, aux.TransactionID, aux.WasRunning, aux.WasEnabled)
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
//   - error: any decode, [NewResource], or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalYAML(unmarshal func(any) error) error {

	var aux struct {
		Action        string `yaml:"action"`
		ResourceURI   string `yaml:"resource_uri"`
		TransactionID string `yaml:"transaction_id"`
		WasRunning    bool   `yaml:"was_running"`
		WasEnabled    bool   `yaml:"was_enabled"`
	}

	if err := unmarshal(&aux); err != nil {
		return fmt.Errorf("service.Receipt: unmarshal YAML: %w", err)
	}

	return r.hydrate(aux.Action, aux.ResourceURI, aux.TransactionID, aux.WasRunning, aux.WasEnabled)
}

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
//   - action: the canonical action name from the decoded envelope.
//   - resourceURI: the resource's URI string from the decoded envelope (canonical "svc:<name>" form).
//   - transactionID: the canonical UUIDv7 string from the decoded envelope.
//   - wasRunning: the pre-call running flag from the decoded envelope.
//   - wasEnabled: the pre-call enabled flag from the decoded envelope.
//
// Returns:
//   - error: a missing-context error, a missing-catalog error, a [NewResource] error, or an
//     [op.ReceiptBase.Restore] failure.
func (r *Receipt) hydrate(action, resourceURI, transactionID string, wasRunning, wasEnabled bool) error {

	existing := r.Resource()
	if existing == nil || existing.RuntimeEnvironment() == nil {
		return fmt.Errorf("service.Receipt: unmarshal requires RuntimeEnvironment on receiver")
	}

	runtimeEnvironment := existing.RuntimeEnvironment()
	if runtimeEnvironment.ResourceCatalog == nil {
		return fmt.Errorf("service.Receipt: unmarshal requires Catalog on RuntimeEnvironment")
	}

	// DiscoverResource handles construction + Catalog.Discover internally; no wrapping factory needed.
	resource, err := DiscoverResource(runtimeEnvironment, strings.TrimPrefix(resourceURI, "svc:"))
	if err != nil {
		return fmt.Errorf("service.Receipt: rehydrate resource %q: %w", resourceURI, err)
	}

	r.ReceiptBase = op.NewReceiptBase(resource)

	if err := r.Restore(op.ReceiptData{
		Action:        action,
		ResourceURI:   resourceURI,
		TransactionID: transactionID,
	}); err != nil {
		return fmt.Errorf("service.Receipt: restore: %w", err)
	}

	r.WasRunning = wasRunning
	r.WasEnabled = wasEnabled

	return nil
}
