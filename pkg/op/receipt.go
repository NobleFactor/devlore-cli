// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Receipt acknowledges a compensable forward method call and carries the minimum state a reversal needs.
//
// Every compensable forward method returns a Receipt alongside its [Product]. The Receipt carries the affected
// [Resource] (Resource()), the moment the call was issued (Timestamp()), and an opaque identifier for correlating the
// forward call with its eventual reversal (TransactionID()). Provider-specific receipts (e.g., file.Receipt) must embed
// [ReceiptBase] to satisfy this interface. The unexported receiptBase method seals the interface to receiverTypes that
// embed [ReceiptBase].
type Receipt interface {
	receiptBase() *ReceiptBase

	// State management

	// Action returns the short, human-facing action name (e.g. "file.link") captured at dispatch.
	//
	// Empty for structural-subgraph audit receipts (no bound action). [Trace.Summarize] keys its per-action tally on
	// this label, so a trace can be summarized without consulting the graph.
	Action() string

	// ActionPath returns the canonical action name of the method that issued this receipt.
	//
	// The value has the form <pkg-path>.<receiverName>.<methodName> and is cached at [Commit].
	ActionPath() string

	// Attempts returns the per-attempt history for retried dispatches. Empty when the dispatch completed on the first
	// attempt.
	Attempts() []Attempt

	// Complement returns the per-call recovery state captured by a compensable forward method, or nil for
	// non-compensable dispatches and for compensable dispatches with no undo state.
	Complement() any

	// Err returns the dispatch error, or nil on success.
	Err() error

	// IsCommitted reports whether this receipt has been finalized with a TransactionID.
	//
	// A committed receipt is ready for archival and reversal. Receipts returned from forward methods are uncommitted;
	// they are committed by the orchestration engine (via [RecoveryStack.Push]) once the forward call succeeds.
	IsCommitted() bool

	// Annotations returns the dispatching unit's annotation map, captured whole at [Commit]. The framework is
	// key-agnostic; tools read their own keys (e.g. writ "project"/"layer", lore "package").
	Annotations() AnnotationMap

	// Resource returns the resource affected by the compensable forward method call, or nil for non-resource-producing
	// dispatches.
	Resource() Resource

	// Result returns the dispatch's return value, or nil for void methods, action-error methods, and failed dispatches.
	Result() any

	// Slots returns the resolved slot values at dispatch time — the audit snapshot of "what inputs did this dispatch
	// see."
	Slots() map[string]any

	// Timestamp returns the moment the call was issued as a [uuid.Time].
	//
	// The timestamp is encoded within the [TransactionID] and becomes available once the receipt is committed.
	Timestamp() uuid.Time

	// TransactionID returns the unique identifier for correlating the forward call with its reversal.
	//
	// The identifier is a UUIDv7 minted at [Commit].
	TransactionID() string

	// UnitID returns the [ExecutableUnit.ID] of the unit that dispatched.
	UnitID() string

	// Mutators (executor stamps audit fields at dispatch exit)

	SetAttempts(attempts []Attempt)
	SetSlots(slots map[string]any)

	// Behaviors

	// Commit finalizes this receipt by minting its TransactionID and stamping the supplied action name.
	//
	// Idempotent: if the receipt is already committed, Commit is a no-op.
	Commit(unit ExecutableUnit, result any, complement any, err error) error
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
	actionPath    string
	annotations   AnnotationMap
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
// method bind the affected resource at construction and defer the per-call reflection + UUID work until inflation, when
// the issuing method is known.
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

// Action returns the short action name (e.g. "file.link") captured at dispatch, or empty for structural-subgraph audit
// receipts.
//
// Returns:
//   - `string`: the short action name; empty when no bound action issued this receipt.
func (b *ReceiptBase) Action() string {
	return b.action
}

// ActionPath returns the canonical action name of the method that issued this receipt.
//
// The value has the form <pkg-path>.<receiverName>.<methodName> and is cached at [ReceiptBase.Inflate] read from
// [Method.ActionName]. No per-call reflect traversal or allocation.
//
// Returns:
//   - `string`: the canonical action name; empty until Inflate runs.
func (b *ReceiptBase) ActionPath() string {
	return b.actionPath
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

// Err returns the dispatch error, or nil on success.
//
// Returns:
//   - `error`: the dispatch error, or nil when the dispatch succeeded.
func (b *ReceiptBase) Err() error {

	return b.err
}

// IsCommitted reports whether this receipt has been finalized with a TransactionID.
//
// Returns:
//   - `bool`: true if the transactionID is not the nil UUID.
func (b *ReceiptBase) IsCommitted() bool {

	return b.transactionID != uuid.Nil
}

// Annotations returns the dispatching unit's annotation map, captured whole at [ReceiptBase.Commit].
//
// The framework is key-agnostic: it carries the map without interpreting it. Tools read their own keys
// (writ "project"/"layer", lore "package") via [AnnotationMap.Get].
//
// Returns:
//   - AnnotationMap: the captured annotations; the zero value for units with none.
func (b *ReceiptBase) Annotations() AnnotationMap {

	return b.annotations
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

// Slots returns the resolved slot values at dispatch time — the audit snapshot of "what inputs did this dispatch see."
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
//   - `string`: the dispatching unit's ID.
func (b *ReceiptBase) UnitID() string {

	return b.unitID
}

// Timestamp returns the timestamp encoded in this receipt's transactionID as a [uuid.Time].
//
// This is a count of 100-nanosecond intervals since the UUID epoch (1582-10-15 UTC). The value corresponds to the
// 48-bit Unix-millisecond timestamp encoded in the first 48 bits. Use [uuid.Time.UnixTime] to project to seconds plus
// nanoseconds suitable for [time.Unix].
//
// Returns:
//   - uuid.Time: the encoded issue time; zero until [ReceiptBase.Commit] runs.
func (b *ReceiptBase) Timestamp() uuid.Time {

	return b.transactionID.Time()
}

// TransactionID returns the receipt's transactionID as a canonical 36-char UUID string.
//
// The transactionID is a UUIDv7 minted at [ReceiptBase.Inflate]; it correlates the forward call with its reversal and
// encodes the call's issue time (see [ReceiptBase.Timestamp]). The string is produced on demand via [uuid.UUID.String]
// — the receipt stores only the 16-byte binary form.
//
// Returns:
//   - `string`: the canonical UUID string; the all-zeros UUID until Inflate runs.
func (b *ReceiptBase) TransactionID() string {

	return b.transactionID.String()
}

// endregion

// region Behaviors

// Commit finalizes the receipt by minting its TransactionID and recording info on the action that committed the result.
//
// Idempotent: if the transactionID is already set, Commit is a no-op and returns nil. Commit fails only if [uuid.NewV7]
// fails; no resource or context lookup is required.
//
// A nil `unit` is valid: immediate-mode dispatch has no graph and no unit to stamp, so the unit-identity fields are
// left zero (an honest "no issuing unit") while the transactionID, result, complement, and error are still recorded.
//
// Parameters:
//   - `unit`: the executable unit whose dispatch produced the result; nil in immediate mode.
//   - `result`: the unit's return value.
//   - `complement`: the reversal artifact (Receipt or RecoveryStack) paired with the forward call.
//   - `err`: the error returned by the forward call, if any.
//
// Returns:
//   - `error`: non-nil when [uuid.NewV7] fails.
func (b *ReceiptBase) Commit(unit ExecutableUnit, result any, complement any, err error) error {

	if b.transactionID != (uuid.UUID{}) {
		return nil
	}

	tid, tidErr := uuid.NewV7()
	if tidErr != nil {
		return tidErr
	}

	b.transactionID = tid

	// A nil unit is valid: immediate-mode dispatch has no graph and no unit to stamp. Leave the unit-identity fields
	// zero rather than dereferencing a nil unit — the transactionID, result, complement, and error below are still
	// recorded, so the receipt stays honest about having had no issuing unit.
	if unit != nil {
		b.unitID = unit.ID()
		b.action = unit.Action().Name()
		b.actionPath = unit.Action().FullName()

		b.annotations = unit.Annotations()
	}

	b.result = result
	b.complement = complement
	b.err = err

	return nil
}

// MarshalJSON encodes the receipt's base state as JSON via the [ReceiptData] shape.
//
// Delegates to [ReceiptBase.MarshalYAML] for the encoded value, then runs [json.Marshal] over it. [ReceiptData]
// carries both `json:` and `yaml:` field tags so the JSON encoder reads its tags directly. Concrete Receipt types with
// no provider-specific fields inherit this method unchanged via embedding; types that carry extra fields override both
// [ReceiptBase.MarshalJSON] and [ReceiptBase.MarshalYAML] together because Go method dispatch on an embedded receiver
// does not see the outer type's overrides.
//
// Returns:
//   - []byte: JSON-encoded object with the [ReceiptData] fields.
//   - `error`: any error from [ReceiptBase.MarshalYAML] or from [json.Marshal].
func (b *ReceiptBase) MarshalJSON() ([]byte, error) {

	v, err := b.MarshalYAML()
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// MarshalYAML returns the receipt's base state as a [ReceiptData] value the YAML encoder serializes.
//
// Per phase-8 13.0(d), Resource is projected to its [Resource.URI] string on the wire — not embedded as a full Resource
// document — so the envelope stays flat and the Unmarshal side can rehydrate the concrete Resource via each
// derivative's NewResource without nested-decoder context plumbing. TransactionID serializes as the canonical 36-char
// UUIDv7 string already produced by [ReceiptBase.TransactionID]. Err round-trips as a `status` string (the error
// message); empty restores as nil. [ReceiptData] is the single source of truth for the wire shape: its `json:` and
// `yaml:` tags drive both encoders, and [ReceiptBase.MarshalJSON] delegates here for the value before running
// [json.Marshal].
//
// Returns:
//   - `any`: the populated [ReceiptData] for the YAML encoder to walk.
//   - `error`: nil under normal conditions.
func (b *ReceiptBase) MarshalYAML() (any, error) {

	return b.Snapshot(), nil
}

// Restore rebuilds this receipt's base state from a [ReceiptData].
//
// [Snapshot] and Restore form the encapsulation-respecting path to read or write the embedded base state from outside
// [op]. Concrete receipt types in other packages embed [ReceiptData] in their own wire-shape struct, extract the
// embedded value during unmarshal, and pass it here. The boundary conversions ([Resource] -> URI, UUID -> 36-char
// string, error -> status string) run at the Snapshot / Restore boundary so downstream encoders see plain field values
// and skip reflection-driven method dispatch on the embedded base.
//
// The receiver MUST be pre-seeded with a [Resource] before Restore is called — typically by reconstructing the
// receipt's base via [NewReceiptBase] with a freshly-built concrete Resource. Restore validates that the pre-seeded
// resource's URI matches snapshot.ResourceURI (sanity check against malformed wire input), parses the transaction ID,
// then writes every base field from the snapshot. The Resource itself is not mutated — its identity was fixed at
// construction.
//
// Restore is one-shot: it errors if the receipt has already been committed or restored. Callers that need to re-bind a
// receipt construct a fresh one.
//
// Parameters:
//   - `snapshot`: the base-state [ReceiptData], identical in shape to the value returned by [Snapshot].
//
// Returns:
//   - `error`: non-nil when the receipt's transactionID is already set, the resource is missing, the
//     resource URI does not match the snapshot, or the transaction_id string is malformed.
func (b *ReceiptBase) Restore(snapshot ReceiptData) error {

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
	b.actionPath = snapshot.ActionPath
	b.annotations = NewAnnotationMap(snapshot.Annotations)
	b.attempts = snapshot.Attempts
	b.complement = snapshot.Complement
	b.result = snapshot.Result
	b.slots = snapshot.Slots
	if snapshot.Status != "" {
		b.err = errors.New(snapshot.Status)
	}
	b.transactionID = tid
	b.unitID = snapshot.UnitID

	return nil
}

// Snapshot returns this receipt's base state as a [ReceiptData].
//
// Snapshot is the read side of the encapsulation boundary. Marshalers can return Snapshot's value directly or embed it
// alongside derivative-specific fields — concrete receipt types in other packages compose [ReceiptData] with their
// provider fields in a single wire-shape struct. The boundary conversions ([Resource] -> URI, UUID -> 36-char string,
// error -> status string) run once here, so downstream encoders see plain field values and skip reflection-driven
// method dispatch on the embedded base.
//
// Returns:
//   - ReceiptData: the receipt's base state with ResourceURI empty when no resource is attached, TransactionID the
//     canonical 36-char UUID string (the all-zeros UUID until Commit runs), and Status the dispatch error's message
//     (empty when Err is nil).
func (b *ReceiptBase) Snapshot() ReceiptData {

	var resourceURI string
	if b.resource != nil {
		resourceURI = b.resource.URI()
	}

	var status string
	if b.err != nil {
		status = b.err.Error()
	}

	return ReceiptData{
		Action:        b.action,
		ActionPath:    b.actionPath,
		Annotations:   b.annotations.values,
		Attempts:      b.attempts,
		Complement:    b.complement,
		ResourceURI:   resourceURI,
		Result:        b.result,
		Slots:         b.slots,
		Status:        status,
		TransactionID: b.transactionID.String(),
		UnitID:        b.unitID,
	}
}

// endregion

// region UNEXPORTED METHODS

// receiptBase seals the [Receipt] interface to types that embed [ReceiptBase] and exposes that embedded base.
//
// Returning the embedded [*ReceiptBase] makes [Receipt] unimplementable from outside this package, so every value
// satisfying [Receipt] is guaranteed to carry the action / resource / transactionID state that the recovery machinery
// (commit, archive-key derivation, timestamp extraction) reads through [ReceiptBase]. The returned pointer also gives
// in-package callers uniform access to that base through the interface, matching [Provider.providerBase] and
// [Resource.resourceBase].
//
// Returns:
//   - *ReceiptBase: the embedded base.
func (b *ReceiptBase) receiptBase() *ReceiptBase { return b }

// endregion

// region SUPPORTING TYPES

// ReceiptData is the canonical wire shape for [ReceiptBase].
//
// [ReceiptBase.Snapshot] and [ReceiptBase.Restore] form the encapsulation-respecting path to read or write the base
// state from outside [op]. Concrete receipt types in other packages embed ReceiptData in their own wire-shape
// struct (combining base and provider-specific fields) and pass the embedded value to [ReceiptBase.Restore] during
// unmarshal. The named type avoids the verbose anonymous-struct repetition a 12-field shape would otherwise demand at
// every call site.
//
// Field-level encoding choices:
//   - Status holds the dispatch error's message; non-empty restores as errors.New(status) so Err()-presence and the
//     human-readable reason survive the wire trip (typed/joined errors collapse into a single error on reload).
//   - Slots / Result / Complement serialize as their natural YAML; on reload they are untyped (map[string]any or
//     primitive) and the framework's Convert cascade retypes them where a typed value is needed (compensation,
//     promise resolution).
//   - ResourceURI carries the resource's identity; the receiver must be pre-seeded with the concrete Resource before
//     Restore is called (Restore validates that URIs match).
type ReceiptData struct {
	Action        string         `json:"action"                 yaml:"action"`
	ActionPath    string         `json:"action_path"            yaml:"action_path"`
	Annotations   map[string]any `json:"annotations,omitempty"  yaml:"annotations,omitempty"`
	Attempts      []Attempt      `json:"attempts,omitempty"     yaml:"attempts,omitempty"`
	Complement    any            `json:"complement,omitempty"   yaml:"complement,omitempty"`
	ResourceURI   string         `json:"resource_uri"           yaml:"resource_uri"`
	Result        any            `json:"result,omitempty"       yaml:"result,omitempty"`
	Slots         map[string]any `json:"slots,omitempty"        yaml:"slots,omitempty"`
	Status        string         `json:"status,omitempty"       yaml:"status,omitempty"`
	TransactionID string         `json:"transaction_id"         yaml:"transaction_id"`
	UnitID        string         `json:"unit_id"                yaml:"unit_id"`
}

// endregion
