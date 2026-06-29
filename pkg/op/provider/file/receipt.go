// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"encoding/json"
	"fmt"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/google/uuid"
)

// MutationKind identifies the filesystem mutation a [Receipt] records, so [Provider.CompensateFileMutation] can invert
// it: remove a created file or directory, restore prior content from recovery for an update or delete, or recreate a
// removed directory.
type MutationKind string

const (
	// MutationCreateFile records a file that did not exist before the write; its undo removes the file.
	MutationCreateFile MutationKind = "create_file"

	// MutationUpdateFile records a file whose prior content was archived to recovery before the overwrite; its undo
	// restores that content.
	MutationUpdateFile MutationKind = "update_file"

	// MutationDeleteFile records a file removed after its content was archived to recovery; its undo restores it.
	MutationDeleteFile MutationKind = "delete_file"

	// MutationCreateDir records a directory the call created; its undo removes it.
	MutationCreateDir MutationKind = "create_dir"

	// MutationDeleteDir records a directory the call removed; its undo recreates it.
	MutationDeleteDir MutationKind = "delete_dir"
)

// compensateFileMutationAction is the dotted compensator name every file.Receipt declares at construction.
//
// [Provider.CompensateFileMutation] inverts any file or directory mutation by dispatching on the receipt's
// [MutationKind]; the name matches the registry's compensator-index key (provider name + snake method name), so a file
// receipt routes to that one compensator regardless of which method or dispatcher created it.
const compensateFileMutationAction = "file.compensate_file_mutation"

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

	// kind records which filesystem mutation produced this receipt, so compensation inverts the right one.
	kind MutationKind

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

// NewReceipt builds a [*Receipt] from a populated [*ReceiptSpec].
//
// The receipt declares its undo at construction: it names [compensateFileMutationAction] as its compensator (so
// [Provider.CompensateFileMutation] inverts it regardless of which method or dispatcher created it) and copies the
// spec's kind and optional boundary / recovery / source. The transactionID is minted later at [op.ReceiptBase.Commit].
//
// Parameters:
//   - `spec`: the populated receipt spec; build it with [NewReceiptSpec] and its With* methods.
//
// Returns:
//   - `*Receipt`: the constructed receipt.
func NewReceipt(spec *ReceiptSpec) *Receipt {
	return &Receipt{
		ReceiptBase:    op.NewReceiptBaseWithCompensator(spec.resource, compensateFileMutationAction),
		kind:           spec.kind,
		boundary:       spec.boundary,
		source:         spec.source,
		recoveryID:     spec.recoveryID,
		recoveryDigest: spec.recoveryDigest,
	}
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

// Kind returns the [MutationKind] this receipt records, or "" when unset.
//
// Returns:
//   - `MutationKind`: the recorded mutation kind.
func (r *Receipt) Kind() MutationKind {
	return r.kind
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
// transaction id, the recovery key/digest, and the mutation kind. The base execution state
// (`unit_id`/`action`/`result`/`status`) rides the
// stack-owned envelope, so it is not repeated here; resume resolves the ids via [op.ResourceCatalog.Lookup] in
// [Receipt.RestoreEncoded]. Both `json:` and `yaml:` tags ride every field so the value flows through either encoder
// via [Receipt.MarshalJSON].
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
		Kind           string `json:"kind,omitempty"            yaml:"kind,omitempty"`
	}{
		ResourceID:     resourceID,
		TransactionID:  r.TransactionID(),
		BoundaryID:     boundaryID,
		SourceID:       sourceID,
		RecoveryID:     recoveryID,
		RecoveryDigest: recoveryDigest,
		Kind:           string(r.kind),
	}, nil
}

