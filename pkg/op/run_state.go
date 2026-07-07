// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "fmt"

// RunState is a [GraphExecutor]'s latched run-state pair: where the run is ([Phase]) and how healthy it is ([State]).
//
// The two dimensions are orthogonal. [Phase] moves forward through the run's lifecycle — preparing, running, the
// pausing/paused rest states, and the two terminal phases (completed, stopped). [State] latches monotonically by
// severity as the run meets trouble — healthy, degraded, failed_execution, failed_compensation — and never improves
// within a run. Completion is a [Phase] event, not a [State] flip: a run that reached its end lands with [State]
// exactly as latched (completed × healthy for a clean run, completed × degraded for one that degraded along the way).
//
// Terminals are derived rather than enumerated — the grid is {[PhaseCompleted], [PhaseStopped]} × [State]. Notable
// cells: completed × healthy is the clean run; completed × failed_execution is a run that continued past a failure;
// stopped × healthy is a clean cancel; stopped × failed_execution is the default stop-on-failure end; stopped ×
// failed_compensation is a failed unwind. The pair is the executor's O(1) answer to "where is the run and how did it
// end"; per-event detail lives on the receipts and the trace's transition journal.
//
// Serializes as a nested object of its two dimensions ({"phase": …, "state": …}); each dimension carries its snake
// name through [Phase] and [State]'s own text/YAML marshalers.
type RunState struct {

	// Phase is where the run is in its lifecycle — the control dimension.
	Phase Phase `json:"phase" yaml:"phase"`

	// State is the run's latched health — the severity dimension, orthogonal to [RunState.Phase].
	State State `json:"state" yaml:"state"`
}

// Phase is where a run is in its lifecycle — the control dimension of the [RunState] pair.
//
// A run is constructed in [PhasePreparing], enters [PhaseRunning] when dispatch begins, and may pass through the
// resumable [PhasePausing]/[PhasePaused] rest states. It ends in one of two terminal phases: [PhaseCompleted] (the
// natural end — the final unit or flow.Complete executes) or [PhaseStopped] (the commanded or policy-driven end). The
// transitional forms [PhasePausing] and [PhaseStopping] carry the requested-but-not-yet-observed gap the control
// plane reads.
//
// Serialized over [phaseNames] in both document formats — [Phase.MarshalText] for JSON, [Phase.MarshalYAML] for
// gopkg.in/yaml.v3, which does not honor [encoding.TextMarshaler].
type Phase int

const (

	// PhasePreparing is the pre-flight phase: variable binding, environment build, and catalog clone. The zero
	// value; entered at construction and exited when the first unit dispatches.
	PhasePreparing Phase = iota

	// PhaseRunning is the active-dispatch phase, from the first unit onward.
	PhaseRunning

	// PhasePausing is the requested-but-not-yet-observed pause: [GraphExecutor.Pause] has been called and the run
	// will suspend at the next pause-point.
	PhasePausing

	// PhasePaused is the suspended, resumable rest state; a future executor built from a serialized trace resumes it
	// to [PhaseRunning].
	PhasePaused

	// PhaseStopping is the requested-but-not-yet-observed stop: a stop command or a Stop transition reaction is
	// unwinding the boundary toward [PhaseStopped].
	PhaseStopping

	// PhaseStopped is the terminal commanded-or-policy-driven end: a stop command, a cancellation, or a
	// TransitionPolicy Stop reaction. Pairs with [State] to name the stop's cause.
	PhaseStopped

	// PhaseCompleted is the terminal natural end: the final unit executes, or flow.Complete executes. [State] stays
	// exactly as latched.
	PhaseCompleted
)

// phaseNames maps each [Phase] to its serialized name.
var phaseNames = [...]string{
	PhasePreparing: "preparing",
	PhaseRunning:   "running",
	PhasePausing:   "pausing",
	PhasePaused:    "paused",
	PhaseStopping:  "stopping",
	PhaseStopped:   "stopped",
	PhaseCompleted: "completed",
}

// State is a run's latched health — the severity dimension of the [RunState] pair, orthogonal to [Phase].
//
// The four values are ordered by severity ([StateHealthy] < [StateDegraded] < [StateFailedExecution] <
// [StateFailedCompensation]), and a run's state latches monotonically toward the worst it meets: it climbs when a
// unit degrades or fails and never falls within a run. [StateDegraded] is reached when a flow.Degraded gate executes;
// [StateFailedExecution] when an unhandled failure or flow.Failed reaches a saga boundary; [StateFailedCompensation]
// when a compensation action itself fails, leaving the system dirty. The severity order is the max-severity latch
// rule directly: a parent latches the worst of its children's reported states.
//
// Serialized over [stateNames] in both document formats — [State.MarshalText] for JSON, [State.MarshalYAML] for
// gopkg.in/yaml.v3, which does not honor [encoding.TextMarshaler].
type State int

const (

	// StateHealthy is the no-failures state; the zero value.
	StateHealthy State = iota

	// StateDegraded marks a run that met a failure a flow.Degraded gate handled: the failure is recorded and
	// execution continues.
	StateDegraded

	// StateFailedExecution marks an unhandled forward failure — a saga boundary exhausted its retries, or flow.Failed
	// executed.
	StateFailedExecution

	// StateFailedCompensation marks a failed unwind: a forward action failed and at least one Compensate also failed,
	// so the system is dirty. The worst state; pairs only with [PhaseStopped].
	StateFailedCompensation
)

