// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package plan_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/flow"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/flow/gen"
)

// The two step-12 edge-coverage rows, previously parked on missing harness mechanisms: a real-executor fixture
// (this file — the op.Plan + GraphExecutor pattern) and a mutable probe (a file the test creates mid-run).

// waitUntilSpec mints one phase's runtime spec for the wait_until fixtures.
func waitUntilSpec(t *testing.T, root string) *op.RuntimeEnvironmentSpec {

	t.Helper()

	confined, err := fsroot.OpenConfined(root)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}
	return op.NewRuntimeEnvironmentSpec("test").
		WithRoot(confined).
		WithApplication(&application.Application{Name: "test"})
}

// TestWaitUntil_MatchAfterNPolls pins the re-poll path returning a late truthy result: the probed file does not
// exist at the first polls and appears mid-run (the mutable probe), and the run completes well before the
// timeout.
func TestWaitUntil_MatchAfterNPolls(t *testing.T) {

	tmp := t.TempDir()
	probe := filepath.Join(tmp, "ready")

	graph, err := op.Plan(context.Background(), waitUntilSpec(t, tmp), func(env *op.RuntimeEnvironment) (*op.Graph, error) {
		planProvider := plan.NewProvider(env)
		exists, err := planProvider.Plan(file.Exists, nil, map[string]any{"path": probe})
		if err != nil {
			return nil, err
		}
		if _, err := planProvider.Plan(flow.WaitUntil, nil, map[string]any{
			"body":     exists,
			"timeout":  "10s",
			"interval": "50ms",
		}); err != nil {
			return nil, err
		}
		return planProvider.AssembleDefinition(
			collectInvocations(planProvider), nil, nil, nil, nil, nil, planProvider.Origin(""))
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	// The mutable probe: the file appears after a few polls.
	go func() {
		time.Sleep(180 * time.Millisecond)
		_ = os.WriteFile(probe, []byte("ready"), 0o644) //nolint:errcheck // the assertion below catches a miss
	}()

	start := time.Now()
	executor := op.NewGraphExecutor(graph, waitUntilSpec(t, tmp))
	if _, err := executor.Run(context.Background(), nil); err != nil {
		t.Fatalf("run: %v (the late-truthy re-poll path failed)", err)
	}

	elapsed := time.Since(start)
	if elapsed < 150*time.Millisecond {
		t.Errorf("run completed in %v — the probe cannot have been polled more than once", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Errorf("run took %v — the late truthy result did not stop the polling", elapsed)
	}
}

// TestWaitUntil_BodyErrorFailsImmediately pins body-error propagation: a body whose dispatch errors on the
// first poll fails the run immediately — not at the timeout.
func TestWaitUntil_BodyErrorFailsImmediately(t *testing.T) {

	tmp := t.TempDir()

	graph, err := op.Plan(context.Background(), waitUntilSpec(t, tmp), func(env *op.RuntimeEnvironment) (*op.Graph, error) {
		planProvider := plan.NewProvider(env)
		// A move whose source never exists: the body's dispatch errors on every poll.
		crash, err := planProvider.Plan(file.Move, nil, map[string]any{
			"source_path":      filepath.Join(tmp, "never-exists"),
			"destination_path": filepath.Join(tmp, "unreachable"),
		})
		if err != nil {
			return nil, err
		}
		if _, err := planProvider.Plan(flow.WaitUntil, nil, map[string]any{
			"body":     crash,
			"timeout":  "30s",
			"interval": "50ms",
		}); err != nil {
			return nil, err
		}
		return planProvider.AssembleDefinition(
			collectInvocations(planProvider), nil, nil, nil, nil, nil, planProvider.Origin(""))
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	start := time.Now()
	executor := op.NewGraphExecutor(graph, waitUntilSpec(t, tmp))
	_, runErr := executor.Run(context.Background(), nil)

	if runErr == nil {
		t.Fatal("run succeeded over a crashing body, want the body error")
	}
	if !strings.Contains(runErr.Error(), "never-exists") {
		t.Errorf("run error %v does not surface the body's failure", runErr)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("run took %v — the body error waited for the timeout instead of failing immediately", elapsed)
	}
}
