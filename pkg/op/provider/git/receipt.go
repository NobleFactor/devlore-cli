// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package git

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/assert"
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
// [op.ReceiptBase.MarshalYAML] unchanged. Only [Receipt.RestoreEncoded] is overridden, since rehydration requires the
// concrete [Resource] type that [op.ReceiptBase] cannot construct generically.
type Receipt struct {
	op.ReceiptBase
}

// NewReceipt constructs a [Receipt] anchored to the cloned [Resource].
//
// The transactionID and action name remain zero-valued until [op.ReceiptBase.Commit] is invoked when the
// receipt lands on a [op.RecoveryStack] via [op.RecoveryStack.PushCompensator].
//
// Parameters:
//   - `resource`: the cloned [Resource] returned by [Provider.Clone].
//
// Returns:
//   - `*Receipt`: the constructed receipt with only its resource populated.
func NewReceipt(resource Resource) *Receipt {
	return &Receipt{ReceiptBase: op.NewReceiptBase(resource)}
}

// region EXPORTED METHODS

// region Behaviors

// RestoreEncoded reconstructs the receipt from its codec-decoded envelope, resolving its [Resource] against the
// rehydrated catalog.
//
// It is the [op.Receipt.RestoreEncoded] override the recovery stack drives at re-arm (via [op.reconstructReceipt]) —
// the env is threaded in explicitly as a parameter, not read off the receiver, so the stack path (which loads a bare
// receipt before the catalog is rehydrated) can reconstruct it. The cloned [Resource] is resolved from
// `base.ResourceURI` via [DiscoverResource]; the base is re-seated via [op.NewReceiptBase] so
// [op.ReceiptBase.Restore]'s URI-match check has a live resource, then Restore writes the full base. Receipt has no
// provider-specific fields, so `fields` is unused.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment; its catalog must hold (or be able to construct) the resource.
//   - `base`: the codec-decoded base execution state.
//   - `_`: the receipt's id-reference sub-field, unused (no provider-specific fields).
//
// Returns:
//   - `error`: non-nil only when the runtime environment or its catalog is missing; resolution and restore
//     failures are verified-side defects and assert.
func (r *Receipt) RestoreEncoded(
	runtimeEnvironment *op.RuntimeEnvironment, base op.ReceiptData, _ map[string]any,
) error {

	if runtimeEnvironment == nil || runtimeEnvironment.ResourceCatalog == nil {
		return fmt.Errorf("git.Receipt: RestoreEncoded requires a runtime environment with a catalog")
	}

	// The resource was produced during the forward run and lives in the rehydrated catalog; resolve it from its URI
	// through the catalog's URI->id namespace (a Resource.URI() is a canonical tag URI, not a DiscoverResource input).
	catalog := runtimeEnvironment.ResourceCatalog
	got, ok := catalog.Lookup(catalog.Current(base.ResourceURI))
	assert.True("git.Receipt: resource "+base.ResourceURI+" in catalog", ok)
	resource, ok := got.(Resource)
	assert.True("git.Receipt: catalog entry is *git.Resource", ok)

	r.ReceiptBase = op.NewReceiptBase(resource)
	assert.NoError("git.Receipt: restore base", r.Restore(base))

	return nil
}

// endregion

// endregion
