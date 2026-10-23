// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"encoding/json"
	"errors"
)

// ConsoleFrame is one frame of a run's console stream (2.7 § The two streams): what a command said, as it said it,
// for the caller. Exactly one of Narration, Output and Control is set, and Channel says which.
//
// On the wire a frame is one SSE event: `event:` is Channel, `id:` is Seq, `data:` is the payload's JSON, which
// [ConsoleFrame.Data] renders (2.7 § The wire, in detail). Events are not console frames: they travel on the events
// stream as [ControlEvent]s, and a devlore-aware child's events arrive there from fd 3 (6.2 § Open 9).
type ConsoleFrame struct {

	// Seq is the per-run monotonic sequence, stamped by the broker on delivery; a gap is a dropped frame.
	Seq uint64

	// Channel names the payload set: narration, output or control.
	Channel Channel

	// Narration is one line a command wrote to stderr, for the caller's Narrator; set when Channel is narration.
	Narration *Narration

	// Output is a chunk a command wrote to stdout, for the unit's result; set when Channel is output.
	Output *Output

	// Control is the exec's end; set when Channel is control.
	Control *Control
}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Data renders the frame's payload as the `data:` field of its SSE event.
//
// Returns:
//   - `[]byte`: the payload's JSON.
//   - `error`: non-nil when no payload matches Channel.
func (f ConsoleFrame) Data() ([]byte, error) {

	switch {
	case f.Channel == ChannelNarration && f.Narration != nil:
		return json.Marshal(f.Narration)
	case f.Channel == ChannelOutput && f.Output != nil:
		return json.Marshal(f.Output)
	case f.Channel == ChannelControl && f.Control != nil:
		return json.Marshal(f.Control)
	default:
		return nil, errors.New("console frame: channel " + string(f.Channel) + " has no payload")
	}
}

// Actions

// Exec returns the exec the frame came from, whichever payload carries it.
//
// Returns:
//   - `ExecID`: the exec id; empty when the frame has no payload.
func (f ConsoleFrame) Exec() ExecID {

	switch {
	case f.Narration != nil:
		return f.Narration.Exec
	case f.Output != nil:
		return f.Output.Exec
	case f.Control != nil:
		return f.Control.Exec
	default:
		return ""
	}
}

// endregion

// endregion

// region EXPORTED FUNCTIONS

// ControlFrame makes the frame that ends an exec. Seq is left for the broker to stamp.
//
// Parameters:
//   - `exec`: the exec that ended.
//   - `exitStatus`: the child's exit status.
//   - `err`: what ended the exec other than its child's exit, when something did; nil otherwise.
//
// Returns:
//   - `ConsoleFrame`: the control frame.
func ControlFrame(exec ExecID, exitStatus int, err error) ConsoleFrame {

	control := &Control{Exec: exec, ExitStatus: exitStatus}
	if err != nil {
		control.Error = err.Error()
	}
	return ConsoleFrame{Channel: ChannelControl, Control: control}
}

// NarrationFrame makes a narration frame. Seq is left for the broker to stamp.
//
// Parameters:
//   - `exec`: the exec that spoke.
//   - `unit`: the unit that asked for the exec.
//   - `level`: the narration level, as the caller's Narrator names them.
//   - `text`: the line, without its newline.
//
// Returns:
//   - `ConsoleFrame`: the narration frame.
func NarrationFrame(exec ExecID, unit, level, text string) ConsoleFrame {
	return ConsoleFrame{Channel: ChannelNarration, Narration: &Narration{Exec: exec, Unit: unit, Level: level, Text: text}}
}

// OutputFrame makes an output frame. Seq is left for the broker to stamp.
//
// Parameters:
//   - `exec`: the exec that wrote.
//   - `bytes`: the chunk, undecoded.
//
// Returns:
//   - `ConsoleFrame`: the output frame.
func OutputFrame(exec ExecID, bytes []byte) ConsoleFrame {
	return ConsoleFrame{Channel: ChannelOutput, Output: &Output{Exec: exec, Bytes: bytes}}
}

// endregion

// region SUPPORTING TYPES

// Channel names what a console frame carries; a string, as every enum of the execution model is (6.1 § Ruled
// 2026-10-21, 2).
type Channel string

// Channel values, the `event:` names of the console stream.
const (

	// ChannelNarration is a line a command wrote to stderr: text, one frame per line.
	ChannelNarration Channel = "narration"

	// ChannelOutput is a chunk a command wrote to stdout: bytes, undecoded.
	ChannelOutput Channel = "output"

	// ChannelControl is the exec's end: its exit status, never inferred from a stream closing.
	ChannelControl Channel = "control"
)

// Narration is the payload of a narration frame: `{"exec":"e-41","unit":"u-12","level":"note","text":"..."}`.
type Narration struct {

	// Exec is the exec that spoke.
	Exec ExecID `json:"exec"`

	// Unit is the unit that asked for the exec.
	Unit string `json:"unit,omitempty"`

	// Level is the narration level, as the caller's Narrator names them: note, warn, error, and the rest.
	Level string `json:"level"`

	// Text is the line, without its newline; invalid UTF-8 replaced by the runner.
	Text string `json:"text"`
}

// Output is the payload of an output frame: `{"exec":"e-41","len":4096,"bytes":"<base64>"}`.
type Output struct {

	// Exec is the exec that wrote.
	Exec ExecID `json:"exec"`

	// Bytes is the chunk, undecoded; base64 on the wire, with its length beside it.
	Bytes []byte `json:"-"`
}

// region EXPORTED METHODS

// region Behaviors

// MarshalJSON renders the output as the wire shows it, the length beside the bytes.
//
// Returns:
//   - `[]byte`: the JSON.
//   - `error`: non-nil when the bytes cannot be encoded, which they always can.
func (o Output) MarshalJSON() ([]byte, error) {
	return json.Marshal(outputWire{Exec: o.Exec, Len: len(o.Bytes), Bytes: o.Bytes})
}

// UnmarshalJSON reads the wire form MarshalJSON writes.
//
// Parameters:
//   - `data`: the JSON.
//
// Returns:
//   - `error`: non-nil when the JSON is not an output payload, or its length disagrees with its bytes.
func (o *Output) UnmarshalJSON(data []byte) error {

	var wire outputWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Len != len(wire.Bytes) {
		return errors.New("output: len disagrees with bytes")
	}
	*o = Output{Exec: wire.Exec, Bytes: wire.Bytes}
	return nil
}

// endregion

// endregion

// Control is the payload of a control frame: `{"exec":"e-41","exit_status":0}`, the end of an exec. For a posted
// unit it also carries the unit's stamped stack, as the trace serializes it.
type Control struct {

	// Exec is the exec that ended.
	Exec ExecID `json:"exec"`

	// ExitStatus is the child's exit status; for a posted unit, 0 when its run completed and 1 when it stopped.
	ExitStatus int `json:"exit_status"`

	// Error is what ended the exec other than its child's exit, when something did: a cancel, a runner failure.
	Error string `json:"error,omitempty"`

	// Outcome is the posted unit's stamped stack, serialized as the trace serializes it (5.2); absent for a command.
	Outcome json.RawMessage `json:"outcome,omitempty"`
}

// outputWire is the wire form of an Output.
type outputWire struct {
	Exec  ExecID `json:"exec"`
	Len   int    `json:"len"`
	Bytes []byte `json:"bytes"`
}

// endregion
