// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Broker is the client interface to the DevLore broker — architecture 2.7's wire with no wire, and the one thing
// iteration 1 of the execution model fixes and never moves (feature #1128).
//
// A caller holds a Broker and nothing else: writ, the authz provider, the command provider and plan.logon are written
// against it. Behind it, in iteration 1, is an in-process stub (#1130, #1131); in iteration 2 the HTTP/2 facade of
// 2.7 § The wire, in detail, which carries exactly these requests and frames, marshaled by their JSON tags; in
// iteration 3 the runner of 6.2. Nothing above the interface changes between them.
//
// Three things cross it, as 2.7 § Dispatch and the fork and § The two streams describe:
//   - An exec, by account: [Broker.Exec] takes an [ExecRequest] and answers with an [ExecID] at once, the `202` of
//     `POST /v1/exec`, or with a [*Refusal], the named policy failure that is never a dialog. [Broker.Post] does the
//     same for a posted unit, the re-rooted graph that `plan.logon` dispatches (2.3 § Ruled 2026-10-22). Everything a
//     command then says arrives on the console.
//   - The two per-run streams, each a channel and a cancel, never blocking: [Broker.Console] carries `narration`,
//     `output` and `control` frames for the caller, [Broker.Events] the run's [ControlEvent]s for whoever watches.
//     Each stream has its own per-run `seq`; a gap in it is a dropped frame, never a reordered one.
//   - The control plane's commands by run id: [Broker.Command] is [ControlPlane.Request] for a run the caller does
//     not hold the executor of, and [Broker.Status] the plain poll of `GET /v1/runs/{id}`.
type Broker interface {

	// Exec asks the broker to run a command as an account. It returns the exec id the console frames carry, or a
	// *Refusal when the broker's policy does not offer the account to this caller.
	Exec(ctx context.Context, request ExecRequest) (ExecID, error)

	// Post asks the broker to run a posted unit, a re-rooted graph, as an account, on this machine or on a host. It
	// returns the exec id the console frames carry, or a *Refusal.
	Post(ctx context.Context, request PostRequest) (ExecID, error)

	// Cancel asks the runner to end one exec's child: SIGTERM, then SIGKILL; the exit arrives as a control frame.
	Cancel(ctx context.Context, exec ExecID) error

	// Console subscribes to a run's console stream: narration, output and control frames, seq-numbered per run, until
	// cancel is called, which closes the channel.
	Console(run RunID) (frames <-chan ConsoleFrame, cancel func())

	// Events subscribes to a run's events stream, seq-numbered per run, until cancel is called, which closes the
	// channel.
	Events(run RunID) (events <-chan ControlEvent, cancel func())

	// Command issues a control command to a run by id and returns its single-response channel; it never blocks.
	Command(run RunID, command ControlCommand) <-chan ControlResponse

	// Status returns a run's current status, the plain poll for a caller that does not stream.
	Status(run RunID) (RunStatus, error)
}

// region SUPPORTING TYPES

// RunID identifies a run to the broker, the `{runID}` of every per-run route.
type RunID string

// ExecID identifies one exec within a run; every console frame carries the id of the exec it came from, and it is
// the whole routing table of the return path (2.7 § Dispatch and the fork, step 2).
type ExecID string

// ExecRequest is the body of `POST /v1/exec`: the command, where it runs and as whom, and what feeds it (6.2 § 7).
//
// A command is an argv vector or a script with arguments, never a string handed to an interpreter's `-c` (6.3);
// exactly one of Argv and Script is set.
type ExecRequest struct {

	// Run is the run the exec belongs to; its console carries the frames.
	Run RunID `json:"run"`

	// Unit is the unit within the run that asked, named on every narration frame.
	Unit string `json:"unit"`

	// Account is the account the command runs as: `root`, `caller`, or a name. Its final form is 6.2 § Open 8; the
	// interface carries it as written.
	Account string `json:"account"`

	// Argv is tier 1 of 6.3: the vector the runner execs, no interpreter at all. Empty when Script is set.
	Argv []string `json:"argv,omitempty"`

	// Script is tier 2 of 6.3: a body the runner writes to a file and runs with its dialect's interpreter, Args as
	// positional parameters, never interpolated. Nil when Argv is set.
	Script *Script `json:"script,omitempty"`

	// Args are the positional parameters a Script receives.
	Args []string `json:"args,omitempty"`

	// Env is added to the child's environment.
	Env map[string]string `json:"env,omitempty"`

	// Cwd is the child's working directory; empty leaves it to the runner.
	Cwd string `json:"cwd,omitempty"`

	// Stdin is what the child reads: bytes, or the auto-confirm stream; nil closes stdin at once.
	Stdin *Stdin `json:"stdin,omitempty"`
}

// region EXPORTED METHODS

// region Behaviors

// Validate reports the first thing wrong with the request, so a broker refuses a malformed exec before it reaches a
// runner.
//
// Returns:
//   - `error`: nil when the request names a run, a unit and an account and carries exactly one of Argv and Script.
func (r ExecRequest) Validate() error {

	switch {
	case r.Run == "":
		return errors.New("exec: no run")
	case r.Unit == "":
		return errors.New("exec: no unit")
	case r.Account == "":
		return errors.New("exec: no account")
	case len(r.Argv) == 0 && r.Script == nil:
		return errors.New("exec: neither argv nor script")
	case len(r.Argv) > 0 && r.Script != nil:
		return errors.New("exec: both argv and script")
	case r.Script != nil && r.Script.Dialect == "":
		return errors.New("exec: script with no dialect")
	default:
		return nil
	}
}

