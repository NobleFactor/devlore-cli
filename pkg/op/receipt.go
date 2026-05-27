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
// [Resource] (Resource()), the moment the call was issued (Timestamp()), and an opaque identifier for correlating
// the forward call with its eventual reversal (TransactionID()). Provider-specific receipts (e.g., file.Receipt)
// must embed [ReceiptBase] to satisfy this interface. The unexported receiptBase method seals the interface to
// receiverTypes that embed [ReceiptBase].
type Receipt interface {
	receiptBase()

	// State management

	// Action returns the canonical action name of the method that issued this receipt.
	//
	// The value has the form <pkg-path>.<receiverName>.<methodName> and is cached at [Commit].
	Action() string

	// Attempts returns the per-attempt history for retried dispatches. Empty when the dispatch
	// completed on the first attempt.
	Attempts() []Attempt

	// Complement returns the per-call recovery state captured by a compensable forward method, or
	// nil for non-compensable dispatches and for compensable dispatches with no undo state.
	Complement() any

	// Err returns the dispatch error, or nil on success.
	Err() error

	// IsCommitted reports whether this receipt has been finalized with a TransactionID.
	//
	// A committed receipt is ready for archival and reversal. Receipts returned from forward methods are
	// uncommitted; they are committed by the orchestration engine (via [RecoveryStack.Push]) once the
	// forward call succeeds.
	IsCommitted() bool

	// Resource returns the resource affected by the compensable forward method call, or nil for
	// non-resource-producing dispatches.
	Resource() Resource

	// Result returns the dispatch's return value, or nil for void methods, action-error methods, and
	// failed dispatches.
	Result() any

	// Slots returns the resolved slot values at dispatch time — the audit snapshot of "what inputs
	// did this dispatch see."
	Slots() map[string]any

	// Timestamp returns the moment the call was issued as a [uuid.Time].
	//
	// The timestamp is encoded within the [TransactionID] and becomes available once the receipt
	// is committed.
	Timestamp() uuid.Time

	// TransactionID returns the unique identifier for correlating the forward call with its reversal.
	//
	// The identifier is a UUIDv7 minted at [Commit].
	TransactionID() string

	// UnitID returns the [ExecutableUnit.ID] of the unit that dispatched.
	UnitID() string

	// Mutators (executor stamps audit fields at dispatch exit)

	SetAttempts(attempts []Attempt)
	SetComplement(complement any)
	SetErr(err error)
	SetResult(result any)
	SetSlots(slots map[string]any)
	SetUnitID(unitID string)

	// Behaviors

	// Commit finalizes this receipt by minting its TransactionID and stamping the supplied action name.
	//
	// Idempotent: if the receipt is already committed, Commit is a no-op.
	Commit(actionName string) error
}

// ReceiptBase holds the resource affected by a compensable forward method call.
//
// The transactionID both correlates the forward call with its reversal and encodes the moment the call was issued.
//
// ReceiverType-specific receipts (e.g., file.Receipt) must embed it by value. The embedded Resource preserves its true
// identity — its fields are never modified by the recovery system. The transactionID is a UUIDv7: its first 48 bits are
// the Unix-millisecond timestamp, making it both unique and time-sortable, and making [ReceiptBase.Timestamp] a pure
// bit-extract over the stored ID — no parsing, no heap allocation. The transactionID is stored in its 16-byte binary
// form to avoid the ~60 bytes and per-access parse cost of the string form; [ReceiptBase.TransactionID] formats on
// demand at serialization or display boundaries.
//
// For providers that also need an archive-storage key (e.g., file.Receipt, which archives displaced bytes to
// [RecoverySite]), the transactionID doubles as the recovery key — [RecoverySite] interprets the receipt's
// TransactionID directly; no per-domain alias is needed.
type ReceiptBase struct {
	action        string
	attempts      []Attempt
	complement    any
	err           error
	resource      Resource
	result        any
	slots         map[string]any
	transactionID uuid.UUID
	unitID        string
}

// NewReceiptBase creates an uninflated ReceiptBase anchored to the given resource.
//
// The transactionID and action remain zero-valued until [ReceiptBase.Inflate] is called. This split lets a provider
// method bind the affected resource at construction and defer the per-call reflection + UUID work until inflation,
// when the issuing method is known.
//
// Parameters:
//   - `resource`: the resource affected by the compensable forward method call.
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
//   - `string`: the canonical action name; empty until Inflate runs.
func (b *ReceiptBase) Action() string {
	return b.action
}

// Attempts returns the per-attempt history for retried dispatches.
//
// Empty when the dispatch completed on the first attempt.
//
// Returns:
//   - []Attempt: the per-attempt history, or nil when no retries occurred.
func (b *ReceiptBase) Attempts() []Attempt {

	return b.attempts
}

