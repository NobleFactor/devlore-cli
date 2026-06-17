// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package encryption

import (
	"encoding/json"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
)

// Receipt holds encryption-specific compensation state for a [Provider.DecryptSopsFile] call.
//
// The embedded [op.ReceiptBase] carries the affected [file.Resource] (the decrypted destination file) and the opaque
// [op.ReceiptBase.TransactionID] minted at [op.ReceiptBase.Commit] time. The destination path is read through the
// resource's [file.Resource.SourcePath] during compensation; no per-receipt path field is needed.
//
// Receipt has no provider-specific fields, so it inherits [op.ReceiptBase.MarshalJSON] and
// [op.ReceiptBase.MarshalYAML] unchanged. Only the unmarshalers are overridden, since rehydration requires the
// concrete [file.Resource] type that [op.ReceiptBase] cannot construct generically.
type Receipt struct {
	op.ReceiptBase
}

// UnmarshalJSON decodes a JSON document produced by [op.ReceiptBase.MarshalJSON] back into the receiver via
// [op.ReceiptBase.Restore].
//
// The receiver MUST be pre-seeded with an [op.RuntimeEnvironment]-bearing zero [file.Resource] so the unmarshaler can
// rehydrate the encoded URI via [file.NewResource].
//
// Parameters:
//   - data: the JSON-encoded receipt bytes.
//
// Returns:
//   - error: any decode error, [file.NewResource] error, or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalJSON(data []byte) error {

	var aux struct {
		Action        string `json:"action"`
		ResourceURI   string `json:"resource_uri"`
		TransactionID string `json:"transaction_id"`
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("encryption.Receipt: unmarshal JSON: %w", err)
	}

	return r.hydrate(aux.Action, aux.ResourceURI, aux.TransactionID)
}

// UnmarshalYAML decodes a YAML node produced by [op.ReceiptBase.MarshalYAML] back into the receiver via
// [op.ReceiptBase.Restore].
//
// The receiver MUST be pre-seeded with an [op.RuntimeEnvironment]-bearing zero [file.Resource]; see
// [Receipt.UnmarshalJSON] for the contract.
//
// Parameters:
//   - unmarshal: the YAML library's decode-into callback.
//
// Returns:
//   - error: any decode, [file.NewResource] error, or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalYAML(unmarshal func(any) error) error {

	var aux struct {
		Action        string `yaml:"action"`
		ResourceURI   string `yaml:"resource_uri"`
		TransactionID string `yaml:"transaction_id"`
	}

	if err := unmarshal(&aux); err != nil {
		return fmt.Errorf("encryption.Receipt: unmarshal YAML: %w", err)
	}

	return r.hydrate(aux.Action, aux.ResourceURI, aux.TransactionID)
}

// hydrate reconstructs the receiver's embedded [op.ReceiptBase] from the decoded base envelope. The
// [file.Resource] is pulled from the [op.ResourceCatalog] on the pre-seeded [op.RuntimeEnvironment] —
// existing entries are re-used (Resource identity is URI-interned); URIs not yet in the catalog are
// constructed via [file.NewResource] and registered through [op.ResourceCatalog.GetOrCreate]. The base is
// re-seated via [op.NewReceiptBase] so [op.ReceiptBase.Restore]'s URI-match check has a live resource to
// compare against, then the serialized-primitive triplet is handed to Restore.
//
// Parameters:
//   - action: the canonical action name from the decoded envelope.
//   - resourceURI: the resource's URI string from the decoded envelope.
//   - transactionID: the canonical UUIDv7 string from the decoded envelope.
//
// Returns:
//   - error: a missing-context error, a missing-catalog error, a [file.NewResource] error, or an
//     [op.ReceiptBase.Restore] failure.
func (r *Receipt) hydrate(action, resourceURI, transactionID string) error {

	existing := r.Resource()
	if existing == nil || existing.RuntimeEnvironment() == nil {
		return fmt.Errorf("encryption.Receipt: unmarshal requires RuntimeEnvironment on receiver")
	}

	runtimeEnvironment := existing.RuntimeEnvironment()
	if runtimeEnvironment.ResourceCatalog == nil {
		return fmt.Errorf("encryption.Receipt: unmarshal requires Catalog on RuntimeEnvironment")
	}

	// file.DiscoverResource handles construction + Catalog.Discover internally; no wrapping factory needed.
	resource, err := file.DiscoverResource(runtimeEnvironment, resourceURI)
	if err != nil {
		return fmt.Errorf("encryption.Receipt: rehydrate resource %q: %w", resourceURI, err)
	}

	r.ReceiptBase = op.NewReceiptBase(resource)

	return r.Restore(op.ReceiptData{
		Action:        action,
		ResourceURI:   resourceURI,
		TransactionID: transactionID,
	})
}
