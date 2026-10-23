// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
	"sync"
)

// Interface Guard: the double is a Broker, at compile time.
var _ Broker = (*RecordingBroker)(nil)

// RecordingBroker is a [Broker] for tests: it records every call and delivers what a test enqueues.
//
// Nothing runs behind it. A test drives it from the far side with [RecordingBroker.Deliver] and
// [RecordingBroker.Emit], which stamp the per-run seq and fan out to subscribers exactly as a broker does, and reads
// what the code under test asked for with [RecordingBroker.Execs] and its siblings. It is a non-test file so every
// package above the interface can test against it without a broker.
type RecordingBroker struct {

	// mu guards everything below.
	mu sync.Mutex

	// execs, posts, cancels and commands record the calls, in order.
	execs    []ExecRequest
	posts    []PostRequest
	cancels  []ExecID
	commands []RecordedCommand

	// next numbers the exec ids: e-1, e-2, ...
	next int

	// refusals, by account: an Exec or Post for the account is refused with the refusal.
	refusals map[string]*Refusal

	// statuses, by run: what Status and Command answer; a run with none is unknown.
	statuses map[RunID]RunStatus

	// consoles and events are the per-run streams, created on first subscription or delivery.
	consoles map[RunID]*brokerStream[ConsoleFrame]
	events   map[RunID]*brokerStream[ControlEvent]
}

// NewRecordingBroker returns an empty double: no runs, no refusals, nothing delivered.
//
// Returns:
//   - `*RecordingBroker`: the double.
func NewRecordingBroker() *RecordingBroker {
	return &RecordingBroker{
		refusals: make(map[string]*Refusal),
		statuses: make(map[RunID]RunStatus),
		consoles: make(map[RunID]*brokerStream[ConsoleFrame]),
		events:   make(map[RunID]*brokerStream[ControlEvent]),
	}
}

// region EXPORTED METHODS

// region State management

// Cancels returns the exec ids Cancel was called with, in order.
//
// Returns:
//   - `[]ExecID`: a copy.
func (b *RecordingBroker) Cancels() []ExecID {

	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]ExecID(nil), b.cancels...)
}

// Commands returns the commands Command was called with, in order.
//
// Returns:
//   - `[]RecordedCommand`: a copy.
func (b *RecordingBroker) Commands() []RecordedCommand {

	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]RecordedCommand(nil), b.commands...)
}

// Execs returns the requests Exec was called with, in order, refused ones included.
//
// Returns:
//   - `[]ExecRequest`: a copy.
func (b *RecordingBroker) Execs() []ExecRequest {

	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]ExecRequest(nil), b.execs...)
}

// Posts returns the requests Post was called with, in order, refused ones included.
//
// Returns:
//   - `[]PostRequest`: a copy.
func (b *RecordingBroker) Posts() []PostRequest {

	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]PostRequest(nil), b.posts...)
}

// Refuse makes every Exec and Post for an account fail with the refusal, as a broker whose policy does not offer the
// account would.
//
// Parameters:
//   - `account`: the account to refuse.
//   - `refusal`: what Exec and Post return for it.
func (b *RecordingBroker) Refuse(account string, refusal *Refusal) {

	b.mu.Lock()
	defer b.mu.Unlock()
	b.refusals[account] = refusal
}

// SetStatus makes a run known and sets what Status and Command answer for it.
//
// Parameters:
//   - `run`: the run.
//   - `status`: its status.
func (b *RecordingBroker) SetStatus(run RunID, status RunStatus) {

	b.mu.Lock()
	defer b.mu.Unlock()
	b.statuses[run] = status
}

// endregion

// region Behaviors

// Fallible actions

// Cancel records the exec id.
//
// Parameters:
//   - `ctx`: unused.
//   - `exec`: the exec to cancel.
//
// Returns:
//   - `error`: always nil.
func (b *RecordingBroker) Cancel(_ context.Context, exec ExecID) error {

	b.mu.Lock()
	defer b.mu.Unlock()
	b.cancels = append(b.cancels, exec)
	return nil
}

