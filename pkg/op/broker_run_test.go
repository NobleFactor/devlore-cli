// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"errors"
	"testing"
	"time"
)

// deliverWhenAsked waits until the double has seen one exec, then delivers the frames to the run.
//
// Parameters:
//   - `t`: the test.
//   - `broker`: the double.
//   - `run`: the run the frames go to.
//   - `frames`: what to deliver, in order.
func deliverWhenAsked(t *testing.T, broker *RecordingBroker, run RunID, frames ...ConsoleFrame) {

	t.Helper()
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for len(broker.Execs()) == 0 {
			if time.Now().After(deadline) {
				return
			}
			time.Sleep(time.Millisecond)
		}
		for _, frame := range frames {
			broker.Deliver(run, frame)
		}
	}()
}

// --- Run ---

// TestRun_CollectsUntilControl pins Run: it subscribes before it asks, collects the exec's frames in order, ignores
// another exec's, and returns at the control frame.
func TestRun_CollectsUntilControl(t *testing.T) {

	broker := NewRecordingBroker()
	deliverWhenAsked(t, broker, "r",
		NarrationFrame("e-1", "u", "warn", "one"),
		OutputFrame("e-9", []byte("not mine")),
		OutputFrame("e-1", []byte("out")),
		OutputFrame("e-1", []byte("put")),
		NarrationFrame("e-1", "u", "warn", "two"),
		ControlFrame("e-1", 4, nil),
		NarrationFrame("e-1", "u", "warn", "after the end"),
	)

	outcome, err := Run(context.Background(), broker, ExecRequest{Run: "r", Unit: "u", Account: "caller", Argv: []string{"x"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if outcome.Exec != "e-1" || outcome.ExitStatus != 4 || string(outcome.Output) != "output" {
		t.Errorf("outcome = %+v", outcome)
	}
	if outcome.Stderr() != "one\ntwo\n" {
		t.Errorf("Stderr() = %q", outcome.Stderr())
	}
}

// TestRun_Refusal pins that a refused exec returns the refusal itself, errors.As-able, with no wait.
func TestRun_Refusal(t *testing.T) {

	broker := NewRecordingBroker()
	broker.Refuse("root", &Refusal{Reason: "no", Account: "root"})

	_, err := Run(context.Background(), broker, ExecRequest{Run: "r", Unit: "u", Account: "root", Argv: []string{"x"}})

	var refusal *Refusal
	if !errors.As(err, &refusal) {
		t.Errorf("Run: got %v, want the refusal", err)
	}
}

// TestRun_ContextEnds pins that a context ending before the control frame ends the wait with the context's error.
func TestRun_ContextEnds(t *testing.T) {

	broker := NewRecordingBroker()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := Run(ctx, broker, ExecRequest{Run: "r", Unit: "u", Account: "caller", Argv: []string{"x"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run: got %v, want the deadline", err)
	}
}

// TestRun_NoBroker pins that a nil broker is an error, not a panic.
func TestRun_NoBroker(t *testing.T) {

	if _, err := Run(context.Background(), nil, ExecRequest{}); !errors.Is(err, errNoBroker) {
		t.Errorf("Run: got %v, want errNoBroker", err)
	}
}
