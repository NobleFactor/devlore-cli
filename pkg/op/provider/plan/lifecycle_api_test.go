// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package plan_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/document"
	"github.com/NobleFactor/devlore-cli/pkg/application"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/plan"

	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/flow/gen"
)

// TestGraphSaveLoadExecuteTrace_ViaPublicAPI walks the full graph lifecycle through the public plan.Provider
// Go API: plan -> save -> load -> execute the loaded graph -> save the execution Trace. It asserts that
// save/load preserves graph identity (checksum), that the loaded graph runs to a terminal state and produces
// its side effect, and that the Trace serializes to a receipt file.
func TestGraphSaveLoadExecuteTrace_ViaPublicAPI(t *testing.T) {
	tmp := t.TempDir()

	root, err := fsroot.OpenConfined(tmp)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	env := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithRoot(root).
		WithApplication(&application.Application{Name: "test"}))

	planProvider := plan.NewProvider(env)

	// plan: a one-node graph that creates a directory.
	target := filepath.Join(tmp, "made")
	invocation, err := planProvider.Plan("file.mkdir", nil, map[string]any{
		"path":  target,
		"chmod": os.FileMode(0o755),
		"chown": "",
	})
	if err != nil {
		t.Fatalf("Plan(file.mkdir): %v", err)
	}

	graph, err := planProvider.AssembleDefinition([]*op.Invocation{invocation}, nil, nil, nil, planProvider.Origin("test"))
	if err != nil {
		t.Fatalf("AssembleDefinition: %v", err)
	}

	// save
	graphPath := filepath.Join(tmp, "graph.json")
	if err := planProvider.SaveDefinition(graph, graphPath); err != nil {
		t.Fatalf("SaveDefinition: %v", err)
	}

	// load
	loaded, err := planProvider.LoadDefinition(graphPath)
	if err != nil {
		t.Fatalf("LoadDefinition: %v", err)
	}
	if loaded.Checksum() != graph.Checksum() {
		t.Errorf("loaded checksum %q != saved %q — save/load did not preserve graph identity",
			loaded.Checksum(), graph.Checksum())
	}

	// execute the LOADED graph (not the in-memory one)
	spec, err := planProvider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	executor := op.NewGraphExecutor(loaded, spec)
	if _, runErr := executor.Run(context.Background(), nil); runErr != nil {
		t.Fatalf("Run(loaded): %v", runErr)
	}

	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		t.Errorf("loaded graph did not create %s (stat err=%v)", target, statErr)
	}

	// save the trace
	tracePath := filepath.Join(tmp, "trace.json")
	trace := executor.Trace()
	if trace.State != op.RunStateCompleted {
		t.Errorf("trace.State = %v, want RunStateCompleted", trace.State)
	}
	if trace.GraphChecksum != loaded.Checksum() {
		t.Errorf("trace.GraphChecksum %q != loaded graph checksum %q", trace.GraphChecksum, loaded.Checksum())
	}
	if err := document.Write(tracePath, trace); err != nil {
		t.Fatalf("document.Write(trace): %v", err)
	}
	if _, statErr := os.Stat(tracePath); statErr != nil {
		t.Errorf("trace receipt not saved: %v", statErr)
	}
}

// TestGraphPauseResume_ViaPublicAPI exercises pause/resume (step 28(b), pseudo replay): a two-node graph is paused
// after its first node completes, then resumed from the [op.Trace]. It asserts the resumed run completes, the
// not-yet-run node's side effect appears, and no unit is re-dispatched — the completed node is replayed from its
// receipt, not re-run (proven by the per-action completed count staying at one per node).
func TestGraphPauseResume_ViaPublicAPI(t *testing.T) {
	tmp := t.TempDir()

	root, err := fsroot.OpenConfined(tmp)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	env := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithRoot(root).
		WithApplication(&application.Application{Name: "test"}))

	planProvider := plan.NewProvider(env)

	// Two independent mkdir nodes; declaration order dispatches the first before the second.
	dirA := filepath.Join(tmp, "a")
	dirB := filepath.Join(tmp, "b")
	inv1, err := planProvider.Plan("file.mkdir", nil,
		map[string]any{"path": dirA, "chmod": os.FileMode(0o755), "chown": ""})
	if err != nil {
		t.Fatalf("Plan(a): %v", err)
	}
	inv2, err := planProvider.Plan("file.mkdir", nil,
		map[string]any{"path": dirB, "chmod": os.FileMode(0o755), "chown": ""})
	if err != nil {
		t.Fatalf("Plan(b): %v", err)
	}
	graph, err := planProvider.AssembleDefinition(
		[]*op.Invocation{inv1, inv2}, nil, nil, nil, planProvider.Origin("test"))
	if err != nil {
		t.Fatalf("AssembleDefinition: %v", err)
	}

	spec, err := planProvider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}

	// First run: pause after the first node completes.
	executor := op.NewGraphExecutor(graph, spec)
	hooks := op.NewHookRegistry()
	hooks.Register(&pauseAfterFirstNode{executor: executor})
	executor.SetHooks(hooks)

	if _, runErr := executor.Run(context.Background(), nil); !errors.Is(runErr, op.ErrPaused) {
		t.Fatalf("first Run: err = %v, want ErrPaused", runErr)
	}
	if executor.State() != op.RunStatePaused {
		t.Fatalf("after pause: state = %v, want RunStatePaused", executor.State())
	}
	if dirExists(dirA) == dirExists(dirB) {
		t.Fatalf("after pause: want exactly one dir, got a=%v b=%v", dirExists(dirA), dirExists(dirB))
	}

	// Resume from the trace with a fresh spec — the first run's env.Close() closed the original spec's confined root,
	// just as a real resume runs in a new process with a freshly built spec.
	resumedSpec, err := planProvider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec (resume): %v", err)
	}
	resumed, err := op.ResumeExecutor(graph, resumedSpec, executor.Trace())
	if err != nil {
		t.Fatalf("ResumeExecutor: %v", err)
	}
	if _, runErr := resumed.Run(context.Background(), nil); runErr != nil {
		t.Fatalf("resumed Run: %v", runErr)
	}
	if resumed.State() != op.RunStateCompleted {
		t.Fatalf("after resume: state = %v, want RunStateCompleted", resumed.State())
	}

	// Both side effects present, and neither node was re-dispatched.
	if !dirExists(dirA) || !dirExists(dirB) {
		t.Errorf("after resume: a=%v b=%v, want both true", dirExists(dirA), dirExists(dirB))
	}
	if got := resumed.Trace().Summarize(graph).ByAction()["file.mkdir"].Completed(); got != 2 {
		t.Errorf("file.mkdir completed = %d, want 2 (one per node; >2 means a node was re-dispatched on resume)", got)
	}
}

