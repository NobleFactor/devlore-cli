// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Snapshot is the serializable projection of a [*GraphExecutor]'s per-run mutable state.
//
// Snapshot pairs with a [*Graph] (loaded separately via [LoadGraph]) to fully describe an execution
// that can be resumed: the graph carries the immutable plan; the snapshot carries the [RunState] at
// the moment of capture, the [*RecoveryStack] of dispatch receipts (audit + compensation), and the
// resolved variable map. The [Snapshot.GraphChecksum] identifies which graph the snapshot was taken
// against; a future resume constructor compares it against the loaded graph's checksum to refuse
// stale snapshots.
//
// A snapshot in [RunStatePaused] is resumable. A snapshot in a terminal state ([RunStateCompleted],
// [RunStateFailed]) is for archival — restoring such a snapshot reconstructs the same terminal state,
// not a runnable executor.
type Snapshot struct {

	// GraphChecksum is the canonical "sha256:<hex>" identity of the graph this snapshot was taken
	// against. Required for resume to refuse mismatched graphs.
	GraphChecksum string `json:"graph_checksum" yaml:"graph_checksum"`

	// State is the executor's [RunState] at the moment the snapshot was taken.
	State RunState `json:"state"           yaml:"state"`

	// Stack is the recovery stack of per-dispatch receipts (audit + compensation entries).
	Stack *RecoveryStack `json:"stack"           yaml:"stack"`

	// Variables is the resolved variable map at the time of the snapshot.
	Variables map[string]Variable `json:"variables,omitempty" yaml:"variables,omitempty"`
}
