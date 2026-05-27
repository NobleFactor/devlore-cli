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
}
