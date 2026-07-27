// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Trace is the serializable projection of a [*GraphExecutor]'s per-run mutable state.
//
// Trace pairs with a [*Graph] (loaded separately via [LoadGraph]) to fully describe an execution
// that can be resumed: the graph carries the immutable plan; the trace carries the [RunStatus] triplet at
// the moment of capture, the [*RecoveryStack] of dispatch receipts (audit + compensation), and the
// resolved variable map. The [Trace.GraphChecksum] identifies which graph the trace was taken
// against; a future resume constructor compares it against the loaded graph's checksum to refuse
// stale traces.
//
// A trace whose [RunStatus.Phase] is [PhasePaused] is resumable. A trace in a terminal phase ([PhaseCompleted] or
// [PhaseStopped]) is for archival — restoring such a trace reconstructs the same terminal triplet, not a runnable
// executor. The compensation-failure contract (phase-8 step 21) makes the stopped × [ConditionCompensationFailed] trace
// a restartable journal: the framework retains the failed unwind's stack (the source plus each receipt's
// `compensation_error`) so the journal survives (landed 2026-07-13); the state-checked resume *from* such a trace — a
// re-query-and-unwind, not a forward retry — is not yet built.
type Trace struct {

	// GraphChecksum is the canonical "sha256:<hex>" identity of the graph this trace was taken
	// against. Required for resume to refuse mismatched graphs.
	GraphChecksum string `json:"graph_checksum" yaml:"graph_checksum"`

	// RunStatus is the executor's [RunStatus] (phase × condition × reason) at the moment the trace was taken.
	RunStatus RunStatus `json:"run_status" yaml:"run_status"`

	// Transitions is the run's flips-only transition journal — one [RunStatusTransition] per recorded change of the
	// [Phase] or [Condition] dimension, in order, written by [GraphExecutor.Transition]. [Trace.RunStatus]
	// is the O(1) answer; this journal answers when and where each flip happened.
	Transitions []RunStatusTransition `json:"transitions,omitempty" yaml:"transitions,omitempty"`

	// Stack is the recovery stack of per-dispatch receipts (audit + compensation entries).
	Stack *RecoveryStack `json:"stack"           yaml:"stack"`

	// Variables is the resolved variable map at the time of the trace.
	Variables map[string]Variable `json:"variables,omitempty" yaml:"variables,omitempty"`

	// Catalog is the serialized resource ledger — every generation keyed by id — captured at Run teardown for
	// every outcome. Resume rehydrates a paused run's ledger into the live [ResourceCatalog] and resolves the
	// recovery stack's receipt id references against it; a completed run's ledger carries the recorded
	// content-identity pair (Etag/Digest, phase-8 step 48) drift attribution reads back.
	Catalog *ResourceLedgerSnapshot `json:"catalog,omitempty" yaml:"catalog,omitempty"`

	// Checksum is the trace's own tier-1 integrity hash — [GitStyleChecksum]("trace", canonical) over
	// [Trace.CanonicalContent] — stamped at persist by the store's WriteTrace and recomputed and compared by
	// [LoadTrace]. Excluded (with Signature) from the canonical bytes, so integrity and authenticity verify
	// independently. See docs/architecture/5-receipt-integrity.md § Document Integrity.
	Checksum string `json:"checksum,omitempty" yaml:"checksum,omitempty"`

	// Signature is the trace's publisher signature, or nil when unsigned (phase-8 step 46). The raw signature
	// covers `devlore.trace.v1 ‖ CanonicalContent`; the store's [WriteTrace] signs at persist, and
	// [Trace.CanonicalContent] excludes this field so verification round-trips.
	Signature *Signature `json:"signature,omitempty" yaml:"signature,omitempty"`
}

