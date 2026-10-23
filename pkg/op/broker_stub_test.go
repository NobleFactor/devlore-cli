// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build unix

package op

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

// newStubBroker builds a runtime environment whose broker is the in-process one, narrating to nowhere.
//
// Parameters:
//   - `t`: the test.
//   - `dryRun`: whether the environment's application says --dry-run.
//
// Returns:
//   - `*InProcessBroker`: the environment's broker.
func newStubBroker(t *testing.T, dryRun bool) *InProcessBroker {

	t.Helper()
	app := &application.Application{Name: "test", Flags: map[string]any{"dry_run": dryRun}}
	spec := NewRuntimeEnvironmentSpec(app.Name).
		WithApplication(app).
		WithStatus(status.NewNarrator(app.Name, sink.Discard()))

	runtimeEnvironment, err := NewRuntimeEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}

	broker, ok := runtimeEnvironment.Broker.(*InProcessBroker)
	if !ok {
		t.Fatalf("the environment's broker is %T, want *InProcessBroker", runtimeEnvironment.Broker)
	}
	return broker
}

// caller makes a caller exec of an argv on run r, unit u.
//
// Parameters:
//   - `argv`: the vector.
//
// Returns:
//   - `ExecRequest`: the request.
func caller(argv ...string) ExecRequest {
	return ExecRequest{Run: "r", Unit: "u", Account: AccountCaller, Argv: argv}
}

// --- Exec ---

