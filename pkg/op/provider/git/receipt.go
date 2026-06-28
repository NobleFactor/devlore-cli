// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"encoding/json"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Receipt holds git-specific compensation state for a [Provider.Clone] call.
//
// The embedded [op.ReceiptBase] carries the affected [Resource] (the cloned local repository) and the opaque
// [op.ReceiptBase.TransactionID] minted at [op.ReceiptBase.Commit] time. Clone is a Bucket-B (creation, not
// displacement) action — there is no prior content to archive, so the recovery key is the receipt's own
// transactionID; compensation simply removes the cloned directory tree.
//
// Receipt has no provider-specific fields, so it inherits [op.ReceiptBase.MarshalJSON] and
// [op.ReceiptBase.MarshalYAML] unchanged. Only the unmarshalers are overridden, since rehydration requires the
// concrete [Resource] type that [op.ReceiptBase] cannot construct generically.
type Receipt struct {
	op.ReceiptBase
}

// NewReceipt constructs a [Receipt] anchored to the cloned [Resource].
//
// The transactionID and action name remain zero-valued until [op.ReceiptBase.Commit] is invoked when the
// receipt lands on a [op.RecoveryStack] via [op.RecoveryStack.PushComplement].
//
// Parameters:
//   - `resource`: the cloned [Resource] returned by [Provider.Clone].
//
// Returns:
//   - `*Receipt`: the constructed receipt with only its resource populated.
func NewReceipt(resource *Resource) *Receipt {
	return &Receipt{ReceiptBase: op.NewReceiptBase(resource)}
}

// region EXPORTED METHODS

// region Behaviors

// UnmarshalJSON decodes a JSON document produced by [op.ReceiptBase.MarshalJSON] back into the receiver via
// [op.ReceiptBase.Restore].
//
// The receiver MUST be pre-seeded with an [op.RuntimeEnvironment]-bearing zero [Resource] so the unmarshaler can
// rehydrate the encoded URI via [NewResource].
//
// Parameters:
//   - `data`: the JSON-encoded receipt bytes.
//
// Returns:
//   - `error`: any decode error, [NewResource] error, or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalJSON(data []byte) error {

	var aux struct {
		Action        string `json:"action"`
		ResourceURI   string `json:"resource_uri"`
		TransactionID string `json:"transaction_id"`
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("git.Receipt: unmarshal JSON: %w", err)
	}

	return r.hydrate(aux.Action, aux.ResourceURI, aux.TransactionID)
}

// UnmarshalYAML decodes a YAML node produced by [op.ReceiptBase.MarshalYAML] back into the receiver via
// [op.ReceiptBase.Restore].
//
// The receiver MUST be pre-seeded with an [op.RuntimeEnvironment]-bearing zero [Resource]; see
// [Receipt.UnmarshalJSON] for the contract.
//
// Parameters:
//   - `unmarshal`: the YAML library's decode-into callback.
//
// Returns:
//   - `error`: any decode error, [NewResource] error, or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalYAML(unmarshal func(any) error) error {

	var aux struct {
		Action        string `yaml:"action"`
		ResourceURI   string `yaml:"resource_uri"`
		TransactionID string `yaml:"transaction_id"`
	}

	if err := unmarshal(&aux); err != nil {
		return fmt.Errorf("git.Receipt: unmarshal YAML: %w", err)
	}

	return r.hydrate(aux.Action, aux.ResourceURI, aux.TransactionID)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// hydrate reconstructs the receiver's embedded [op.ReceiptBase] from the decoded base envelope. The
// [Resource] is pulled from the [op.ResourceCatalog] on the pre-seeded [op.RuntimeEnvironment] —
// existing entries are re-used (Resource identity is URI-interned); URIs not yet in the catalog are
// constructed via [NewResource] and registered through [op.ResourceCatalog.GetOrCreate]. The base is
// re-seated via [op.NewReceiptBase] so [op.ReceiptBase.Restore]'s URI-match check has a live resource to
// compare against, then the serialized-primitive triplet is handed to Restore.
//
// Parameters:
//   - `action`: the canonical action name from the decoded envelope.
//   - `resourceURI`: the resource's URI string from the decoded envelope.
//   - `transactionID`: the canonical UUIDv7 string from the decoded envelope.
//
// Returns:
//   - `error`: a missing-context error, a missing-catalog error, a [NewResource] error, or an
//     [op.ReceiptBase.Restore] failure.
func (r *Receipt) hydrate(action, resourceURI, transactionID string) error {

	existing := r.Resource()
	if existing == nil || existing.RuntimeEnvironment() == nil {
		return fmt.Errorf("git.Receipt: unmarshal requires RuntimeEnvironment on receiver")
	}

	runtimeEnvironment := existing.RuntimeEnvironment()
	if runtimeEnvironment.ResourceCatalog == nil {
		return fmt.Errorf("git.Receipt: unmarshal requires Catalog on RuntimeEnvironment")
	}

	// DiscoverResource handles construction + Catalog.Discover internally; no wrapping factory needed.
	resource, err := DiscoverResource(runtimeEnvironment, resourceURI)
	if err != nil {
		return fmt.Errorf("git.Receipt: rehydrate resource %q: %w", resourceURI, err)
	}

	r.ReceiptBase = op.NewReceiptBase(resource)

	return r.Restore(op.ReceiptData{
		ForwardAction: action,
		ResourceURI:   resourceURI,
		TransactionID: transactionID,
	})
}

// endregion

// endregion
