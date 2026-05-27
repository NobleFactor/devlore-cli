// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// RunState is the [GraphExecutor]'s top-level execution state.
//
// The executor transitions through these values during a single Run, reaching a terminal state
// ([RunStateCompleted] or [RunStateFailed]) at Run exit. [RunStatePaused] is non-terminal — a paused
// executor sits idle holding its state and can be resumed by a future executor constructed from a
// serialized snapshot. The state is held as an O(1) cache: walking the [RecoveryStack] to derive the
// current state is O(n) in dispatch count, and the stack alone cannot distinguish
// completed-with-receipts from paused, or completed-cleanly from degraded.
type RunState int

const (

	// RunStatePending is the executor's pre-execution state. Set at construction; transitions to
	// [RunStateRunning] at the head of [GraphExecutor.Run].
	RunStatePending RunState = iota

	// RunStateRunning is the active-dispatching state. Transitions to [RunStateCompleted] at clean Run
	// exit, [RunStateFailed] at unrecoverable Run exit, [RunStateDegraded] on recoverable error, or
	// [RunStatePaused] on a pause signal.
	RunStateRunning

	// RunStatePaused is the suspended-but-resumable state. The executor holds its [RecoveryStack],
	// resolved variables, and active activation frames; a future executor constructed from a serialized
	// snapshot can resume from this point.
	RunStatePaused

	// RunStateDegraded marks a Run that hit recoverable errors but is still progressing. Per-receipt
	// error details live on the [RecoveryStack]; this top-level marker is the cheap "are we degraded?"
	// signal that callers consult without walking the stack.
	RunStateDegraded

	// RunStateCompleted is the terminal success state. Reached at clean Run exit, whether from
	// [RunStateRunning] or [RunStateDegraded].
	RunStateCompleted

	// RunStateFailed is the terminal failure state. Reached at unrecoverable Run exit, whether from
	// [RunStateRunning] or [RunStateDegraded].
	RunStateFailed
)

// String returns the canonical lower-case label for this state.
//
// Returns:
//   - `string`: "pending", "running", "paused", "degraded", "completed", "failed", or "unknown".
func (s RunState) String() string {

	switch s {
	case RunStatePending:
		return "pending"
	case RunStateRunning:
		return "running"
	case RunStatePaused:
		return "paused"
	case RunStateDegraded:
		return "degraded"
	case RunStateCompleted:
		return "completed"
	case RunStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}