// TestInProcessBroker_CallerArgv pins the direct path: an argv runs as the caller, its stdout is the output, each
// stderr line a narration frame naming the unit, and its exit status the control frame.
func TestInProcessBroker_CallerArgv(t *testing.T) {

	broker := newStubBroker(t, false)

	outcome, err := Run(context.Background(), broker, caller("sh", "-c", "printf hi; echo err >&2; exit 3"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if outcome.Exec != "e-1" || outcome.ExitStatus != 3 || string(outcome.Output) != "hi" || outcome.Error != "" {
		t.Errorf("outcome = %+v", outcome)
	}
	if len(outcome.Narration) != 1 || outcome.Narration[0].Text != "err" || outcome.Narration[0].Unit != "u" {
		t.Errorf("narration = %+v", outcome.Narration)
	}
}

// TestInProcessBroker_Script pins tier 2: the body is written to a file and run by sh with the args as positional
// parameters, never interpolated.
func TestInProcessBroker_Script(t *testing.T) {

	broker := newStubBroker(t, false)
	request := ExecRequest{
		Run: "r", Unit: "u", Account: AccountCaller,
		Script: &Script{Dialect: "sh", Body: `printf '%s-%s' "$1" "$2"`},
		Args:   []string{"a", "b; echo injected"},
	}

	outcome, err := Run(context.Background(), broker, request)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(outcome.Output) != "a-b; echo injected" || outcome.ExitStatus != 0 {
		t.Errorf("outcome = %+v", outcome)
	}

	request.Script.Dialect = "ksh"
	if _, err := broker.Exec(context.Background(), request); err == nil {
		t.Error("an unknown dialect: want an error")
	}
}

// TestInProcessBroker_StdinEnvCwd pins the request's three attributes: stdin bytes reach the child, Env joins its
// environment, Cwd is its directory.
func TestInProcessBroker_StdinEnvCwd(t *testing.T) {

	broker := newStubBroker(t, false)
	directory := t.TempDir()
	request := caller("sh", "-c", `cat; printf '%s' "$DEVLORE_TEST_X"; pwd -P`)
	request.Stdin = &Stdin{Bytes: []byte("in\n")}
	request.Env = map[string]string{"DEVLORE_TEST_X": "x"}
	request.Cwd = directory

	outcome, err := Run(context.Background(), broker, request)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	resolved, _ := filepath.EvalSymlinks(directory)
	if want := "in\nx" + resolved + "\n"; string(outcome.Output) != want {
		t.Errorf("output = %q, want %q", outcome.Output, want)
	}
}

// TestInProcessBroker_AutoConfirm pins the auto-confirm stdin: the child reads y after y, then end of file.
func TestInProcessBroker_AutoConfirm(t *testing.T) {

	broker := newStubBroker(t, false)
	request := caller("head", "-n", "2")
	request.Stdin = &Stdin{AutoConfirm: true}

	outcome, err := Run(context.Background(), broker, request)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(outcome.Output) != "y\ny\n" {
		t.Errorf("output = %q", outcome.Output)
	}
}

// TestInProcessBroker_LaunchFailure pins a child that cannot be launched: exit status -1 and the error on the
// control frame, never a hang.
func TestInProcessBroker_LaunchFailure(t *testing.T) {

	broker := newStubBroker(t, false)

	outcome, err := Run(context.Background(), broker, caller("/nonexistent/devlore-test-binary"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if outcome.ExitStatus != -1 || outcome.Error == "" {
		t.Errorf("outcome = %+v", outcome)
	}
}

// TestInProcessBroker_DryRun pins dry-run: the command is narrated and not launched, and the control frame says 0.
func TestInProcessBroker_DryRun(t *testing.T) {

	broker := newStubBroker(t, true)

	outcome, err := Run(context.Background(), broker, caller("sh", "-c", "exit 7"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if outcome.ExitStatus != 0 || len(outcome.Output) != 0 {
		t.Errorf("dry-run outcome = %+v", outcome)
	}
}

// TestInProcessBroker_Cancel pins Cancel: the child is killed and its end still arrives as a control frame.
func TestInProcessBroker_Cancel(t *testing.T) {

	broker := newStubBroker(t, false)
	frames, cancel := broker.Console("r")
	defer cancel()

	id, err := broker.Exec(context.Background(), caller("sh", "-c", "sleep 30"))
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for broker.Cancel(context.Background(), id) != nil {
		if time.Now().After(deadline) {
			t.Fatal("Cancel never found the child")
		}
		time.Sleep(10 * time.Millisecond)
	}

	for frame := range frames {
		if frame.Channel == ChannelControl {
			if frame.Control.ExitStatus != -1 {
				t.Errorf("a killed child's exit status = %d, want -1", frame.Control.ExitStatus)
			}
			return
		}
	}
	t.Fatal("the console closed before the control frame")
}

// --- The accounts ---

// TestInProcessBroker_Accounts pins which accounts the broker stands for: the caller with no prefix, root with
// `sudo -n` in front, anyone else refused by name.
func TestInProcessBroker_Accounts(t *testing.T) {

	prefix, err := accountPrefix(AccountCaller)
	if err != nil || prefix != nil {
		t.Errorf("caller: prefix %v, err %v", prefix, err)
	}

	prefix, err = accountPrefix(AccountRoot)
	if err != nil || strings.Join(prefix, " ") != "sudo -n" {
		t.Errorf("root: prefix %v, err %v", prefix, err)
	}

	argv, cleanup, err := argvFor(ExecRequest{Argv: []string{"apt-get", "update"}}, prefix)
	if err != nil || cleanup != nil || strings.Join(argv, " ") != "sudo -n apt-get update" {
		t.Errorf("root argv = %v, cleanup %v, err %v", argv, cleanup != nil, err)
	}

	broker := newStubBroker(t, false)
	request := caller("id")
	request.Account = "svc-media"
	_, err = broker.Exec(context.Background(), request)

	var refusal *Refusal
	if !errors.As(err, &refusal) || refusal.Account != "svc-media" {
		t.Errorf("another account: got %v, want a refusal naming it", err)
	}
}

// --- Post, Track, Command, Status ---

// TestInProcessBroker_PostRefused pins that a posted unit is refused by name until plan.logon lands.
func TestInProcessBroker_PostRefused(t *testing.T) {

	broker := newStubBroker(t, false)

	_, err := broker.Post(context.Background(), PostRequest{Run: "r", Unit: "u", Account: AccountRoot, Graph: []byte("{}")})

	var refusal *Refusal
	if !errors.As(err, &refusal) {
		t.Errorf("Post: got %v, want a refusal", err)
	}
}

// TestInProcessBroker_TrackCommandStatus pins the run registry: a tracked run's Command reaches its plane and its
// Status reads now; an unknown run is an error on both; untrack forgets it.
func TestInProcessBroker_TrackCommandStatus(t *testing.T) {

	broker := newStubBroker(t, false)
	plane := NewControlPlane()
	current := RunStatus{Phase: PhaseRunning}
	untrack := broker.Track("r", plane, func() RunStatus { return current })

	response := broker.Command("r", ControlPause)
	request, pending := plane.poll()
	if !pending || request.cmd != ControlPause {
		t.Fatal("Command did not reach the plane")
	}
	current = RunStatus{Phase: PhasePaused, Reason: ReasonPaused}
	request.response <- ControlResponse{Status: current}
	if got := <-response; got.Status.Phase != PhasePaused {
		t.Errorf("response = %+v", got)
	}

	if got, err := broker.Status("r"); err != nil || got.Phase != PhasePaused {
		t.Errorf("Status = %+v, %v", got, err)
	}

	untrack()
	if _, err := broker.Status("r"); err == nil {
		t.Error("after untrack: want an error")
	}
	if got := <-broker.Command("r", ControlStop); got.Err == nil {
		t.Error("after untrack: want an error response")
	}
}
