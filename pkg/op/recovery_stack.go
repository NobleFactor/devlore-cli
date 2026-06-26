// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// RecoveryStack accumulates compensable operations in LIFO order.
//
// On Unwind, each entry is compensated in reverse order. All entries are attempted regardless of individual failures;
// errors are joined via errors.Join.
//
// Per-subgraph executors own one stack each, chained into a tree (phase-8 step 28). A child stack is nested *down*
// into its parent via [RecoveryStack.PushNested], so Unwind cascades compensation through the tree; and it points
// *up* at its parent via the `parent` field, so [RecoveryStack.ResultByUnitID] walks the chain to resolve a promise
// against an ancestor stack's receipt. The nesting is durable (serialized in a [Trace]); the parent pointer is
// transient — re-derived from the nesting on load, never serialized.
type RecoveryStack struct {

	// entries is the LIFO list of compensable and audit entries pushed onto this stack.
	entries []recoveryEntry

	// parent is the enclosing subgraph's stack, or nil at the root of the chain. [RecoveryStack.ResultByUnitID] walks
	// up through it for promise resolution. Never serialized; re-derived from the nesting on [Trace] load.
	parent *RecoveryStack
}

// recoveryEntry captures one entry on a [RecoveryStack].
//
// Two kinds of entries exist:
//
//   - Receipt-bearing: receipt is non-nil; compensate is pre-bound by [RecoveryStack.pushReceipt] to invoke the
//     action's Compensate companion at unwind time. Persistable via [RecoveryStack.MarshalJSON].
//
//   - Nested: recoveryStack is non-nil; compensate runs sub.Unwind() as a transactional unit. Persistable.
type recoveryEntry struct {
	recoveryStack *RecoveryStack  // nested entries; nil otherwise
	receipt       Receipt         // receipt-bearing entries; nil otherwise
	compensate    func(any) error // pre-bound undo (receipt.Resource for receipt entries; recoveryStack for nested)
	restore       *receiptRestore // decoded envelope retained at load for a resource receipt; reconstructed at re-arm
}

// receiptRestore retains a resource receipt's codec-decoded envelope between load and resume re-arm.
//
// At load there is no runtime environment to resolve a receipt's id references, so the stack keeps the decoded envelope
// — the base execution state plus the provider's id-reference sub-field — and [RecoveryStack.rearm] reconstructs the
// concrete receipt from it once the catalog is rehydrated. Both halves are format-neutral: whichever codec read the
// trace produced the [ReceiptData] and the `map[string]any`, so reconstruction never re-parses format-specific bytes.
type receiptRestore struct {
	base   ReceiptData
	fields map[string]any
}

// NewRecoveryStack creates an empty RecoveryStack at the root of a chain (no parent).
//
// Returns:
//   - `*RecoveryStack`: the new root stack.
func NewRecoveryStack() *RecoveryStack {
	return newRecoveryStack(nil)
}

// newRecoveryStack creates an empty RecoveryStack chained to `parent`.
//
// [RecoveryStack.ResultByUnitID] walks up through `parent` to resolve a promise against an ancestor stack's receipt; a
// nil `parent` marks the root of the chain.
//
// Parameters:
//   - `parent`: the enclosing subgraph's stack, or nil for the root.
//
// Returns:
//   - `*RecoveryStack`: the new chained stack.
func newRecoveryStack(parent *RecoveryStack) *RecoveryStack {
	return &RecoveryStack{parent: parent}
}