// CanonicalContent returns the trace's canonical bytes: its YAML document form without the integrity fields.
//
// The canonical bytes are what the trace's checksum and signature both cover (the signer prefixes the
// devlore.trace.v1 namespace — phase-8 step 46, mirroring [Graph.CanonicalContent]). Unlike the graph — whose
// canonical serialization is hand-built and round-trip-stable — the trace's live form holds typed values (receipt
// results, catalog resources) that are not a marshal fixed point with their decoded document forms. Canonical
// is therefore defined over the DOCUMENT form: marshal, decode generically, strip `checksum` and `signature`,
// re-marshal — which produces identical bytes from a live trace and from a decoded document (yaml key ordering
// is stable), so one checksum and one signature verify on both sides.
//
// Returns:
//   - `[]byte`: the canonical YAML bytes.
//   - `error`: non-nil if a marshaling step fails.
func (t *Trace) CanonicalContent() ([]byte, error) {

	document, err := yaml.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("op.Trace.CanonicalContent: %w", err)
	}

	canonical, err := canonicalTraceBytes(document)
	if err != nil {
		return nil, fmt.Errorf("op.Trace.CanonicalContent: %w", err)
	}
	return canonical, nil
}

// canonicalTraceBytes canonicalizes a trace document's bytes: generic decode, strip the integrity fields, re-marshal.
//
// Both integrity checks derive their input through this one helper. [Trace.CanonicalContent] feeds it the live
// trace's marshaled bytes; [LoadTrace] feeds it the raw document bytes as read. Canonicalizing the DOCUMENT (not a
// decoded-then-re-marshaled typed trace) is what makes stamp and verify agree: a decoded trace's typed values
// (receipt results, catalog resources) do not re-marshal to byte-identical form, but the generic map of the same
// document does.
//
// Parameters:
//   - `document`: the trace document's YAML bytes.
//
// Returns:
//   - `[]byte`: the canonical YAML bytes, integrity fields stripped.
//   - `error`: non-nil if a marshaling step fails.
func canonicalTraceBytes(document []byte) ([]byte, error) {

	var generic map[string]any
	if err := yaml.Unmarshal(document, &generic); err != nil {
		return nil, err
	}
	delete(generic, "checksum")
	delete(generic, "signature")

	return yaml.Marshal(generic)
}

// SignWith signs the trace through `sign`, setting the signature exactly once.
//
// The seam keeps pkg/op crypto-free, mirroring [Graph.SignWith]: this method supplies the canonical bytes and
// stores the result; the signer (pkg/signing) owns the ciphersuite and key custody.
//
// Parameters:
//   - `sign`: computes the [*Signature] over the canonical bytes (the signer prefixes its namespace).
//
// Returns:
//   - `error`: non-nil when the trace is already signed, canonicalization fails, or `sign` fails.
func (t *Trace) SignWith(sign func(canonical []byte) (*Signature, error)) error {

	if t.Signature != nil {
		return fmt.Errorf("op.Trace.SignWith: trace %s is already signed", t.GraphChecksum)
	}

	canonical, err := t.CanonicalContent()
	if err != nil {
		return fmt.Errorf("op.Trace.SignWith: %w", err)
	}

	signature, err := sign(canonical)
	if err != nil {
		return fmt.Errorf("op.Trace.SignWith: %w", err)
	}

	t.Signature = signature
	return nil
}

// StampChecksum computes and sets the trace's tier-1 checksum over its canonical bytes.
//
// Idempotent: the canonical bytes exclude the checksum field, so restamping recomputes the same value. The
// store's WriteTrace stamps at persist; [LoadTrace] recomputes and compares.
//
// Returns:
//   - `error`: non-nil when canonicalization fails.
func (t *Trace) StampChecksum() error {

	canonical, err := t.CanonicalContent()
	if err != nil {
		return fmt.Errorf("op.Trace.StampChecksum: %w", err)
	}

	t.Checksum = GitStyleChecksum("trace", canonical)
	return nil
}

