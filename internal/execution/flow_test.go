// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

func (a *flowAction) Name() string           { return a.name }
func (a *flowAction) Params() []op.ParamInfo { return nil }
func (a *flowAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.Complement, err error) {
	return a.result, "undo:" + a.name, nil
}
func (a *flowAction) Undo(_ *op.Context, _ op.Complement) error {
	return nil
}

// failingFlowAction always returns a configured error from Do.
type failingFlowAction struct {
	name string
	err  error
}

func (a *failingFlowAction) Name() string           { return a.name }
func (a *failingFlowAction) Params() []op.ParamInfo { return nil }
func (a *failingFlowAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.Complement, err error) {
	return nil, nil, a.err
}
func (a *failingFlowAction) Undo(_ *op.Context, _ op.Complement) error {
	return nil
}

// trackingFlowAction succeeds on Do and tracks whether Undo was called.
type trackingFlowAction struct {
	name   string
	result op.Result
	undone bool
}

func (a *trackingFlowAction) Name() string           { return a.name }
func (a *trackingFlowAction) Params() []op.ParamInfo { return nil }
func (a *trackingFlowAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.Complement, err error) {
	return a.result, "undo:" + a.name, nil
}
func (a *trackingFlowAction) Undo(_ *op.Context, _ op.Complement) error {
	a.undone = true
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

func TestFlowChooseImplementsCompensableAction(t *testing.T) {
	var action op.Action = &flow.Choose{}
	if _, ok := action.(op.CompensableAction); !ok {
		t.Error("Choose should implement CompensableAction")
	}
}

func TestFlowChooseDoNilGraph(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &flow.Choose{}

	_, _, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "some-phase",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no graph in context") {
		t.Errorf("expected 'no graph in context' error, got: %v", err)
	}
}

func TestFlowChooseDoPhaseNotFound(t *testing.T) {
	graph := &op.Graph{
		Phases: []*op.Phase{},
	}
	ctx := &op.Context{Context: context.Background(), Graph: graph}
	act := &flow.Choose{}

	_, _, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "phase") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'phase not found' error, got: %v", err)
	}
}

func TestFlowChooseDoMultiNodePhase(t *testing.T) {
	nodeA := &op.Node{ID: "a", Action: &flowAction{name: "test.a", result: "result-a"}}
	nodeB := &op.Node{ID: "b", Action: &flowAction{name: "test.b", result: "result-b"}}
	nodeC := &op.Node{ID: "c", Action: &flowAction{name: "test.c", result: "result-c"}}

	graph := &op.Graph{
		Nodes: []*op.Node{nodeA, nodeB, nodeC},
		Edges: []op.Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
		},
		Phases: []*op.Phase{
			{ID: "phase-abc", NodeIDs: []string{"a", "b", "c"}},
		},
	}

	ctx := &op.Context{Context: context.Background(), Graph: graph}
	act := &flow.Choose{}

	result, _, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "phase-abc",
	})
	if err != nil {
		t.Fatalf("choose Do: %v", err)
	}
	if result != "result-c" {
		t.Errorf("expected terminal result 'result-c', got %v", result)
	}
}

func TestFlowChooseDoErrorPropagation(t *testing.T) {
	bodyErr := errors.New("action failed")
	node := &op.Node{ID: "bad", Action: &failingFlowAction{name: "test.bad", err: bodyErr}}

	graph := &op.Graph{
		Nodes: []*op.Node{node},
		Phases: []*op.Phase{
			{ID: "phase-err", NodeIDs: []string{"bad"}},
		},
	}

	ctx := &op.Context{Context: context.Background(), Graph: graph}
	act := &flow.Choose{}

	_, _, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "phase-err",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "phase-err") {
		t.Errorf("expected phase ID in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("expected node ID in error, got: %v", err)
	}
	if !errors.Is(err, bodyErr) {
		t.Errorf("expected wrapped body error, got: %v", err)
	}
}

func TestFlowChooseDoErrorUnwindsCompletedNodes(t *testing.T) {
	step1 := &trackingFlowAction{name: "test.step1", result: "r1"}
	step2Err := errors.New("step2 boom")
	step2 := &failingFlowAction{name: "test.step2", err: step2Err}
	step3 := &trackingFlowAction{name: "test.step3", result: "r3"}

	nodeA := &op.Node{ID: "a", Action: step1}
	nodeB := &op.Node{ID: "b", Action: step2}
	nodeC := &op.Node{ID: "c", Action: step3}

	graph := &op.Graph{
		Nodes: []*op.Node{nodeA, nodeB, nodeC},
		Edges: []op.Edge{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
		},
		Phases: []*op.Phase{
			{ID: "phase-unwind", NodeIDs: []string{"a", "b", "c"}},
		},
	}

	ctx := &op.Context{Context: context.Background(), Graph: graph}
	act := &flow.Choose{}

	_, _, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "phase-unwind",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, step2Err) {
		t.Errorf("expected wrapped step2 error, got: %v", err)
	}
	if !step1.undone {
		t.Error("expected step1 to be undone after step2 failed")
	}
	if step3.undone {
		t.Error("step3 should not have been reached")
	}
}

