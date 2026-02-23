// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution_test

import (
	"context"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/execution/flow"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// --- test helpers ---

// flowAction is a simple action for flow tests.
type flowAction struct {
	name   string
	result op.Result
}

func (a *flowAction) Name() string { return a.name }
func (a *flowAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.UndoState, retErr error) {
	return a.result, "undo:" + a.name, nil
}
func (a *flowAction) Undo(_ *op.Context, _ op.UndoState) error {
	return nil
}

// --- Choose tests ---

func TestFlowChooseDoWhenTrue(t *testing.T) {
	nodeA := &op.Node{ID: "a", Action: &flowAction{name: "test.a", result: "result-a"}}

	graph := &op.Graph{
		Nodes: []*op.Node{nodeA},
		Phases: []*op.Phase{
			{ID: "phase-a", NodeIDs: []string{"a"}},
		},
	}

	ctx := &op.Context{Context: context.Background(), Graph: graph}
	act := &flow.Choose{}

	result, undo, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "phase-a",
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

func TestFlowChooseDoWhenFalseWithElse(t *testing.T) {
	nodeD := &op.Node{ID: "d", Action: &flowAction{name: "test.d", result: "else-result"}}

	graph := &op.Graph{
		Nodes: []*op.Node{nodeD},
		Phases: []*op.Phase{
			{ID: "phase-else", NodeIDs: []string{"d"}},
		},
	}

	ctx := &op.Context{Context: context.Background(), Graph: graph}
	act := &flow.Choose{}

	result, _, err := act.Do(ctx, map[string]any{
		"when": false,
		"else": "phase-else",
	})
	if err != nil {
		t.Fatalf("choose Do else: %v", err)
	}
	if result != "else-result" {
		t.Errorf("expected else-result, got %v", result)
	}
}

func TestFlowChooseDoWhenFalseNoElse(t *testing.T) {
	graph := &op.Graph{}
	ctx := &op.Context{Context: context.Background(), Graph: graph}
	action := &flow.Choose{}

	result, _, err := action.Do(ctx, map[string]any{
		"when": false,
		"then": "phase-x",
	})
	if err != nil {
		t.Fatal("expected no error for false with no else — should be no-op")
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestFlowChooseUndo(t *testing.T) {
	action := &flow.Choose{}

	err := action.Undo(nil, nil)
	if err != nil {
		t.Fatalf("flow.choose Undo: %v", err)
	}
}

// --- Gather tests ---

func TestFlowGatherDo(t *testing.T) {
	bodyNode := &op.Node{ID: "body", Action: &flowAction{name: "test.body", result: "processed"}}

	graph := &op.Graph{
		Nodes: []*op.Node{bodyNode},
		Phases: []*op.Phase{
			{ID: "gather-body", NodeIDs: []string{"body"}},
		},
	}

	ctx := &op.Context{
		Context: context.Background(),
		Graph:   graph,
		NodeID:  "gather-1",
	}

	action := &flow.Gather{}
	result, undo, err := action.Do(ctx, map[string]any{
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
}

func TestFlowGatherDoEmpty(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.Gather{}

	result, undo, err := action.Do(ctx, map[string]any{
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
	bodyNode := &op.Node{ID: "body", Action: &flowAction{name: "test.body", result: "done"}}

	graph := &op.Graph{
		Nodes: []*op.Node{bodyNode},
		Phases: []*op.Phase{
			{ID: "body-phase", NodeIDs: []string{"body"}},
		},
	}

	ctx := &op.Context{
		Context: context.Background(),
		Graph:   graph,
		NodeID:  "gather-concurrent",
	}

	action := &flow.Gather{}
	result, _, err := action.Do(ctx, map[string]any{
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
}

func TestFlowGatherDoProxySlots(t *testing.T) {
	// Body node with a proxy slot that resolves per-item.
	bodyNode := &op.Node{
		ID:     "body",
		Action: &flowAction{name: "test.body", result: "done"},
		Slots: map[string]op.SlotValue{
			"name": {GatherRef: "gather-proxy", Field: "name"},
		},
	}

	graph := &op.Graph{
		Nodes: []*op.Node{bodyNode},
		Phases: []*op.Phase{
			{ID: "body-phase", NodeIDs: []string{"body"}},
		},
	}

	ctx := &op.Context{
		Context: context.Background(),
		Graph:   graph,
		NodeID:  "gather-proxy",
	}

	action := &flow.Gather{}
	items := []any{
		map[string]any{"name": "alpha"},
		map[string]any{"name": "beta"},
	}

	result, _, err := action.Do(ctx, map[string]any{
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
	action := &flow.Gather{}

	err := action.Undo(nil, nil)
	if err != nil {
		t.Fatalf("flow.gather Undo: %v", err)
	}
}

// --- Elevate tests ---

func TestFlowElevateDo(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.Elevate{}

	result, undo, err := action.Do(ctx, nil)
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
	var action op.Action = &flow.Elevate{}
	if _, ok := action.(op.CompensableAction); ok {
		t.Error("Elevate should not implement CompensableAction")
	}
}

// --- WaitUntil tests ---

func TestFlowWaitUntilDoImmediate(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.WaitUntil{}

	result, _, err := action.Do(ctx, map[string]any{
		"target":    "ready",
		"predicate": flow.PredicateFunc(func(_ any) (bool, error) { return true, nil }),
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
	var action op.Action = &flow.WaitUntil{}
	if _, ok := action.(op.CompensableAction); ok {
		t.Error("WaitUntil should not implement CompensableAction")
	}
}

// --- Name tests ---

func TestFlowChooseName(t *testing.T) {
	action := &flow.Choose{}
	if action.Name() != "flow.choose" {
		t.Errorf("expected 'flow.choose', got %q", action.Name())
	}
}

func TestFlowGatherName(t *testing.T) {
	action := &flow.Gather{}
	if action.Name() != "flow.gather" {
		t.Errorf("expected 'flow.gather', got %q", action.Name())
	}
}

func TestFlowElevateName(t *testing.T) {
	action := &flow.Elevate{}
	if action.Name() != "flow.elevate" {
		t.Errorf("expected 'flow.elevate', got %q", action.Name())
	}
}

func TestFlowWaitUntilName(t *testing.T) {
	action := &flow.WaitUntil{}
	if action.Name() != "flow.wait_until" {
		t.Errorf("expected 'flow.wait_until', got %q", action.Name())
	}
}

// --- Integration test ---

// TestGatherIntegration tests Gather via phased execution with a real graph.
func TestGatherIntegration(t *testing.T) {
	bodyNode := &op.Node{
		ID:     "body-action",
		Action: &flowAction{name: "test.body", result: "done"},
	}

	gatherNode := &op.Node{
		ID:     "gather",
		Action: &flow.Gather{},
		Slots: map[string]op.SlotValue{
			"items": {Immediate: []any{"a", "b", "c"}},
			"do":    {Immediate: "body-phase"},
		},
	}

	// Main phase contains the gather node; body phase is executed by gather.
	mainPhase := &op.Phase{ID: "main", NodeIDs: []string{"gather"}}
	bodyPhase := &op.Phase{ID: "body-phase", NodeIDs: []string{"body-action"}}

	graph := &op.Graph{
		State:  op.StatePending,
		Nodes:  []*op.Node{gatherNode, bodyNode},
		Phases: []*op.Phase{mainPhase, bodyPhase},
	}

	engine := execution.NewGraphExecutor(execution.ExecutorOptions{})
	err := engine.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if gatherNode.Status != op.StatusCompleted {
		t.Errorf("gather status: expected completed, got %s", gatherNode.Status)
	}
}
