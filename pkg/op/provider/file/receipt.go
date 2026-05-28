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
//   - resource: the [Resource] affected by the compensable forward method call.
//
// Returns:
//   - *Receipt: the constructed receipt with only its resource populated.
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
//   - resource: the [Resource] affected by the compensable forward method call.
//   - boundary: the [Resource] marking the existing-state edge; compensation stops at it (exclusive).
//
// Returns:
//   - *Receipt: the constructed receipt with both resource and boundary populated.
func NewReceiptWithBoundary(resource, boundary *Resource) *Receipt {
	return &Receipt{ReceiptBase: op.NewReceiptBase(resource), boundary: boundary}
}

// Boundary returns the transactional boundary [Resource] supplied at construction, or nil if none was set.
//
// Compensation methods read this value to bound their cleanup walk: any walk that would step past boundary (an upward
// walk reaching it, or a downward walk descending into it) must halt. A nil boundary signals that the forward action
// did not record a creation subtree and the compensation method has no boundary-driven cleanup to perform.
//
// Returns:
//   - *Resource: the boundary supplied to [NewReceiptWithBoundary], or nil for receipts built via [NewReceipt].
func (r *Receipt) Boundary() *Resource {
	return r.boundary
}

// Source returns the original location [Resource] for move-like operations, or nil if none was set.
//
// Returns:
//   - *Resource: the source resource.
func (r *Receipt) Source() *Resource {
	return r.source
}

// SetSource sets the original location [Resource] for move-like operations.
func (r *Receipt) SetSource(source *Resource) {
	r.source = source
}

// RecoveryID returns the recovery ID for the file overwritten at the destination, or an empty string if none.
//
// Returns:
//   - string: the recovery ID.
func (r *Receipt) RecoveryID() string {
	if r.recoveryID != uuid.Nil {
		return r.recoveryID.String()
	}
	return ""
}

// SetRecoveryID sets the recovery ID for the file overwritten at the destination.
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

// RecoveryDigest returns the digest of the bytes archived under [Receipt.RecoveryID] at archive time. The zero
// [op.Digest] value indicates no digest was captured (typically when nothing was archived).
//
// Compensation methods read this value to verify the recovery archive's integrity before restoration: re-hash the
// archive's current bytes, compare against the stored digest, error on mismatch (the archive was tampered with
// between the forward action and compensation).
//
// Returns:
//   - op.Digest: the captured digest, or the zero value when none was set.
func (r *Receipt) RecoveryDigest() op.Digest {
	return r.recoveryDigest
}

// SetRecoveryDigest stores the digest of the archived bytes. Forward methods that archive content via
// [op.RecoverySite.ArchiveFile] capture the bytes' digest at archive time and stash it here so compensation can
// later verify the archive has not been tampered with.
//
// Parameters:
//   - d: the [op.Digest] to store; pass the zero value to clear.
func (r *Receipt) SetRecoveryDigest(d op.Digest) {
	r.recoveryDigest = d
}

// MarshalJSON encodes the receipt as JSON: the base envelope (action, resource_uri, transaction_id) extended with
// the optional boundary_uri and source_uri.
//
// Delegates to [Receipt.MarshalYAML] for the wire-shape value, then runs [json.Marshal] over it.
//
// Returns:
//   - []byte: JSON-encoded object carrying the base envelope plus boundary_uri and source_uri.
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
// Embeds [op.ReceiptBase.Snapshot]'s wire-primitive triplet alongside the boundary and source URIs. Both `json:` and
// `yaml:` tags ride on every field so the same value flows through either encoder via [Receipt.MarshalJSON]'s
// delegation. boundary_uri and source_uri use an `omitempty` tag so receipts that don't need them emit a clean envelope.
//
// Returns:
//   - any: the populated anonymous struct for the YAML encoder to walk.
//   - error: nil under normal conditions.
func (r *Receipt) MarshalYAML() (any, error) {

	base := r.Snapshot()

	var boundaryURI string
	if r.boundary != nil {
		boundaryURI = r.boundary.URI()
	}

	var sourceURI string
	if r.source != nil {
		sourceURI = r.source.URI()
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
		Action         string `json:"action"                     yaml:"action"`
		ResourceURI    string `json:"resource_uri"               yaml:"resource_uri"`
		TransactionID  string `json:"transaction_id"             yaml:"transaction_id"`
		BoundaryURI    string `json:"boundary_uri,omitempty"     yaml:"boundary_uri,omitempty"`
		SourceURI      string `json:"source_uri,omitempty"       yaml:"source_uri,omitempty"`
		RecoveryID     string `json:"recovery_id,omitempty"      yaml:"recovery_id,omitempty"`
		RecoveryDigest string `json:"recovery_digest,omitempty"  yaml:"recovery_digest,omitempty"`
	}{
		Action:         base.Action,
		ResourceURI:    base.ResourceURI,
		TransactionID:  base.TransactionID,
		BoundaryURI:    boundaryURI,
		SourceURI:      sourceURI,
		RecoveryID:     recoveryID,
		RecoveryDigest: recoveryDigest,
	}, nil
}

