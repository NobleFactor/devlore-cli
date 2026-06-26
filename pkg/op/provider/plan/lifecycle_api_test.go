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
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
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

// TestGraphSaveLoadResume_ViaPublicAPI exercises the full save→load→resume round-trip (step 28(b), rows 23–26): a
// paused run's Trace is written to disk, reloaded, and resumed on a fresh executor. It proves the recovery stack and
// its receipts survive serialization carrying the execution state resume needs (ids, results, status, complement).
func TestGraphSaveLoadResume_ViaPublicAPI(t *testing.T) {
	tmp := t.TempDir()

	root, err := fsroot.OpenConfined(tmp)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	env := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithRoot(root).
		WithApplication(&application.Application{Name: "test"}))

	planProvider := plan.NewProvider(env)

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

	executor := op.NewGraphExecutor(graph, spec)
	hooks := op.NewHookRegistry()
	hooks.Register(&pauseAfterFirstNode{executor: executor})
	executor.SetHooks(hooks)

	if _, runErr := executor.Run(context.Background(), nil); !errors.Is(runErr, op.ErrPaused) {
		t.Fatalf("first Run: err = %v, want ErrPaused", runErr)
	}
	if dirExists(dirA) == dirExists(dirB) {
		t.Fatalf("after pause: want exactly one dir, got a=%v b=%v", dirExists(dirA), dirExists(dirB))
	}

	// Save the Trace to disk and reload it — the serialize round-trip.
	tracePath := filepath.Join(tmp, "trace.json")
	original := executor.Trace()
	if original.Catalog == nil || len(original.Catalog.Entries) == 0 {
		t.Fatalf("Trace.Catalog: want a non-empty resource ledger snapshot, got %+v", original.Catalog)
	}
	if writeErr := document.Write(tracePath, original); writeErr != nil {
		t.Fatalf("document.Write(trace): %v", writeErr)
	}
	reloaded, err := document.ReadFile[op.Trace](tracePath)
	if err != nil {
		t.Fatalf("document.ReadFile(trace): %v", err)
	}
	if reloaded.State != op.RunStatePaused {
		t.Fatalf("reloaded trace state = %v, want RunStatePaused", reloaded.State)
	}
	gotEntries := 0
	if reloaded.Catalog != nil {
		gotEntries = len(reloaded.Catalog.Entries)
	}
	if gotEntries != len(original.Catalog.Entries) {
		t.Fatalf("Trace.Catalog round-trip: entries = %d, want %d", gotEntries, len(original.Catalog.Entries))
	}

	// Resume from the RELOADED trace on a fresh executor.
	resumedSpec, err := planProvider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec (resume): %v", err)
	}
	resumed, err := op.ResumeExecutor(graph, resumedSpec, reloaded)
	if err != nil {
		t.Fatalf("ResumeExecutor: %v", err)
	}
	if _, runErr := resumed.Run(context.Background(), nil); runErr != nil {
		t.Fatalf("resumed Run: %v", runErr)
	}
	if resumed.State() != op.RunStateCompleted {
		t.Fatalf("after resume: state = %v, want RunStateCompleted", resumed.State())
	}
	if !dirExists(dirA) || !dirExists(dirB) {
		t.Errorf("after resume: a=%v b=%v, want both true", dirExists(dirA), dirExists(dirB))
	}
	if got := resumed.Trace().Summarize(graph).ByAction()["file.mkdir"].Completed(); got != 2 {
		t.Errorf("file.mkdir completed = %d, want 2 (>2 means a node was re-dispatched after reload)", got)
	}
}

// TestGraphResumeThenFail_RollsBack_ViaPublicAPI is the B3 headline: a run pauses after one mkdir, is saved and
// reloaded, and the resumed run fails at the un-run frontier — compensation of the re-armed pre-pause receipt rolls
// back the directory created before the pause.
//
// It runs through both document formats: a JSON- and a YAML-loaded trace must reconstruct their recovery stack and
// receipts identically (format-neutral reconstruction), so the rollback holds whichever format the trace was stored in.
func TestGraphResumeThenFail_RollsBack_ViaPublicAPI(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) { resumeThenFailRollsBack(t, format) })
	}
}

// resumeThenFailRollsBack runs the resume-then-fail rollback scenario with the trace saved and reloaded in `format`.
func resumeThenFailRollsBack(t *testing.T, format string) {
	t.Helper()

	tmp := t.TempDir()

	root, err := fsroot.OpenConfined(tmp)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	env := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithRoot(root).
		WithApplication(&application.Application{Name: "test"}))
	planProvider := plan.NewProvider(env)

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

	executor := op.NewGraphExecutor(graph, spec)
	hooks := op.NewHookRegistry()
	hooks.Register(&pauseAfterFirstNode{executor: executor})
	executor.SetHooks(hooks)

	if _, runErr := executor.Run(context.Background(), nil); !errors.Is(runErr, op.ErrPaused) {
		t.Fatalf("first Run: err = %v, want ErrPaused", runErr)
	}

	// One dir was created before the pause; the other node is the un-run frontier resumed next.
	ranPath, unrunPath := dirA, dirB
	if !dirExists(dirA) {
		ranPath, unrunPath = dirB, dirA
	}
	if !dirExists(ranPath) {
		t.Fatalf("after pause: expected exactly one dir, got a=%v b=%v", dirExists(dirA), dirExists(dirB))
	}

	tracePath := filepath.Join(tmp, "trace."+format)
	if writeErr := document.Write(tracePath, executor.Trace()); writeErr != nil {
		t.Fatalf("document.Write(trace): %v", writeErr)
	}
	reloaded, err := document.ReadFile[op.Trace](tracePath)
	if err != nil {
		t.Fatalf("document.ReadFile(trace): %v", err)
	}

	// Make the un-run mkdir fail on resume by occupying its path with a regular file.
	if writeErr := os.WriteFile(unrunPath, []byte("conflict"), 0o644); writeErr != nil {
		t.Fatalf("seed conflict file: %v", writeErr)
	}

	resumedSpec, err := planProvider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec (resume): %v", err)
	}
	resumed, err := op.ResumeExecutor(graph, resumedSpec, reloaded)
	if err != nil {
		t.Fatalf("ResumeExecutor: %v", err)
	}
	if _, runErr := resumed.Run(context.Background(), nil); runErr == nil || errors.Is(runErr, op.ErrPaused) {
		t.Fatalf("resumed Run: want a failure, got %v", runErr)
	}

	// Compensation of the re-armed pre-pause receipt must have removed the directory created before the pause.
	if dirExists(ranPath) {
		t.Fatalf("resume-then-fail: pre-pause dir %q was not rolled back", ranPath)
	}
}