// Push appends a [Receipt] onto the stack as an audit-trail entry.
//
// Step 12 broadens [RecoveryStack] from a compensable-only ledger to an every-dispatch ledger: the executor calls Push
// at every dispatch exit (cancellation, Do-error, success). When the receipt carries a non-nil complement, the entry is
// also wired for compensation — [Unwind] invokes the action's Compensate companion at rollback, reached through
// `runtimeEnvironment` rather than the receipt's resource (so a resource-less complement still compensates). Otherwise,
// the entry is audit-only and [Unwind] skips it.
//
// The receipt is committed (idempotently) using its already-stamped action name. Receipts without a stamped action name
// skip commit; their TransactionID stays empty until a later [Receipt.Commit] runs.
//
// Parameters:
//   - `receipt`: the receipt to push. Must be non-nil.
//   - `runtimeEnvironment`: the executor's environment, used to resolve and invoke the Compensate companion at unwind.
//
// Returns:
//   - `error`: non-nil if receipt is nil or commit fails.
func (s *RecoveryStack) Push(receipt Receipt, runtimeEnvironment *RuntimeEnvironment) error {

	if receipt == nil {
		return errors.New("RecoveryStack.Push: receipt is nil")
	}

	entry := recoveryEntry{receipt: receipt}

	// Compensation binding: a receipt is compensable iff it carries a non-nil complement — the per-call undo state, of
	// any shape (a resource action's receipt, a recovery stack, or a slice of stacks). The env comes from the executor,
	// not the receipt's resource, so a resource-less complement (a combinator or file.WalkTree stack) still compensates
	// via its Undo companion. Audit-only entries (no complement) leave compensate nil; Unwind walks past them.

	if receipt.Complement() != nil {
		entry.compensate = func(_ any) error { return invokeCompensateForReceipt(runtimeEnvironment, receipt) }
	}

	s.entries = append(s.entries, entry)
	return nil
}

// PushNested appends a substack as a single transactional entry on this stack.
//
// The nested entry preserves the saga boundary: at unwind time the substack is unwound as a unit (its own LIFO walk,
// its own error aggregation) before the outer stack continues.
//
// A nil sub is treated as an empty saga and contributes nothing.
func (s *RecoveryStack) PushNested(recoveryStack *RecoveryStack) {

	if recoveryStack == nil {
		return
	}

	compensate := func(_ any) error { return recoveryStack.Unwind() }

	s.entries = append(s.entries, recoveryEntry{
		recoveryStack: recoveryStack,
		compensate:    compensate,
	})
}

// Unwind rolls back all stack entries in LIFO order.
//
// All entries are attempted regardless of individual failures. Errors are joined via errors.Join.
func (s *RecoveryStack) Unwind() error {

	var errs []error

	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		if entry.compensate != nil {
			if err := entry.compensate(nil); err != nil {
				errs = append(errs, err)
			}
		}
	}

	s.entries = nil
	return errors.Join(errs...)
}

// rearm reconstructs concrete receipts and binds their compensation closures after a resume rehydrates the ledger.
//
// At load a resource receipt is a bare [ReceiptBase] (no env to resolve its ids); this walks the restored tree and, for
// each entry that retained its encoded envelope, reconstructs the concrete receipt via [Receipt.RestoreEncoded] against
// the now-rehydrated catalog and binds its compensate closure so a resumed-then-failed unwind rolls it back. A subgraph
// receipt keeps its bare base (its complement is the reconstructed child stack) and recurses.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment, its catalog already rehydrated.
//
// Returns:
//   - `error`: a reconstruction failure (unresolved id, unknown action, malformed envelope).
func (s *RecoveryStack) rearm(runtimeEnvironment *RuntimeEnvironment) error {

	for i := range s.entries {
		entry := &s.entries[i]

		if entry.recoveryStack != nil {
			if err := entry.recoveryStack.rearm(runtimeEnvironment); err != nil {
				return err
			}
			continue
		}

		if entry.receipt == nil {
			continue
		}

		if entry.restore != nil {
			restore := entry.restore
			concrete, err := reconstructReceipt(runtimeEnvironment, restore.base.Action, restore.base, restore.fields)
			if err != nil {
				return err
			}

			// A resource receipt is its own complement: the compensable forward method returns (result, complement,
			// error) with the receipt as the complement, and Commit stores that self-reference. Reconstruction has no
			// forward call, so reinstate the identity here unless the receipt restored a complement of its own.
			if concrete.Complement() == nil {
				concrete.receiptBase().complement = concrete
			}

			entry.receipt = concrete
			entry.restore = nil
		}

		if err := retypeResult(runtimeEnvironment, entry.receipt); err != nil {
			return err
		}

		if entry.receipt.Complement() != nil {
			receipt := entry.receipt
			entry.compensate = func(_ any) error { return invokeCompensateForReceipt(runtimeEnvironment, receipt) }
		}

		if childStack, ok := entry.receipt.Complement().(*RecoveryStack); ok {
			if err := childStack.rearm(runtimeEnvironment); err != nil {
				return err
			}
		}
	}

	return nil
}

