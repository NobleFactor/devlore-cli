// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution_test

import (
	"context"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/execution/flow"
)

// --- test helpers ---

// flowAction is a simple action for flow tests.
type flowAction struct {
	name   string
	result execution.Result
}

func (a *flowAction) Name() string { return a.name }
func (a *flowAction) Do(_ *execution.Context, _ map[string]any) (execution.Result, execution.UndoState, error) {
	return a.result, "undo:" + a.name, nil
}
func (a *flowAction) Undo(_ *execution.Context, _ map[string]any, _ execution.UndoState) error {
	return nil
}

// flowPredicate is a simple predicate for flow tests.
type flowPredicate struct {
	match bool
}

func (p *flowPredicate) Eval(_ any) (bool, error) { return p.match, nil }
func (p *flowPredicate) String() string {
	if p.match {
		return "always-true"
	}
	return "always-false"
}

// --- Choose tests ---

func TestFlowChooseDo(t *testing.T) {
	nodeA := &execution.Node{ID: "a", Action: &flowAction{name: "test.a", result: "result-a"}}
	nodeB := &execution.Node{ID: "b", Action: &flowAction{name: "test.b", result: "result-b"}}

	graph := &execution.Graph{
		Nodes: []*execution.Node{nodeA, nodeB},
		Phases: []*execution.Phase{
			{ID: "phase-a", NodeIDs: []string{"a"}},
			{ID: "phase-b", NodeIDs: []string{"b"}},
		},
	}

	ctx := &execution.Context{Context: context.Background(), Graph: graph}
	op := &flow.Choose{}

	result, undo, err := op.Do(ctx, map[string]any{
		"input": "hello",
		"cases": []execution.ChooseCase{
			{Predicate: &flowPredicate{match: true}, PhaseID: "phase-a"},
			{Predicate: &flowPredicate{match: false}, PhaseID: "phase-b"},
		},
	})
	if err != nil {
		t.Fatalf("choose Do: %v", err)
	}
	if result != "result-a" {
		t.Errorf("expected result-a, got %v", result)
	}
	if undo == nil {
		t.Error("expected non-nil undo state")
	}
}

func TestFlowChooseDoDefault(t *testing.T) {
	nodeD := &execution.Node{ID: "d", Action: &flowAction{name: "test.d", result: "default-result"}}

	graph := &execution.Graph{
		Nodes: []*execution.Node{nodeD},
		Phases: []*execution.Phase{
			{ID: "phase-default", NodeIDs: []string{"d"}},
		},
	}

	ctx := &execution.Context{Context: context.Background(), Graph: graph}
	op := &flow.Choose{}

	result, _, err := op.Do(ctx, map[string]any{
		"input": "hello",
		"cases": []execution.ChooseCase{
			{Predicate: &flowPredicate{match: false}, PhaseID: "phase-default"},
		},
		"default": "phase-default",
	})
	if err != nil {
		t.Fatalf("choose Do default: %v", err)
	}
	if result != "default-result" {
		t.Errorf("expected default-result, got %v", result)
	}
}

func TestFlowChooseDoNoMatch(t *testing.T) {
	graph := &execution.Graph{}
	ctx := &execution.Context{Context: context.Background(), Graph: graph}
	op := &flow.Choose{}

	_, _, err := op.Do(ctx, map[string]any{
		"input": "hello",
		"cases": []execution.ChooseCase{
			{Predicate: &flowPredicate{match: false}, PhaseID: "phase-x"},
		},
	})
	if err == nil {
		t.Fatal("expected error when no predicate matches and no default")
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

// --- Gather tests ---

func TestFlowGatherDo(t *testing.T) {
	bodyNode := &execution.Node{ID: "body", Action: &flowAction{name: "test.body", result: "processed"}}

	graph := &execution.Graph{
		Nodes: []*execution.Node{bodyNode},
		Phases: []*execution.Phase{
			{ID: "gather-body", NodeIDs: []string{"body"}},
		},
	}

	ctx := &execution.Context{
		Context: context.Background(),
		Graph:   graph,
		NodeID:  "gather-1",
	}

	op := &flow.Gather{}
	result, undo, err := op.Do(ctx, map[string]any{
		"items": []any{"x", "y", "z"},
		"do":    "gather-body",
	})
	if err != nil {
		t.Fatalf("gather Do: %v", err)
	}

	results, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any result, got %T", result)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if r != "processed" {
			t.Errorf("result[%d]: expected 'processed', got %v", i, r)
		}
	}
	if undo == nil {
		t.Fatal("expected non-nil undo state")
	}

	gs, ok := undo.(*execution.GatherUndoState)
	if !ok {
		t.Fatalf("expected *GatherUndoState, got %T", undo)
	}
	if len(gs.Iterations) != 3 {
		t.Errorf("expected 3 iteration undo entries, got %d", len(gs.Iterations))
	}
}

