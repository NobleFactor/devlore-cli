// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import "testing"

// TestControlPlane_RequestResponse pins the commands-in round-trip: Request returns a response channel, poll drains
// the command on the executor side, and the response sent back reaches the original channel (the stream-id
// correlation).
func TestControlPlane_RequestResponse(t *testing.T) {

	p := NewControlPlane()

	response := p.Request(ControlPause)

	request, pending := p.poll()
	if !pending {
		t.Fatal("poll: want a pending request")
	}
	if request.cmd != ControlPause {
		t.Errorf("drained cmd = %s, want pause", request.cmd)
	}

	want := RunStatus{Phase: PhasePaused, Reason: ReasonPaused}
	request.response <- ControlResponse{Status: want}

	if got := <-response; got.Status != want {
		t.Errorf("response status = %+v, want %+v", got.Status, want)
	}

	if _, pending := p.poll(); pending {
		t.Error("poll: want no pending request after the drain")
	}
}

// TestControlPlane_Request_QueueFull pins the non-blocking Request contract: once the inbound buffer is full, Request
// still returns immediately, with an error response rather than blocking.
func TestControlPlane_Request_QueueFull(t *testing.T) {

	p := NewControlPlane()

	for range cap(p.requests) {
		p.Request(ControlPause) // fill the buffer; never drained
	}

	if got := <-p.Request(ControlStop); got.Err == nil {
		t.Error("Request on a full queue: want an error response, got nil")
	}
}

// TestControlPlane_SubscribeEmit pins the events-out fan-out: every live subscriber receives each emitted event with
// a monotonically increasing Seq, and cancel closes the subscription.
func TestControlPlane_SubscribeEmit(t *testing.T) {

	p := NewControlPlane()

	first, cancelFirst := p.Subscribe()
	second, cancelSecond := p.Subscribe()
	defer cancelSecond()

	p.emit(ControlEvent{Kind: EventPhaseChanged, Status: RunStatus{Phase: PhaseRunning}})

	for name, ch := range map[string]<-chan ControlEvent{"first": first, "second": second} {
		event := <-ch
		if event.Kind != EventPhaseChanged || event.Status.Phase != PhaseRunning {
			t.Errorf("%s: event = %+v, want phase_changed × running", name, event)
		}
		if event.Seq != 1 {
			t.Errorf("%s: Seq = %d, want 1", name, event.Seq)
		}
	}

	// Seq increments per emit, across subscribers.
	p.emit(ControlEvent{Kind: EventError})
	if event := <-second; event.Seq != 2 {
		t.Errorf("second emit Seq = %d, want 2", event.Seq)
	}

	// cancel removes the subscription and closes its channel; a later emit never reaches it.
	<-first // drain the Seq-2 event buffered on first
	cancelFirst()
	p.emit(ControlEvent{Kind: EventResult})
	if _, open := <-first; open {
		t.Error("first channel: want closed after cancel")
	}
}

// TestControlPlane_Emit_DropsSlowSubscriber pins the backpressure contract: emit is non-blocking, so a subscriber
// that stops draining fills its buffer and then drops — the run is never stalled by an observer.
func TestControlPlane_Emit_DropsSlowSubscriber(t *testing.T) {

	p := NewControlPlane()

	events, cancel := p.Subscribe()
	defer cancel()

	// Emit far past the buffer without ever draining. If emit blocked on a full subscriber this would deadlock.
	for range 100 {
		p.emit(ControlEvent{Kind: EventPhaseChanged})
	}

	if buffered := len(events); buffered != cap(events) {
		t.Errorf("buffered events = %d, want %d (drops beyond capacity)", buffered, cap(events))
	}
}