// endregion

// endregion

// Script is a tier-2 command: a static body in a dialect, run from a file with positional arguments (6.3 § 3).
type Script struct {

	// Dialect names the interpreter the body is written for: `sh`, `pwsh`.
	Dialect string `json:"dialect"`

	// Body is the script, written to a file in the runner's scratch root and never interpolated.
	Body string `json:"body"`
}

// Stdin is what a child reads, an attribute of the request rather than a stream (6.2 § 4.1): a byte string, or the
// auto-confirm stream `pkg/platform` fed by hand.
//
// On the wire it is the string `auto_confirm`, or an object `{"bytes": "<base64>"}`.
type Stdin struct {

	// AutoConfirm feeds the child an endless `y\n`; set, Bytes is ignored.
	AutoConfirm bool

	// Bytes is what the child reads, then end of file.
	Bytes []byte
}

// region EXPORTED METHODS

// region Behaviors

// MarshalJSON renders the stdin as the wire shows it: the string `auto_confirm`, or `{"bytes": "<base64>"}`.
//
// Returns:
//   - `[]byte`: the JSON.
//   - `error`: non-nil when the bytes cannot be encoded, which they always can.
func (s Stdin) MarshalJSON() ([]byte, error) {

	if s.AutoConfirm {
		return json.Marshal(stdinAutoConfirm)
	}
	return json.Marshal(stdinBytes{Bytes: s.Bytes})
}

// UnmarshalJSON reads the two wire shapes MarshalJSON writes.
//
// Parameters:
//   - `data`: the JSON, a string or an object.
//
// Returns:
//   - `error`: non-nil when the JSON is neither `"auto_confirm"` nor `{"bytes": ...}`.
func (s *Stdin) UnmarshalJSON(data []byte) error {

	var word string
	if err := json.Unmarshal(data, &word); err == nil {
		if word != stdinAutoConfirm {
			return fmt.Errorf("stdin: unknown word %q", word)
		}
		*s = Stdin{AutoConfirm: true}
		return nil
	}

	var object stdinBytes
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("stdin: %w", err)
	}
	*s = Stdin{Bytes: object.Bytes}
	return nil
}

// endregion

// endregion

// PostRequest is the body of the posted unit (2.7 § The remote leg): a graph re-rooted from the body of a
// `plan.logon` subgraph, its inputs resolved to literals, run as an account on this machine or on a host. The
// control frame that ends it carries the unit's stamped stack.
type PostRequest struct {

	// Run is the run the unit belongs to; its console carries the frames.
	Run RunID `json:"run"`

	// Unit is the logon subgraph that posted, named on the control frame that ends it.
	Unit string `json:"unit"`

	// Account is the account the posted graph runs as, the logon's.
	Account string `json:"account"`

	// Host is the machine whose broker runs it; empty is this one.
	Host string `json:"host,omitempty"`

	// Graph is the re-rooted unit as a graph document (architecture 2 § Serialization), base64 on the wire.
	Graph []byte `json:"graph"`

	// Signature is the coordinator's signature over Graph (architecture 5, tier 2), which the broker verifies before
	// it runs anything; empty is unsigned, which a broker may refuse.
	Signature []byte `json:"signature,omitempty"`
}

// region EXPORTED METHODS

// region Behaviors

// Validate reports the first thing wrong with the request.
//
// Returns:
//   - `error`: nil when the request names a run, a unit and an account and carries a graph.
func (r PostRequest) Validate() error {

	switch {
	case r.Run == "":
		return errors.New("post: no run")
	case r.Unit == "":
		return errors.New("post: no unit")
	case r.Account == "":
		return errors.New("post: no account")
	case len(r.Graph) == 0:
		return errors.New("post: no graph")
	default:
		return nil
	}
}

// endregion

// endregion

// Refusal is the named policy failure of `POST /v1/exec` (2.7 § Authorization at the socket): the broker does not
// offer the account to this caller. It is an error a caller discriminates with errors.As, and it reaches the unit as
// the failure its ErrorAction handles; it is never a dialog.
type Refusal struct {

	// Reason is the sentence, e.g. "account root is not offered to uid 501 by this broker".
	Reason string `json:"error"`

	// Offer names the offer the policy consulted, when one was.
	Offer string `json:"offer,omitempty"`

	// Account is the account that was asked for.
	Account string `json:"account,omitempty"`
}

// region EXPORTED METHODS

// region Behaviors

// Error returns the refusal's sentence.
//
// Returns:
//   - `string`: Reason.
func (r *Refusal) Error() string {
	return r.Reason
}

// endregion

// endregion

// stdinBytes is the object form of a Stdin on the wire.
type stdinBytes struct {
	Bytes []byte `json:"bytes"`
}

// endregion

// stdinAutoConfirm is the word form of a Stdin on the wire.
const stdinAutoConfirm = "auto_confirm"
