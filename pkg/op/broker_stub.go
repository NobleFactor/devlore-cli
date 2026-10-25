// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// Interface Guard: the in-process broker is a Broker, at compile time.
var _ Broker = (*InProcessBroker)(nil)

// InProcessBroker is the [Broker] that lives in the app's process and forks commands from it: iteration 1's stub,
// and the direct path of 2.7 that outlives it.
//
// The owner, 2026-10-22: "we launch commands from the app process space. and this is a good optimization for when
// you're not in a login." A command for the caller's own account needs no logon and no policy, so it runs here, as
// the runtime environment's [process.Runner] runs it today, with the same frames a runner daemon would send: that is
// the direct path, kept. A command for `root` runs here with `sudo -n` in front of it, 6.2 § 10's interim row and
// the one place the word appears in this module; iteration 3 replaces that path with the root runner behind the
// broker daemon. Any other account is refused with a named [Refusal], because without a broker daemon there is no
// logon to make.
//
// Frames reach the run's console as the child speaks, one narration frame per write to stderr, one output frame per
// write to stdout, one control frame at exit; a subscriber that has fallen behind misses frames, since the spool
// that never drops is the runner's (6.2 § 4.1). Events, the event sink and the never-blocked child are lane 3's
// (#1131).
type InProcessBroker struct {

	// mu guards everything below.
	mu sync.Mutex

	// runtimeEnvironment supplies the process runner: its context, dry-run flag, pipeline and Narrator.
	runtimeEnvironment *RuntimeEnvironment

	// next numbers the exec ids: e-1, e-2, ...
	next int

	// children holds each running exec's cancel, which ends its child; removed at exit. The child itself is the run
	// goroutine's alone: its Process is written by Start there, and nothing else reads it.
	children map[ExecID]context.CancelFunc

	// runs is the registry Command and Status answer from: a run this process holds, by id.
	runs map[RunID]trackedRun

	// consoles and events are the per-run streams, created on first subscription or delivery.
	consoles map[RunID]*brokerStream[ConsoleFrame]
	events   map[RunID]*brokerStream[ControlEvent]
}

// NewInProcessBroker returns a broker bound to a runtime environment, whose process runner forks its commands.
//
// Parameters:
//   - `runtimeEnvironment`: the environment; its Context ends every child, its dry-run flag skips them.
//
// Returns:
//   - `*InProcessBroker`: the broker.
func NewInProcessBroker(runtimeEnvironment *RuntimeEnvironment) *InProcessBroker {
	return &InProcessBroker{
		runtimeEnvironment: runtimeEnvironment,
		children:           make(map[ExecID]context.CancelFunc),
		runs:               make(map[RunID]trackedRun),
		consoles:           make(map[RunID]*brokerStream[ConsoleFrame]),
		events:             make(map[RunID]*brokerStream[ControlEvent]),
	}
}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Cancel ends one exec's child by canceling the exec's context, which kills it; its exit then arrives as a
// control frame like any other, or a launch failure does when it had not started yet.
//
// Parameters:
//   - `ctx`: unused.
//   - `id`: the exec.
//
// Returns:
//   - `error`: non-nil when no such exec is running.
func (b *InProcessBroker) Cancel(_ context.Context, id ExecID) error {

	b.mu.Lock()
	cancel, running := b.children[id]
	b.mu.Unlock()

	if !running {
		return fmt.Errorf("broker: no running exec %q", id)
	}
	cancel()
	return nil
}

// Exec forks the command from this process, as the caller or as root through `sudo -n`, and returns its exec id
// at once; the child's narration, output and exit follow on the run's console.
//
// Parameters:
//   - `ctx`: ends the child when it ends.
//   - `request`: the exec; `Account` is `caller` or `root`.
//
// Returns:
//   - `ExecID`: e-<n>.
//   - `error`: the request's validation error, a *Refusal for an account this broker cannot stand for, or why the
//     command could not be prepared.
func (b *InProcessBroker) Exec(ctx context.Context, request ExecRequest) (ExecID, error) {

	if err := request.Validate(); err != nil {
		return "", err
	}

	prefix, err := accountPrefix(request.Account)
	if err != nil {
		return "", err
	}

	argv, cleanup, err := argvFor(request, prefix)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(ctx)

	b.mu.Lock()
	b.next++
	id := ExecID(fmt.Sprintf("e-%d", b.next))
	b.children[id] = cancel
	b.mu.Unlock()

	go b.run(ctx, id, request, argv, cleanup)
	return id, nil
}

