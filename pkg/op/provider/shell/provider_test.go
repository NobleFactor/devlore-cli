// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package shell

import (
	"context"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

// newTestProvider builds a provider on a runtime environment whose broker is the one the spec names, or the
// in-process one when nil, narrating to nowhere.
//
// Parameters:
//   - `t`: the test.
//   - `broker`: the broker, or nil for the in-process one.
//
// Returns:
//   - `*Provider`: the provider.
func newTestProvider(t *testing.T, broker op.Broker) *Provider {

	t.Helper()
	app := &application.Application{Name: "test"}
	spec := op.NewRuntimeEnvironmentSpec(app.Name).
		WithApplication(app).
		WithBroker(broker).
		WithStatus(status.NewNarrator(app.Name, sink.Discard()))

	runtimeEnvironment, err := op.NewRuntimeEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}
	return NewProvider(runtimeEnvironment)
}

// --- Exec ---

func TestExec_Success(t *testing.T) {
	p := newTestProvider(t, nil)

	result, err := p.Exec("echo hello")
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result == nil {
		t.Fatal("Exec() returned nil result")
	}
	if result.Command != "echo hello" {
		t.Errorf("result.Command = %q, want %q", result.Command, "echo hello")
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("result.Stdout = %q, want it to contain %q", result.Stdout, "hello")
	}
	if result.ExitCode != 0 {
		t.Errorf("result.ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestExec_EmptyCommand(t *testing.T) {
	p := newTestProvider(t, nil)

	result, err := p.Exec("")
	if err == nil {
		t.Fatal("Exec() with empty command should return error")
	}
	if !strings.Contains(err.Error(), "no command specified") {
		t.Errorf("error = %q, want message containing %q", err, "no command specified")
	}
	if result != nil {
		t.Errorf("result = %v, want nil on empty-command error", result)
	}
}

func TestExec_Failure(t *testing.T) {
	p := newTestProvider(t, nil)

	result, err := p.Exec("exit 1")
	if err == nil {
		t.Fatal("Exec() with 'exit 1' should return non-nil error")
	}
	if result == nil {
		t.Fatal("Exec() should still return a populated result on subprocess failure")
	}
	if result.ExitCode == 0 {
		t.Errorf("result.ExitCode = %d, want non-zero on subprocess failure", result.ExitCode)
	}
}

// TestExec_ThroughTheBroker pins that the command goes through the environment's broker as a caller script of
// dialect sh, the body the command itself, and that the result folds back from the frames.
func TestExec_ThroughTheBroker(t *testing.T) {

	broker := op.NewRecordingBroker()
	broker.Complete(0)
	p := newTestProvider(t, broker)

	result, err := p.Exec("echo through")
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if result.ExitCode != 0 || result.Command != "echo through" {
		t.Errorf("result = %+v", result)
	}

	execs := broker.Execs()
	if len(execs) != 1 {
		t.Fatalf("execs = %+v", execs)
	}
	exec := execs[0]
	if exec.Account != op.AccountCaller || exec.Script == nil || exec.Script.Dialect != "sh" || exec.Script.Body != "echo through" ||
		exec.Unit != "shell.exec" || len(exec.Argv) != 0 {
		t.Errorf("exec = %+v", exec)
	}
}

// TestExec_Refused pins that a broker's refusal is the error and there is no result.
func TestExec_Refused(t *testing.T) {

	broker := op.NewRecordingBroker()
	broker.Refuse(op.AccountCaller, &op.Refusal{Reason: "no", Account: op.AccountCaller})
	p := newTestProvider(t, broker)

	result, err := p.Exec("echo never")
	if err == nil || result != nil {
		t.Errorf("Exec() = %+v, %v; want nil and the refusal", result, err)
	}
}