// Discard drops all entries without unwinding.
func (s *RecoveryStack) Discard() {
	s.entries = nil
}

// ResultByUnitID returns the most recent receipt's [Receipt.Result] for the unit identified by `unitID`, searching this
// stack and then walking up the parent chain.
//
// The stack tree is the source of truth for per-dispatch results: every dispatch exit pushes a receipt with the
// producing unit's ID and result, so promise-style "look up an upstream unit's output" queries walk the stacks instead
// of a separate results map. Each stack is searched LIFO so a retried unit returns its latest outcome; when the unit is
// not found, the search continues into the stack's `parent`, so a promise to an upstream producer in an ancestor
// subgraph resolves against that ancestor's stack.
//
// The walk only ever goes *up* the chain, never *down* into nested substacks (a producer always runs before its
// consumer, and so lives in this stack or an ancestor, never in a child).
//
// Parameters:
//   - `unitID`: the [ExecutableUnit.ID] of the producing unit.
//
// Returns:
//   - `any`: the matched receipt's result, or nil when no match is found in this stack or any ancestor.
//   - `bool`: true when a matching receipt was found, false otherwise.
func (s *RecoveryStack) ResultByUnitID(unitID string) (any, bool) {

	for stack := s; stack != nil; stack = stack.parent {
		for i := len(stack.entries) - 1; i >= 0; i-- {
			r := stack.entries[i].receipt
			if r != nil && r.UnitID() == unitID {
				return r.Result(), true
			}
		}
	}

	return nil, false
}

// receiptByUnitID returns the receipt for `unitID` from this stack's own entries (not the parent chain), searched LIFO.
//
// Resume reads it against the stack a unit is handed to decide the unit's fate: a receipt with a nil error marks a
// completed unit to replay; an ErrPaused receipt marks an in-progress subgraph to re-enter. The lookup stays on this
// stack — a unit's own receipt lives on its own stack, never an ancestor's — unlike [RecoveryStack.ResultByUnitID].
//
// Parameters:
//   - `unitID`: the [ExecutableUnit.ID] to look up.
//
// Returns:
//   - `Receipt`: the matching receipt, or nil when none is found on this stack.
//   - `bool`: true when a receipt for `unitID` is present.
func (s *RecoveryStack) receiptByUnitID(unitID string) (Receipt, bool) {

	for i := len(s.entries) - 1; i >= 0; i-- {
		r := s.entries[i].receipt
		if r != nil && r.UnitID() == unitID {
			return r, true
		}
	}

	return nil, false
}

// supersede removes the top-most entry whose receipt is for `unitID`, dropping it from this stack.
//
// Resume calls this when an in-progress subgraph re-enters: its stale ErrPaused receipt is removed before the subgraph
// re-dispatches, so the fresh completion receipt replaces it rather than leaving a duplicate on the stack.
//
// Parameters:
//   - `unitID`: the [ExecutableUnit.ID] whose entry to remove.
func (s *RecoveryStack) supersede(unitID string) {

	for i := len(s.entries) - 1; i >= 0; i-- {
		r := s.entries[i].receipt
		if r != nil && r.UnitID() == unitID {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return
		}
	}
}

// Receipts returns every receipt-bearing entry on this stack, descending into nested substacks, in push
// order (oldest first).
//
// Unlike [RecoveryStack.ResultByUnitID] — which searches only this stack's top level — Receipts flattens nested
// substacks so callers that summarize a whole execution (see [Trace.Summarize]) observe every dispatched unit's
// receipt, including per-iteration combinator children. Nested-stack marker entries contribute their contained
// receipts, not themselves; and a receipt whose complement is itself a [RecoveryStack] — a subgraph or file.WalkTree
// dispatch — also contributes that child stack's receipts, since the child no longer rides a separate nested entry.
//
// Returns:
//   - []Receipt: the flattened receipts in push order; empty when the stack holds none.
func (s *RecoveryStack) Receipts() []Receipt {

	var receipts []Receipt
	for _, entry := range s.entries {
		switch {
		case entry.receipt != nil:
			receipts = append(receipts, entry.receipt)
			if childStack, ok := entry.receipt.Complement().(*RecoveryStack); ok {
				receipts = append(receipts, childStack.Receipts()...)
			}
		case entry.recoveryStack != nil:
			receipts = append(receipts, entry.recoveryStack.Receipts()...)
		}
	}
	return receipts
}

