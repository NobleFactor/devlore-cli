// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package flow

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// testProvider builds a flow.Provider with a minimal RuntimeEnvironment suitable for the saga-shape
// signature tests that don't dispatch into child ExecutableUnits.
func testProvider(t *testing.T) *Provider {
	t.Helper()
	ctx := &op.RuntimeEnvironment{}
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// stubAction is the minimum op.Action implementation needed to construct a Subgraph in tests that
// never dispatch through the action (Do is unreachable from the test paths that use it).
type stubAction struct{}

func (stubAction) FullName() string                                                       { return "stub.action" }
func (stubAction) Name() string                                                            { return "action" }
func (stubAction) Method() *op.Method                                                      { return nil }
func (stubAction) Params() []op.Parameter                                                  { return nil }
func (stubAction) Do(*op.ActivationRecord, map[string]any) (op.Result, op.Complement, error) { return nil, nil, nil }

// subgraphActivation builds an empty Subgraph + an activation pointing at it, suitable for the
// saga-shape tests below. The activation's `dispatchChild` is nil (would be installed by the
// executor on the bound-action path); these tests do not exercise the children-walk.
func subgraphActivation(t *testing.T) *op.ActivationRecord {
	t.Helper()
	subgraph := op.NewSubgraph("test", stubAction{})
	return op.NewActivationRecord(nil, subgraph, &op.RuntimeEnvironment{})
}

func TestChoose_ReturnsRecoveryStack(t *testing.T) {

	p := testProvider(t)

	chosen, stack, err := p.Choose("default-value")
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if chosen != "default-value" {
		t.Errorf("chosen = %v, want \"default-value\"", chosen)
	}
	if stack == nil {
		t.Fatal("Choose() returned nil *RecoveryStack; want empty stack per the saga-shape contract")
	}
	if stack.Len() != 0 {
		t.Errorf("Choose() returned stack with %d entries; want 0 (empty stub stack)", stack.Len())
	}
}

func TestChoose_TruthyCaseReturnsThen(t *testing.T) {

	p := testProvider(t)

	chosen, stack, err := p.Choose("default", Case{When: false, Then: "skip-1"}, Case{When: true, Then: "winner"}, Case{When: true, Then: "skip-2"})
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if chosen != "winner" {
		t.Errorf("chosen = %v, want \"winner\"", chosen)
	}
	if stack == nil {
		t.Fatal("Choose() returned nil *RecoveryStack")
	}
}

func TestCompensateChoose_NilStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateChoose(nil); err != nil {
		t.Errorf("CompensateChoose(nil) error = %v, want nil", err)
	}
}

func TestCompensateChoose_EmptyStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateChoose(op.NewRecoveryStack()); err != nil {
		t.Errorf("CompensateChoose(empty) error = %v, want nil", err)
	}
}

func TestChoose_CompensateChoose_RoundTrip(t *testing.T) {

	p := testProvider(t)

	_, stack, err := p.Choose("default", Case{When: true, Then: "winner"})
	if err != nil {
		t.Fatalf("Choose() error = %v", err)
	}
	if compensateErr := p.CompensateChoose(stack); compensateErr != nil {
		t.Errorf("CompensateChoose() error = %v, want nil (empty-stack unwind is a no-op)", compensateErr)
	}
}

func TestSubgraph_ReturnsRecoveryStack(t *testing.T) {

	p := testProvider(t)

	result, stack, err := p.Subgraph(subgraphActivation(t), nil, nil)
	if err != nil {
		t.Fatalf("Subgraph() error = %v", err)
	}
	if result != nil {
		t.Errorf("Subgraph() returned %v; want nil (container has no terminal output of its own)", result)
	}
	if stack == nil {
		t.Fatal("Subgraph() returned nil *RecoveryStack; want empty stack per the saga-shape contract")
	}
	if stack.Len() != 0 {
		t.Errorf("Subgraph() returned stack with %d entries; want 0 (empty subgraph dispatched zero children)", stack.Len())
	}
}

func TestSubgraph_RejectsItems(t *testing.T) {

	p := testProvider(t)

	_, _, err := p.Subgraph(subgraphActivation(t), []any{1, 2, 3}, nil)
	if err == nil {
		t.Fatal("Subgraph() with non-empty items returned nil error; want \"items iteration not yet implemented\"")
	}
}

func TestCompensateSubgraph_NilStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateSubgraph(nil); err != nil {
		t.Errorf("CompensateSubgraph(nil) error = %v, want nil", err)
	}
}

func TestSubgraph_CompensateSubgraph_RoundTrip(t *testing.T) {

	p := testProvider(t)

	_, stack, err := p.Subgraph(subgraphActivation(t), nil, nil)
	if err != nil {
		t.Fatalf("Subgraph() error = %v", err)
	}
	if compensateErr := p.CompensateSubgraph(stack); compensateErr != nil {
		t.Errorf("CompensateSubgraph() error = %v, want nil (empty-stack unwind is a no-op)", compensateErr)
	}
}

func TestCompensateGather_NilStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateGather(nil); err != nil {
		t.Errorf("CompensateGather(nil) error = %v, want nil", err)
	}
}

func TestCompensateGather_EmptyStack_NoOp(t *testing.T) {

	p := testProvider(t)

	if err := p.CompensateGather(op.NewRecoveryStack()); err != nil {
		t.Errorf("CompensateGather(empty) error = %v, want nil", err)
	}
}