// UnmarshalJSON decodes a JSON document produced by [Receipt.MarshalJSON] back into the receiver via
// [op.ReceiptBase.Restore].
//
// The receiver MUST be pre-seeded with an [op.RuntimeEnvironment]-bearing zero [Resource] so the unmarshaler can
// rehydrate the encoded URIs via [NewResource].
//
// Parameters:
//   - data: the JSON-encoded receipt bytes.
//
// Returns:
//   - error: any decode error, [NewResource] error, or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalJSON(data []byte) error {

	var aux struct {
		Action         string `json:"action"`
		ResourceURI    string `json:"resource_uri"`
		TransactionID  string `json:"transaction_id"`
		BoundaryURI    string `json:"boundary_uri"`
		SourceURI      string `json:"source_uri"`
		RecoveryID     string `json:"recovery_id"`
		RecoveryDigest string `json:"recovery_digest"`
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("file.Receipt: unmarshal JSON: %w", err)
	}

	return r.hydrate(aux.Action, aux.ResourceURI, aux.TransactionID, aux.BoundaryURI, aux.SourceURI, aux.RecoveryID, aux.RecoveryDigest)
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
//   - error: any decode error, [NewResource] error, or [op.ReceiptBase.Restore] failure.
func (r *Receipt) UnmarshalYAML(unmarshal func(any) error) error {

	var aux struct {
		Action         string `yaml:"action"`
		ResourceURI    string `yaml:"resource_uri"`
		TransactionID  string `yaml:"transaction_id"`
		BoundaryURI    string `yaml:"boundary_uri"`
		SourceURI      string `yaml:"source_uri"`
		RecoveryID     string `yaml:"recovery_id"`
		RecoveryDigest string `yaml:"recovery_digest"`
	}

	if err := unmarshal(&aux); err != nil {
		return fmt.Errorf("file.Receipt: unmarshal YAML: %w", err)
	}

	return r.hydrate(aux.Action, aux.ResourceURI, aux.TransactionID, aux.BoundaryURI, aux.SourceURI, aux.RecoveryID, aux.RecoveryDigest)
}

// hydrate reconstructs the receiver's embedded [op.ReceiptBase], boundary and source from the decoded envelope. The
// resources are pulled from the [op.ResourceCatalog] on the pre-seeded [op.RuntimeEnvironment] — existing entries are
// re-used (Resource identity is interned by URI); URIs not yet in the catalog are constructed via [NewResource] and
// registered through [op.ResourceCatalog.Link]. After resolution, the base is re-seated via [op.NewReceiptBase] (so
// [op.ReceiptBase.Restore]'s URI-match check has a live resource to compare against), the wire-primitive triplet is
// handed to Restore, and the boundary, source, recoveryID, and recoveryDigest are set when present.
//
// Parameters:
//   - action: the canonical action name from the decoded envelope.
//   - resourceURI: the resource's URI string from the decoded envelope.
//   - transactionID: the canonical UUIDv7 string from the decoded envelope.
//   - boundaryURI: the boundary's URI string from the decoded envelope; empty when the receipt records no boundary.
//   - sourceURI: the source's URI string from the decoded envelope; empty when the receipt records no source.
//   - recoveryID: the recovery archive UUID from the decoded envelope; empty when no archive was made.
//   - recoveryDigest: the canonical "<algo>:<hex>" form of the archived bytes' digest; empty when none was captured.
//
// Returns:
//   - error: a missing-context error, a missing-catalog error, a [NewResource] error, or an
//     [op.ReceiptBase.Restore] failure.
func (r *Receipt) hydrate(action, resourceURI, transactionID, boundaryURI, sourceURI string, recoveryID string, recoveryDigest string) error {

	existing := r.Resource()
	if existing == nil || existing.RuntimeEnvironment() == nil {
		return fmt.Errorf("file.Receipt: unmarshal requires RuntimeEnvironment on receiver")
	}

	runtimeEnvironment := existing.RuntimeEnvironment()
	if runtimeEnvironment.ResourceCatalog == nil {
		return fmt.Errorf("file.Receipt: unmarshal requires Catalog on RuntimeEnvironment")
	}

	// DiscoverResource handles construction + Catalog.Discover internally; no wrapping factory needed.
	resource, err := DiscoverResource(runtimeEnvironment, resourceURI)
	if err != nil {
		return fmt.Errorf("file.Receipt: rehydrate resource %q: %w", resourceURI, err)
	}

	r.ReceiptBase = op.NewReceiptBase(resource)

	if err := r.Restore(struct {
		Action        string `json:"action"         yaml:"action"`
		ResourceURI   string `json:"resource_uri"   yaml:"resource_uri"`
		TransactionID string `json:"transaction_id" yaml:"transaction_id"`
	}{
		Action:        action,
		ResourceURI:   resourceURI,
		TransactionID: transactionID,
	}); err != nil {
		return fmt.Errorf("file.Receipt: restore: %w", err)
	}

	if boundaryURI != "" {

		boundaryConcrete, err := DiscoverResource(runtimeEnvironment, boundaryURI)
		if err != nil {
			return fmt.Errorf("file.Receipt: rehydrate boundary %q: %w", boundaryURI, err)
		}

		r.boundary = boundaryConcrete
	}

	if sourceURI != "" {

		sourceConcrete, err := DiscoverResource(runtimeEnvironment, sourceURI)
		if err != nil {
			return fmt.Errorf("file.Receipt: rehydrate source %q: %w", sourceURI, err)
		}

		r.source = sourceConcrete
	}

	if recoveryID != "" {
		r.recoveryID, err = uuid.Parse(recoveryID)
		if err != nil {
			return fmt.Errorf("file.Receipt: parse recoveryID %q: %w", recoveryID, err)
		}
	}

	if recoveryDigest != "" {
		digest, err := op.ParseDigest(recoveryDigest)
		if err != nil {
			return fmt.Errorf("file.Receipt: parse recoveryDigest %q: %w", recoveryDigest, err)
		}
		r.recoveryDigest = digest
	}

	return nil
}