// RestoreEncoded reconstructs the receipt from its codec-decoded envelope, resolving its resource id references against
// the runtime environment's rehydrated ledger.
//
// It is the [op.Receipt.RestoreEncoded] override for file receipts. The recovery stack already decoded the envelope —
// through whichever codec read the trace — so this consumes decoded values, never bytes: `base` carries the execution
// state and `fields` the id-reference sub-field. It resolves `resource_id`, `boundary_id`, and `source_id` via
// [op.ResourceCatalog.Lookup] (the ledger having been rehydrated first), seeds the base via [op.NewReceiptBase] +
// [op.ReceiptBase.Restore], and restores the recovery key and digest. Resolving by id (not URI) pins the exact
// generation the receipt captured, even after the URI was shadowed by a later one.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment; its catalog must already hold the saved generations.
//   - `base`: the codec-decoded base execution state.
//   - `fields`: the receipt's id-reference sub-field, decoded to a format-neutral map.
//
// Returns:
//   - `error`: a missing catalog, an unresolved id, or a malformed recovery field.
func (r *Receipt) RestoreEncoded(
	runtimeEnvironment *op.RuntimeEnvironment, base op.ReceiptData, fields map[string]any,
) error {

	if runtimeEnvironment == nil || runtimeEnvironment.ResourceCatalog == nil {
		return fmt.Errorf("file.Receipt: RestoreEncoded requires a runtime environment with a catalog")
	}

	resource, err := lookupResource(runtimeEnvironment, stringField(fields, "resource_id"))
	if err != nil {
		return err
	}

	r.ReceiptBase = op.NewReceiptBase(resource)
	if err := r.Restore(op.ReceiptData{
		ForwardAction:      base.ForwardAction,
		CompensatingAction: base.CompensatingAction,
		UnitID:             base.UnitID,
		Result:             base.Result,
		ResultType:         base.ResultType,
		Status:             base.Status,
		ResourceURI:        resource.URI(),
		TransactionID:      stringField(fields, "transaction_id"),
	}); err != nil {
		return fmt.Errorf("file.Receipt: RestoreEncoded restore base: %w", err)
	}

	if boundaryID := stringField(fields, "boundary_id"); boundaryID != "" {
		if r.boundary, err = lookupResource(runtimeEnvironment, boundaryID); err != nil {
			return err
		}
	}

	if sourceID := stringField(fields, "source_id"); sourceID != "" {
		if r.source, err = lookupResource(runtimeEnvironment, sourceID); err != nil {
			return err
		}
	}

	if recoveryID := stringField(fields, "recovery_id"); recoveryID != "" {
		if r.recoveryID, err = uuid.Parse(recoveryID); err != nil {
			return fmt.Errorf("file.Receipt: RestoreEncoded parse recovery_id %q: %w", recoveryID, err)
		}
	}

	if recoveryDigest := stringField(fields, "recovery_digest"); recoveryDigest != "" {
		if r.recoveryDigest, err = op.ParseDigest(recoveryDigest); err != nil {
			return fmt.Errorf("file.Receipt: RestoreEncoded parse recovery_digest %q: %w", recoveryDigest, err)
		}
	}

	r.kind = MutationKind(stringField(fields, "kind"))

	return nil
}

// endregion

// endregion

// region SUPPORTING TYPES

// ReceiptSpec is the fluent builder for a [*Receipt], mirroring the [op.NodeSpec] / [op.NewNode] shape used across the
// framework. The required identity — the affected [Resource] and the [MutationKind] — is supplied to [NewReceiptSpec];
// optional compensation state (boundary, recovery, source) is added through the With* methods. Hand a populated spec to
// [NewReceipt].
type ReceiptSpec struct {
	resource       *Resource
	kind           MutationKind
	boundary       *Resource
	source         *Resource
	recoveryID     uuid.UUID
	recoveryDigest op.Digest
}

// NewReceiptSpec returns a [*ReceiptSpec] for a `kind` mutation of `resource`, ready for optional With* population.
//
// Parameters:
//   - `resource`: the [Resource] affected by the compensable forward method call.
//   - `kind`: the [MutationKind] the receipt records.
//
// Returns:
//   - `*ReceiptSpec`: the spec with its required identity populated.
func NewReceiptSpec(resource *Resource, kind MutationKind) *ReceiptSpec {
	return &ReceiptSpec{resource: resource, kind: kind}
}

// WithBoundary records the transactional boundary — the nearest pre-existing ancestor a create walks back to during
// compensation — and returns the spec for chaining.
//
// Parameters:
//   - `boundary`: the existing-state edge; compensation stops at it (exclusive).
//
// Returns:
//   - `*ReceiptSpec`: the receiver, for chaining.
func (s *ReceiptSpec) WithBoundary(boundary *Resource) *ReceiptSpec {
	s.boundary = boundary
	return s
}

// WithRecovery records the recovery archive of the displaced content and returns the spec for chaining.
//
// A non-UUID `recoveryID` clears the id (a malformed key cannot name an archive); recovery keys produced by
// [op.RecoverySite] are always valid UUIDs, so this matches the prior construction, whose parse error was discarded.
//
// Parameters:
//   - `recoveryID`: the [op.RecoverySite] key for the archived bytes, as a UUID string.
//   - `digest`: the digest of those bytes, captured at archive time for tamper detection.
//
// Returns:
//   - `*ReceiptSpec`: the receiver, for chaining.
func (s *ReceiptSpec) WithRecovery(recoveryID string, digest op.Digest) *ReceiptSpec {
	s.recoveryID, _ = uuid.Parse(recoveryID)
	s.recoveryDigest = digest
	return s
}

// WithSource records the original location for a move and returns the spec for chaining.
//
// Parameters:
//   - `source`: the move's origin [*Resource], to which compensation moves the file back.
//
// Returns:
//   - `*ReceiptSpec`: the receiver, for chaining.
func (s *ReceiptSpec) WithSource(source *Resource) *ReceiptSpec {
	s.source = source
	return s
}

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

// stringField returns the string value at `key` in a decoded receipt sub-field, or "" when absent or not a string.
//
// The sub-field arrives as a format-neutral map (decoded by whichever codec read the trace), so reads go through a
// typed lookup rather than struct-tag decoding; an absent or wrong-typed value yields "", which the caller treats as
// "not present".
//
// Parameters:
//   - `fields`: the decoded id-reference sub-field.
//   - `key`: the field name to read.
//
// Returns:
//   - `string`: the value, or "" when absent or not a string.
func stringField(fields map[string]any, key string) string {

	value, _ := fields[key].(string)
	return value
}

// endregion