func TestFlowGatherDoEmpty(t *testing.T) {
	ctx := &execution.Context{Context: context.Background()}
	op := &flow.Gather{}

	result, undo, err := op.Do(ctx, map[string]any{
		"items": []any{},
		"do":    "any-phase",
	})
	if err != nil {
		t.Fatalf("gather Do empty: %v", err)
	}
	results, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
	if undo == nil {
		t.Error("expected non-nil undo state")
	}
}

func TestFlowGatherDoConcurrent(t *testing.T) {
	bodyNode := &execution.Node{ID: "body", Action: &flowAction{name: "test.body", result: "done"}}

	graph := &execution.Graph{
		Nodes: []*execution.Node{bodyNode},
		Phases: []*execution.Phase{
			{ID: "body-phase", NodeIDs: []string{"body"}},
		},
	}

	ctx := &execution.Context{
		Context: context.Background(),
		Graph:   graph,
		NodeID:  "gather-concurrent",
	}

	op := &flow.Gather{}
	result, undo, err := op.Do(ctx, map[string]any{
		"items": []any{"a", "b", "c", "d", "e"},
		"do":    "body-phase",
		"limit": 3,
	})
	if err != nil {
		t.Fatalf("gather concurrent Do: %v", err)
	}

	results, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result)
	}
	if len(results) != 5 {
		t.Errorf("expected 5 results, got %d", len(results))
	}

	gs := undo.(*execution.GatherUndoState)
	if len(gs.Iterations) != 5 {
		t.Errorf("expected 5 iteration undo entries, got %d", len(gs.Iterations))
	}
}

func TestFlowGatherDoProxySlots(t *testing.T) {
	// Body node with a proxy slot that resolves per-item.
	bodyNode := &execution.Node{
		ID:     "body",
		Action: &flowAction{name: "test.body", result: "done"},
		Slots: map[string]execution.SlotValue{
			"name": {GatherRef: "gather-proxy", Field: "name"},
		},
	}

	graph := &execution.Graph{
		Nodes: []*execution.Node{bodyNode},
		Phases: []*execution.Phase{
			{ID: "body-phase", NodeIDs: []string{"body"}},
		},
	}

	ctx := &execution.Context{
		Context: context.Background(),
		Graph:   graph,
		NodeID:  "gather-proxy",
	}

	op := &flow.Gather{}
	items := []any{
		map[string]any{"name": "alpha"},
		map[string]any{"name": "beta"},
	}

	result, _, err := op.Do(ctx, map[string]any{
		"items": items,
		"do":    "body-phase",
	})
	if err != nil {
		t.Fatalf("gather proxy Do: %v", err)
	}

	results, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
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

// --- Elevate tests ---

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

func TestFlowElevateNotCompensableAction(t *testing.T) {
	var action execution.Action = &flow.Elevate{}
	if _, ok := action.(execution.CompensableAction); ok {
		t.Error("Elevate should not implement CompensableAction")
	}
}

// --- WaitUntil tests ---

func TestFlowWaitUntilDoImmediate(t *testing.T) {
	ctx := &execution.Context{Context: context.Background()}
	op := &flow.WaitUntil{}

	result, _, err := op.Do(ctx, map[string]any{
		"target":    "ready",
		"predicate": &flowPredicate{match: true},
		"timeout":   "10s",
	})
	if err != nil {
		t.Fatalf("wait_until Do: %v", err)
	}
	if result != "ready" {
		t.Errorf("expected 'ready', got %v", result)
	}
}

func TestFlowWaitUntilNotCompensableAction(t *testing.T) {
	var action execution.Action = &flow.WaitUntil{}
	if _, ok := action.(execution.CompensableAction); ok {
		t.Error("WaitUntil should not implement CompensableAction")
	}
}

// --- Name tests ---

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

func TestFlowWaitUntilName(t *testing.T) {
	op := &flow.WaitUntil{}
	if op.Name() != "flow.wait_until" {
		t.Errorf("expected 'flow.wait_until', got %q", op.Name())
	}
}

// --- Integration test ---

// TestGatherIntegration tests Gather via phased execution with a real graph.
func TestGatherIntegration(t *testing.T) {
	bodyNode := &execution.Node{
		ID:     "body-action",
		Action: &flowAction{name: "test.body", result: "done"},
	}

	gatherNode := &execution.Node{
		ID:     "gather",
		Action: &flow.Gather{},
		Slots: map[string]execution.SlotValue{
			"items": {Immediate: []any{"a", "b", "c"}},
			"do":    {Immediate: "body-phase"},
		},
	}

	// Main phase contains the gather node; body phase is executed by gather.
	mainPhase := &execution.Phase{ID: "main", NodeIDs: []string{"gather"}}
	bodyPhase := &execution.Phase{ID: "body-phase", NodeIDs: []string{"body-action"}}

	graph := &execution.Graph{
		State:  execution.StatePending,
		Nodes:  []*execution.Node{gatherNode, bodyNode},
		Phases: []*execution.Phase{mainPhase, bodyPhase},
	}

	engine := execution.NewGraphExecutor(execution.ExecutorOptions{})
	err := engine.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if gatherNode.Status != execution.StatusCompleted {
		t.Errorf("gather status: expected completed, got %s", gatherNode.Status)
	}
}
