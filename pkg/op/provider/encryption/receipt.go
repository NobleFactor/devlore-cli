// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package encryption

import (
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
// [op.ReceiptBase.MarshalYAML] unchanged. Only [Receipt.RestoreEncoded] is overridden, since rehydration requires the
// concrete [file.Resource] type that [op.ReceiptBase] cannot construct generically.
type Receipt struct {
	op.ReceiptBase
}

// region EXPORTED METHODS

// region Behaviors

// RestoreEncoded reconstructs the receipt from its codec-decoded envelope, resolving its [file.Resource] against the
// rehydrated catalog.
//
// It is the [op.Receipt.RestoreEncoded] override the recovery stack drives at re-arm (via [op.reconstructReceipt]) —
// the env is threaded in explicitly as a parameter, not read off the receiver, so the stack path (which loads a bare
// receipt before the catalog is rehydrated) can reconstruct it. The destination [file.Resource] is resolved from
// `base.ResourceURI` via the catalog namespace; the base is re-seated via [op.NewReceiptBase] so
// [op.ReceiptBase.Restore]'s URI-match check has a live resource, then Restore writes the full base. Receipt has no
// provider-specific fields, so `fields` is unused.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment; its catalog must hold (or be able to construct) the resource.
//   - `base`: the codec-decoded base execution state.
//   - `_`: the receipt's id-reference sub-field, unused (no provider-specific fields).
//
// Returns:
//   - `error`: a missing catalog, a resource-resolution failure, or an [op.ReceiptBase.Restore] failure.
func (r *Receipt) RestoreEncoded(
	runtimeEnvironment *op.RuntimeEnvironment, base op.ReceiptData, _ map[string]any,
) error {

	if runtimeEnvironment == nil || runtimeEnvironment.ResourceCatalog == nil {
		return fmt.Errorf("encryption.Receipt: RestoreEncoded requires a runtime environment with a catalog")
	}

	// The destination file was produced during the forward run and lives in the rehydrated catalog; resolve it from its
	// URI through the catalog's URI->id namespace (a Resource.URI() is a canonical tag URI, not a DiscoverResource input).
	catalog := runtimeEnvironment.ResourceCatalog
	got, ok := catalog.Lookup(catalog.Current(base.ResourceURI))
	if !ok {
		return fmt.Errorf("encryption.Receipt: RestoreEncoded: resource %q not in catalog", base.ResourceURI)
	}
	resource, ok := got.(*file.Regular)
	if !ok {
		return fmt.Errorf("encryption.Receipt: RestoreEncoded: catalog entry for %q is %T, want *file.Regular",
			base.ResourceURI, got)
	}

	r.ReceiptBase = op.NewReceiptBase(resource)

	if err := r.Restore(base); err != nil {
		return fmt.Errorf("encryption.Receipt: RestoreEncoded restore: %w", err)
	}

	return nil
}

// endregion

// endregion