func TestFlowChooseUndoWithState(t *testing.T) {
	step1 := &trackingFlowAction{name: "test.step1", result: "r1"}
	step2 := &trackingFlowAction{name: "test.step2", result: "r2"}

	nodeA := &op.Node{ID: "a", Action: step1}
	nodeB := &op.Node{ID: "b", Action: step2}

	graph := &op.Graph{
		Nodes: []*op.Node{nodeA, nodeB},
		Edges: []op.Edge{{From: "a", To: "b"}},
		Phases: []*op.Phase{
			{ID: "phase-undo", NodeIDs: []string{"a", "b"}},
		},
	}

	ctx := &op.Context{Context: context.Background(), Graph: graph}
	act := &flow.Choose{}

	_, undoState, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "phase-undo",
	})
	if err != nil {
		t.Fatalf("choose Do: %v", err)
	}
	if undoState == nil {
		t.Fatal("expected non-nil undo state")
	}

	// Reset tracking flags to isolate Undo effects.
	step1.undone = false
	step2.undone = false

	err = act.Undo(ctx, undoState)
	if err != nil {
		t.Fatalf("choose Undo: %v", err)
	}
	if !step1.undone {
		t.Error("expected step1 to be undone")
	}
	if !step2.undone {
		t.Error("expected step2 to be undone")
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

func TestFlowGatherImplementsCompensableAction(t *testing.T) {
	var action op.Action = &flow.Gather{}
	if _, ok := action.(op.CompensableAction); !ok {
		t.Error("Gather should implement CompensableAction")
	}
}

func TestFlowGatherDoMissingItemsSlot(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.Gather{}

	_, _, err := action.Do(ctx, map[string]any{
		"do": "some-phase",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing 'items' slot") {
		t.Errorf("expected 'missing items' error, got: %v", err)
	}
}

func TestFlowGatherDoInvalidItemsType(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.Gather{}

	_, _, err := action.Do(ctx, map[string]any{
		"items": "not-a-slice",
		"do":    "some-phase",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "'items' slot must be []any") {
		t.Errorf("expected 'items must be []any' error, got: %v", err)
	}
}

func TestFlowGatherDoMissingDoSlot(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.Gather{}

	_, _, err := action.Do(ctx, map[string]any{
		"items": []any{"a"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing or invalid 'do' slot") {
		t.Errorf("expected 'missing do' error, got: %v", err)
	}
}

func TestFlowGatherDoMissingGraph(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.Gather{}

	_, _, err := action.Do(ctx, map[string]any{
		"items": []any{"a"},
		"do":    "some-phase",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no graph in context") {
		t.Errorf("expected 'no graph' error, got: %v", err)
	}
}

func TestFlowGatherDoPhaseNotFound(t *testing.T) {
	graph := &op.Graph{}
	ctx := &op.Context{Context: context.Background(), Graph: graph}
	action := &flow.Gather{}

	_, _, err := action.Do(ctx, map[string]any{
		"items": []any{"a"},
		"do":    "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestFlowGatherDoSequentialError(t *testing.T) {
	bodyErr := errors.New("body failed")
	bodyNode := &op.Node{ID: "body", Action: &failingFlowAction{name: "test.body", err: bodyErr}}

	graph := &op.Graph{
		Nodes: []*op.Node{bodyNode},
		Phases: []*op.Phase{
			{ID: "body-phase", NodeIDs: []string{"body"}},
		},
	}

	ctx := &op.Context{
		Context: context.Background(),
		Graph:   graph,
		NodeID:  "gather-seq-err",
	}

	action := &flow.Gather{}
	_, _, err := action.Do(ctx, map[string]any{
		"items": []any{"a", "b"},
		"do":    "body-phase",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "iteration 0 failed") {
		t.Errorf("expected 'iteration 0 failed' error, got: %v", err)
	}
	if !errors.Is(err, bodyErr) {
		t.Errorf("expected wrapped body error, got: %v", err)
	}
}

func TestFlowGatherDoConcurrentError(t *testing.T) {
	bodyErr := errors.New("concurrent body failed")
	bodyNode := &op.Node{ID: "body", Action: &failingFlowAction{name: "test.body", err: bodyErr}}

	graph := &op.Graph{
		Nodes: []*op.Node{bodyNode},
		Phases: []*op.Phase{
			{ID: "body-phase", NodeIDs: []string{"body"}},
		},
	}

	ctx := &op.Context{
		Context: context.Background(),
		Graph:   graph,
		NodeID:  "gather-concurrent-err",
	}

	action := &flow.Gather{}
	_, _, err := action.Do(ctx, map[string]any{
		"items": []any{"a", "b", "c"},
		"do":    "body-phase",
		"limit": 2,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "iteration") || !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected iteration failure error, got: %v", err)
	}
}

func TestFlowGatherUndoWithState(t *testing.T) {
	bodyAction := &trackingFlowAction{name: "test.body", result: "done"}
	bodyNode := &op.Node{ID: "body", Action: bodyAction}

	graph := &op.Graph{
		Nodes: []*op.Node{bodyNode},
		Phases: []*op.Phase{
			{ID: "body-phase", NodeIDs: []string{"body"}},
		},
	}

	ctx := &op.Context{
		Context: context.Background(),
		Graph:   graph,
		NodeID:  "gather-undo",
	}

	action := &flow.Gather{}
	_, undoState, err := action.Do(ctx, map[string]any{
		"items": []any{"x", "y"},
		"do":    "body-phase",
	})
	if err != nil {
		t.Fatalf("gather Do: %v", err)
	}
	if undoState == nil {
		t.Fatal("expected non-nil undo state")
	}

	// Reset tracking to isolate Undo effects.
	bodyAction.undone = false

	err = action.Undo(ctx, undoState)
	if err != nil {
		t.Fatalf("gather Undo: %v", err)
	}
	if !bodyAction.undone {
		t.Error("expected body action to be undone")
	}
}

// --- Elevate tests ---

func TestFlowElevateDo(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.Elevate{}

	result, _, err := action.Do(ctx, nil)
	if err != nil {
		t.Fatalf("flow.elevate Do: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
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

func TestFlowWaitUntilDoPolling(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.WaitUntil{}

	calls := 0
	pred := flow.PredicateFunc(func(_ any) (bool, error) {
		calls++
		return calls >= 3, nil
	})

	result, _, err := action.Do(ctx, map[string]any{
		"target":    "ready",
		"predicate": pred,
		"timeout":   "5s",
		"interval":  "10ms",
	})
	if err != nil {
		t.Fatalf("wait_until Do polling: %v", err)
	}
	if result != "ready" {
		t.Errorf("expected 'ready', got %v", result)
	}
	if calls < 3 {
		t.Errorf("expected at least 3 predicate calls, got %d", calls)
	}
}

func TestFlowWaitUntilDoTimeout(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.WaitUntil{}

	pred := flow.PredicateFunc(func(_ any) (bool, error) {
		return false, nil
	})

	_, _, err := action.Do(ctx, map[string]any{
		"target":    "never",
		"predicate": pred,
		"timeout":   "50ms",
		"interval":  "10ms",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout after") {
		t.Errorf("expected 'timeout after' error, got: %v", err)
	}
}

func TestFlowWaitUntilDoContextCancellation(t *testing.T) {
	baseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	ctx := &op.Context{Context: baseCtx}
	action := &flow.WaitUntil{}

	pred := flow.PredicateFunc(func(_ any) (bool, error) {
		return false, nil
	})

	_, _, err := action.Do(ctx, map[string]any{
		"target":    "never",
		"predicate": pred,
		"timeout":   "10s",
		"interval":  "100ms",
	})
	if err == nil {
		t.Fatal("expected context error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}
}

func TestFlowWaitUntilDoMissingPredicate(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.WaitUntil{}

	_, _, err := action.Do(ctx, map[string]any{
		"target":  "ready",
		"timeout": "5s",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing or invalid 'predicate' slot") {
		t.Errorf("expected 'missing predicate' error, got: %v", err)
	}
}

func TestFlowWaitUntilDoMissingTimeout(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.WaitUntil{}

	_, _, err := action.Do(ctx, map[string]any{
		"target":    "ready",
		"predicate": flow.PredicateFunc(func(_ any) (bool, error) { return true, nil }),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "'timeout' slot is required") {
		t.Errorf("expected 'timeout required' error, got: %v", err)
	}
}

func TestFlowWaitUntilDoInvalidTimeoutDuration(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.WaitUntil{}

	_, _, err := action.Do(ctx, map[string]any{
		"target":    "ready",
		"predicate": flow.PredicateFunc(func(_ any) (bool, error) { return true, nil }),
		"timeout":   "not-a-duration",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid timeout duration") {
		t.Errorf("expected 'invalid timeout duration' error, got: %v", err)
	}
}

func TestFlowWaitUntilDoPredicateError(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	action := &flow.WaitUntil{}

	predErr := errors.New("check failed")
	pred := flow.PredicateFunc(func(_ any) (bool, error) {
		return false, predErr
	})

	_, _, err := action.Do(ctx, map[string]any{
		"target":    "ready",
		"predicate": pred,
		"timeout":   "5s",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "predicate error") {
		t.Errorf("expected 'predicate error', got: %v", err)
	}
	if !errors.Is(err, predErr) {
		t.Errorf("expected wrapped predicate error, got: %v", err)
	}
}

// --- Register test ---

func TestFlowRegister(t *testing.T) {
	reg := op.NewActionRegistry()
	op.InitAll(reg, op.Context{})

	for _, name := range []string{"flow.choose", "flow.gather", "flow.elevate", "flow.wait_until"} {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("expected %q to be registered via InitAll", name)
		}
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

	engine := execution.NewGraphExecutor(execution.ExecutorOptions{Root: t.TempDir()})
	err := engine.Run(context.Background(), graph)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if gatherNode.Status != op.StatusCompleted {
		t.Errorf("gather status: expected completed, got %s", gatherNode.Status)
	}
}