// stateNames maps each [State] to its serialized name.
var stateNames = [...]string{
	StateHealthy:            "healthy",
	StateDegraded:           "degraded",
	StateFailedExecution:    "failed_execution",
	StateFailedCompensation: "failed_compensation",
}

// region EXPORTED METHODS

// region Behaviors

// String renders the pair as "<phase>/<state>" (e.g. "completed/healthy", "stopped/failed_compensation").
//
// Returns:
//   - `string`: the two dimensions' serialized names joined by a slash.
func (r RunState) String() string {
	return fmt.Sprintf("%s/%s", r.Phase, r.State)
}

// MarshalText encodes this phase as its serialized name.
//
// Satisfies [encoding.TextMarshaler], so JSON documents carry "running" / "completed" rather than a bare integer.
//
// Returns:
//   - `[]byte`: the name from [phaseNames].
//   - `error`: non-nil when the value is out of range.
func (p Phase) MarshalText() ([]byte, error) {

	if int(p) < 0 || int(p) >= len(phaseNames) {
		return nil, fmt.Errorf("op.Phase: unknown value %d", int(p))
	}

	return []byte(phaseNames[p]), nil
}

// MarshalYAML encodes this phase as its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextMarshaler], so YAML documents need this companion to carry "running"
// / "completed" rather than a bare integer.
//
// Returns:
//   - `any`: the name from [phaseNames], as a string.
//   - `error`: non-nil when the value is out of range.
func (p Phase) MarshalYAML() (any, error) {

	text, err := p.MarshalText()
	if err != nil {
		return nil, err
	}

	return string(text), nil
}

// String returns this phase's serialized name.
//
// Returns:
//   - `string`: the name from [phaseNames], or "Phase(<n>)" for an out-of-range value.
func (p Phase) String() string {

	if int(p) >= 0 && int(p) < len(phaseNames) {
		return phaseNames[p]
	}

	return fmt.Sprintf("Phase(%d)", int(p))
}

// UnmarshalText decodes a phase from its serialized name.
//
// Satisfies [encoding.TextUnmarshaler] for JSON documents.
//
// Parameters:
//   - `text`: one of the [phaseNames] entries.
//
// Returns:
//   - `error`: non-nil when `text` names no phase.
func (p *Phase) UnmarshalText(text []byte) error {

	name := string(text)

	for value, candidate := range phaseNames {
		if candidate == name {
			*p = Phase(value)
			return nil
		}
	}

	return fmt.Errorf("op.Phase: unknown name %q", name)
}

// UnmarshalYAML decodes a phase from its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextUnmarshaler], so YAML documents need this companion.
//
// Parameters:
//   - `unmarshal`: the YAML node decoder supplied by the yaml package.
//
// Returns:
//   - `error`: non-nil when the node is not a string or names no phase.
func (p *Phase) UnmarshalYAML(unmarshal func(any) error) error {

	var name string
	if err := unmarshal(&name); err != nil {
		return err
	}

	return p.UnmarshalText([]byte(name))
}

// MarshalText encodes this state as its serialized name.
//
// Satisfies [encoding.TextMarshaler], so JSON documents carry "degraded" / "failed_execution" rather than a bare
// integer.
//
// Returns:
//   - `[]byte`: the name from [stateNames].
//   - `error`: non-nil when the value is out of range.
func (s State) MarshalText() ([]byte, error) {

	if int(s) < 0 || int(s) >= len(stateNames) {
		return nil, fmt.Errorf("op.State: unknown value %d", int(s))
	}

	return []byte(stateNames[s]), nil
}

// MarshalYAML encodes this state as its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextMarshaler], so YAML documents need this companion to carry
// "degraded" / "failed_execution" rather than a bare integer.
//
// Returns:
//   - `any`: the name from [stateNames], as a string.
//   - `error`: non-nil when the value is out of range.
func (s State) MarshalYAML() (any, error) {

	text, err := s.MarshalText()
	if err != nil {
		return nil, err
	}

	return string(text), nil
}

// String returns this state's serialized name.
//
// Returns:
//   - `string`: the name from [stateNames], or "State(<n>)" for an out-of-range value.
func (s State) String() string {

	if int(s) >= 0 && int(s) < len(stateNames) {
		return stateNames[s]
	}

	return fmt.Sprintf("State(%d)", int(s))
}

// UnmarshalText decodes a state from its serialized name.
//
// Satisfies [encoding.TextUnmarshaler] for JSON documents.
//
// Parameters:
//   - `text`: one of the [stateNames] entries.
//
// Returns:
//   - `error`: non-nil when `text` names no state.
func (s *State) UnmarshalText(text []byte) error {

	name := string(text)

	for value, candidate := range stateNames {
		if candidate == name {
			*s = State(value)
			return nil
		}
	}

	return fmt.Errorf("op.State: unknown name %q", name)
}

// UnmarshalYAML decodes a state from its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextUnmarshaler], so YAML documents need this companion.
//
// Parameters:
//   - `unmarshal`: the YAML node decoder supplied by the yaml package.
//
// Returns:
//   - `error`: non-nil when the node is not a string or names no state.
func (s *State) UnmarshalYAML(unmarshal func(any) error) error {

	var name string
	if err := unmarshal(&name); err != nil {
		return err
	}

	return s.UnmarshalText([]byte(name))
}

// endregion

// endregion
