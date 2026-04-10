// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// ---------------------------------------------------------------------------
// Mock action types
// ---------------------------------------------------------------------------

// echoAction is a CompensableAction that returns its name as result.
type echoAction struct {
	name       string
	undoOrder  *[]string
	undoneWith any
}

func (a *echoAction) Name() string           { return a.name }
func (a *echoAction) Params() []op.Parameter { return nil }

func (a *echoAction) Do(_ *op.ExecutionContext, _ map[string]any) (op.Result, op.Complement, error) {
	return a.name, "undo:" + a.name, nil
}

func (a *echoAction) Undo(_ *op.ExecutionContext, state op.Complement) error {
	a.undoneWith = state
	if a.undoOrder != nil {
		*a.undoOrder = append(*a.undoOrder, a.name)
	}
	return nil
}

// failAction always returns the configured error from Do.
type failAction struct {
	name string
	err  error
}

func (a *failAction) Name() string           { return a.name }
func (a *failAction) Params() []op.Parameter { return nil }

func (a *failAction) Do(_ *op.ExecutionContext, _ map[string]any) (op.Result, op.Complement, error) {
	return nil, nil, a.err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// testNode creates a node with a test action override.
func testNode(id string, action op.Action) *op.Node {
	n := &op.Node{ID: id}
	n.SetAction(action)
	return n
}

// buildGraph constructs an op.Graph with one subgraph containing the given nodes
// connected by sequential edges (A→B→C→…).
func buildGraph(subgraphID string, nodes ...*op.Node) *op.Graph {
	var children []op.SubgraphChild
	var edges []op.Edge
	for i, n := range nodes {
		children = append(children, op.SubgraphChild{Node: n})
		if i > 0 {
			edges = append(edges, op.Edge{From: nodes[i-1].ID, To: n.ID})
		}
	}
	return &op.Graph{
		Children: []op.SubgraphChild{
			{Subgraph: &op.Subgraph{ID: subgraphID, Children: children, Edges: edges}},
		},
	}
}

// newProvider creates a flow Provider with a graph and bound context.
func newProvider(graph *op.Graph) *Provider {
	ctx := &op.ExecutionContext{
		Context:  context.Background(),
		Registry: op.NewReceiverRegistry(),
		Data:     map[string]any{"graph": graph},
	}
	graph.Rebind(ctx)
	return &Provider{
		ProviderBase: op.NewProviderBase(ctx),
		Graph:        graph,
	}
}

// ---------------------------------------------------------------------------
// Complete
// ---------------------------------------------------------------------------

func TestCompleteWithOutput(t *testing.T) {
	p := newProvider(&op.Graph{})
	result := p.Complete(42)
	if result != 42 {
		t.Errorf("Complete(42) = %v, want 42", result)
	}
}

func TestCompleteWithNil(t *testing.T) {
	p := newProvider(&op.Graph{})
	result := p.Complete(nil)
	if result != nil {
		t.Errorf("Complete(nil) = %v, want nil", result)
	}
}

func TestCompleteWithString(t *testing.T) {
	p := newProvider(&op.Graph{})
	result := p.Complete("done")
	if result != "done" {
		t.Errorf("Complete('done') = %v, want 'done'", result)
	}
}

// ---------------------------------------------------------------------------
// Degraded
// ---------------------------------------------------------------------------

func TestDegradedPlainString(t *testing.T) {
	p := newProvider(&op.Graph{})
	msg := p.Degraded("service unavailable", nil, nil)
	if msg != "service unavailable" {
		t.Errorf("Degraded() = %q, want %q", msg, "service unavailable")
	}
}

func TestDegradedWithArgs(t *testing.T) {
	p := newProvider(&op.Graph{})
	msg := p.Degraded("service {{index .Args 0}} is slow", []any{"auth"}, nil)
	if !strings.Contains(msg, "auth") {
		t.Errorf("Degraded() = %q, want to contain 'auth'", msg)
	}
}

func TestDegradedWithKwargs(t *testing.T) {
	p := newProvider(&op.Graph{})
	msg := p.Degraded("timeout on {{.service}}", nil, map[string]any{"service": "db"})
	if !strings.Contains(msg, "db") {
		t.Errorf("Degraded() = %q, want to contain 'db'", msg)
	}
}

// ---------------------------------------------------------------------------
// Fatal
// ---------------------------------------------------------------------------

func TestFatalPlainString(t *testing.T) {
	p := newProvider(&op.Graph{})
	err := p.Fatal("disk full", nil, nil)
	if err == nil {
		t.Fatal("Fatal() = nil, want error")
	}
	fatalErr, ok := err.(*op.FatalError)
	if !ok {
		t.Fatalf("Fatal() type = %T, want *op.FatalError", err)
	}
	if fatalErr.Message != "disk full" {
		t.Errorf("Fatal().Message = %q, want %q", fatalErr.Message, "disk full")
	}
}

func TestFatalWithArgs(t *testing.T) {
	p := newProvider(&op.Graph{})
	err := p.Fatal("disk full on {{index .Args 0}}", []any{"/dev/sda1"}, nil)
	if err == nil {
		t.Fatal("Fatal() = nil, want error")
	}
	if !strings.Contains(err.Error(), "/dev/sda1") {
		t.Errorf("Fatal().Error() = %q, want to contain '/dev/sda1'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Elevate
// ---------------------------------------------------------------------------

func TestElevate(t *testing.T) {
	p := newProvider(&op.Graph{})
	p.Elevate() // should not panic
}

// ---------------------------------------------------------------------------
// Choose
// ---------------------------------------------------------------------------

func TestChooseWhenTrue(t *testing.T) {
	node := testNode("a", &echoAction{name: "alpha"})
	graph := buildGraph("then-phase", node)
	p := newProvider(graph)

	result, err := p.Choose(true, "then-phase")
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if result != "alpha" {
		t.Errorf("result = %v, want %q", result, "alpha")
	}
}

func TestChooseWhenFalse(t *testing.T) {
	graph := &op.Graph{}
	p := newProvider(graph)

	result, err := p.Choose(false, "then-phase")
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
}

func TestChooseEmptyThen(t *testing.T) {
	graph := &op.Graph{}
	p := newProvider(graph)

	result, err := p.Choose(true, "")
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
}

func TestChooseMultiNodePhase(t *testing.T) {
	nodeA := testNode("a", &echoAction{name: "alpha"})
	nodeB := testNode("b", &echoAction{name: "bravo"})
	nodeC := testNode("c", &echoAction{name: "charlie"})
	graph := buildGraph("phase-abc", nodeA, nodeB, nodeC)
	p := newProvider(graph)

	result, err := p.Choose(true, "phase-abc")
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if result != "charlie" {
		t.Errorf("terminal result = %v, want %q", result, "charlie")
	}
}

func TestChoosePhaseNotFound(t *testing.T) {
	graph := &op.Graph{}
	p := newProvider(graph)

	_, err := p.Choose(true, "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestChooseErrorPropagation(t *testing.T) {
	bodyErr := errors.New("action failed")
	node := testNode("bad", &failAction{name: "test.bad", err: bodyErr})
	graph := buildGraph("phase-err", node)
	p := newProvider(graph)

	_, err := p.Choose(true, "phase-err")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestChooseErrorUnwindsCompletedNodes(t *testing.T) {
	step1 := &echoAction{name: "step1"}
	step2Err := errors.New("step2 boom")
	step3 := &echoAction{name: "step3"}

	nodeA := testNode("a", step1)
	nodeB := testNode("b", &failAction{name: "test.step2", err: step2Err})
	nodeC := testNode("c", step3)

	graph := buildGraph("phase-unwind", nodeA, nodeB, nodeC)
	p := newProvider(graph)

	_, err := p.Choose(true, "phase-unwind")
	if err == nil {
		t.Fatal("expected error")
	}
	// step1 should have completed, step3 should not have run.
	if nodeA.Status != op.StatusCompleted {
		t.Errorf("nodeA status = %v, want completed", nodeA.Status)
	}
}

// ---------------------------------------------------------------------------
// WaitUntil
// ---------------------------------------------------------------------------

func TestWaitUntilImmediate(t *testing.T) {
	p := newProvider(&op.Graph{})

	result, err := p.WaitUntil("ready", func(_ any) (bool, error) { return true, nil }, 10*time.Second, 0)
	if err != nil {
		t.Fatalf("WaitUntil: %v", err)
	}
	if result != "ready" {
		t.Errorf("result = %v, want %q", result, "ready")
	}
}

func TestWaitUntilPolling(t *testing.T) {
	p := newProvider(&op.Graph{})

	calls := 0
	pred := func(_ any) (bool, error) {
		calls++
		return calls >= 3, nil
	}

	result, err := p.WaitUntil("ready", pred, 5*time.Second, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitUntil: %v", err)
	}
	if result != "ready" {
		t.Errorf("result = %v, want %q", result, "ready")
	}
	if calls < 3 {
		t.Errorf("expected at least 3 predicate calls, got %d", calls)
	}
}

func TestWaitUntilTimeout(t *testing.T) {
	p := newProvider(&op.Graph{})

	_, err := p.WaitUntil("never", func(_ any) (bool, error) { return false, nil }, 50*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected 'timeout' error, got: %v", err)
	}
}

func TestWaitUntilContextCancelled(t *testing.T) {
	baseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	ctx := &op.ExecutionContext{
		Context:  baseCtx,
		Registry: op.NewReceiverRegistry(),
	}
	graph := &op.Graph{}
	graph.Rebind(ctx)
	p := &Provider{
		ProviderBase: op.NewProviderBase(ctx),
		Graph:        graph,
	}

	_, err := p.WaitUntil("never", func(_ any) (bool, error) { return false, nil }, 10*time.Second, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected context error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}
}

func TestWaitUntilZeroTimeout(t *testing.T) {
	p := newProvider(&op.Graph{})

	_, err := p.WaitUntil("ready", func(_ any) (bool, error) { return true, nil }, 0, 0)
	if err == nil {
		t.Fatal("expected error for zero timeout")
	}
	if !strings.Contains(err.Error(), "timeout is required") {
		t.Errorf("expected 'timeout is required' error, got: %v", err)
	}
}

func TestWaitUntilPredicateError(t *testing.T) {
	p := newProvider(&op.Graph{})

	predErr := errors.New("check failed")
	_, err := p.WaitUntil("ready", func(_ any) (bool, error) { return false, predErr }, 5*time.Second, 0)
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
