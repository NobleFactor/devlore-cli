// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/execution/flow"
	"github.com/NobleFactor/devlore-cli/internal/execution/provider/file"
)

func TestFlowChooseDo(t *testing.T) {
	ctx := &execution.Context{Context: context.Background()}
	op := &flow.Choose{}

	result, undo, err := op.Do(ctx, nil)
	if err != nil {
		t.Fatalf("flow.choose Do: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if undo != nil {
		t.Errorf("expected nil undo state, got %v", undo)
	}
}

func TestFlowChooseUndo(t *testing.T) {
	ctx := &execution.Context{Context: context.Background()}
	op := &flow.Choose{}

	err := op.Undo(ctx, nil, nil)
	if err != nil {
		t.Fatalf("flow.choose Undo: %v", err)
	}
}

func TestFlowGatherDo(t *testing.T) {
	ctx := &execution.Context{Context: context.Background()}
	op := &flow.Gather{}

	result, undo, err := op.Do(ctx, nil)
	if err != nil {
		t.Fatalf("flow.gather Do: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if undo != nil {
		t.Errorf("expected nil undo state, got %v", undo)
	}
}

func TestFlowGatherUndo(t *testing.T) {
	ctx := &execution.Context{Context: context.Background()}
	op := &flow.Gather{}

	err := op.Undo(ctx, nil, nil)
	if err != nil {
		t.Fatalf("flow.gather Undo: %v", err)
	}
}

func TestFlowElevateDo(t *testing.T) {
	ctx := &execution.Context{Context: context.Background()}
	op := &flow.Elevate{}

	result, undo, err := op.Do(ctx, nil)
	if err != nil {
		t.Fatalf("flow.elevate Do: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if undo != nil {
		t.Errorf("expected nil undo state, got %v", undo)
	}
}

func TestFlowElevateUndo(t *testing.T) {
	ctx := &execution.Context{Context: context.Background()}
	op := &flow.Elevate{}

	err := op.Undo(ctx, nil, nil)
	if err != nil {
		t.Fatalf("flow.elevate Undo: %v", err)
	}
}

func TestFlowChooseName(t *testing.T) {
	op := &flow.Choose{}
	if op.Name() != "flow.choose" {
		t.Errorf("expected 'flow.choose', got %q", op.Name())
	}
}

func TestFlowGatherName(t *testing.T) {
	op := &flow.Gather{}
	if op.Name() != "flow.gather" {
		t.Errorf("expected 'flow.gather', got %q", op.Name())
	}
}

func TestFlowElevateName(t *testing.T) {
	op := &flow.Elevate{}
	if op.Name() != "flow.elevate" {
		t.Errorf("expected 'flow.elevate', got %q", op.Name())
	}
}

// TestGatherIntegration tests a graph with 3 predecessors → gather → successor.
// Verifies that all predecessors complete before the successor runs.
func TestGatherIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create 3 source files
	srcA := filepath.Join(tmpDir, "a.txt")
	srcB := filepath.Join(tmpDir, "b.txt")
	srcC := filepath.Join(tmpDir, "c.txt")
	if err := os.WriteFile(srcA, []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcB, []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcC, []byte("c"), 0644); err != nil {
		t.Fatal(err)
	}

	fp := &file.Provider{}

	// 3 link nodes → gather → final link
	nodeA := testNode("a", &file.Link{Impl: fp}, srcA, filepath.Join(tmpDir, "out_a"))
	nodeB := testNode("b", &file.Link{Impl: fp}, srcB, filepath.Join(tmpDir, "out_b"))
	nodeC := testNode("c", &file.Link{Impl: fp}, srcC, filepath.Join(tmpDir, "out_c"))

	gatherNode := &execution.Node{ID: "gather", Action: &flow.Gather{}}

	// Final node depends on gather
	srcFinal := filepath.Join(tmpDir, "final.txt")
	if err := os.WriteFile(srcFinal, []byte("final"), 0644); err != nil {
		t.Fatal(err)
	}
	finalNode := testNode("final", &file.Link{Impl: fp}, srcFinal, filepath.Join(tmpDir, "out_final"))

	graph := &execution.Graph{
		Nodes: []*execution.Node{nodeA, nodeB, nodeC, gatherNode, finalNode},
		Edges: []execution.Edge{
			{From: "a", To: "gather"},
			{From: "b", To: "gather"},
			{From: "c", To: "gather"},
			{From: "gather", To: "final"},
		},
	}

	engine := execution.NewGraphExecutor(execution.ExecutorOptions{})
	results, err := runGraph(context.Background(), engine, graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// All 5 nodes should complete
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Status != execution.ResultCompleted {
			t.Errorf("node %s: expected completed, got %s (error: %v)", r.NodeID, r.Status, r.Error)
		}
	}

	// Verify gather ran before final: find positions in results
	gatherIdx := -1
	finalIdx := -1
	for i, r := range results {
		if r.NodeID == "gather" {
			gatherIdx = i
		}
		if r.NodeID == "final" {
			finalIdx = i
		}
	}
	if gatherIdx >= finalIdx {
		t.Errorf("expected gather (idx %d) before final (idx %d)", gatherIdx, finalIdx)
	}

	// Verify all predecessors ran before gather
	for _, r := range results[:gatherIdx] {
		if r.NodeID == "final" {
			t.Error("final should not run before gather")
		}
	}

	// Verify final output exists
	if _, err := os.Lstat(filepath.Join(tmpDir, "out_final")); err != nil {
		t.Errorf("expected final output to exist: %v", err)
	}
}
