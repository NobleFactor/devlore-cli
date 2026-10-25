// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"fmt"
	"io"
)

// region EXPORTED FUNCTIONS

// Run execs a command through a broker and waits for it: it subscribes to the run's console before asking, so no
// frame is missed, then collects the exec's frames until its control frame.
//
// The synchronous shape every provider needs until it streams for itself: the shell provider's result, a package
// manager's captured output. Narration is collected, and written as it arrives to the writer [WithNarrator] names,
// which is the caller's narration sink.
//
// Parameters:
//   - `ctx`: ends the wait, and the child with it.
//   - `broker`: the broker.
//   - `request`: the exec.
//   - `options`: [WithNarrator], to write narration as it arrives.
//
// Returns:
//   - `Outcome`: the exit status, the output and the narration, in order.
//   - `error`: the exec's refusal or validation error, the context's end, or a broker that closed the console.
func Run(ctx context.Context, broker Broker, request ExecRequest, options ...RunOption) (Outcome, error) {

	var settings runSettings
	for _, option := range options {
		option(&settings)
	}

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
				outcome.Narration = append(outcome.Narration, frame.Narration.Bytes...)
				if settings.narrator != nil {
					_, _ = settings.narrator.Write(frame.Narration.Bytes) //nolint:errcheck // narration is best-effort
				}
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

// WithNarrator makes [Run] write each narration frame's bytes to the writer as the frame arrives: the caller's narration
// sink, normally its [status.Narrator], so a child's stderr shows as it is written and the caller's `--silent` governs
// it.
//
// Parameters:
//   - `narrator`: the writer; nil writes nothing.
//
// Returns:
//   - `RunOption`: the option.
func WithNarrator(narrator io.Writer) RunOption {
	return func(settings *runSettings) {
		settings.narrator = narrator
	}
}

// region SUPPORTING TYPES

// RunOption configures a [Run].
type RunOption func(*runSettings)

// runSettings is what the options of a [Run] set.
type runSettings struct {
	narrator io.Writer // where narration is written as it arrives; nil for nowhere
}

// Outcome is what [Run] collected of one exec.
type Outcome struct {

	// Exec is the exec's id.
	Exec ExecID

	// ExitStatus is the child's exit status; -1 when it could not be launched or was killed.
	ExitStatus int

	// Output is everything the child wrote to stdout, in order.
	Output []byte

	// Narration is everything the child wrote to stderr, in order, unaltered.
	Narration []byte

	// Error is what ended the exec other than its child's exit, when something did.
	Error string
}

// region EXPORTED METHODS

// region Behaviors

// Stderr returns everything the child wrote to stderr, as a string.
//
// Returns:
//   - `string`: the narration, unaltered.
func (o Outcome) Stderr() string {
	return string(o.Narration)
}

// endregion

// endregion

// endregion
