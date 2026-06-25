// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"encoding/json"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/google/uuid"
)

// Receipt holds the file-specific compensation state that the recovery system needs to undo a compensable forward call.
//
// The embedded [op.ReceiptBase] carries the affected [Resource] whose identity is preserved across compensation, and an
// opaque [op.ReceiptBase.TransactionID] that [op.RecoverySite] interprets as the recovery key when restoring archived
// bytes. SourcePath always reflects the file's true home — the location compensation will write back to.
//
// The optional boundary [Resource] marks the edge between existing filesystem state and the subtree the forward action
// created. Compensation walks toward boundary and stops at it (exclusive). [Provider.Mkdir], for example, sets boundary
// to the nearest pre-existing ancestor of its target directory so [Provider.CompensateMkdir] knows where to halt the
// upward removal walk. Methods that do not need a transactional anchor leave boundary nil.
//
// The optional source [Resource] records the original location for move-like operations.
//
// The optional recoveryDigest records the digest of the archived bytes at archive time. Compensation re-hashes the
// recovery archive and compares against this stored value to detect tampering of the recovery store between the
// forward action and compensation. Empty when no archive was made (recoveryID is also empty in that case).
type Receipt struct {
	op.ReceiptBase

	// boundary marks the edge for parent directory pruning.
	boundary *Resource

	// source records the original location for Move/Backup.
	source *Resource

	// recoveryID is the UUID returned by RecoverySite.ArchiveFile for the file overwritten at the destination.
	// Optional.
	recoveryID uuid.UUID

	// recoveryDigest is the digest of the bytes archived under recoveryID, captured at archive time. Used by
	// compensation to verify the recovery archive has not been tampered with before restoration. Optional —
	// zero value indicates no digest was captured (typically because nothing was archived).
	recoveryDigest op.Digest
}

// NewReceipt constructs a [Receipt] anchored to the affected [Resource] with no transactional boundary.
//
// The transactionID and action name remain zero-valued until [op.ReceiptBase.Commit] is invoked when the
// receipt lands on a [op.RecoveryStack] via [op.RecoveryStack.PushComplement]. Use this constructor wherever a
// forward action needs to return a Receipt rather than building the struct literal inline, and the action did
// not need to record a creation boundary for compensation.
//
// Parameters:
//   - `resource`: the [Resource] affected by the compensable forward method call.
//
// Returns:
//   - `*Receipt`: the constructed receipt with only its resource populated.
func NewReceipt(resource *Resource) *Receipt {
	return &Receipt{ReceiptBase: op.NewReceiptBase(resource)}
}

// NewReceiptWithBoundary constructs a [Receipt] anchored to the affected [Resource] with a transactional boundary.
//
// Use this constructor when the forward action created or modified a subtree of filesystem state and compensation
// must stop at a known edge to avoid removing pre-existing entries. [Provider.Mkdir] is the canonical caller:
// resource is the directory created and boundary is the nearest pre-existing ancestor — compensation walks from
// resource up to but excluding boundary, removing each empty directory along the way.
//
// Parameters:
//   - `resource`: the [Resource] affected by the compensable forward method call.
//   - `boundary`: the [Resource] marking the existing-state edge; compensation stops at it (exclusive).
//
// Returns:
//   - `*Receipt`: the constructed receipt with both resource and boundary populated.
func NewReceiptWithBoundary(resource, boundary *Resource) *Receipt {
	return &Receipt{ReceiptBase: op.NewReceiptBase(resource), boundary: boundary}
}

// region EXPORTED METHODS

// region State management

// Boundary returns the transactional boundary [Resource] supplied at construction, or nil if none was set.
//
// Compensation methods read this value to bound their cleanup walk: any walk that would step past boundary (an upward
// walk reaching it, or a downward walk descending into it) must halt. A nil boundary signals that the forward action
// did not record a creation subtree and the compensation method has no boundary-driven cleanup to perform.
//
// Returns:
//   - `*Resource`: the boundary supplied to [NewReceiptWithBoundary], or nil for receipts built via [NewReceipt].
func (r *Receipt) Boundary() *Resource {
	return r.boundary
}

// RecoveryDigest returns the digest of the bytes archived under [Receipt.RecoveryID] at archive time. The zero
// [op.Digest] value indicates no digest was captured (typically when nothing was archived).
//
// Compensation methods read this value to verify the recovery archive's integrity before restoration: re-hash the
// archive's current bytes, compare against the stored digest, error on mismatch (the archive was tampered with
// between the forward action and compensation).
//
// Returns:
//   - `op.Digest`: the captured digest, or the zero value when none was set.
func (r *Receipt) RecoveryDigest() op.Digest {
	return r.recoveryDigest
}

