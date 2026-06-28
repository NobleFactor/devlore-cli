// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Trace is the serializable projection of a [*GraphExecutor]'s per-run mutable state.
//
// Trace pairs with a [*Graph] (loaded separately via [LoadGraph]) to fully describe an execution
// that can be resumed: the graph carries the immutable plan; the trace carries the [RunState] at
// the moment of capture, the [*RecoveryStack] of dispatch receipts (audit + compensation), and the
// resolved variable map. The [Trace.GraphChecksum] identifies which graph the trace was taken
// against; a future resume constructor compares it against the loaded graph's checksum to refuse
// stale traces.
//
// A trace in [RunStatePaused] is resumable. A trace in a terminal state ([RunStateCompleted],
// [RunStateFailed]) is for archival — restoring such a trace reconstructs the same terminal state,
// not a runnable executor.
type Trace struct {

	// GraphChecksum is the canonical "sha256:<hex>" identity of the graph this trace was taken
	// against. Required for resume to refuse mismatched graphs.
	GraphChecksum string `json:"graph_checksum" yaml:"graph_checksum"`

	// State is the executor's [RunState] at the moment the trace was taken.
	State RunState `json:"state"           yaml:"state"`

	// Stack is the recovery stack of per-dispatch receipts (audit + compensation entries).
	Stack *RecoveryStack `json:"stack"           yaml:"stack"`

	// Variables is the resolved variable map at the time of the trace.
	Variables map[string]Variable `json:"variables,omitempty" yaml:"variables,omitempty"`

	// Catalog is the serialized resource ledger — every generation keyed by id — captured at pause. Resume rehydrates
	// it into the live [ResourceCatalog] and resolves the recovery stack's receipt id references against it.
	Catalog *ResourceLedgerSnapshot `json:"catalog,omitempty" yaml:"catalog,omitempty"`
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
