// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package op

import (
	"context"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/platform"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
	"github.com/NobleFactor/devlore-cli/pkg/status"
)

// newRecordedEnvironment builds a runtime environment whose broker is a double, narrating to nowhere.
//
// Parameters:
//   - `t`: the test.
//
// Returns:
//   - `*RuntimeEnvironment`: the environment.
//   - `*RecordingBroker`: its broker.
func newRecordedEnvironment(t *testing.T) (*RuntimeEnvironment, *RecordingBroker) {

	t.Helper()
	broker := NewRecordingBroker()
	app := &application.Application{Name: "test"}
	spec := NewRuntimeEnvironmentSpec(app.Name).
		WithApplication(app).
		WithBroker(broker).
		WithStatus(status.NewNarrator(app.Name, sink.Discard()))

	runtimeEnvironment, err := NewRuntimeEnvironment(context.Background(), spec)
	if err != nil {
		t.Fatalf("NewRuntimeEnvironment: %v", err)
	}
	return runtimeEnvironment, broker
}

// --- PlatformRunner ---

// TestPlatformRunner_ThroughTheBroker pins the adapter: a manager's command becomes an exec by account with its
// environment and the auto-confirm stdin, and the frames fold back into the platform's Result, newlines trimmed.
func TestPlatformRunner_ThroughTheBroker(t *testing.T) {

	runtimeEnvironment, broker := newRecordedEnvironment(t)
	deliverWhenAsked(t, broker, platformRun,
		NarrationFrame("e-1", "root", "warn", "Reading package lists..."),
		OutputFrame("e-1", []byte("done\n")),
		ControlFrame("e-1", 0, nil),
	)

	result := PlatformRunner(runtimeEnvironment)(context.Background(), platform.CommandRequest{
		Account: platform.AccountRoot, Argv: []string{"apt-get", "update"},
		Env: map[string]string{"DEBIAN_FRONTEND": "noninteractive"}, AutoConfirm: true,
	})

	if !result.OK || result.Code != 0 || result.Stdout != "done" || result.Stderr != "Reading package lists..." {
		t.Errorf("result = %+v", result)
	}

	execs := broker.Execs()
	if len(execs) != 1 {
		t.Fatalf("execs = %+v", execs)
	}
	exec := execs[0]
	if exec.Run != platformRun || exec.Account != "root" || exec.Argv[0] != "apt-get" ||
		exec.Env["DEBIAN_FRONTEND"] != "noninteractive" || exec.Stdin == nil || !exec.Stdin.AutoConfirm {
		t.Errorf("exec = %+v", exec)
	}
}

// TestPlatformRunner_Refusal pins that a refused account is a Result with Code -1 naming the refusal, as the
// platform's own runner reports a launch it could not make.
func TestPlatformRunner_Refusal(t *testing.T) {

	runtimeEnvironment, broker := newRecordedEnvironment(t)
	broker.Refuse("root", &Refusal{Reason: "account root is not offered", Account: "root"})

	result := PlatformRunner(runtimeEnvironment)(context.Background(), platform.CommandRequest{
		Account: platform.AccountRoot, Argv: []string{"apt-get", "update"},
	})

	if result.OK || result.Code != -1 || result.Stderr != "account root is not offered" {
		t.Errorf("result = %+v", result)
	}
}

// TestNewRuntimeEnvironment_PlatformRunsThroughTheBroker pins the wiring: an environment that detects its platform
// gives it a runner that reaches the environment's broker.
func TestNewRuntimeEnvironment_PlatformRunsThroughTheBroker(t *testing.T) {

	runtimeEnvironment, broker := newRecordedEnvironment(t)
	if runtimeEnvironment.Platform == nil {
		t.Skip("no platform detected on this host")
	}
	broker.Complete(0)

	// Whatever the host's default manager is, a query of it is a command through the broker.
	runtimeEnvironment.Platform.PackageManager().Search("devlore-test-query", 1)

	if len(broker.Execs()) == 0 {
		t.Error("the detected platform's command did not reach the environment's broker")
	}
}