// RecoveryID returns the recovery ID for the file overwritten at the destination, or an empty string if none.
//
// Returns:
//   - `string`: the recovery ID.
func (r *Receipt) RecoveryID() string {
	if r.recoveryID != uuid.Nil {
		return r.recoveryID.String()
	}
	return ""
}

// SetRecoveryDigest stores the digest of the archived bytes. Forward methods that archive content via
// [op.RecoverySite.ArchiveFile] capture the bytes' digest at archive time and stash it here so compensation can
// later verify the archive has not been tampered with.
//
// Parameters:
//   - `d`: the [op.Digest] to store; pass the zero value to clear.
func (r *Receipt) SetRecoveryDigest(d op.Digest) {
	r.recoveryDigest = d
}

// SetRecoveryID sets the recovery ID for the file overwritten at the destination.
//
// Parameters:
//   - `id`: the recovery ID as a UUID string; an empty string clears it.
//
// Returns:
//   - `error`: non-nil when `id` is non-empty and not a valid UUID.
func (r *Receipt) SetRecoveryID(id string) error {
	if id == "" {
		r.recoveryID = uuid.Nil
		return nil
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	r.recoveryID = parsed
	return nil
}

// SetSource sets the original location [Resource] for move-like operations.
//
// Parameters:
//   - `source`: the original-location [*Resource] to record.
func (r *Receipt) SetSource(source *Resource) {
	r.source = source
}

// Source returns the original location [Resource] for move-like operations, or nil if none was set.
//
// Returns:
//   - `*Resource`: the source resource.
func (r *Receipt) Source() *Resource {
	return r.source
}

// endregion

// region Behaviors

// MarshalJSON encodes the receipt's compensation state as JSON — the resource, boundary, and source catalog ids plus
// the transaction id and recovery key/digest.
//
// Delegates to [Receipt.MarshalYAML] for the serialized-shape value, then runs [json.Marshal] over it.
//
// Returns:
//   - `[]byte`: JSON-encoded object carrying the receipt's id references and recovery key.
//   - `error`: any error from [Receipt.MarshalYAML] or [json.Marshal].
func (r *Receipt) MarshalJSON() ([]byte, error) {

	v, err := r.MarshalYAML()
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// MarshalYAML returns the receipt's compensation state as an anonymous struct value the YAML encoder serializes.
//
// This is the `receipt` sub-field the recovery stack embeds for a resource receipt: resource, boundary, and source are
// emitted as catalog **ids** (a URI is not a unique identity — a shadowed generation shares its URI), alongside the
// transaction id and the recovery key/digest. The base execution state (`unit_id`/`action`/`result`/`status`) rides the
// stack-owned envelope, so it is not repeated here; resume resolves the ids via [op.ResourceCatalog.Lookup] in
// [Receipt.RestoreEncoded]. Both `json:` and `yaml:` tags ride every field so the value flows through either encoder via
// [Receipt.MarshalJSON].
//
// Returns:
//   - `any`: the populated anonymous struct for the YAML encoder to walk.
//   - `error`: nil under normal conditions.
func (r *Receipt) MarshalYAML() (any, error) {

	var resourceID string
	if resource := r.Resource(); resource != nil {
		resourceID = resource.ID()
	}

	var boundaryID string
	if r.boundary != nil {
		boundaryID = r.boundary.ID()
	}

	var sourceID string
	if r.source != nil {
		sourceID = r.source.ID()
	}

	var recoveryID string
	if r.recoveryID != uuid.Nil {
		recoveryID = r.recoveryID.String()
	}

	var recoveryDigest string
	if r.recoveryDigest.Algorithm != "" {
		recoveryDigest = r.recoveryDigest.String()
	}

	return struct {
		ResourceID     string `json:"resource_id"               yaml:"resource_id"`
		TransactionID  string `json:"transaction_id"            yaml:"transaction_id"`
		BoundaryID     string `json:"boundary_id,omitempty"     yaml:"boundary_id,omitempty"`
		SourceID       string `json:"source_id,omitempty"       yaml:"source_id,omitempty"`
		RecoveryID     string `json:"recovery_id,omitempty"     yaml:"recovery_id,omitempty"`
		RecoveryDigest string `json:"recovery_digest,omitempty" yaml:"recovery_digest,omitempty"`
	}{
		ResourceID:     resourceID,
		TransactionID:  r.TransactionID(),
		BoundaryID:     boundaryID,
		SourceID:       sourceID,
		RecoveryID:     recoveryID,
		RecoveryDigest: recoveryDigest,
	}, nil
}

// RestoreEncoded reconstructs the receipt from a recovery-stack envelope, resolving its resource id references against
// the runtime environment's rehydrated ledger.
//
// It is the [op.Receipt.RestoreEncoded] override for file receipts. The envelope carries the base execution state
// (`unit_id`/`action`/`result`/`status`) plus a `receipt` sub-field of id references; this resolves `resource_id`,
// `boundary_id`, and `source_id` via [op.ResourceCatalog.Lookup] (the ledger having been rehydrated first), seeds the
// base via [op.NewReceiptBase] + [op.ReceiptBase.Restore], and restores the recovery key and digest. Resolving by id
// (not URI) pins the exact generation the receipt captured, even after the URI was shadowed by a later one.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment; its catalog must already hold the saved generations.
//   - `data`: the JSON of one receipt envelope.
//
// Returns:
//   - `error`: a missing catalog, an unresolved id, or a malformed envelope / recovery field.
func (r *Receipt) RestoreEncoded(runtimeEnvironment *op.RuntimeEnvironment, data []byte) error {

	if runtimeEnvironment == nil || runtimeEnvironment.ResourceCatalog == nil {
		return fmt.Errorf("file.Receipt: RestoreEncoded requires a runtime environment with a catalog")
	}

	var envelope struct {
		UnitID  string          `json:"unit_id"`
		Action  string          `json:"action"`
		Result  any             `json:"result"`
		Status  string          `json:"status"`
		Receipt json.RawMessage `json:"receipt"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("file.Receipt: RestoreEncoded: %w", err)
	}

	var aux struct {
		ResourceID     string `json:"resource_id"`
		TransactionID  string `json:"transaction_id"`
		BoundaryID     string `json:"boundary_id"`
		SourceID       string `json:"source_id"`
		RecoveryID     string `json:"recovery_id"`
		RecoveryDigest string `json:"recovery_digest"`
	}
	if err := json.Unmarshal(envelope.Receipt, &aux); err != nil {
		return fmt.Errorf("file.Receipt: RestoreEncoded receipt sub-field: %w", err)
	}

	resource, err := lookupResource(runtimeEnvironment, aux.ResourceID)
	if err != nil {
		return err
	}

	r.ReceiptBase = op.NewReceiptBase(resource)
	if err := r.Restore(op.ReceiptData{
		Action:        envelope.Action,
		ActionPath:    envelope.Action,
		UnitID:        envelope.UnitID,
		Result:        envelope.Result,
		Status:        envelope.Status,
		ResourceURI:   resource.URI(),
		TransactionID: aux.TransactionID,
	}); err != nil {
		return fmt.Errorf("file.Receipt: RestoreEncoded restore base: %w", err)
	}

	if aux.BoundaryID != "" {
		if r.boundary, err = lookupResource(runtimeEnvironment, aux.BoundaryID); err != nil {
			return err
		}
	}

	if aux.SourceID != "" {
		if r.source, err = lookupResource(runtimeEnvironment, aux.SourceID); err != nil {
			return err
		}
	}

	if aux.RecoveryID != "" {
		if r.recoveryID, err = uuid.Parse(aux.RecoveryID); err != nil {
			return fmt.Errorf("file.Receipt: RestoreEncoded parse recovery_id %q: %w", aux.RecoveryID, err)
		}
	}

	if aux.RecoveryDigest != "" {
		if r.recoveryDigest, err = op.ParseDigest(aux.RecoveryDigest); err != nil {
			return fmt.Errorf("file.Receipt: RestoreEncoded parse recovery_digest %q: %w", aux.RecoveryDigest, err)
		}
	}

	return nil
}

// endregion

// endregion

// region HELPER FUNCTIONS

// lookupResource resolves a catalog id to its concrete [*Resource], or errors when the id is absent or typed wrong.
//
// Resume reconstructs a receipt's resource references by id (not URI) so a shadowed generation resolves to the exact
// resource the receipt captured; the ledger must already be rehydrated.
//
// Parameters:
//   - `runtimeEnvironment`: the environment whose rehydrated catalog holds the generation.
//   - `id`: the catalog id (`res-N`) captured in the receipt's encoding.
//
// Returns:
//   - `*Resource`: the resolved file resource.
//   - `error`: when the id is not in the catalog or its entry is not a file resource.
func lookupResource(runtimeEnvironment *op.RuntimeEnvironment, id string) (*Resource, error) {

	got, ok := runtimeEnvironment.ResourceCatalog.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("file.Receipt: resource id %q not in catalog", id)
	}

	resource, ok := got.(*Resource)
	if !ok {
		return nil, fmt.Errorf("file.Receipt: catalog entry %q is %T, want *file.Resource", id, got)
	}

	return resource, nil
}

// endregion
