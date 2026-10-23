// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
)

// region EXPORTED FUNCTIONS

// Run execs a command through a broker and waits for it: it subscribes to the run's console before asking, so no
// frame is missed, then collects the exec's frames until its control frame.
//
// The synchronous shape every provider needs until it streams for itself: the shell provider's result, a package
// manager's captured output. Narration is collected, not rendered; the caller renders it, or does not.
//
// Parameters:
//   - `ctx`: ends the wait, and the child with it.
//   - `broker`: the broker.
//   - `request`: the exec.
//
// Returns:
//   - `Outcome`: the exit status, the output and the narration, in order.
//   - `error`: the exec's refusal or validation error, the context's end, or a broker that closed the console.
func Run(ctx context.Context, broker Broker, request ExecRequest) (Outcome, error) {

	if broker == nil {
		return Outcome{}, errNoBroker
	}

	frames, cancel := broker.Console(request.Run)
	defer cancel()

	exec, err := broker.Exec(ctx, request)
	if err != nil {
		return Outcome{}, err
	}

	outcome := Outcome{Exec: exec}

	for {
		select {
		case <-ctx.Done():
			return outcome, ctx.Err()
		case frame, open := <-frames:
			if !open {
				return outcome, fmt.Errorf("broker: console of run %q closed before exec %s ended", request.Run, exec)
			}
			if frame.Exec() != exec {
				continue
			}
			switch frame.Channel {
			case ChannelNarration:
				outcome.Narration = append(outcome.Narration, *frame.Narration)
			case ChannelOutput:
				outcome.Output = append(outcome.Output, frame.Output.Bytes...)
			case ChannelControl:
				outcome.ExitStatus = frame.Control.ExitStatus
				outcome.Error = frame.Control.Error
				return outcome, nil
			}
		}
	}
}

// endregion

// region SUPPORTING TYPES

// Outcome is what [Run] collected of one exec.
type Outcome struct {

	// Exec is the exec's id.
	Exec ExecID

	// ExitStatus is the child's exit status; -1 when it could not be launched or was killed.
	ExitStatus int

	// Output is everything the child wrote to stdout, in order.
	Output []byte

	// Narration is every line the child wrote to stderr, in order.
	Narration []Narration

	// Error is what ended the exec other than its child's exit, when something did.
	Error string
}

// region EXPORTED METHODS

// region Behaviors

// Stderr joins the narration lines with newlines, for a caller that wants the child's stderr as one string.
//
// Returns:
//   - `string`: the lines, each followed by a newline; empty when there were none.
func (o Outcome) Stderr() string {

	var stderr []byte
	for _, line := range o.Narration {
		stderr = append(stderr, line.Text...)
		stderr = append(stderr, '\n')
	}
	return string(stderr)
}

// endregion

// endregion

// endregion
