// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- Exec, Post, Cancel ---

// TestRecordingBroker_ExecRecordsAndNumbers pins that Exec and Post record their requests and number exec ids
// across both, and that a malformed request is refused by validation before it is recorded.
func TestRecordingBroker_ExecRecordsAndNumbers(t *testing.T) {

	broker := NewRecordingBroker()
	ctx := context.Background()

	first, err := broker.Exec(ctx, ExecRequest{Run: "r", Unit: "u-1", Account: "caller", Argv: []string{"true"}})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	second, err := broker.Post(ctx, PostRequest{Run: "r", Unit: "u-2", Account: "root", Graph: []byte("{}")})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if first != "e-1" || second != "e-2" {
		t.Errorf("ids = %s, %s; want e-1, e-2", first, second)
	}

	if _, err := broker.Exec(ctx, ExecRequest{Run: "r"}); err == nil {
		t.Error("a malformed exec: want an error")
	}

	if execs := broker.Execs(); len(execs) != 1 || execs[0].Unit != "u-1" {
		t.Errorf("Execs() = %+v", execs)
	}
	if posts := broker.Posts(); len(posts) != 1 || posts[0].Unit != "u-2" {
		t.Errorf("Posts() = %+v", posts)
	}

	if err := broker.Cancel(ctx, first); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancels := broker.Cancels(); len(cancels) != 1 || cancels[0] != first {
		t.Errorf("Cancels() = %v", cancels)
	}
}

// TestRecordingBroker_Refuse pins that a refused account fails Exec and Post with the refusal, errors.As-able, and
// that the request is still recorded, as a broker's audit would.
func TestRecordingBroker_Refuse(t *testing.T) {

	broker := NewRecordingBroker()
	broker.Refuse("root", &Refusal{Reason: "account root is not offered to uid 501 by this broker", Offer: "off_host_root"})

	_, err := broker.Exec(context.Background(), ExecRequest{Run: "r", Unit: "u", Account: "root", Argv: []string{"id"}})

	var refusal *Refusal
	if !errors.As(err, &refusal) || refusal.Offer != "off_host_root" {
		t.Fatalf("exec as root: got %v, want the refusal", err)
	}
	if len(broker.Execs()) != 1 {
		t.Error("a refused exec is not recorded")
	}
}

// --- Console, Events ---

// TestRecordingBroker_ConsoleDeliversInOrder pins the console stream: frames arrive in delivery order with the
// run's seq stamped from 1, two runs count independently, and cancel closes the channel.
func TestRecordingBroker_ConsoleDeliversInOrder(t *testing.T) {

	broker := NewRecordingBroker()

	frames, cancel := broker.Console("r-1")
	other, cancelOther := broker.Console("r-2")
	defer cancelOther()

	broker.Deliver("r-1", NarrationFrame("e-1", "u", "note", "one"))
	broker.Deliver("r-1", OutputFrame("e-1", []byte("two")))
	broker.Deliver("r-2", ControlFrame("e-9", 0, nil))
	broker.Deliver("r-1", ControlFrame("e-1", 0, nil))

	for want, channel := range []Channel{ChannelNarration, ChannelOutput, ChannelControl} {
		frame := <-frames
		if frame.Seq != uint64(want+1) || frame.Channel != channel {
			t.Errorf("frame %d = seq %d %s; want seq %d %s", want, frame.Seq, frame.Channel, want+1, channel)
		}
	}

	if frame := <-other; frame.Seq != 1 || frame.Channel != ChannelControl {
		t.Errorf("the other run's frame = seq %d %s; want seq 1 control", frame.Seq, frame.Channel)
	}

	cancel()
	cancel() // idempotent
	if _, open := <-frames; open {
		t.Error("cancel: the channel is still open")
	}
}

// TestRecordingBroker_EventsDeliver pins the events stream: an emitted event arrives with the run's events seq,
// which counts separately from the console's.
func TestRecordingBroker_EventsDeliver(t *testing.T) {

	broker := NewRecordingBroker()

	events, cancel := broker.Events("r")
	defer cancel()

	broker.Deliver("r", NarrationFrame("e-1", "u", "note", "console first"))
	broker.Emit("r", ControlEvent{Kind: EventPhaseChanged, Status: RunStatus{Phase: PhaseRunning}})

	if event := <-events; event.Seq != 1 || event.Kind != EventPhaseChanged {
		t.Errorf("event = seq %d kind %d; want seq 1 phase_changed", event.Seq, event.Kind)
	}
}

// TestRecordingBroker_SlowSubscriberReceivesEverything pins delivery that never drops (#1131): far more frames than
// any channel buffer are delivered to a subscriber that is not reading, each Deliver returns at once, and the
// subscriber then reads every frame, seq contiguous from 1.
func TestRecordingBroker_SlowSubscriberReceivesEverything(t *testing.T) {

	const total = 10000

	broker := NewRecordingBroker()
	frames, cancel := broker.Console("r")
	defer cancel()

	start := time.Now()
	for range total {
		broker.Deliver("r", OutputFrame("e-1", []byte("x")))
	}
	broker.Deliver("r", ControlFrame("e-1", 0, nil))
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Deliver blocked on the slow subscriber: %s for %d frames", elapsed, total+1)
	}

	for want := uint64(1); want <= total+1; want++ {
		frame := <-frames
		if frame.Seq != want {
			t.Fatalf("frame %d: seq %d", want, frame.Seq)
		}
	}
}

// --- Command, Status ---

// TestRecordingBroker_CommandAndStatus pins that Command records and answers with the run's status at once, that
// Status returns it, and that an unknown run is an error on both.
func TestRecordingBroker_CommandAndStatus(t *testing.T) {

	broker := NewRecordingBroker()
	broker.SetStatus("r", RunStatus{Phase: PhasePaused, Reason: ReasonPaused})

	if got := <-broker.Command("r", ControlPause); got.Err != nil || got.Status.Phase != PhasePaused {
		t.Errorf("command: %+v", got)
	}
	if commands := broker.Commands(); len(commands) != 1 || commands[0].Command != ControlPause {
		t.Errorf("Commands() = %+v", commands)
	}

	status, err := broker.Status("r")
	if err != nil || status.Phase != PhasePaused {
		t.Errorf("status: %+v, %v", status, err)
	}

	if got := <-broker.Command("nobody", ControlStop); got.Err == nil {
		t.Error("an unknown run: want an error response")
	}
	if _, err := broker.Status("nobody"); err == nil {
		t.Error("an unknown run: want an error")
	}
}
