// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// ---------------------------------------------------------------------------
// Mock action types
// ---------------------------------------------------------------------------

// echoAction is a CompensableAction that returns its name as result and
// "undo:<name>" as undo state. Undo appends the name to undoOrder (if set)
// and records the received UndoState in undoneWith.
type echoAction struct {
	name       string
	undoOrder  *[]string // optional shared tracker for undo-order verification
	undoneWith any       // last UndoState received by Undo
}

func (a *echoAction) Name() string { return a.name }

func (a *echoAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.UndoState, err error) {
	return a.name, "undo:" + a.name, nil
}

func (a *echoAction) Undo(_ *op.Context, state op.UndoState) error {
	a.undoneWith = state
	if a.undoOrder != nil {
		*a.undoOrder = append(*a.undoOrder, a.name)
	}
	return nil
}

// failAction is a plain Action (not CompensableAction) whose Do always
// returns the configured error.
type failAction struct {
	name string
	err  error
}

func (a *failAction) Name() string { return a.name }

func (a *failAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.UndoState, err error) {
	return nil, nil, a.err
}

// notCompensableAction implements CompensableAction but its Undo always
// returns ErrNotCompensable. Do succeeds normally.
type notCompensableAction struct {
	name string
}

func (a *notCompensableAction) Name() string { return a.name }

func (a *notCompensableAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.UndoState, err error) {
	return a.name, nil, nil
}

func (a *notCompensableAction) Undo(_ *op.Context, _ op.UndoState) error {
	return op.ErrNotCompensable
}

// failUndoAction implements CompensableAction with a Do that succeeds and
// an Undo that always returns the configured error.
type failUndoAction struct {
	name string
	err  error
}

func (a *failUndoAction) Name() string { return a.name }

func (a *failUndoAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.UndoState, err error) {
	return a.name, nil, nil
}

func (a *failUndoAction) Undo(_ *op.Context, _ op.UndoState) error {
	return a.err
}

// countAction is a CompensableAction that counts Do calls and optionally
// fails on the Nth call (1-based). Thread-safe for concurrent gather tests.
type countAction struct {
	name   string
	failAt int // 1-based call number to fail on; 0 = never fail
	mu     sync.Mutex
	calls  int
	undone int
}

func (a *countAction) Name() string { return a.name }

func (a *countAction) Do(_ *op.Context, _ map[string]any) (result op.Result, undo op.UndoState, err error) {
	a.mu.Lock()
	a.calls++
	n := a.calls
	a.mu.Unlock()

	if a.failAt > 0 && n == a.failAt {
		return nil, nil, fmt.Errorf("call %d failed", n)
	}
	return fmt.Sprintf("r%d", n), fmt.Sprintf("u%d", n), nil
}