// Len returns the number of entries on the stack.
func (s *RecoveryStack) Len() int {
	return len(s.entries)
}

// MarshalJSON encodes the stack's entries as a JSON object.
//
// Encoded form: `{"entries": [...]}` where each element is either a receipt envelope or a nested substack (`{"sub":
// {...}}`) — disjoint field sets, no `kind` tag. The receipt envelope (see [receiptEnvelope]) is the stack-owned record
// of a dispatch's execution state, read off the [Receipt] interface, so a reloaded stack carries every unit's id,
// result, status, and child-stack complement regardless of which concrete receipt produced it. A `*RecoveryStack`
// complement encodes recursively via this same method.
func (s *RecoveryStack) MarshalJSON() ([]byte, error) {

	v, err := s.MarshalYAML()
	if err != nil {
		return nil, err
	}

	return json.Marshal(v)
}

// receiptEnvelope is the stack-owned, provider-agnostic encoding of one receipt's execution state.
//
// The recovery stack — not each provider's receipt — owns this, so a new or maintained provider cannot forget to encode
// the fields resume needs. It reads them off the [Receipt] interface ([Receipt.UnitID], [Receipt.Result], etc.), so a
// reloaded receipt restores enough to be skipped (a successful unit replays its result), adopted (a subgraph's
// `*RecoveryStack` complement is reconstructed), and summarized — independent of the concrete receipt's own encoding.
type receiptEnvelope struct {
	UnitID     string         `json:"unit_id"              yaml:"unit_id"`
	Action     string         `json:"action,omitempty"     yaml:"action,omitempty"`
	Result     any            `json:"result,omitempty"      yaml:"result,omitempty"`
	ResultType string         `json:"result_type,omitempty" yaml:"result_type,omitempty"`
	Status     string         `json:"status,omitempty"      yaml:"status,omitempty"`
	Complement *RecoveryStack `json:"complement,omitempty"  yaml:"complement,omitempty"`
	Receipt    any            `json:"receipt,omitempty"     yaml:"receipt,omitempty"`
}

// MarshalYAML returns the stack's entries as anonymous struct values the encoder walks.
//
// Source of truth for the encoded shape; [RecoveryStack.MarshalJSON] delegates here. Each receipt entry becomes a
// [receiptEnvelope] carrying its execution state; a child-stack complement encodes recursively, while a resource
// receipt's compensation state rides a `receipt` sub-field carrying the receipt's own id-based encoding (see
// file.Receipt.MarshalYAML).
func (s *RecoveryStack) MarshalYAML() (any, error) {

	entries := make([]any, 0, len(s.entries))

	for _, e := range s.entries {
		switch {
		case e.recoveryStack != nil:
			entries = append(entries, struct {
				Sub *RecoveryStack `json:"sub" yaml:"sub"`
			}{Sub: e.recoveryStack})
		case e.receipt != nil:
			envelope := receiptEnvelope{
				UnitID:     e.receipt.UnitID(),
				Action:     e.receipt.Action(),
				Result:     e.receipt.Result(),
				ResultType: e.receipt.ResultType(),
				Status:     errStatus(e.receipt.Err()),
			}
			if childStack, ok := e.receipt.Complement().(*RecoveryStack); ok {
				envelope.Complement = childStack
			} else if complement, isReceipt := e.receipt.Complement().(Receipt); isReceipt && complement == e.receipt {
				// A single-resource receipt is its own complement (the forward method returns it as the complement);
				// its id-based encoding rides the `receipt` sub-field (see file.Receipt.MarshalYAML), reconstructed
				// against the rehydrated ledger at resume. The other legal complement shapes are not reconstructed
				// here: a []Receipt (e.g. pkg.Install) is a follow-up — carrying no sub-field, it resumes without that
				// receipt's compensation rather than failing; a *RecoveryStack rides the `complement` field above.
				envelope.Receipt = e.receipt
			}
			entries = append(entries, envelope)
		}
	}

	return struct {
		Entries []any `json:"entries" yaml:"entries"`
	}{Entries: entries}, nil
}