// Post refuses: the posted unit arrives with plan.logon (2.3 § Ruled 2026-10-22), lane 9 of #1127.
//
// Parameters:
//   - `ctx`: unused.
//   - `request`: the posted unit.
//
// Returns:
//   - `ExecID`: empty.
//   - `error`: the request's validation error, or the named refusal.
func (b *InProcessBroker) Post(_ context.Context, request PostRequest) (ExecID, error) {

	if err := request.Validate(); err != nil {
		return "", err
	}
	return "", &Refusal{
		Reason:  "posted units are not available in the in-process broker until plan.logon lands",
		Account: request.Account,
	}
}

// Status returns the status of a run this process holds.
//
// Parameters:
//   - `run`: the run.
//
// Returns:
//   - `RunStatus`: its status, read now.
//   - `error`: non-nil when the run is not tracked here.
func (b *InProcessBroker) Status(run RunID) (RunStatus, error) {

	b.mu.Lock()
	tracked, known := b.runs[run]
	b.mu.Unlock()

	if !known {
		return RunStatus{}, fmt.Errorf("broker: unknown run %q", run)
	}
	return tracked.status(), nil
}

// Actions

// Command issues the command to the control plane of a run this process holds; an unknown run answers with an
// error at once.
//
// Parameters:
//   - `run`: the run.
//   - `command`: the command.
//
// Returns:
//   - `<-chan ControlResponse`: the plane's single-response channel.
func (b *InProcessBroker) Command(run RunID, command ControlCommand) <-chan ControlResponse {

	b.mu.Lock()
	tracked, known := b.runs[run]
	b.mu.Unlock()

	if !known {
		response := make(chan ControlResponse, 1)
		response <- ControlResponse{Err: fmt.Errorf("broker: unknown run %q", run)}
		return response
	}
	return tracked.plane.Request(command)
}

// Console subscribes to the run's console stream.
//
// Parameters:
//   - `run`: the run.
//
// Returns:
//   - `<-chan ConsoleFrame`: every frame delivered after the call, in order, seq stamped.
//   - `func()`: the unsubscribe; idempotent; closes the channel.
func (b *InProcessBroker) Console(run RunID) (frames <-chan ConsoleFrame, cancel func()) {

	b.mu.Lock()
	defer b.mu.Unlock()
	return b.console(run).subscribe(&b.mu)
}

// Events subscribes to the run's events stream.
//
// Parameters:
//   - `run`: the run.
//
// Returns:
//   - `<-chan ControlEvent`: every event emitted after the call, in order, seq stamped.
//   - `func()`: the unsubscribe; idempotent; closes the channel.
func (b *InProcessBroker) Events(run RunID) (events <-chan ControlEvent, cancel func()) {

	b.mu.Lock()
	defer b.mu.Unlock()
	return b.event(run).subscribe(&b.mu)
}

// Track registers a run this process holds, so Command and Status can reach it by id, and returns the untrack.
//
// Parameters:
//   - `run`: the run's id.
//   - `plane`: the run's control plane.
//   - `status`: reads the run's status now.
//
// Returns:
//   - `func()`: forgets the run; idempotent.
func (b *InProcessBroker) Track(run RunID, plane *ControlPlane, status func() RunStatus) (untrack func()) {

	b.mu.Lock()
	b.runs[run] = trackedRun{plane: plane, status: status}
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.runs, run)
	}
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
func (b *InProcessBroker) console(run RunID) *brokerStream[ConsoleFrame] {

	stream, known := b.consoles[run]
	if !known {
		stream = newBrokerStream[ConsoleFrame]()
		b.consoles[run] = stream
	}
	return stream
}