func (a *countAction) Undo(_ *op.Context, _ op.UndoState) error {
	a.mu.Lock()
	a.undone++
	a.mu.Unlock()
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildGraph constructs an op.Graph with one phase containing the given nodes
// connected by sequential edges (A→B→C→…).
func buildGraph(phaseID string, nodes ...*op.Node) *op.Graph {
	ids := make([]string, len(nodes))
	var edges []op.Edge
	for i, n := range nodes {
		ids[i] = n.ID
		if i > 0 {
			edges = append(edges, op.Edge{From: nodes[i-1].ID, To: n.ID})
		}
	}
	return &op.Graph{
		Nodes:  nodes,
		Edges:  edges,
		Phases: []*op.Phase{{ID: phaseID, NodeIDs: ids}},
	}
}

// flowContext creates an op.Context suitable for flow action tests.
func flowContext(graph *op.Graph, nodeID string) *op.Context {
	return &op.Context{
		Context: context.Background(),
		Graph:   graph,
		NodeID:  nodeID,
	}
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

func TestRegisterAddsAllActions(t *testing.T) {
	reg := op.NewActionRegistry()
	Register(reg)

	want := []string{"flow.choose", "flow.gather", "flow.elevate", "flow.wait_until"}
	for _, name := range want {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("expected %q to be registered", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Elevate
// ---------------------------------------------------------------------------

func TestElevateName(t *testing.T) {
	act := &Elevate{}
	if got := act.Name(); got != "flow.elevate" {
		t.Errorf("Name() = %q, want %q", got, "flow.elevate")
	}
}

func TestElevateDoReturnsNil(t *testing.T) {
	act := &Elevate{}
	result, undo, err := act.Do(&op.Context{Context: context.Background()}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil", undo)
	}
}

func TestElevateIsNotCompensable(t *testing.T) {
	var act op.Action = &Elevate{}
	if _, ok := act.(op.CompensableAction); ok {
		t.Error("Elevate should NOT implement CompensableAction")
	}
}

// ---------------------------------------------------------------------------
// Choose
// ---------------------------------------------------------------------------

func TestChooseName(t *testing.T) {
	act := &Choose{}
	if got := act.Name(); got != "flow.choose" {
		t.Errorf("Name() = %q, want %q", got, "flow.choose")
	}
}

func TestChooseIsCompensable(t *testing.T) {
	var act op.Action = &Choose{}
	if _, ok := act.(op.CompensableAction); !ok {
		t.Error("Choose should implement CompensableAction")
	}
}

func TestChooseWhenTrue(t *testing.T) {
	node := &op.Node{ID: "a", Action: &echoAction{name: "alpha"}}
	graph := buildGraph("then-phase", node)
	ctx := flowContext(graph, "choose")

	act := &Choose{}
	result, undo, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "then-phase",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "alpha" {
		t.Errorf("result = %v, want %q", result, "alpha")
	}
	if undo == nil {
		t.Error("expected non-nil undo state")
	}
}

func TestChooseWhenFalse(t *testing.T) {
	node := &op.Node{ID: "e", Action: &echoAction{name: "else-val"}}
	graph := buildGraph("else-phase", node)
	ctx := flowContext(graph, "choose")

	act := &Choose{}
	result, _, err := act.Do(ctx, map[string]any{
		"when": false,
		"else": "else-phase",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "else-val" {
		t.Errorf("result = %v, want %q", result, "else-val")
	}
}

func TestChooseWhenFalseNoElse(t *testing.T) {
	graph := &op.Graph{}
	ctx := flowContext(graph, "choose")

	act := &Choose{}
	result, undo, err := act.Do(ctx, map[string]any{
		"when": false,
		"then": "then-phase",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil", undo)
	}
}

func TestChooseWhenMissing(t *testing.T) {
	// "when" slot absent defaults to false; no "else" provided → no-op.
	graph := &op.Graph{}
	ctx := flowContext(graph, "choose")

	act := &Choose{}
	result, undo, err := act.Do(ctx, map[string]any{
		"then": "then-phase",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil", undo)
	}
}

func TestChooseMultiNodePhase(t *testing.T) {
	nodeA := &op.Node{ID: "a", Action: &echoAction{name: "alpha"}}
	nodeB := &op.Node{ID: "b", Action: &echoAction{name: "bravo"}}
	nodeC := &op.Node{ID: "c", Action: &echoAction{name: "charlie"}}
	graph := buildGraph("phase-abc", nodeA, nodeB, nodeC)
	ctx := flowContext(graph, "choose")

	act := &Choose{}
	result, _, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "phase-abc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "charlie" {
		t.Errorf("terminal result = %v, want %q", result, "charlie")
	}
}

func TestChooseNilGraph(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &Choose{}

	_, _, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "phase",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no graph in context") {
		t.Errorf("error = %v, want 'no graph in context'", err)
	}
}

func TestChoosePhaseNotFound(t *testing.T) {
	graph := &op.Graph{}
	ctx := flowContext(graph, "choose")
	act := &Choose{}

	_, _, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found'", err)
	}
}

func TestChooseNodeError(t *testing.T) {
	bodyErr := errors.New("kaboom")
	node := &op.Node{ID: "bad", Action: &failAction{name: "test.bad", err: bodyErr}}
	graph := buildGraph("err-phase", node)
	ctx := flowContext(graph, "choose")

	act := &Choose{}
	_, _, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "err-phase",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "err-phase") {
		t.Errorf("error should mention phase ID, got: %v", err)
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("error should mention node ID, got: %v", err)
	}
	if !errors.Is(err, bodyErr) {
		t.Errorf("error should wrap body error, got: %v", err)
	}
}

func TestChooseNodeErrorUnwindsCompleted(t *testing.T) {
	step1 := &echoAction{name: "step1"}
	step2Err := errors.New("step2 boom")
	step2 := &failAction{name: "step2", err: step2Err}

	nodeA := &op.Node{ID: "a", Action: step1}
	nodeB := &op.Node{ID: "b", Action: step2}
	graph := buildGraph("unwind-phase", nodeA, nodeB)
	ctx := flowContext(graph, "choose")

	act := &Choose{}
	_, _, err := act.Do(ctx, map[string]any{
		"when": true,
		"then": "unwind-phase",
	})
	if !errors.Is(err, step2Err) {
		t.Fatalf("expected step2 error, got: %v", err)
	}
	if step1.undoneWith == nil {
		t.Error("step1 should have been undone after step2 failed")
	}
}

func TestChooseUndoNilState(t *testing.T) {
	act := &Choose{}
	if err := act.Undo(nil, nil); err != nil {
		t.Fatalf("Undo(nil) error: %v", err)
	}
}

func TestChooseUndoReverseOrder(t *testing.T) {
	var order []string
	actionA := &echoAction{name: "a", undoOrder: &order}
	actionB := &echoAction{name: "b", undoOrder: &order}
	actionC := &echoAction{name: "c", undoOrder: &order}

	state := &chooseUndoState{
		Entries: []execution.RecoveryEntry{
			{Node: &op.Node{ID: "n1", Action: actionA}, UndoState: "sa"},
			{Node: &op.Node{ID: "n2", Action: actionB}, UndoState: "sb"},
			{Node: &op.Node{ID: "n3", Action: actionC}, UndoState: "sc"},
		},
	}

	act := &Choose{}
	err := act.Undo(&op.Context{Context: context.Background()}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"c", "b", "a"}
	if !slices.Equal(order, want) {
		t.Errorf("undo order = %v, want %v", order, want)
	}
}

func TestChooseUndoSkipsNotCompensable(t *testing.T) {
	var order []string
	actionA := &echoAction{name: "a", undoOrder: &order}
	actionNC := &notCompensableAction{name: "nc"}
	actionC := &echoAction{name: "c", undoOrder: &order}

	state := &chooseUndoState{
		Entries: []execution.RecoveryEntry{
			{Node: &op.Node{ID: "n1", Action: actionA}},
			{Node: &op.Node{ID: "n2", Action: actionNC}},
			{Node: &op.Node{ID: "n3", Action: actionC}},
		},
	}

	act := &Choose{}
	err := act.Undo(&op.Context{Context: context.Background()}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"c", "a"}
	if !slices.Equal(order, want) {
		t.Errorf("undo order = %v, want %v (NC should be skipped)", order, want)
	}
}

func TestChooseUndoCollectsErrors(t *testing.T) {
	errA := errors.New("undo-a failed")
	errB := errors.New("undo-b failed")

	state := &chooseUndoState{
		Entries: []execution.RecoveryEntry{
			{Node: &op.Node{ID: "n1", Action: &failUndoAction{name: "a", err: errA}}},
			{Node: &op.Node{ID: "n2", Action: &failUndoAction{name: "b", err: errB}}},
		},
	}

	act := &Choose{}
	err := act.Undo(&op.Context{Context: context.Background()}, state)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errA) {
		t.Errorf("joined error should contain errA: %v", err)
	}
	if !errors.Is(err, errB) {
		t.Errorf("joined error should contain errB: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Gather
// ---------------------------------------------------------------------------

func TestGatherName(t *testing.T) {
	act := &Gather{}
	if got := act.Name(); got != "flow.gather" {
		t.Errorf("Name() = %q, want %q", got, "flow.gather")
	}
}

func TestGatherIsCompensable(t *testing.T) {
	var act op.Action = &Gather{}
	if _, ok := act.(op.CompensableAction); !ok {
		t.Error("Gather should implement CompensableAction")
	}
}

func TestGatherSequential(t *testing.T) {
	body := &op.Node{ID: "body", Action: &echoAction{name: "echo"}}
	graph := buildGraph("body-phase", body)
	ctx := flowContext(graph, "gather")

	act := &Gather{}
	result, undo, err := act.Do(ctx, map[string]any{
		"items": []any{"a", "b", "c"},
		"do":    "body-phase",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results, ok := result.([]any)
	if !ok {
		t.Fatalf("result type = %T, want []any", result)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
	if undo == nil {
		t.Error("expected non-nil undo state")
	}
}

func TestGatherConcurrent(t *testing.T) {
	body := &op.Node{ID: "body", Action: &echoAction{name: "echo"}}
	graph := buildGraph("body-phase", body)
	ctx := flowContext(graph, "gather")

	act := &Gather{}
	result, _, err := act.Do(ctx, map[string]any{
		"items": []any{"a", "b", "c", "d", "e"},
		"do":    "body-phase",
		"limit": 3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	results, ok := result.([]any)
	if !ok {
		t.Fatalf("result type = %T, want []any", result)
	}
	if len(results) != 5 {
		t.Errorf("len(results) = %d, want 5", len(results))
	}
}

func TestGatherEmptyItems(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &Gather{}

	result, undo, err := act.Do(ctx, map[string]any{
		"items": []any{},
		"do":    "any-phase",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results, ok := result.([]any)
	if !ok {
		t.Fatalf("result type = %T, want []any", result)
	}
	if len(results) != 0 {
		t.Errorf("len(results) = %d, want 0", len(results))
	}
	if undo == nil {
		t.Error("expected non-nil undo state")
	}
}

func TestGatherSingleItemUsesConcurrentPath(t *testing.T) {
	// limit=5 but len(items)=1 routes through sequential path (len <= 1).
	body := &op.Node{ID: "body", Action: &echoAction{name: "echo"}}
	graph := buildGraph("body-phase", body)
	ctx := flowContext(graph, "gather")

	act := &Gather{}
	result, _, err := act.Do(ctx, map[string]any{
		"items": []any{"solo"},
		"do":    "body-phase",
		"limit": 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results, ok := result.([]any)
	if !ok {
		t.Fatalf("result type = %T, want []any", result)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
}

func TestGatherProxySlots(t *testing.T) {
	body := &op.Node{
		ID:     "body",
		Action: &echoAction{name: "echo"},
		Slots: map[string]op.SlotValue{
			"item": {GatherRef: "gather-node", Field: "name"},
		},
	}
	graph := buildGraph("body-phase", body)
	ctx := flowContext(graph, "gather-node")

	act := &Gather{}
	items := []any{
		map[string]any{"name": "alpha"},
		map[string]any{"name": "beta"},
	}
	result, _, err := act.Do(ctx, map[string]any{
		"items": items,
		"do":    "body-phase",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	results, ok := result.([]any)
	if !ok {
		t.Fatalf("result type = %T, want []any", result)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
}

func TestGatherSequentialStopsOnError(t *testing.T) {
	// 3 items; body action fails on 2nd call.
	// Item A succeeds, item B fails, item C never runs. A is unwound.
	action := &countAction{name: "body", failAt: 2}
	body := &op.Node{ID: "body", Action: action}
	graph := buildGraph("body-phase", body)
	ctx := flowContext(graph, "gather")

	act := &Gather{}
	_, _, err := act.Do(ctx, map[string]any{
		"items": []any{"a", "b", "c"},
		"do":    "body-phase",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if action.calls != 2 {
		t.Errorf("calls = %d, want 2 (C should never run)", action.calls)
	}
	if action.undone != 1 {
		t.Errorf("undone = %d, want 1 (A should be unwound)", action.undone)
	}
}

func TestGatherConcurrentError(t *testing.T) {
	action := &countAction{name: "body", failAt: 1}
	body := &op.Node{ID: "body", Action: action}
	graph := buildGraph("body-phase", body)
	ctx := flowContext(graph, "gather")

	act := &Gather{}
	_, _, err := act.Do(ctx, map[string]any{
		"items": []any{"a", "b", "c"},
		"do":    "body-phase",
		"limit": 2,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "iteration") {
		t.Errorf("error should mention iteration, got: %v", err)
	}
}

func TestGatherMissingItems(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &Gather{}

	_, _, err := act.Do(ctx, map[string]any{"do": "phase"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing 'items' slot") {
		t.Errorf("error = %v, want 'missing items slot'", err)
	}
}

func TestGatherInvalidItemsType(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &Gather{}

	_, _, err := act.Do(ctx, map[string]any{
		"items": "not-a-slice",
		"do":    "phase",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "'items' slot must be []any") {
		t.Errorf("error = %v, want 'items must be []any'", err)
	}
}

func TestGatherMissingDoSlot(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &Gather{}

	_, _, err := act.Do(ctx, map[string]any{
		"items": []any{"a"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing or invalid 'do' slot") {
		t.Errorf("error = %v, want 'missing or invalid do slot'", err)
	}
}

func TestGatherEmptyDoSlot(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &Gather{}

	_, _, err := act.Do(ctx, map[string]any{
		"items": []any{"a"},
		"do":    "",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing or invalid 'do' slot") {
		t.Errorf("error = %v, want 'missing or invalid do slot'", err)
	}
}

func TestGatherNilGraph(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &Gather{}

	_, _, err := act.Do(ctx, map[string]any{
		"items": []any{"a"},
		"do":    "phase",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no graph in context") {
		t.Errorf("error = %v, want 'no graph in context'", err)
	}
}

func TestGatherPhaseNotFound(t *testing.T) {
	graph := &op.Graph{}
	ctx := flowContext(graph, "gather")
	act := &Gather{}

	_, _, err := act.Do(ctx, map[string]any{
		"items": []any{"a"},
		"do":    "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found'", err)
	}
}

func TestGatherUndoNilState(t *testing.T) {
	act := &Gather{}
	if err := act.Undo(nil, nil); err != nil {
		t.Fatalf("Undo(nil) error: %v", err)
	}
}

func TestGatherUndoReverseOrder(t *testing.T) {
	var order []string
	actionA := &echoAction{name: "a", undoOrder: &order}
	actionB := &echoAction{name: "b", undoOrder: &order}
	actionC := &echoAction{name: "c", undoOrder: &order}

	state := &gatherUndoState{
		Iterations: []iterationUndo{
			{Entries: []execution.RecoveryEntry{{Node: &op.Node{ID: "n1", Action: actionA}, UndoState: "sa"}}},
			{Entries: []execution.RecoveryEntry{{Node: &op.Node{ID: "n2", Action: actionB}, UndoState: "sb"}}},
			{Entries: []execution.RecoveryEntry{{Node: &op.Node{ID: "n3", Action: actionC}, UndoState: "sc"}}},
		},
	}

	act := &Gather{}
	err := act.Undo(&op.Context{Context: context.Background()}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"c", "b", "a"}
	if !slices.Equal(order, want) {
		t.Errorf("undo order = %v, want %v", order, want)
	}
}

func TestGatherUndoSkipsNotCompensable(t *testing.T) {
	var order []string
	actionA := &echoAction{name: "a", undoOrder: &order}
	actionNC := &notCompensableAction{name: "nc"}
	actionC := &echoAction{name: "c", undoOrder: &order}

	state := &gatherUndoState{
		Iterations: []iterationUndo{
			{Entries: []execution.RecoveryEntry{{Node: &op.Node{ID: "n1", Action: actionA}}}},
			{Entries: []execution.RecoveryEntry{{Node: &op.Node{ID: "n2", Action: actionNC}}}},
			{Entries: []execution.RecoveryEntry{{Node: &op.Node{ID: "n3", Action: actionC}}}},
		},
	}

	act := &Gather{}
	err := act.Undo(&op.Context{Context: context.Background()}, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"c", "a"}
	if !slices.Equal(order, want) {
		t.Errorf("undo order = %v, want %v (NC should be skipped)", order, want)
	}
}

func TestGatherUndoCollectsErrors(t *testing.T) {
	errA := errors.New("undo-a failed")
	errB := errors.New("undo-b failed")

	state := &gatherUndoState{
		Iterations: []iterationUndo{
			{Entries: []execution.RecoveryEntry{
				{Node: &op.Node{ID: "n1", Action: &failUndoAction{name: "a", err: errA}}},
			}},
			{Entries: []execution.RecoveryEntry{
				{Node: &op.Node{ID: "n2", Action: &failUndoAction{name: "b", err: errB}}},
			}},
		},
	}

	act := &Gather{}
	err := act.Undo(&op.Context{Context: context.Background()}, state)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errA) {
		t.Errorf("joined error should contain errA: %v", err)
	}
	if !errors.Is(err, errB) {
		t.Errorf("joined error should contain errB: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Gather helpers (unexported)
// ---------------------------------------------------------------------------

func TestExtractItemsValid(t *testing.T) {
	items, err := extractItems(map[string]any{"items": []any{"a", "b"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}
}

func TestExtractItemsMissing(t *testing.T) {
	_, err := extractItems(map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error = %v, want 'missing'", err)
	}
}

func TestExtractItemsWrongType(t *testing.T) {
	_, err := extractItems(map[string]any{"items": "not-a-slice"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "must be []any") {
		t.Errorf("error = %v, want 'must be []any'", err)
	}
}

func TestExtractLimitInt(t *testing.T) {
	got := extractLimit(map[string]any{"limit": 5})
	if got != 5 {
		t.Errorf("extractLimit(5) = %d, want 5", got)
	}
}

func TestExtractLimitFloat64(t *testing.T) {
	got := extractLimit(map[string]any{"limit": 3.0})
	if got != 3 {
		t.Errorf("extractLimit(3.0) = %d, want 3", got)
	}
}

func TestExtractLimitZero(t *testing.T) {
	got := extractLimit(map[string]any{"limit": 0})
	if got != 1 {
		t.Errorf("extractLimit(0) = %d, want 1 (default)", got)
	}
}

func TestExtractLimitNegative(t *testing.T) {
	got := extractLimit(map[string]any{"limit": -1})
	if got != 1 {
		t.Errorf("extractLimit(-1) = %d, want 1 (default)", got)
	}
}

func TestExtractLimitNonNumeric(t *testing.T) {
	got := extractLimit(map[string]any{"limit": "fast"})
	if got != 1 {
		t.Errorf("extractLimit(\"fast\") = %d, want 1 (default)", got)
	}
}

func TestExtractLimitMissing(t *testing.T) {
	got := extractLimit(map[string]any{})
	if got != 1 {
		t.Errorf("extractLimit(missing) = %d, want 1 (default)", got)
	}
}

// ---------------------------------------------------------------------------
// WaitUntil
// ---------------------------------------------------------------------------

func TestWaitUntilName(t *testing.T) {
	act := &WaitUntil{}
	if got := act.Name(); got != "flow.wait_until" {
		t.Errorf("Name() = %q, want %q", got, "flow.wait_until")
	}
}

func TestWaitUntilIsNotCompensable(t *testing.T) {
	var act op.Action = &WaitUntil{}
	if _, ok := act.(op.CompensableAction); ok {
		t.Error("WaitUntil should NOT implement CompensableAction")
	}
}

func TestWaitUntilImmediateMatch(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &WaitUntil{}

	result, _, err := act.Do(ctx, map[string]any{
		"target":    "ready",
		"predicate": PredicateFunc(func(_ any) (bool, error) { return true, nil }),
		"timeout":   "10s",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ready" {
		t.Errorf("result = %v, want %q", result, "ready")
	}
}

func TestWaitUntilPolling(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &WaitUntil{}

	calls := 0
	pred := PredicateFunc(func(_ any) (bool, error) {
		calls++
		return calls >= 3, nil
	})

	result, _, err := act.Do(ctx, map[string]any{
		"target":    "ready",
		"predicate": pred,
		"timeout":   "5s",
		"interval":  "10ms",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ready" {
		t.Errorf("result = %v, want %q", result, "ready")
	}
	if calls < 3 {
		t.Errorf("calls = %d, want >= 3", calls)
	}
}

func TestWaitUntilTimeout(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &WaitUntil{}

	_, _, err := act.Do(ctx, map[string]any{
		"target":    "never",
		"predicate": PredicateFunc(func(_ any) (bool, error) { return false, nil }),
		"timeout":   "50ms",
		"interval":  "10ms",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout after") {
		t.Errorf("error = %v, want 'timeout after'", err)
	}
}

func TestWaitUntilContextCancelled(t *testing.T) {
	baseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	ctx := &op.Context{Context: baseCtx}
	act := &WaitUntil{}

	_, _, err := act.Do(ctx, map[string]any{
		"target":    "never",
		"predicate": PredicateFunc(func(_ any) (bool, error) { return false, nil }),
		"timeout":   "10s",
		"interval":  "100ms",
	})
	if err == nil {
		t.Fatal("expected context error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestWaitUntilMissingPredicate(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &WaitUntil{}

	_, _, err := act.Do(ctx, map[string]any{
		"target":  "ready",
		"timeout": "5s",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing or invalid 'predicate' slot") {
		t.Errorf("error = %v, want 'missing or invalid predicate slot'", err)
	}
}

func TestWaitUntilMissingTimeout(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &WaitUntil{}

	_, _, err := act.Do(ctx, map[string]any{
		"target":    "ready",
		"predicate": PredicateFunc(func(_ any) (bool, error) { return true, nil }),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "'timeout' slot is required") {
		t.Errorf("error = %v, want 'timeout slot is required'", err)
	}
}

func TestWaitUntilInvalidTimeoutString(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &WaitUntil{}

	_, _, err := act.Do(ctx, map[string]any{
		"target":    "ready",
		"predicate": PredicateFunc(func(_ any) (bool, error) { return true, nil }),
		"timeout":   "not-a-duration",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid timeout duration") {
		t.Errorf("error = %v, want 'invalid timeout duration'", err)
	}
}

func TestWaitUntilPredicateError(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &WaitUntil{}

	predErr := errors.New("check failed")
	_, _, err := act.Do(ctx, map[string]any{
		"target":    "ready",
		"predicate": PredicateFunc(func(_ any) (bool, error) { return false, predErr }),
		"timeout":   "5s",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "predicate error") {
		t.Errorf("error should mention 'predicate error', got: %v", err)
	}
	if !errors.Is(err, predErr) {
		t.Errorf("error should wrap predErr, got: %v", err)
	}
}

func TestWaitUntilPredicateErrorDuringPolling(t *testing.T) {
	ctx := &op.Context{Context: context.Background()}
	act := &WaitUntil{}

	calls := 0
	pollErr := errors.New("poll check failed")
	pred := PredicateFunc(func(_ any) (bool, error) {
		calls++
		if calls >= 3 {
			return false, pollErr
		}
		return false, nil
	})

	_, _, err := act.Do(ctx, map[string]any{
		"target":    "ready",
		"predicate": pred,
		"timeout":   "5s",
		"interval":  "10ms",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "predicate error") {
		t.Errorf("error should mention 'predicate error', got: %v", err)
	}
	if !errors.Is(err, pollErr) {
		t.Errorf("error should wrap pollErr, got: %v", err)
	}
	if calls < 3 {
		t.Errorf("calls = %d, want >= 3 (error should happen during polling)", calls)
	}
}

// ---------------------------------------------------------------------------
// WaitUntil helpers (unexported)
// ---------------------------------------------------------------------------

func TestParseDurationSlotString(t *testing.T) {
	d, err := parseDurationSlot(map[string]any{"timeout": "5s"}, "timeout", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 5*time.Second {
		t.Errorf("duration = %v, want 5s", d)
	}
}

func TestParseDurationSlotDuration(t *testing.T) {
	d, err := parseDurationSlot(map[string]any{"timeout": 5 * time.Second}, "timeout", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 5*time.Second {
		t.Errorf("duration = %v, want 5s", d)
	}
}

func TestParseDurationSlotMissing(t *testing.T) {
	d, err := parseDurationSlot(map[string]any{}, "timeout", 42*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 42*time.Millisecond {
		t.Errorf("duration = %v, want 42ms (default)", d)
	}
}

func TestParseDurationSlotNil(t *testing.T) {
	d, err := parseDurationSlot(map[string]any{"timeout": nil}, "timeout", 42*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 42*time.Millisecond {
		t.Errorf("duration = %v, want 42ms (default)", d)
	}
}

func TestParseDurationSlotInvalidString(t *testing.T) {
	_, err := parseDurationSlot(map[string]any{"timeout": "nope"}, "timeout", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid timeout duration") {
		t.Errorf("error = %v, want 'invalid timeout duration'", err)
	}
}

func TestParseDurationSlotInvalidType(t *testing.T) {
	_, err := parseDurationSlot(map[string]any{"timeout": 42}, "timeout", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid timeout type") {
		t.Errorf("error = %v, want 'invalid timeout type'", err)
	}
}
