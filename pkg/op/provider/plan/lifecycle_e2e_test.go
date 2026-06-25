// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package plan_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/document"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"
	"github.com/NobleFactor/devlore-cli/pkg/op/starlarkbridge"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/flow/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/plan/gen"
)

// Two execution surfaces. The Go API drives the [op.GraphExecutor] directly — run to completion, pause + resume, and
// fail + rollback. The Starlark API drives execution from a .star script via plan.run — run to completion and fail +
// rollback (a failure inside plan.run unwinds and compensates automatically, the same Run() path the Go executor uses).
// Pausing a live run is an out-of-process control-plane concern — the pending eventing API (pause/stop/status over
// HTTP) — not something a synchronous .star script requests of its own run, so it is exercised only on the Go side.
// Receipt and *RecoveryStack complement shapes are both exercised: each `mkdir` produces a single `Receipt`, and the
// graph root is a subgraph whose complement is a `*RecoveryStack` that rollback cascades through.

// graphMaker builds a fresh two-`mkdir` graph (dirA, dirB) rooted under tmp and returns the [*plan.Provider] used to
// make execution specs. The path baked into each node lives under tmp so every scenario gets isolated side effects.
type graphMaker func(t *testing.T, tmp string) (graph *op.Graph, provider *plan.Provider, dirA, dirB string)

// TestLifecycle_ViaGoAPI runs the three execution scenarios on a graph built through the Go plan API.
func TestLifecycle_ViaGoAPI(t *testing.T) {
	t.Run("RunToCompletion", func(t *testing.T) { scenarioRunToCompletion(t, goGraphMaker) })
	t.Run("PauseAndResume", func(t *testing.T) { scenarioPauseAndResume(t, goGraphMaker) })
	t.Run("FailAndRollback", func(t *testing.T) { scenarioFailAndRollback(t, goGraphMaker) })
}

// TestLifecycle_ViaStarlark drives execution from Starlark itself via plan.run: a .star script builds, saves, loads,
// and runs the graph. Two scenarios — run to completion, and fail + rollback (a failure inside plan.run unwinds and
// compensates automatically, the same Run() path the Go executor uses). Pause/resume is not exercised here: pausing a
// run is an out-of-process control-plane concern (the pending eventing API), not something a synchronous .star script
// requests of its own run.
func TestLifecycle_ViaStarlark(t *testing.T) {
	t.Run("RunToCompletion", func(t *testing.T) {
		tmp := t.TempDir()
		dirA, dirB := filepath.Join(tmp, "a"), filepath.Join(tmp, "b")
		if err := runStarlarkLifecycle(t, tmp, dirA, dirB); err != nil {
			t.Fatalf("plan.run: %v", err)
		}
		if !dirExists(dirA) || !dirExists(dirB) {
			t.Fatalf("after completion: a=%v b=%v, want both true", dirExists(dirA), dirExists(dirB))
		}
	})

	t.Run("FailAndRollback", func(t *testing.T) {
		tmp := t.TempDir()
		dirA, dirB := filepath.Join(tmp, "a"), filepath.Join(tmp, "b")

		// Occupy dirB with a regular file so the second mkdir fails at run time.
		if err := os.WriteFile(dirB, []byte("conflict"), 0o644); err != nil {
			t.Fatalf("seed conflict file: %v", err)
		}

		if err := runStarlarkLifecycle(t, tmp, dirA, dirB); err == nil {
			t.Fatalf("plan.run: want a failure (mkdir b conflicts), got nil")
		}

		// The framework must roll back the directory created before the failure — automatically, inside plan.run.
		if dirExists(dirA) {
			t.Fatalf("rollback: pre-failure dir %q was not rolled back when the run failed", dirA)
		}
	})
}