// SetAttempts replaces the per-attempt history with `attempts`.
//
// Parameters:
//   - `attempts`: the per-attempt history to stamp on the receipt.
func (b *ReceiptBase) SetAttempts(attempts []Attempt) {

	b.attempts = attempts
}

// Complement returns the per-call recovery state captured by a compensable forward method.
//
// Returns nil for non-compensable dispatches and for compensable dispatches with no undo state.
//
// Returns:
//   - `any`: the recovery state, or nil when none was captured.
func (b *ReceiptBase) Complement() any {

	return b.complement
}

// SetComplement stamps the per-call recovery state `complement` on the receipt.
//
// Parameters:
//   - `complement`: the recovery state captured by the compensable forward method.
func (b *ReceiptBase) SetComplement(complement any) {

	b.complement = complement
}

// Err returns the dispatch error, or nil on success.
//
// Returns:
//   - `error`: the dispatch error, or nil when the dispatch succeeded.
func (b *ReceiptBase) Err() error {

	return b.err
}

// SetErr stamps the dispatch error `err` on the receipt.
//
// Parameters:
//   - `err`: the dispatch error to stamp (nil on success).
func (b *ReceiptBase) SetErr(err error) {

	b.err = err
}

// IsCommitted reports whether this receipt has been finalized with a TransactionID.
//
// Returns:
//   - `bool`: true if the transactionID is not the nil UUID.
func (b *ReceiptBase) IsCommitted() bool {
	return b.transactionID != uuid.Nil
}

// Resource returns the resource affected by the compensable forward method call, or nil for
// non-resource-producing dispatches.
//
// Returns:
//   - `Resource`: the affected resource set at [NewReceiptBase] (or nil when none).
func (b *ReceiptBase) Resource() Resource {
	return b.resource
}

// Result returns the dispatch's return value.
//
// Returns nil for void methods, action-error methods, and failed dispatches.
//
// Returns:
//   - `any`: the dispatch's return value, or nil when the method returned nothing or failed.
func (b *ReceiptBase) Result() any {

	return b.result
}

// SetResult stamps the dispatch's return value `result` on the receipt.
//
// Parameters:
//   - `result`: the dispatch's return value (nil for void methods, action-error methods, and failed
//     dispatches).
func (b *ReceiptBase) SetResult(result any) {

	b.result = result
}

// Slots returns the resolved slot values at dispatch time — the audit snapshot of "what inputs did
// this dispatch see."
//
// Returns:
//   - map[string]any: the resolved slot snapshot keyed by parameter name.
func (b *ReceiptBase) Slots() map[string]any {

	return b.slots
}

// SetSlots stamps the resolved slot snapshot `slots` on the receipt.
//
// Parameters:
//   - `slots`: the resolved slot values keyed by parameter name.
func (b *ReceiptBase) SetSlots(slots map[string]any) {

	b.slots = slots
}

// UnitID returns the [ExecutableUnit.ID] of the unit that dispatched.
//
// Returns:
//   - `string`: the dispatching unit's ID; empty until [SetUnitID] runs.
func (b *ReceiptBase) UnitID() string {

	return b.unitID
}