// deliver stamps the frame with the run's next console seq and fans it out.
//
// Parameters:
//   - `run`: the run.
//   - `frame`: the frame; its Seq is overwritten.
func (b *InProcessBroker) deliver(run RunID, frame ConsoleFrame) {

	b.mu.Lock()
	defer b.mu.Unlock()
	stream := b.console(run)
	frame.Seq = stream.next()
	stream.publish(frame)
}

// event returns the run's events stream, creating it. The caller holds mu.
//
// Parameters:
//   - `run`: the run.
//
// Returns:
//   - `*brokerStream[ControlEvent]`: the stream.
func (b *InProcessBroker) event(run RunID) *brokerStream[ControlEvent] {

	stream, known := b.events[run]
	if !known {
		stream = newBrokerStream[ControlEvent]()
		b.events[run] = stream
	}
	return stream
}

// run forks the child through the environment's process runner, frames what it says onto the run's console, and
// ends with a control frame. It runs on its own goroutine, one per exec.
//
// Parameters:
//   - `ctx`: the exec's own context; Cancel ends it, and the child with it.
//   - `id`: the exec id the frames carry.
//   - `request`: the exec.
//   - `argv`: the vector to fork, the account's prefix included.
//   - `cleanup`: removes the script file, when there is one; nil otherwise.
func (b *InProcessBroker) run(ctx context.Context, id ExecID, request ExecRequest, argv []string, cleanup func()) {

	if cleanup != nil {
		defer cleanup()
	}
	defer func() {
		b.mu.Lock()
		cancel := b.children[id]
		delete(b.children, id)
		b.mu.Unlock()
		cancel()
	}()

	child := commandFor(ctx, request, argv)

	stdout := &outputWriter{broker: b, run: request.Run, exec: id}
	narration := &narrationWriter{broker: b, run: request.Run, exec: id, unit: request.Unit}

	launched, err := b.runtimeEnvironment.runner().Stream(child, stdout, narration)

	status := 0
	switch {
	case err != nil:
		status = -1
	case launched && child.ProcessState != nil:
		status = child.ProcessState.ExitCode()
	}
	b.deliver(request.Run, ControlFrame(id, status, err))
}

// endregion

// endregion

// region SUPPORTING TYPES

// trackedRun is a run this process holds: its plane, and how to read its status.
type trackedRun struct {
	plane  *ControlPlane
	status func() RunStatus
}

// narrationWriter turns each write of a child's stderr into a narration frame on the run's console, the bytes as the
// child wrote them (ruled 2026-10-23).
type narrationWriter struct {
	broker *InProcessBroker
	run    RunID
	exec   ExecID
	unit   string
}

// region EXPORTED METHODS

// region Behaviors

// Write delivers the bytes as one narration frame.
//
// Parameters:
//   - `p`: the bytes the child wrote.
//
// Returns:
//   - `int`: len(p).
//   - `error`: always nil.
func (w *narrationWriter) Write(p []byte) (int, error) {

	w.broker.deliver(w.run, NarrationFrame(w.exec, w.unit, bytes.Clone(p)))
	return len(p), nil
}

// endregion

// endregion

// outputWriter turns each write of a child's stdout into an output frame on the run's console.
type outputWriter struct {
	broker *InProcessBroker
	run    RunID
	exec   ExecID
}

// region EXPORTED METHODS

// region Behaviors

// Write delivers the bytes as one output frame.
//
// Parameters:
//   - `p`: the bytes the child wrote.
//
// Returns:
//   - `int`: len(p).
//   - `error`: always nil.
func (w *outputWriter) Write(p []byte) (int, error) {

	w.broker.deliver(w.run, OutputFrame(w.exec, bytes.Clone(p)))
	return len(p), nil
}

// endregion

// endregion

// endregion

// region HELPER FUNCTIONS

