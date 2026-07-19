// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"time"
)

// RunStatus reports where a run is ([Phase]), the worst trouble it has met ([Condition]), and why it last changed.
//
// [Phase] and [Condition] are orthogonal. [Phase] moves forward through the run's lifecycle — preparing, running,
// the pausing/paused rest states, and the two terminal phases (completed, stopped). [Condition] only worsens —
// it climbs by severity as the run meets trouble (healthy, degraded, execution-failed, compensation-failed) and
// never improves within a run. Completion is a [Phase] event, not a [Condition] change: a run that reached its
// end lands with [Condition] exactly as it stood (completed × healthy for a clean run, completed × degraded for one
// that degraded along the way). [RunStatus.Reason] is the prose driver of the latest move — carried on the status
// so one logged on its own reads informatively ("stopped/compensation_failed: unwind failed").
//
// Terminals are derived rather than enumerated — the grid is {[PhaseCompleted], [PhaseStopped]} × [Condition].
// Notable cells: completed × healthy is the clean run; completed × execution_failed is a run that continued past a
// failure; stopped × healthy is a clean cancel; stopped × execution_failed is the default stop-on-failure end;
// stopped × compensation_failed is a failed unwind. The status is the executor's O(1) answer to "where is the run,
// how did it end, and why"; per-event detail lives on the receipts and the trace's transition journal.
//
// Serializes as a nested object of its dimensions ({"phase": …, "condition": …, "reason": …}); each enum dimension
// carries its snake name through [Phase] and [Condition]'s own text/YAML marshalers, and an empty reason is omitted.
type RunStatus struct {

	// Phase is where the run is in its lifecycle — the control dimension.
	Phase Phase `json:"phase" yaml:"phase"`

	// Condition is the worst trouble the run has met — the severity dimension, orthogonal to [RunStatus.Phase].
	Condition Condition `json:"condition" yaml:"condition"`

	// Reason names the class of event that drove the latest phase or condition move — a closed [Reason] vocabulary
	// for machine dispatch and diagnostics. [ReasonUnspecified] (the zero value) when the run has not left its
	// healthy default.
	Reason Reason `json:"reason,omitempty" yaml:"reason,omitempty"`

	// Message is the free-text detail behind [RunStatus.Reason] (e.g. "flow.degraded executed: disk 90% full",
	// typically an err.Error()), carried on the status for informative logging. Empty when there is nothing to add.
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

// region EXPORTED METHODS

// region Behaviors

// String renders the status as "<phase>/<condition>", appending ": <message>" when a message is present.
//
// Returns:
//   - `string`: e.g. "running/healthy", "completed/degraded", or "stopped/compensation_failed: unwind failed".
func (r RunStatus) String() string {

	if r.Message == "" {
		return fmt.Sprintf("%s/%s", r.Phase, r.Condition)
	}

	return fmt.Sprintf("%s/%s: %s", r.Phase, r.Condition, r.Message)
}

// endregion

// endregion

// region SUPPORTING TYPES

// Condition is the worst trouble a run has met — the severity dimension of [RunStatus], orthogonal to [Phase].
//
// The four values are ordered by severity ([ConditionHealthy] < [ConditionDegraded] < [ConditionExecutionFailed] <
// [ConditionCompensationFailed]), and a run's condition only worsens: it climbs when a unit
// degrades or fails and never falls within a run. A [ConditionDegraded] is reached when a flow.Degraded
// gate executes; [ConditionExecutionFailed] when an unhandled failure or flow.Failed reaches a saga boundary;
// [ConditionCompensationFailed] when a compensation action itself fails, leaving the system dirty. The severity
// order encodes the bubble-up rule directly: a parent takes the worst of its children's reported conditions.
//
// Serialized over [conditionNames] in both document formats — [Condition.MarshalText] for JSON,
// [Condition.MarshalYAML] for gopkg.in/yaml.v3, which does not honor [encoding.TextMarshaler].
type Condition int

const (

	// ConditionHealthy is the no degradations or failures condition; the zero value.
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
// The serialized names are the snake forms of the identifiers — `execution_failed` / `compensation_failed` for
// `ConditionExecutionFailed` / `ConditionCompensationFailed` (the documented vocabulary in the architecture and
// contract docs); identifier and serialized name share the same subject-verb word order.
var conditionNames = [...]string{
	ConditionHealthy:            "healthy",
	ConditionDegraded:           "degraded",
	ConditionExecutionFailed:    "execution_failed",
	ConditionCompensationFailed: "compensation_failed",
}

// region EXPORTED METHODS

// region Behaviors

// MarshalText encodes this condition as its serialized name.
//
// Satisfies [encoding.TextMarshaler], so JSON documents carry "degraded" / "execution_failed" rather than a bare
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
// "degraded" / "execution_failed" rather than a bare integer.
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
//   - `unmarshal`: the YAML node decoder supplied by the `yaml` package.
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

// Phase is where a run is in its lifecycle — the control dimension of [RunStatus].
//
// A run is constructed in [PhasePreparing], enters [PhaseRunning] when dispatch begins, and may pass through the
// resumable [PhasePausing]/[PhasePaused] rest states. It ends in one of two terminal phases: [PhaseCompleted] (the
// natural end — the final unit or flow.Complete executes) or [PhaseStopped] (the commanded or policy-driven end). The
// transitional forms [PhasePausing] and [PhaseStopping] carry the requested-but-not-yet-observed gap that the control
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

	// PhasePausing is the requested-but-not-yet-observed pause: [GraphExecutor.Pause] has been called, and the run
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

	// PhaseCompleted is the natural end: the final unit executes, or flow.Complete executes. The [Condition]
	// stays exactly as set.
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

// region EXPORTED METHODS

// region Behaviors

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
//   - `unmarshal`: the YAML node decoder supplied by the `yaml` package.
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

// endregion

// endregion

// Reason names the class of event that drove a [RunStatus] to its latest phase or condition — a closed, coarse
// vocabulary for machine dispatch and diagnostics, distinct from the free-text [RunStatus.Message]. Two families:
// health reasons name a condition's cause ([ReasonActionFailed], [ReasonCompensationFailed], [ReasonRetryVetoed],
// [ReasonHandlerFailed], [ReasonAbsorbed], [ReasonDegraded], [ReasonFailed], [ReasonPreflightFailed],
// [ReasonFrameworkFailed]); lifecycle reasons name a phase move ([ReasonStarted], [ReasonCompleted], [ReasonStopped],
// [ReasonPaused]). The zero value
// [ReasonUnspecified] serializes to the empty string.
//
// Serialized over [reasonNames] in both document formats — [Reason.MarshalText] for JSON, [Reason.MarshalYAML] for
// gopkg.in/yaml.v3, which does not honor [encoding.TextMarshaler].
type Reason int

const (
	// ReasonUnspecified is the zero value — no reason recorded, a run at its healthy default.
	ReasonUnspecified Reason = iota

	// ReasonActionFailed marks an execution failure from an action's error return — the objective default.
	ReasonActionFailed

	// ReasonCompensationFailed marks a compensation failure from a compensating action's error return.
	ReasonCompensationFailed

	// ReasonRetryVetoed marks a retry loop ended by an OnRetry veto rather than by exhaustion.
	ReasonRetryVetoed

	// ReasonHandlerFailed marks an OnError or OnRetry handler that itself errored or broke.
	ReasonHandlerFailed

	// ReasonAbsorbed marks a failure an OnError handler recovered — the pending flip was rejected.
	ReasonAbsorbed

	// ReasonDegraded marks a subjective degrade asserted by flow.Degraded.
	ReasonDegraded

	// ReasonFailed marks an execution failure asserted by flow.Failed — subjective, distinct from an action's error.
	ReasonFailed

	// ReasonPreflightFailed marks a failure during the preparing phase (ledger rehydrate, stack re-arm, variable bind).
	ReasonPreflightFailed

	// ReasonFrameworkFailed marks a framework dispatch failure that is not an action's error return — no action bound,
	// action-name resolution failure, or malformed decision topology at runtime. A structural error, so it bypasses
	// OnError rather than being absorbed as an incidental failure.
	ReasonFrameworkFailed

	// ReasonStarted marks the move into the running phase.
	ReasonStarted

	// ReasonCompleted marks the move into the completed phase.
	ReasonCompleted

	// ReasonStopped marks the move into the stopped phase.
	ReasonStopped

	// ReasonPaused marks the move into the paused phase.
	ReasonPaused

	// ReasonUnwound marks the resume de-escalation: a resumed state-checked unwind cleared a compensation_failed
	// trace back to execution_failed — the one sanctioned downward condition move (step 21's Restart contract).
	ReasonUnwound
)

// reasonNames maps each [Reason] to its serialized name.
//
// The names are snake-case tokens; [ReasonUnspecified] maps to the empty string, so an unset reason is omitted from
// documents (the field carries `omitempty`).
var reasonNames = [...]string{
	ReasonUnspecified:        "",
	ReasonActionFailed:       "action_failed",
	ReasonCompensationFailed: "compensation_failed",
	ReasonRetryVetoed:        "retry_vetoed",
	ReasonHandlerFailed:      "handler_failed",
	ReasonAbsorbed:           "absorbed",
	ReasonDegraded:           "degraded",
	ReasonFailed:             "failed",
	ReasonPreflightFailed:    "preflight_failed",
	ReasonFrameworkFailed:    "framework_failed",
	ReasonStarted:            "started",
	ReasonCompleted:          "completed",
	ReasonStopped:            "stopped",
	ReasonPaused:             "paused",
	ReasonUnwound:            "unwound",
}

// region EXPORTED METHODS

// region Behaviors

// MarshalText encodes this reason as its serialized name.
//
// Satisfies [encoding.TextMarshaler], so JSON documents carry "action_failed" / "paused" rather than a bare integer.
//
// Returns:
//   - `[]byte`: the name from [reasonNames].
//   - `error`: non-nil when the value is out of range.
func (r Reason) MarshalText() ([]byte, error) {

	if int(r) < 0 || int(r) >= len(reasonNames) {
		return nil, fmt.Errorf("op.Reason: unknown value %d", int(r))
	}

	return []byte(reasonNames[r]), nil
}

// MarshalYAML encodes this reason as its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextMarshaler], so YAML documents need this companion to carry
// "action_failed" / "paused" rather than a bare integer.
//
// Returns:
//   - `any`: the name from [reasonNames], as a string.
//   - `error`: non-nil when the value is out of range.
func (r Reason) MarshalYAML() (any, error) {

	text, err := r.MarshalText()
	if err != nil {
		return nil, err
	}

	return string(text), nil
}

// String returns this reason's serialized name.
//
// Returns:
//   - `string`: the name from [reasonNames], or "Reason(<n>)" for an out-of-range value.
func (r Reason) String() string {

	if int(r) >= 0 && int(r) < len(reasonNames) {
		return reasonNames[r]
	}

	return fmt.Sprintf("Reason(%d)", int(r))
}

// UnmarshalText decodes a reason from its serialized name.
//
// Satisfies [encoding.TextUnmarshaler] for JSON documents.
//
// Parameters:
//   - `text`: one of the [reasonNames] entries.
//
// Returns:
//   - `error`: non-nil when `text` names no reason.
func (r *Reason) UnmarshalText(text []byte) error {

	name := string(text)

	for value, candidate := range reasonNames {
		if candidate == name {
			*r = Reason(value)
			return nil
		}
	}

	return fmt.Errorf("op.Reason: unknown name %q", name)
}

// UnmarshalYAML decodes a reason from its serialized name.
//
// gopkg.in/yaml.v3 does not honor [encoding.TextUnmarshaler], so YAML documents need this companion.
//
// Parameters:
//   - `unmarshal`: the YAML node decoder supplied by the `yaml` package.
//
// Returns:
//   - `error`: non-nil when the node is not a string or names no reason.
func (r *Reason) UnmarshalYAML(unmarshal func(any) error) error {

	var name string
	if err := unmarshal(&name); err != nil {
		return err
	}

	return r.UnmarshalText([]byte(name))
}

// endregion

// endregion

// RunStatusTransition is one entry in a [Trace]'s transition journal — a recorded flip of the run's status.
//
// Flips-only: the journal records actual changes to the run's [Phase] or [Condition], with when each happened,
// which unit drove it, and why. A repeat driver that does not change the status (a second flow.Degraded while
// already degraded) is a receipt, not a transition. The executor's [RunStatus] stays the O(1) answer; the journal
// answers "when did the run flip to degraded?" and "where did it flip to execution_failed?" directly,
// cross-referenced to per-event detail on the receipts by [RunStatusTransition.UnitID].
type RunStatusTransition struct {

	// Phase is the [Phase] the run is in after this transition.
	Phase Phase `json:"phase" yaml:"phase"`

	// Condition is the [Condition] the run is in after this transition.
	Condition Condition `json:"condition" yaml:"condition"`

	// At is when the flip was recorded, stamped by [GraphExecutor.Transition].
	At time.Time `json:"at" yaml:"at"`

	// UnitID is the unit whose outcome drove the flip; empty for run-level events (a pause command, pre-flight).
	UnitID string `json:"unit_id,omitempty" yaml:"unit_id,omitempty"`

	// Reason names the class of event that drove the flip — a [Reason] token for machine dispatch and diagnostics.
	Reason Reason `json:"reason,omitempty" yaml:"reason,omitempty"`

	// Message is the free-text detail behind [RunStatusTransition.Reason] (e.g. "flow.degraded executed: disk full").
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

// endregion
