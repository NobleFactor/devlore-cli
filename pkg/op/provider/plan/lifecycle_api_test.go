// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package plan_test

import (
	"context"
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

	graph, err := planProvider.Assemble([]*op.Invocation{invocation}, nil, nil, nil, planProvider.Origin("test"))
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	// save
	graphPath := filepath.Join(tmp, "graph.json")
	if err := planProvider.Save(graph, graphPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// load
	loaded, err := planProvider.Load(graphPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
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
