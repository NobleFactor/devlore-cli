// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Receipt acknowledges a compensable forward method call and carries the minimum state a reversal needs.
//
// Every compensable forward method returns a Receipt alongside its [Product]. The Receipt carries the affected
// [Resource] (Resource()), the moment the call was issued (Timestamp()), and an opaque identifier for
// correlating the forward call with its eventual reversal (TransactionID()). Provider-specific receipts
// (e.g., file.Receipt) must embed [ReceiptBase] to satisfy this interface. The unexported receiptBase method
// seals the interface to receiverTypes that embed [ReceiptBase].
type Receipt interface {
	Action() string
	Commit(actionName string) error
	Resource() Resource
	Timestamp() uuid.Time
	TransactionID() string
	receiptBase()
}

// ReceiptBase holds the resource affected by a compensable forward method call.
//
// The transactionID both correlates the forward call with its reversal and encodes the moment the call was issued.
//
// ReceiverType-specific receipts (e.g., file.Tombstone) must embed it by value. The embedded Resource
// preserves its true identity — its fields are never modified by the recovery system. The transactionID is
// a UUIDv7: its first 48 bits are the Unix-millisecond timestamp, making it both unique and time-sortable,
// and making [ReceiptBase.Timestamp] a pure bit-extract over the stored ID — no parsing, no heap allocation. The
// transactionID is stored in its 16-byte binary form to avoid the ~60 bytes and per-access parse cost of the
// string form; [ReceiptBase.TransactionID] formats on demand at serialization or display boundaries.
//
// For providers that also need an archive-storage key (e.g., file.Tombstone, which archives displaced bytes
// to [RecoverySite]), the transactionID doubles as the recovery key. Per-domain aliases (e.g.,
// file.Tombstone.RecoveryID) expose it under the domain-appropriate name.
type ReceiptBase struct {
	action        string
	resource      Resource
	transactionID uuid.UUID
}

// NewReceiptBase creates an uninflated ReceiptBase anchored to the given resource.
//
// The transactionID and action remain zero-valued until [ReceiptBase.Inflate] is called. This split lets a
// provider method bind the affected resource at construction and defer the per-call reflection + UUID work
// until inflation, when the issuing method is known.
//
// Parameters:
//   - resource: the resource affected by the compensable forward method call.
//
// Returns:
//   - ReceiptBase: the constructed base with only resource populated.
func NewReceiptBase(resource Resource) ReceiptBase {
	return ReceiptBase{resource: resource}
}

// region EXPORTED METHODS

// region State management

// Action returns the canonical action name of the method that issued this receipt.
//
// The value has the form <pkg-path>.<receiverName>.<methodName> and is cached at [ReceiptBase.Inflate]
// (read from [Method.ActionName]). No per-call reflect traversal or allocation.
//
// Returns:
//   - string: the canonical action name; empty until Inflate runs.
func (b *ReceiptBase) Action() string {
	return b.action
}

// Resource returns the resource affected by the compensable forward method call.
//
// Returns:
//   - Resource: the affected resource set at [NewReceiptBase] (or replaced at [ReceiptBase.Inflate]).
func (b *ReceiptBase) Resource() Resource {
	return b.resource
}

// Timestamp returns the timestamp encoded in this receipt's transactionID as a [uuid.Time] — a count of
// 100-nanosecond intervals since the UUID epoch (1582-10-15 UTC).
//
// For UUIDv7, the value corresponds to the 48-bit Unix-millisecond timestamp encoded in the first 48 bits. Use
// [uuid.Time.UnixTime] to project to seconds + nanoseconds suitable for [time.Unix].
//
// Returns:
//   - uuid.Time: the encoded issue time; zero until Inflate runs.
func (b *ReceiptBase) Timestamp() uuid.Time {
	return b.transactionID.Time()
}

// TransactionID returns the receipt's transactionID as a canonical 36-char UUID string.
//
// The transactionID is a UUIDv7 minted at [ReceiptBase.Inflate]; it correlates the forward call with its
// reversal and encodes the call's issue time (see [ReceiptBase.Timestamp]). The string is produced on demand
// via [uuid.UUID.String] — the receipt stores only the 16-byte binary form.
//
// Returns:
//   - string: the canonical UUID string; the all-zeros UUID until Inflate runs.
func (b *ReceiptBase) TransactionID() string {
	return b.transactionID.String()
}

// endregion

// region Behaviors

// Commit finalizes this receipt by minting its TransactionID and stamping the supplied action name.
//
// Idempotent: if the transactionID is already set, Commit is a no-op and returns nil. Commit fails only if
// [uuid.NewV7] fails; no resource or context lookup is required.
//
// Parameters:
//   - actionName: the canonical action name in fully-qualified form
//     (<pkg-path>.<receiverName>.<methodName>) — typically supplied by [RecoveryStack.Push] from the issuing
//     [Action.FullName] or by [Method.Invoke] from the method's own [Method.ActionName].
//
// Returns:
//   - error: non-nil when [uuid.NewV7] fails.
func (b *ReceiptBase) Commit(actionName string) error {

	if b.transactionID != (uuid.UUID{}) {
		return nil
	}

	tid, err := uuid.NewV7()
	if err != nil {
		return err
	}

	b.transactionID = tid
	b.action = actionName

	return nil
}

