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
// The transactionID and action name remain zero-valued until [op.ReceiptBase.Commit] is invoked by
// [op.RecoveryStack.PushReceipt].
//
// Parameters:
//   - resource: the cloned [Resource] returned by [Provider.Clone].
//
// Returns:
//   - *Receipt: the constructed receipt with only its resource populated.
func NewReceipt(resource *Resource) *Receipt {
	return &Receipt{ReceiptBase: op.NewReceiptBase(resource)}
}

// UnmarshalJSON decodes a JSON document produced by [op.ReceiptBase.MarshalJSON] back into the receiver via
// [op.ReceiptBase.Restore].
//
// The receiver MUST be pre-seeded with an [op.ExecutionContext]-bearing zero [Resource] so the unmarshaler can
// rehydrate the encoded URI via [NewResource].
//
// Parameters:
//   - data: the JSON-encoded receipt bytes.
//
// Returns:
//   - error: any decode error, [NewResource] error, or [op.ReceiptBase.Restore] failure.
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
// The receiver MUST be pre-seeded with an [op.ExecutionContext]-bearing zero [Resource]; see
// [Receipt.UnmarshalJSON] for the contract.
//
// Parameters:
//   - unmarshal: the YAML library's decode-into callback.
//
// Returns:
//   - error: any decode error, [NewResource] error, or [op.ReceiptBase.Restore] failure.
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

// hydrate reconstructs the receiver's embedded [op.ReceiptBase] from the decoded base envelope. The
// [Resource] is pulled from the [op.ResourceCatalog] on the pre-seeded [op.ExecutionContext] —
// existing entries are re-used (Resource identity is URI-interned); URIs not yet in the catalog are
// constructed via [NewResource] and registered through [op.ResourceCatalog.GetOrCreate]. The base is
// re-seated via [op.NewReceiptBase] so [op.ReceiptBase.Restore]'s URI-match check has a live resource to
// compare against, then the wire-primitive triplet is handed to Restore.
//
// Parameters:
//   - action: the canonical action name from the decoded envelope.
//   - resourceURI: the resource's URI string from the decoded envelope.
//   - transactionID: the canonical UUIDv7 string from the decoded envelope.
//
// Returns:
//   - error: a missing-context error, a missing-catalog error, a [NewResource] error, or an
//     [op.ReceiptBase.Restore] failure.
func (r *Receipt) hydrate(action, resourceURI, transactionID string) error {

	existing := r.Resource()
	if existing == nil || existing.ExecutionContext() == nil {
		return fmt.Errorf("git.Receipt: unmarshal requires ExecutionContext on receiver")
	}

	ctx := existing.ExecutionContext()
	if ctx.Catalog == nil {
		return fmt.Errorf("git.Receipt: unmarshal requires Catalog on ExecutionContext")
	}

	resource, err := ctx.Catalog.GetOrCreate(resourceURI, func() (op.Resource, error) {
		return NewResource(ctx, resourceURI)
	})
	if err != nil {
		return fmt.Errorf("git.Receipt: rehydrate resource %q: %w", resourceURI, err)
	}

	r.ReceiptBase = op.NewReceiptBase(resource)

	return r.Restore(struct {
		Action        string `json:"action"         yaml:"action"`
		ResourceURI   string `json:"resource_uri"   yaml:"resource_uri"`
		TransactionID string `json:"transaction_id" yaml:"transaction_id"`
	}{
		Action:        action,
		ResourceURI:   resourceURI,
		TransactionID: transactionID,
	})
}