// LoadTrace decodes a persisted trace document and verifies its tier-1 checksum.
//
// The verification is the trust boundary for every downstream trace consumer: an unreadable document, a
// missing checksum, or a mismatch is an error — expected external corruption. Past this gate, decode
// failures are bugs and panic (docs/architecture/5-receipt-integrity.md § The Checksum Trust Boundary).
// There is no unverified read path: a trace written before checksums existed is refused.
//
// Parameters:
//   - `data`: the trace document's YAML bytes.
//
// Returns:
//   - `*Trace`: the decoded, integrity-verified trace.
//   - `error`: non-nil on decode failure, a missing checksum, or a checksum mismatch.
func LoadTrace(data []byte) (*Trace, error) {

	var t Trace
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("op.LoadTrace: yaml decode: %w", err)
	}

	if t.Checksum == "" {
		return nil, fmt.Errorf("op.LoadTrace: document carries no checksum")
	}

	// Canonicalize the RAW document bytes — not the decoded trace, whose typed values do not re-marshal to
	// byte-identical form. See [canonicalTraceBytes].
	canonical, err := canonicalTraceBytes(data)
	if err != nil {
		return nil, fmt.Errorf("op.LoadTrace: %w", err)
	}

	if recomputed := GitStyleChecksum("trace", canonical); recomputed != t.Checksum {
		return nil, fmt.Errorf("op.LoadTrace: checksum mismatch: document %q, recomputed %q", t.Checksum, recomputed)
	}

	return &t, nil
}

// Summary is the per-action tally of an execution, reconstructed from a [Trace] by [Trace.Summarize].
//
// It replaces the execution summary the mutable [Graph] carried before the graph-immutability seal: the
// counts now derive from the trace's receipt stack rather than from per-node state on the graph.
type Summary struct {

	// byAction maps each completed action's short name (e.g. "file.link") to its tally.
	byAction map[string]ActionSummary

	// skipped is the number of planned graph nodes that never dispatched (no receipt).
	skipped int

	// failed is the number of node dispatches that returned an error.
	failed int
}

// ActionSummary is the per-action slice of a [Summary].
type ActionSummary struct {
	completed int
}

// ByAction returns the per-action tallies keyed by short action name (e.g. "file.link").
func (s Summary) ByAction() map[string]ActionSummary { return s.byAction }

// Skipped returns the number of planned nodes that never dispatched.
func (s Summary) Skipped() int { return s.skipped }

// Failed returns the number of node dispatches that returned an error.
func (s Summary) Failed() int { return s.failed }

// Completed returns the number of successful dispatches tallied for this action.
func (a ActionSummary) Completed() int { return a.completed }

// Summarize reconstructs a [Summary] of this trace's execution.
//
// Walks the trace's receipt stack ([RecoveryStack.Receipts]) and tallies, per dispatched action, the
// dispatches that completed (keyed by the receipt's short [Receipt.ActionLabel], e.g. "file.link") versus
// those that failed. Receipts with an empty label — audit-only entries pushed at a non-dispatching exit
// (cancellation, pause, or a unit whose action never resolved) — are skipped, so a failure is not double-counted
// against both a failing node and its propagating parent.
//
// `graph` is optional and consulted only for the skipped count: nodes in `graph` with no receipt are counted
// as skipped (planned but never reached; the executor unwinds on first failure). A nil `graph` yields a
// [Summary] with no skipped count — the per-action and failed tallies come from the trace alone.
//
// Parameters:
//   - `graph`: the executed graph, or nil. When supplied, its [Graph.Nodes] provide the planned set for the
//     skipped count.
//
// Returns:
//   - Summary: the reconstructed per-action / skipped / failed tally.
func (t *Trace) Summarize(graph *Graph) Summary {

	byAction := make(map[string]ActionSummary)
	dispatched := make(map[string]struct{})
	failed := 0

	if t.Stack != nil {
		for _, receipt := range t.Stack.Receipts() {

			label := receipt.ForwardAction()
			if label == "" {
				continue
			}

			dispatched[receipt.UnitID()] = struct{}{}

			if receipt.Err() != nil {
				failed++
				continue
			}

			tally := byAction[label]
			tally.completed++
			byAction[label] = tally
		}
	}

	skipped := 0
	if graph != nil {
		for _, node := range graph.Nodes() {
			if _, ok := dispatched[node.ID()]; !ok {
				skipped++
			}
		}
	}

	return Summary{byAction: byAction, skipped: skipped, failed: failed}
}