// accountPrefix returns what is put in front of the argv for an account this broker can stand for.
//
// Parameters:
//   - `account`: `caller` or `root`.
//
// Returns:
//   - `[]string`: nothing for the caller; `sudo -n` for root on Unix, the one place the word appears.
//   - `error`: a *Refusal for any other account, and for root on Windows, where the interim elevation is a UAC
//     dialog this broker never raises (6.2 § 10).
func accountPrefix(account string) ([]string, error) {

	switch account {
	case AccountCaller:
		return nil, nil
	case AccountRoot:
		if runtime.GOOS == "windows" {
			return nil, &Refusal{Reason: "account root is not available without a broker on Windows", Account: account}
		}
		return []string{"sudo", "-n"}, nil
	default:
		return nil, &Refusal{Reason: fmt.Sprintf("account %s is not available without a broker", account), Account: account}
	}
}

// argvFor builds the vector to fork: the account's prefix, then the request's argv, or the interpreter of the
// script's dialect on the file the body was written to, then the args.
//
// Parameters:
//   - `request`: the exec.
//   - `prefix`: the account's prefix.
//
// Returns:
//   - `[]string`: the vector.
//   - `func()`: removes the script's directory; nil for an argv request.
//   - `error`: non-nil for an unknown dialect, or when the script file cannot be written.
func argvFor(request ExecRequest, prefix []string) (argv []string, cleanup func(), err error) {

	if request.Script == nil {
		return append(append([]string(nil), prefix...), request.Argv...), nil, nil
	}

	var interpreter []string
	switch request.Script.Dialect {
	case "sh":
		interpreter = []string{"sh"}
	case "pwsh":
		interpreter = []string{"pwsh", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File"}
	default:
		return nil, nil, fmt.Errorf("exec: unknown script dialect %q", request.Script.Dialect)
	}

	directory, err := os.MkdirTemp("", "devlore-exec-")
	if err != nil {
		return nil, nil, fmt.Errorf("exec: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(directory) } //nolint:errcheck // a scratch directory; nothing to do if it stays

	file := filepath.Join(directory, "script."+request.Script.Dialect)
	if err := os.WriteFile(file, []byte(request.Script.Body), 0o600); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("exec: %w", err)
	}

	argv = append(append([]string(nil), prefix...), interpreter...)
	argv = append(argv, file)
	argv = append(argv, request.Args...)
	return argv, cleanup, nil
}

// commandFor prepares the child from the vector and the request's environment, directory and stdin.
//
// Parameters:
//   - `ctx`: the child's context.
//   - `request`: the exec.
//   - `argv`: the vector to fork.
//
// Returns:
//   - `*exec.Cmd`: the prepared child; its streams are the runner's to set.
func commandFor(ctx context.Context, request ExecRequest, argv []string) *exec.Cmd {

	child := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // G204: argv from the request, never parsed
	child.Dir = request.Cwd

	// A killed child may leave grandchildren holding its pipes: dash forks `sleep` where bash execs it. Without a
	// bound, Wait would block until they exit, and a cancel would hang the broker. The runner of iteration 3 owns the
	// whole tree (6.2 § 4.3's Job Object, a process group on Unix); until then the pipes are abandoned after this.
	child.WaitDelay = pipeWaitDelay

	if len(request.Env) > 0 {
		child.Env = os.Environ()
		for name, value := range request.Env {
			child.Env = append(child.Env, name+"="+value)
		}
	}

	switch {
	case request.Stdin == nil:
	case request.Stdin.AutoConfirm:
		child.Stdin = strings.NewReader(strings.Repeat("y\n", autoConfirmReplies))
	default:
		child.Stdin = bytes.NewReader(request.Stdin.Bytes)
	}
	return child
}

// endregion

// Account names the in-process broker stands for, the platform's (6.2 § Open 8 has their final form).
const (

	// AccountCaller is the process's own account: the direct path, no logon, no policy.
	AccountCaller = platform.AccountCaller

	// AccountRoot is root: `sudo -n` here, the root runner in iteration 3.
	AccountRoot = platform.AccountRoot
)

// pipeWaitDelay bounds how long an exec waits for a killed child's pipes to close behind its grandchildren.
const pipeWaitDelay = 2 * time.Second

// autoConfirmReplies is how many `y` answers an auto-confirm stdin carries: far more than any command asks, then EOF.
// What pkg/platform fed by hand before the broker (6.2 § Open 4).
const autoConfirmReplies = 4096

var errNoBroker = errors.New("broker: none on the runtime environment")