// TestGraphResumePromiseFidelity_ViaPublicAPI proves cross-pause promise fidelity: a consumer that runs after resume,
// depending on a producer that ran before the pause, receives the producer's result retyped to the concrete type its
// parameter needs. The producer's reloaded result is the untyped tag-URI string; the consumer (file.exists, whose
// parameter is *file.Resource) must get it rebuilt through the Convert cascade at promise resolution, or it fails on a
// bare string. Runs through both JSON and YAML traces.
func TestGraphResumePromiseFidelity_ViaPublicAPI(t *testing.T) {
	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) { resumePromiseFidelity(t, format) })
	}
}

// resumePromiseFidelity runs the cross-pause promise-fidelity scenario with the trace saved and reloaded in `format`.
func resumePromiseFidelity(t *testing.T, format string) {
	t.Helper()

	tmp := t.TempDir()

	root, err := fsroot.OpenConfined(tmp)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	env := op.NewRuntimeEnvironment(context.Background(), op.NewRuntimeEnvironmentSpec("test").
		WithRoot(root).
		WithApplication(&application.Application{Name: "test"}))
	planProvider := plan.NewProvider(env)

	dir := filepath.Join(tmp, "d")
	producer, err := planProvider.Plan("file.mkdir", nil,
		map[string]any{"path": dir, "chmod": os.FileMode(0o755), "chown": ""})
	if err != nil {
		t.Fatalf("Plan(mkdir): %v", err)
	}

	// The consumer depends on the producer's resource result via a promise (the *Invocation slot value).
	consumer, err := planProvider.Plan("file.exists", nil, map[string]any{"resource": producer})
	if err != nil {
		t.Fatalf("Plan(exists): %v", err)
	}

	graph, err := planProvider.AssembleDefinition(
		[]*op.Invocation{producer, consumer}, nil, nil, nil, planProvider.Origin("test"))
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

	// The producer runs and the pause lands before the consumer, which depends on it.
	if _, runErr := executor.Run(context.Background(), nil); !errors.Is(runErr, op.ErrPaused) {
		t.Fatalf("first Run: err = %v, want ErrPaused", runErr)
	}
	if !dirExists(dir) {
		t.Fatalf("after pause: producer dir %q was not created", dir)
	}

	tracePath := filepath.Join(tmp, "trace."+format)
	if writeErr := document.Write(tracePath, executor.Trace()); writeErr != nil {
		t.Fatalf("document.Write(trace): %v", writeErr)
	}
	reloaded, err := document.ReadFile[op.Trace](tracePath)
	if err != nil {
		t.Fatalf("document.ReadFile(trace): %v", err)
	}

	resumedSpec, err := planProvider.Spec("test", tmp, nil)
	if err != nil {
		t.Fatalf("Spec (resume): %v", err)
	}
	resumed, err := op.ResumeExecutor(graph, resumedSpec, reloaded)
	if err != nil {
		t.Fatalf("ResumeExecutor: %v", err)
	}

	// The consumer resolves its promise to the producer's reloaded (untyped) result and must receive it retyped to
	// *file.Resource via the Convert cascade — file.Exists fails on a bare string.
	if _, runErr := resumed.Run(context.Background(), nil); runErr != nil {
		t.Fatalf("resumed Run: consumer must retype the reloaded producer result, got %v", runErr)
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

// TestResourceLedgerRehydrate_PreservesIDs is the B2 positive check: a ledger snapshot rehydrates into a live catalog
// whose entries keep their original ids, so the recovery stack's id references resolve via Lookup after save/load.
func TestResourceLedgerRehydrate_PreservesIDs(t *testing.T) {
	tmp := t.TempDir()

	root, err := fsroot.OpenConfined(tmp)
	if err != nil {
		t.Fatalf("fsroot.OpenConfined: %v", err)
	}

	spec := op.NewRuntimeEnvironmentSpec("test").
		WithRoot(root).
		WithApplication(&application.Application{Name: "test"})

	env := op.NewRuntimeEnvironment(context.Background(), spec)
	resource, err := file.NewResource(env, nil, filepath.Join(tmp, "x"))
	if err != nil {
		t.Fatalf("file.NewResource: %v", err)
	}

	snapshot := env.ResourceCatalog.Snapshot()
	if len(snapshot.Entries) == 0 {
		t.Fatalf("Snapshot: want a non-empty ledger, got none")
	}

	resumeEnv := op.NewRuntimeEnvironment(context.Background(), spec)
	restored, err := snapshot.Rehydrate(resumeEnv)
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}

	got, ok := restored.Lookup(resource.ID())
	if !ok {
		t.Fatalf("Lookup(%q): not found after rehydrate", resource.ID())
	}
	if got.URI() != resource.URI() {
		t.Fatalf("rehydrated URI = %q, want %q", got.URI(), resource.URI())
	}
}
