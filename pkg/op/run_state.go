// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "fmt"

// RunState is the [GraphExecutor]'s top-level execution state.
//
// The executor transitions through these values during a single Run, reaching a terminal state ([RunStateCompleted],
// [RunStateFailed], or [RunStateFailedCompensation]) at Run exit. [RunStatePaused] is non-terminal — a paused
// executor sits idle holding its state and can be resumed by a future executor constructed from a serialized
// snapshot. The state is held as an O(1) cache: walking the [RecoveryStack] to derive the current state is O(n) in
// dispatch count, and the stack alone cannot distinguish completed-with-receipts from paused, or completed-cleanly
// from degraded.
//
// Serialized over [runStateNames] ("pending" / "running" / "paused" / "degraded" / "completed" / "failed" /
// "failed_compensation") in both document formats — [RunState.MarshalText] for JSON, [RunState.MarshalYAML] for
// gopkg.in/yaml.v3, which does not honor [encoding.TextMarshaler].
type RunState int

const (

	// RunStatePending is the executor's pre-execution state. Set at construction; transitions to
	// [RunStateRunning] at the head of [GraphExecutor.Run].
	RunStatePending RunState = iota

	// RunStateRunning is the active-dispatching state. Transitions to [RunStateCompleted] at clean Run
	// exit, [RunStateFailed] at unrecoverable Run exit with a clean unwind, [RunStateFailedCompensation] when the
	// unwind itself fails, [RunStateDegraded] on recoverable error, or [RunStatePaused] on a pause signal.
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

	// RunStateFailed is the terminal failure state for an unrecoverable Run whose recovery stack unwound cleanly:
	// every completed action's Compensate succeeded, so the system is back at its pre-run state. Reached from
	// [RunStateRunning] or [RunStateDegraded].
	RunStateFailed

	// RunStateFailedCompensation is the terminal state for an unhandled failure whose unwind itself failed: a
	// forward action failed AND at least one Compensate returned an error, so the system is dirty — partially
	// compensated, in a state no plan describes. Categorically worse than [RunStateFailed] and never lumped with
	// it: the Run error names every compensation that failed, and the trace is the restartable journal of the
	// compensation-failure contract (phase-8 step 21).
	RunStateFailedCompensation
)

// runStateNames maps each [RunState] to its serialized name.
var runStateNames = [...]string{
	RunStatePending:            "pending",
	RunStateRunning:            "running",
	RunStatePaused:             "paused",
	RunStateDegraded:           "degraded",
	RunStateCompleted:          "completed",
	RunStateFailed:             "failed",
	RunStateFailedCompensation: "failed_compensation",
}

// region EXPORTED METHODS

// region Behaviors

// MarshalText encodes this run state as its serialized name.
//
// Satisfies [encoding.TextMarshaler], so JSON documents carry "failed" / "failed_compensation" rather than a bare
// integer.
//
// Returns:
//   - `[]byte`: the name from [runStateNames].
//   - `error`: non-nil when the value is out of range.
func (s RunState) MarshalText() ([]byte, error) {

	if int(s) < 0 || int(s) >= len(runStateNames) {
		return nil, fmt.Errorf("op.RunState: unknown value %d", int(s))
	}

	return []byte(runStateNames[s]), nil
}

// MarshalYAML encodes this run state as its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextMarshaler], so YAML documents need this companion to carry
// "failed" / "failed_compensation" rather than a bare integer.
//
// Returns:
//   - `any`: the name from [runStateNames], as a string.
//   - `error`: non-nil when the value is out of range.
func (s RunState) MarshalYAML() (any, error) {

	text, err := s.MarshalText()
	if err != nil {
		return nil, err
	}

	return string(text), nil
}

// String returns this run state's serialized name.
//
// Returns:
//   - `string`: the name from [runStateNames], or "RunState(<n>)" for an out-of-range value.
func (s RunState) String() string {

	if int(s) >= 0 && int(s) < len(runStateNames) {
		return runStateNames[s]
	}

	return fmt.Sprintf("RunState(%d)", int(s))
}

// UnmarshalText decodes a run state from its serialized name.
//
// Satisfies [encoding.TextUnmarshaler] for JSON documents.
//
// Parameters:
//   - `text`: one of the [runStateNames] entries.
//
// Returns:
//   - `error`: non-nil when `text` names no run state.
func (s *RunState) UnmarshalText(text []byte) error {

	name := string(text)

	for value, candidate := range runStateNames {
		if candidate == name {
			*s = RunState(value)
			return nil
		}
	}

	return fmt.Errorf("op.RunState: unknown name %q", name)
}

// UnmarshalYAML decodes a run state from its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextUnmarshaler], so YAML documents need this companion.
//
// Parameters:
//   - `unmarshal`: the YAML node decoder supplied by the yaml package.
//
// Returns:
//   - `error`: non-nil when the node is not a string or names no run state.
func (s *RunState) UnmarshalYAML(unmarshal func(any) error) error {

	var name string
	if err := unmarshal(&name); err != nil {
		return err
	}

	return s.UnmarshalText([]byte(name))
}

// endregion

// endregion
