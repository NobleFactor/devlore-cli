// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// ControlPlane is a run's bidirectional command / event channel — the in-process core of the control plane
// (architecture 2.7).
//
// Two directions cross one plane, both fully async (nothing here blocks the run or the consumer):
//   - Commands in — a consumer calls [ControlPlane.Request] with a [ControlCommand] and gets back a response channel
//     (the future / stream-id correlation). The executor drains pending requests at each control-point via
//     [ControlPlane.poll] and answers each on its own channel, so a slow response never blocks another (no response
//     head-of-line blocking).
//   - Events out — a consumer calls [ControlPlane.Subscribe] to receive pushed [ControlEvent]s; the executor emits
//     via [ControlPlane.emit], a non-blocking fan-out that drops rather than stall the run when a subscriber can't
//     keep up.
//
// One plane is shared by a whole run — every child executor holds the same pointer (see
// [GraphExecutor.newChildExecutor]) — so a command issued anywhere is observed at the next control-point wherever it
// falls in the tree, and an event emitted anywhere reaches every subscriber. The wire listener (HTTP/2) that bridges
// a remote consumer to a plane is a separate layer; this type is transport-agnostic.
type ControlPlane struct {

	// requests is the inbound command conduit — a single ordered channel drained at control-points, so commands take
	// effect in arrival order at a safe point. Buffered so a producer (including a hook on the executor's own
	// goroutine) never blocks enqueuing.
	requests chan controlRequest

	// mu guards subscribers.
	mu sync.Mutex

	// subscribers is the set of live event channels; emit fans out to each.
	subscribers map[chan ControlEvent]struct{}

	// seq is the monotonic event sequence, stamped on every emitted [ControlEvent] so a subscriber can detect the gap
	// when the fan-out drops under backpressure.
	seq atomic.Uint64
}

// NewControlPlane returns an empty control plane ready for [ControlPlane.Request] and [ControlPlane.Subscribe].
//
// Returns:
//   - *ControlPlane: the constructed plane.
func NewControlPlane() *ControlPlane {
	return &ControlPlane{
		requests:    make(chan controlRequest, 16),
		subscribers: make(map[chan ControlEvent]struct{}),
	}
}

// region EXPORTED METHODS

// region Behaviors

// Request submits `cmd` and returns its response channel — the consumer side of commands-in. It never blocks.
//
// The channel yields exactly one [ControlResponse], sent when the executor serves the command at its next
// control-point (or immediately, with an error, when the inbound queue is somehow full). The caller selects on the
// channel when ready; a caller that wants to block just receives from it.
//
// Parameters:
//   - `cmd`: the command to issue ([ControlPause] / [ControlStop] / [ControlStep]).
//
// Returns:
//   - `<-chan ControlResponse`: a single-response channel (the future / stream-id correlation).
func (p *ControlPlane) Request(cmd ControlCommand) <-chan ControlResponse {

	response := make(chan ControlResponse, 1)

	select {
	case p.requests <- controlRequest{cmd: cmd, response: response}:
	default:
		response <- ControlResponse{Err: fmt.Errorf("control plane: request queue full")}
	}

	return response
}

// Subscribe registers for pushed events and returns the stream plus an unsubscribe func — the consumer side of
// events-out. It never blocks.
//
// The returned channel receives every [ControlEvent] emitted after the call, in order, until `cancel` is invoked
// (which removes the subscription and closes the channel). The buffer is bounded; a subscriber that falls behind
// drops events rather than stall the run — the `Seq` gap on the next delivered event reveals the loss.
//
// Returns:
//   - `<-chan ControlEvent`: the pushed event stream.
//   - `func()`: the unsubscribe; idempotent.
func (p *ControlPlane) Subscribe() (<-chan ControlEvent, func()) {

	events := make(chan ControlEvent, 64)

	p.mu.Lock()
	p.subscribers[events] = struct{}{}
	p.mu.Unlock()

	cancel := func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if _, live := p.subscribers[events]; live {
			delete(p.subscribers, events)
			close(events)
		}
	}

	return events, cancel
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// poll non-blockingly drains one pending request — the executor side of commands-in, called at each control-point.
//
// Returns:
//   - `controlRequest`: the drained request; zero value when none pended.
//   - `bool`: true when a request was drained.
func (p *ControlPlane) poll() (controlRequest, bool) {

	select {
	case request := <-p.requests:
		return request, true
	default:
		return controlRequest{}, false
	}
}

// emit stamps `event` with the next sequence number and fans it out to every subscriber — the executor side of
// events-out.
//
// The per-subscriber send is non-blocking: a subscriber whose buffer is full is skipped (the event is dropped for
// that subscriber only), so a slow observer never stalls the run. The dropped subscriber sees the gap on its next
// delivered event's `Seq`.
//
// Parameters:
//   - `event`: the event to emit; its `Seq` is stamped here.
func (p *ControlPlane) emit(event ControlEvent) {

	event.Seq = p.seq.Add(1)

	p.mu.Lock()
	defer p.mu.Unlock()

	for subscriber := range p.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

// endregion

// endregion

// region SUPPORTING TYPES

// ControlCommand is a command a consumer issues to steer a run.
type ControlCommand int32

// ControlCommand values.
const (
	// ControlPause halts at the next control-point and preserves the recovery stack as the resume point
	// ([PhasePaused], resumable via [ResumeExecutor]).
	ControlPause ControlCommand = iota

	// ControlStop halts at the next control-point, unwinds (compensating completed work), and terminates
	// ([PhaseStopped]); not resumable.
	ControlStop

	// ControlStep advances one unit then re-pauses. Reserved; not yet served.
	ControlStep
)

// String returns the lowercase command name.
//
// Returns:
//   - `string`: "pause" / "stop" / "step", or "control(<n>)" for an unknown value.
func (c ControlCommand) String() string {
	switch c {
	case ControlPause:
		return "pause"
	case ControlStop:
		return "stop"
	case ControlStep:
		return "step"
	default:
		return fmt.Sprintf("control(%d)", int32(c))
	}
}

// ControlResponse is the acknowledgement of a [ControlCommand] — the response half of a request/response pair.
type ControlResponse struct {

	// Status is the run status the command produced (e.g. [PhasePaused] for a served pause).
	Status RunStatus

	// Err is non-nil when the command was rejected (an unsupported command, or a full inbound queue).
	Err error
}

// ControlEventKind classifies a pushed [ControlEvent].
type ControlEventKind int

// ControlEventKind values.
const (
	// EventPhaseChanged reports a [RunStatus] transition (including per-unit progress within [PhaseRunning]).
	EventPhaseChanged ControlEventKind = iota

	// EventError reports a unit or run error.
	EventError

	// EventResult reports that a terminal result is available.
	EventResult
)

// ControlEvent is one pushed observation of a run — the event half of the plane.
type ControlEvent struct {

	// Seq is the per-run monotonic sequence number, stamped by [ControlPlane.emit]; a gap reveals a dropped event.
	Seq uint64

	// Kind classifies the event.
	Kind ControlEventKind

	// Status is the run status at the moment of the event.
	Status RunStatus

	// UnitID is the unit involved, when relevant (empty for run-level events).
	UnitID string

	// Err is set for [EventError].
	Err error
}

// controlRequest pairs an inbound command with the channel its response is sent on (the stream-id correlation).
type controlRequest struct {
	cmd      ControlCommand
	response chan ControlResponse
}

// endregion