// TestGraphPauseResumeNested_ViaPublicAPI exercises resume across a nested subgraph (the recursive adopt): the run
// pauses inside an inner flow.subgraph after its first child, then resumes. Both levels — the root and the inner
// subgraph — adopt their restored child stacks; the completed child is replayed and the pending one dispatched.
func TestGraphPauseResumeNested_ViaPublicAPI(t *testing.T) {
	tmp := t.TempDir()

	root, err := fsroot.OpenConfined(tmp)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	env := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithRoot(root).
		WithApplication(&application.Application{Name: "test"}))

	planProvider := plan.NewProvider(env)

	dirB := filepath.Join(tmp, "b")
	dirC := filepath.Join(tmp, "c")
	invB, err := planProvider.Plan("file.mkdir", nil,
		map[string]any{"path": dirB, "chmod": os.FileMode(0o755), "chown": ""})
	if err != nil {
		t.Fatalf("Plan(b): %v", err)
	}
	invC, err := planProvider.Plan("file.mkdir", nil,
		map[string]any{"path": dirC, "chmod": os.FileMode(0o755), "chown": ""})
	if err != nil {
		t.Fatalf("Plan(c): %v", err)
	}
	subInv, err := planProvider.Plan("flow.subgraph", nil, map[string]any{"body": []any{invB, invC}})
	if err != nil {
		t.Fatalf("Plan(flow.subgraph): %v", err)
	}
	graph, err := planProvider.AssembleDefinition(
		[]*op.Invocation{subInv}, nil, nil, nil, planProvider.Origin("test"))
	if err != nil {
		t.Fatalf("AssembleDefinition: %v", err)
	}

	spec, err := planProvider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}

	executor := op.NewGraphExecutor(graph, spec)
	hooks := op.NewHookRegistry()
	hooks.Register(&pauseAfterFirstNode{executor: executor})
	executor.SetHooks(hooks)

	if _, runErr := executor.Run(context.Background(), nil); !errors.Is(runErr, op.ErrPaused) {
		t.Fatalf("first Run: err = %v, want ErrPaused", runErr)
	}
	if dirExists(dirB) == dirExists(dirC) {
		t.Fatalf("after pause: want exactly one dir, got b=%v c=%v", dirExists(dirB), dirExists(dirC))
	}

	resumedSpec, err := planProvider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec (resume): %v", err)
	}
	resumed, err := op.ResumeExecutor(graph, resumedSpec, executor.Trace())
	if err != nil {
		t.Fatalf("ResumeExecutor: %v", err)
	}
	if _, runErr := resumed.Run(context.Background(), nil); runErr != nil {
		t.Fatalf("resumed Run: %v", runErr)
	}
	if resumed.State() != op.RunStateCompleted {
		t.Fatalf("after resume: state = %v, want RunStateCompleted", resumed.State())
	}
	if !dirExists(dirB) || !dirExists(dirC) {
		t.Errorf("after resume: b=%v c=%v, want both true", dirExists(dirB), dirExists(dirC))
	}
	if got := resumed.Trace().Summarize(graph).ByAction()["file.mkdir"].Completed(); got != 2 {
		t.Errorf("file.mkdir completed = %d, want 2 (one per node; >2 means re-dispatch on resume)", got)
	}
}

// dirExists reports whether a directory exists at `path`.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// pauseAfterFirstNode is a [op.LifecycleHook] that pauses its executor the first time any node completes — used to
// stop a run mid-graph so the remainder can be resumed from the trace.
type pauseAfterFirstNode struct {
	executor *op.GraphExecutor
	fired    bool
}

func (h *pauseAfterFirstNode) OnNodeStart(*op.RuntimeEnvironment, string, map[string]any) {}

func (h *pauseAfterFirstNode) OnNodeComplete(_ *op.RuntimeEnvironment, _ string, _ op.Result, _ error) {
	if !h.fired {
		h.fired = true
		_ = h.executor.Pause()
	}
}

func (h *pauseAfterFirstNode) OnSubgraphStart(*op.RuntimeEnvironment, string) {}

func (h *pauseAfterFirstNode) OnSubgraphComplete(*op.RuntimeEnvironment, string, error) {}