// scenarioRunToCompletion runs the graph straight through and asserts both directories were created.
func scenarioRunToCompletion(t *testing.T, makeGraph graphMaker) {
	tmp := t.TempDir()
	graph, provider, dirA, dirB := makeGraph(t, tmp)

	executor := op.NewGraphExecutor(graph, mustSpec(t, provider, tmp))
	if _, err := executor.Run(context.Background(), nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if executor.State() != op.RunStateCompleted {
		t.Fatalf("state = %v, want RunStateCompleted", executor.State())
	}
	if !dirExists(dirA) || !dirExists(dirB) {
		t.Fatalf("after completion: a=%v b=%v, want both true", dirExists(dirA), dirExists(dirB))
	}
}

// scenarioPauseAndResume pauses after the first node, saves + reloads the trace, resumes on a fresh executor, and
// asserts the run completes without re-dispatching the already-done node.
func scenarioPauseAndResume(t *testing.T, makeGraph graphMaker) {
	tmp := t.TempDir()
	graph, provider, dirA, dirB := makeGraph(t, tmp)

	executor := op.NewGraphExecutor(graph, mustSpec(t, provider, tmp))
	installPauseAfterFirst(executor)
	if _, err := executor.Run(context.Background(), nil); !errors.Is(err, op.ErrPaused) {
		t.Fatalf("first Run: err = %v, want ErrPaused", err)
	}
	if dirExists(dirA) == dirExists(dirB) {
		t.Fatalf("after pause: want exactly one dir, got a=%v b=%v", dirExists(dirA), dirExists(dirB))
	}

	reloaded := saveAndReload(t, tmp, executor.Trace())
	resumed, err := op.ResumeExecutor(graph, mustSpec(t, provider, tmp), reloaded)
	if err != nil {
		t.Fatalf("ResumeExecutor: %v", err)
	}
	if _, err := resumed.Run(context.Background(), nil); err != nil {
		t.Fatalf("resumed Run: %v", err)
	}
	if resumed.State() != op.RunStateCompleted {
		t.Fatalf("after resume: state = %v, want RunStateCompleted", resumed.State())
	}
	if !dirExists(dirA) || !dirExists(dirB) {
		t.Fatalf("after resume: a=%v b=%v, want both true", dirExists(dirA), dirExists(dirB))
	}
	if got := resumed.Trace().Summarize(graph).ByAction()["file.mkdir"].Completed(); got != 2 {
		t.Errorf("file.mkdir completed = %d, want 2 (>2 means a node was re-dispatched after reload)", got)
	}
}

// scenarioFailAndRollback pauses after the first node, occupies the un-run frontier so its mkdir fails, resumes, and
// asserts the failure rolls back the directory created before the pause via the re-armed receipt's compensation.
func scenarioFailAndRollback(t *testing.T, makeGraph graphMaker) {
	tmp := t.TempDir()
	graph, provider, dirA, dirB := makeGraph(t, tmp)

	executor := op.NewGraphExecutor(graph, mustSpec(t, provider, tmp))
	installPauseAfterFirst(executor)
	if _, err := executor.Run(context.Background(), nil); !errors.Is(err, op.ErrPaused) {
		t.Fatalf("first Run: err = %v, want ErrPaused", err)
	}

	ranPath, unrunPath := dirA, dirB
	if !dirExists(dirA) {
		ranPath, unrunPath = dirB, dirA
	}
	if !dirExists(ranPath) {
		t.Fatalf("after pause: want exactly one dir, got a=%v b=%v", dirExists(dirA), dirExists(dirB))
	}

	reloaded := saveAndReload(t, tmp, executor.Trace())

	// Make the un-run mkdir fail on resume by occupying its path with a regular file.
	if err := os.WriteFile(unrunPath, []byte("conflict"), 0o644); err != nil {
		t.Fatalf("seed conflict file: %v", err)
	}

	resumed, err := op.ResumeExecutor(graph, mustSpec(t, provider, tmp), reloaded)
	if err != nil {
		t.Fatalf("ResumeExecutor: %v", err)
	}
	if _, err := resumed.Run(context.Background(), nil); err == nil || errors.Is(err, op.ErrPaused) {
		t.Fatalf("resumed Run: want a failure, got %v", err)
	}
	if dirExists(ranPath) {
		t.Fatalf("resume-then-fail: pre-pause dir %q was not rolled back", ranPath)
	}
}

// goGraphMaker builds the two-`mkdir` graph through the Go plan API.
func goGraphMaker(t *testing.T, tmp string) (*op.Graph, *plan.Provider, string, string) {
	t.Helper()

	_, provider := newLifecycleEnv(t, tmp)
	dirA, dirB := filepath.Join(tmp, "a"), filepath.Join(tmp, "b")

	inv1, err := provider.Plan("file.mkdir", nil, map[string]any{"path": dirA, "chmod": os.FileMode(0o755), "chown": ""})
	if err != nil {
		t.Fatalf("Plan(a): %v", err)
	}
	inv2, err := provider.Plan("file.mkdir", nil, map[string]any{"path": dirB, "chmod": os.FileMode(0o755), "chown": ""})
	if err != nil {
		t.Fatalf("Plan(b): %v", err)
	}

	graph, err := provider.AssembleDefinition([]*op.Invocation{inv1, inv2}, nil, nil, nil, provider.Origin("test"))
	if err != nil {
		t.Fatalf("AssembleDefinition: %v", err)
	}
	return graph, provider, dirA, dirB
}

// runStarlarkLifecycle writes a .star that builds, saves, loads, and runs a two-`mkdir` graph (dirA, dirB) entirely
// through the Starlark plan API, and returns plan.run's error (nil on a clean run). Execution happens inside the
// script — including automatic rollback when the run fails.
func runStarlarkLifecycle(t *testing.T, tmp, dirA, dirB string) error {
	t.Helper()

	environment, _ := newLifecycleEnv(t, tmp)
	graphPath := filepath.Join(tmp, "graph.json")

	script := fmt.Sprintf(`
a = plan.file.mkdir(path = %q, chmod = 0o755, chown = "")
b = plan.file.mkdir(path = %q, chmod = 0o755, chown = "")
graph = plan.assemble_definition([a, b])
plan.save_definition(graph, %q)
loaded = plan.load_definition(%q)
plan.run(loaded, plan.spec())
`, dirA, dirB, graphPath, graphPath)

	scriptPath := filepath.Join(tmp, "lifecycle.star")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	_, err := starlarkbridge.NewRuntime(environment).Invoke("lifecycle.star", tmp)
	return err
}

// newLifecycleEnv mints a confined-root runtime environment over tmp and a plan provider bound to it. The environment
// is for planning / graph loading / spec creation only; each Run builds its own environment from the spec.
func newLifecycleEnv(t *testing.T, tmp string) (*op.RuntimeEnvironment, *plan.Provider) {
	t.Helper()

	root, err := fsroot.OpenConfined(tmp)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	environment := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithRoot(root).
		WithApplication(&application.Application{Name: "test"}))
	t.Cleanup(func() { _ = environment.Close() })

	return environment, plan.NewProvider(environment)
}

// mustSpec returns a fresh execution spec rooted at tmp, failing the test on error.
func mustSpec(t *testing.T, provider *plan.Provider, tmp string) *op.RuntimeEnvironmentSpec {
	t.Helper()

	spec, err := provider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	return spec
}

// saveAndReload writes the trace to tmp/trace.json and reads it back — the save/load half of a real resume.
func saveAndReload(t *testing.T, tmp string, trace *op.Trace) *op.Trace {
	t.Helper()

	tracePath := filepath.Join(tmp, "trace.json")
	if err := document.Write(tracePath, trace); err != nil {
		t.Fatalf("document.Write(trace): %v", err)
	}
	reloaded, err := document.ReadFile[op.Trace](tracePath)
	if err != nil {
		t.Fatalf("document.ReadFile(trace): %v", err)
	}
	return reloaded
}

// installPauseAfterFirst registers the pause-after-first-node hook on the executor.
func installPauseAfterFirst(executor *op.GraphExecutor) {
	hooks := op.NewHookRegistry()
	hooks.Register(&pauseAfterFirstNode{executor: executor})
	executor.SetHooks(hooks)
}
