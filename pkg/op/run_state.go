// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import "fmt"

// RunStatus is a [GraphExecutor]'s latched run-status triplet: where the run is ([Phase]), how healthy it is
// ([Condition]), and why it last moved ([RunStatus.Reason]).
//
// [Phase] and [Condition] are orthogonal. [Phase] moves forward through the run's lifecycle — preparing, running,
// the pausing/paused rest states, and the two terminal phases (completed, stopped). [Condition] latches
// monotonically by severity as the run meets trouble — healthy, degraded, execution-failed, compensation-failed —
// and never improves within a run. Completion is a [Phase] event, not a [Condition] flip: a run that reached its
// end lands with [Condition] exactly as latched (completed × healthy for a clean run, completed × degraded for one
// that degraded along the way). [RunStatus.Reason] is the prose driver of the latest move — carried on the latched
// triplet so a status logged on its own reads informatively ("stopped/failed_compensation: unwind failed").
//
// Terminals are derived rather than enumerated — the grid is {[PhaseCompleted], [PhaseStopped]} × [Condition].
// Notable cells: completed × healthy is the clean run; completed × failed_execution is a run that continued past a
// failure; stopped × healthy is a clean cancel; stopped × failed_execution is the default stop-on-failure end;
// stopped × failed_compensation is a failed unwind. The triplet is the executor's O(1) answer to "where is the run,
// how did it end, and why"; per-event detail lives on the receipts and the trace's transition journal.
//
// Serializes as a nested object of its dimensions ({"phase": …, "condition": …, "reason": …}); each enum dimension
// carries its snake name through [Phase] and [Condition]'s own text/YAML marshalers, and an empty reason is omitted.
type RunStatus struct {

	// Phase is where the run is in its lifecycle — the control dimension.
	Phase Phase `json:"phase" yaml:"phase"`

	// Condition is the run's latched health — the severity dimension, orthogonal to [RunStatus.Phase].
	Condition Condition `json:"condition" yaml:"condition"`

	// Reason is the prose driver of the latest phase or condition move (e.g. "retry budget exhausted",
	// "unwind failed: compensation error"), carried on the latched triplet for informative logging. Empty when the
	// run has not left its healthy default.
	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// Phase is where a run is in its lifecycle — the control dimension of the [RunStatus] triplet.
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
	// TransitionPolicy Stop reaction. Pairs with [Condition] to name the stop's cause.
	PhaseStopped

	// PhaseCompleted is the terminal natural end: the final unit executes, or flow.Complete executes. [Condition]
	// stays exactly as latched.
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

// Condition is a run's latched health — the severity dimension of the [RunStatus] triplet, orthogonal to [Phase].
//
// The four values are ordered by severity ([ConditionHealthy] < [ConditionDegraded] < [ConditionExecutionFailed] <
// [ConditionCompensationFailed]), and a run's condition latches monotonically toward the worst it meets: it climbs
// when a unit degrades or fails and never falls within a run. [ConditionDegraded] is reached when a flow.Degraded
// gate executes; [ConditionExecutionFailed] when an unhandled failure or flow.Failed reaches a saga boundary;
// [ConditionCompensationFailed] when a compensation action itself fails, leaving the system dirty. The severity
// order is the max-severity latch rule directly: a parent latches the worst of its children's reported conditions.
//
// Serialized over [conditionNames] in both document formats — [Condition.MarshalText] for JSON,
// [Condition.MarshalYAML] for gopkg.in/yaml.v3, which does not honor [encoding.TextMarshaler].
type Condition int

const (

	// ConditionHealthy is the no-failures condition; the zero value.
	ConditionHealthy Condition = iota

	// ConditionDegraded marks a run that met a failure a flow.Degraded gate handled: the failure is recorded and
	// execution continues.
	ConditionDegraded

	// ConditionExecutionFailed marks an unhandled forward failure — a saga boundary exhausted its retries, or
	// flow.Failed executed.
	ConditionExecutionFailed

	// ConditionCompensationFailed marks a failed unwind: a forward action failed and at least one Compensate also
	// failed, so the system is dirty. The worst condition; pairs only with [PhaseStopped].
	ConditionCompensationFailed
)

// conditionNames maps each [Condition] to its serialized name.
//
// The serialized names keep the settled snake forms `failed_execution` / `failed_compensation` (the documented
// vocabulary in the architecture and contract docs); the Go identifiers read subject-verb (`ExecutionFailed`) for
// call-site readability, so identifier and serialized word order deliberately differ.
var conditionNames = [...]string{
	ConditionHealthy:            "healthy",
	ConditionDegraded:           "degraded",
	ConditionExecutionFailed:    "failed_execution",
	ConditionCompensationFailed: "failed_compensation",
}

// region EXPORTED METHODS

// region Behaviors

// String renders the triplet as "<phase>/<condition>", appending ": <reason>" when a reason is present.
//
// Returns:
//   - `string`: e.g. "running/healthy", "completed/degraded", or "stopped/failed_compensation: unwind failed".
func (r RunStatus) String() string {

	if r.Reason == "" {
		return fmt.Sprintf("%s/%s", r.Phase, r.Condition)
	}

	return fmt.Sprintf("%s/%s: %s", r.Phase, r.Condition, r.Reason)
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

// MarshalText encodes this condition as its serialized name.
//
// Satisfies [encoding.TextMarshaler], so JSON documents carry "degraded" / "failed_execution" rather than a bare
// integer.
//
// Returns:
//   - `[]byte`: the name from [conditionNames].
//   - `error`: non-nil when the value is out of range.
func (c Condition) MarshalText() ([]byte, error) {

	if int(c) < 0 || int(c) >= len(conditionNames) {
		return nil, fmt.Errorf("op.Condition: unknown value %d", int(c))
	}

	return []byte(conditionNames[c]), nil
}

// MarshalYAML encodes this condition as its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextMarshaler], so YAML documents need this companion to carry
// "degraded" / "failed_execution" rather than a bare integer.
//
// Returns:
//   - `any`: the name from [conditionNames], as a string.
//   - `error`: non-nil when the value is out of range.
func (c Condition) MarshalYAML() (any, error) {

	text, err := c.MarshalText()
	if err != nil {
		return nil, err
	}

	return string(text), nil
}

// String returns this condition's serialized name.
//
// Returns:
//   - `string`: the name from [conditionNames], or "Condition(<n>)" for an out-of-range value.
func (c Condition) String() string {

	if int(c) >= 0 && int(c) < len(conditionNames) {
		return conditionNames[c]
	}

	return fmt.Sprintf("Condition(%d)", int(c))
}

// UnmarshalText decodes a condition from its serialized name.
//
// Satisfies [encoding.TextUnmarshaler] for JSON documents.
//
// Parameters:
//   - `text`: one of the [conditionNames] entries.
//
// Returns:
//   - `error`: non-nil when `text` names no condition.
func (c *Condition) UnmarshalText(text []byte) error {

	name := string(text)

	for value, candidate := range conditionNames {
		if candidate == name {
			*c = Condition(value)
			return nil
		}
	}

	return fmt.Errorf("op.Condition: unknown name %q", name)
}

// UnmarshalYAML decodes a condition from its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextUnmarshaler], so YAML documents need this companion.
//
// Parameters:
//   - `unmarshal`: the YAML node decoder supplied by the yaml package.
//
// Returns:
//   - `error`: non-nil when the node is not a string or names no condition.
func (c *Condition) UnmarshalYAML(unmarshal func(any) error) error {

	var name string
	if err := unmarshal(&name); err != nil {
		return err
	}

	return c.UnmarshalText([]byte(name))
}

// endregion

// endregion