// Exec records the request and returns the next exec id, or the refusal set for its account.
//
// Parameters:
//   - `ctx`: unused.
//   - `request`: the exec.
//
// Returns:
//   - `ExecID`: e-<n>, numbered across Exec and Post.
//   - `error`: the request's validation error, or the account's *Refusal.
func (b *RecordingBroker) Exec(_ context.Context, request ExecRequest) (ExecID, error) {

	if err := request.Validate(); err != nil {
		return "", err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.execs = append(b.execs, request)
	if refusal, refused := b.refusals[request.Account]; refused {
		return "", refusal
	}
	return b.nextExec(), nil
}

// Post records the request and returns the next exec id, or the refusal set for its account.
//
// Parameters:
//   - `ctx`: unused.
//   - `request`: the posted unit.
//
// Returns:
//   - `ExecID`: e-<n>, numbered across Exec and Post.
//   - `error`: the request's validation error, or the account's *Refusal.
func (b *RecordingBroker) Post(_ context.Context, request PostRequest) (ExecID, error) {

	if err := request.Validate(); err != nil {
		return "", err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.posts = append(b.posts, request)
	if refusal, refused := b.refusals[request.Account]; refused {
		return "", refusal
	}
	return b.nextExec(), nil
}

// Status returns the status SetStatus gave the run.
//
// Parameters:
//   - `run`: the run.
//
// Returns:
//   - `RunStatus`: its status.
//   - `error`: non-nil when the run is unknown.
func (b *RecordingBroker) Status(run RunID) (RunStatus, error) {

	b.mu.Lock()
	defer b.mu.Unlock()
	status, known := b.statuses[run]
	if !known {
		return RunStatus{}, fmt.Errorf("broker: unknown run %q", run)
	}
	return status, nil
}

// Actions

// Command records the command and answers at once with the run's status, or an error for an unknown run.
//
// Parameters:
//   - `run`: the run.
//   - `command`: the command.
//
// Returns:
//   - `<-chan ControlResponse`: a channel holding the one response.
func (b *RecordingBroker) Command(run RunID, command ControlCommand) <-chan ControlResponse {

	response := make(chan ControlResponse, 1)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.commands = append(b.commands, RecordedCommand{Run: run, Command: command})
	status, known := b.statuses[run]
	if !known {
		response <- ControlResponse{Err: fmt.Errorf("broker: unknown run %q", run)}
	} else {
		response <- ControlResponse{Status: status}
	}
	return response
}

// Console subscribes to the run's console stream.
//
// Parameters:
//   - `run`: the run.
//
// Returns:
//   - `<-chan ConsoleFrame`: every frame delivered after the call, in order, seq stamped.
//   - `func()`: the unsubscribe; idempotent; closes the channel.
func (b *RecordingBroker) Console(run RunID) (frames <-chan ConsoleFrame, cancel func()) {

	b.mu.Lock()
	defer b.mu.Unlock()
	return b.console(run).subscribe(&b.mu)
}

// Deliver stamps the frame with the run's next console seq and fans it out to the run's console subscribers, as a
// broker does; a subscriber that has fallen behind misses it, and the gap in seq shows.
//
// Parameters:
//   - `run`: the run.
//   - `frame`: the frame; its Seq is overwritten.
func (b *RecordingBroker) Deliver(run RunID, frame ConsoleFrame) {

	b.mu.Lock()
	defer b.mu.Unlock()
	stream := b.console(run)
	frame.Seq = stream.next()
	stream.publish(frame)
}

// Emit stamps the event with the run's next events seq and fans it out to the run's events subscribers.
//
// Parameters:
//   - `run`: the run.
//   - `event`: the event; its Seq is overwritten.
func (b *RecordingBroker) Emit(run RunID, event ControlEvent) {

	b.mu.Lock()
	defer b.mu.Unlock()
	stream := b.event(run)
	event.Seq = stream.next()
	stream.publish(event)
}

// Events subscribes to the run's events stream.
//
// Parameters:
//   - `run`: the run.
//
// Returns:
//   - `<-chan ControlEvent`: every event emitted after the call, in order, seq stamped.
//   - `func()`: the unsubscribe; idempotent; closes the channel.
func (b *RecordingBroker) Events(run RunID) (events <-chan ControlEvent, cancel func()) {

	b.mu.Lock()
	defer b.mu.Unlock()
	return b.event(run).subscribe(&b.mu)
}

// endregion

// endregion

// region UNEXPORTED METHODS

// region Behaviors

// console returns the run's console stream, creating it. The caller holds mu.
//
// Parameters:
//   - `run`: the run.
//
// Returns:
//   - `*brokerStream[ConsoleFrame]`: the stream.
func (b *RecordingBroker) console(run RunID) *brokerStream[ConsoleFrame] {

	stream, known := b.consoles[run]
	if !known {
		stream = newBrokerStream[ConsoleFrame]()
		b.consoles[run] = stream
	}
	return stream
}

// event returns the run's events stream, creating it. The caller holds mu.
//
// Parameters:
//   - `run`: the run.
//
// Returns:
//   - `*brokerStream[ControlEvent]`: the stream.
func (b *RecordingBroker) event(run RunID) *brokerStream[ControlEvent] {

	stream, known := b.events[run]
	if !known {
		stream = newBrokerStream[ControlEvent]()
		b.events[run] = stream
	}
	return stream
}

// nextExec mints the next exec id. The caller holds mu.
//
// Returns:
//   - `ExecID`: e-<n>.
func (b *RecordingBroker) nextExec() ExecID {

	b.next++
	return ExecID(fmt.Sprintf("e-%d", b.next))
}

// endregion

// endregion

// region SUPPORTING TYPES

// RecordedCommand is one Command call the double saw.
type RecordedCommand struct {

	// Run is the run the command was issued to.
	Run RunID

	// Command is the command.
	Command ControlCommand
}

// endregion