// MarshalJSON encodes the receipt's base state as JSON: action, resource, transaction_id.
//
// Delegates to [ReceiptBase.MarshalYAML] for the wire-shape value, then runs [json.Marshal] over it. The
// anonymous struct returned by MarshalYAML carries both `json:` and `yaml:` field tags, so the JSON encoder
// reads its tags directly. Concrete Receipt types with no provider-specific fields inherit this method
// unchanged via embedding; types that carry extra fields override both [ReceiptBase.MarshalJSON] and
// [ReceiptBase.MarshalYAML] together because Go method dispatch on an embedded receiver does not see the
// outer type's overrides.
//
// Returns:
//   - []byte: JSON-encoded object with keys "action", "resource", "transaction_id".
//   - error: any error from [ReceiptBase.MarshalYAML] or from [json.Marshal] (including from the embedded
//     Resource's own marshaler).
func (b *ReceiptBase) MarshalJSON() ([]byte, error) {

	v, err := b.MarshalYAML()
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// MarshalYAML returns the receipt's base state as an anonymous struct value the YAML encoder serializes.
//
// The returned struct is the single source of truth for the receipt's wire shape: its `json:` and `yaml:`
// field tags drive both encoders, and [ReceiptBase.MarshalJSON] delegates here for the value before running
// [json.Marshal]. Resource serializes via the concrete Resource type's own [yaml.Marshaler]; TransactionID
// serializes as the canonical 36-char UUIDv7 string already produced by [ReceiptBase.TransactionID].
//
// Returns:
//   - any: the populated anonymous struct for the YAML encoder to walk.
//   - error: nil under normal conditions.
func (b *ReceiptBase) MarshalYAML() (any, error) {

	return struct {
		Action        string   `json:"action"         yaml:"action"`
		Resource      Resource `json:"resource"       yaml:"resource"`
		TransactionID string   `json:"transaction_id" yaml:"transaction_id"`
	}{
		Action:        b.action,
		Resource:      b.resource,
		TransactionID: b.transactionID.String(),
	}, nil
}

// Restore rebuilds this receipt's base state from a snapshot.
//
// [Snapshot] and Restore form the only encapsulation-respecting path to read or write the embedded fields from outside
// [op]. A derivative
//
// - unmarshals into its own anonymous struct,
// - resolves the resource URI through the [ResourceCatalog] (per the invariant that every URI is round-trippable),
// - converts the transactionID string to a [uuid.UUID],
// - packs the trio into a snapshot, and
// - hands it to Restore.
//
// Direct field mutation is impossible. This method is the derivative's only legitimate hook into the base. Restore is
// one-shot: it errors if the receipt has already been committed or restored. Callers that need to re-bind a receipt
// construct a fresh one.
//
// Parameters:
//   - snapshot: the base-state snapshot, identical in shape to the value returned by [Snapshot]. Anonymous
//     struct typing prevents callers from forging one outside the Snapshot/[Restore] boundary.
//
// Returns:
//   - error: non-nil when the receipt's transactionID is already set (Restore on a committed receipt).
func (b *ReceiptBase) Restore(snapshot struct {
	Action        string
	Resource      Resource
	TransactionID uuid.UUID
}) error {

	if b.transactionID != (uuid.UUID{}) {
		return fmt.Errorf("restore failed: transaction ID already set")
	}

	b.action = snapshot.Action
	b.resource = snapshot.Resource
	b.transactionID = snapshot.TransactionID

	return nil
}

// Snapshot returns this receipt's base state as an anonymous struct.
//
// Snapshot is the read side of the encapsulation boundary. It pairs with [Restore]. Derivative types use Snapshot to
// lift the base state into their own marshaler: pull the trio out, convert Resource to its URI string and TransactionID
// to its canonical UUID string, and emit alongside the derivative's provider-specific fields. The returned type is
// intentionally anonymous — no caller can construct one directly; only Snapshot can produce it, and only Restore can
// consume the matching shape.
//
// Returns:
//   - struct: a snapshot of the receipt's base state — Action, Resource (the live catalog entry), and
//     TransactionID (the 16-byte UUIDv7).
func (b *ReceiptBase) Snapshot() struct {
	Action        string
	Resource      Resource
	TransactionID uuid.UUID
} {
	return struct {
		Action        string
		Resource      Resource
		TransactionID uuid.UUID
	}{
		Action:        b.action,
		Resource:      b.resource,
		TransactionID: b.transactionID,
	}
}

// endregion

// region UNEXPORTED METHODS

// receiptBase seals the [Receipt] interface to types that embed [ReceiptBase].
//
// This method has no function body and is never called directly. Its sole purpose is to make [Receipt] unimplementable
// from outside this package, so every value satisfying [Receipt] is guaranteed to carry the action / resource /
// transactionID state that the recovery machinery (commit, archive-key derivation, timestamp extraction) reads through
// [ReceiptBase].
func (b *ReceiptBase) receiptBase() {}

// endregion