// SetUnitID stamps the dispatching unit's ID `unitID` on the receipt.
//
// Parameters:
//   - `unitID`: the [ExecutableUnit.ID] of the unit that dispatched.
func (b *ReceiptBase) SetUnitID(unitID string) {

	b.unitID = unitID
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
//   - `string`: the canonical UUID string; the all-zeros UUID until Inflate runs.
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
//   - `actionName`: the canonical action name in fully-qualified form
//     (<pkg-path>.<receiverName>.<methodName>) — typically supplied by [RecoveryStack.Push] from the issuing
//     [Action.FullName] or by [Method.Invoke] from the method's own [Method.ActionName].
//
// Returns:
//   - `error`: non-nil when [uuid.NewV7] fails.
func (b *ReceiptBase) Commit(actionName string) error {

	if b.action == "" {
		b.action = actionName
	}

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

// MarshalJSON encodes the receipt's base envelope as JSON: action, resource_uri, transaction_id.
//
// Delegates to [ReceiptBase.MarshalYAML] for the encoded value, then runs [json.Marshal] over it. The anonymous
// struct returned by MarshalYAML carries both `json:` and `yaml:` field tags so the JSON encoder reads its tags
// directly. Concrete Receipt types with no provider-specific fields inherit this method unchanged via embedding;
// types that carry extra fields override both [ReceiptBase.MarshalJSON] and [ReceiptBase.MarshalYAML] together
// because Go method dispatch on an embedded receiver does not see the outer type's overrides.
//
// Returns:
//   - []byte: JSON-encoded object with keys "action", "resource_uri", "transaction_id".
//   - `error`: any error from [ReceiptBase.MarshalYAML] or from [json.Marshal].
func (b *ReceiptBase) MarshalJSON() ([]byte, error) {

	v, err := b.MarshalYAML()
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// MarshalYAML returns the receipt's base envelope as an anonymous struct value the YAML encoder serializes.
//
// Per phase-8 13.0(d), Resource is projected to its [Resource.URI] string on the wire — not embedded as a full
// Resource document — so the envelope stays flat and the Unmarshal side can rehydrate the concrete Resource via
// each derivative's NewResource without nested-decoder context plumbing. TransactionID serializes as the canonical
// 36-char UUIDv7 string already produced by [ReceiptBase.TransactionID]. The returned anonymous struct is the
// single source of truth for the wire shape: its `json:` and `yaml:` tags drive both encoders, and
// [ReceiptBase.MarshalJSON] delegates here for the value before running [json.Marshal].
//
// Returns:
//   - `any`: the populated anonymous struct for the YAML encoder to walk.
//   - `error`: nil under normal conditions.
func (b *ReceiptBase) MarshalYAML() (any, error) {
	return b.Snapshot(), nil
}

// Restore rebuilds this receipt's base state from a snapshot of wire-primitive strings.
//
// [Snapshot] and Restore form the only encapsulation-respecting path to read or write the embedded base
// state from outside [op]. The shape is the wire shape — three strings, ready to embed in a marshaler's
// anonymous struct or to read straight from a decoder's. Per the project serializer fast-path pattern,
// the wire-primitive form lets downstream encoders skip reflect-based method dispatch entirely.
//
// The receiver MUST be pre-seeded with a [Resource] before Restore is called — typically by reconstructing
// the receipt's base via [NewReceiptBase] with a freshly-built concrete Resource. Restore validates that
// the pre-seeded resource's URI matches snapshot.ResourceURI (sanity check against malformed wire input),
// then writes Action and TransactionID. The Resource itself is not mutated by Restore — its identity was
// fixed at construction.
//
// Restore is one-shot: it errors if the receipt has already been committed or restored. Callers that need
// to re-bind a receipt construct a fresh one.
//
// Parameters:
//   - `snapshot`: the base-state snapshot, identical in shape to the value returned by [Snapshot]. Anonymous
//     struct typing prevents callers from forging one outside the Snapshot/Restore boundary.
//
// Returns:
//   - `error`: non-nil when the receipt's transactionID is already set, the resource is missing, the
//     resource URI does not match the snapshot, or the transaction_id string is malformed.
func (b *ReceiptBase) Restore(snapshot struct {
	Action        string `json:"action"         yaml:"action"`
	ResourceURI   string `json:"resource_uri"   yaml:"resource_uri"`
	TransactionID string `json:"transaction_id" yaml:"transaction_id"`
}) error {

	if b.transactionID != (uuid.UUID{}) {
		return fmt.Errorf("restore failed: transaction ID already set")
	}

	if b.resource == nil {
		return fmt.Errorf("restore failed: resource must be pre-seeded before Restore")
	}

	if b.resource.URI() != snapshot.ResourceURI {
		return fmt.Errorf("restore failed: pre-seeded resource URI %q does not match snapshot URI %q",
			b.resource.URI(), snapshot.ResourceURI)
	}

	tid, err := uuid.Parse(snapshot.TransactionID)
	if err != nil {
		return fmt.Errorf("restore failed: parse transaction_id %q: %w", snapshot.TransactionID, err)
	}

	b.action = snapshot.Action
	b.transactionID = tid

	return nil
}

// Snapshot returns this receipt's base state as an anonymous struct of wire-primitive strings.
//
// Snapshot is the read side of the encapsulation boundary. Marshalers can return Snapshot's value directly
// (or embed it alongside derivative-specific fields) — the encoder sees a struct of strings and hits its
// fast path with no reflect-based method dispatch on the fields. Per the project serializer fast-path
// pattern, the conversion work (UUID → canonical 36-char string, [Resource] → [Resource.URI]) runs once
// here at the boundary instead of repeatedly inside the encoder's reflection machinery.
//
// The returned type is intentionally anonymous — no caller can construct one directly; only Snapshot can
// produce it, and only Restore can consume the matching shape.
//
// Returns:
//   - struct: a snapshot of the receipt's base state — Action, ResourceURI (empty when no resource is
//     attached), and TransactionID (canonical 36-char UUID string; the all-zeros UUID until Commit runs).
func (b *ReceiptBase) Snapshot() struct {
	Action        string `json:"action"         yaml:"action"`
	ResourceURI   string `json:"resource_uri"   yaml:"resource_uri"`
	TransactionID string `json:"transaction_id" yaml:"transaction_id"`
} {
	var resourceURI string
	if b.resource != nil {
		resourceURI = b.resource.URI()
	}

	return struct {
		Action        string `json:"action"         yaml:"action"`
		ResourceURI   string `json:"resource_uri"   yaml:"resource_uri"`
		TransactionID string `json:"transaction_id" yaml:"transaction_id"`
	}{
		Action:        b.action,
		ResourceURI:   resourceURI,
		TransactionID: b.transactionID.String(),
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