// recoveryEntryData is the codec-decoded shape of one stack entry — a nested substack or a receipt envelope.
//
// [RecoveryStack.UnmarshalJSON] and [RecoveryStack.UnmarshalYAML] decode a slice of these (each through its own codec),
// then [RecoveryStack.fromEntries] builds the live entries. A `sub` marks a nested stack; otherwise the base execution
// state rides the envelope fields and a resource receipt's id references ride `receipt` as a format-neutral map the
// receipt resolves at re-arm. The nested `*RecoveryStack` fields decode recursively through the same two unmarshalers,
// so the whole tree reconstructs in whichever format the trace was stored.
type recoveryEntryData struct {
	Sub        *RecoveryStack `json:"sub,omitempty"        yaml:"sub,omitempty"`
	UnitID     string         `json:"unit_id,omitempty"    yaml:"unit_id,omitempty"`
	Action     string         `json:"action,omitempty"     yaml:"action,omitempty"`
	Result     any            `json:"result,omitempty"      yaml:"result,omitempty"`
	ResultType string         `json:"result_type,omitempty" yaml:"result_type,omitempty"`
	Status     string         `json:"status,omitempty"      yaml:"status,omitempty"`
	Complement *RecoveryStack `json:"complement,omitempty"  yaml:"complement,omitempty"`
	Receipt    map[string]any `json:"receipt,omitempty"     yaml:"receipt,omitempty"`
}

// UnmarshalJSON reconstructs the stack tree from the JSON form encoded by [RecoveryStack.MarshalJSON].
//
// It decodes the entries into [recoveryEntryData] and delegates to [RecoveryStack.fromEntries], which the YAML reader
// ([RecoveryStack.UnmarshalYAML]) shares — so JSON and YAML reconstruct identically. Reconstruction consumes the
// decoded values, never re-parsed bytes, per the format-neutral requirement (a trace must reload and verify across
// JSON/YAML/Protobuf — see the step doc's "Format-neutral trace reconstruction" section).
//
// Parameters:
//   - `data`: the JSON produced by [RecoveryStack.MarshalJSON].
//
// Returns:
//   - `error`: non-nil on malformed input.
func (s *RecoveryStack) UnmarshalJSON(data []byte) error {

	var encoded struct {
		Entries []recoveryEntryData `json:"entries"`
	}
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}

	return s.fromEntries(encoded.Entries)
}

// UnmarshalYAML reconstructs the stack tree from the YAML form encoded by [RecoveryStack.MarshalYAML].
//
// The YAML mirror of [RecoveryStack.UnmarshalJSON]: it decodes the entries into [recoveryEntryData] through the YAML
// codec and delegates to the shared [RecoveryStack.fromEntries] builder, so a YAML-stored trace reconstructs through
// the same format-neutral path as a JSON one.
//
// Parameters:
//   - `unmarshal`: the YAML node decoder supplied by the yaml package.
//
// Returns:
//   - `error`: non-nil on malformed input.
func (s *RecoveryStack) UnmarshalYAML(unmarshal func(any) error) error {

	var encoded struct {
		Entries []recoveryEntryData `yaml:"entries"`
	}
	if err := unmarshal(&encoded); err != nil {
		return err
	}

	return s.fromEntries(encoded.Entries)
}

// fromEntries builds the live recovery entries from their codec-decoded [recoveryEntryData].
//
// A `Sub` entry becomes a nested substack. Otherwise the envelope's base execution state seeds a bare [ReceiptBase] (no
// environment exists at load to resolve ids), and a present `receipt` sub-field is retained as a [receiptRestore] so
// [RecoveryStack.rearm] reconstructs the concrete receipt against the rehydrated catalog at resume. That is enough for
// resume to skip, adopt, and summarize; a resource receipt's own undo state is restored at re-arm, not here.
//
// Parameters:
//   - `entries`: the decoded entries, in stack order.
//
// Returns:
//   - `error`: non-nil when a bare receipt's base restore fails.
func (s *RecoveryStack) fromEntries(entries []recoveryEntryData) error {

	s.entries = make([]recoveryEntry, 0, len(entries))

	for _, e := range entries {

		if e.Sub != nil {
			s.entries = append(s.entries, recoveryEntry{recoveryStack: e.Sub})
			continue
		}

		base := ReceiptData{
			UnitID:     e.UnitID,
			Action:     e.Action,
			ActionPath: e.Action,
			Result:     e.Result,
			ResultType: e.ResultType,
			Status:     e.Status,
		}
		if e.Complement != nil {
			base.Complement = e.Complement
		}

		receipt := &ReceiptBase{}
		if err := receipt.RestoreEncoded(nil, base, nil); err != nil {
			return err
		}

		entry := recoveryEntry{receipt: receipt}
		if len(e.Receipt) > 0 {
			entry.restore = &receiptRestore{base: base, fields: e.Receipt}
		}

		s.entries = append(s.entries, entry)
	}

	return nil
}

// errStatus returns an error's message, or "" for a nil error — the encoded form of a receipt's [Receipt.Err].
func errStatus(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// invokeCompensateForReceipt resolves a receipt's [Compensate] companion via the registry and invokes it.
//
// Used by [RecoveryStack.Push]'s pre-bound compensation closure at [RecoveryStack.Unwind] time. The env is supplied by
// the executor — not read off the receipt's resource — so a resource-less complement (a combinator or file.WalkTree
// recovery stack) still resolves its companion. [ReceiverRegistry.ActionByPath] looks up the action by its committed
// Go-qualified name; a receipt that recorded the dotted name instead (a unit that bound its action by name — the graph
// root, every combinator) falls back to [RuntimeEnvironment.ActionByName]. [Method.Undo] then dispatches to the
// [Compensate] companion with [Receipt.Complement] as the undo state.
//
// [ErrNotCompensable] from the companion is treated as a success (logged elsewhere; not surfaced as an error).
//
// Parameters:
//   - `runtimeEnvironment`: the executor's environment; resolves the [ReceiverRegistry] provider for the action.
//   - `receipt`: the audit receipt whose [Receipt.Complement] is the undo state handed to the companion.
//
// Returns:
//   - `error`: non-nil when the env is nil, the action is unregistered, the provider fails, or the companion fails.
func invokeCompensateForReceipt(runtimeEnvironment *RuntimeEnvironment, receipt Receipt) error {

	if runtimeEnvironment == nil {
		return fmt.Errorf("invokeCompensateForReceipt: receipt %s has no runtime environment", receipt.ActionPath())
	}

	providerReceiverType, method, ok := ReceiverRegistry().ActionByPath(receipt.ActionPath())

	if !ok {
		// A unit that binds its action by name (the graph root and every combinator) records the dotted action name —
		// e.g. "flow.subgraph" — as its action path, not the Go-qualified ActionName that ActionByPath keys on. Resolve
		// the dotted name through the environment's action resolver (the same one dispatch uses) and retry on the
		// resolved Go-qualified path.
		resolved, resolveErr := runtimeEnvironment.ActionByName(receipt.ActionPath())
		if resolveErr == nil && resolved != nil {
			providerReceiverType, method, ok = ReceiverRegistry().ActionByPath(resolved.FullName())
		}
	}

	if !ok {
		return fmt.Errorf("invokeCompensateForReceipt: no registered action %q", receipt.ActionPath())
	}

	provider, err := runtimeEnvironment.cachedProvider(providerReceiverType)
	if err != nil {
		return fmt.Errorf("invokeCompensateForReceipt: cache provider %q: %w", providerReceiverType.Name(), err)
	}

	activationRecord := &ActivationRecord{RuntimeEnvironment: runtimeEnvironment, Context: runtimeEnvironment.Context}

	if undoErr := method.Undo(activationRecord, provider, receipt.Complement()); undoErr != nil {
		if errors.Is(undoErr, ErrNotCompensable) {
			return nil
		}
		return undoErr
	}

	return nil
}

// retypeResult retypes a receipt's reloaded (untyped) result to its produced Go type, restoring full type fidelity.
//
// The produced type id was recorded at [ReceiptBase.Commit] (see [canonicalID]); it is authoritative even when a
// combinator's static return is `any`. Resolution goes through [receiverRegistry.ProductTypeByID] and the value through
// the [Convert] cascade (a struct hydrates from its map, a plain resource resolves through the rehydrated catalog). The
// produced-type-id is scoped to struct/scalar/resource: a result it cannot reconstruct — notably a content-addressable
// observation, which round-trips by re-observe-and-verify, not reconstruction — is left as-is, as are a nil result and
// an empty or unknown id, rather than failing the resume.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment, forwarded to [Convert] for env-sensitive types (resources).
//   - `receipt`: the receipt whose result is retyped in place.
//
// Returns:
//   - `error`: reserved; currently always nil — a retype that does not apply leaves the value as-is.
func retypeResult(runtimeEnvironment *RuntimeEnvironment, receipt Receipt) error {

	result := receipt.Result()
	if result == nil {
		return nil
	}

	productType, ok := ReceiverRegistry().ProductTypeByID(receipt.ResultType())
	if !ok {
		return nil
	}

	retyped, err := Convert(runtimeEnvironment, result, productType)
	if err != nil {
		// The produced-type-id is scoped to struct/scalar/resource; a value it cannot reconstruct -- notably a
		// content-addressable observation, which round-trips by re-observe-and-verify, not reconstruction -- is left
		// as-is rather than failing the resume. A consumer that needs the concrete type fails at its own dispatch.
		return nil
	}

	receipt.receiptBase().result = retyped
	return nil
}

// reconstructReceipt rebuilds the concrete receipt for an action from its codec-decoded envelope.
//
// The concrete type is read off the action's Compensate companion (the same companion compensation resolves at unwind),
// so `op` instantiates the right receipt without importing the provider or consulting a registry — the type comes from
// the action name every codec carries, which is what keeps reconstruction format-neutral (no type-URL registry needed).
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment.
//   - `action`: the receipt's dotted action name.
//   - `base`: the codec-decoded base execution state.
//   - `fields`: the receipt's id-reference sub-field, decoded to a format-neutral map.
//
// Returns:
//   - `Receipt`: the reconstructed concrete receipt.
//   - `error`: an unknown action, a non-Receipt companion parameter, or a [Receipt.RestoreEncoded] failure.
func reconstructReceipt(runtimeEnvironment *RuntimeEnvironment, action string, base ReceiptData,
	fields map[string]any) (Receipt, error) {

	receiptType, err := receiptTypeForAction(runtimeEnvironment, action)
	if err != nil {
		return nil, err
	}

	if receiptType.Kind() != reflect.Pointer {
		return nil, fmt.Errorf("reconstructReceipt: action %q companion parameter %s is not a pointer", action, receiptType)
	}

	receipt, ok := reflect.New(receiptType.Elem()).Interface().(Receipt)
	if !ok {
		return nil, fmt.Errorf("reconstructReceipt: action %q reconstructs %s, which is not a Receipt", action, receiptType)
	}

	if err := receipt.RestoreEncoded(runtimeEnvironment, base, fields); err != nil {
		return nil, err
	}

	return receipt, nil
}

// receiptTypeForAction returns the concrete receipt type the action's Compensate companion declares.
//
// Resolution mirrors [invokeCompensateForReceipt]: [receiverRegistry.ActionByPath] for a Go-qualified name, falling
// back to [RuntimeEnvironment.ActionByName] for a dotted name. The companion's last parameter is the receipt type.
//
// Parameters:
//   - `runtimeEnvironment`: the resume environment (for the dotted-name fallback).
//   - `action`: the receipt's action name.
//
// Returns:
//   - `reflect.Type`: the companion's receipt parameter type (a pointer type).
//   - `error`: an unregistered action or an action with no compensation companion.
func receiptTypeForAction(runtimeEnvironment *RuntimeEnvironment, action string) (reflect.Type, error) {

	_, method, ok := ReceiverRegistry().ActionByPath(action)
	if !ok {
		resolved, resolveErr := runtimeEnvironment.ActionByName(action)
		if resolveErr == nil && resolved != nil {
			_, method, ok = ReceiverRegistry().ActionByPath(resolved.FullName())
		}
	}
	if !ok {
		return nil, fmt.Errorf("receiptTypeForAction: no registered action %q", action)
	}

	complementType, ok := method.complementType()
	if !ok {
		return nil, fmt.Errorf("receiptTypeForAction: action %q has no compensation companion", action)
	}

	return complementType, nil
}
